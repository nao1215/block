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
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/fetch"
	"github.com/nao1215/block/internal/lockfile"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/resolver"
	"github.com/nao1215/block/internal/shim"
	"github.com/nao1215/block/internal/store"
	"github.com/nao1215/block/internal/version"
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
	// Self is the block binary the shims point at. Empty means "ask the
	// operating system", which is what every real run does.
	Self   string
	Stdout io.Writer
	Stderr io.Writer
}

// ManifestPath is the project's block.toml.
func (a *App) ManifestPath() string { return filepath.Join(a.Dir, manifest.FileName) }

// LockPath is the project's block.lock.
func (a *App) LockPath() string { return filepath.Join(a.Dir, lockfile.FileName) }

func (a *App) loadManifest() (*manifest.Manifest, error) {
	m, err := manifest.Load(a.ManifestPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, diag.ManifestMissing.Errorf("%s not found", manifest.FileName)
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
		return recipe.Source{}, diag.UnknownTool.Errorf("unknown tool %q: it is not in the registry (run \"block list\" to see the supported tools); define [tools.%s.source] in %s to use it anyway",
			t.Name, t.Name, manifest.FileName)
	}
	return rec.Source, nil
}

// manifestCommandConflict reports an ambiguous toolchain from block.toml and
// the registry alone — no network, no lockfile. It is the same rule
// [commandConflict] applies to a lockfile, moved to the earliest point it can
// be answered, so `block lock` refuses before it downloads anything.
func (a *App) manifestCommandConflict(m *manifest.Manifest) error {
	var claimed commandSet
	for _, t := range m.Tools {
		src, err := a.sourceFor(t)
		if err != nil {
			return err
		}
		if err := claimed.add(t.Name, src.Bin); err != nil {
			return err
		}
	}
	return nil
}

// commandSet accumulates which tool provides which command, and is where the
// refusal below is actually written. Two callers ask the same question of two
// different things — a manifest plus the registry, and a lockfile — and the
// answer has to be the same one, phrased the same way, or a toolchain could
// be refused at lock time and accepted at sync time.
type commandSet struct {
	seen map[string]commandOwner
}

type commandOwner struct{ tool, bin string }

// add records one tool's executables, or reports the collision they cause.
//
// The comparison is case-insensitive on every platform: see
// [recipe.CommandKey].
func (c *commandSet) add(tool string, bins []string) error {
	if c.seen == nil {
		c.seen = map[string]commandOwner{}
	}
	for _, b := range bins {
		key := recipe.CommandKey(b)
		first, ok := c.seen[key]
		switch {
		case !ok:
			c.seen[key] = commandOwner{tool: tool, bin: b}
		case first.tool != tool:
			return diag.CommandConflict.Errorf("tools %q and %q both provide the command %q; remove one from %s",
				first.tool, tool, recipe.CommandName(b), manifest.FileName)
		default:
			return diag.CommandConflict.Errorf("tool %q lists %q and %q, which are both the command %q",
				tool, first.bin, b, recipe.CommandName(b))
		}
	}
	return nil
}

// lockResult describes what lock decided for one tool.
type lockResult struct {
	name   string
	before string // "" when the tool had no pin
	after  string
	// changes lists the differences from the previous pin other than the
	// version: a changed constraint, executables, or artifacts. They matter
	// because block.lock is rewritten for any of them, not only for a new
	// version.
	changes []string
}

func (r lockResult) moved() bool { return r.before != "" && r.before != r.after }

// differs reports whether writing the plan would change this tool's pin.
func (r lockResult) differs() bool { return r.before == "" || r.moved() || len(r.changes) > 0 }

