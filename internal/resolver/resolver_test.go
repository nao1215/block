package resolver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/version"
)

type fake struct {
	tags     []string
	releases map[string]*github.Release
	err      error
	// relErr is what ReleaseByTag fails with, when the upstream is down
	// rather than merely missing a release.
	relErr  error
	lookups []string
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

func (f *fake) Private(context.Context, string) (bool, error) { return false, nil }

func (f *fake) ReleaseByTag(_ context.Context, _, tag string) (*github.Release, error) {
	f.lookups = append(f.lookups, tag)
	if f.relErr != nil {
		return nil, f.relErr
	}
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

// Some upstreams stamp the build commit into the release asset's own name
// (vyper, Nethermind, Nimbus), so a github_release resolves the tagged commit
// too — while still taking the URL and digest from the release itself.
func TestResolveReleaseAssetNamedAfterTheCommit(t *testing.T) {
	t.Parallel()
	vyper := recipe.Source{
		Type: recipe.TypeGitHubRelease, Repo: "vyperlang/vyper",
		Asset: "vyper.{version}+commit.{commit}.{os}", Bin: []string{"vyper"},
	}
	asset := "vyper.0.4.3+commit.01234567.linux"
	f := &fake{tags: []string{"v0.4.3"}, releases: map[string]*github.Release{"v0.4.3": rel("v0.4.3", false, asset)}}
	res, err := Resolve(context.Background(), f, vyper, version.MustParseConstraint("0.4"))
	if err != nil || res.Commit == "" || res.Release == nil {
		t.Fatalf("Resolve(commit asset) = %+v, %v", res, err)
	}
	art, err := ArtifactFor(res, vyper, platform.Platform{OS: "linux", Arch: "amd64"})
	if err != nil || art.URL != "https://dl/v0.4.3/"+asset {
		t.Errorf("ArtifactFor(commit asset) = %+v, %v", art, err)
	}
	// A release that resolves but whose commit cannot be read is an error,
	// not an artifact named after an empty commit.
	if _, err := Resolve(context.Background(), &failingCommits{fake: *f}, vyper, version.MustParseConstraint("0.4")); err == nil {
		t.Error("Resolve() must fail when the commit cannot be resolved")
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

// A release carrying two files of one name is not something to resolve by
// taking the first: they are different downloads, and which one a lockfile
// pinned would depend on the order the API answered in.
func TestArtifactForRefusesTwoAssetsOfOneName(t *testing.T) {
	t.Parallel()
	src := recipe.Source{
		Type: recipe.TypeGitHubRelease, Repo: "example/dup",
		Asset: "dup_{version}_{os}_{arch}.tar.gz", Bin: []string{"dup"},
	}
	p := platform.Platform{OS: "linux", Arch: "amd64"}
	name := "dup_1.0.0_linux_amd64.tar.gz"
	res := Resolution{
		Version: version.MustParse("1.0.0"),
		Release: &github.Release{TagName: "v1.0.0", Assets: []github.Asset{
			{Name: name, BrowserDownloadURL: "https://example.com/second/" + name},
			{Name: name, BrowserDownloadURL: "https://example.com/first/" + name},
		}},
	}
	_, err := ArtifactFor(res, src, p)
	if err == nil {
		t.Fatal("ArtifactFor chose between two assets of one name")
	}
	if diag.Of(err) != diag.AmbiguousAsset {
		t.Errorf("code = %v, want %v", diag.Of(err), diag.AmbiguousAsset)
	}
	// Both URLs are named, in a fixed order, so the message does not depend
	// on the order the API answered in either.
	for _, want := range []string{"https://example.com/first/", "https://example.com/second/"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}
	if strings.Index(err.Error(), "/first/") > strings.Index(err.Error(), "/second/") {
		t.Errorf("error %q lists the assets in the order they arrived", err)
	}
}

func TestExactTagKeepsTheWholeCommitOfAHyphenatedChannel(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	for _, channel := range []string{"nightly", "pre-release", "dev-2"} {
		t.Run(channel, func(t *testing.T) {
			t.Parallel()
			tag := channel + "-" + sha
			f := &fake{releases: map[string]*github.Release{tag: rel(tag, true, "t_"+sha+"_linux_amd64.tar.gz")}}
			res, err := ExactTag(context.Background(), f, src(), channel, tag)
			if err != nil {
				t.Fatal(err)
			}
			if res.Commit != sha {
				t.Fatalf("commit = %q, want %q", res.Commit, sha)
			}
			if res.Tag != tag || res.Channel != channel {
				t.Fatalf("resolution = %+v", res)
			}
		})
	}
}

func TestExactTagReportsAMissingRelease(t *testing.T) {
	t.Parallel()
	f := &fake{releases: map[string]*github.Release{}}
	_, err := ExactTag(context.Background(), f, src(), "nightly", "nightly-abc")
	if !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResolveChannelRefusesWhatCannotBePinned(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	s := src()
	s.Channels = map[string]recipe.Channel{"nightly": {Asset: "t_nightly_{os}_{arch}.tar.gz"}}
	t.Run("no release under the commit", func(t *testing.T) {
		t.Parallel()
		f := &fake{tags: []string{"nightly"}, releases: map[string]*github.Release{}}
		_, err := ResolveChannel(context.Background(), f, s, "nightly")
		if diag.Of(err) != diag.ChannelNotPinnable {
			t.Fatalf("err = %v, want %s", err, diag.ChannelNotPinnable)
		}
		if !strings.Contains(err.Error(), "nightly-"+sha) {
			t.Fatalf("err = %v, want the tag it looked for", err)
		}
	})
	t.Run("a draft under the commit", func(t *testing.T) {
		t.Parallel()
		r := rel("nightly-"+sha, true, "t_nightly_linux_amd64.tar.gz")
		r.Draft = true
		f := &fake{tags: []string{"nightly"}, releases: map[string]*github.Release{r.TagName: r}}
		_, err := ResolveChannel(context.Background(), f, s, "nightly")
		if diag.Of(err) != diag.ChannelNotPinnable || !strings.Contains(err.Error(), "draft") {
			t.Fatalf("err = %v, want %s about a draft", err, diag.ChannelNotPinnable)
		}
	})
	t.Run("an upstream error is not a refusal", func(t *testing.T) {
		t.Parallel()
		f := &fake{tags: []string{"nightly"}, relErr: errors.New("boom")}
		_, err := ResolveChannel(context.Background(), f, s, "nightly")
		if err == nil || diag.Of(err) == diag.ChannelNotPinnable || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err = %v, want a plain failure", err)
		}
	})
}

// A constraint that names one release of a channel is resolved by looking the
// tag up, with no dereference in between: the whole point is that nothing
// about it can move.
func TestResolveChannelRelease(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	s := src()
	s.Channels = map[string]recipe.Channel{"nightly": {Asset: "t_nightly_{os}_{arch}.tar.gz"}}
	tag := "nightly-" + sha
	// A nightly is flagged a pre-release upstream, and is asked for by name
	// here, so the flag is not an objection.
	f := &fake{tags: []string{"nightly"}, releases: map[string]*github.Release{tag: rel(tag, true, "t_nightly_linux_amd64.tar.gz")}}
	res, err := ResolveChannelConstraint(context.Background(), f, s, version.MustParseConstraint(tag))
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case res.Tag != tag:
		t.Errorf("Tag = %q, want %q", res.Tag, tag)
	case res.Channel != "nightly":
		t.Errorf("Channel = %q", res.Channel)
	case res.Identity() != tag:
		t.Errorf("Identity() = %q, want %q", res.Identity(), tag)
	// The commit comes out of the tag, so no second request is made for it.
	case res.Commit != sha:
		t.Errorf("Commit = %q, want %q", res.Commit, sha)
	}
	if strings.Join(f.lookups, ",") != tag {
		t.Errorf("lookups = %v, want the tag alone", f.lookups)
	}
	// The asset is the channel's, not a version's: a channel release has no
	// version to render.
	a, err := ArtifactFor(res, s, platform.Platform{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(a.URL, "/t_nightly_linux_amd64.tar.gz") {
		t.Errorf("URL = %q", a.URL)
	}
}

// The recipe has the final say on what a constraint names: a release line an
// upstream happens to call "dev-1234567" is that line, not one release of a
// line called "dev".
func TestResolveChannelConstraintPrefersADeclaredChannel(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	s := src()
	s.Channels = map[string]recipe.Channel{"dev-1234567": {Asset: "t_dev_{os}_{arch}.tar.gz"}}
	tag := "dev-1234567-" + sha
	f := &fake{tags: []string{"dev-1234567"}, releases: map[string]*github.Release{tag: rel(tag, true, "t_dev_linux_amd64.tar.gz")}}
	res, err := ResolveChannelConstraint(context.Background(), f, s, version.MustParseConstraint("dev-1234567"))
	if err != nil {
		t.Fatal(err)
	}
	// It was dereferenced, which is what a moving tag gets and a named
	// release does not.
	if res.Tag != tag || res.Channel != "dev-1234567" {
		t.Fatalf("resolution = %+v, want the moving tag dereferenced", res)
	}
}

func TestResolveChannelReleaseRefusals(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	s := src()
	s.Channels = map[string]recipe.Channel{"nightly": {Asset: "t_nightly_{os}_{arch}.tar.gz"}}

	t.Run("a line the recipe does not declare", func(t *testing.T) {
		t.Parallel()
		f := &fake{releases: map[string]*github.Release{}}
		_, err := ResolveChannelConstraint(context.Background(), f, s, version.MustParseConstraint("canary-"+sha))
		if diag.Of(err) != diag.NoSuchChannel || !strings.Contains(err.Error(), "it has nightly") {
			t.Fatalf("err = %v, want %s naming the channels there are", err, diag.NoSuchChannel)
		}
	})
	t.Run("a release the upstream does not publish", func(t *testing.T) {
		t.Parallel()
		f := &fake{releases: map[string]*github.Release{}}
		_, err := ResolveChannelConstraint(context.Background(), f, s, version.MustParseConstraint("nightly-"+sha))
		if diag.Of(err) != diag.UpstreamNotFound || !strings.Contains(err.Error(), "nightly-"+sha) {
			t.Fatalf("err = %v, want %s naming the tag", err, diag.UpstreamNotFound)
		}
	})
	t.Run("a draft", func(t *testing.T) {
		t.Parallel()
		r := rel("nightly-"+sha, true, "t_nightly_linux_amd64.tar.gz")
		r.Draft = true
		f := &fake{releases: map[string]*github.Release{r.TagName: r}}
		_, err := ResolveChannelConstraint(context.Background(), f, s, version.MustParseConstraint("nightly-"+sha))
		if diag.Of(err) != diag.NoPublishedRelease || !strings.Contains(err.Error(), "draft") {
			t.Fatalf("err = %v, want %s about a draft", err, diag.NoPublishedRelease)
		}
	})
	t.Run("an upstream error is not a refusal", func(t *testing.T) {
		t.Parallel()
		f := &fake{relErr: errors.New("boom")}
		_, err := ResolveChannelConstraint(context.Background(), f, s, version.MustParseConstraint("nightly-"+sha))
		if err == nil || diag.Of(err) == diag.UpstreamNotFound || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err = %v, want a plain failure", err)
		}
	})
}

// A name that is neither a declared channel nor one release of one is the
// refusal an unknown channel has always earned, whichever entry point asked.
func TestResolveChannelConstraintRefusesAnUndeclaredLine(t *testing.T) {
	t.Parallel()
	f := &fake{}
	t.Run("a source with channels", func(t *testing.T) {
		t.Parallel()
		s := src()
		s.Channels = map[string]recipe.Channel{"nightly": {Asset: "t_nightly_{os}_{arch}.tar.gz"}}
		_, err := ResolveChannelConstraint(context.Background(), f, s, version.MustParseConstraint("canary"))
		if diag.Of(err) != diag.NoSuchChannel || !strings.Contains(err.Error(), "it has nightly") {
			t.Fatalf("err = %v, want %s naming the channels there are", err, diag.NoSuchChannel)
		}
	})
	t.Run("a source with none", func(t *testing.T) {
		t.Parallel()
		_, err := ResolveChannelConstraint(context.Background(), f, src(), version.MustParseConstraint("nightly"))
		if diag.Of(err) != diag.NoSuchChannel || !strings.Contains(err.Error(), "ask for a version instead") {
			t.Fatalf("err = %v, want %s pointing at versions", err, diag.NoSuchChannel)
		}
	})
}
