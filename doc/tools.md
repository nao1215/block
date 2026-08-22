# Tools

Every CLI block can install today: 48 tools across 17 blockchain systems, from
the registry snapshot embedded in the binary. The same list, offline, from the
block you have installed:

```shell
block list                 # every tool, with the systems it serves
block list ethereum        # one system, with the commands each tool provides
```

A tool that serves more than one system is listed under each — an IBC relayer
belongs to both Cosmos and IBC work. Commands are what lands on your PATH after
`block sync`. Platforms are the upstream's decision, not block's: where a
project publishes no build, block says so rather than substituting something
else.

Listing is discovery, not selection. Nothing here is installed until you name
it in `block.toml`, which stays the one place that says what your
toolchain is.

## Find a system

| System | Tools |
|:--|:--|
| [Aptos](#aptos) | 1 |
| [Avalanche](#avalanche) | 2 |
| [Bitcoin](#bitcoin) | 3 |
| [Cardano](#cardano) | 1 |
| [Celestia](#celestia) | 2 |
| [Cosmos](#cosmos) | 9 |
| [Ethereum](#ethereum) | 19 |
| [Hyperledger Fabric](#hyperledger-fabric) | 1 |
| [IBC](#ibc) | 2 |
| [Internet Computer](#internet-computer) | 1 |
| [IPFS](#ipfs) | 1 |
| [NEAR](#near) | 1 |
| [Solana](#solana) | 4 |
| [Starknet](#starknet) | 3 |
| [Stellar](#stellar) | 1 |
| [Zero-knowledge circuits](#zero-knowledge-circuits) | 2 |
| [ZKsync](#zksync) | 1 |

## Aptos

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `aptos` | `aptos` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64) | Aptos CLI: compile, test and publish Move packages and run a local network |

## Avalanche

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `avalanche-cli` | `avalanche` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Creates and runs Avalanche L1s and local test networks |
| `avalanchego` | `avalanchego` | Linux (x86-64, arm64) | AvalancheGo, the node implementation of the Avalanche network |

## Bitcoin

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `bitcoin-core` | `bitcoind`, `bitcoin-cli`, `bitcoin-tx`, `bitcoin-util`, `bitcoin-wallet` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Bitcoin reference implementation: full node, wallet and transaction tools |
| `btcd` | `btcd`, `btcctl` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Alternative full-node Bitcoin implementation written in Go |
| `ord` | `ord` | Linux (x86-64), macOS (x86-64, arm64) | Ordinal theory index, block explorer and wallet for Bitcoin |

## Cardano

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `cardano-node` | `cardano-node`, `cardano-cli`, `cardano-submit-api` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Cardano block-producing node and the cardano-cli that drives it |

## Celestia

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `celestia-app` | `celestia-appd` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Celestia consensus node (celestia-appd) |
| `celestia-node` | `celestia` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Celestia data-availability node: bridge, full and light nodes |

## Cosmos

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `celestia-app` | `celestia-appd` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Celestia consensus node (celestia-appd) |
| `celestia-node` | `celestia` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Celestia data-availability node: bridge, full and light nodes |
| `cometbft` | `cometbft` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64, arm64) | Byzantine fault-tolerant consensus engine and node behind Cosmos SDK chains |
| `cosmos-relayer` | `rly` | Linux (x86-64, arm64), macOS (x86-64, arm64) | IBC relayer for Cosmos SDK chains written in Go, run as rly |
| `cosmovisor` | `cosmovisor` | Linux (x86-64, arm64), macOS (x86-64) | Supervises a Cosmos SDK node binary across scheduled chain upgrades |
| `gaia` | `gaiad` | Linux (x86-64), macOS (x86-64) | Cosmos Hub node (gaiad) |
| `hermes` | `hermes` | Linux (x86-64, arm64), macOS (x86-64, arm64) | IBC relayer connecting Cosmos SDK chains, written in Rust |
| `ignite` | `ignite` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Scaffolds, builds and serves Cosmos SDK blockchains |
| `osmosis` | `osmosisd` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Osmosis appchain node (osmosisd), the Cosmos AMM |

## Ethereum

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `anvil-zksync` | `anvil-zksync` | Linux (x86-64, arm64), macOS (x86-64, arm64) | In-memory ZKsync node for local development and testing |
| `besu` | `besu`, `evmtool` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Ethereum execution client written in Java, with the evmtool EVM debugger |
| `echidna` | `echidna` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Property-based fuzzer for EVM smart contracts |
| `erigon` | `erigon` | Linux (x86-64, arm64) | Efficiency-focused Ethereum execution client written in Go |
| `ethdo` | `ethdo` | Linux (x86-64, arm64), macOS (arm64) | Command-line client for Ethereum consensus-layer accounts and validators |
| `foundry` | `forge`, `cast`, `anvil`, `chisel` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Fast Ethereum application toolkit: build, test, deploy and inspect contracts |
| `geth` | `geth` | Linux (x86-64, arm64) | go-ethereum, the Go implementation of an Ethereum execution client |
| `geth-tools` | `abigen`, `evm`, `rlpdump` | Linux (x86-64, arm64) | go-ethereum developer tools: abigen, evm and rlpdump |
| `grandine` | `grandine` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64) | Ethereum consensus (beacon chain) client written in Rust |
| `hevm` | `hevm` | Linux (x86-64), macOS (x86-64, arm64) | EVM implementation for symbolic execution and equivalence checking |
| `lighthouse` | `lighthouse` | Linux (x86-64, arm64), macOS (arm64) | Ethereum consensus (beacon chain) client written in Rust |
| `medusa` | `medusa` | Linux (x86-64), macOS (arm64), Windows (x86-64) | Parallelised coverage-guided fuzzer for EVM smart contracts |
| `nimbus-eth2` | `nimbus_beacon_node`, `nimbus_validator_client` | Linux (x86-64, arm64), macOS (arm64), Windows (x86-64) | Nimbus Ethereum consensus client, built for low-resource machines |
| `prysm` | `beacon-chain` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64) | Prysm beacon node, the Go Ethereum consensus-layer client |
| `prysm-validator` | `validator` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64) | Prysm validator client, run beside a beacon node to propose and attest |
| `reth` | `reth` | Linux (x86-64, arm64), macOS (arm64) | Modular Ethereum execution client written in Rust |
| `solc` | `solc` | Linux (x86-64), macOS (x86-64, arm64), Windows (x86-64) | The Solidity smart-contract compiler |
| `sp1` | `cargo-prove` | Linux (x86-64, arm64), macOS (x86-64, arm64) | SP1 zkVM toolchain: build, prove and verify RISC-V programs as cargo prove |
| `vyper` | `vyper` | Linux (x86-64), macOS (arm64), Windows (x86-64) | The Vyper smart-contract compiler, a Pythonic language for the EVM |

## Hyperledger Fabric

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `fabric` | `peer`, `orderer`, `configtxgen`, `configtxlator`, `cryptogen`, `discover`, `osnadmin` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64) | Hyperledger Fabric peer, orderer and channel tooling |

## IBC

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `cosmos-relayer` | `rly` | Linux (x86-64, arm64), macOS (x86-64, arm64) | IBC relayer for Cosmos SDK chains written in Go, run as rly |
| `hermes` | `hermes` | Linux (x86-64, arm64), macOS (x86-64, arm64) | IBC relayer connecting Cosmos SDK chains, written in Rust |

## Internet Computer

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `dfx` | `dfx` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Internet Computer SDK CLI for building and deploying canisters |

## IPFS

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `kubo` | `ipfs` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Reference IPFS implementation, the ipfs node and CLI |

## NEAR

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `near-cli` | `near` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64) | NEAR command-line interface for accounts, contracts and transactions |

## Solana

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `agave` | `solana`, `solana-keygen`, `solana-test-validator`, `agave-ledger-tool` | Linux (x86-64), macOS (x86-64, arm64), Windows (x86-64) | Solana validator client and CLI suite, including a local test validator |
| `anchor` | `anchor` | Linux (x86-64), macOS (x86-64, arm64), Windows (x86-64) | Framework and CLI for writing, testing and deploying Solana programs |
| `solana-verify` | `solana-verify` | Linux (x86-64), macOS (arm64) | Verifies that an on-chain Solana program matches its source |
| `surfpool` | `surfpool` | Linux (x86-64), macOS (x86-64, arm64), Windows (x86-64) | Local Solana network that streams mainnet state for pre-deployment testing |

## Starknet

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `scarb` | `scarb` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Cairo package manager and build tool for Starknet projects |
| `starkli` | `starkli` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Starknet command-line interface for accounts, contracts and calls |
| `starknet-foundry` | `snforge`, `sncast` | Linux (x86-64, arm64), macOS (x86-64, arm64) | Testing and deployment toolkit for Cairo contracts: snforge and sncast |

## Stellar

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `stellar` | `stellar` | Linux (x86-64, arm64), macOS (x86-64, arm64), Windows (x86-64) | Stellar CLI for building, testing and deploying Soroban contracts |

## Zero-knowledge circuits

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `circom` | `circom` | Linux (x86-64), macOS (arm64), Windows (x86-64) | Compiler for the circom zero-knowledge circuit language |
| `sp1` | `cargo-prove` | Linux (x86-64, arm64), macOS (x86-64, arm64) | SP1 zkVM toolchain: build, prove and verify RISC-V programs as cargo prove |

## ZKsync

| Tool | Commands | Platforms | What it is |
|:--|:--|:--|:--|
| `anvil-zksync` | `anvil-zksync` | Linux (x86-64, arm64), macOS (x86-64, arm64) | In-memory ZKsync node for local development and testing |

## Not here yet?

A tool the registry does not carry — or one you want from a fork — is declared
in your own `block.toml`, with the same fields a registry recipe uses:

```toml
[tools.foo]
version = "1.2"

[tools.foo.source]
type = "github_release"
repo = "example/foo"
asset = "foo_{version}_{os}_{arch}.tar.gz"
bin = ["foo"]
```

See [Bring your own tool](/cookbook/#bring-your-own-tool) for the whole set of
knobs. Nobody waits for a registry merge; promoting a definition to
[block-registry](https://github.com/nao1215/block-registry) afterwards is a copy.

<!-- Generated by "make doc" from the recipes in registry/, vendored from
     block-registry at 09593c6e02543c11444cc54318601be63f03a3c5. Do not edit by hand. -->
