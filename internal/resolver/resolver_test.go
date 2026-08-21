package resolver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/version"
)

type fake struct {
	tags     []string
	releases map[string]*github.Release
	err      error
	lookups  []string
}

func (f *fake) Tags(_ context.Context, _, prefix string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []string
	for _, t := range f.tags {
		if strings.HasPrefix(t, prefix) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fake) Commit(_ context.Context, _, ref string) (string, error) {
	for _, t := range f.tags {
		if t == ref {
			return "0123456789abcdef0123456789abcdef01234567", nil
		}
	}
	return "", github.ErrNotFound
}

func (f *fake) ReleaseByTag(_ context.Context, _, tag string) (*github.Release, error) {
	f.lookups = append(f.lookups, tag)
	r, ok := f.releases[tag]
	if !ok {
		return nil, github.ErrNotFound
	}
	return r, nil
}

func src() recipe.Source {
	return recipe.Source{Type: recipe.TypeGitHubRelease, Repo: "o/r", Asset: "t_{version}_{os}_{arch}.tar.gz", Bin: []string{"t"}}
}

func rel(tag string, pre bool, assets ...string) *github.Release {
	r := &github.Release{TagName: tag, Prerelease: pre}
	for _, a := range assets {
		r.Assets = append(r.Assets, github.Asset{Name: a, BrowserDownloadURL: "https://dl/" + tag + "/" + a})
	}
	return r
}

func TestVersions(t *testing.T) {
	t.Parallel()
	f := &fake{tags: []string{"v1.7.1", "v1.6.0", "nightly-x", "v1.8.0-rc1", "v2.0.0", "stable", "v1.7.0"}}
	vs, err := Versions(context.Background(), f, src(), version.MustParseConstraint("1"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.String())
	}
	if strings.Join(got, ",") != "1.6.0,1.7.0,1.7.1" {
		t.Errorf("Versions() = %v", got)
	}
}

func TestResolvePicksNewestPublishedRelease(t *testing.T) {
	t.Parallel()
	f := &fake{
		tags: []string{"v1.7.0", "v1.7.1", "v1.7.4", "v1.7.5", "v1.7.6", "v1.9.0"},
		releases: map[string]*github.Release{
			"v1.7.0": rel("v1.7.0", false, "t_1.7.0_linux_amd64.tar.gz"),
			"v1.7.1": rel("v1.7.1", false, "t_1.7.1_linux_amd64.tar.gz"),
			"v1.7.4": rel("v1.7.4", false, "t_1.7.4_linux_amd64.tar.gz"),
			// v1.7.5 has no release; v1.7.6 is a pre-release flagged release.
			"v1.7.6": rel("v1.7.6", true, "t_1.7.6_linux_amd64.tar.gz"),
			"v1.9.0": {TagName: "v1.9.0", Draft: true},
		},
	}
	res, err := Resolve(context.Background(), f, src(), version.MustParseConstraint("1.7"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Version.String() != "1.7.4" {
		t.Errorf("Resolve() = %s", res.Version)
	}
	if strings.Join(f.lookups, ",") != "v1.7.6,v1.7.5,v1.7.4" {
		t.Errorf("lookups = %v (newest first, stop at first published)", f.lookups)
	}
	art, err := ArtifactFor(res, src(), platform.Platform{OS: "linux", Arch: "amd64"})
	if err != nil || art.URL != "https://dl/v1.7.4/t_1.7.4_linux_amd64.tar.gz" || art.SHA256 != "" {
		t.Errorf("ArtifactFor() = %+v, %v", art, err)
	}
	_, err = ArtifactFor(res, src(), platform.Platform{OS: "darwin", Arch: "arm64"})
	if err == nil || !strings.Contains(err.Error(), `has no asset "t_1.7.4_darwin_arm64.tar.gz" (assets: t_1.7.4_linux_amd64.tar.gz)`) {
		t.Errorf("Artifact(missing) error = %v", err)
	}
	limited := src()
	limited.Platforms = []string{"linux/amd64"}
	_, err = ArtifactFor(res, limited, platform.Platform{OS: "darwin", Arch: "arm64"})
	var unsupported *recipe.UnsupportedPlatformError
	if !errors.As(err, &unsupported) {
		t.Errorf("ArtifactFor(unsupported) error = %v", err)
	}
}

func TestArtifactForUsesGitHubDigest(t *testing.T) {
	t.Parallel()
	r := rel("v1.0.0", false, "t_1.0.0_linux_amd64.tar.gz")
	r.Assets[0].Digest = "sha256:" + strings.Repeat("a", 64)
	art, err := ArtifactFor(Resolution{Version: version.MustParse("1.0.0"), Release: r}, src(), platform.Platform{OS: "linux", Arch: "amd64"})
	if err != nil || art.SHA256 != strings.Repeat("a", 64) {
		t.Errorf("ArtifactFor() = %+v, %v", art, err)
	}
	r.Assets[0].Digest = "md5:abc"
	art, _ = ArtifactFor(Resolution{Version: version.MustParse("1.0.0"), Release: r}, src(), platform.Platform{OS: "linux", Arch: "amd64"})
	if art.SHA256 != "" {
		t.Error("a non-sha256 digest must be ignored")
	}
}

func TestResolveHTTP(t *testing.T) {
	t.Parallel()
	http := recipe.Source{Type: recipe.TypeHTTP, Repo: "ethereum/go-ethereum", URL: "https://dl.example/geth-{os}-{arch}-{version}-{commit}.tar.gz", StripComponents: 1, Bin: []string{"geth"}}
	f := &fake{tags: []string{"v1.17.4", "v1.17.5", "v1.18.0-rc1"}}
	res, err := Resolve(context.Background(), f, http, version.MustParseConstraint("1.17"))
	if err != nil || res.Version.String() != "1.17.5" || res.Commit == "" || res.Release != nil {
		t.Fatalf("Resolve(http) = %+v, %v", res, err)
	}
	if len(f.lookups) != 0 {
		t.Error("http sources must not look up releases")
	}
	art, err := ArtifactFor(res, http, platform.Platform{OS: "linux", Arch: "arm64"})
	if err != nil || art.URL != "https://dl.example/geth-linux-arm64-1.17.5-01234567.tar.gz" || art.SHA256 != "" {
		t.Errorf("ArtifactFor(http) = %+v, %v", art, err)
	}
	// Without {commit} no commit lookup happens.
	plain := http
	plain.URL = "https://dl.example/geth-{version}.tar.gz"
	res, err = Exact(context.Background(), &failingCommits{}, plain, version.MustParse("1.17.4"))
	if err != nil || res.Commit != "" {
		t.Errorf("Exact(no commit) = %+v, %v", res, err)
	}
	_, err = Exact(context.Background(), &failingCommits{}, http, version.MustParse("1.17.4"))
	if err == nil || !strings.Contains(err.Error(), "commit of ethereum/go-ethereum v1.17.4") {
		t.Errorf("Exact(commit error) = %v", err)
	}
}

type failingCommits struct{ fake }

func (f *failingCommits) Commit(context.Context, string, string) (string, error) {
	return "", errors.New("boom")
}

func TestResolveErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := Resolve(ctx, &fake{tags: []string{"v1.0.0"}}, src(), version.MustParseConstraint("2"))
	if err == nil || err.Error() != `no version of o/r matches "2"` {
		t.Errorf("error = %v", err)
	}
	_, err = Resolve(ctx, &fake{tags: []string{"v2.0.0"}, releases: map[string]*github.Release{"v2.0.0": rel("v2.0.0", true)}}, src(), version.MustParseConstraint("2"))
	if err == nil || err.Error() != `no published release of o/r matches "2" (checked the newest 1 tags)` {
		t.Errorf("error = %v", err)
	}
	boom := errors.New("boom")
	_, err = Resolve(ctx, &fake{err: boom}, src(), version.MustParseConstraint("2"))
	if !errors.Is(err, boom) {
		t.Errorf("error = %v", err)
	}
	// A transport error on the release lookup is surfaced, not skipped.
	failing := &failingReleases{fake: &fake{tags: []string{"v1.0.0"}}}
	_, err = Resolve(ctx, failing, src(), version.MustParseConstraint("1"))
	if err == nil || err.Error() != "boom" {
		t.Errorf("error = %v", err)
	}
	// The lookup budget caps how far back we go.
	var tags []string
	for i := range 30 {
		tags = append(tags, version.Version{Major: 1, Patch: i}.String())
	}
	for i := range tags {
		tags[i] = "v" + tags[i]
	}
	f := &fake{tags: tags, releases: map[string]*github.Release{"v1.0.0": rel("v1.0.0", false)}}
	_, err = Resolve(ctx, f, src(), version.MustParseConstraint("1"))
	if err == nil || !strings.Contains(err.Error(), "checked the newest 10 tags") || len(f.lookups) != maxReleaseLookups {
		t.Errorf("error = %v, lookups = %d", err, len(f.lookups))
	}
}

type failingReleases struct{ *fake }

func (f *failingReleases) ReleaseByTag(context.Context, string, string) (*github.Release, error) {
	return nil, errors.New("boom")
}
