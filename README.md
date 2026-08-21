# block — lock your blockchain toolchain

[![MultiPlatformUnitTest](https://github.com/nao1215/block/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/unit_test.yml)
[![E2E](https://github.com/nao1215/block/actions/workflows/e2e.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/e2e.yml)
[![golangci-lint](https://github.com/nao1215/block/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/nao1215/block/actions/workflows/golangci-lint.yml)
![Coverage](doc/coverage.svg)

`block` pins the blockchain CLI tools a repository depends on — Foundry, an IBC
relayer, whatever your chains need — and reproduces exactly the same toolchain
on every developer machine and in CI.

```console
$ git clone <project> && cd <project>
$ block sync --locked
foundry  1.7.4   installed
hermes   1.13.3  installed
$ block exec forge test
```

Two files do all the work:

| File | Written by | Holds |
| --- | --- | --- |
| `block.toml` | you | the tools you want and roughly which versions |
| `block.lock` | `block lock` | the exact release, download URL and SHA-256 per platform |

block is not a package manager. It downloads release archives from upstream,
verifies them, and puts them on `PATH` for the command you run. Nothing more.

## Why block

- **Project-local, not machine-global.** Each repository declares its own
  toolchain; two projects on one machine can use different Foundry versions
  without fighting.
- **Lockfile-driven reproducibility.** `block.lock` records the artifact URL
  and checksum for every platform you care about. CI installs exactly that,
  or fails.
- **CI is a first-class user.** `block sync --locked` never resolves versions,
  never rewrites the lockfile and fails loudly on any drift.
- **Upstream releases are detected, not catalogued.** The registry holds a
  *recipe* per tool (repository, asset naming, executables), not a list of
  versions. A new upstream release needs no registry change.
- **Multi-chain repositories are one toolchain.** EVM and IBC tools sit in one
  manifest and one lockfile.
- **Single static binary, no runtime dependencies.** No mise, aqua, Nix,
  Docker or package manager is invoked.

## Install

Download a release archive from the
[releases page](https://github.com/nao1215/block/releases) or build from source:

```shell
go install github.com/nao1215/block@latest
```

Homebrew:

```shell
brew install nao1215/tap/block
```

## Quick start

```console
$ block init
created block.toml
```

`block.toml`:

```toml
[tools]
foundry = "1.7"
hermes = "1.13"
```

```console
$ block lock
downloading https://github.com/foundry-rs/foundry/releases/download/v1.7.4/foundry_v1.7.4_linux_amd64.tar.gz
downloading https://github.com/informalsystems/hermes/releases/download/v1.13.3/hermes-v1.13.3-x86_64-unknown-linux-gnu.tar.gz
foundry  locked 1.7.4   +linux/amd64
hermes   locked 1.13.3  +linux/amd64
wrote block.lock

$ block sync
foundry  1.7.4   installed
hermes   1.13.3  installed

$ block exec forge --version
forge Version: 1.7.4-stable
```

Commit both `block.toml` and `block.lock`.

## Commands

| Command | What it does | Touches the network | Writes `block.lock` |
| --- | --- | --- | --- |
| `block init` | write a starter `block.toml` | no | no |
| `block lock` | resolve `block.toml` into `block.lock`; keeps pins that still satisfy the manifest | yes | yes |
| `block sync` | install everything in `block.lock` for this machine; locks first if needed | if needed | only if stale or missing |
| `block sync --locked` | install exactly what `block.lock` says, or fail | downloads only | never |
| `block update [tool...]` | move pins to the newest release allowed by `block.toml` | yes | yes |
| `block outdated` | show pins that have a newer matching upstream release | yes | no |
| `block exec <cmd> [args...]` | run a command with the locked tools first on `PATH` | no | no |
| `block registry` | list the tools the built-in registry knows | no | no |
| `block version` | print the version | no | no |

`block lock` and `block update` are deliberately different: `lock` only fills
gaps (new tools, changed constraints, new platforms) and otherwise leaves every
pin alone; `update` is the only command that moves an existing pin forward.

`block exec` puts every executable listed in `block.lock` first on `PATH` and
runs the command, so `block exec make test` works when your Makefile calls
`forge`. It exits with the command's status and never downloads anything —
run `block sync` first.

All commands find `block.toml` in the current directory or any parent, so
they work from a sub-package of a monorepo.

## block.toml

```toml
# Optional. Platforms `block lock` resolves artifacts for.
# Default: the platform you run lock on.
platforms = ["linux/amd64", "darwin/arm64"]

[tools]
foundry = "1.7"      # newest 1.7.x
hermes = "1"         # newest 1.x.y
```

A version is a **dotted prefix**: `"1"` means the newest `1.x.y`, `"1.7"` the
newest `1.7.y`, `"1.7.4"` exactly that release. There are no operators or
ranges, and pre-releases (`1.8.0-rc1`) are never selected.

Tool names are looked up in the built-in [registry](./registry). A tool that
is not in the registry — or one you want to fetch from a fork — can be defined
in the project itself:

```toml
[tools.foo]
version = "1.2"

[tools.foo.source]
type = "github_release"
repo = "example/foo"
asset = "foo_{version}_{os}_{arch}.tar.gz"   # {version} has no "v" prefix
bin = ["foo"]                                # executables inside the archive
# tag_prefix = "v"                           # text before the version in tags
# platforms = ["linux/amd64", "darwin/arm64"]
# [tools.foo.source.os]   linux = "unknown-linux-gnu"   # rename {os}
# [tools.foo.source.arch] amd64 = "x86_64"              # rename {arch}
```

A project-local source uses exactly the same model as a registry recipe, so
moving a definition into the registry is a copy.

Supported platforms: `linux/amd64`, `linux/arm64`, `darwin/amd64`,
`darwin/arm64`.

## block.lock

```toml
# This file is generated by block. Do not edit it by hand.

version = 1

[[tools]]
name = "foundry"
constraint = "1.7"
version = "1.7.4"
[tools.source]
type = "github_release"
repo = "foundry-rs/foundry"
asset = "foundry_v{version}_{os}_{arch}.tar.gz"
platforms = ["linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"]
bin = ["forge", "cast", "anvil", "chisel"]

[[tools.artifacts]]
platform = "darwin/arm64"
url = "https://github.com/foundry-rs/foundry/releases/download/v1.7.4/foundry_v1.7.4_darwin_arm64.tar.gz"
sha256 = "…"

[[tools.artifacts]]
platform = "linux/amd64"
url = "https://github.com/foundry-rs/foundry/releases/download/v1.7.4/foundry_v1.7.4_linux_amd64.tar.gz"
sha256 = "…"
```

The lockfile is self-sufficient: CI needs only the `block` binary and this
file. The recipe is copied into it so a later registry change cannot silently
alter what `sync --locked` installs.

Checksums are computed by block when it first downloads an artifact
(trust-on-first-use at `lock` time); every later download, on every machine,
must match.

## CI

```yaml
- uses: actions/checkout@v4
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
- run: block sync --locked
- run: block exec forge test
```

`block sync --locked` fails, without resolving anything, when:

- `block.lock` is missing;
- `block.toml` and `block.lock` disagree (a tool was added, removed, or its
  constraint or project-local source changed);
- `block.lock` has no artifact for the runner's platform;
- a downloaded artifact's SHA-256 does not match.

Set `GITHUB_TOKEN` to raise the GitHub API rate limit for `lock`, `update`
and `outdated`. `sync --locked` does not call the API at all.

Keeping up with upstream is a separate, explicit step — for example a
scheduled job that runs `block update` and opens a pull request with the new
`block.lock`.

## Store and cache

Downloads and installs live under one directory shared by every project:

```text
$BLOCK_HOME/                               default: ~/.local/share/block
  cache/sha256/<digest>                    downloaded archives, content-addressed
  tools/<name>/<version>-<digest12>/       extracted installs
```

Two projects that pin the same artifact share one download and one install.
Caching this directory in CI, keyed by `block.lock`, makes `sync --locked` an
offline operation. `XDG_DATA_HOME` is honored when `BLOCK_HOME` is unset.

## How versions are resolved

```text
git tags of the repository           (GET /repos/{repo}/git/matching-refs/tags/{prefix})
  → drop tags that are not <prefix>MAJOR.MINOR.PATCH    (nightly-…, stable, …)
  → drop pre-releases, apply the constraint
  → newest first: fetch the release for that tag        (GET /repos/{repo}/releases/tags/{tag})
  → skip drafts, pre-release-flagged releases and tags without a release
  → pick the asset named by the recipe for each platform
```

Listing tags instead of paging through `/releases` matters for projects such
as Foundry, whose release list is dominated by hundreds of nightly builds.

## Registry

The built-in registry is a directory of TOML recipes, one per tool, embedded
into the binary:

```toml
# registry/hermes.toml
name = "hermes"

[source]
type = "github_release"
repo = "informalsystems/hermes"
asset = "hermes-v{version}-{arch}-{os}.tar.gz"
platforms = ["linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"]
bin = ["hermes"]

[source.os]
linux = "unknown-linux-gnu"
darwin = "apple-darwin"

[source.arch]
amd64 = "x86_64"
arm64 = "aarch64"
```

A recipe changes only when an upstream renames its assets or moves
repositories. New releases need nothing. Recipes are data, never commands.

Currently registered: `foundry` (`forge`, `cast`, `anvil`, `chisel`) and
`hermes`. The registry is intentionally small; it grows when a recipe is
needed, not to pad a list. Tools that do not publish per-platform archives on
GitHub Releases (for example go-ethereum, whose builds come from a separate
download server keyed by commit hash) are not supported yet.

## Security

- Artifacts are fetched over HTTPS only. Plain HTTP is accepted only for
  loopback addresses so offline test servers can stand in for GitHub.
- Every download is hashed while streaming and must match the SHA-256 in
  `block.lock` before extraction. The cache stores blobs under their digest,
  so a cached file can never be the wrong bytes for its name.
- Archives (`.tar.gz`, `.tgz`, `.zip`) are extracted defensively: absolute
  paths, `..` components, symlinks and hard links are refused; only the
  executable bit is preserved from archive permissions.
- Installs are atomic: extraction happens in a temporary directory that is
  renamed into place only after every entry succeeded and every declared
  executable exists.
- `block exec` never installs. `block sync --locked` never resolves. Nothing
  falls back to an artifact the lockfile does not name.

See [SECURITY.md](./SECURITY.md) for the reporting policy.

### Verifying release integrity

Release archives are signed with cosign (keyless, via GitHub OIDC) and ship
with an SBOM and build provenance:

```shell
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nao1215/block/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

## Non-goals

block deliberately does **not**:

- act as a general package manager (no npm, cargo, pip, apt, brew sources);
- judge protocol compatibility between tools (client/consensus pairs, hard
  forks, chain IDs, RPC capabilities);
- manage wallets, keys, validators, staking or nodes;
- generate Docker Compose files or manage Solidity library dependencies;
- run scripts from the registry.

If a feature would turn block into mise or aqua, it does not belong here.

## Development

```shell
make test       # unit tests with -race
make e2e        # offline end-to-end suite (needs atago)
make lint       # golangci-lint v2
make coverage   # unit + e2e coverage combined into cover.out
```

The E2E suite ([e2e/atago](./e2e/atago)) is the CLI contract: every
user-visible behaviour — output, exit codes, files written, error messages —
is pinned there against the real binary and an offline fake GitHub. See
[CONTRIBUTING.md](./CONTRIBUTING.md).

## License

[MIT](./LICENSE)
