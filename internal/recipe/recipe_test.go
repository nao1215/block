package recipe

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/version"
)

func foundry() Source {
	return Source{
		Type:      TypeGitHubRelease,
		Repo:      "foundry-rs/foundry",
		Asset:     "foundry_v{version}_{os}_{arch}.tar.gz",
		Platforms: []string{"linux/amd64", "darwin/arm64"},
		Bin:       []string{"forge", "cast"},
	}
}

func hermes() Source {
	return Source{
		Type:  TypeGitHubRelease,
		Repo:  "informalsystems/hermes",
		Asset: "hermes-v{version}-{arch}-{os}.tar.gz",
		OS:    map[string]string{"linux": "unknown-linux-gnu", "darwin": "apple-darwin"},
		Arch:  map[string]string{"amd64": "x86_64", "arm64": "aarch64"},
		Bin:   []string{"hermes"},
	}
}

func TestSourceValidate(t *testing.T) {
	t.Parallel()
	if err := foundry().Validate(); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
	withChannel := foundry()
	withChannel.Channels = map[string]Channel{"nightly": {Asset: "foundry_nightly_{os}_{arch}.tar.gz"}}
	if err := withChannel.Validate(); err != nil {
		t.Fatalf("valid channel rejected: %v", err)
	}
	mutate := func(f func(*Source)) Source {
		s := foundry()
		f(&s)
		return s
	}
	tests := []struct {
		name string
		src  Source
		want string
	}{
		{"type", mutate(func(s *Source) { s.Type = "cargo" }), `unsupported source type "cargo"`},
		{"url on release", mutate(func(s *Source) { s.URL = "https://x/{version}" }), `url is only valid for type "http"`},
		{"negative strip", mutate(func(s *Source) { s.StripComponents = -1 }), "strip_components must not be negative"},
		{"raw two bins", mutate(func(s *Source) { s.Asset = "solc-{version}-{os}"; s.Bin = []string{"a", "b"} }), "needs exactly one bare bin name"},
		{"raw nested bin", mutate(func(s *Source) { s.Asset = "solc-{version}-{os}"; s.Bin = []string{"bin/solc"} }), "needs exactly one bare bin name"},
		{"raw strip", mutate(func(s *Source) { s.Asset = "solc-{version}"; s.Bin = []string{"solc"}; s.StripComponents = 1 }), "strip_components is only valid for archives"},
		{"http no url", Source{Type: TypeHTTP, Repo: "o/r", Bin: []string{"x"}}, "url template is required"},
		{"http no version", Source{Type: TypeHTTP, Repo: "o/r", URL: "https://x/y.tar.gz", Bin: []string{"x"}}, "must contain {version}"},
		{"http scheme", Source{Type: TypeHTTP, Repo: "o/r", URL: "ftp://x/{version}.tar.gz", Bin: []string{"x"}}, "must start with https://"},
		{"http with asset", Source{Type: TypeHTTP, Repo: "o/r", URL: "https://x/{version}.tar.gz", Asset: "a", Bin: []string{"x"}}, `asset is only valid for type "github_release"`},
		{"repo empty", mutate(func(s *Source) { s.Repo = "" }), "invalid repo"},
		{"repo no slash", mutate(func(s *Source) { s.Repo = "foundry" }), "invalid repo"},
		{"repo extra slash", mutate(func(s *Source) { s.Repo = "a/b/c" }), "invalid repo"},
		{"asset empty", mutate(func(s *Source) { s.Asset = "" }), "asset template is required"},
		{"asset path", mutate(func(s *Source) { s.Asset = "dir/foundry_{version}.tar.gz" }), "bare file name"},
		{"bin empty", mutate(func(s *Source) { s.Bin = nil }), "bin must list"},
		{"bin blank", mutate(func(s *Source) { s.Bin = []string{""} }), "bin entry is empty"},
		{"bin abs", mutate(func(s *Source) { s.Bin = []string{"/usr/bin/forge"} }), "invalid bin entry"},
		{"bin traversal", mutate(func(s *Source) { s.Bin = []string{"../forge"} }), "invalid bin entry"},
		{"bin unclean", mutate(func(s *Source) { s.Bin = []string{"bin/./forge"} }), "invalid bin entry"},
		{"platform", mutate(func(s *Source) { s.Platforms = []string{"plan9/amd64"} }), "unsupported platform"},
		// A channel's asset template is held to the same shape as the
		// source's own, minus {version}, which a channel release has none of.
		{"channel name", mutate(func(s *Source) { s.Channels = map[string]Channel{"Nightly": {Asset: "f_{os}_{arch}.tar.gz"}} }), "invalid channel"},
		{"channel on http", Source{Type: TypeHTTP, Repo: "o/r", URL: "https://x/{version}.tar.gz", Bin: []string{"x"}, Channels: map[string]Channel{"nightly": {Asset: "f_{os}_{arch}.tar.gz"}}}, `channels need type "github_release"`},
		{"channel asset empty", mutate(func(s *Source) { s.Channels = map[string]Channel{"nightly": {}} }), `channel "nightly": asset template is required`},
		{"channel asset path", mutate(func(s *Source) { s.Channels = map[string]Channel{"nightly": {Asset: "dir/f_{os}.tar.gz"}} }), "bare file name"},
		{"channel asset version", mutate(func(s *Source) { s.Channels = map[string]Channel{"nightly": {Asset: "f_{version}_{os}.tar.gz"}} }), "a channel release has no version"},
		{"channel asset target", mutate(func(s *Source) { s.Channels = map[string]Channel{"nightly": {Asset: "f_{target}.tar.gz"}} }), "uses {target} but no [source.target] table"},
		{"channel asset kind", mutate(func(s *Source) { s.Channels = map[string]Channel{"nightly": {Asset: "f_{os}_{arch}"}} }), "is not the same kind of artifact as"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.src.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRecipeValidate(t *testing.T) {
	t.Parallel()
	valid := Recipe{Name: "foundry", Ecosystems: []string{"ethereum"}, Description: "Fast Ethereum application toolkit", Source: foundry()}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := valid
	bad.Name = "Foundry"
	if err := bad.Validate(); err == nil {
		t.Error("upper-case name accepted")
	}
	bad = valid
	bad.Source = Source{}
	err := bad.Validate()
	if err == nil || !strings.Contains(err.Error(), `tool "foundry"`) {
		t.Errorf("error should name the tool: %v", err)
	}
}

func TestRecipeDescription(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, description, want string
	}{
		{"empty", "", "description is required"},
		{"blank", "   ", "description is required"},
		{"padded", " a tool ", "leading or trailing whitespace"},
		{"multi line", "a tool\nand more", "single line"},
		{"tab", "a\ttool", "single line"},
		{"too long", strings.Repeat("x", maxDescription+1), "keep it under 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := Recipe{Name: "foundry", Ecosystems: []string{"ethereum"}, Description: tt.description, Source: foundry()}
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() error = %v, want containing %q", err, tt.want)
			}
			if err != nil && !strings.Contains(err.Error(), `tool "foundry"`) {
				t.Errorf("error should name the tool: %v", err)
			}
		})
	}
	ok := Recipe{Name: "foundry", Ecosystems: []string{"ethereum"}, Description: strings.Repeat("x", maxDescription), Source: foundry()}
	if err := ok.Validate(); err != nil {
		t.Errorf("a description of exactly the maximum length was rejected: %v", err)
	}
}

