---
title: Getting started
description: "Declare a toolchain in block.toml, lock it, install it, and run your tools with it."
toc: true
---

This page takes a repository from nothing to a pinned toolchain. It assumes
block is [installed](/install/); the whole path is four steps and two committed
files.

## Declare a toolchain

Write `block.toml` next to your project:

```toml
# Optional. Platforms to resolve artifacts for; the machine you lock on by
# default. Declare both when the team is on macOS and CI runs Linux.
platforms = ["darwin/arm64", "linux/amd64"]

[tools]
foundry = "1.7"
hermes = "1.13"
```

A version is a dotted prefix: `"1"` is the newest 1.x.y, `"1.7"` the newest
1.7.y, `"1.7.1"` exactly that release. There are no operators or ranges, and
pre-releases are never selected. How tightly to pin, and why, is in the
[cookbook](/cookbook/#choose-how-tightly-to-pin).

Do not know what to write? Ask:

```console
$ block list ethereum
NAME              COMMANDS                                      DESCRIPTION
anvil-zksync      anvil-zksync                                  In-memory ZKsync node for local development and testing
echidna           echidna                                       Property-based fuzzer for EVM smart contracts
erigon            erigon                                        Efficiency-focused Ethereum execution client written in Go
ethdo             ethdo                                         Command-line client for Ethereum consensus-layer accounts and validators
foundry           forge, cast, anvil, chisel                    Fast Ethereum application toolkit: build, test, deploy and inspect contracts
geth              geth                                          go-ethereum, the Go implementation of an Ethereum execution client
geth-tools        abigen, evm, rlpdump                          go-ethereum developer tools: abigen, evm and rlpdump
hevm              hevm                                          EVM implementation for symbolic execution and equivalence checking
lighthouse        lighthouse                                    Ethereum consensus (beacon chain) client written in Rust
medusa            medusa                                        Parallelised coverage-guided fuzzer for EVM smart contracts
nimbus-eth2       nimbus_beacon_node, nimbus_validator_client   Nimbus Ethereum consensus client, built for low-resource machines
prysm             beacon-chain                                  Prysm beacon node, the Go Ethereum consensus-layer client
prysm-validator   validator                                     Prysm validator client, run beside a beacon node to propose and attest
reth              reth                                          Modular Ethereum execution client written in Rust
solc              solc                                          The Solidity smart-contract compiler
vyper             vyper                                         The Vyper smart-contract compiler, a Pythonic language for the EVM
```

## Lock, sync, run

```console
$ block lock
foundry  locked 1.7.1
hermes   locked 1.13.3
wrote block.lock

$ block sync
foundry  1.7.1   installed
hermes   1.13.3  installed
commands: anvil, cast, chisel, forge, hermes

$ block exec forge test
```

Commit both `block.toml` and `block.lock`. Everyone else — and CI — runs
`block sync` and gets the same tools, byte for byte.

## Skip the prefix

`block sync` puts one file per command in `$BLOCK_HOME/shims`. Add it to
`PATH` once and the tools are just tools, switching with the directory you are
in:

```shell
export PATH="$HOME/.local/share/block/shims:$PATH"   # Unix
```

```powershell
[Environment]::SetEnvironmentVariable(
  "Path", "$env:LOCALAPPDATA\block\shims;$env:Path", "User")   # Windows
```

```console
$ forge test     # the version this project locked
```

There is no shell hook and nothing is written to your startup files. See
[Commands](/commands/#shims-the-tools-by-their-own-names). `block which forge`
prints which executable that is, and `block completion` gives your shell
tab-completion for the rest — see
[Shell completion](/commands/#shell-completion).

## Tools the registry does not have yet

Define the source in your own project and use it today; nothing waits on a
registry pull request:

```toml
[tools.foo]
version = "1.2"

[tools.foo.source]
type = "github_release"
repo = "example/foo"
asset = "foo_{version}_{os}_{arch}.tar.gz"
bin = ["foo"]
```

The format is identical to a registry recipe, so promoting it later is a copy.
The full set of fields is in [Bring your own tool](/cookbook/#bring-your-own-tool).

## Where to go next

[examples/](https://github.com/nao1215/block/tree/main/examples) has a
ready-made manifest for eight kinds of repository — EVM contracts, an Ethereum
node pair, a Cosmos appchain, a Solana program, Bitcoin, Starknet, a
multi-chain tree, and one that brings its own tool. Each is checked on every
push and re-resolved against the real upstreams weekly.

The [cookbook](/cookbook/) is the practical reference: locking for a platform
you are not on, moving one pin forward, caching the toolchain in CI, running an
`anvil` devnet, and reading a refusal when block says no. [Commands](/commands/)
is the per-command reference, and [Tools](/tools/) is everything block can
install.