// Lock resolves block.toml against upstream and writes block.lock. Every
// tool is re-resolved to the newest release its constraint allows; names
// limits re-resolution to those tools and keeps the other pins. In check
// mode nothing is written and nothing is downloaded, and ErrOutdated reports
// that block.lock would change.
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
			return diag.ToolNotDeclared.Errorf("tool %q is not declared in %s", n, manifest.FileName)
		}
		only[n] = true
	}
	// Before any resolution: the commands a toolchain will provide are known
	// from the manifest and the registry alone, so an ambiguous one is
	// refused offline rather than after downloading artifacts for it.
	if err := a.manifestCommandConflict(m); err != nil {
		return err
	}
	next, results, err := a.plan(ctx, m, old, only, check)
	if err != nil {
		return err
	}
	// And again on the plan, which is what actually gets written: a
	// project-local source can change a tool's executables between the two.
	if err := commandConflict(next); err != nil {
		return err
	}
	dropped := droppedTools(old, next)
	if check {
		a.printResults(results, dropped, "missing", true)
		for _, r := range results {
			if r.differs() {
				return ErrOutdated
			}
		}
		if len(dropped) > 0 || old == nil {
			return ErrOutdated
		}
		return nil
	}
	changed, err := a.writeLock(old, next)
	if err != nil {
		return err
	}
	a.printResults(results, dropped, "locked", false)
	if changed {
		fmt.Fprintf(a.Stdout, "wrote %s\n", lockfile.FileName)
	} else {
		fmt.Fprintf(a.Stdout, "%s is up to date\n", lockfile.FileName)
	}
	return nil
}

// plan builds the lockfile block lock would write. In check mode nothing is
// downloaded: an artifact whose digest could only be learned by downloading
// is left empty, which the comparison reports as a change — it is one, since
// such an artifact is always a new or moved URL.
func (a *App) plan(ctx context.Context, m *manifest.Manifest, old *lockfile.Lock, only map[string]bool, check bool) (*lockfile.Lock, []lockResult, error) {
	next := &lockfile.Lock{Version: lockfile.FormatVersion}
	var results []lockResult
	for _, t := range m.Tools {
		src, err := a.sourceFor(t)
		if err != nil {
			return nil, nil, err
		}
		var prev *lockfile.Tool
		if old != nil {
			prev, _ = old.Tool(t.Name)
		}
		entry, err := a.lockTool(ctx, t, src, prev, a.platformsFor(m, prev, src), len(only) == 0 || only[t.Name], check)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		next.Tools = append(next.Tools, *entry)
		results = append(results, result(prev, entry))
	}
	next.Sort()
	return next, results, nil
}

// platformsFor decides which platforms a tool is locked for.
//
// What block.toml asks for is required: an explicit platforms list, or this
// machine when there is none. A tool that does not ship for one of those is
// an error, not something to quietly skip.
//
// Without an explicit list the manifest only says "this machine", so the
// platforms an existing pin already covers are carried along as well —
// locking on a laptop must not drop the artifact CI needs. Those inherited
// platforms are optional: when the upstream stops shipping one, it is
// dropped with a notice instead of failing the lock.
func (a *App) platformsFor(m *manifest.Manifest, prev *lockfile.Tool, src recipe.Source) []platform.Platform {
	out := m.EffectivePlatforms(a.Platform)
	if len(m.Platforms) == 0 && prev != nil {
		for _, art := range prev.Artifacts {
			p, err := platform.Parse(art.Platform)
			switch {
			case err != nil, slices.Contains(out, p):
				continue
			case !src.Supports(p):
				fmt.Fprintf(a.Stderr, "%s: dropping %s: the source no longer ships it\n", prev.Name, p)
				continue
			}
			out = append(out, p)
		}
	}
	platform.Sort(out)
	return out
}

