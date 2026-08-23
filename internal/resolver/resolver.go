// Package resolver turns a recipe plus a version constraint into a concrete
// upstream version and its per-platform artifacts.
//
//	tags (git/matching-refs)  ->  parse with recipe  ->  apply constraint
//	  github_release: newest first, fetch the release by tag, skip draft /
//	                  pre-release, pick the asset the recipe names
//	  http:           newest tag wins
//
// A channel constraint takes a different road, because a channel is a tag that
// moves: it is dereferenced to the commit it points at, and the release tagged
// "<channel>-<commit>" is what gets pinned. A constraint that names such a
// release outright takes the same road with the dereference left out. See
// [ResolveChannelConstraint].
//
// Either type additionally resolves the tagged commit when its template
// carries {commit}.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/version"
)

// maxReleaseLookups bounds how many candidate tags are checked for a
// published release before giving up.
const maxReleaseLookups = 10

// Releases is the upstream query surface the resolver needs.
type Releases interface {
	Tags(ctx context.Context, repo, prefix string) ([]string, error)
	ReleaseByTag(ctx context.Context, repo, tag string) (*github.Release, error)
	Commit(ctx context.Context, repo, ref string) (string, error)
	// Private reports whether a repository is private, and so whether its
	// release assets can only be downloaded through the API with a token.
	Private(ctx context.Context, repo string) (bool, error)
}

// Resolution is a chosen release plus what naming its artifacts needs: the
// release for github_release, and the tagged commit for a {commit} template.
//
// A release is either a version or a channel release. Tag is what the upstream
// calls it either way, and never a tag that moves: [Identity] is what a
// lockfile records.
type Resolution struct {
	Version version.Version
	// Channel is the release line this came from, or "" for a version.
	Channel string
	// Tag is the tag the release carries.
	Tag     string
	Release *github.Release
	Commit  string
}

// Identity is what block.lock records as the release: the version for a
// version constraint, and the tag that will not move for a channel.
func (r Resolution) Identity() string {
	if r.Channel != "" {
		return r.Tag
	}
	return r.Version.String()
}

// Artifact is a resolved download: its URL and, when the upstream publishes
// one, its SHA-256.
type Artifact struct {
	URL    string
	SHA256 string
	// APIURL is the asset's API endpoint, for a github_release source; the
	// bytes of a private repository's asset are had from it and nowhere
	// else. Empty for an http source.
	APIURL string
	// Size is the asset's size as the upstream reports it, or 0 when the
	// upstream does not say.
	Size int64
}

// Versions lists every release version the recipe's tags expose that
// satisfies c, sorted ascending. Pre-releases are excluded by the constraint.
func Versions(ctx context.Context, rel Releases, src recipe.Source, c version.Constraint) ([]version.Version, error) {
	tags, err := rel.Tags(ctx, src.Repo, src.EffectiveTagPrefix())
	if err != nil {
		return nil, err
	}
	var vs []version.Version
	for _, t := range tags {
		v, ok := src.ParseTag(t)
		if ok && c.Matches(v) {
			vs = append(vs, v)
		}
	}
	version.Sort(vs)
	return vs, nil
}

// Resolve picks the newest version satisfying c that the source can deliver.
func Resolve(ctx context.Context, rel Releases, src recipe.Source, c version.Constraint) (Resolution, error) {
	vs, err := Versions(ctx, rel, src, c)
	if err != nil {
		return Resolution{}, err
	}
	if len(vs) == 0 {
		return Resolution{}, diag.NoMatchingVersion.Errorf("no version of %s matches %q", src.Repo, c)
	}
	if src.Type == recipe.TypeHTTP {
		return Exact(ctx, rel, src, vs[len(vs)-1])
	}
	lookups := 0
	for i := len(vs) - 1; i >= 0 && lookups < maxReleaseLookups; i-- {
		lookups++
		r, err := rel.ReleaseByTag(ctx, src.Repo, src.Tag(vs[i]))
		if errors.Is(err, github.ErrNotFound) {
			continue
		}
		if err != nil {
			return Resolution{}, err
		}
		if r.Draft || r.Prerelease {
			continue
		}
		res := Resolution{Version: vs[i], Tag: src.Tag(vs[i]), Release: r}
		if err := resolveCommit(ctx, rel, src, &res); err != nil {
			return Resolution{}, err
		}
		return res, nil
	}
	return Resolution{}, diag.NoPublishedRelease.Errorf("no published release of %s matches %q (checked the newest %d tags)", src.Repo, c, lookups)
}