func TestValidateName(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"foundry", "go-ethereum", "solc_js", "a1"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Foundry", "foundry rs", "foundry/rs", "日本語", "foo.bar"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) accepted", bad)
		}
	}
}

func TestTagRoundTrip(t *testing.T) {
	t.Parallel()
	v := version.MustParse("1.7.4")
	src := foundry()
	if got := src.Tag(v); got != "v1.7.4" {
		t.Errorf("Tag() = %q", got)
	}
	if got, ok := src.ParseTag("v1.7.4"); !ok || got != v {
		t.Errorf("ParseTag(v1.7.4) = %v, %v", got, ok)
	}
	for _, bad := range []string{"1.7.4", "nightly-abc", "stable", "v1", "vv1.7.4"} {
		if _, ok := src.ParseTag(bad); ok {
			t.Errorf("ParseTag(%q) accepted", bad)
		}
	}
	empty := ""
	bare := Source{TagPrefix: &empty}
	if got := bare.Tag(v); got != "1.7.4" {
		t.Errorf("bare Tag() = %q", got)
	}
	if _, ok := bare.ParseTag("v1.7.4"); ok {
		t.Error("bare prefix accepted a v tag")
	}
	if got, ok := bare.ParseTag("1.7.4"); !ok || got != v {
		t.Errorf("bare ParseTag = %v, %v", got, ok)
	}
	rel := "release-"
	custom := Source{TagPrefix: &rel}
	if got, ok := custom.ParseTag("release-2.0.0"); !ok || got.Major != 2 {
		t.Errorf("custom ParseTag = %v, %v", got, ok)
	}
}

