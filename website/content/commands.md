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
| `block which <cmd>` | never | never | never | never | no |
| `block status [--json]` | never | never | never | never | no |
| `block list [ecosystem]` | never | never | never | never | no |
| `block explain <code>` | never | never | never | never | no |
| `block completion <shell>` | never | never | never | never | no |

No flag turns a "never" cell into a yes.

## `block lock`

Resolves every tool in `block.toml` to the newest upstream release its
constraint allows, and records the download URL and SHA-256 per platform.
Naming tools re-resolves only those and keeps the other pins:

```console
$ block lock              # every tool
$ block lock hermes       # only hermes
```

It is the only command that moves a pin.

A constraint is a dotted version prefix (`"1"`, `"1.7"`, `"1.7.4"`), one
pre-release named exactly (`"1.8.0-rc1"`), or a channel, which is a release
line an upstream publishes under a tag that moves:

```toml
[tools]
foundry = "nightly"
```

A channel is the only input that floats, and `block.lock` never records it as
one. lock follows the moving tag and pins the release published for the commit
under it, whose tag never moves again:

```console
$ block lock
foundry  locked nightly-5e88010a83d1b87b8f4d13058e42a2949d3e9dc0
```

From there a nightly is a pin like any other: `sync` installs that release,
and the pin moves only when `lock` runs again. The upstream and the recipe
decide which channels a tool has; asking for one it does not publish lists the
ones it does.

One release of a channel can also be named outright, by the tag the upstream
published it under:

```toml
[tools]
foundry = "nightly-e469863b1ac3f2d9d48f9d25d068a14861060cb3"
```

`nightly` resolves through whatever the moving tag points at today, so it
follows the upstream's retagging and stops when the upstream stops retagging.
That has happened to Foundry's `nightly`: the daily builds still arrive as
`nightly-<commit>` tags, and the moving tag no longer catches up with them.
A named tag leaves nothing for the upstream to move. `block lock` resolves it in
one request and writes the same pin every time.

A commit the upstream does not publish is `BLK2003`. A release line the recipe
does not declare is `BLK2009`, the same refusal `foundry = "canary"` gets, and
it lists the channels that exist.

## `block lock --check`

The same resolution, without writing or downloading anything. It compares the
whole prospective lockfile, so it also reports changes that leave the version
alone, such as a narrowed constraint, a renamed executable or a moved artifact:

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

Installs every artifact `block.lock` names for this machine. It needs only the
lockfile's exact URLs, with no registry or GitHub API. It fails without
resolving or writing anything when the lockfile is missing, disagrees with
`block.toml` or lacks this platform, or when an artifact's checksum does not
match.

Tools are downloaded and installed a few at a time, up to an internal limit
that has no flag or setting. The report is one line per tool, in lockfile
order, printed after every tool has finished:

```console
$ block sync
foundry  1.7.4   installed
hermes   1.13.0  installed
commands: anvil, cast, chisel, forge, hermes
```

The last line lists the commands `block.lock` names for this project, whether
they were installed just now or were already there. The shim directory they
live in is shared with every other project you have synced, and `--verbose`
prints that directory as well:

```console
$ block sync --verbose
foundry  1.7.4   installed
hermes   1.13.0  installed
commands: anvil, cast, chisel, forge, hermes
shim directory: /home/me/.local/share/block/shims
shims written: anvil, cast, chisel, forge, hermes
shims present: anvil, cast, chisel, forge, gaiad, geth, hermes
```

`shims present` is every command the directory serves, including the ones
other projects put there. Nothing is ever removed from it.

Each install still verifies the artifact against the locked SHA-256, uses the
content-addressed cache when the artifact is already there, and moves it into
the store with one atomic rename. The first failure stops the rest and is the
one error reported.

## `block exec`

Runs a command with every executable from `block.lock` first on `PATH`, and
exits with the command's status:

```console
$ block exec forge test
$ block exec make test
$ block exec ./scripts/integration-test.sh
```

It never downloads, installs or resolves, but it checks offline that the
toolchain is the one `block.toml` asks for and that the install is intact.
`SIGINT` and `SIGTERM` reach the child, so a node or a local test network
shuts down the way it would outside block.

## `block which`

Prints the absolute path of the executable `block exec` would run for a
command, from the project's installed toolchain:

```console
$ block which solc
/home/me/.local/share/block/tools/solc/0.8.30-1a2b3c4d5e6f/solc
```

Unlike the shell's `which`, it never consults `PATH`. The answer comes from
`block.toml`, `block.lock` and the store, so a binary of the same name
installed some other way does not change it. From a nested directory it uses
the enclosing project, as `exec` does.

