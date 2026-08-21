---
title: block
---

`block` pins the blockchain CLI tools a repository depends on — Foundry, geth,
reth, Lighthouse, Bitcoin Core, Agave, Anchor, Gaia, CometBFT, Hermes — and
reproduces exactly the same toolchain on every developer machine and in CI.

```console
$ git clone <project> && cd <project>
$ block sync
foundry  1.7.4   installed
hermes   1.13.3  installed
$ block exec forge test
```

Three commands, one direction:

```text
block.toml  ──block lock──▶  block.lock  ──block sync──▶  installed toolchain  ──block exec──▶  command
```

```text
block lock   resolves the toolchain.
block sync   installs the locked toolchain.
block exec   runs with the installed toolchain.
```

> `sync` never resolves. `exec` never installs. `lock` is the only operation
> that can move a pin.

## Why

- **Project-local, not machine-global.** Each repository declares its own
  toolchain, so two projects on one machine can use different Foundry
  versions without fighting.
- **Lockfile-driven reproducibility.** `block.lock` records the artifact URL
  and checksum for every platform you care about. `sync` installs exactly
  that, or fails.
- **CI is a first-class user.** The same command means the same thing on a
  laptop and on a runner: no resolution, no lockfile rewrite, no special flag.
- **Upstream releases are detected, not catalogued.** The
  [registry](https://nao1215.github.io/block-registry/) holds a recipe per
  tool, not a list of versions, so a new release needs no registry change.
- **A single static binary.** No mise, aqua, Nix, Docker or package manager is
  involved.

## What it is not

block manages blockchain CLIs. It is not a general package manager, and it
does not manage the Go, Rust, Node or Python toolchains, your wallets, keys,
validators, node configuration or chain state.

[Get started](getting-started/) in about two minutes.
