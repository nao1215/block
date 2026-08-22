# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- An archive whose members link to each other could not be installed at all.
  Every link was refused, and real distributions rely on them — Nethermind
  ships `Nethermind.Runner` pointing at the executable beside it, and a
  versioned shared library is almost always a name and a link. A link is now
  extracted when what it points at resolves inside the install directory,
  and refused when it does not: an absolute target, a Windows path, a
  climb out with `..`, or a hard link to a member the archive never wrote.
  Where the filesystem will not make the link — Windows, without the
  privilege block will not ask for — the target is copied instead.

### Added
- `grandine`, the Rust Ethereum consensus client, is in the registry: 48
  tools now. Its releases are one raw executable per platform with a
  published SHA-256, so locking it downloads nothing.
- Tests for the guards a mutation audit found nothing was holding: a store
  path component carrying a NUL, a pin kept although the project-local
  recipe beside it changed, a release with two assets of one name, two
  executables whose command names differ only in case, a diagnostic code
  registered twice, and the depth guard that stops two shim directories on
  PATH from handing a command back and forth.

### Fixed
- Two `block sync` runs at once — two projects, or a person and a build
  script — failed each other. The shim directory is global, and whoever lost
  the race to create a file got "file exists", both wrote the marker through
  one shared temporary name so one renamed a path the other had already
  moved, and a rebuild emptied the directory first, so a command the other
  project had just put there disappeared. Each shim is now built at a unique
  path and renamed into place, which is atomic: both runs finish and no
  command is ever missing in between.
- A rebuild of the shim directory no longer deletes, or turns into a shim, a
  file block did not put there. The marker records which commands the
  directory serves (`block-shims 3`), so a rebuild recreates exactly those; a
  marker from an older block is still recovered by looking at the directory.
- `block lock` asked GitHub for a second page of tags it could not have.
  The matching-refs endpoint answers a repository with many tags by returning
  all of them at once whatever `per_page` says — nine thousand refs, nearly
  four megabytes, for one such repository — and block only learned that from
  a second identical request. A response longer than the page asked for now
  ends the walk, halving the API calls for every repository with more than a
  hundred tags.
- A response larger than the 8 MiB block reads was reported as an invalid
  response from the API. It now says it is block's own limit.

## [0.2.0] - 2026-08-22

A bug-fix release. Three of these change what block accepts or says, which is
why it is a minor rather than a patch: a lockfile carrying a version like
"1.7.4-" is now refused, a recipe whose `{target}` table is incomplete is now
refused, and a project whose `platforms` list excludes the machine running
`sync` gets a different diagnostic code.

### Fixed
- An upstream can publish two tags that are one version to semver — "1.2" and
  "1.2.0", or a release and the same release carrying build metadata. Which of
  them `block lock` pinned depended on the order GitHub answered in, so the
  same manifest against the same upstream could produce two different
  lockfiles. Ordering is now total, and the plain spelling is the one pinned.
- `block explain +1005` and `-1005` printed a code; only the spellings block
  prints — `BLK1005`, `blk1005`, `1005` — are accepted now.
- An empty `argv[0]` made block run as a shim for a command called `.`: the
  guard against it never fired, because `filepath.Base("")` is `"."`. An
  invocation block cannot identify is block itself.
- `block sync` and `block exec` told a reader whose `block.toml` declares a
  `platforms` list that excludes their machine that `block.lock` was stale
  and to run `block lock` — a loop, since lock resolves exactly the platforms
  the manifest names and would write the same file back. They now report
  BLK1006 and name the edit that fixes it. Where re-running lock does help,
  the advice is unchanged.
- Every command run through `block exec` or a shim inherited a PATH ending in
  a separator when the environment's own PATH was empty. An empty PATH entry
  means the current directory to execvp, to the shell and to most tools, so
  the project's working directory was on the search path of every command the
  toolchain ran.
- `block lock` dropped the pin of a tool removed from `block.toml` without
  saying so; only `--check` named it. Both now print the dropped pin, in the
  same aligned column as the tools that were kept.
- A recipe whose artifact template uses `{target}` is now refused unless
  `[source.target]` names every platform it claims to ship for. One that did
  not resolved to "unsupported platform linux/arm64 (available: …,
  linux/arm64, …)", an error contradicting itself.
- Archive extraction is bounded as a whole (8 GiB, 200000 entries) and not
  only per file, which is what the per-file cap alone could not do: many
  small members fill a disk as well as one enormous one.