It fails as `exec` would, with the same codes, when the project is not ready:
a missing lockfile (BLK1003), a lockfile that no longer matches `block.toml`
(BLK1005), a tool that is locked but not yet installed (BLK4004). For a command
no locked tool provides, `exec` falls through to `PATH` but `which` refuses it
(BLK5001), because block itself runs nothing for that command:

```console
$ block which forge
block: BLK4004: foundry 1.7.4 is not installed; run "block sync"

$ block which node
block: BLK5001: block.toml does not lock a tool providing the command "node"
```

It prints one line on stdout, so a script can use it directly:

```console
$ SOLC="$(block which solc)" && "$SOLC" --version
```

## `block status`

Reports what `block.toml`, `block.lock` and the store say about each tool. It
resolves, downloads, installs and writes nothing, and uses no network:

```console
$ block status
TOOL      WANTED   LOCKED   INSTALLED   STATE
foundry   1.7      1.7.4    1.7.4       ok
hermes    1.13     1.13.0   -           missing

run "block sync" to install the locked toolchain
```

| State | What it means | What fixes it |
| --- | --- | --- |
| `ok` | installed, and the pin is what `block.toml` asks for | nothing |
| `missing` | pinned, not installed for this machine | `block sync` |
| `damaged` | installed, but the marker or an executable is gone | `block sync` |
| `stale` | `block.lock` does not match `block.toml`: not locked yet, a changed constraint or source, no artifact for this machine, or a pin nobody declares any more | `block lock` |

| Exit | Meaning |
| --- | --- |
| 0 | every tool is `ok` |
| 2 | something needs doing |
| 1 | error: no `block.toml`, or a `block.lock` that cannot be read |

`--json` prints the same report as one object, for CI and other tools:

```console
$ block status --json | jq -r '.tools[] | select(.state != "ok") | .name'
hermes
```

Every key is always present, so a consumer can read `.ready` for the overall
answer and `.tools[].state` per tool without checking that a field exists.

## Shims: the tools by their own names

`block sync` also puts one small file per command in `$BLOCK_HOME/shims`.
Add that directory to `PATH` once by hand (block never edits your startup
files), and the commands work by their own names:

```console
$ cd defi && forge --version
forge Version: 1.5.1-v1.5.1

$ cd ../bridge && forge --version
forge Version: 1.7.1
```

Each shim is the block binary under another name. Run as `forge`, it finds the
`block.toml` above the working directory, reads that project's `block.lock`,
and runs the version pinned there. The shim stores nothing, so one `forge`
serves every project, nothing needs regenerating when you switch branches, and
there is no shell hook or `eval`.

A shim does what `block exec` does and no more. It never resolves, downloads,
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

The snapshot is a vendored copy of
[block-registry](https://github.com/nao1215/block-registry) at one revision,
built into the binary, so a block version always pairs with a registry it was
tested against. `block version` prints the revision.

block never derives a toolchain from an ecosystem, because "Ethereum" means
contract development to one repository and validator operation to another.
You pick the tools and `block.toml` records them.

## `block explain`

Every refusal block prints carries a code such as `BLK1003` or `BLK3002`.
`block explain` prints what it means without a browser:

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

The prefix is optional and case does not matter: `block explain blk1003` and
`block explain 1003` find the same entry. Like `list`, it is offline and
read-only.

[Error codes](/errors/) lists every code, generated from the same registry this
command reads.

## Shell completion

`block completion` prints the script that lets a shell complete block's
commands and flags. Load it once from your shell's startup file:

```console
$ echo 'source <(block completion bash)' >> ~/.bashrc
```

```console
$ echo 'source <(block completion zsh)' >> ~/.zshrc
```

On zsh, a file in `fpath` works as well, so long as `compinit` runs after it:

```console
$ block completion zsh > "${fpath[1]}/_block"
```

```console
$ block completion fish > ~/.config/fish/completions/block.fish
```

PowerShell, in `$PROFILE`:

```powershell
block completion powershell | Out-String | Invoke-Expression
```

bash can also take the script from its completion directory instead of
`~/.bashrc`:

```console
$ block completion bash > /etc/bash_completion.d/block
```

Inside a project, `block lock <TAB>` offers the tools of `block.toml` and
`block which <TAB>` the commands of `block.lock`. Anywhere, `block list <TAB>`
offers the ecosystems and `block explain <TAB>` the error codes built into the
binary. Completion only reads; it never resolves, downloads or installs.

## `block version`

Prints the version of block and the `block-registry` revision whose recipes are
compiled into it, so a resolution can be traced back to a reviewed recipe.
`block --version` prints the same:

```console
$ block version
block v0.1.0
registry df8fa5946e00 (51 recipes from https://github.com/nao1215/block-registry)
```
