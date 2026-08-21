---
title: block
---

A blockchain project's build depends on binaries nobody declared: whichever
Foundry the last person installed, whichever `solc` was on that runner,
whichever relayer the integration test found. block makes that set explicit and
reproducible — declared in `block.toml`, pinned by checksum in `block.lock`,
and identical on every machine and every runner.

![block: two lines of block.toml, then lock, sync and a real tool running](/img/demo.gif)

## Try it in 30 seconds

If you have Go, paste this into an empty directory:

```shell
printf '[tools]\nfoundry = "1.7"\n' > block.toml
go run github.com/nao1215/block@latest lock   # resolve: block.toml -> block.lock
go run github.com/nao1215/block@latest sync   # install what block.lock pins
go run github.com/nao1215/block@latest exec forge --version
```

```text
foundry  locked 1.7.1
wrote block.lock
foundry  1.7.1  installed
shims: anvil, cast, chisel, forge
forge Version: 1.7.1-v1.7.1
```

`sync` is a step, not a formality: `exec` never installs anything, so it runs
what `sync` put on disk or it refuses.

Foundry publishes Linux and macOS builds only, and block says so rather than
substituting something else. On Windows, the same 30 seconds with a tool that
does ship there:

```shell
printf '[tools]\nsolc = "0.8"\n' > block.toml
go run github.com/nao1215/block@latest lock
go run github.com/nao1215/block@latest sync
go run github.com/nao1215/block@latest exec solc --version
```

What each CLI ships for is in [Tools](/tools/).

Two files now say what your toolchain is. Commit both, and everyone else runs
`block sync`.

## Three lifecycle commands, one direction

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

47 tools across 17 blockchain systems, from Bitcoin Core and Foundry to Agave,
Gaia, Hermes and Starknet Foundry. Ask the binary — offline, with no
`block.toml` and no token:

![block list cosmos, and a misspelled ecosystem answering with the ones that exist](/img/list.gif)

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

## One machine, one toolchain per project

`block sync` writes one file per command into `$BLOCK_HOME/shims`. Put that
directory on `PATH` once, by hand, and the version follows the directory you
are standing in — no shell hook, no `eval`, no activation:

![two repositories pinning different Foundry versions, each running its own](/img/shims.gif)

Outside a block project, or for a command the current project does not lock,
the shim steps aside and runs the next command of that name on `PATH`.

## Next

The [cookbook](/cookbook/) has the rest: locking for a platform you are not on,
caching the toolchain in CI, bringing your own tool, and reading a refusal.
[Reference](/reference/) is the file formats and the machinery underneath them.

To start from something rather than a blank file,
[examples/](https://github.com/nao1215/block/tree/main/examples) holds a
`block.toml` for each of eight kinds of repository — EVM contracts, an Ethereum
node pair, a Cosmos appchain, a Solana program, Bitcoin, Starknet, a
multi-chain tree, and one that brings its own tool.

## Install

```shell
go install github.com/nao1215/block@latest
```

Homebrew, Scoop, prebuilt packages and release archives are on the
[install page](/install/).
