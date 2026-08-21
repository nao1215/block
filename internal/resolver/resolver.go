// Package resolver turns a recipe plus a version constraint into a concrete
// upstream release and its per-platform artifacts.
//
//	tags (git/matching-refs)  ->  parse with recipe  ->  apply constraint
//	  ->  newest first, fetch release by tag  ->  skip draft / pre-release
//	  ->  pick the asset the recipe names for each platform
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
}

// Resolution is a chosen version and the release that publishes it.
type Resolution struct {
	Version version.Version
	Release *github.Release
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

// Resolve picks the newest version satisfying c that has a published,
// non-draft, non-prerelease GitHub release.
func Resolve(ctx context.Context, rel Releases, src recipe.Source, c version.Constraint) (Resolution, error) {
	vs, err := Versions(ctx, rel, src, c)
	if err != nil {
		return Resolution{}, err
	}
	if len(vs) == 0 {
		return Resolution{}, fmt.Errorf("no version of %s matches %q", src.Repo, c)
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
		return Resolution{Version: vs[i], Release: r}, nil
	}
	return Resolution{}, fmt.Errorf("no published release of %s matches %q (checked the newest %d tags)", src.Repo, c, lookups)
}

// Artifact returns the download URL of the asset the recipe names for p.
func Artifact(res Resolution, src recipe.Source, p platform.Platform) (string, error) {
	name, err := src.AssetName(res.Version, p)
	if err != nil {
		return "", err
	}
	a, ok := res.Release.Asset(name)
	if !ok {
		names := make([]string, 0, len(res.Release.Assets))
		for _, x := range res.Release.Assets {
			names = append(names, x.Name)
		}
		return "", fmt.Errorf("release %s of %s has no asset %q (assets: %s)", src.Tag(res.Version), src.Repo, name, strings.Join(names, ", "))
	}
	return a.BrowserDownloadURL, nil
}