// Exact resolves an already chosen version, for example to add a platform to
// an existing pin.
func Exact(ctx context.Context, rel Releases, src recipe.Source, v version.Version) (Resolution, error) {
	res := Resolution{Version: v, Tag: src.Tag(v)}
	if src.Type != recipe.TypeHTTP {
		r, err := rel.ReleaseByTag(ctx, src.Repo, src.Tag(v))
		if err != nil {
			return Resolution{}, fmt.Errorf("release %s of %s: %w", src.Tag(v), src.Repo, err)
		}
		res.Release = r
	}
	if err := resolveCommit(ctx, rel, src, &res); err != nil {
		return Resolution{}, err
	}
	return res, nil
}

// resolveCommit fills in the commit a {commit} template needs. Both source
// types can want it: an http download server names its archives after the
// build commit, and so do some release assets (vyper, Nethermind, Nimbus).
func resolveCommit(ctx context.Context, rel Releases, src recipe.Source, res *Resolution) error {
	if !src.NeedsCommit() {
		return nil
	}
	sha, err := rel.Commit(ctx, src.Repo, res.Tag)
	if err != nil {
		return fmt.Errorf("commit of %s %s: %w", src.Repo, res.Tag, err)
	}
	res.Commit = sha
	return nil
}

// ResolveChannel pins the release a channel currently points at.
//
// A channel is a tag an upstream moves — Foundry retags "nightly" every night
// — and a lockfile may not record one. What it records instead is the release
// under the commit that tag points at today: block dereferences the moving tag
// and asks for the release tagged "<channel>-<commit>", which is a tag the
// upstream never touches again. Two API calls, no listing, and the answer is
// as immutable as a version.
//
// An upstream that moves a tag without publishing one for the commit beneath
// it cannot be pinned this way, and block refuses rather than recording a URL
// whose contents will change.
func ResolveChannel(ctx context.Context, rel Releases, src recipe.Source, channel string) (Resolution, error) {
	if _, ok := src.Channel(channel); !ok {
		return Resolution{}, noSuchChannel(src, channel)
	}
	commit, err := rel.Commit(ctx, src.Repo, channel)
	if err != nil {
		return Resolution{}, fmt.Errorf("channel %s of %s: %w", channel, src.Repo, err)
	}
	res := Resolution{Channel: channel, Commit: commit, Tag: channel + "-" + commit}
	r, err := rel.ReleaseByTag(ctx, src.Repo, res.Tag)
	if errors.Is(err, github.ErrNotFound) {
		return Resolution{}, diag.ChannelNotPinnable.Errorf(
			"%s moves the tag %q but publishes no release %q for the commit it points at, so there is nothing that will not move to pin",
			src.Repo, channel, res.Tag)
	}
	if err != nil {
		return Resolution{}, err
	}
	if r.Draft {
		return Resolution{}, diag.ChannelNotPinnable.Errorf("release %s of %s is a draft", res.Tag, src.Repo)
	}
	res.Release = r
	return res, nil
}

// noSuchChannel is the refusal a channel name the recipe does not declare
// earns, naming the ones it does. Both roads into a channel report it, and
// they have to report it the same way.
func noSuchChannel(src recipe.Source, channel string) error {
	declared := src.ChannelNames()
	if len(declared) == 0 {
		return diag.NoSuchChannel.Errorf("%s publishes no channel %q; ask for a version instead", src.Repo, channel)
	}
	return diag.NoSuchChannel.Errorf("%s publishes no channel %q (it has %s)", src.Repo, channel, strings.Join(declared, ", "))
}

// ResolveChannelConstraint pins what a channel constraint asks for, whichever
// of the two shapes it has: the release the moving tag points at right now, or
// the one release the constraint names in full.
//
// The recipe has the final say on which it is. A source is free to call a
// moving release line anything, "nightly-2" included, so a constraint that
// spells a declared channel whole is that channel — the shape of the string is
// only consulted for a name no channel claims.
func ResolveChannelConstraint(ctx context.Context, rel Releases, src recipe.Source, c version.Constraint) (Resolution, error) {
	if _, ok := src.Channel(c.String()); ok {
		return ResolveChannel(ctx, rel, src, c.String())
	}
	if tag := c.ChannelRelease(); tag != "" {
		return ResolveChannelRelease(ctx, rel, src, c.Channel(), tag)
	}
	return ResolveChannel(ctx, rel, src, c.Channel())
}

