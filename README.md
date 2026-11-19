<!-- ALL-CONTRIBUTORS-BADGE:START - Do not remove or modify this section -->
[![All Contributors](https://img.shields.io/badge/all_contributors-1-orange.svg?style=flat-square)](#contributors-)
<!-- ALL-CONTRIBUTORS-BADGE:END -->

[![Build](https://github.com/nao1215/block/actions/workflows/build.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/build.yml)
[![MultiPlatformUnitTest](https://github.com/nao1215/block/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/unit_test.yml)
[![E2E](https://github.com/nao1215/block/actions/workflows/e2e.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/e2e.yml)
![Coverage](https://raw.githubusercontent.com/nao1215/octocovs-central-repo/main/badges/nao1215/block/coverage.svg)
[![Gitleaks](https://github.com/nao1215/block/actions/workflows/gitleaks.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/gitleaks.yml)
[![Trivy](https://github.com/nao1215/block/actions/workflows/trivy.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/trivy.yml)
[![govulncheck](https://github.com/nao1215/block/actions/workflows/govulncheck.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/govulncheck.yml)
[![Gosec](https://github.com/nao1215/block/actions/workflows/gosec.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/gosec.yml)
[![reviewdog](https://github.com/nao1215/block/actions/workflows/reviewdog.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/reviewdog.yml)
[![tested with atago](https://img.shields.io/badge/tested%20with-atago-7c3aed?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI2ZmZiIgZD0iTTMuNiA0LjIgMTEuOSAxMmwtOC4zIDcuOC0xLjktMi4yTDcuOSAxMiAxLjcgNi40eiIvPjxyZWN0IGZpbGw9IiNmZmYiIHg9IjEyLjYiIHk9IjE3LjIiIHdpZHRoPSI5LjciIGhlaWdodD0iMi44IiByeD0iMS40Ii8%2BPC9zdmc%2B&logoColor=white)](https://github.com/nao1215/atago)
[![measured with himorime](https://img.shields.io/badge/measured%20with-himorime-d9480f?logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0ibm9uZSIgc3Ryb2tlPSIjZmZmIiBzdHJva2Utd2lkdGg9IjIuNCIgc3Ryb2tlLWxpbmVjYXA9InJvdW5kIiBkPSJNNC4yIDE4LjVBOSA5IDAgMSAxIDE5LjggMTguNSIvPjxwYXRoIGZpbGw9Im5vbmUiIHN0cm9rZT0iI2ZmZiIgc3Ryb2tlLXdpZHRoPSIyLjQiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgZD0iTTEyIDE0LjUgMTYuNSA5Ii8%2BPGNpcmNsZSBmaWxsPSIjZmZmIiBjeD0iMTIiIGN5PSIxNC41IiByPSIyLjIiLz48L3N2Zz4=&logoColor=white)](https://github.com/nao1215/himorime)
[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/block.svg)](https://pkg.go.dev/github.com/nao1215/block)
[![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/nao1215/block/total)](https://github.com/nao1215/block/releases)
[![License](https://img.shields.io/github/license/nao1215/block)](./LICENSE)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/nao1215/block/badge)](https://scorecard.dev/viewer/?uri=github.com/nao1215/block)

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
| `block completion <shell>` | a completion script for bash, zsh, fish or PowerShell |

`sync` never resolves, `exec` never installs, and nothing updates by itself.
`foundry = "nightly"` works too: it pins the release under the tag that moves.
So does `foundry = "nightly-<commit>"`, which names one nightly outright and
never moves at all — [when to use which](https://nao1215.github.io/block/cookbook/#ask-for-one-particular-nightly).

block installs prebuilt artifacts that upstreams publish, and does not build
tools from source. `go install`, `cargo install`, `make` and upstream build
scripts are outside what a recipe can ask for, so adding a tool cannot add a
way to run something during installation. Every resolved artifact is pinned
in `block.lock` with its SHA-256.

That is a deliberate boundary. Not building at install time keeps a tool's
dependency resolver, its build scripts and the system toolchain out of the
step, which narrows the installation-time attack surface. It is worth having
where CI and developer machines can reach signing keys and other credentials.
It does not make an artifact safe: a digest pins what you get, not who built
it. [Security](https://nao1215.github.io/block/security/#what-block-installs)
has the reasoning and the limits.

## Install

```shell
go install github.com/nao1215/block@latest
```

```shell
brew install --cask nao1215/tap/block        # macOS, Linux
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

## Testing

| Technique | Bug class caught here |
|---|---|
| [Unit tests and atago E2E](./e2e/atago) | Wrong output, exit codes, files written and error codes of each command |
| [Property-based tests](./internal/version/properties_test.go) | Version order that is not a total preorder, `Latest` picking a lower or non-matching tag, a spelling lost on the way to block.lock |
| [Differential tests against golang.org/x/mod/semver](./internal/version/properties_test.go) | Pre-release precedence that disagrees with semver |
| [Fuzzing](./internal/manifest/manifest_test.go) | Crashes and round trips that drift in block.toml, block.lock, recipes, versions and archive paths |
| [Mutation testing](./.github/workflows/mutation.yml) | Conditions the unit tests run but do not check (89.81% efficacy on 2026-11-19) |
| [Live registry check](./registry/live_test.go) | Recipes that point at an asset the upstream no longer publishes |
| [Paired CI benchmarks](./bench/himorime.yaml) | Commands that got slower or use more memory |

Commands and details: [CONTRIBUTING.md](./CONTRIBUTING.md#7-testing-techniques).

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
| [Security](https://nao1215.github.io/block/security/) | what block does and does not guarantee while installing binaries |

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
