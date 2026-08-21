package registry

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/version"
)

// want describes what one embedded recipe must render, so that a typo in a
// TOML file fails here instead of at a user's first lock.
type want struct {
	ecosystem  string
	sourceKind string
	// artifacts maps "os/arch" to the asset name or url the recipe renders
	// for the sample version. A platform absent from the map must not be
	// supported by the recipe.
	artifacts map[string]string
	sample    string
	commit    string
	bins      []string
	strip     int
}

//nolint:gochecknoglobals // table shared by the tests below
var recipes = map[string]want{
	"bitcoin-core": {
		ecosystem: "bitcoin", sourceKind: recipe.TypeHTTP, sample: "29.4", commit: "abcdef1234",
		artifacts: map[string]string{
			"linux/amd64":  "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-x86_64-linux-gnu.tar.gz",
			"linux/arm64":  "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-aarch64-linux-gnu.tar.gz",
			"darwin/amd64": "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-arm64-apple-darwin.tar.gz",
		},
		bins: []string{"bin/bitcoind", "bin/bitcoin-cli", "bin/bitcoin-tx", "bin/bitcoin-util", "bin/bitcoin-wallet"}, strip: 1,
	},
	"foundry": {
		ecosystem: "ethereum", sourceKind: recipe.TypeGitHubRelease, sample: "1.7.1",
		artifacts: map[string]string{
			"linux/amd64":  "foundry_v1.7.1_linux_amd64.tar.gz",
			"linux/arm64":  "foundry_v1.7.1_linux_arm64.tar.gz",
			"darwin/amd64": "foundry_v1.7.1_darwin_amd64.tar.gz",
			"darwin/arm64": "foundry_v1.7.1_darwin_arm64.tar.gz",
		},
		bins: []string{"forge", "cast", "anvil", "chisel"},
	},
	"solc": {
		ecosystem: "ethereum", sourceKind: recipe.TypeGitHubRelease, sample: "0.8.36",
		artifacts: map[string]string{
			"linux/amd64":  "solc-static-linux",
			"darwin/amd64": "solc-macos",
			"darwin/arm64": "solc-macos",
		},
		bins: []string{"solc"},
	},
	"geth": {
		ecosystem: "ethereum", sourceKind: recipe.TypeHTTP, sample: "1.17.5", commit: "9621c6ad10934a01",
		artifacts: map[string]string{
			"linux/amd64": "https://gethstore.blob.core.windows.net/builds/geth-linux-amd64-1.17.5-9621c6ad.tar.gz",
			"linux/arm64": "https://gethstore.blob.core.windows.net/builds/geth-linux-arm64-1.17.5-9621c6ad.tar.gz",
		},
		bins: []string{"geth"}, strip: 1,
	},
	"reth": {
		ecosystem: "ethereum", sourceKind: recipe.TypeGitHubRelease, sample: "2.5.1",
		artifacts: map[string]string{
			"linux/amd64":  "reth-v2.5.1-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "reth-v2.5.1-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/arm64": "reth-v2.5.1-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"reth"},
	},
	"lighthouse": {
		ecosystem: "ethereum", sourceKind: recipe.TypeGitHubRelease, sample: "8.2.2",
		artifacts: map[string]string{
			"linux/amd64":  "lighthouse-v8.2.2-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "lighthouse-v8.2.2-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/arm64": "lighthouse-v8.2.2-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"lighthouse"},
	},
	"agave": {
		ecosystem: "solana", sourceKind: recipe.TypeGitHubRelease, sample: "4.2.1",
		artifacts: map[string]string{
			"linux/amd64":  "solana-release-x86_64-unknown-linux-gnu.tar.bz2",
			"darwin/amd64": "solana-release-x86_64-apple-darwin.tar.bz2",
			"darwin/arm64": "solana-release-aarch64-apple-darwin.tar.bz2",
		},
		bins: []string{"bin/solana", "bin/solana-keygen", "bin/solana-test-validator", "bin/agave-ledger-tool"}, strip: 1,
	},
	"anchor": {
		ecosystem: "solana", sourceKind: recipe.TypeGitHubRelease, sample: "1.1.2",
		artifacts: map[string]string{
			"linux/amd64":  "anchor-1.1.2-x86_64-unknown-linux-gnu",
			"darwin/amd64": "anchor-1.1.2-x86_64-apple-darwin",
			"darwin/arm64": "anchor-1.1.2-aarch64-apple-darwin",
		},
		bins: []string{"anchor"},
	},
	"surfpool": {
		ecosystem: "solana", sourceKind: recipe.TypeGitHubRelease, sample: "1.5.0",
		artifacts: map[string]string{
			"linux/amd64":  "surfpool-linux-x64.tar.gz",
			"darwin/amd64": "surfpool-darwin-x64.tar.gz",
			"darwin/arm64": "surfpool-darwin-arm64.tar.gz",
		},
		bins: []string{"surfpool"},
	},
	"gaia": {
		ecosystem: "cosmos", sourceKind: recipe.TypeGitHubRelease, sample: "27.6.0",
		artifacts: map[string]string{
			"linux/amd64":  "gaiad-v27.6.0-linux-amd64",
			"darwin/amd64": "gaiad-v27.6.0-darwin-amd64",
		},
		bins: []string{"gaiad"},
	},
	"cometbft": {
		ecosystem: "cosmos", sourceKind: recipe.TypeGitHubRelease, sample: "1.0.1",
		artifacts: map[string]string{
			"linux/amd64":  "cometbft_1.0.1_linux_amd64.tar.gz",
			"linux/arm64":  "cometbft_1.0.1_linux_arm64.tar.gz",
			"darwin/amd64": "cometbft_1.0.1_darwin_amd64.tar.gz",
			"darwin/arm64": "cometbft_1.0.1_darwin_arm64.tar.gz",
		},
		bins: []string{"cometbft"},
	},
	"osmosis": {
		ecosystem: "cosmos", sourceKind: recipe.TypeGitHubRelease, sample: "31.0.3",
		artifacts: map[string]string{
			"linux/amd64":  "osmosisd-31.0.3-linux-amd64.tar.gz",
			"linux/arm64":  "osmosisd-31.0.3-linux-arm64.tar.gz",
			"darwin/amd64": "osmosisd-31.0.3-darwin-amd64.tar.gz",
			"darwin/arm64": "osmosisd-31.0.3-darwin-arm64.tar.gz",
		},
		bins: []string{"osmosisd"},
	},
	"hermes": {
		ecosystem: "ibc", sourceKind: recipe.TypeGitHubRelease, sample: "1.13.3",
		artifacts: map[string]string{
			"linux/amd64":  "hermes-v1.13.3-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "hermes-v1.13.3-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/amd64": "hermes-v1.13.3-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "hermes-v1.13.3-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"hermes"},
	},
}

func TestBuiltinCoversEveryRecipe(t *testing.T) {
	t.Parallel()
	r, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Names()) != len(recipes) {
		t.Fatalf("registry has %v but the test table has %d entries", r.Names(), len(recipes))
	}
	for _, name := range r.Names() {
		w, ok := recipes[name]
		if !ok {
			t.Errorf("recipe %s is not covered by the test table", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec, _ := r.Lookup(name)
			if rec.Ecosystem != w.ecosystem {
				t.Errorf("ecosystem = %q, want %q", rec.Ecosystem, w.ecosystem)
			}
			if rec.Source.Type != w.sourceKind {
				t.Errorf("type = %q, want %q", rec.Source.Type, w.sourceKind)
			}
			if strings.Join(rec.Source.Bin, ",") != strings.Join(w.bins, ",") {
				t.Errorf("bin = %v, want %v", rec.Source.Bin, w.bins)
			}
			if rec.Source.StripComponents != w.strip {
				t.Errorf("strip_components = %d, want %d", rec.Source.StripComponents, w.strip)
			}
			v := version.MustParse(w.sample)
			for _, p := range platform.Supported() {
				got, err := rec.Source.Render(v, p, w.commit)
				expect, supported := w.artifacts[p.String()]
				switch {
				case !supported:
					if err == nil {
						t.Errorf("%s: rendered %q for an unsupported platform", p, got)
					}
				case err != nil:
					t.Errorf("%s: %v", p, err)
				case got != expect:
					t.Errorf("%s: rendered %q, want %q", p, got, expect)
				}
			}
		})
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
