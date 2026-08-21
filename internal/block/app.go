// Package block implements the use cases behind each CLI command. The cmd
// package only parses flags; everything observable (files written, lines
// printed, errors returned) is decided here so that it can be driven from
// tests without a process boundary.
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

// Init writes a starter block.toml into Dir. It refuses to overwrite.
func (a *App) Init() error {
	path := a.ManifestPath()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", manifest.FileName)
	}
	const mode = 0o644
	if err := os.WriteFile(path, manifest.Template(), mode); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "created %s\n", manifest.FileName)
	return nil
}

func (a *App) loadManifest() (*manifest.Manifest, error) {
	m, err := manifest.Load(a.ManifestPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s not found (run \"block init\" to create one)", manifest.FileName)
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

// lockOptions steers Lock between the conservative default and an update.
type lockOptions struct {
	// update lists tools whose version must be re-resolved even though the
	// pinned one still satisfies the constraint. Empty with updateAll false
	// keeps every satisfied pin.
	update    map[string]bool
	updateAll bool
	// extraPlatforms are locked in addition to the manifest's platforms.
	extraPlatforms []platform.Platform
}

// lockResult describes what changed for one tool.
type lockResult struct {
	name     string
	before   string // "" when newly locked
	after    string
	newPlats []string
}

// lock resolves the manifest into a lockfile, reusing every pin that still
// satisfies its constraint unless opts asks for an update.
func (a *App) lock(ctx context.Context, m *manifest.Manifest, old *lockfile.Lock, opts lockOptions) (*lockfile.Lock, []lockResult, error) {
	next := &lockfile.Lock{Version: lockfile.FormatVersion}
	plats := m.EffectivePlatforms(a.Platform)
	for _, p := range opts.extraPlatforms {
		if !containsPlatform(plats, p) {
			plats = append(plats, p)
		}
	}
	platform.Sort(plats)
	var results []lockResult
	for _, t := range m.Tools {
		src, err := a.sourceFor(t)
		if err != nil {
			return nil, nil, err
		}
		// prev is the old pin when one exists; it is reusable only when it
		// was resolved from the same constraint and source and still
		// satisfies the constraint.
		var prev *lockfile.Tool
		reusable := false
		if old != nil {
			if e, ok := old.Tool(t.Name); ok {
				prev = e
				v, err := e.ParsedVersion()
				reusable = err == nil && e.Constraint == t.Constraint.String() && e.Source.Equal(src) && t.Constraint.Matches(v)
			}
		}
		update := opts.updateAll || opts.update[t.Name]
		entry, res, err := a.lockTool(ctx, t, src, prev, plats, !reusable || update)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		next.Tools = append(next.Tools, *entry)
		results = append(results, res)
	}
	return next, results, nil
}

// lockTool pins one tool. prev is the previous pin for display and artifact
// reuse; resolve forces a fresh upstream resolution instead of keeping prev.
func (a *App) lockTool(ctx context.Context, t manifest.Tool, src recipe.Source, prev *lockfile.Tool, plats []platform.Platform, resolve bool) (*lockfile.Tool, lockResult, error) {
	res := lockResult{name: t.Name}
	entry := &lockfile.Tool{Name: t.Name, Constraint: t.Constraint.String(), Source: src}
	if prev != nil {
		res.before = prev.Version
	}
	var resolution resolver.Resolution
	switch {
	case prev != nil && !resolve:
		entry.Version = prev.Version
		entry.Artifacts = append(entry.Artifacts, prev.Artifacts...)
	default:
		r, err := resolver.Resolve(ctx, a.Releases, src, t.Constraint)
		if err != nil {
			return nil, res, err
		}
		resolution = r
		entry.Version = r.Version.String()
		// Artifacts of an identical pin resolved from the same source are
		// still valid: keep them instead of re-downloading.
		if prev != nil && prev.Version == entry.Version && prev.Source.Equal(src) {
			entry.Artifacts = append(entry.Artifacts, prev.Artifacts...)
		}
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
		url, err := resolver.Artifact(resolution, src, p)
		if err != nil {
			return nil, res, err
		}
		fmt.Fprintf(a.Stderr, "downloading %s\n", url)
		_, sha, _, err := a.Fetcher.Fetch(ctx, url, "")
		if err != nil {
			return nil, res, err
		}
		entry.SetArtifact(lockfile.Artifact{Platform: p.String(), URL: url, SHA256: sha})
		res.newPlats = append(res.newPlats, p.String())
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

func containsPlatform(ps []platform.Platform, p platform.Platform) bool {
	for _, x := range ps {
		if x == p {
			return true
		}
	}
	return false
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

// Lock resolves block.toml into block.lock. Pins that still satisfy their
// constraint are kept; only new tools, changed constraints and missing
// platforms are resolved.
func (a *App) Lock(ctx context.Context) error {
	m, err := a.loadManifest()
	if err != nil {
		return err
	}
	old, err := a.loadLock()
	if err != nil {
		return err
	}
	next, results, err := a.lock(ctx, m, old, lockOptions{})
	if err != nil {
		return err
	}
	changed, err := a.writeLock(old, next)
	if err != nil {
		return err
	}
	a.printLockResults(results, "locked")
	if changed {
		fmt.Fprintf(a.Stdout, "wrote %s\n", lockfile.FileName)
	} else {
		fmt.Fprintf(a.Stdout, "%s is up to date\n", lockfile.FileName)
	}
	return nil
}

// Update re-resolves the named tools (all when names is empty) to the newest
// versions that satisfy block.toml and rewrites block.lock.
func (a *App) Update(ctx context.Context, names []string) error {
	m, err := a.loadManifest()
	if err != nil {
		return err
	}
	old, err := a.loadLock()
	if err != nil {
		return err
	}
	opts := lockOptions{updateAll: len(names) == 0, update: map[string]bool{}}
	for _, n := range names {
		if _, ok := m.Tool(n); !ok {
			return fmt.Errorf("tool %q is not declared in %s", n, manifest.FileName)
		}
		opts.update[n] = true
	}
	next, results, err := a.lock(ctx, m, old, opts)
	if err != nil {
		return err
	}
	changed, err := a.writeLock(old, next)
	if err != nil {
		return err
	}
	a.printLockResults(results, "updated")
	if changed {
		fmt.Fprintf(a.Stdout, "wrote %s\n", lockfile.FileName)
	} else {
		fmt.Fprintf(a.Stdout, "%s is up to date\n", lockfile.FileName)
	}
	return nil
}

func (a *App) printLockResults(results []lockResult, verb string) {
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	for _, r := range results {
		var state string
		switch {
		case r.before == "":
			state = verb + " " + r.after
		case r.before != r.after:
			state = r.before + " -> " + r.after
		default:
			state = r.after + " (unchanged)"
		}
		if len(r.newPlats) > 0 {
			state += "  +" + strings.Join(r.newPlats, ", ")
		}
		fmt.Fprintf(tw, "%s\t%s\n", r.name, state)
	}
	_ = tw.Flush()
}

// Outdated reports, for every locked tool, the newest upstream version that
// still satisfies its constraint. It never writes anything.
func (a *App) Outdated(ctx context.Context) error {
	m, err := a.loadManifest()
	if err != nil {
		return err
	}
	l, err := a.loadLock()
	if err != nil {
		return err
	}
	if l == nil {
		return fmt.Errorf("%s not found (run \"block lock\" first)", lockfile.FileName)
	}
	if reasons := Check(m, l, nil); len(reasons) > 0 {
		return staleError(reasons)
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	outdated := 0
	for _, t := range m.Tools {
		entry, _ := l.Tool(t.Name)
		src, err := a.sourceFor(t)
		if err != nil {
			return err
		}
		res, err := resolver.Resolve(ctx, a.Releases, src, t.Constraint)
		if err != nil {
			return fmt.Errorf("%s: %w", t.Name, err)
		}
		if res.Version.String() != entry.Version {
			outdated++
			fmt.Fprintf(tw, "%s\t%s -> %s\n", t.Name, entry.Version, res.Version)
		}
	}
	_ = tw.Flush()
	if outdated == 0 {
		fmt.Fprintln(a.Stdout, "all tools are up to date")
	}
	return nil
}

// Check compares a manifest with a lockfile and lists every reason the
// lockfile cannot be trusted for the given platforms. An empty result means
// the lockfile is current.
func Check(m *manifest.Manifest, l *lockfile.Lock, plats []platform.Platform) []string {
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
		if t.Source != nil && !t.Source.Equal(e.Source) {
			reasons = append(reasons, fmt.Sprintf("%s: the source in %s differs from the one in %s", t.Name, manifest.FileName, lockfile.FileName))
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
	return fmt.Errorf("%s is out of date with %s:\n  %s\nrun \"block lock\" to update it", lockfile.FileName, manifest.FileName, strings.Join(reasons, "\n  "))
}

// Sync installs every tool in block.lock for the current platform. Without
// locked, a missing or stale lockfile is (re)locked first and the current
// platform is added when absent. With locked, any of those conditions is an
// error and nothing is resolved.
func (a *App) Sync(ctx context.Context, locked bool) error {
	m, err := a.loadManifest()
	if err != nil {
		return err
	}
	l, err := a.loadLock()
	if err != nil {
		return err
	}
	if !a.Platform.IsSupported() {
		return fmt.Errorf("unsupported platform %s", a.Platform)
	}
	here := []platform.Platform{a.Platform}
	switch {
	case locked && l == nil:
		return fmt.Errorf("%s not found (run \"block lock\" and commit the result)", lockfile.FileName)
	case locked:
		if reasons := Check(m, l, here); len(reasons) > 0 {
			return fmt.Errorf("--locked: %w", staleError(reasons))
		}
	case l == nil || len(Check(m, l, here)) > 0:
		next, _, err := a.lock(ctx, m, l, lockOptions{extraPlatforms: here})
		if err != nil {
			return err
		}
		if _, err := a.writeLock(l, next); err != nil {
			return err
		}
		l = next
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
		return "", fmt.Errorf("%s has no artifact for %s (add it to platforms in %s and run \"block lock\")", lockfile.FileName, a.Platform, manifest.FileName)
	}
	dir := a.Store.InstallDir(t.Name, t.Version, art.SHA256)
	if a.Store.IsInstalled(dir) {
		return "cached", nil
	}
	path, _, _, err := a.Fetcher.Fetch(ctx, art.URL, art.SHA256)
	if err != nil {
		return "", err
	}
	if err := a.Store.Install(path, assetName(art.URL), dir, t.Source.Bin); err != nil {
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
		return nil, fmt.Errorf("%s not found (run \"block sync\" first)", lockfile.FileName)
	}
	var dirs []string
	for _, t := range l.Tools {
		art, ok := t.Artifact(a.Platform)
		if !ok {
			return nil, fmt.Errorf("%s: %s has no artifact for %s", t.Name, lockfile.FileName, a.Platform)
		}
		dir := a.Store.InstallDir(t.Name, t.Version, art.SHA256)
		if !a.Store.IsInstalled(dir) {
			return nil, fmt.Errorf("%s %s is not installed (run \"block sync\")", t.Name, t.Version)
		}
		dirs = append(dirs, store.BinDirs(dir, t.Source.Bin)...)
	}
	return dirs, nil
}
