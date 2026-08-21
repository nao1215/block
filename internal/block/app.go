// Package block implements the three operations behind the CLI:
//
//	lock = resolve   block.toml -> block.lock   (the only thing that moves a pin)
//	sync = install   block.lock -> store        (never resolves, never writes the lock)
//	exec = run       store      -> command      (never downloads, never installs)
//
// The cmd package only parses flags; everything observable (files written,
// lines printed, errors returned) is decided here.
package block

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/nao1215/block/internal/fetch"
	"github.com/nao1215/block/internal/lockfile"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/resolver"
	"github.com/nao1215/block/internal/store"
	"github.com/nao1215/block/registry"
)

// ErrOutdated is returned by Lock in check mode when block.lock would change.
var ErrOutdated = errors.New("block.lock is outdated")

// App wires the collaborators a command needs.
type App struct {
	// Dir is the project directory that holds block.toml.
	Dir      string
	Platform platform.Platform
	Registry *registry.Registry
	Releases resolver.Releases
	Fetcher  *fetch.Fetcher
	Store    *store.Store
	Stdout   io.Writer
	Stderr   io.Writer
}

// ManifestPath is the project's block.toml.
func (a *App) ManifestPath() string { return filepath.Join(a.Dir, manifest.FileName) }

// LockPath is the project's block.lock.
func (a *App) LockPath() string { return filepath.Join(a.Dir, lockfile.FileName) }

func (a *App) loadManifest() (*manifest.Manifest, error) {
	m, err := manifest.Load(a.ManifestPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s not found", manifest.FileName)
	}
	return m, err
}

// loadLock returns nil, nil when the lockfile does not exist.
func (a *App) loadLock() (*lockfile.Lock, error) {
	l, err := lockfile.Load(a.LockPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // absence is a normal state here
	}
	return l, err
}

// sourceFor returns the effective recipe source of a manifest tool: the
// project-local definition when present, otherwise the registry's.
func (a *App) sourceFor(t manifest.Tool) (recipe.Source, error) {
	if t.Source != nil {
		return *t.Source, nil
	}
	rec, ok := a.Registry.Lookup(t.Name)
	if !ok {
		return recipe.Source{}, fmt.Errorf("unknown tool %q: it is not in the registry (known tools: %s); define [tools.%s.source] in %s to use it anyway",
			t.Name, strings.Join(a.Registry.Names(), ", "), t.Name, manifest.FileName)
	}
	return rec.Source, nil
}

// lockResult describes what lock decided for one tool.
type lockResult struct {
	name   string
	before string // "" when the tool had no pin
	after  string
}

func (r lockResult) changed() bool { return r.before != r.after }

// Lock resolves block.toml against upstream and writes block.lock. Every
// tool is re-resolved to the newest release its constraint allows; names
// limits re-resolution to those tools and keeps the other pins. In check
// mode nothing is written and ErrOutdated reports that the lock would change.
func (a *App) Lock(ctx context.Context, names []string, check bool) error {
	m, err := a.loadManifest()
	if err != nil {
		return err
	}
	old, err := a.loadLock()
	if err != nil {
		return err
	}
	only := map[string]bool{}
	for _, n := range names {
		if _, ok := m.Tool(n); !ok {
			return fmt.Errorf("tool %q is not declared in %s", n, manifest.FileName)
		}
		only[n] = true
	}
	next := &lockfile.Lock{Version: lockfile.FormatVersion}
	plats := m.EffectivePlatforms(a.Platform)
	var results []lockResult
	for _, t := range m.Tools {
		src, err := a.sourceFor(t)
		if err != nil {
			return err
		}
		var prev *lockfile.Tool
		if old != nil {
			prev, _ = old.Tool(t.Name)
		}
		resolve := len(only) == 0 || only[t.Name]
		entry, res, err := a.lockTool(ctx, t, src, prev, plats, resolve, check)
		if err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
		next.Tools = append(next.Tools, *entry)
		results = append(results, res)
	}
	if check {
		return a.report(results, old, next)
	}
	changed, err := a.writeLock(old, next)
	if err != nil {
		return err
	}
	a.printResults(results, "locked", "")
	if changed {
		fmt.Fprintf(a.Stdout, "wrote %s\n", lockfile.FileName)
	} else {
		fmt.Fprintf(a.Stdout, "%s is up to date\n", lockfile.FileName)
	}
	return nil
}

