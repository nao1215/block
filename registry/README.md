# block registry

One TOML recipe per tool. A recipe is **how to find and fetch** a tool, not a
list of its versions: a new upstream release needs no change here. Recipes
change only when the upstream moves repositories, renames assets, or changes
how it distributes builds.

This directory is embedded into the `block` binary today and is laid out so
it can move to its own repository (`block-registry`) unchanged.

## Recipe

```toml
name = "hermes"                         # must equal the file name

[source]
type = "github_release"                 # or "http"
repo = "informalsystems/hermes"         # versions come from this repo's tags
# tag_prefix = "v"                      # text before MAJOR.MINOR.PATCH in tags
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
| `url` | — | HTTPS URL template; may use `{commit}` (first 8 hex digits of the tagged commit) |
| `strip_components` | leading path components to drop on extraction | same |
| `bin` | executables inside the archive; for a raw executable asset, the one name to install it under | same |
| `platforms` | `os/arch` pairs the upstream ships; empty means all four | same |

Placeholders: `{version}` (no `v`), `{os}`, `{arch}`, `{commit}` (http only).
An `asset` without an archive extension (`.tar.gz`, `.tgz`, `.zip`) is a
single raw executable.

## Source types and their order of preference

block picks the most self-contained, fastest and safest way to obtain a tool
and states it in the recipe. block executes the recipe deterministically; it
never falls back from one type to another at run time.

1. `github_release` — official prebuilt GitHub Release asset (Foundry, Hermes,
   solc). GitHub's own sha256 for the asset is used when it exists, so `lock`
   needs no download; otherwise the digest is recorded on first download.
2. `http` — official prebuilt artifact on the upstream's own download server
   (go-ethereum). The digest is recorded on first download.
3. *(not yet implemented)* official GitHub content / archive.
4. *(not yet implemented)* language package registries: `go_install`,
   `cargo`, `npm`, `pipx`.
5. *(not yet implemented)* limited, known source builds: `go_build`,
   `cargo_build`.

Types 3–5 are added when a blockchain CLI actually needs them, one at a time,
and each is implemented as a source type whose meaning and safety boundary
block understands. There is no `install = "curl ... | bash"` and there never
will be: a recipe is data, never a command.

## Checks

`go test ./registry/` validates every recipe (schema, file name, asset
rendering). The future `block-registry` repository adds a scheduled CI job
that applies each recipe to the newest upstream release, downloads the
artifact, confirms the executables exist and runs a minimal probe such as
`--version`, so that "recipe still works" is verified continuously and a
human is only needed when an upstream changes its distribution.
