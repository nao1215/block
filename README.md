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

block is a reproducible toolchain manager for blockchain development. Declare
the CLIs your project builds with in `block.toml`; everyone and CI install the
same versions from `block.lock`.

Documentation: https://nao1215.github.io/block/

## Try it in 30 seconds

```shell
printf '[tools]\nfoundry = "1.7"\n' > block.toml
go run github.com/nao1215/block@latest lock   # block.toml -> block.lock
go run github.com/nao1215/block@latest sync   # install what block.lock pins
go run github.com/nao1215/block@latest exec forge --version
```

Commit both files. Everyone else runs `block sync` and gets the same binaries.

Foundry ships Linux and macOS builds only. On Windows, the same four lines with
a tool that does:

```shell
printf '[tools]\nsolc = "0.8"\n' > block.toml
go run github.com/nao1215/block@latest lock
go run github.com/nao1215/block@latest sync
go run github.com/nao1215/block@latest exec solc --version
```

Which CLIs are available: [Tools](https://nao1215.github.io/block/tools/).

## Commands

| | |
|:--|:--|
| `block lock [tool...]` | resolve `block.toml` into `block.lock` — the only command that moves a pin |
| `block sync` | install what `block.lock` pins, or fail |
| `block exec <cmd>` | run a command with the locked toolchain on `PATH` |
| `block which <cmd>` | the executable `exec` would run, as an absolute path — never `PATH` |
| `block status [--json]` | what the two files and the store say, read-only |
| `block list [ecosystem]` | the tools block can install |
| `block explain <code>` | what a `BLK` error code means |
| `block completion <shell>` | a completion script for bash, zsh or fish |

`sync` never resolves, `exec` never installs, and nothing updates by itself.
`foundry = "nightly"` works too: it pins the release under the tag that moves.

## Install

```shell
go install github.com/nao1215/block@latest
```

```shell
brew install --cask nao1215/tap/block        # macOS, Linux
scoop bucket add nao1215 https://github.com/nao1215/block && scoop install nao1215/block
```

The [releases page](https://github.com/nao1215/block/releases) also has `.deb`,
`.rpm`, `.apk` and archives. In GitHub Actions:

```yaml
- uses: nao1215/setup-block@v0
  with:
    sync: "true"
- run: block exec forge test
```

## Supported OS (unit testing with GitHub Actions)

- Linux
- macOS
- Windows

## Documentation

| | |
|:--|:--|
| [Getting started](https://nao1215.github.io/block/getting-started/) | from nothing to a pinned toolchain |
| [Cookbook](./doc/cookbook.md) | recipes indexed by task |
| [examples/](./examples) | ready-made `block.toml` files |
| [Commands](https://nao1215.github.io/block/commands/) | every command, in detail |
| [Reference](https://nao1215.github.io/block/reference/) | file formats, the store, how versions resolve |
| [Error codes](https://nao1215.github.io/block/errors/) | every `BLK` code and what to do about it |
| [CI](https://nao1215.github.io/block/ci/) | GitHub Actions, GitLab, CircleCI, Docker |
| [Security](https://nao1215.github.io/block/security/) | what block guarantees while downloading binaries |

## Related

- [block-registry](https://github.com/nao1215/block-registry) — where the recipes are written
- [setup-block](https://github.com/nao1215/setup-block) — GitHub Action that installs block and caches its toolchain

## Contributing

Issues and pull requests are welcome; see [CONTRIBUTING.md](./CONTRIBUTING.md).
`make test`, `make e2e` and `make lint` are what CI runs. Contributions are not
only about code: a GitHub Star also motivates development.

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