// report prints the check-mode summary and returns ErrOutdated when the
// lockfile would change: a moved pin, a new tool, a dropped tool, or a
// platform that still needs an artifact.
func (a *App) report(results []lockResult, old, next *lockfile.Lock) error {
	a.printResults(results, "missing", " (up-to-date)")
	outdated := false
	for _, r := range results {
		if r.changed() {
			outdated = true
		}
	}
	if old == nil {
		outdated = true
	} else {
		for _, e := range old.Tools {
			if _, ok := next.Tool(e.Name); !ok {
				fmt.Fprintf(a.Stdout, "%s  %s (no longer in %s)\n", e.Name, e.Version, manifest.FileName)
				outdated = true
			}
		}
		for _, e := range next.Tools {
			if p, ok := old.Tool(e.Name); ok && p.Version == e.Version && len(p.Artifacts) < len(e.Artifacts) {
				outdated = true
			}
		}
	}
	if outdated {
		return ErrOutdated
	}
	return nil
}

// lockTool pins one tool. When resolve is false the previous pin is kept as
// is. In check mode no artifact is downloaded: missing platforms are noted by
// comparing artifact counts in report.
func (a *App) lockTool(ctx context.Context, t manifest.Tool, src recipe.Source, prev *lockfile.Tool, plats []platform.Platform, resolve, check bool) (*lockfile.Tool, lockResult, error) {
	res := lockResult{name: t.Name}
	entry := &lockfile.Tool{Name: t.Name, Constraint: t.Constraint.String(), Bin: append([]string(nil), src.Bin...), Source: src.Hash()}
	if prev != nil {
		res.before = prev.Version
	}
	// The previous artifacts stay valid only for the same version resolved
	// through the same recipe.
	reuse := func() {
		if prev != nil && prev.Version == entry.Version && prev.Source == entry.Source {
			entry.Artifacts = append(entry.Artifacts, prev.Artifacts...)
		}
	}
	var resolution resolver.Resolution
	if !resolve && prev != nil && prev.Constraint == t.Constraint.String() && prev.Source == entry.Source {
		entry.Version = prev.Version
		reuse()
	} else {
		r, err := resolver.Resolve(ctx, a.Releases, src, t.Constraint)
		if err != nil {
			return nil, res, err
		}
		resolution = r
		entry.Version = r.Version.String()
		reuse()
	}
	res.after = entry.Version
	for _, p := range plats {
		if _, ok := entry.Artifact(p); ok {
			continue
		}
		if resolution.Release == nil {
			r, err := a.resolveExact(ctx, src, entry)
			if err != nil {
				return nil, res, err
			}
			resolution = r
		}
		artifactURL, err := resolver.Artifact(resolution, src, p)
		if err != nil {
			return nil, res, err
		}
		if check {
			entry.SetArtifact(lockfile.Artifact{Platform: p.String(), URL: artifactURL, SHA256: strings.Repeat("0", 64)}) //nolint:mnd // placeholder, never written
			continue
		}
		fmt.Fprintf(a.Stderr, "downloading %s\n", artifactURL)
		_, sha, _, err := a.Fetcher.Fetch(ctx, artifactURL, "")
		if err != nil {
			return nil, res, err
		}
		entry.SetArtifact(lockfile.Artifact{Platform: p.String(), URL: artifactURL, SHA256: sha})
	}
	return entry, res, nil
}

// resolveExact re-fetches the release of an already pinned version, needed
// when a new platform is added to a pin that is otherwise kept.
func (a *App) resolveExact(ctx context.Context, src recipe.Source, entry *lockfile.Tool) (resolver.Resolution, error) {
	v, err := entry.ParsedVersion()
	if err != nil {
		return resolver.Resolution{}, err
	}
	r, err := a.Releases.ReleaseByTag(ctx, src.Repo, src.Tag(v))
	if err != nil {
		return resolver.Resolution{}, fmt.Errorf("release %s of %s: %w", src.Tag(v), src.Repo, err)
	}
	return resolver.Resolution{Version: v, Release: r}, nil
}

// writeLock persists next when it differs from old and reports whether it did.
func (a *App) writeLock(old, next *lockfile.Lock) (bool, error) {
	nextData, err := lockfile.Marshal(next)
	if err != nil {
		return false, err
	}
	if old != nil {
		oldData, err := lockfile.Marshal(old)
		if err == nil && string(oldData) == string(nextData) {
			return false, nil
		}
	}
	return true, lockfile.Write(a.LockPath(), next)
}

// printResults prints one line per tool: "name  old -> new", "name  <verb>
// new" for tools without a previous pin, or "name  new<same>" when unchanged.
func (a *App) printResults(results []lockResult, verb, same string) {
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	for _, r := range results {
		var state string
		switch {
		case r.before == "":
			state = verb + " " + r.after
		case r.changed():
			state = r.before + " -> " + r.after
		default:
			state = r.after + same
		}
		fmt.Fprintf(tw, "%s\t%s\n", r.name, state)
	}
	_ = tw.Flush()
}

