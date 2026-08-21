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

And one question answered offline — *what tools does block support?*:

```console
$ block list
NAME      SOURCE           BINARIES
foundry   github_release   forge, cast, anvil, chisel
geth      http             geth
hermes    github_release   hermes
solc      github_release   solc
```

block is not a package manager. It downloads release archives from upstream,
verifies them, and puts them on `PATH` for the command you run. Nothing more.

## Why block

- **Project-local, not machine-global.** Each repository declares its own
  toolchain; two projects on one machine can use different Foundry versions
  without fighting.
- **Lockfile-driven reproducibility.** `block.lock` records the artifact URL
  and checksum for every platform you care about. `sync` installs exactly
  that, or fails.
- **CI is a first-class user.** `block sync` is the same command with the same
  meaning locally and in CI: it never resolves, never rewrites the lockfile,
  and fails loudly on any drift. No special flag.
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

Write `block.toml`:

```toml
[tools]
foundry = "1.7"
hermes = "1.13"
```

```console
$ block lock
downloading https://github.com/foundry-rs/foundry/releases/download/v1.7.4/foundry_v1.7.4_linux_amd64.tar.gz
downloading https://github.com/informalsystems/hermes/releases/download/v1.13.3/hermes-v1.13.3-x86_64-unknown-linux-gnu.tar.gz
foundry  locked 1.7.4
hermes   locked 1.13.3
wrote block.lock

$ block sync
foundry  1.7.4   installed
hermes   1.13.3  installed

$ block exec forge --version
forge Version: 1.7.4-stable
```

Commit both `block.toml` and `block.lock`.

## Commands

| Command | Resolves versions | Writes `block.lock` | Downloads | Installs | Runs your command |
| --- | :-: | :-: | :-: | :-: | :-: |
| `block lock [tool...]` | yes | yes | only artifacts whose upstream publishes no digest | no | no |
| `block lock --check` | yes | **no** | no | no | no |
| `block sync` | **no** | **no** | locked URLs, when not cached | yes | no |
| `block exec <cmd>` | **no** | **no** | **no** | **no** | yes |
| `block list` | **no** | **no** | **no** | **no** | no |

### `block lock`

Resolves every tool in `block.toml` to the newest upstream release its
constraint allows and records the download URL and SHA-256 per platform in
`block.lock`. Running it again later moves pins forward to whatever upstream
has published since — that is the only way a pin ever moves.

```console
$ block lock              # every tool
$ block lock hermes       # re-resolve hermes, keep the other pins
```

Output is one line per tool: `locked 1.7.4` for a new pin, `1.7.4 -> 1.7.5`
for a moved one, `1.7.4` for an unchanged one.

When the upstream publishes a checksum (GitHub records a sha256 for every
release asset uploaded since 2025), `lock` writes it down without
downloading anything. Otherwise it downloads the artifact once to record its
digest (trust on first use); every later download, on every machine, must
match.

### `block lock --check`

Performs the same resolution as `lock` but writes nothing:

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
| 2 | `block.lock` would change (newer release, new or dropped tool, new platform) |
| 1 | error (network, manifest, …) |

Use it in a scheduled job to learn about upstream releases without touching
the repository; run `block lock` and open a pull request when it exits 2.
`--check` stops at version discovery: it never downloads artifacts.

### `block sync`

Installs every artifact `block.lock` names for this machine into the shared
store. It needs the lockfile's exact URLs and nothing else — no registry, no
GitHub API. It fails, without resolving or writing anything, when:

- `block.lock` is missing;
- `block.toml` and `block.lock` disagree: a tool was added or removed, a
  constraint changed, or a project-local source changed;
- `block.lock` has no artifact for this platform;
- a downloaded artifact's SHA-256 does not match.

```console
$ block sync
block: block.lock is stale; run "block lock"
  hermes is declared in block.toml but missing from block.lock
```

`sync` never runs `lock` for you. A pin changes only when you ask.

### `block exec`

Runs a command with every executable from `block.lock` first on `PATH` and
exits with the command's status. Any command works, so build scripts that
call the tools see the locked versions:

```console
$ block exec forge test
$ block exec make test
$ block exec ./scripts/integration-test.sh
```

`exec` never downloads, installs or resolves. If a tool is not installed:

