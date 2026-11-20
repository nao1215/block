---
title: block
---

block is a reproducible toolchain manager for blockchain development. A
blockchain project's build often depends on binaries nobody declared, such as
whichever Foundry the last person installed or whichever `solc` was on that
runner. With block you declare them in `block.toml`, and `block.lock` pins them
by checksum so every machine and runner gets the same ones.

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
commands: anvil, cast, chisel, forge
forge Version: 1.7.1-v1.7.1
```

`exec` never installs anything. It runs what `sync` put on disk, or it
refuses.

Foundry publishes Linux and macOS builds only, and block reports that instead
of substituting something else. On Windows, use a tool that ships there:

```shell
printf '[tools]\nsolc = "0.8"\n' > block.toml
go run github.com/nao1215/block@latest lock
go run github.com/nao1215/block@latest sync
go run github.com/nao1215/block@latest exec solc --version
```

What each CLI ships for is in [Tools](/tools/). Commit `block.toml` and
`block.lock`, and everyone else runs `block sync`.

## Three lifecycle commands, one direction

```text
block.toml  ──block lock──▶  block.lock  ──block sync──▶  installed toolchain  ──block exec──▶  command
```

```text
block lock   resolves the toolchain.
block sync   installs the locked toolchain.
block exec   runs with the installed toolchain.
```

`sync` never resolves and `exec` never installs. `lock` is the only operation
that can move a pin, so a build cannot pick up a release that happened
overnight, and a stale lockfile is an error.

## What can it install?

51 tools across 17 blockchain systems, from Bitcoin Core and Foundry to Agave,
Gaia, Hermes and Starknet Foundry. The binary lists them offline, with no
`block.toml` and no token:

![block list cosmos, and a misspelled ecosystem answering with the ones that exist](/img/list.gif)

The whole catalogue, with commands and platform coverage, is in
[Tools](/tools/). A tool that is missing, or a fork of one that is listed, takes
four lines in your own `block.toml`, with no registry merge to wait for.

## Why block?

| You want | Use |
|:--|:--|
| A pinned language runtime and a general-purpose tool manager | [mise](https://mise.jdx.dev/), [aqua](https://aquaproj.github.io/) |
| A whole operating system reproduced, or isolation from untrusted code | Docker, [Nix](https://nixos.org/) |
| The blockchain CLIs one repository builds and tests with, pinned by checksum and shared across projects | block |

block only handles blockchain CLIs, per project, verified on download. Recipes
contain no scripts, so adding a tool cannot add a way to run something. It
manages no language runtimes and is not a package manager.
[Compared to Docker](/comparison/) covers the overlap and when a container is
the better choice.

## One machine, one toolchain per project

`block sync` writes one file per command into `$BLOCK_HOME/shims`. Put that
directory on `PATH` once, by hand, and the version follows the directory you
are in. There is no shell hook, `eval` or activation step:

![two repositories pinning different Foundry versions, each running its own](/img/shims.gif)

Outside a block project, or for a command the current project does not lock,
the shim steps aside and runs the next command of that name on `PATH`.

## Next

The [cookbook](/cookbook/) covers locking for a platform you are not on,
caching the toolchain in CI, bringing your own tool, and reading a refusal.
[Reference](/reference/) describes the file formats and how they work.

[examples/](https://github.com/nao1215/block/tree/main/examples) holds a
`block.toml` for eight kinds of repository: EVM contracts, an Ethereum node
pair, a Cosmos appchain, a Solana program, Bitcoin, Starknet, a multi-chain
tree, and one that brings its own tool.

## Install

```shell
go install github.com/nao1215/block@latest
```

Homebrew, prebuilt packages and release archives are on the
[install page](/install/).