- A version ending in a bare hyphen (`1.7.4-`) parsed as the release before
  it while keeping the hyphen in its spelling, so a hand-edited `block.lock`
  could pin it and install into a `1.7.4--…` directory. It is now refused
  as an empty pre-release, the way an empty build field already was.
- After a block upgrade, rebuilding the global shim directory recreated only
  the commands of the project that noticed the upgrade; every other
  project's shims disappeared until that project synced again. The rebuild
  now recreates every command the directory already served.
- A download whose digest did not match the lockfile removed the blob it
  hashed to from the cache even when that blob was already there, verified,
  as another tool's artifact. Only a freshly written blob is discarded.
- A GitHub asset digest that is not 64 hex characters is no longer copied
  into `block.lock` — which would then have refused to read it — but
  treated as absent, so the artifact is downloaded and hashed instead.
- An archive member whose name carries a NUL byte is refused by name, with
  the member named in the error, instead of failing with the operating
  system's "invalid argument".
- A repository in a project-local source is checked for the characters
  GitHub allows in an owner or a name, so a typo is reported at the manifest
  instead of surfacing as a "not found" from the wrong API endpoint.

### Added
- Fuzz tests for the version constraint parser, the lockfile parse/marshal
  round trip, the manifest parser, archive member path resolution and
  lockfile `bin` entries. Their seed corpora run as ordinary unit tests.

### Changed
- The README, the documentation front page and the site tagline say what
  block is heading toward — a Blockchain Development Toolbox for local
  blockchain development — and that the toolchain is the only tool in it
  today. Nothing about what block does changed.

## [0.1.0] - 2026-08-22

The first release. Everything below is new, so this entry describes what block
is rather than what changed.

### Added
- Three lifecycle commands: `block lock [tool...]` (resolve; `--check` reports
  without writing and exits 2 when the lock would change), `block sync`
  (install, never resolves or writes the lock) and `block exec <command>` (run,
  never installs). Beside them, three that only read: `block list [ecosystem]`
  (the embedded registry snapshot — offline, no `block.toml`, no token),
  `block explain <code>` (what a `BLK` diagnostic code means) and
  `block version` (also `block --version`, which prints the same two lines from
  the same code).
- `block.toml` manifest with dotted-prefix version constraints (`"1"`, `"1.7"`,
  `"1.7.4"`), an optional `platforms` list and project-local
  `[tools.<name>.source]` definitions.
- `block.lock` lockfile recording the exact version, executables, a
  fingerprint of a project-local recipe, and the download URL plus SHA-256 of
  every artifact per platform. The same manifest resolved against the same
  upstream produces the same bytes, whatever order the tools are written in.
- `block lock --check` compares the whole prospective lockfile, not just
  versions: a changed constraint, executables, unpack depth, project-local
  source or artifact URL is reported by name even when the resolved version
  is identical. It writes nothing and downloads nothing, and exits 2 when
  block.lock would change.
- `block lock` re-evaluates artifacts against the current registry recipe, so
  a recipe fix reaches the lockfile at an unchanged upstream version, while
  `block sync` keeps installing from the URLs and digests already locked. A
  recorded digest is reused whenever the URL is unchanged.
- `block exec` verifies offline that `block.lock` still matches `block.toml`
  and that the install is intact before running anything, and forwards
  `SIGINT` and `SIGTERM` to the child, reporting the child's exit status
  (`128+signal` when a signal ended it).