func TestSupportedPlatforms(t *testing.T) {
	t.Parallel()
	all := Source{}
	if got := all.SupportedPlatforms(); len(got) != len(platform.Supported()) {
		t.Errorf("empty platforms should mean all, got %v", got)
	}
	got := platform.Strings(foundry().SupportedPlatforms())
	if strings.Join(got, ",") != "darwin/arm64,linux/amd64" {
		t.Errorf("SupportedPlatforms() = %v", got)
	}
	if !foundry().Supports(platform.Platform{OS: "linux", Arch: "amd64"}) || foundry().Supports(platform.Platform{OS: "linux", Arch: "arm64"}) {
		t.Error("Supports is wrong")
	}
}

func TestAssetName(t *testing.T) {
	t.Parallel()
	v := version.MustParse("1.7.4")
	got, err := foundry().AssetName(v, platform.Platform{OS: "linux", Arch: "amd64"})
	if err != nil || got != "foundry_v1.7.4_linux_amd64.tar.gz" {
		t.Errorf("AssetName() = %q, %v", got, err)
	}
	got, err = hermes().AssetName(version.MustParse("1.13.0"), platform.Platform{OS: "darwin", Arch: "arm64"})
	if err != nil || got != "hermes-v1.13.0-aarch64-apple-darwin.tar.gz" {
		t.Errorf("AssetName() = %q, %v", got, err)
	}
	_, err = foundry().AssetName(v, platform.Platform{OS: "linux", Arch: "arm64"})
	var unsupported *UnsupportedPlatformError
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %v, want UnsupportedPlatformError", err)
	}
	if unsupported.Error() != "unsupported platform linux/arm64 (available: darwin/arm64, linux/amd64)" {
		t.Errorf("Error() = %q", unsupported.Error())
	}
}

func TestEqual(t *testing.T) {
	t.Parallel()
	a, b := foundry(), foundry()
	if !a.Equal(b) {
		t.Fatal("identical sources differ")
	}
	b.Platforms = []string{"darwin/arm64", "linux/amd64"}
	if !a.Equal(b) {
		t.Error("platform order should not matter")
	}
	v := "v"
	b.TagPrefix = &v
	if !a.Equal(b) {
		t.Error("explicit default prefix should equal implicit")
	}
	cases := []func(*Source){
		func(s *Source) { s.Repo = "other/repo" },
		func(s *Source) { s.Asset = "x_{version}.zip" },
		func(s *Source) { s.Bin = []string{"cast", "forge"} },
		func(s *Source) { s.Platforms = []string{"linux/amd64"} },
		func(s *Source) { s.OS = map[string]string{"linux": "Linux"} },
		func(s *Source) { s.Arch = map[string]string{"amd64": "x64"} },
		func(s *Source) { e := ""; s.TagPrefix = &e },
	}
	for i, f := range cases {
		c := foundry()
		f(&c)
		if a.Equal(c) {
			t.Errorf("case %d: differing sources reported equal", i)
		}
	}
	h1, h2 := hermes(), hermes()
	h2.OS["linux"] = "linux-musl"
	if h1.Equal(h2) {
		t.Error("map value change not detected")
	}
}

func TestHash(t *testing.T) {
	t.Parallel()
	a, b := foundry(), foundry()
	if a.Hash() != b.Hash() || !strings.HasPrefix(a.Hash(), "sha256:") {
		t.Fatalf("Hash() unstable: %s vs %s", a.Hash(), b.Hash())
	}
	b.Platforms = []string{"darwin/arm64", "linux/amd64"}
	if a.Hash() != b.Hash() {
		t.Error("platform order must not change the hash")
	}
	v := "v"
	b.TagPrefix = &v
	if a.Hash() != b.Hash() {
		t.Error("explicit default prefix must not change the hash")
	}
	for i, f := range []func(*Source){
		func(s *Source) { s.Repo = "other/repo" },
		func(s *Source) { s.Asset = "x_{version}.zip" },
		func(s *Source) { s.Bin = []string{"cast", "forge"} },
		func(s *Source) { s.Platforms = []string{"linux/amd64"} },
		func(s *Source) { s.OS = map[string]string{"linux": "Linux"} },
		func(s *Source) { s.Arch = map[string]string{"amd64": "x64"} },
		func(s *Source) { e := ""; s.TagPrefix = &e },
	} {
		c := foundry()
		f(&c)
		if a.Hash() == c.Hash() {
			t.Errorf("case %d: differing sources hash equal", i)
		}
	}
	if hermes().Hash() == foundry().Hash() {
		t.Error("different recipes hash equal")
	}
}