// lockTool pins one tool. With resolve set, the tool is re-resolved against
// upstream and every artifact is re-rendered from the current recipe, so a
// recipe that renamed an asset takes effect even when the version is
// unchanged. Without it the previous pin is kept verbatim — that is what
// naming other tools on the command line means — except that a platform it
// does not cover yet is resolved at the pinned version.
func (a *App) lockTool(ctx context.Context, t manifest.Tool, src recipe.Source, prev *lockfile.Tool, plats []platform.Platform, resolve, check bool) (*lockfile.Tool, error) {
	entry := &lockfile.Tool{
		Name:            t.Name,
		Constraint:      t.Constraint.String(),
		Bin:             append([]string(nil), src.Bin...),
		StripComponents: src.StripComponents,
	}
	if t.Source != nil {
		entry.Source = src.Hash()
	}
	var resolution resolver.Resolution
	// resolved says whether resolution describes entry.Version; a kept pin
	// leaves it unset until a missing platform forces an exact lookup.
	resolved := false
	keep := !resolve && prev != nil && prev.Constraint == entry.Constraint && prev.Source == entry.Source
	switch {
	case keep:
		entry.Version = prev.Version
		entry.Bin = append([]string(nil), prev.Bin...)
		entry.StripComponents = prev.StripComponents
		for _, p := range plats {
			if art, ok := prev.Artifact(p); ok {
				entry.SetArtifact(art)
			}
		}
	default:
		r, err := a.resolve(ctx, src, t.Constraint)
		if err != nil {
			return nil, err
		}
		resolution, resolved = r, true
		// What the lockfile records is never the thing block.toml asked for
		// when that thing moves: a channel resolves to the tag under it.
		entry.Version = r.Identity()
	}
	// Whether the repository is private is asked once per tool, and only
	// when an artifact is actually being pinned: a kept pin already says.
	private, privateKnown := false, false
	for _, p := range plats {
		if _, ok := entry.Artifact(p); ok {
			continue
		}
		if !resolved {
			r, err := a.resolveExact(ctx, src, t.Constraint, entry)
			if err != nil {
				return nil, err
			}
			resolution, resolved = r, true
		}
		art, err := resolver.ArtifactFor(resolution, src, p)
		if err != nil {
			return nil, err
		}
		// An artifact the upstream already says is too large is refused
		// here, before a pin nothing could ever install is written.
		if err := a.Fetcher.CheckSize(art.URL, art.Size); err != nil {
			return nil, err
		}
		pinned := lockfile.Artifact{Platform: p.String(), URL: art.URL}
		if art.APIURL != "" && !privateKnown {
			if private, err = a.Releases.Private(ctx, src.Repo); err != nil {
				return nil, err
			}
			privateKnown = true
		}
		if private {
			pinned.APIURL = art.APIURL
		}
		sha, err := a.digestFor(ctx, prev, pinned, art.SHA256, check)
		if err != nil {
			return nil, err
		}
		pinned.SHA256 = sha
		entry.SetArtifact(pinned)
	}
	return entry, nil
}

// digestFor settles an artifact's checksum without downloading more than it
// must: the upstream's own digest when it publishes one, else the digest
// already locked for the very same URL, else — outside check mode — the
// digest of one download.
func (a *App) digestFor(ctx context.Context, prev *lockfile.Tool, art lockfile.Artifact, upstream string, check bool) (string, error) {
	if upstream != "" {
		return upstream, nil
	}
	if prev != nil {
		if old, ok := prevArtifact(prev, art.Platform); ok && old.URL == art.URL {
			return old.SHA256, nil
		}
	}
	if check {
		// Unknown without downloading, and only ever unknown for an artifact
		// that is new or has moved: reported as a change either way.
		return "", nil
	}
	fmt.Fprintf(a.Stderr, "downloading %s\n", art.URL)
	_, sha, err := a.fetchArtifact(ctx, art, "")
	if err != nil {
		return "", err
	}
	return sha, nil
}

// fetchArtifact downloads one artifact and returns the cached blob and its
// digest. A public artifact comes from the URL block.lock records. A private
// release asset cannot: that URL answers a browser session and nothing else,
// so the lockfile also records the asset's API URL, and the download goes
// there with the token — to the host that already saw the token when the
// release was resolved, and to no other. Without a token for that host the
// download is refused up front, rather than tried and reported as "not
// found", which is what the upstream would say and not what is wrong.
func (a *App) fetchArtifact(ctx context.Context, art lockfile.Artifact, want string) (path, sha string, err error) {
	from := art.URL
	if art.APIURL != "" {
		if !a.Fetcher.Credential.Allows(art.APIURL) {
			return "", "", diag.DownloadFailed.Errorf("download %s: it is a private release asset, and no GITHUB_TOKEN (or GH_TOKEN) is set for %s", art.URL, assetHost(art.APIURL))
		}
		from = art.APIURL
	}
	path, sha, _, err = a.Fetcher.Fetch(ctx, from, want)
	return path, sha, err
}

