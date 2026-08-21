package registry

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/version"
)

func TestBuiltin(t *testing.T) {
	t.Parallel()
	r, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Names(), ","); got != "foundry,geth,hermes,solc" {
		t.Errorf("Names() = %s", got)
	}
	foundry, ok := r.Lookup("foundry")
	if !ok {
		t.Fatal("foundry missing")
	}
	name, err := foundry.Source.AssetName(version.MustParse("1.7.4"), platform.Platform{OS: "darwin", Arch: "arm64"})
	if err != nil || name != "foundry_v1.7.4_darwin_arm64.tar.gz" {
		t.Errorf("foundry asset = %q, %v", name, err)
	}
	hermes, _ := r.Lookup("hermes")
	name, err = hermes.Source.AssetName(version.MustParse("1.13.3"), platform.Platform{OS: "linux", Arch: "amd64"})
	if err != nil || name != "hermes-v1.13.3-x86_64-unknown-linux-gnu.tar.gz" {
		t.Errorf("hermes asset = %q, %v", name, err)
	}
	geth, _ := r.Lookup("geth")
	url, err := geth.Source.Render(version.MustParse("1.17.5"), platform.Platform{OS: "linux", Arch: "amd64"}, "9621c6ad10934a01b5514886fb6fbd87640b6c05")
	if err != nil || url != "https://gethstore.blob.core.windows.net/builds/geth-linux-amd64-1.17.5-9621c6ad.tar.gz" || geth.Source.StripComponents != 1 {
		t.Errorf("geth url = %q, %v", url, err)
	}
	if geth.Source.Supports(platform.Platform{OS: "darwin", Arch: "arm64"}) {
		t.Error("geth has no macOS builds")
	}
	solc, _ := r.Lookup("solc")
	for p, want := range map[platform.Platform]string{
		{OS: "linux", Arch: "amd64"}:  "solc-static-linux",
		{OS: "darwin", Arch: "arm64"}: "solc-macos",
	} {
		name, err := solc.Source.AssetName(version.MustParse("0.8.30"), p)
		if err != nil || name != want {
			t.Errorf("solc asset for %s = %q, %v", p, name, err)
		}
	}
	if solc.Source.IsArchive() {
		t.Error("solc ships raw executables")
	}
	if _, ok := r.Lookup("reth"); ok {
		t.Error("unexpected recipe")
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		fs   fstest.MapFS
		want string
	}{
		{"syntax", fstest.MapFS{"a.toml": {Data: []byte("name = \n")}}, "a.toml"},
		{"unknown key", fstest.MapFS{"a.toml": {Data: []byte("name = \"a\"\nhomepage = \"x\"\n[source]\ntype = \"github_release\"\nrepo = \"o/r\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\n")}}, `unknown key "homepage"`},
		{"invalid", fstest.MapFS{"a.toml": {Data: []byte("name = \"a\"\n[source]\ntype = \"github_release\"\n")}}, `tool "a"`},
		{"name mismatch", fstest.MapFS{"b.toml": {Data: []byte("name = \"a\"\n[source]\ntype = \"github_release\"\nrepo = \"o/r\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\n")}}, `recipe name "a" does not match the file name`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(tt.fs)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load() error = %v, want containing %q", err, tt.want)
			}
		})
	}
	r, err := Load(fstest.MapFS{"README.md": {Data: []byte("docs")}, "sub": {Mode: 0o755 | 1<<31}})
	if err != nil || len(r.Names()) != 0 {
		t.Errorf("Load(non-recipes) = %v, %v", r, err)
	}
}
