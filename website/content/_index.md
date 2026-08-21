---
title: block
---

A blockchain project's build depends on binaries nobody declared: whichever
Foundry the last person installed, whichever `solc` was on that runner,
whichever relayer the integration test found. block makes that set explicit and
reproducible — declared in `block.toml`, pinned by checksum in `block.lock`,
and identical on every machine and every runner.

## Try it in 30 seconds

If you have Go, paste this into an empty directory:

```shell
printf '[tools]\nfoundry = "1.7"\n' > block.toml
go run github.com/nao1215/block@latest lock
go run github.com/nao1215/block@latest exec forge --version
```

```text
foundry  locked 1.7.1
wrote block.lock
forge Version: 1.7.1-v1.7.1
```

Two files now say what your toolchain is. Commit both, and everyone else runs
`block sync`.

## Three commands, one direction

```text
block.toml  ──block lock──▶  block.lock  ──block sync──▶  installed toolchain  ──block exec──▶  command
```

```text
block lock   resolves the toolchain.
block sync   installs the locked toolchain.
block exec   runs with the installed toolchain.
```

`sync` never resolves. `exec` never installs. `lock` is the only operation that
can move a pin — so a build cannot quietly pick up a release that happened
overnight, and a stale lockfile is an error rather than a guess.

## What can it install?

45 CLIs across 17 blockchain systems, from Bitcoin Core and Foundry to Agave,
Gaia, Hermes and Starknet Foundry. Ask the binary, offline:

```shell
block list solana
```

```text
NAME            COMMANDS                                                          DESCRIPTION
agave           solana, solana-keygen, solana-test-validator, agave-ledger-tool   Solana validator client and CLI suite, including a local test validator
anchor          anchor                                                            Framework and CLI for writing, testing and deploying Solana programs
solana-verify   solana-verify                                                     Verifies that an on-chain Solana program matches its source
surfpool        surfpool                                                          Local Solana network that streams mainnet state for pre-deployment testing
```

The whole catalogue, with commands and platform coverage: [Tools](/tools/). A
tool that is not there — or a fork of one that is — is four lines in your own
`block.toml`; nobody waits for a registry merge.

## Why block?

Pick the tool that fits the job:

| You want | Use |
|:--|:--|
| A pinned language runtime and a general-purpose tool manager | [mise](https://mise.jdx.dev/), [aqua](https://aquaproj.github.io/) |
| A whole operating system reproduced, or isolation from untrusted code | Docker, [Nix](https://nixos.org/) |
| The blockchain CLIs one repository builds and tests with, pinned by checksum and shared across projects | block |

block's emphasis is narrow on purpose: blockchain CLIs, project-local, verified
on download, with no scripts in the recipes and no way for adding a tool to add
a way to run something. It manages no language runtimes, and it is not trying
to become a package manager. See [Compared to Docker](/comparison/) for the
overlap, and where a container is still the better answer.

## Next

```shell
block sync                       # install exactly what block.lock names
block exec forge test            # run with the locked toolchain on PATH
block lock --check               # has upstream published anything newer?
```

The [cookbook](/cookbook/) has the rest: locking for a platform you are not on,
running tools by their own names, caching the toolchain in CI, bringing your own
tool, and reading a refusal. To start from something rather than a blank file,
[examples/](https://github.com/nao1215/block/tree/main/examples) holds a
`block.toml` for each of eight kinds of repository.

## Install

```shell
go install github.com/nao1215/block@latest
```

Homebrew, Scoop, prebuilt packages and release archives are on the
[install page](/install/).