// assetHost names the host an API URL is served by, for a message.
func assetHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}

// resolve turns a constraint into a release: the newest version it allows, or
// — for a channel — whatever that channel points at right now, pinned to the
// tag under it.
func (a *App) resolve(ctx context.Context, src recipe.Source, c version.Constraint) (resolver.Resolution, error) {
	if c.IsChannel() {
		return resolver.ResolveChannel(ctx, a.Releases, src, c.Channel())
	}
	return resolver.Resolve(ctx, a.Releases, src, c)
}

// resolveExact re-resolves an already pinned release, needed when a new
// platform is added to a pin that is otherwise kept. It never moves the pin:
// a channel is re-read by the tag the lockfile records, not by the tag that
// moves.
func (a *App) resolveExact(ctx context.Context, src recipe.Source, c version.Constraint, entry *lockfile.Tool) (resolver.Resolution, error) {
	if c.IsChannel() {
		return resolver.ExactTag(ctx, a.Releases, src, c.Channel(), entry.Version)
	}
	v, err := entry.ParsedVersion()
	if err != nil {
		return resolver.Resolution{}, err
	}
	return resolver.Exact(ctx, a.Releases, src, v)
}

// result compares a planned pin with the previous one. Everything block.lock
// records is compared, not just the version: a lock that would be rewritten
// must be reported as such.
func result(prev, next *lockfile.Tool) lockResult {
	res := lockResult{name: next.Name, after: next.Version}
	if prev == nil {
		return res
	}
	res.before = prev.Version
	// A constraint that moved the version has already said so on the version
	// line; one that did not is exactly the change a version comparison
	// misses.
	if prev.Constraint != next.Constraint && prev.Version == next.Version {
		res.changes = append(res.changes, "constraint")
	}
	if strings.Join(prev.Bin, "\x00") != strings.Join(next.Bin, "\x00") {
		res.changes = append(res.changes, "bin")
	}
	if prev.StripComponents != next.StripComponents {
		res.changes = append(res.changes, "strip_components")
	}
	if prev.Source != next.Source {
		res.changes = append(res.changes, "source")
	}
	for _, c := range artifactChanges(prev, next) {
		// A new version moves every artifact; saying so adds nothing to the
		// version line. A platform appearing or disappearing does.
		if res.moved() && !strings.HasSuffix(c, " added") && !strings.HasSuffix(c, " removed") {
			continue
		}
		res.changes = append(res.changes, c)
	}
	return res
}

// artifactChanges lists the per-platform artifact differences between two
// pins. An empty planned digest means "only a download could tell", which
// happens exactly when the URL is new or has moved.
func artifactChanges(prev, next *lockfile.Tool) []string {
	var out []string
	seen := map[string]bool{}
	for _, a := range next.Artifacts {
		seen[a.Platform] = true
		before, ok := prevArtifact(prev, a.Platform)
		switch {
		case !ok:
			out = append(out, "artifact for "+a.Platform+" added")
		case before.URL != a.URL, before.APIURL != a.APIURL, a.SHA256 == "", before.SHA256 != a.SHA256:
			out = append(out, "artifact for "+a.Platform)
		}
	}
	for _, a := range prev.Artifacts {
		if !seen[a.Platform] {
			out = append(out, "artifact for "+a.Platform+" removed")
		}
	}
	sort.Strings(out)
	return out
}

func prevArtifact(prev *lockfile.Tool, platformName string) (lockfile.Artifact, bool) {
	for _, a := range prev.Artifacts {
		if a.Platform == platformName {
			return a, true
		}
	}
	return lockfile.Artifact{}, false
}

