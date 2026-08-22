---
title: Commands
description: "What each block command does, and the boundaries none of them cross."
toc: true
---

| Command | Resolves versions | Writes `block.lock` | Downloads | Installs | Runs your command |
| --- | :-: | :-: | :-: | :-: | :-: |
| `block lock [tool...]` | yes | yes | artifacts with no upstream digest | no | no |
| `block lock --check` | yes | never | never | no | no |
| `block sync` | never | never | locked URLs, when not cached | yes | no |
| `block exec <cmd>` | never | never | never | never | yes |
| `block list [ecosystem]` | never | never | never | never | no |
| `block explain <code>` | never | never | never | never | no |

"never" is a guarantee, not a default: there is no flag that turns any of those
cells into a yes.

## `block lock`

Resolves every tool in `block.toml` to the newest upstream release its
constraint allows, and records the download URL and SHA-256 per platform.
Naming tools re-resolves only those and keeps the other pins:

```console
$ block lock              # every tool
$ block lock hermes       # only hermes
```

It is the only command that moves a pin.

## `block lock --check`

The same resolution, writing nothing and downloading nothing. It compares the
whole prospective lockfile, so a change that leaves the version alone — a
narrowed constraint, a renamed executable, a moved artifact — is reported too:

```console
$ block lock --check
foundry  1.7.1 -> 1.7.2
hermes   1.13.3 (up-to-date)
$ echo $?
2
```

| Exit | Meaning |
| --- | --- |
| 0 | `block.lock` is current |
| 2 | `block.lock` would change |
| 1 | error |

## `block sync`

Installs every artifact `block.lock` names for this machine. It needs the
lockfile's exact URLs and nothing else — no registry, no GitHub API — and
fails, without resolving or writing anything, when the lockfile is missing,
disagrees with `block.toml`, lacks this platform, or an artifact's checksum
does not match.

## `block exec`

Runs a command with every executable from `block.lock` first on `PATH`, and
exits with the command's status:

```console
$ block exec forge test
$ block exec make test
$ block exec ./scripts/integration-test.sh
```

It never downloads, installs or resolves — but it does check, offline, that
the toolchain is the one `block.toml` asks for and that the install is intact.
`SIGINT` and `SIGTERM` reach the child, so a node or a local test network
shuts down the way it would outside block.

## Shims: the tools by their own names

`block sync` also puts one small file per command in `$BLOCK_HOME/shims`.
Add that directory to `PATH` once — by hand; block never edits your startup
files — and the commands are just commands:

```console
$ cd defi && forge --version
forge Version: 1.5.1-v1.5.1

$ cd ../bridge && forge --version
forge Version: 1.7.1
```

Each shim is the block binary under another name. Run as `forge`, it finds the
`block.toml` above the working directory, reads that project's `block.lock`,
and runs the version pinned there. Nothing is stored in the shim, so there is
one `forge` for every project, nothing to regenerate when you switch branches,
and no shell hook or `eval` anywhere.

A shim does what `block exec` does and no more — it never resolves, downloads,
installs or writes:

```console
$ forge test
block: BLK4004: foundry 1.7.1 is not installed; run "block sync"
```

Outside a project, or for a command the current project does not lock, it runs
the next command of that name on `PATH` instead, so the directory being on
`PATH` cannot take a tool away from the rest of your system.

CI should keep using `block exec`: it needs no `PATH` setup and says what is
happening.

## `block list`

Answers *what can block install?*, and with an argument, *which tools exist
for this blockchain system?* It reads the registry snapshot embedded in the
binary: no network, no `block.toml`, no `block.lock`.

That snapshot is a vendored copy of
[block-registry](https://github.com/nao1215/block-registry) at one revision,
built into the binary rather than fetched, so a block version always pairs
with a registry it was tested against. `block version` prints which revision
it carries.

Listing is discovery, not selection. block never derives a toolchain from an
ecosystem — "Ethereum" means contract development to one repository and
validator operation to another. You pick, `block.toml` records.

## `block explain`

Every refusal block prints carries a code — `BLK1003`, `BLK3002` — and this is
how to read one without a browser:

```console
$ block explain BLK1003
BLK1003  block.lock not found

The project declares a toolchain but has never resolved it, so there is
nothing to install or run. block will not resolve one on the spot: which
version a build uses is a decision that gets committed, not one made by
whoever happened to run the command.

Fix
Run "block lock" to resolve block.toml into block.lock, and commit both
files.

Exits 1. Since v0.1.0.
https://nao1215.github.io/block/errors/#blk1003--blocklock-not-found
```

The prefix is optional and the case does not matter, because neither survives
being retyped from a terminal: `block explain blk1003` and `block explain 1003`
find the same entry. Like `list`, it is offline and read-only.

The whole set is at [Error codes](/errors/), generated from the same registry
this command reads, so a code cannot mean one thing in a terminal and another
in a browser.

## `block version`

Prints the version of block, and the `block-registry` revision whose recipes
are compiled into it — so a resolution can be traced back to a reviewed recipe.
`block --version` is the same answer:

```console
$ block version
block v0.1.0
registry 09593c6e0254 (48 recipes from https://github.com/nao1215/block-registry)
```
