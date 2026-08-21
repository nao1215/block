package registry

import (
	"slices"
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
	ecosystems []string
	// description is the exact sentence the recipe must carry, so that a
	// reworded description is a deliberate change rather than a drive-by one.
	description string
	sourceKind  string
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
		ecosystems: []string{"bitcoin"}, description: "Bitcoin reference implementation: full node, wallet and transaction tools",
		sourceKind: recipe.TypeHTTP, sample: "29.4", commit: "abcdef1234",
		artifacts: map[string]string{
			"linux/amd64":  "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-x86_64-linux-gnu.tar.gz",
			"linux/arm64":  "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-aarch64-linux-gnu.tar.gz",
			"darwin/amd64": "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "https://bitcoincore.org/bin/bitcoin-core-29.4/bitcoin-29.4-arm64-apple-darwin.tar.gz",
		},
		bins: []string{"bin/bitcoind", "bin/bitcoin-cli", "bin/bitcoin-tx", "bin/bitcoin-util", "bin/bitcoin-wallet"}, strip: 1,
	},
	"foundry": {
		ecosystems: []string{"ethereum"}, description: "Fast Ethereum application toolkit: build, test, deploy and inspect contracts",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.7.1",
		artifacts: map[string]string{
			"linux/amd64":  "foundry_v1.7.1_linux_amd64.tar.gz",
			"linux/arm64":  "foundry_v1.7.1_linux_arm64.tar.gz",
			"darwin/amd64": "foundry_v1.7.1_darwin_amd64.tar.gz",
			"darwin/arm64": "foundry_v1.7.1_darwin_arm64.tar.gz",
		},
		bins: []string{"forge", "cast", "anvil", "chisel"},
	},
	"solc": {
		ecosystems: []string{"ethereum"}, description: "The Solidity smart-contract compiler",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.8.36",
		artifacts: map[string]string{
			"linux/amd64":  "solc-static-linux",
			"darwin/amd64": "solc-macos",
			"darwin/arm64": "solc-macos",
		},
		bins: []string{"solc"},
	},
	"geth": {
		ecosystems: []string{"ethereum"}, description: "go-ethereum, the Go implementation of an Ethereum execution client",
		sourceKind: recipe.TypeHTTP, sample: "1.17.5", commit: "9621c6ad10934a01",
		artifacts: map[string]string{
			"linux/amd64": "https://gethstore.blob.core.windows.net/builds/geth-linux-amd64-1.17.5-9621c6ad.tar.gz",
			"linux/arm64": "https://gethstore.blob.core.windows.net/builds/geth-linux-arm64-1.17.5-9621c6ad.tar.gz",
		},
		bins: []string{"geth"}, strip: 1,
	},
	"reth": {
		ecosystems: []string{"ethereum"}, description: "Modular Ethereum execution client written in Rust",
		sourceKind: recipe.TypeGitHubRelease, sample: "2.5.1",
		artifacts: map[string]string{
			"linux/amd64":  "reth-v2.5.1-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "reth-v2.5.1-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/arm64": "reth-v2.5.1-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"reth"},
	},
	"lighthouse": {
		ecosystems: []string{"ethereum"}, description: "Ethereum consensus (beacon chain) client written in Rust",
		sourceKind: recipe.TypeGitHubRelease, sample: "8.2.2",
		artifacts: map[string]string{
			"linux/amd64":  "lighthouse-v8.2.2-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "lighthouse-v8.2.2-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/arm64": "lighthouse-v8.2.2-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"lighthouse"},
	},
	"agave": {
		ecosystems: []string{"solana"}, description: "Solana validator client and CLI suite, including a local test validator",
		sourceKind: recipe.TypeGitHubRelease, sample: "4.2.1",
		artifacts: map[string]string{
			"linux/amd64":  "solana-release-x86_64-unknown-linux-gnu.tar.bz2",
			"darwin/amd64": "solana-release-x86_64-apple-darwin.tar.bz2",
			"darwin/arm64": "solana-release-aarch64-apple-darwin.tar.bz2",
		},
		bins: []string{"bin/solana", "bin/solana-keygen", "bin/solana-test-validator", "bin/agave-ledger-tool"}, strip: 1,
	},
	"anchor": {
		ecosystems: []string{"solana"}, description: "Framework and CLI for writing, testing and deploying Solana programs",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.1.2",
		artifacts: map[string]string{
			"linux/amd64":  "anchor-1.1.2-x86_64-unknown-linux-gnu",
			"darwin/amd64": "anchor-1.1.2-x86_64-apple-darwin",
			"darwin/arm64": "anchor-1.1.2-aarch64-apple-darwin",
		},
		bins: []string{"anchor"},
	},
	"surfpool": {
		ecosystems: []string{"solana"}, description: "Local Solana network that streams mainnet state for pre-deployment testing",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.5.0",
		artifacts: map[string]string{
			"linux/amd64":  "surfpool-linux-x64.tar.gz",
			"darwin/amd64": "surfpool-darwin-x64.tar.gz",
			"darwin/arm64": "surfpool-darwin-arm64.tar.gz",
		},
		bins: []string{"surfpool"},
	},
	"gaia": {
		ecosystems: []string{"cosmos"}, description: "Cosmos Hub node (gaiad)",
		sourceKind: recipe.TypeGitHubRelease, sample: "27.6.0",
		artifacts: map[string]string{
			"linux/amd64":  "gaiad-v27.6.0-linux-amd64",
			"darwin/amd64": "gaiad-v27.6.0-darwin-amd64",
		},
		bins: []string{"gaiad"},
	},
	"cometbft": {
		ecosystems: []string{"cosmos"}, description: "Byzantine fault-tolerant consensus engine and node behind Cosmos SDK chains",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.0.1",
		artifacts: map[string]string{
			"linux/amd64":  "cometbft_1.0.1_linux_amd64.tar.gz",
			"linux/arm64":  "cometbft_1.0.1_linux_arm64.tar.gz",
			"darwin/amd64": "cometbft_1.0.1_darwin_amd64.tar.gz",
			"darwin/arm64": "cometbft_1.0.1_darwin_arm64.tar.gz",
		},
		bins: []string{"cometbft"},
	},
	"osmosis": {
		ecosystems: []string{"cosmos"}, description: "Osmosis appchain node (osmosisd), the Cosmos AMM",
		sourceKind: recipe.TypeGitHubRelease, sample: "31.0.3",
		artifacts: map[string]string{
			"linux/amd64":  "osmosisd-31.0.3-linux-amd64.tar.gz",
			"linux/arm64":  "osmosisd-31.0.3-linux-arm64.tar.gz",
			"darwin/amd64": "osmosisd-31.0.3-darwin-amd64.tar.gz",
			"darwin/arm64": "osmosisd-31.0.3-darwin-arm64.tar.gz",
		},
		bins: []string{"osmosisd"},
	},
	"hermes": {
		ecosystems: []string{"cosmos", "ibc"}, description: "IBC relayer connecting Cosmos SDK chains, written in Rust",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.13.3",
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
	if len(r.Recipes()) != len(recipes) {
		t.Fatalf("registry has %d recipes but the test table has %d entries", len(r.Recipes()), len(recipes))
	}
	for _, name := range ecosystemNames(r) {
		w, ok := recipes[name]
		if !ok {
			t.Errorf("recipe %s is not covered by the test table", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec, _ := r.Lookup(name)
			if strings.Join(rec.Ecosystems, ",") != strings.Join(w.ecosystems, ",") {
				t.Errorf("ecosystems = %v, want %v", rec.Ecosystems, w.ecosystems)
			}
			if rec.Description != w.description {
				t.Errorf("description = %q, want %q", rec.Description, w.description)
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

// ecosystemNames lists the registry's tool names, sorted.
func ecosystemNames(r *Registry) []string {
	recs := r.Recipes()
	out := make([]string, len(recs))
	for i, rec := range recs {
		out[i] = rec.Name
	}
	return out
}

func TestEcosystemDiscovery(t *testing.T) {
	t.Parallel()
	r, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.Ecosystems(), ", "); got != "bitcoin, cosmos, ethereum, ibc, solana" {
		t.Errorf("Ecosystems() = %q", got)
	}
	// Every ecosystem a recipe claims is discoverable, and every recipe is
	// reachable through each ecosystem it claims.
	for _, rec := range r.Recipes() {
		for _, e := range rec.Ecosystems {
			if !slices.Contains(r.Ecosystems(), e) {
				t.Errorf("%s claims ecosystem %q that Ecosystems() does not list", rec.Name, e)
			}
			if !slices.ContainsFunc(r.ByEcosystem(e), func(x recipe.Recipe) bool { return x.Name == rec.Name }) {
				t.Errorf("%s is missing from ByEcosystem(%q)", rec.Name, e)
			}
		}
	}
	// A tool serving two ecosystems appears under both.
	for _, e := range []string{"cosmos", "ibc"} {
		if !slices.ContainsFunc(r.ByEcosystem(e), func(x recipe.Recipe) bool { return x.Name == "hermes" }) {
			t.Errorf("hermes is missing from ByEcosystem(%q)", e)
		}
	}
	names := func(recs []recipe.Recipe) string {
		out := make([]string, len(recs))
		for i, rec := range recs {
			out[i] = rec.Name
		}
		return strings.Join(out, ",")
	}
	if got := names(r.ByEcosystem("cosmos")); got != "cometbft,gaia,hermes,osmosis" {
		t.Errorf("ByEcosystem(cosmos) = %q (must be sorted by name)", got)
	}
	if got := r.ByEcosystem("sui"); got != nil {
		t.Errorf("ByEcosystem(unknown) = %v", got)
	}
	if got := strings.Join(ecosystemNames(r), ","); got != "agave,anchor,bitcoin-core,cometbft,foundry,gaia,geth,hermes,lighthouse,osmosis,reth,solc,surfpool" {
		t.Errorf("Recipes() = %q (must be sorted by name)", got)
	}
}

func TestEcosystemsAreSortedRegardlessOfRecipeOrder(t *testing.T) {
	t.Parallel()
	const body = `name = "tool"
ecosystems = ["ibc", "cosmos"]
description = "A tool"
[source]
type = "github_release"
repo = "o/r"
asset = "tool_{version}.tar.gz"
bin = ["tool"]
`
	r, err := Load(fstest.MapFS{"tool.toml": {Data: []byte(body)}})
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := r.Lookup("tool")
	if strings.Join(rec.Ecosystems, ",") != "cosmos,ibc" {
		t.Errorf("ecosystems = %v, want them sorted", rec.Ecosystems)
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
		{"unknown key", fstest.MapFS{"a.toml": {Data: []byte("name = \"a\"\necosystems = [\"x\"]\ndescription = \"A tool\"\nhomepage = \"x\"\n[source]\ntype = \"github_release\"\nrepo = \"o/r\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\n")}}, `unknown key "homepage"`},
		{"invalid", fstest.MapFS{"a.toml": {Data: []byte("name = \"a\"\necosystems = [\"x\"]\ndescription = \"A tool\"\n[source]\ntype = \"github_release\"\n")}}, `tool "a"`},
		{"no ecosystems", fstest.MapFS{"a.toml": {Data: []byte("name = \"a\"\ndescription = \"A tool\"\n[source]\ntype = \"github_release\"\nrepo = \"o/r\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\n")}}, `tool "a": ecosystems is required`},
		{"no description", fstest.MapFS{"a.toml": {Data: []byte("name = \"a\"\necosystems = [\"x\"]\n[source]\ntype = \"github_release\"\nrepo = \"o/r\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\n")}}, `tool "a": description is required`},
		{"name mismatch", fstest.MapFS{"b.toml": {Data: []byte("name = \"a\"\necosystems = [\"x\"]\ndescription = \"A tool\"\n[source]\ntype = \"github_release\"\nrepo = \"o/r\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\n")}}, `recipe name "a" does not match the file name`},
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
	if err != nil || len(r.Recipes()) != 0 {
		t.Errorf("Load(non-recipes) = %v, %v", r, err)
	}
}
