# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `BLK` diagnostic codes on every refusal, a `block explain <code>` lookup,
  and a generated reference page (`doc/errors.md`, published at
  https://nao1215.github.io/block/errors/). The thousands digit says where the
  fix lives; every coded error exits 1.
- `block --help` carries the documentation, error-code, bug-report and
  GitHub Sponsors addresses.
- Three lifecycle commands: `block lock [tool...]` (resolve; `--check` reports without
  writing and exits 2 when the lock would change), `block sync` (install,
  never resolves or writes the lock) and `block exec <command>` (run, never
  installs). Plus `block list [ecosystem]` (the embedded registry snapshot —
  offline and read-only) and `block version`.
- `block.toml` manifest with dotted-prefix version constraints (`"1"`, `"1.7"`,
  `"1.7.4"`), an optional `platforms` list and project-local
  `[tools.<name>.source]` definitions.
- `block.lock` lockfile recording the exact version, executables, a
  fingerprint of a project-local recipe, and the download URL plus SHA-256 of
  every artifact per platform.
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
- A built-in registry of recipes covering the blockchain systems a project is
  likely to need — Bitcoin, Ethereum, Solana, Cosmos and IBC, Celestia,
  Cardano, Aptos, NEAR, Starknet, Stellar, Avalanche, the Internet Computer,
  IPFS, Hyperledger Fabric, ZKsync and zero-knowledge circuits — all served by
  the two implemented source types. `block list` prints what the binary
  actually carries, and [doc/tools.md](./doc/tools.md), generated from the
  recipes, is the same catalogue as a page; neither this file nor the README
  keeps a second copy that could fall behind.
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
  exactly as much as `block exec` does: nothing. Outside a project, or for a
  command a project does not lock, it runs the next command of that name on
  `PATH`.
- Windows is a supported platform: `windows/amd64` and `windows/arm64` join
  the platform model, block itself ships Windows builds, and the shims are
  placed with hard links or copies rather than symlinks so no Developer Mode
  or elevation is needed. `$BLOCK_HOME` defaults to `%LOCALAPPDATA%\block`
  there. Which tools are installable is still the upstream's decision, and the
  registry records it per tool — see the Platforms column in
  [doc/tools.md](./doc/tools.md). block reports a platform an upstream does
  not ship for rather than substituting something else.
- Content-addressed download cache and per-version installs under
  `$BLOCK_HOME`, shared across projects.
- Security: HTTPS-only transport across redirects, streaming SHA-256
  verification, re-hashed cache hits, defensive archive extraction (no
  traversal, no links), executable paths validated identically in recipes and
  lockfiles, atomic installs marked complete only when every declared
  executable is present and runnable, a version alphabet closed tightly enough
  that a hand-edited `block.lock` cannot name a path outside `$BLOCK_HOME`, a
  check that a pinned version satisfies the constraint it was resolved from,
  and a refusal to let one command name mean two executables — across tools or
  within one, compared without regard to case on every platform.
- Offline atago end-to-end suite driving the real binary against a fake
  GitHub, plus unit tests for every pure package.
- `make registry-live` and the scheduled *Registry (live)* workflow check
  every recipe against the real upstreams — newest stable version, artifact
  per platform, checksum, unpack, and a probe of every declared executable —
  so routine upstream releases need no human attention.