// ResolveChannelRelease pins one release of a channel by the tag the upstream
// published it under.
//
// It is the second half of [ResolveChannel] on its own. A channel is pinned by
// dereferencing its moving tag and taking the release published for the commit
// beneath it; a constraint such as "nightly-<commit>" already names that
// release, so there is nothing to dereference and nothing that could move. One
// API call, and re-running lock writes the same pin back for as long as the
// upstream keeps the release.
//
// The channel still has to be one the recipe declares, because the asset names
// come from it: a nightly's archive is called after the line, not after a
// version. A pre-release flag is not an objection here — a nightly is a
// pre-release by definition, and this is the tag the project asked for by
// name — but a draft is, since its assets are not published at all.
func ResolveChannelRelease(ctx context.Context, rel Releases, src recipe.Source, channel, tag string) (Resolution, error) {
	if _, ok := src.Channel(channel); !ok {
		return Resolution{}, noSuchChannel(src, channel)
	}
	r, err := rel.ReleaseByTag(ctx, src.Repo, tag)
	if errors.Is(err, github.ErrNotFound) {
		return Resolution{}, diag.UpstreamNotFound.Errorf(
			"%s publishes no release %q; %q names one release of the %q channel, and the upstream has to carry that tag",
			src.Repo, tag, tag, channel)
	}
	if err != nil {
		return Resolution{}, err
	}
	if r.Draft {
		return Resolution{}, diag.NoPublishedRelease.Errorf("release %s of %s is a draft, so it publishes no assets", tag, src.Repo)
	}
	res := Resolution{Channel: channel, Tag: tag, Release: r}
	// The commit is in the tag, so a {commit} template costs no lookup. The
	// channel may carry hyphens of its own, so the whole of it is cut.
	if sha, ok := strings.CutPrefix(tag, channel+"-"); ok && sha != "" {
		res.Commit = sha
	}
	return res, nil
}

// ExactTag re-resolves a release block already pinned, by the tag the lockfile
// records. It is what adds a platform to a channel pin without moving it.
func ExactTag(ctx context.Context, rel Releases, src recipe.Source, channel, tag string) (Resolution, error) {
	r, err := rel.ReleaseByTag(ctx, src.Repo, tag)
	if err != nil {
		return Resolution{}, fmt.Errorf("release %s of %s: %w", tag, src.Repo, err)
	}
	res := Resolution{Channel: channel, Tag: tag, Release: r}
	// The commit is in the tag block wrote, so a template that needs one does
	// not cost a lookup: "nightly-<sha>" is where it came from. The channel
	// itself may carry hyphens ("pre-release"), so strip it as a whole rather
	// than cutting at the first one.
	if sha, ok := strings.CutPrefix(tag, channel+"-"); ok && sha != "" {
		res.Commit = sha
	}
	return res, nil
}

// ArtifactFor returns the download for p. For github_release the asset must
// exist in the release and may carry GitHub's digest; for http the URL is
// rendered from the recipe and the digest is learned on download.
func ArtifactFor(res Resolution, src recipe.Source, p platform.Platform) (Artifact, error) {
	render := func() (string, error) {
		if res.Channel != "" {
			return src.RenderChannel(res.Channel, p, res.Commit)
		}
		return src.Render(res.Version, p, res.Commit)
	}
	name, err := render()
	if err != nil {
		return Artifact{}, err
	}
	if src.Type == recipe.TypeHTTP {
		return Artifact{URL: name}, nil
	}
	if res.Release == nil {
		return Artifact{}, diag.Internal.Errorf("release not resolved")
	}
	matches := res.Release.AssetsNamed(name)
	switch {
	case len(matches) == 0:
		names := make([]string, 0, len(res.Release.Assets))
		for _, x := range res.Release.Assets {
			names = append(names, x.Name)
		}
		return Artifact{}, diag.AssetMissing.Errorf("release %s of %s has no asset %q (assets: %s)", res.Tag, src.Repo, name, strings.Join(names, ", "))
	case len(matches) > 1:
		urls := make([]string, 0, len(matches))
		for _, m := range matches {
			urls = append(urls, m.BrowserDownloadURL)
		}
		sort.Strings(urls)
		return Artifact{}, diag.AmbiguousAsset.Errorf("release %s of %s has %d assets named %q (%s); block will not choose between them",
			res.Tag, src.Repo, len(matches), name, strings.Join(urls, ", "))
	}
	a := matches[0]
	return Artifact{URL: a.BrowserDownloadURL, SHA256: a.SHA256(), APIURL: a.URL, Size: a.Size}, nil
}
