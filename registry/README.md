# block registry

One TOML recipe per tool. A recipe is **how to find and fetch** a tool, not a
list of its versions: a new upstream release needs no change here. Recipes
change only when the upstream moves repositories, renames assets, changes how
it distributes builds, or drops a platform.

This directory is a snapshot of
[block-registry](https://github.com/nao1215/block-registry), which is where
recipes are written and reviewed. The snapshot is embedded into the `block`
binary, so `block list` and `block lock` work offline and a block version
always pairs with a registry it was tested against. Edit a recipe here only
as part of syncing the snapshot; the canonical copy lives there.

## Where a recipe may download from

A recipe states exactly one download source. block executes it
deterministically and never falls back to another at run time. Which sources
are allowed is a rule block-registry writes down and its linter enforces, not
a matter of taste:

| tier | source | type | what bounds it |
| --- | --- | --- | --- |
| 1 | a GitHub Release asset of the repository the recipe already names | `github_release` | the artifact and the version tag come from the same project, and GitHub publishes the asset's SHA-256 |
| 2 | a prebuilt artifact on the upstream's own download server | `http` | the host must be listed for that repository in block-registry's `policy/hosts.toml`, with the reason a release asset will not do |

Today three recipes need tier 2 — `bitcoin-core`, `geth` and `geth-tools` —
because those projects build binaries and publish them on their own server
rather than attaching them to a GitHub release. Everything else is tier 1.

There is no tier 3. No `install = "curl … | bash"`, no
`command = "make install"`, no package-manager shell-out, and no
arbitrary-script escape hatch, ever. block does not manage language runtimes
(Go, Rust, Node, Python) either, so a tool distributed only through npm, PyPI
or crates.io is not here.

## Recipe

```toml
name = "hermes"                         # must equal the file name
ecosystems = ["cosmos", "ibc"]          # the blockchain systems it serves
description = "IBC relayer connecting Cosmos SDK chains, written in Rust"

[source]
type = "github_release"                 # or "http"
repo = "informalsystems/hermes"         # versions come from this repo's tags
# tag_prefix = "v"                      # text before MAJOR.MINOR[.PATCH] in tags
asset = "hermes-v{version}-{arch}-{os}.tar.gz"
platforms = ["linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"]
bin = ["hermes"]                        # executables, relative to the archive root

[source.os]                             # rename {os} (Go's GOOS) for the upstream
linux = "unknown-linux-gnu"
darwin = "apple-darwin"

[source.arch]                           # rename {arch} (Go's GOARCH)
amd64 = "x86_64"
arm64 = "aarch64"
```

| field | `github_release` | `http` |
| --- | --- | --- |
| `repo` | tags and release assets | tags (and the tagged commit) |
| `asset` | asset file name template | — |
| `url` | — | HTTPS URL template |
| `strip_components` | leading path components to drop when unpacking | same |
| `bin` | executables inside the archive; for a raw-executable asset, the one name to install it under | same |
| `platforms` | `os/arch` pairs the upstream ships; empty means all four, or the keys of `target` | same |
| `os`, `arch` | rename `{os}` / `{arch}` | same |
| `target` | `os/arch` → the upstream's whole platform string, for `{target}` | same |
| `ecosystems`, `description` | metadata about the tool — block attaches no behaviour to either | same |

Placeholders: `{version}` (as the upstream spells it, without the tag
prefix), `{os}`, `{arch}`, `{target}`, and `{commit}` — the first 8 hex digits
of the commit the version tag points at, for upstreams that stamp the build
commit into the artifact's name (vyper, Nimbus, go-ethereum). Resolving one
costs an extra API call, so use it only where the upstream leaves no choice.

Archives may be `.tar.gz` / `.tgz`, `.tar.bz2` / `.tbz2` or `.zip`. An asset
name without one of those extensions is a single raw executable, installed
under the one name in `bin`.

`target` exists because some upstreams do not name platforms as a product of
OS and architecture: Bitcoin Core writes `aarch64-linux-gnu` but
`arm64-apple-darwin`. Use `os`/`arch` when they suffice, `target` when they
do not.

## Metadata

`ecosystems` and `description` say what a tool *is*; nothing about resolution
depends on them. They exist so that the registry, rather than a README
somewhere, is the one place that answers "what is this tool?" and "what can I
use for this chain?" — `block list` prints both, and whatever reads the
registry later (a `block-registry` site, generated documentation) has them
too.

`ecosystems` is a required, non-empty list of canonical names. `block list`
with no argument prints the ones in use; adding another means writing it in a
recipe and nothing else, because block derives the available names from the
snapshot rather than hard-coding them. A tool may
serve several systems — Hermes is used from both `cosmos` and `ibc` work —
and is then listed under each. Keep the names lower-case: they are what users
type after `block list`.

The metadata is for discovery, classification and display only. block never
uses it to select tools, install them, judge compatibility, resolve
dependencies or generate a default toolchain: `block list ethereum` shows the
candidates, and the human writes `block.toml`.

A description is required, and is one plain sentence under 100 characters,
with no leading or trailing whitespace and no line breaks. Write it so it
reads on its own beside the tool's name — "Cosmos Hub node (gaiad)", not
"This is the node for the Cosmos Hub". Say what the tool is, not how good it
is: upstream marketing ("blazing fast", "the best place to…") does not
belong here.

## Version discovery

Versions always come from the repository's git tags, never from a list kept
here:

```text
tags → strip tag_prefix → keep what parses as a version → drop pre-releases
     → apply the manifest constraint → newest first → resolve the artifact
```

Listing tags rather than paging `/releases` matters for upstreams such as
Foundry, whose release list is dominated by nightly builds.

## Checks

Two layers, deliberately separate.

**Static, on every push.** `go test ./registry/` validates every recipe
against a table that pins, for each supported platform, the exact asset name
or URL it renders, plus its description and executables. A typo fails there
rather than at a user's first `block lock`. It touches no network.

**Live, on a schedule.** `make registry-live` (the
[Registry (live)](../.github/workflows/registry-live.yml) workflow, weekly and
on demand) takes each recipe to the real upstream: it discovers the newest
stable version the way block does, resolves the artifact for every declared
platform and confirms it exists, then downloads the one for the runner,
verifies its checksum, unpacks it, checks that every declared executable is
there, and runs each one (`--version`, `version`, `-version`, then `--help`
for the tools that have no version to report). Limit it while working on a
recipe:

```shell
make registry-live RECIPE=foundry
```

It downloads real artifacts and calls the GitHub API, so it is never mixed
into the unit or E2E suites, and transient upstream failures are retried
rather than reported as a broken recipe.

Routine upstream versions never involve a human. A human is needed only when
the live check fails — an asset renamed, a repository moved, a distribution
method or a platform dropped.
