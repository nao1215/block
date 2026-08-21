---
title: Commands
description: "What each block command does, and the boundaries none of them cross."
---

| Command | Resolves versions | Writes `block.lock` | Downloads | Installs | Runs your command |
| --- | :-: | :-: | :-: | :-: | :-: |
| `block lock [tool...]` | yes | yes | artifacts with no upstream digest | no | no |
| `block lock --check` | yes | **no** | **no** | no | no |
| `block sync` | **no** | **no** | locked URLs, when not cached | yes | no |
| `block exec <cmd>` | **no** | **no** | **no** | **no** | yes |
| `block list [ecosystem]` | **no** | **no** | **no** | **no** | no |

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
foundry  1.7.4 -> 1.7.5
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

### Interactive use

There is no shell activation, no shims, and nothing written to your shell's
startup files: block sets `PATH` only for the process it starts. Two projects
can pin different versions without either leaking into the other, and there is
no "current version" to switch.

The price is typing `block exec`. For a session of hand-run commands, start a
shell inside the toolchain — `exec` runs any command, including your shell:

```console
$ block exec $SHELL
$ forge test          # the pinned forge, for the rest of this shell
```

## `block list`

Answers *what can block install?*, and with an argument, *which tools exist
for this blockchain system?* It reads the registry snapshot embedded in the
binary: no network, no `block.toml`, no `block.lock`.

Listing is discovery, not selection. block never derives a toolchain from an
ecosystem — "Ethereum" means contract development to one repository and
validator operation to another. You pick, `block.toml` records.