```console
$ block exec forge test
block: foundry 1.7.4 is not installed; run "block sync"
```

### `block list`

Prints the tools in the registry snapshot embedded in this binary: name,
source type and the executables each provides. It is read-only and offline —
no resolution, no network, no `block.toml` — so its output is deterministic
for a given block version, works when GitHub is down, and needs no token.
Project-local tools are not listed; a project's own toolchain is its
`block.toml` and `block.lock`.

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
# strip_components = 1                       # drop a wrapping directory
# [tools.foo.source.os]   linux = "unknown-linux-gnu"   # rename {os}
# [tools.foo.source.arch] amd64 = "x86_64"              # rename {arch}
```

A project-local source uses exactly the same model as a registry recipe, so
moving a definition into the registry is a copy. That is the intended path:
define the tool in your project, use it, and promote it to the registry once
it is useful to more than one project. Nobody waits for a registry merge.

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

Three files, three responsibilities:

| | Answers | Lives in |
| --- | --- | --- |
| registry recipe | *how* to resolve a tool | `registry/*.toml` (embedded) |
| `block.toml` | *what* you want | your repository |
| `block.lock` | *what was decided* | your repository |

The lock holds facts only — exact version, executables, URL and digest per
platform. A project-local source additionally records a fingerprint of its
definition (`source = "sha256:…"`), so editing it makes the pin stale. A
registry recipe change never affects an existing lock: `sync` needs only the
URLs and digests already written down.

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
- run: block sync
- run: block exec forge test
```

There is no CI flag: `block sync` is always a locked operation. `GITHUB_TOKEN`
is only relevant to `block lock`, which calls the GitHub API; `sync` and
`exec` do not.

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

## Registry and source types

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

Not every blockchain CLI is a GitHub Release asset, and block does not stop
there. A recipe names one of the source types below — the most
self-contained, fastest and safest one the upstream allows — and block
executes it deterministically, never falling back to another at run time:

| type | what it does | example |
| --- | --- | --- |
| `github_release` | versions from git tags; download a release asset (archive or a single raw executable); use GitHub's sha256 when recorded | foundry, hermes, solc |
| `http` | versions from git tags; download from the upstream's own HTTPS server; `{commit}` expands to the tagged commit for vendors that name builds by it | geth |

Next in line, each added only when a tool needs it: official GitHub content
or archives, language package registries (`go_install`, `cargo`, `npm`,
`pipx`), and limited known source builds (`go_build`, `cargo_build`). Every
one of them will be a source type whose meaning and safety boundary block
understands. There is no `install = "curl … | bash"` escape hatch and there
never will be.

Currently registered: `foundry` (`forge`, `cast`, `anvil`, `chisel`), `geth`
(Linux only — upstream stopped shipping macOS builds), `hermes` and `solc`.
See [registry/README.md](./registry/README.md) for the recipe schema.

The registry will move to its own repository, `block-registry`, as the
canonical source of recipes. block will still embed a tested snapshot of it
at build time: `block list` and `block lock` read that snapshot and never
fetch the registry at run time, so a block version always pairs with a
registry it was tested against.

## Security

- Artifacts are fetched over HTTPS only. Plain HTTP is accepted only for
  loopback addresses so offline test servers can stand in for GitHub.
- Every download is hashed while streaming and must match the SHA-256 in
  `block.lock` before extraction. The digest comes from the upstream when it
  publishes one (GitHub's per-asset sha256) and from the first download
  otherwise. The cache stores blobs under their digest, so a cached file can
  never be the wrong bytes for its name.
- Archives (`.tar.gz`, `.tgz`, `.zip`) are extracted defensively: absolute
  paths, `..` components, symlinks and hard links are refused — also after
  `strip_components` — and only the executable bit is preserved from archive
  permissions. A raw executable is copied under its one `bin` name.
- Installs are atomic: extraction happens in a temporary directory that is
  renamed into place only after every entry succeeded and every declared
  executable exists.
- `sync` never resolves and `exec` never installs. Nothing falls back to an
  artifact the lockfile does not name.

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
- run scripts from the registry;
- offer shell integration, `block env`, `block add`/`remove`, or any command
  that resolves or installs implicitly.

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