func TestRecipeEcosystems(t *testing.T) {
	t.Parallel()
	base := Recipe{Name: "hermes", Description: "IBC relayer", Source: foundry()}
	multi := base
	multi.Ecosystems = []string{"cosmos", "ibc"}
	if err := multi.Validate(); err != nil {
		t.Fatalf("a tool serving two ecosystems was rejected: %v", err)
	}
	tests := []struct {
		name string
		eco  []string
		want string
	}{
		{"missing", nil, "ecosystems is required"},
		{"empty", []string{}, "ecosystems is required"},
		{"blank name", []string{""}, "ecosystem: tool name is empty"},
		{"upper case", []string{"Ethereum"}, `invalid tool name "Ethereum"`},
		{"spaced", []string{"cosmos sdk"}, "invalid tool name"},
		{"duplicate", []string{"cosmos", "cosmos"}, `ecosystem "cosmos" is listed twice`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := base
			r.Ecosystems = tt.eco
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() error = %v, want containing %q", err, tt.want)
			}
			if err != nil && !strings.Contains(err.Error(), `tool "hermes"`) {
				t.Errorf("error should name the tool: %v", err)
			}
		})
	}
}

func TestTargetMap(t *testing.T) {
	t.Parallel()
	// Bitcoin Core spells arm64 differently per OS, so whole platform
	// strings are mapped instead of {os}/{arch}.
	src := Source{
		Type: TypeHTTP, Repo: "bitcoin/bitcoin",
		URL: "https://example.org/bitcoin-{version}-{target}.tar.gz",
		Target: map[string]string{
			"linux/arm64":  "aarch64-linux-gnu",
			"darwin/arm64": "arm64-apple-darwin",
		},
		Bin: []string{"bin/bitcoind"}, StripComponents: 1,
	}
	if err := src.Validate(); err != nil {
		t.Fatal(err)
	}
	// With no platforms list, the target keys are the supported platforms.
	if got := platform.Strings(src.SupportedPlatforms()); strings.Join(got, ",") != "darwin/arm64,linux/arm64" {
		t.Errorf("SupportedPlatforms() = %v", got)
	}
	got, err := src.Render(version.MustParse("29.4"), platform.Platform{OS: "darwin", Arch: "arm64"}, "")
	if err != nil || got != "https://example.org/bitcoin-29.4-arm64-apple-darwin.tar.gz" {
		t.Errorf("Render() = %q, %v", got, err)
	}
	if _, err := src.Render(version.MustParse("29.4"), platform.Platform{OS: "linux", Arch: "amd64"}, ""); err == nil {
		t.Error("rendered an unmapped platform")
	}
	missing := src
	missing.Target = nil
	if err := missing.Validate(); err == nil || !strings.Contains(err.Error(), "no [source.target] table") {
		t.Errorf("Validate() error = %v", err)
	}
	other := src
	other.Target = map[string]string{"linux/arm64": "aarch64-linux-gnu", "darwin/arm64": "other"}
	if src.Equal(other) || src.Hash() == other.Hash() {
		t.Error("target must be part of identity")
	}
	bad := src
	bad.Target = map[string]string{"plan9/amd64": "x"}
	if err := bad.Validate(); err == nil {
		t.Error("an unsupported platform key was accepted")
	}
}