// Check compares a manifest with a lockfile and lists every reason the
// lockfile cannot be trusted for the given platforms. An empty result means
// the lockfile is current. It needs no network and no registry beyond the
// recipe fingerprint.
func Check(m *manifest.Manifest, l *lockfile.Lock, sources map[string]recipe.Source, plats []platform.Platform) []string {
	var reasons []string
	for _, t := range m.Tools {
		e, ok := l.Tool(t.Name)
		if !ok {
			reasons = append(reasons, fmt.Sprintf("%s is declared in %s but missing from %s", t.Name, manifest.FileName, lockfile.FileName))
			continue
		}
		if e.Constraint != t.Constraint.String() {
			reasons = append(reasons, fmt.Sprintf("%s: %s wants %q but %s was resolved from %q", t.Name, manifest.FileName, t.Constraint, lockfile.FileName, e.Constraint))
		}
		if src, ok := sources[t.Name]; ok && src.Hash() != e.Source {
			reasons = append(reasons, fmt.Sprintf("%s: the source definition changed since %s was resolved", t.Name, lockfile.FileName))
		}
		for _, p := range plats {
			if _, ok := e.Artifact(p); !ok {
				reasons = append(reasons, fmt.Sprintf("%s: %s has no artifact for %s", t.Name, lockfile.FileName, p))
			}
		}
	}
	for _, e := range l.Tools {
		if _, ok := m.Tool(e.Name); !ok {
			reasons = append(reasons, fmt.Sprintf("%s is in %s but not declared in %s", e.Name, lockfile.FileName, manifest.FileName))
		}
	}
	return reasons
}

func staleError(reasons []string) error {
	return fmt.Errorf("%s is stale; run \"block lock\"\n  %s", lockfile.FileName, strings.Join(reasons, "\n  "))
}

// Sync installs every tool in block.lock for the current platform. It never
// resolves a version and never writes block.lock: a missing or stale lock, a
// missing platform artifact or a checksum mismatch is an error.
func (a *App) Sync(ctx context.Context) error {
	m, err := a.loadManifest()
	if err != nil {
		return err
	}
	l, err := a.loadLock()
	if err != nil {
		return err
	}
	if l == nil {
		return fmt.Errorf("%s not found; run \"block lock\"", lockfile.FileName)
	}
	if !a.Platform.IsSupported() {
		return fmt.Errorf("unsupported platform %s", a.Platform)
	}
	sources := map[string]recipe.Source{}
	for _, t := range m.Tools {
		if src, err := a.sourceFor(t); err == nil {
			sources[t.Name] = src
		}
	}
	if reasons := Check(m, l, sources, []platform.Platform{a.Platform}); len(reasons) > 0 {
		return staleError(reasons)
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	for _, t := range l.Tools {
		state, err := a.install(ctx, &t)
		if err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, t.Version, state)
	}
	return tw.Flush()
}

// install makes one locked tool available and reports "installed" or "cached".
func (a *App) install(ctx context.Context, t *lockfile.Tool) (string, error) {
	art, ok := t.Artifact(a.Platform)
	if !ok {
		return "", fmt.Errorf("%s has no artifact for %s", lockfile.FileName, a.Platform)
	}
	dir := a.Store.InstallDir(t.Name, t.Version, art.SHA256)
	if a.Store.IsInstalled(dir) {
		return "cached", nil
	}
	blob, _, _, err := a.Fetcher.Fetch(ctx, art.URL, art.SHA256)
	if err != nil {
		return "", err
	}
	if err := a.Store.Install(blob, assetName(art.URL), dir, t.Bin); err != nil {
		return "", err
	}
	return "installed", nil
}

// assetName extracts the file name from a download URL so the archive format
// can be detected from its extension.
func assetName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return path.Base(u.Path)
}

// Env returns the PATH entries that expose every locked tool for the current
// platform. It fails when a tool has not been synced.
func (a *App) Env() ([]string, error) {
	l, err := a.loadLock()
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, fmt.Errorf("%s not found; run \"block lock\" and \"block sync\"", lockfile.FileName)
	}
	var dirs []string
	for _, t := range l.Tools {
		art, ok := t.Artifact(a.Platform)
		if !ok {
			return nil, fmt.Errorf("%s: %s has no artifact for %s; run \"block lock\" and \"block sync\"", t.Name, lockfile.FileName, a.Platform)
		}
		dir := a.Store.InstallDir(t.Name, t.Version, art.SHA256)
		if !a.Store.IsInstalled(dir) {
			return nil, fmt.Errorf("%s %s is not installed; run \"block sync\"", t.Name, t.Version)
		}
		dirs = append(dirs, store.BinDirs(dir, t.Bin)...)
	}
	return dirs, nil
}