// droppedTools lists the pins that are in the lockfile but no longer in the
// manifest.
func droppedTools(old, next *lockfile.Lock) []lockfile.Tool {
	if old == nil {
		return nil
	}
	var out []lockfile.Tool
	for _, t := range old.Tools {
		if _, ok := next.Tool(t.Name); !ok {
			out = append(out, t)
		}
	}
	return out
}

// commandConflict refuses a toolchain in which one command name could mean
// two executables. Which one PATH order happens to pick is not something a
// project should depend on — and it is not even stable, because a shim
// resolves the command through the lockfile while PATH resolves it by
// directory order, so the two can disagree inside one toolchain.
func commandConflict(l *lockfile.Lock) error {
	var claimed commandSet
	for _, t := range l.Tools {
		if err := claimed.add(t.Name, t.Bin); err != nil {
			return err
		}
	}
	return nil
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

// printResults prints one line per tool: the version it resolved to, how it
// moved, and what else about the pin would change. The pins that are on their
// way out are printed too, in the same column layout — a tool removed from
// block.toml loses its pin when the file is written, and a run that says only
// what it kept does not say what it did.
func (a *App) printResults(results []lockResult, dropped []lockfile.Tool, verb string, check bool) {
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	defer func() { _ = tw.Flush() }()
	defer func() {
		for _, t := range dropped {
			fmt.Fprintf(tw, "%s\t%s (no longer in %s)\n", t.Name, t.Version, manifest.FileName)
		}
	}()
	for _, r := range results {
		var state string
		switch {
		case r.before == "":
			state = verb + " " + r.after
		case r.moved():
			state = r.before + " -> " + r.after
		default:
			state = r.after
		}
		switch {
		case len(r.changes) > 0:
			state += " (" + strings.Join(r.changes, ", ") + ")"
		case check && !r.differs():
			state += " (up-to-date)"
		}
		fmt.Fprintf(tw, "%s\t%s\n", r.name, state)
	}
}

// Disagreement is one way a lockfile fails to describe what block.toml asks
// for. It carries the same finding twice because two commands need it in two
// shapes: sync and exec refuse with the whole sentence, and status has a
// column to put it in.
type Disagreement struct {
	// Tool is the tool the disagreement is about.
	Tool string
	// Short names it in a few words, for a table cell.
	Short string
	// Long says the whole thing, for a refusal.
	Long string
}

// Check compares a manifest with a lockfile and lists every reason the
// lockfile cannot be trusted for the given platforms. An empty result means
// the lockfile is current. It needs no network and no registry: only a
// project-local source is fingerprinted, so registry changes never stale a
// lock.
func Check(m *manifest.Manifest, l *lockfile.Lock, plats []platform.Platform) []string {
	found := Disagreements(m, l, plats)
	reasons := make([]string, 0, len(found))
	for _, d := range found {
		reasons = append(reasons, d.Long)
	}
	return reasons
}

// Disagreements is [Check] with the findings kept apart, so that a caller
// reporting them per tool does not have to take them apart again.
func Disagreements(m *manifest.Manifest, l *lockfile.Lock, plats []platform.Platform) []Disagreement {
	var found []Disagreement
	for _, t := range m.Tools {
		found = append(found, DisagreementsFor(t, l, plats)...)
	}
	for _, e := range l.Tools {
		if _, ok := m.Tool(e.Name); !ok {
			found = append(found, Disagreement{
				Tool:  e.Name,
				Short: "not in " + manifest.FileName,
				Long:  fmt.Sprintf("%s is in %s but not declared in %s", e.Name, lockfile.FileName, manifest.FileName),
			})
		}
	}
	return found
}

// DisagreementsFor lists what one declared tool disagrees with the lockfile
// about. A tool with nothing to report is one sync can install.
func DisagreementsFor(t manifest.Tool, l *lockfile.Lock, plats []platform.Platform) []Disagreement {
	e, ok := l.Tool(t.Name)
	if !ok {
		return []Disagreement{{
			Tool:  t.Name,
			Short: "not in " + lockfile.FileName,
			Long:  fmt.Sprintf("%s is declared in %s but missing from %s", t.Name, manifest.FileName, lockfile.FileName),
		}}
	}
	var found []Disagreement
	if e.Constraint != t.Constraint.String() {
		found = append(found, Disagreement{
			Tool:  t.Name,
			Short: "constraint changed",
			Long:  fmt.Sprintf("%s: %s wants %q but %s was resolved from %q", t.Name, manifest.FileName, t.Constraint, lockfile.FileName, e.Constraint),
		})
	}
	// The fingerprint the manifest asks for now, against the one the pin was
	// resolved from. A registry tool has none — that is what the empty string
	// means in a lockfile — so taking the source out of block.toml is a
	// change like any other: without comparing both directions, a tool moved
	// from its own [tools.x.source] to the registry would keep installing the
	// artifact the removed source chose.
	want := ""
	if t.Source != nil {
		want = t.Source.Hash()
	}
	if want != e.Source {
		found = append(found, Disagreement{
			Tool:  t.Name,
			Short: "source changed",
			Long:  fmt.Sprintf("%s: the source definition changed since %s was resolved", t.Name, lockfile.FileName),
		})
	}
	for _, p := range plats {
		if _, ok := e.Artifact(p); !ok {
			found = append(found, Disagreement{
				Tool:  t.Name,
				Short: "no artifact for " + p.String(),
				Long:  fmt.Sprintf("%s: %s has no artifact for %s", t.Name, lockfile.FileName, p),
			})
		}
	}
	return found
}

// platformNotDeclared reports the error to give when block.toml names the
// platforms the project supports and this machine is not one of them.
//
// The lockfile then has no artifact for this machine and never will: `block
// lock` resolves exactly the platforms the manifest asks for, so telling the
// reader the lock is stale would send them round a loop where the command
// they are told to run writes the same file back. What has to change is
// block.toml, and that is what this says.
func platformNotDeclared(m *manifest.Manifest, p platform.Platform) error {
	if len(m.Platforms) == 0 || slices.Contains(m.Platforms, p) {
		return nil
	}
	return diag.LockPlatformMissing.Errorf("%s declares platforms %s, and this machine is %s; add %q to that list and run \"block lock\"",
		manifest.FileName, strings.Join(platform.Strings(m.Platforms), ", "), p, p)
}

func staleError(reasons []string) error {
	return diag.LockStale.Errorf("%s is stale; run \"block lock\"\n  %s", lockfile.FileName, strings.Join(reasons, "\n  "))
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
		return diag.LockMissing.Errorf("%s not found; run \"block lock\"", lockfile.FileName)
	}
	if !a.Platform.IsSupported() {
		return diag.PlatformUnsupported.Errorf("unsupported platform %s", a.Platform)
	}
	// Asked before staleness, because a manifest that does not name this
	// machine makes every "run block lock" answer below a wrong one.
	if err := platformNotDeclared(m, a.Platform); err != nil {
		return err
	}
	if reasons := Check(m, l, []platform.Platform{a.Platform}); len(reasons) > 0 {
		return staleError(reasons)
	}
	if err := commandConflict(l); err != nil {
		return err
	}
	// What an interrupted run left behind is cleared before this one adds
	// to the store, so a cache kept for years holds artifacts, not leftovers.
	a.Fetcher.Sweep(staleAfter)
	a.Store.SweepTemp(staleAfter)
	states, err := a.installAll(ctx, l.Tools)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0) //nolint:mnd // column padding
	for i, t := range l.Tools {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, t.Version, states[i])
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return a.ensureShims(l)
}

