package recipe

import (
	"errors"
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
		{"type", mutate(func(s *Source) { s.Type = "http" }), `unsupported source type "http"`},
		{"repo empty", mutate(func(s *Source) { s.Repo = "" }), "invalid repo"},
		{"repo no slash", mutate(func(s *Source) { s.Repo = "foundry" }), "invalid repo"},
		{"repo extra slash", mutate(func(s *Source) { s.Repo = "a/b/c" }), "invalid repo"},
		{"asset empty", mutate(func(s *Source) { s.Asset = "" }), "asset template is required"},
		{"asset no version", mutate(func(s *Source) { s.Asset = "foundry_{os}.tar.gz" }), "must contain {version}"},
		{"asset path", mutate(func(s *Source) { s.Asset = "dir/foundry_{version}.tar.gz" }), "bare file name"},
		{"asset format", mutate(func(s *Source) { s.Asset = "foundry_{version}.tar.xz" }), "must end in .tar.gz, .tgz or .zip"},
		{"bin empty", mutate(func(s *Source) { s.Bin = nil }), "bin must list"},
		{"bin blank", mutate(func(s *Source) { s.Bin = []string{""} }), "bin entry is empty"},
		{"bin abs", mutate(func(s *Source) { s.Bin = []string{"/usr/bin/forge"} }), "invalid bin entry"},
		{"bin traversal", mutate(func(s *Source) { s.Bin = []string{"../forge"} }), "invalid bin entry"},
		{"bin unclean", mutate(func(s *Source) { s.Bin = []string{"bin/./forge"} }), "invalid bin entry"},
		{"platform", mutate(func(s *Source) { s.Platforms = []string{"windows/amd64"} }), "unsupported platform"},
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
	if err := (Recipe{Name: "foundry", Source: foundry()}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Recipe{Name: "Foundry", Source: foundry()}).Validate(); err == nil {
		t.Error("upper-case name accepted")
	}
	err := (Recipe{Name: "foundry", Source: Source{}}).Validate()
	if err == nil || !strings.Contains(err.Error(), `tool "foundry"`) {
		t.Errorf("error should name the tool: %v", err)
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
	for _, bad := range []string{"1.7.4", "nightly-abc", "stable", "v1.7", "vv1.7.4"} {
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
