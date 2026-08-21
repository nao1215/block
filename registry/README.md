# block registry

One TOML recipe per tool. A recipe is **how to find and fetch** a tool, not a
list of its versions: a new upstream release needs no change here. Recipes
change only when the upstream moves repositories, renames assets, changes how
it distributes builds, or drops a platform.

This directory is embedded into the `block` binary today and is laid out so
it can move to its own repository (`block-registry`) unchanged.

## Order of preference

A recipe states exactly one install method. block executes it
deterministically and never falls back to another at run time. When writing a
recipe, pick the highest method the upstream actually supports:

| priority | method | type | status |
| --- | --- | --- | --- |
| 1 | official prebuilt GitHub Release artifact | `github_release` | implemented |
| 2 | official prebuilt artifact on the upstream's download server | `http` | implemented |
| 3 | official package registry (`go install`, `cargo install`, npm, pipx) | — | not implemented |
| 4 | limited build from official source | — | not implemented |

Every tool in this directory — across Bitcoin, Ethereum, Solana, Cosmos and
IBC — is served by the first two. A third type is added only when a tool
genuinely cannot be obtained with them, and it will be a type whose meaning
and safety boundary block understands. There is no `install = "curl … | bash"`,
no `command = "make install"`, and no arbitrary-script escape hatch, ever.
block does not manage language runtimes (Go, Rust, Node, Python) either.

## Recipe

```toml
name = "hermes"                         # must equal the file name
ecosystem = "ibc"                       # bitcoin, ethereum, solana, cosmos, ibc
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
| `ecosystem`, `description` | metadata about the tool — block attaches no behaviour to either | same |

Placeholders: `{version}` (as the upstream spells it, without the tag
prefix), `{os}`, `{arch}`, `{target}`, and `{commit}` (`http` only — the first
8 hex digits of the commit the version tag points at).

Archives may be `.tar.gz` / `.tgz`, `.tar.bz2` / `.tbz2` or `.zip`. An asset
name without one of those extensions is a single raw executable, installed
under the one name in `bin`.

`target` exists because some upstreams do not name platforms as a product of
OS and architecture: Bitcoin Core writes `aarch64-linux-gnu` but
`arm64-apple-darwin`. Use `os`/`arch` when they suffice, `target` when they
do not.

## Metadata

`ecosystem` and `description` say what a tool *is*; nothing about resolution
depends on them. They exist so that the registry, rather than a README
somewhere, is the one place that answers "what is this tool?" — for `block
list` today and for whatever reads the registry later (a `block-registry`
site, generated documentation).

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

`go test ./registry/` validates every recipe against a table that pins, for
each supported platform, the exact asset name or URL it renders — a typo
fails there rather than at a user's first `block lock`.

The future `block-registry` repository adds a scheduled CI job that, for every
recipe, discovers the newest stable upstream version, resolves the artifact
for each supported platform, verifies the digest, downloads it, checks that
the declared executables exist and probes them (`--version` or equivalent).
Routine version updates never involve a human; a human is needed only when a
recipe breaks — an asset renamed, a repository moved, a distribution method
or a platform dropped.