// syncJobs is how many tools a sync downloads and installs at once. A
// toolchain is a handful of archives from a handful of hosts, and what bounds
// the wall clock is the slowest of them rather than their sum; more than a few
// in flight would only contend for the disk and trip a release host's rate
// limit. It is a constant, not a setting: nothing about the result depends on
// it, and a knob whose value changes nothing observable is a knob nobody
// should have to learn.
const syncJobs = 4

// installAll installs every tool, up to syncJobs of them at a time, and
// returns their states in lockfile order. The first failure cancels the
// context the others are downloading under, and is the error reported — the
// rest stopped because of it, and saying so would bury the cause under its
// consequences.
//
// Tools are independent in everything they touch: each has its own install
// directory, each download lands in its own temporary file and is published
// under its digest by a rename, and two tools that share an artifact both
// arrive at the same content-addressed blob. Nothing is printed until every
// tool is done, so the report reads the same whatever order they finished in.
func (a *App) installAll(ctx context.Context, tools []lockfile.Tool) ([]string, error) {
	states := make([]string, len(tools))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(syncJobs)
	for i := range tools {
		g.Go(func() error {
			t := &tools[i]
			state, err := a.install(ctx, t)
			if err != nil {
				return fmt.Errorf("%s: %w", t.Name, err)
			}
			states[i] = state
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return states, nil
}

// ensureShims makes the commands this project locks runnable by their own
// names. The shims are global and version-free — which version each one runs
// is decided per invocation from the working directory — so this only ever
// adds names that have never been synced before.
func (a *App) ensureShims(l *lockfile.Lock) error {
	self := a.Self
	if self == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locating the block binary for the shims: %w", err)
		}
		self = exe
	}
	var commands []string
	for _, t := range l.Tools {
		for _, b := range t.Bin {
			commands = append(commands, recipe.CommandName(b))
		}
	}
	sort.Strings(commands)
	created, err := shim.Ensure(a.Store, self, commands)
	if err != nil {
		return err
	}
	if len(created) == 0 {
		return nil
	}
	fmt.Fprintf(a.Stdout, "shims: %s\n", strings.Join(created, ", "))
	if !shim.OnPath(a.Store) {
		fmt.Fprintf(a.Stderr, "note: add %s to PATH to run these directly, or keep using \"block exec\"\n", shim.Dir(a.Store))
	}
	return nil
}

// staleAfter is how old a temporary file or directory must be before a sync
// treats it as the leftover of a run that was killed rather than of one that
// is still going. A download is bounded by the HTTP client's timeout and an
// extraction by the disk, both far inside this.
const staleAfter = 24 * time.Hour

// install makes one locked tool available and reports "installed" or "cached".
func (a *App) install(ctx context.Context, t *lockfile.Tool) (string, error) {
	art, ok := t.Artifact(a.Platform)
	if !ok {
		return "", diag.LockPlatformMissing.Errorf("%s has no artifact for %s", lockfile.FileName, a.Platform)
	}
	dir, err := a.Store.InstallDir(t.Name, t.Version, art.SHA256)
	if err != nil {
		return "", err
	}
	if a.Store.IsInstalled(dir, t.Bin) {
		return "cached", nil
	}
	blob, _, err := a.fetchArtifact(ctx, art, art.SHA256)
	if err != nil {
		return "", err
	}
	if err := a.Store.Install(blob, assetName(art.URL), dir, t.Bin, t.StripComponents); err != nil {
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

// Toolchain loads the project's installed toolchain: the same offline view of
// block.toml, block.lock and the store that a shim uses.
func (a *App) Toolchain() (*Toolchain, error) {
	if _, err := os.Stat(a.ManifestPath()); err != nil {
		return nil, diag.ManifestMissing.Errorf("%s not found", manifest.FileName)
	}
	return OpenToolchain(a.Dir, a.Platform, a.Store)
}

// Env returns the PATH entries that expose every locked tool for the current
// platform, after checking offline that the toolchain is the one block.toml
// asks for and that it is actually installed. It resolves nothing, downloads
// nothing and writes nothing.
func (a *App) Env() ([]string, error) {
	t, err := a.Toolchain()
	if err != nil {
		return nil, err
	}
	return t.PathDirs(), nil
}
