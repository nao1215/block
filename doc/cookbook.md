# Cookbook

Copyable recipes for pinning a blockchain toolchain. Each runs as shown; swap
in your own tool names and versions.

Three commands carry the workflow:

```text
block.toml  ──block lock──▶  block.lock  ──block sync──▶  installed toolchain  ──block exec──▶  command
```

`lock` resolves, `sync` installs and `exec` runs. `sync` never resolves,
`exec` never installs, and only `lock` can move a pin.

## Find a recipe by task

| I want to | Go to |
|:--|:--|
| Start pinning a toolchain in an existing repository | [Pin a toolchain in five lines](#pin-a-toolchain-in-five-lines) |
| Find out which tool to name for my chain | [Find the tool for a chain](#find-the-tool-for-a-chain) |
| Pin Foundry, and only Foundry | [Pin one tool](#pin-one-tool) |
| Pin the Solidity compiler Forge uses, not just Foundry | [Pin the Solidity compiler too](#pin-the-solidity-compiler-too) |
| Pin an exact version rather than a line | [Choose how tightly to pin](#choose-how-tightly-to-pin) |
| Make CI install the same toolchain as my laptop | [Lock for a platform you are not on](#lock-for-a-platform-you-are-not-on) |
| Set up a new machine from a checkout | [Reproduce the toolchain elsewhere](#reproduce-the-toolchain-elsewhere) |
| Move one pin forward without touching the others | [Update one tool](#update-one-tool) |
| Learn about upstream releases without editing anything | [Watch for upstream releases](#watch-for-upstream-releases) |
| Run `forge` without typing `block exec` | [Run tools by their own names](#run-tools-by-their-own-names) |
| Use different Foundry versions in two repositories | [Two projects, two toolchains](#two-projects-two-toolchains) |
| Add a tool the registry does not carry | [Bring your own tool](#bring-your-own-tool) |
| Pin a fork of a tool that is in the registry | [Pin a fork](#pin-a-fork) |
| Run a Cosmos chain and an EVM chain from one repository | [One repository, several chains](#one-repository-several-chains) |
| Run an `anvil` devnet against locked contracts | [Local devnet](#local-devnet) |
| Install the toolchain in GitHub Actions | [GitHub Actions](#github-actions) |
| Make CI installs offline and fast | [Cache the toolchain in CI](#cache-the-toolchain-in-ci) |
| Fail CI when the lockfile is out of date | [Guard the lockfile in CI](#guard-the-lockfile-in-ci) |
| Use block from GitLab CI, CircleCI or Jenkins | [CI without GitHub Actions](#ci-without-github-actions) |
| Pin the toolchain inside a Docker image | [Docker](#docker) |
| Drive block from a Makefile or an npm script | [Makefiles and task runners](#makefiles-and-task-runners) |
| Work out why `sync` or `exec` refused | [Read a refusal](#read-a-refusal) |
| Find where the downloads went, and take them back | [The store](#the-store) |
| Check what a release archive really is | [Verify a block release](#verify-a-block-release) |

## Pin a toolchain in five lines

Write `block.toml` beside your `foundry.toml` or `go.mod`, name the tools, lock,
and commit both files:

```shell
cat > block.toml <<'TOML'
platforms = ["darwin/arm64", "linux/amd64"]

[tools]
foundry = "1.7"
TOML
block lock
git add block.toml block.lock
```

```text
downloading https://github.com/foundry-rs/foundry/releases/download/v1.7.1/foundry_v1.7.1_linux_amd64.tar.gz
foundry  locked 1.7.1
wrote block.lock
```

Every machine and runner then gets the same toolchain from those two files:

```shell
block sync
block exec forge test
```

There is no `block init`, because a command that writes the four-line manifest
would have to guess what your project needs. [examples/](../examples) has a
manifest for eight kinds of repository: EVM contracts, an Ethereum node pair, a
Cosmos appchain, a Solana program, Bitcoin, Starknet, a multi-chain tree, and
one that brings its own tool.

## Find the tool for a chain

`block list` reads the registry snapshot compiled into the binary. It is
offline, needs no `block.toml`, and never installs anything:

```shell
block list                 # every tool, with the systems it serves
block list ethereum        # one system, with the commands each tool provides
block list cosmos
```

```text
NAME             COMMANDS        DESCRIPTION
celestia-app     celestia-appd   Celestia consensus node (celestia-appd)
celestia-node    celestia        Celestia data-availability node: bridge, full and light nodes
cometbft         cometbft        Byzantine fault-tolerant consensus engine and node behind Cosmos SDK chains
cosmos-relayer   rly             IBC relayer for Cosmos SDK chains written in Go, run as rly
cosmovisor       cosmovisor      Supervises a Cosmos SDK node binary across scheduled chain upgrades
gaia             gaiad           Cosmos Hub node (gaiad)
hermes           hermes          IBC relayer connecting Cosmos SDK chains, written in Rust
ignite           ignite          Scaffolds, builds and serves Cosmos SDK blockchains
osmosis          osmosisd        Osmosis appchain node (osmosisd), the Cosmos AMM
```

To find the tool that ships a command you know by name:

```shell
block list ethereum | grep -w cast          # which tool gives me "cast"?
block list | grep -i fuzz                   # what fuzzers are there?
```

A misspelling names the systems that exist instead of returning nothing:

```shell
block list etheruem
```

```text
block: BLK1010: unknown ecosystem "etheruem"
available ecosystems: aptos, avalanche, bitcoin, cardano, celestia, cosmos, ethereum, fabric, ibc, icp, ipfs, near, solana, starknet, stellar, zk, zksync
```

The same catalogue, rendered as a page: [Tools](/tools/).

`block list ethereum` never adds anything to `block.toml`. "Ethereum" means
contract development to one repository and validator operation to another, and
block does not guess which you meant.

## Pin one tool

```toml
[tools]
foundry = "1.7"
```

```shell
block lock
block sync
block exec forge --version
```

```text
forge Version: 1.7.1-v1.7.1
```

`foundry` is one entry with four commands (`forge`, `cast`, `anvil` and
`chisel`), because the upstream ships them together. All four are on `PATH`
inside `block exec`:

```shell
block exec cast --version
block exec anvil --version
```

## Pin the Solidity compiler too

Pinning Foundry does not pin the compiler. `forge` fetches the `solc` a project
asks for through svm, its own compiler downloader, and keeps it under
`~/.svm/`, so a project whose `block.toml` names only Foundry compiles with a
binary `block.lock` says nothing about:

```toml
[tools]
foundry = "1.7"
solc    = "0.8.28"
```

```shell
block lock
block sync
block exec forge build --use "$(block which solc)" --offline
```

```text
Compiling 1 files with Solc 0.8.28
Solc 0.8.28 finished in 3.69ms
Compiler run successful!
```

`solc = "0.8.28"` in `foundry.toml` chooses a compiler version; it does not
choose where the binary comes from, and `forge` will still download one to
satisfy it. `block which solc` prints the absolute path of the compiler in the
store, `--use` hands `forge` that file instead of svm's, and `--offline` turns
a download it would have made anyway into a refusal:

```shell
block exec forge build --offline
```

```text
Error: can't install missing solc 0.8.28 in offline mode
```

`forge test`, `forge create` and `forge script` take the same two flags as
`forge build`, so put them in the Makefile target or the CI step:

```makefile
SOLC := $(shell block which solc)

build:
	block exec forge build --use "$(SOLC)" --offline

test:
	block exec forge test --use "$(SOLC)" --offline
```

`pragma solidity ^0.8.20` is the range of compilers a source file accepts,
checked by whichever compiler runs. The pin is `block.lock`, and `--use` makes
`forge` follow it.

## Choose how tightly to pin

A version in `block.toml` is a dotted prefix. There are no operators or
ranges, and a pre-release is selected only when it is named:

```toml
[tools]
solc    = "0.8.28"     # exactly that release, forever
foundry = "1.7"        # newest 1.7.x at the last "block lock"
hermes  = "1"          # newest 1.x.y at the last "block lock"
reth    = "1.9.0-rc1"  # that pre-release and no other
```

The resolved version goes into `block.lock`, and that is what installs. The
prefix only says how far a later `block lock` may move the pin; `sync` always
installs exactly what is locked.

Pin an exact version when the build depends on it, such as a Solidity compiler
whose output must be reproducible or a chain binary that has to match the
network. Pin the minor line to take patches without deciding each one.

## Follow a nightly without giving up the lockfile

Some teams build against Foundry's nightly. Ask for the channel by name:

```toml
[tools]
foundry = "nightly"
```

```console
$ block lock
foundry  locked nightly-5e88010a83d1b87b8f4d13058e42a2949d3e9dc0
```

`nightly` is a tag the upstream moves every night, so `block.lock` does not
record it. lock pins the release published for the commit under it, a tag that
never moves, with its URL and SHA-256. Everyone who runs `block sync` installs
the same nightly until you run `block lock` again:

```console
$ block lock --check
foundry  nightly-5e88010a83d1b… -> nightly-2b2c1b6f0a94…
$ echo $?
2
```

The upstream decides which channels exist. Asking for one it does not publish
lists the ones it does.

## Ask for one particular nightly

`foundry = "nightly"` follows a tag the upstream moves, and an upstream can stop
moving it. Foundry has: daily builds still arrive as their own
`nightly-<commit>` tags, but the moving tag no longer follows them, so
`"nightly"` keeps resolving to the build it last pointed at.

Name the tag instead:

```toml
[tools]
foundry = "nightly-e469863b1ac3f2d9d48f9d25d068a14861060cb3"
```

```console
$ block lock
foundry  locked nightly-e469863b1ac3f2d9d48f9d25d068a14861060cb3
```

Copy the tag from the upstream's releases page, or from a `block.lock` that
`"nightly"` already wrote. The assets, the URL and SHA-256 in `block.lock`, and
`block sync` work as before.

| Written | `block lock` gives you | Running `block lock` again |
| --- | --- | --- |
| `foundry = "nightly"` | whatever the moving tag points at now | moves the pin when the upstream retagged |
| `foundry = "nightly-<commit>"` | that build | writes the same pin back |

Both forms lock an immutable build. With the second, `block lock` on a fresh
checkout also gives everybody the build you chose instead of whatever is current.
Use it to bisect an upstream regression, when a bug report names a nightly, and
for now to get a recent Foundry nightly.

A commit the upstream does not publish is `BLK2003`, and a release line the
recipe does not declare is `BLK2009`.

## Lock for a platform you are not on

List every platform your team and CI use:

```toml
platforms = ["darwin/arm64", "linux/amd64"]

[tools]
foundry = "1.7"
```

```shell
block lock
```

`block.lock` then has a URL and a SHA-256 per platform, and the Linux runner
installs from the file the macOS laptop wrote. Without the list, `lock` resolves
only for the machine it runs on, and the runner stops with:

```text
block: BLK1005: block.lock is stale; run "block lock"
  foundry: block.lock has no artifact for linux/amd64
```

because `sync` never installs anything the lockfile does not name.

A list that leaves out the current machine gets a different message, because
re-running `lock` would write the same file back:

```text
block: BLK1006: block.toml declares platforms darwin/arm64, and this machine is linux/amd64; add "linux/amd64" to that list and run "block lock"
```

Supported values: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
`windows/amd64`, `windows/arm64`. Which of them a tool has is up to the
upstream and recorded in the registry; see the Platforms column in
[Tools](/tools/).

Adding a platform later is one line and one re-lock:

```shell
sed -i 's|^platforms = .*|platforms = ["darwin/arm64", "darwin/amd64", "linux/amd64"]|' block.toml
block lock
```

Removing `platforms` later keeps what CI needs: `lock` keeps the platforms the
existing lockfile covers, so re-locking on a laptop cannot drop the Linux
artifact.

## Reproduce the toolchain elsewhere

A fresh machine or clone needs no configuration:

```shell
git clone <project> && cd <project>
block sync
block exec forge test
```

```text
foundry  1.7.1   installed
hermes   1.13.3  installed
commands: anvil, cast, chisel, forge, hermes
```

`sync` reads only `block.lock`, with no registry lookup, GitHub API call or
token. It downloads the listed URLs, checks each against its recorded SHA-256,
and installs. With the store already populated, a second run takes a few
milliseconds and uses no network:

```shell
time block sync
```

## Update one tool

```shell
block lock hermes          # move hermes, keep every other pin exactly as it is
```

```text
hermes  1.13.2 -> 1.13.3
```

The whole set:

```shell
block lock
```

```text
foundry  1.7.1 -> 1.7.2
hermes   1.13.3
```

`locked X` is a new pin, `X -> Y` a moved one, and a bare version did not move.
`sync`, `exec`, a registry update or time passing never move a pin.

Review the diff the way you review a dependency bump:

```shell
git diff block.lock
```

## Watch for upstream releases

`block lock --check` performs the same resolution and writes nothing:

```shell
block lock --check
```

```text
foundry  1.7.1 -> 1.7.2
hermes   1.13.3 (up-to-date)
```

| Exit | Meaning |
|:-:|:--|
| 0 | `block.lock` is current |
| 2 | `block.lock` would change: a newer release, a tool added or removed, a new platform |
| 1 | error (network, manifest, …) |

A weekly job that opens a pull request when something moved:

```yaml
name: Toolchain updates
on:
  schedule: [{ cron: "0 6 * * 1" }]
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  bump:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: nao1215/setup-block@v0
      - id: check
        # `cmd || echo ...` treats every failure as staleness, including exit 1
        # — a network error or a broken manifest — and would open a pull
        # request from a resolution that never finished. Only 2 means "the
        # lockfile would change"; 1 is an error and has to fail the job.
        run: |
          set +e
          block lock --check
          status=$?
          set -e
          case "$status" in
            0) ;;
            2) echo "stale=2" >> "$GITHUB_OUTPUT" ;;
            *) exit "$status" ;;
          esac
      - if: steps.check.outputs.stale == '2'
        run: block lock
      - if: steps.check.outputs.stale == '2'
        uses: peter-evans/create-pull-request@v7
        with:
          title: "chore: move the toolchain pins forward"
          branch: block/toolchain-update
```

`--check` never downloads an artifact, so a scheduled run costs a handful of
GitHub API calls.

## Run tools by their own names

`block sync` writes one small file per command into `$BLOCK_HOME/shims`. Put
that directory on `PATH` once, by hand, and run the commands by name:

```shell
# Unix, in ~/.bashrc or ~/.zshrc
export PATH="$HOME/.local/share/block/shims:$PATH"
```

```powershell
# Windows, once
[Environment]::SetEnvironmentVariable(
  "Path", "$env:LOCALAPPDATA\block\shims;$env:Path", "User")
```

```shell
forge test
cast block latest
hermes health-check
```

Each shim is the block binary under another name. Run as `forge`, it finds the
`block.toml` above the working directory, reads that project's `block.lock`,
and runs the version pinned there. The shim stores nothing, so one `forge`
serves every project, nothing needs regenerating when you switch branches, and
there is no shell hook, `eval` or `direnv`.

Outside a block project, or for a command the project does not lock, the shim
runs the next command of that name on `PATH`:

```shell
cd /tmp && forge --version     # a forge installed some other way still runs
```

So the shims directory cannot hide a tool from the rest of your system. In CI,
keep using `block exec`, which needs no `PATH` setup.

## Two projects, two toolchains

```shell
cd ~/src/defi   && cat block.toml
```

```toml
[tools]
foundry = "1.5.1"
```

```shell
cd ~/src/bridge && cat block.toml
```

```toml
[tools]
foundry = "1.7"
```

With the shims on `PATH`, the version follows the current directory:

```shell
cd ~/src/defi   && forge --version
```

```text
forge Version: 1.5.1-v1.5.1
```

```shell
cd ~/src/bridge && forge --version
```

```text
forge Version: 1.7.1
```

Both installs live in one store, shared with every project that pins the same
artifact, so a second project pinning it downloads nothing.

Commands find `block.toml` in the current directory or any parent, so this
works from a sub-package of a monorepo too:

```shell
cd ~/src/monorepo/packages/contracts && forge test    # uses ~/src/monorepo/block.lock
```

## Bring your own tool

A tool the registry does not carry is declared in `block.toml` itself, with the
same fields a registry recipe uses:

```toml
[tools.foo]
version = "1.2"

[tools.foo.source]
type = "github_release"
repo = "example/foo"
asset = "foo_{version}_{os}_{arch}.tar.gz"   # {version} carries no "v"
bin = ["foo"]                                # executables inside the archive
```

The knobs, all optional:

```toml
[tools.foo.source]
type = "github_release"
repo = "example/foo"
asset = "foo-{version}-{target}.tar.gz"
bin = ["bin/foo", "bin/foo-helper"]   # paths inside the archive
tag_prefix = "release-"               # tags are "release-1.2.0", not "v1.2.0"
strip_components = 1                  # the archive wraps everything in foo-1.2.0/
platforms = ["linux/amd64", "darwin/arm64"]

[tools.foo.source.os]                 # rename {os}
linux = "unknown-linux-gnu"
darwin = "apple-darwin"

[tools.foo.source.arch]               # rename {arch}
amd64 = "x86_64"
arm64 = "aarch64"

[tools.foo.source.target]             # or replace the whole os/arch pair with {target}
"linux/amd64" = "x86_64-linux-gnu"
"darwin/arm64" = "arm64-apple-darwin"
```

Set `tag_prefix = ""` explicitly for upstreams whose tags are bare `1.2.0`.
An `asset` with no archive extension is a single raw executable, installed
under the one name in `bin`.

Lock it like anything else:

```shell
block lock foo
block exec foo --version
```

The lockfile records a fingerprint of the definition, so editing the source
later makes the pin stale:

```text
block: BLK1005: block.lock is stale; run "block lock"
  foo: the source definition changed since block.lock was resolved
```

When more than one project needs it, copy the definition to
[block-registry](https://github.com/nao1215/block-registry); the fields are the
same. Until then you do not have to wait for a registry merge.

## Pin a fork

A project-local source overrides the registry for that name, so the rest of
the manifest and every `block exec` stay unchanged.

```toml
[tools.foundry]
version = "1.7"

[tools.foundry.source]
type = "github_release"
repo = "our-org/foundry"
asset = "foundry_v{version}_{os}_{arch}.tar.gz"
bin = ["forge", "cast", "anvil", "chisel"]
```

```shell
block lock foundry
block exec forge --version
```

To go back to upstream, delete the `[tools.foundry.source]` table and re-lock.

## One repository, several chains

EVM and IBC tools are one toolchain, one manifest and one lockfile:

```toml
platforms = ["darwin/arm64", "linux/amd64"]

[tools]
foundry = "1.7"        # forge, cast, anvil, chisel
solc    = "0.8"        # the compiler your CI must agree on
gaia    = "27"         # gaiad, the Cosmos Hub node
hermes  = "1.13"       # the IBC relayer
```

```shell
block sync
block exec ./scripts/e2e.sh     # every command above is on PATH inside the script
```

`block exec` puts the toolchain on `PATH` for whatever it runs, so build
scripts, `make` and test harnesses see the locked versions without knowing
about block:

```shell
block exec make integration-test
block exec npm run e2e
block exec pytest tests/
```

## Local devnet

```toml
[tools]
foundry = "1.7"
```

```shell
block sync
block exec anvil &                       # a local EVM node on 8545
block exec forge script script/Deploy.s.sol --broadcast --rpc-url http://127.0.0.1:8545
```

block forwards `SIGINT` and `SIGTERM` to the child, so `Ctrl-C` shuts a node
down as it would outside block. block exits with the child's status, or
`128+signal` when a signal ended it.

```shell
block exec anvil
# ^C
echo $?     # 130
```

The node's data directory, ports and files are ordinary local ones; block maps,
mounts and forwards nothing.

## GitHub Actions

```yaml
- uses: actions/checkout@v6
- uses: nao1215/setup-block@v0
  with:
    sync: "true"
- run: block exec forge test
```

[setup-block](https://nao1215.github.io/setup-block/) installs the CLI, verifies
its checksum, exports `$BLOCK_HOME`, caches the store on your `block.lock`, and
runs `sync`. Without the action:

```yaml
- uses: actions/checkout@v6
- name: Install block
  env:
    BLOCK_VERSION: 0.1.0
  run: |
    curl -sSfL "https://github.com/nao1215/block/releases/download/v${BLOCK_VERSION}/block_${BLOCK_VERSION}_linux_amd64.tar.gz" | tar xz
    sudo install -m 0755 block /usr/local/bin/block
- uses: actions/cache@v4
  with:
    path: ~/.local/share/block
    key: block-${{ runner.os }}-${{ hashFiles('block.lock') }}
- run: block sync
- run: block exec forge test
```

There is no CI mode. `block sync` does the same on a runner as on a laptop:
install what the lockfile says, or fail.

## Cache the toolchain in CI

Key the cache on `block.lock`, which names every artifact and digest the job
uses:

```yaml
- uses: actions/cache@v4
  with:
    path: ~/.local/share/block
    key: block-${{ runner.os }}-${{ hashFiles('block.lock') }}
```

On a hit, `block sync` downloads nothing. The store is content-addressed and a
cached file is re-hashed before use, so a truncated archive from a
half-restored cache is fetched again instead of installed.

Cache the same directory on any runner by pointing `BLOCK_HOME` at a path the
CI system already caches:

```shell
export BLOCK_HOME="$CI_PROJECT_DIR/.block"
```

## Guard the lockfile in CI

`sync` already refuses when `block.toml` and `block.lock` disagree, so a pull
request that edits the manifest without re-locking fails on the `sync` step. To
fail earlier, with a clearer step name:

```yaml
- name: block.lock matches block.toml
  run: block sync
```

To report pins that have fallen behind upstream without failing the build:

```yaml
- name: Report toolchain updates
  continue-on-error: true
  run: block lock --check
```

`--check` exits 2 when something moved. Keep it out of the required checks,
since a new upstream release does not mean the build is broken.

## CI without GitHub Actions

The install is a tarball and the commands are the same everywhere. GitLab CI:

```yaml
variables:
  BLOCK_HOME: "$CI_PROJECT_DIR/.block"

toolchain:
  cache:
    key:
      files: [block.lock]
    paths: [".block"]
  before_script:
    - curl -sSfL "https://github.com/nao1215/block/releases/download/v0.1.0/block_0.1.0_linux_amd64.tar.gz" | tar xz -C /usr/local/bin block
    - block sync
  script:
    - block exec forge test
```

CircleCI:

```yaml
steps:
  - checkout
  - restore_cache: { keys: ["block-{{ checksum \"block.lock\" }}"] }
  - run: curl -sSfL "https://github.com/nao1215/block/releases/download/v0.1.0/block_0.1.0_linux_amd64.tar.gz" | sudo tar xz -C /usr/local/bin block
  - run: block sync
  - save_cache:
      key: block-{{ checksum "block.lock" }}
      paths: ["~/.local/share/block"]
  - run: block exec forge test
```

`GITHUB_TOKEN` matters only to `block lock`, which calls the GitHub API. `sync`
and `exec` never do, so a CI job that only syncs needs no token at all.

## Docker

`block sync` inside a `Dockerfile` pins the image's tools by checksum:

```dockerfile
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*
RUN curl -sSfL https://github.com/nao1215/block/releases/download/v0.1.0/block_0.1.0_linux_amd64.tar.gz \
  | tar xz -C /usr/local/bin block

WORKDIR /src
COPY block.toml block.lock ./
RUN block sync                       # cached layer: it changes when block.lock does
COPY . .
RUN block exec forge build
```

Copying `block.toml` and `block.lock` before the rest of the source makes the
toolchain layer cacheable, as with `go.mod`/`go.sum` or `package-lock.json`.

## Makefiles and task runners

Put `block exec` in the recipe, and everyone who runs `make test` gets the
locked toolchain whether or not they know block is there:

```makefile
.PHONY: test deploy toolchain

toolchain:
	block sync

test: toolchain
	block exec forge test

deploy: toolchain
	block exec forge script script/Deploy.s.sol --broadcast
```

One `block exec` around a whole script is usually better than one per command:
the toolchain is on `PATH` for everything inside it.

An editor plugin, a linter's `--solc` flag or a Hardhat config that needs a
tool's path can get it from `block which`, which prints the absolute path
`block exec` would run and never looks at `PATH`:

```shell
solhint --solc "$(block which solc)" 'contracts/**/*.sol'
```

It fails where `exec` would (not synced, not locked). Unlike `exec`, it also
refuses a command the project does not lock instead of falling back to `PATH`,
so a Makefile that uses it cannot pick up a system copy.

```json
{
  "scripts": {
    "test": "block exec forge test",
    "node": "block exec anvil"
  }
}
```

## Read a refusal

block refuses where guessing would defeat the lock, and each message names the
command that fixes it.

Each refusal carries a code such as `BLK1003` or `BLK3002`, which stays the
same when the sentence beside it is reworded. Look one up in the terminal:

```shell
block explain BLK1003
```

The whole list, with what block observed and what to do about it, is at
[Error codes](https://nao1215.github.io/block/errors/). The thousands digit
says where the fix lives: `1` the project's own files, `2` resolving against an
upstream, `3` the download, `4` the install, `5` running a command, `6` a
refusal on security grounds. All of them exit 1. Exit 2 is reserved for
`block lock --check`, where it means the lockfile would change.

Nothing has been locked yet:

```text
block: BLK1003: block.lock not found; run "block lock" and "block sync"
```

The manifest changed and the lockfile has not caught up. Every disagreement is
listed:

```text
block: BLK1005: block.lock is stale; run "block lock"
  foundry: block.toml wants "1.6" but block.lock was resolved from "1.7"
  hermes is declared in block.toml but missing from block.lock
```

The lockfile is current, but nothing is installed, as in a fresh clone or a
cleared store:

```text
block: BLK4004: foundry 1.7.1 is not installed; run "block sync"
```

An install was interrupted. block writes a completion marker last and renames
the directory into place atomically, so an install without that marker, or
missing one of its executables, is replaced instead of run:

```text
block: BLK4003: foundry 1.7.1 is damaged: executable "cast" is missing; run "block sync"
```

You asked for a command nothing locks, and `PATH` has no other:

```text
block: BLK5001: command "no-such-tool" not found in the locked toolchain or on PATH
```

A download does not match the digest recorded when the pin was made:

```text
block: BLK3002: foundry: checksum mismatch for https://github.com/foundry-rs/foundry/releases/download/v1.7.1/foundry_v1.7.1_linux_amd64.tar.gz: want sha256 0000…, got 9f86…
```

Nothing is extracted or installed. Run it once more in case a proxy truncated
the transfer; if it persists, the artifact at that URL is not the one that was
locked.

The upstream ships no build for this machine, and block does not substitute
another:

```text
block: BLK2007: maconly: unsupported platform linux/amd64 (available: darwin/arm64)
```

The name is not in the registry, and the message gives both ways forward:

```text
block: BLK1007: unknown tool "solana": it is not in the registry (run "block list" to see the supported tools); define [tools.solana.source] in block.toml to use it anyway
```

No release matches the constraint you wrote:

```text
block: BLK2001: foundry: no version of foundry-rs/foundry matches "3"
```

`block lock` calls the GitHub API, and an unauthenticated runner has 60 calls
an hour:

```text
block: BLK2005: foundry: github api: rate limit exceeded (set GITHUB_TOKEN to raise the limit); resets at 2023-11-14T22:13:20Z
```

`sync` and `exec` never call the API, so this can interrupt a re-lock but not a
build.

A constraint that looks like a range is refused:

```text
block: BLK1002: block.toml: tool "foundry": invalid version constraint "^1.7": component "^1" is not a number
```

Two tools that provide the same command name are reported instead of resolved
by `PATH` order:

```text
block: BLK1009: tools "agave" and "my-solana-fork" both provide the command "solana"; remove one from block.toml
```

The same inside one tool, where two paths in the archive end in one command
name:

```text
block: BLK1009: tool "foo" lists "bin/foo" and "sbin/foo", which are both the command "foo"
```

Command names are compared case-insensitively on every platform, as Windows
resolves them on `PATH`. A lockfile is read everywhere, so a toolchain that
would collide on Windows is refused when anyone runs `block lock`, on any OS:

```text
block: BLK1009: tool "foo" lists "foo" and "FOO", which are both the command "FOO"
```

A shim run outside a block project, for a command nothing else on `PATH`
provides:

```text
block: BLK5004: forge: no block project here and no forge elsewhere on PATH
```

## The store

Everything block downloads and installs lives under one directory, shared by
every project:

```text
$BLOCK_HOME/                               Unix:    ~/.local/share/block
  cache/sha256/<digest>                    Windows: %LOCALAPPDATA%\block
  tools/<name>/<version>-<digest12>/
  shims/<command>
```

```shell
du -sh ~/.local/share/block          # how much is it holding?
ls ~/.local/share/block/tools        # what is installed
```

Two projects that pin the same artifact share one download and one install.
`XDG_DATA_HOME` is honoured when `BLOCK_HOME` is unset.

Point it elsewhere for one command, a shell or a CI job:

```shell
BLOCK_HOME=/tmp/block-scratch block sync
export BLOCK_HOME="$CI_PROJECT_DIR/.block"
```

To reclaim the space, delete the directory. Only the `shims` entry on your
`PATH` refers into it, and the next `block sync` rebuilds it from `block.lock`:

```shell
rm -rf ~/.local/share/block
block sync
```

## Verify a block release

Every published archive and `.deb`, `.rpm` and `.apk` package is listed in
`checksums.txt`, has an SPDX SBOM beside it, and carries SLSA build provenance
attested through GitHub OIDC.

cosign signs `checksums.txt` keyless. Verify that signature, then check your
download against its line in the list. Provenance is attested per artifact, so
`gh attestation verify` works on whichever file you downloaded.

```shell
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nao1215/block/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

Which registry revision a given binary carries, and how many recipes that is:

```shell
block version
```

```text
block v0.1.0
registry df8fa5946e00 (51 recipes from https://github.com/nao1215/block-registry)
```

The recipes are vendored from
[block-registry](https://github.com/nao1215/block-registry) at one revision and
embedded, so a resolution traces back to a reviewed recipe, and `block list` and
`block lock` need no registry service.
