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
	"agave": {
		ecosystems: []string{"solana"}, description: "Solana validator client and CLI suite, including a local test validator",
		sourceKind: recipe.TypeGitHubRelease, sample: "4.2.1",
		artifacts: map[string]string{
			"linux/amd64":   "solana-release-x86_64-unknown-linux-gnu.tar.bz2",
			"darwin/amd64":  "solana-release-x86_64-apple-darwin.tar.bz2",
			"darwin/arm64":  "solana-release-aarch64-apple-darwin.tar.bz2",
			"windows/amd64": "solana-release-x86_64-pc-windows-msvc.tar.bz2",
		},
		bins: []string{"bin/solana", "bin/solana-keygen", "bin/solana-test-validator", "bin/agave-ledger-tool"}, strip: 1,
	},
	"anchor": {
		ecosystems: []string{"solana"}, description: "Framework and CLI for writing, testing and deploying Solana programs",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.1.2",
		artifacts: map[string]string{
			"linux/amd64":   "anchor-1.1.2-x86_64-unknown-linux-gnu",
			"darwin/amd64":  "anchor-1.1.2-x86_64-apple-darwin",
			"darwin/arm64":  "anchor-1.1.2-aarch64-apple-darwin",
			"windows/amd64": "anchor-1.1.2-x86_64-pc-windows-msvc.exe",
		},
		bins: []string{"anchor"},
	},
	"anvil-zksync": {
		ecosystems: []string{"ethereum", "zksync"}, description: "In-memory ZKsync node for local development and testing",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.6.11",
		artifacts: map[string]string{
			"linux/amd64":  "anvil-zksync-v0.6.11-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "anvil-zksync-v0.6.11-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/amd64": "anvil-zksync-v0.6.11-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "anvil-zksync-v0.6.11-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"anvil-zksync"},
	},
	"aptos": {
		ecosystems: []string{"aptos"}, description: "Aptos CLI: compile, test and publish Move packages and run a local network",
		sourceKind: recipe.TypeGitHubRelease, sample: "9.5.0",
		artifacts: map[string]string{
			"linux/amd64":   "aptos-cli-9.5.0-Linux-x86_64.zip",
			"linux/arm64":   "aptos-cli-9.5.0-Linux-aarch64.zip",
			"darwin/amd64":  "aptos-cli-9.5.0-macOS-x86_64.zip",
			"darwin/arm64":  "aptos-cli-9.5.0-macOS-arm64.zip",
			"windows/amd64": "aptos-cli-9.5.0-Windows-x86_64.zip",
		},
		bins: []string{"aptos"},
	},
	"besu": {
		ecosystems: []string{"ethereum"}, description: "Ethereum execution client written in Java, with the evmtool EVM debugger",
		sourceKind: recipe.TypeGitHubRelease, sample: "26.8.0",
		artifacts: map[string]string{
			"linux/amd64":  "besu-26.8.0.tar.gz",
			"linux/arm64":  "besu-26.8.0.tar.gz",
			"darwin/amd64": "besu-26.8.0.tar.gz",
			"darwin/arm64": "besu-26.8.0.tar.gz",
		},
		bins: []string{"bin/besu", "bin/evmtool"}, strip: 1,
	},
	"avalanche-cli": {
		ecosystems: []string{"avalanche"}, description: "Creates and runs Avalanche L1s and local test networks",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.9.6",
		artifacts: map[string]string{
			"linux/amd64":  "avalanche-cli_1.9.6_linux_amd64.tar.gz",
			"linux/arm64":  "avalanche-cli_1.9.6_linux_arm64.tar.gz",
			"darwin/amd64": "avalanche-cli_1.9.6_darwin_amd64.tar.gz",
			"darwin/arm64": "avalanche-cli_1.9.6_darwin_arm64.tar.gz",
		},
		bins: []string{"avalanche"},
	},
	"avalanchego": {
		ecosystems: []string{"avalanche"}, description: "AvalancheGo, the node implementation of the Avalanche network",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.14.2",
		artifacts: map[string]string{
			"linux/amd64": "avalanchego-linux-amd64-v1.14.2.tar.gz",
			"linux/arm64": "avalanchego-linux-arm64-v1.14.2.tar.gz",
		},
		bins: []string{"avalanchego"}, strip: 1,
	},
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
	"btcd": {
		ecosystems: []string{"bitcoin"}, description: "Alternative full-node Bitcoin implementation written in Go",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.26.2",
		artifacts: map[string]string{
			"linux/amd64":  "btcd-linux-amd64-v0.26.2.tar.gz",
			"linux/arm64":  "btcd-linux-arm64-v0.26.2.tar.gz",
			"darwin/amd64": "btcd-darwin-amd64-v0.26.2.tar.gz",
			"darwin/arm64": "btcd-darwin-arm64-v0.26.2.tar.gz",
		},
		bins: []string{"btcd", "btcctl"}, strip: 1,
	},
	"cardano-node": {
		ecosystems: []string{"cardano"}, description: "Cardano block-producing node and the cardano-cli that drives it",
		sourceKind: recipe.TypeGitHubRelease, sample: "11.0.1",
		artifacts: map[string]string{
			"linux/amd64":  "cardano-node-11.0.1-linux-amd64.tar.gz",
			"linux/arm64":  "cardano-node-11.0.1-linux-arm64.tar.gz",
			"darwin/amd64": "cardano-node-11.0.1-macos-amd64.tar.gz",
			"darwin/arm64": "cardano-node-11.0.1-macos-arm64.tar.gz",
		},
		bins: []string{"bin/cardano-node", "bin/cardano-cli", "bin/cardano-submit-api"},
	},
	"celestia-app": {
		ecosystems: []string{"celestia", "cosmos"}, description: "Celestia consensus node (celestia-appd)",
		sourceKind: recipe.TypeGitHubRelease, sample: "9.0.6",
		artifacts: map[string]string{
			"linux/amd64":  "celestia-app_Linux_x86_64.tar.gz",
			"linux/arm64":  "celestia-app_Linux_arm64.tar.gz",
			"darwin/amd64": "celestia-app_Darwin_x86_64.tar.gz",
			"darwin/arm64": "celestia-app_Darwin_arm64.tar.gz",
		},
		bins: []string{"celestia-appd"},
	},
	"celestia-node": {
		ecosystems: []string{"celestia", "cosmos"}, description: "Celestia data-availability node: bridge, full and light nodes",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.31.4",
		artifacts: map[string]string{
			"linux/amd64":  "celestia-node_Linux_x86_64.tar.gz",
			"linux/arm64":  "celestia-node_Linux_arm64.tar.gz",
			"darwin/amd64": "celestia-node_Darwin_x86_64.tar.gz",
			"darwin/arm64": "celestia-node_Darwin_arm64.tar.gz",
		},
		bins: []string{"celestia"},
	},
	"circom": {
		ecosystems: []string{"zk"}, description: "Compiler for the circom zero-knowledge circuit language",
		sourceKind: recipe.TypeGitHubRelease, sample: "2.2.3",
		// The macOS asset is named for Intel and is an arm64 binary, so the
		// recipe maps darwin/arm64 onto it and claims no darwin/amd64.
		artifacts: map[string]string{
			"linux/amd64":   "circom-linux-amd64",
			"darwin/arm64":  "circom-macos-amd64",
			"windows/amd64": "circom-windows-amd64.exe",
		},
		bins: []string{"circom"},
	},
	"cometbft": {
		ecosystems: []string{"cosmos"}, description: "Byzantine fault-tolerant consensus engine and node behind Cosmos SDK chains",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.0.1",
		artifacts: map[string]string{
			"linux/amd64":   "cometbft_1.0.1_linux_amd64.tar.gz",
			"linux/arm64":   "cometbft_1.0.1_linux_arm64.tar.gz",
			"darwin/amd64":  "cometbft_1.0.1_darwin_amd64.tar.gz",
			"darwin/arm64":  "cometbft_1.0.1_darwin_arm64.tar.gz",
			"windows/amd64": "cometbft_1.0.1_windows_amd64.tar.gz",
			"windows/arm64": "cometbft_1.0.1_windows_arm64.tar.gz",
		},
		bins: []string{"cometbft"},
	},
	"cosmos-relayer": {
		ecosystems: []string{"cosmos", "ibc"}, description: "IBC relayer for Cosmos SDK chains written in Go, run as rly",
		sourceKind: recipe.TypeGitHubRelease, sample: "2.6.0",
		artifacts: map[string]string{
			"linux/amd64":  "Cosmos.Relayer_2.6.0_linux_amd64.tar.gz",
			"linux/arm64":  "Cosmos.Relayer_2.6.0_linux_arm64.tar.gz",
			"darwin/amd64": "Cosmos.Relayer_2.6.0_darwin_amd64.tar.gz",
			"darwin/arm64": "Cosmos.Relayer_2.6.0_darwin_arm64.tar.gz",
		},
		bins: []string{"rly"}, strip: 1,
	},
	"cosmovisor": {
		ecosystems: []string{"cosmos"}, description: "Supervises a Cosmos SDK node binary across scheduled chain upgrades",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.7.1",
		artifacts: map[string]string{
			"linux/amd64":  "cosmovisor-v1.7.1-linux-amd64.tar.gz",
			"linux/arm64":  "cosmovisor-v1.7.1-linux-arm64.tar.gz",
			"darwin/amd64": "cosmovisor-v1.7.1-darwin-amd64.tar.gz",
		},
		bins: []string{"cosmovisor"},
	},
	"dfx": {
		ecosystems: []string{"icp"}, description: "Internet Computer SDK CLI for building and deploying canisters",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.32.0",
		artifacts: map[string]string{
			"linux/amd64":  "dfx-0.32.0-x86_64-linux.tar.gz",
			"linux/arm64":  "dfx-0.32.0-aarch64-linux.tar.gz",
			"darwin/amd64": "dfx-0.32.0-x86_64-darwin.tar.gz",
			"darwin/arm64": "dfx-0.32.0-aarch64-darwin.tar.gz",
		},
		bins: []string{"dfx"},
	},
	"echidna": {
		ecosystems: []string{"ethereum"}, description: "Property-based fuzzer for EVM smart contracts",
		sourceKind: recipe.TypeGitHubRelease, sample: "2.3.3",
		artifacts: map[string]string{
			"linux/amd64":  "echidna-2.3.3-x86_64-linux.tar.gz",
			"linux/arm64":  "echidna-2.3.3-aarch64-linux.tar.gz",
			"darwin/amd64": "echidna-2.3.3-x86_64-macos.tar.gz",
			"darwin/arm64": "echidna-2.3.3-aarch64-macos.tar.gz",
		},
		bins: []string{"echidna"},
	},
	"erigon": {
		ecosystems: []string{"ethereum"}, description: "Efficiency-focused Ethereum execution client written in Go",
		sourceKind: recipe.TypeGitHubRelease, sample: "3.5.5",
		artifacts: map[string]string{
			"linux/amd64": "erigon_v3.5.5_linux_amd64.tar.gz",
			"linux/arm64": "erigon_v3.5.5_linux_arm64.tar.gz",
		},
		bins: []string{"erigon"}, strip: 1,
	},
	"ethdo": {
		ecosystems: []string{"ethereum"}, description: "Command-line client for Ethereum consensus-layer accounts and validators",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.39.1",
		// Same as circom: the darwin-amd64 tarball holds an arm64 binary.
		artifacts: map[string]string{
			"linux/amd64":  "ethdo-1.39.1-linux-amd64.tar.gz",
			"linux/arm64":  "ethdo-1.39.1-linux-arm64.tar.gz",
			"darwin/arm64": "ethdo-1.39.1-darwin-amd64.tar.gz",
		},
		bins: []string{"ethdo"},
	},
	"fabric": {
		ecosystems: []string{"fabric"}, description: "Hyperledger Fabric peer, orderer and channel tooling",
		sourceKind: recipe.TypeGitHubRelease, sample: "3.1.5",
		artifacts: map[string]string{
			"linux/amd64":   "hyperledger-fabric-linux-amd64-3.1.5.tar.gz",
			"linux/arm64":   "hyperledger-fabric-linux-arm64-3.1.5.tar.gz",
			"darwin/amd64":  "hyperledger-fabric-darwin-amd64-3.1.5.tar.gz",
			"darwin/arm64":  "hyperledger-fabric-darwin-arm64-3.1.5.tar.gz",
			"windows/amd64": "hyperledger-fabric-windows-amd64-3.1.5.tar.gz",
		},
		bins: []string{"bin/peer", "bin/orderer", "bin/configtxgen", "bin/configtxlator", "bin/cryptogen", "bin/discover", "bin/osnadmin"},
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
	"gaia": {
		ecosystems: []string{"cosmos"}, description: "Cosmos Hub node (gaiad)",
		sourceKind: recipe.TypeGitHubRelease, sample: "27.6.0",
		artifacts: map[string]string{
			"linux/amd64":  "gaiad-v27.6.0-linux-amd64",
			"darwin/amd64": "gaiad-v27.6.0-darwin-amd64",
		},
		bins: []string{"gaiad"},
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
	"geth-tools": {
		ecosystems: []string{"ethereum"}, description: "go-ethereum developer tools: abigen, evm and rlpdump",
		sourceKind: recipe.TypeHTTP, sample: "1.17.5", commit: "9621c6ad10934a01",
		artifacts: map[string]string{
			"linux/amd64": "https://gethstore.blob.core.windows.net/builds/geth-alltools-linux-amd64-1.17.5-9621c6ad.tar.gz",
			"linux/arm64": "https://gethstore.blob.core.windows.net/builds/geth-alltools-linux-arm64-1.17.5-9621c6ad.tar.gz",
		},
		bins: []string{"abigen", "evm", "rlpdump"}, strip: 1,
	},
	"grandine": {
		ecosystems: []string{"ethereum"}, description: "Ethereum consensus (beacon chain) client written in Rust",
		sourceKind: recipe.TypeGitHubRelease, sample: "2.0.6",
		artifacts: map[string]string{
			"linux/amd64":   "grandine-2.0.6-linux-x64",
			"linux/arm64":   "grandine-2.0.6-linux-arm64",
			"darwin/amd64":  "grandine-2.0.6-osx-x64",
			"darwin/arm64":  "grandine-2.0.6-osx-arm64",
			"windows/amd64": "grandine-2.0.6-win-x64.exe",
		},
		bins: []string{"grandine"},
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
	"hevm": {
		ecosystems: []string{"ethereum"}, description: "EVM implementation for symbolic execution and equivalence checking",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.58.0",
		artifacts: map[string]string{
			"linux/amd64":  "hevm-x86_64-linux",
			"darwin/amd64": "hevm-x86_64-macos",
			"darwin/arm64": "hevm-arm64-macos",
		},
		bins: []string{"hevm"},
	},
	"ignite": {
		ecosystems: []string{"cosmos"}, description: "Scaffolds, builds and serves Cosmos SDK blockchains",
		sourceKind: recipe.TypeGitHubRelease, sample: "29.10.1",
		artifacts: map[string]string{
			"linux/amd64":  "ignite_29.10.1_linux_amd64.tar.gz",
			"linux/arm64":  "ignite_29.10.1_linux_arm64.tar.gz",
			"darwin/amd64": "ignite_29.10.1_darwin_amd64.tar.gz",
			"darwin/arm64": "ignite_29.10.1_darwin_arm64.tar.gz",
		},
		bins: []string{"ignite"},
	},
	"kubo": {
		ecosystems: []string{"ipfs"}, description: "Reference IPFS implementation, the ipfs node and CLI",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.43.0",
		artifacts: map[string]string{
			"linux/amd64":  "kubo_v0.43.0_linux-amd64.tar.gz",
			"linux/arm64":  "kubo_v0.43.0_linux-arm64.tar.gz",
			"darwin/amd64": "kubo_v0.43.0_darwin-amd64.tar.gz",
			"darwin/arm64": "kubo_v0.43.0_darwin-arm64.tar.gz",
		},
		bins: []string{"ipfs"}, strip: 1,
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
	"medusa": {
		ecosystems: []string{"ethereum"}, description: "Parallelised coverage-guided fuzzer for EVM smart contracts",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.5.1",
		artifacts: map[string]string{
			"linux/amd64":   "medusa-linux-x64.tar.gz",
			"darwin/arm64":  "medusa-mac-arm64.tar.gz",
			"windows/amd64": "medusa-win-x64.tar.gz",
		},
		bins: []string{"medusa"},
	},
	"near-cli": {
		ecosystems: []string{"near"}, description: "NEAR command-line interface for accounts, contracts and transactions",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.30.0",
		artifacts: map[string]string{
			"linux/amd64":   "near-cli-rs-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":   "near-cli-rs-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/amd64":  "near-cli-rs-x86_64-apple-darwin.tar.gz",
			"darwin/arm64":  "near-cli-rs-aarch64-apple-darwin.tar.gz",
			"windows/amd64": "near-cli-rs-x86_64-pc-windows-msvc.tar.gz",
		},
		bins: []string{"near"}, strip: 1,
	},
	"nimbus-eth2": {
		ecosystems: []string{"ethereum"}, description: "Nimbus Ethereum consensus client, built for low-resource machines",
		sourceKind: recipe.TypeGitHubRelease, sample: "26.7.0", commit: "4110bc7828a45518",
		artifacts: map[string]string{
			"linux/amd64":   "nimbus-eth2_Linux_amd64_26.7.0_4110bc78.tar.gz",
			"linux/arm64":   "nimbus-eth2_Linux_arm64v8_26.7.0_4110bc78.tar.gz",
			"darwin/arm64":  "nimbus-eth2_macOS_arm64_26.7.0_4110bc78.tar.gz",
			"windows/amd64": "nimbus-eth2_Windows_amd64_26.7.0_4110bc78.tar.gz",
		},
		bins: []string{"build/nimbus_beacon_node", "build/nimbus_validator_client"}, strip: 1,
	},
	"ord": {
		ecosystems: []string{"bitcoin"}, description: "Ordinal theory index, block explorer and wallet for Bitcoin",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.29.0",
		artifacts: map[string]string{
			"linux/amd64":  "ord-0.29.0-x86_64-unknown-linux-gnu.tar.gz",
			"darwin/amd64": "ord-0.29.0-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "ord-0.29.0-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"ord"}, strip: 1,
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
	"prysm": {
		ecosystems: []string{"ethereum"}, description: "Prysm beacon node, the Go Ethereum consensus-layer client",
		sourceKind: recipe.TypeGitHubRelease, sample: "7.1.8",
		artifacts: map[string]string{
			"linux/amd64":   "beacon-chain-v7.1.8-linux-amd64",
			"linux/arm64":   "beacon-chain-v7.1.8-linux-arm64",
			"darwin/amd64":  "beacon-chain-v7.1.8-darwin-amd64",
			"darwin/arm64":  "beacon-chain-v7.1.8-darwin-arm64",
			"windows/amd64": "beacon-chain-v7.1.8-windows-amd64.exe",
		},
		bins: []string{"beacon-chain"},
	},
	"prysm-validator": {
		ecosystems: []string{"ethereum"}, description: "Prysm validator client, run beside a beacon node to propose and attest",
		sourceKind: recipe.TypeGitHubRelease, sample: "7.1.8",
		artifacts: map[string]string{
			"linux/amd64":   "validator-v7.1.8-linux-amd64",
			"linux/arm64":   "validator-v7.1.8-linux-arm64",
			"darwin/amd64":  "validator-v7.1.8-darwin-amd64",
			"darwin/arm64":  "validator-v7.1.8-darwin-arm64",
			"windows/amd64": "validator-v7.1.8-windows-amd64.exe",
		},
		bins: []string{"validator"},
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
	"scarb": {
		ecosystems: []string{"starknet"}, description: "Cairo package manager and build tool for Starknet projects",
		sourceKind: recipe.TypeGitHubRelease, sample: "2.20.0",
		artifacts: map[string]string{
			"linux/amd64":  "scarb-v2.20.0-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "scarb-v2.20.0-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/amd64": "scarb-v2.20.0-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "scarb-v2.20.0-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"bin/scarb"}, strip: 1,
	},
	"solana-verify": {
		ecosystems: []string{"solana"}, description: "Verifies that an on-chain Solana program matches its source",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.5.1",
		artifacts: map[string]string{
			"linux/amd64":  "solana-verify-0.5.1-linux",
			"darwin/arm64": "solana-verify-0.5.1-macos",
		},
		bins: []string{"solana-verify"},
	},
	"solc": {
		ecosystems: []string{"ethereum"}, description: "The Solidity smart-contract compiler",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.8.36",
		artifacts: map[string]string{
			"linux/amd64":   "solc-static-linux",
			"darwin/amd64":  "solc-macos",
			"darwin/arm64":  "solc-macos",
			"windows/amd64": "solc-windows.exe",
		},
		bins: []string{"solc"},
	},
	"sp1": {
		ecosystems: []string{"ethereum", "zk"}, description: "SP1 zkVM toolchain: build, prove and verify RISC-V programs as cargo prove",
		sourceKind: recipe.TypeGitHubRelease, sample: "6.4.0",
		artifacts: map[string]string{
			"linux/amd64":  "cargo_prove_v6.4.0_linux_amd64.tar.gz",
			"linux/arm64":  "cargo_prove_v6.4.0_linux_arm64.tar.gz",
			"darwin/amd64": "cargo_prove_v6.4.0_darwin_amd64.tar.gz",
			"darwin/arm64": "cargo_prove_v6.4.0_darwin_arm64.tar.gz",
		},
		bins: []string{"cargo-prove"},
	},
	"starkli": {
		ecosystems: []string{"starknet"}, description: "Starknet command-line interface for accounts, contracts and calls",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.4.2",
		artifacts: map[string]string{
			"linux/amd64":  "starkli-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "starkli-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/amd64": "starkli-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "starkli-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"starkli"},
	},
	"starknet-foundry": {
		ecosystems: []string{"starknet"}, description: "Testing and deployment toolkit for Cairo contracts: snforge and sncast",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.63.0",
		artifacts: map[string]string{
			"linux/amd64":  "starknet-foundry-v0.63.0-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":  "starknet-foundry-v0.63.0-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/amd64": "starknet-foundry-v0.63.0-x86_64-apple-darwin.tar.gz",
			"darwin/arm64": "starknet-foundry-v0.63.0-aarch64-apple-darwin.tar.gz",
		},
		bins: []string{"bin/snforge", "bin/sncast"}, strip: 1,
	},
	"stellar": {
		ecosystems: []string{"stellar"}, description: "Stellar CLI for building, testing and deploying Soroban contracts",
		sourceKind: recipe.TypeGitHubRelease, sample: "27.1.0",
		artifacts: map[string]string{
			"linux/amd64":   "stellar-cli-27.1.0-x86_64-unknown-linux-gnu.tar.gz",
			"linux/arm64":   "stellar-cli-27.1.0-aarch64-unknown-linux-gnu.tar.gz",
			"darwin/amd64":  "stellar-cli-27.1.0-x86_64-apple-darwin.tar.gz",
			"darwin/arm64":  "stellar-cli-27.1.0-aarch64-apple-darwin.tar.gz",
			"windows/amd64": "stellar-cli-27.1.0-x86_64-pc-windows-msvc.tar.gz",
		},
		bins: []string{"stellar"},
	},
	"surfpool": {
		ecosystems: []string{"solana"}, description: "Local Solana network that streams mainnet state for pre-deployment testing",
		sourceKind: recipe.TypeGitHubRelease, sample: "1.5.0",
		artifacts: map[string]string{
			"linux/amd64":   "surfpool-linux-x64.tar.gz",
			"darwin/amd64":  "surfpool-darwin-x64.tar.gz",
			"darwin/arm64":  "surfpool-darwin-arm64.tar.gz",
			"windows/amd64": "surfpool-windows-x64.tar.gz",
		},
		bins: []string{"surfpool"},
	},
	"vyper": {
		ecosystems: []string{"ethereum"}, description: "The Vyper smart-contract compiler, a Pythonic language for the EVM",
		sourceKind: recipe.TypeGitHubRelease, sample: "0.4.3", commit: "bff19ea204059290",
		artifacts: map[string]string{
			"linux/amd64":   "vyper.0.4.3+commit.bff19ea2.linux",
			"darwin/arm64":  "vyper.0.4.3+commit.bff19ea2.darwin",
			"windows/amd64": "vyper.0.4.3+commit.bff19ea2.windows.exe",
		},
		bins: []string{"vyper"},
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
	// Ecosystem names are recipe data, so the set is whatever the table
	// above pins per recipe — asserting a literal list here would only
	// restate it, and would have to be edited for every tool added.
	want := map[string]bool{}
	for _, w := range recipes {
		for _, e := range w.ecosystems {
			want[e] = true
		}
	}
	got := r.Ecosystems()
	if len(got) != len(want) || !slices.IsSorted(got) {
		t.Errorf("Ecosystems() = %v, want the %d names the recipes claim, sorted", got, len(want))
	}
	for _, e := range got {
		if !want[e] {
			t.Errorf("Ecosystems() lists %q, which no recipe claims", e)
		}
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
	if got := names(r.ByEcosystem("cosmos")); got != "celestia-app,celestia-node,cometbft,cosmos-relayer,cosmovisor,gaia,hermes,ignite,osmosis" {
		t.Errorf("ByEcosystem(cosmos) = %q (must be sorted by name)", got)
	}
	if got := r.ByEcosystem("polkadot"); got != nil {
		t.Errorf("ByEcosystem(unknown) = %v", got)
	}
	if !slices.IsSorted(ecosystemNames(r)) {
		t.Errorf("Recipes() = %v (must be sorted by name)", ecosystemNames(r))
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