func TestHTTPAndRawSources(t *testing.T) {
	t.Parallel()
	geth := Source{Type: TypeHTTP, Repo: "ethereum/go-ethereum", URL: "https://dl/geth-{os}-{arch}-{version}-{commit}.tar.gz", StripComponents: 1, Bin: []string{"geth"}}
	if err := geth.Validate(); err != nil {
		t.Fatal(err)
	}
	if !geth.NeedsCommit() || !geth.IsArchive() {
		t.Error("NeedsCommit/IsArchive wrong for geth")
	}
	got, err := geth.Render(version.MustParse("1.17.5"), platform.Platform{OS: "linux", Arch: "arm64"}, "9621c6ad10934a01b5514886fb6fbd87640b6c05")
	if err != nil || got != "https://dl/geth-linux-arm64-1.17.5-9621c6ad.tar.gz" {
		t.Errorf("Render() = %q, %v", got, err)
	}
	if _, err := geth.Render(version.MustParse("1.17.5"), platform.Platform{OS: "linux", Arch: "arm64"}, ""); err == nil {
		t.Error("Render without a commit succeeded")
	}
	solc := Source{Type: TypeGitHubRelease, Repo: "argotorg/solidity", Asset: "solc-{os}", OS: map[string]string{"linux": "static-linux"}, Bin: []string{"solc"}}
	if err := solc.Validate(); err != nil {
		t.Fatal(err)
	}
	if solc.IsArchive() || solc.NeedsCommit() {
		t.Error("raw executable misclassified")
	}
	if !IsArchiveName("a.tgz") || IsArchiveName("solc-macos") {
		t.Error("IsArchiveName wrong")
	}
	a, b := geth, geth
	b.StripComponents = 0
	if a.Equal(b) || a.Hash() == b.Hash() {
		t.Error("strip_components must be part of identity")
	}
	b = geth
	b.URL = "https://other/{version}.tar.gz"
	if a.Equal(b) || a.Hash() == b.Hash() {
		t.Error("url must be part of identity")
	}
}

func TestValidateBinRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"forge", "bin/forge", "a/b/c"} {
		if err := ValidateBin(ok); err != nil {
			t.Errorf("ValidateBin(%q) = %v", ok, err)
		}
	}
	// A lockfile is untrusted input: none of these may be accepted, because
	// each one could place a file outside the install directory.
	for _, bad := range []string{
		"", "/usr/bin/forge", "../forge", "a/../../forge", "./forge", "bin/./forge",
		"bin//forge", "bin/", ".", "..", "a\\b", "C:/forge", "forge\x00",
	} {
		if err := ValidateBin(bad); err == nil {
			t.Errorf("ValidateBin(%q) accepted", bad)
		}
	}
	if CommandName("bin/solana") != "solana" || CommandName("forge") != "forge" {
		t.Error("CommandName is wrong")
	}
	dup := foundry()
	dup.Bin = []string{"forge", "forge"}
	if err := dup.Validate(); err == nil || !strings.Contains(err.Error(), `bin "forge" is listed twice`) {
		t.Errorf("Validate() error = %v", err)
	}
}

// A repository is spliced into API URLs as written, so a character GitHub
// never puts in an owner or a name is a typo to refuse here rather than a
// "not found" from some other endpoint.
func TestValidateRefusesRepoCharactersGitHubDoesNotUse(t *testing.T) {
	t.Parallel()
	for _, repo := range []string{"foundry rs/foundry", "foundry-rs/foundry?x=1", "a/b#c", "a%2Fb/c", "./x", "a/..", "ünicode/x", "a/b\n"} {
		s := Source{Type: TypeGitHubRelease, Repo: repo, Asset: "x.tar.gz", Bin: []string{"x"}}
		err := s.Validate()
		if err == nil || !strings.Contains(err.Error(), "invalid repo") {
			t.Errorf("Validate(repo %q) = %v, want an invalid repo error", repo, err)
		}
	}
	for _, repo := range []string{"foundry-rs/foundry", "informalsystems/hermes", "a.b/c_d", "A1/B-2", "org/repo.js"} {
		s := Source{Type: TypeGitHubRelease, Repo: repo, Asset: "x.tar.gz", Bin: []string{"x"}}
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(repo %q) = %v", repo, err)
		}
	}
}

// ValidateBin is the whole defence between a lockfile entry and a path under
// $BLOCK_HOME, so an accepted entry must always resolve inside the install
// directory and never outside it.
func FuzzValidateBin(f *testing.F) {
	for _, s := range []string{"forge", "bin/forge", "../x", "/x", "a/../../b", "a//b", "./a", "", "a\\b", "C:x", "a\x00b", "..", "."} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b string) {
		if err := ValidateBin(b); err != nil {
			return
		}
		if path.Clean(b) != b || path.IsAbs(b) || strings.HasPrefix(b, "../") || b == ".." || b == "." {
			t.Fatalf("ValidateBin(%q) accepted an entry that is not a clean relative path", b)
		}
		rel := filepath.FromSlash(b)
		joined := filepath.Join("root", rel)
		if r, err := filepath.Rel("root", joined); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			t.Fatalf("ValidateBin(%q) accepted an entry that escapes the install directory: %q", b, joined)
		}
		if CommandName(b) == "" || CommandName(b) == "." || CommandName(b) == ".." || strings.Contains(CommandName(b), "/") {
			t.Fatalf("ValidateBin(%q) accepted an entry with no command name (%q)", b, CommandName(b))
		}
	})
}

