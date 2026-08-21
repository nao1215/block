<!-- ALL-CONTRIBUTORS-BADGE:START - Do not remove or modify this section -->
[![All Contributors](https://img.shields.io/badge/all_contributors-1-orange.svg?style=flat-square)](#contributors-)
<!-- ALL-CONTRIBUTORS-BADGE:END -->

![Coverage](doc/coverage.svg)
[![Build](https://github.com/nao1215/block/actions/workflows/build.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/build.yml)
[![MultiPlatformUnitTest](https://github.com/nao1215/block/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/unit_test.yml)
[![E2E](https://github.com/nao1215/block/actions/workflows/e2e.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/e2e.yml)
[![reviewdog](https://github.com/nao1215/block/actions/workflows/reviewdog.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/reviewdog.yml)
[![tested with atago](https://img.shields.io/badge/tested%20with-atago-7c3aed?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTMuNiA0LjIgMTEuOSAxMmwtOC4zIDcuOC0xLjktMi4yTDcuOSAxMiAxLjcgNi40eiIvPjxyZWN0IGZpbGw9IiNmZmYiIHg9IjEyLjYiIHk9IjE3LjIiIHdpZHRoPSI5LjciIGhlaWdodD0iMi44IiByeD0iMS40Ii8%2BPC9zdmc%2B&logoColor=white)](https://github.com/nao1215/atago)
[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/block.svg)](https://pkg.go.dev/github.com/nao1215/block)
![GitHub](https://img.shields.io/github/license/nao1215/block)
[![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/nao1215/block/total)](https://github.com/nao1215/block/releases)

![demo](./doc/img/demo.gif)

block pins the blockchain CLI tools a repository depends on — Foundry, geth, Lighthouse, Agave, Gaia, an IBC relayer, whatever your chains need — and reproduces exactly the same toolchain on every developer machine and in CI. Tools are declared in `block.toml`, pinned by URL and SHA-256 in `block.lock`, and installed from that lockfile alone. It is a single static binary: no mise, aqua, Nix, Docker or package manager is involved.

Documentation: https://nao1215.github.io/block/

## Try it in 30 seconds

If you have Go, paste this into an empty directory:

```shell
printf '[tools]\nfoundry = "1.7"\n' > block.toml
go run github.com/nao1215/block@latest lock   # resolve: block.toml -> block.lock
go run github.com/nao1215/block@latest sync   # install what block.lock pins
go run github.com/nao1215/block@latest exec forge --version
```

Each step does one job and no more. `sync` is not optional before `exec`:
`exec` never installs anything, so it runs the toolchain `sync` put on disk or
it refuses.

Two files now say what your toolchain is. Commit both; everyone else — and CI —
runs `block sync` and gets the same binaries, byte for byte.

```console
$ git clone <project> && cd <project>
$ block sync
foundry  1.7.1   installed
hermes   1.13.3  installed
$ block exec forge test
```

## Three lifecycle commands, one direction

```text
block.toml  ──block lock──▶  block.lock  ──block sync──▶  installed toolchain  ──block exec──▶  command
```

| Command | Resolves versions | Writes `block.lock` | Downloads | Installs | Runs your command |
| --- | :-: | :-: | :-: | :-: | :-: |
| `block lock [tool...]` | yes | yes | only artifacts whose upstream publishes no digest | no | no |
| `block lock --check` | yes | never | never | no | no |
| `block sync` | never | never | locked URLs, when not cached | yes | no |
| `block exec <cmd>` | never | never | never | never | yes |
| `block list [ecosystem]` | never | never | never | never | no |
| `block explain <code>` | never | never | never | never | no |
| `forge`, `cast`, … (a shim) | never | never | never | never | yes |

"never" is a guarantee, not a default: no flag turns any of those cells into a
yes. A build cannot quietly pick up a release that happened overnight, and a
stale lockfile is an error rather than a guess.

Full detail: [Commands](https://nao1215.github.io/block/commands/).

## Run the tools by their own names

`block sync` writes one file per command into `$BLOCK_HOME/shims`. Put that
directory on `PATH` once, by hand, and the version follows the project you are
standing in — no shell hook, no `eval`, no activation.

![shims](./doc/img/shims.gif)

Outside a block project, or for a command the current project does not lock,
the shim steps aside and runs the next command of that name on `PATH`.

## Which tools can I use for this chain?

45 tools across 17 blockchain systems, answered offline from the registry
compiled into the binary — no network, no `block.toml`, no token.

![list](./doc/img/list.gif)

The whole catalogue, with the commands and platforms of each tool, is in
[doc/tools.md](./doc/tools.md), generated from the recipes themselves.
Listing is discovery, not selection: block never derives a toolchain from an
ecosystem. You pick, and `block.toml` records.

## Supported OS (unit testing with GitHub Actions)

- Linux
- macOS
- Windows

## Why block

- Blockchain CLIs, whatever their distribution. Release assets, raw
  executables and vendor download servers alike, resolved and verified the
  same way.
- Project-local, not machine-global. Two repositories on one machine can use
  different Foundry versions without fighting.
- Lockfile-driven. `block.lock` records the artifact URL and SHA-256 for every
  platform you care about; `sync` installs exactly that, or fails.
- CI is a first-class user. `block sync` is the same command with the same
  meaning locally and on a runner. No special flag.
- Upstream releases are detected, not catalogued. A recipe is a rule, so a new
  version of a tool needs no change anywhere.
- Multi-chain repositories are one toolchain. EVM and IBC tools sit in one
  manifest and one lockfile.

Where a container image is the better answer, and the numbers behind the
choice: [Compared to Docker](https://nao1215.github.io/block/comparison/).

## Install

```shell
go install github.com/nao1215/block@latest
```

```shell
brew install --cask nao1215/tap/block        # macOS, Linux
scoop bucket add nao1215 https://github.com/nao1215/block && scoop install nao1215/block
```

The [releases page](https://github.com/nao1215/block/releases) also carries
`.deb`, `.rpm` and `.apk` packages and archives for every supported platform.
Signature and provenance verification, and putting the shims on `PATH`, are on
the [install page](https://nao1215.github.io/block/install/).

In GitHub Actions:

```yaml
- uses: nao1215/setup-block@v0
  with:
    sync: "true"
- run: block exec forge test
```

## Documentation

| | |
|:--|:--|
| [Getting started](https://nao1215.github.io/block/getting-started/) | from nothing to a pinned toolchain |
| [Cookbook](./doc/cookbook.md) | 23 recipes indexed by task, including every refusal block can print |
| [examples/](./examples) | ready-made `block.toml` files for eight kinds of repository |
| [Commands](https://nao1215.github.io/block/commands/) | what each command does, and the boundaries none of them cross |
| [Reference](https://nao1215.github.io/block/reference/) | `block.toml`, `block.lock`, the store, version resolution, the recipe format |
| [Tools](./doc/tools.md) | every CLI block can install, by blockchain system |
| [Error codes](https://nao1215.github.io/block/errors/) | every `BLK` code block can report, and what to do about it |
| [CI](https://nao1215.github.io/block/ci/) | GitHub Actions, GitLab, CircleCI, Docker, and keeping pins current |
| [Security](https://nao1215.github.io/block/security/) | what block guarantees while downloading and running third-party binaries |

## Development

```shell
make test            # unit tests with -race
make e2e             # offline end-to-end suite (needs atago)
make lint            # golangci-lint v2
make coverage        # unit + e2e coverage combined into cover.out
make doc             # regenerate doc/tools.md from the registry recipes
make demo            # re-record the README GIFs (needs vhs and ffmpeg)
make website         # build the documentation site (needs hugo)
make registry-live   # check every recipe against the real upstreams (network)
make examples-live   # check that examples/*.toml still resolve (network)
```

The E2E suite ([e2e/atago](./e2e/atago)) is the CLI contract: every
user-visible behaviour — output, exit codes, files written, error messages —
is pinned there against the real binary and an offline fake GitHub. See
[CONTRIBUTING.md](./CONTRIBUTING.md).

## Related

- [Documentation website](https://nao1215.github.io/block/)
- [block-registry](https://github.com/nao1215/block-registry) — the canonical source of the recipes block embeds
- [setup-block](https://github.com/nao1215/setup-block) — GitHub Action that installs block and caches its toolchain

## The name

A block is the unit a chain is made of, and to block is to hold something
still. The tool does the second to the tools that build the first.

## Contributing

Issues and pull requests are welcome; see [CONTRIBUTING.md](./CONTRIBUTING.md).
Contributions are not only about code: a GitHub Star also motivates
development.

## LICENSE

The block project is licensed under the terms of [MIT LICENSE](./LICENSE).

## Contributors ✨

Thanks goes to these wonderful people ([emoji key](https://allcontributors.org/docs/en/emoji-key)):

<!-- ALL-CONTRIBUTORS-LIST:START - Do not remove or modify this section -->
<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
<table>
  <tbody>
    <tr>
      <td align="center" valign="top" width="14.28%"><a href="https://debimate.jp/"><img src="https://avatars.githubusercontent.com/u/22737008?v=4?s=75" width="75px;" alt="CHIKAMATSU Naohiro"/><br /><sub><b>CHIKAMATSU Naohiro</b></sub></a><br /><a href="https://github.com/nao1215/block/commits?author=nao1215" title="Code">💻</a> <a href="https://github.com/nao1215/block/commits?author=nao1215" title="Documentation">📖</a></td>
    </tr>
  </tbody>
</table>

<!-- markdownlint-restore -->
<!-- prettier-ignore-end -->

<!-- ALL-CONTRIBUTORS-LIST:END -->
