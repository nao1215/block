---
title: Getting started
description: "Install block, declare a toolchain in block.toml, lock it, and run it."
---

## Install

```shell
go install github.com/nao1215/block@latest
```

Or download an archive from the
[releases page](https://github.com/nao1215/block/releases), or use Homebrew:

```shell
brew install nao1215/tap/block
```

In GitHub Actions, use
[setup-block](https://nao1215.github.io/setup-block/).

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
1.7.y, `"1.7.4"` exactly that release. There are no operators or ranges, and
pre-releases are never selected.

Do not know what to write? Ask:

```console
$ block list ethereum
NAME         COMMANDS                     DESCRIPTION
foundry      forge, cast, anvil, chisel   Fast Ethereum application toolkit: build, test, deploy and inspect contracts
geth         geth                         go-ethereum, the Go implementation of an Ethereum execution client
lighthouse   lighthouse                   Ethereum consensus (beacon chain) client written in Rust
reth         reth                         Modular Ethereum execution client written in Rust
solc         solc                         The Solidity smart-contract compiler
```

## Lock, sync, run

```console
$ block lock
foundry  locked 1.7.4
hermes   locked 1.13.3
wrote block.lock

$ block sync
foundry  1.7.4   installed
hermes   1.13.3  installed

$ block exec forge test
```

Commit both `block.toml` and `block.lock`. Everyone else — and CI — runs
`block sync` and gets the same tools, byte for byte.

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
