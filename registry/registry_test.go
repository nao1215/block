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
	if got := strings.Join(r.Names(), ","); got != "foundry,hermes" {
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
	if _, ok := r.Lookup("geth"); ok {
		t.Error("geth should not be registered yet")
	}
	// Every recipe must ship for every platform block supports, so that a
	// multi-platform block.toml never fails on a registry tool.
	for _, n := range r.Names() {
		rec, _ := r.Lookup(n)
		for _, p := range platform.Supported() {
			if !rec.Source.Supports(p) {
				t.Errorf("%s does not support %s", n, p)
			}
		}
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