- A built-in registry of 47 recipes across 17 blockchain systems — Bitcoin,
  Ethereum, Solana, Cosmos and IBC, Celestia, Cardano, Aptos, NEAR, Starknet,
  Stellar, Avalanche, the Internet Computer, IPFS, Hyperledger Fabric, ZKsync
  and zero-knowledge circuits — all served by the two implemented source types.
  `block list` prints what the binary actually carries, and
  [doc/tools.md](./doc/tools.md), generated from the recipes, is the same
  catalogue as a page; neither this file nor the README keeps a second copy
  that could fall behind. The recipes are a vendored snapshot of
  [block-registry](https://github.com/nao1215/block-registry), and
  `block version` says which revision a binary carries.
- `block list` says what each tool is and which blockchain systems it serves;
  `block list <ecosystem>` narrows to one system and shows the commands each
  tool provides. A tool may serve several systems and is listed under each;
  an unknown name reports the ones that exist. Every recipe carries a
  required `ecosystems` list and a one-sentence `description`, both validated
  by the registry tests. The metadata is discovery only: block never derives
  a toolchain from an ecosystem.
- Source types: `github_release` (versions from git tags, artifacts from
  release assets — `.tar.gz`, `.tar.bz2` or `.zip` archives, or a single raw
  executable — using GitHub's per-asset sha256 when recorded) and `http`
  (prebuilt artifacts on the upstream's own server). `{commit}` covers
  vendors that name builds by the tagged commit, `{target}` those whose
  platform strings are not a product of OS and architecture, and
  `strip_components` unwraps versioned archive directories. Versions may be
  two-component (`29.0`) with a bare pre-release suffix (`29.1rc1`), as
  Bitcoin Core tags them. Drafts, pre-releases and unparsable tags are
  skipped.
- Shims: `block sync` puts one file per command in `$BLOCK_HOME/shims`, so
  that adding that directory to `PATH` once makes `forge`, `cast`, `geth` and
  the rest run the version the working directory's project locked. Each shim
  is the block binary under another name and resolves the project per
  invocation, so there is nothing per-project to generate, no shell hook, and
  nothing written to startup files. A shim resolves, downloads and installs
  exactly as much as `block exec` does: nothing, and the two are asserted to
  answer identically — same version, same exit status, same refusal. Outside a
  project, or for a command a project does not lock, it runs the next command
  of that name on `PATH`.
- Windows is a supported platform: `windows/amd64` and `windows/arm64` join
  the platform model, block itself ships Windows builds, and the shims are
  placed with hard links or copies rather than symlinks so no Developer Mode
  or elevation is needed. `$BLOCK_HOME` defaults to `%LOCALAPPDATA%\block`
  there. Which tools are installable is still the upstream's decision, and the
  registry records it per tool — see the Platforms column in
  [doc/tools.md](./doc/tools.md). block reports a platform an upstream does
  not ship for rather than substituting something else.
- The registry records what an upstream actually publishes rather than what it
  names: `circom` and `ethdo` publish one macOS build each, called `amd64` and
  containing an arm64 binary, so they are offered to Apple Silicon and refused
  on Intel instead of installing something that will not start.
- Content-addressed download cache and per-version installs under
  `$BLOCK_HOME`, shared across projects.
- `BLK` diagnostic codes on every refusal, a `block explain <code>` lookup and
  a generated reference page (`doc/errors.md`, published at
  https://nao1215.github.io/block/errors/). The thousands digit says where the
  fix lives — the project's own files, resolution, the download, the install,
  running a command, a refusal on security grounds. Every coded error exits 1;
  exit 2 stays what it was, `block lock --check` reporting that the lockfile
  would change.
- Security: HTTPS-only transport across redirects, streaming SHA-256
  verification, re-hashed cache hits, and defensive archive extraction — no
  traversal, no symbolic or hard links, no device nodes, no member naming a
  Windows drive or a UNC share, and no two members writing one file, all
  refused on every platform rather than only where the name would have meant
  something. Executable paths are validated identically in recipes and
  lockfiles, installs are atomic and marked complete only when every declared
  executable is present and runnable, the version alphabet is closed tightly
  enough that a hand-edited `block.lock` cannot name a path outside
  `$BLOCK_HOME`, a pinned version must satisfy the constraint it was resolved
  from, a release publishing one asset name twice is refused rather than
  resolved by picking one, and one command name may not mean two executables —
  across tools or within one, compared without regard to case everywhere. A
  recipe is data: there is no key that means "run this", in the registry or in
  a project's own source table.
- Offline atago end-to-end suite driving the real binary against a fake
  GitHub, plus unit tests for every pure package. The suite runs on Linux,
  macOS and Windows, and skips a scenario only where the thing it asserts does
  not exist on that platform.
- `make registry-live` and the scheduled *Registry (live)* workflow check
  every recipe against the real upstreams — newest stable version, artifact
  per platform, checksum, unpack, and a probe of every declared executable —
  on one runner per platform block supports, so routine upstream releases need
  no human attention.
- `make docs-smoke` and the scheduled *Documentation smoke* workflow read the
  quickstarts out of README.md and the website's front page and run them for
  real on a clean machine, so a promise that stops working fails in CI rather
  than in front of a reader.

[Unreleased]: https://github.com/nao1215/block/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/nao1215/block/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/nao1215/block/releases/tag/v0.1.0
