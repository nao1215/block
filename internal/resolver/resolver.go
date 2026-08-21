// Package resolver turns a recipe plus a version constraint into a concrete
// upstream version and its per-platform artifacts.
//
//	tags (git/matching-refs)  ->  parse with recipe  ->  apply constraint
//	  github_release: newest first, fetch the release by tag, skip draft /
//	                  pre-release, pick the asset the recipe names
//	  http:           newest tag wins
//
// Either type additionally resolves the tagged commit when its template
// carries {commit}.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
}

// Resolution is a chosen version plus what naming its artifacts needs: the
// release for github_release, and the tagged commit for a {commit} template.
type Resolution struct {
	Version version.Version
	Release *github.Release
	Commit  string
}

// Artifact is a resolved download: its URL and, when the upstream publishes
// one, its SHA-256.
type Artifact struct {
	URL    string
	SHA256 string
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
		return Resolution{}, fmt.Errorf("no version of %s matches %q", src.Repo, c)
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
		res := Resolution{Version: vs[i], Release: r}
		if err := resolveCommit(ctx, rel, src, &res); err != nil {
			return Resolution{}, err
		}
		return res, nil
	}
	return Resolution{}, fmt.Errorf("no published release of %s matches %q (checked the newest %d tags)", src.Repo, c, lookups)
}

// Exact resolves an already chosen version, for example to add a platform to
// an existing pin.
func Exact(ctx context.Context, rel Releases, src recipe.Source, v version.Version) (Resolution, error) {
	res := Resolution{Version: v}
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
	tag := src.Tag(res.Version)
	sha, err := rel.Commit(ctx, src.Repo, tag)
	if err != nil {
		return fmt.Errorf("commit of %s %s: %w", src.Repo, tag, err)
	}
	res.Commit = sha
	return nil
}

// ArtifactFor returns the download for p. For github_release the asset must
// exist in the release and may carry GitHub's digest; for http the URL is
// rendered from the recipe and the digest is learned on download.
func ArtifactFor(res Resolution, src recipe.Source, p platform.Platform) (Artifact, error) {
	name, err := src.Render(res.Version, p, res.Commit)
	if err != nil {
		return Artifact{}, err
	}
	if src.Type == recipe.TypeHTTP {
		return Artifact{URL: name}, nil
	}
	if res.Release == nil {
		return Artifact{}, errors.New("release not resolved")
	}
	a, ok := res.Release.Asset(name)
	if !ok {
		names := make([]string, 0, len(res.Release.Assets))
		for _, x := range res.Release.Assets {
			names = append(names, x.Name)
		}
		return Artifact{}, fmt.Errorf("release %s of %s has no asset %q (assets: %s)", src.Tag(res.Version), src.Repo, name, strings.Join(names, ", "))
	}
	return Artifact{URL: a.BrowserDownloadURL, SHA256: a.SHA256()}, nil
}