// A recipe that claims a platform its {target} table has no name for used to
// validate, and then refuse that platform at resolution time with
// "unsupported platform linux/arm64 (available: ..., linux/arm64, ...)" — an
// error contradicting itself that only the recipe's author could fix.
func TestValidateRequiresATargetForEveryPlatform(t *testing.T) {
	t.Parallel()
	missing := Source{
		Type: TypeHTTP, Repo: "bitcoin/bitcoin",
		URL:       "https://x/bitcoin-{version}-{target}.tar.gz",
		Platforms: []string{"linux/amd64", "linux/arm64"},
		Target:    map[string]string{"linux/amd64": "x86_64-linux-gnu"},
		Bin:       []string{"bitcoind"},
	}
	err := missing.Validate()
	if err == nil || !strings.Contains(err.Error(), "platform linux/arm64 is listed but [source.target] has no name for it") {
		t.Fatalf("Validate() = %v, want a refusal naming the platform", err)
	}
	// Covered: the same recipe with the mapping filled in.
	complete := missing
	complete.Target = map[string]string{"linux/amd64": "x86_64-linux-gnu", "linux/arm64": "aarch64-linux-gnu"}
	if err := complete.Validate(); err != nil {
		t.Errorf("Validate(complete) = %v", err)
	}
	// And a recipe whose platforms come from the table's own keys, which is
	// how every registry recipe using {target} is written.
	fromKeys := complete
	fromKeys.Platforms = nil
	if err := fromKeys.Validate(); err != nil {
		t.Errorf("Validate(platforms from the target keys) = %v", err)
	}
}

// [Source.Hash] joins SupportedPlatforms and block.lock records that hash for
// a project-local source, so the order this returns is part of the lockfile
// format: reordering it would stale every such lockfile for no reason a
// reader could see. It is pinned here rather than left to be noticed.
func TestSupportedPlatformsOrderIsPartOfTheLockFormat(t *testing.T) {
	t.Parallel()
	base := Source{Type: TypeGitHubRelease, Repo: "o/r", Asset: "x.tar.gz", Bin: []string{"x"}}
	if got := platform.Strings(base.SupportedPlatforms()); strings.Join(got, ",") != strings.Join(platform.Strings(platform.Supported()), ",") {
		t.Errorf("default platforms = %v, want %v", got, platform.Supported())
	}
	// A declared list is sorted, whatever order it was written in.
	declared := base
	declared.Platforms = []string{"linux/arm64", "darwin/amd64", "linux/amd64"}
	if got := strings.Join(platform.Strings(declared.SupportedPlatforms()), ","); got != "darwin/amd64,linux/amd64,linux/arm64" {
		t.Errorf("declared platforms = %q, want them sorted", got)
	}
	// So two spellings of one list are one pin.
	other := declared
	other.Platforms = []string{"darwin/amd64", "linux/amd64", "linux/arm64"}
	if declared.Hash() != other.Hash() {
		t.Error("the order platforms are written in changed the source hash")
	}
}

// Windows resolves a command on PATH without regard to case, so "foo" and
// "FOO" are one command there and two everywhere else. A lockfile is
// committed and read on every platform, so the collision is refused
// everywhere — found by whoever runs lock, not by whoever runs Windows.
func TestValidateBinsRefusesCommandsDifferingOnlyInCase(t *testing.T) {
	t.Parallel()
	err := ValidateBins([]string{"forge", "FORGE"})
	if err == nil || !strings.Contains(err.Error(), "both the command") {
		t.Fatalf("ValidateBins(forge, FORGE) = %v, want a refusal", err)
	}
	// Two paths ending in the same name, differing only in case, are the
	// same collision.
	if err := ValidateBins([]string{"bin/cast", "sbin/CAST"}); err == nil {
		t.Error("ValidateBins accepted bin/cast beside sbin/CAST")
	}
	// And CommandKey is what decides, so it folds case and nothing else.
	if CommandKey("bin/Forge") != "forge" || CommandKey("forge") != CommandKey("FORGE") {
		t.Errorf("CommandKey folds something other than case: %q", CommandKey("bin/Forge"))
	}
	// Commands that really are different are still allowed.
	if err := ValidateBins([]string{"forge", "cast", "bin/anvil"}); err != nil {
		t.Errorf("ValidateBins(distinct commands) = %v", err)
	}
}
