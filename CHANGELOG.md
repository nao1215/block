# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Three commands: `block lock [tool...]` (resolve; `--check` reports without
  writing and exits 2 when the lock would change), `block sync` (install,
  never resolves or writes the lock) and `block exec <command>` (run, never
  installs). Plus `block list` (the embedded registry snapshot: name, source
  type, executables — offline and read-only) and `block version`.
- `block.toml` manifest with dotted-prefix version constraints (`"1"`, `"1.7"`,
  `"1.7.4"`), an optional `platforms` list and project-local
  `[tools.<name>.source]` definitions.
- `block.lock` lockfile recording the exact version, executables, a
  fingerprint of the recipe, and the download URL plus SHA-256 of every
  artifact per platform.
- Built-in registry recipes across five ecosystems, all served by the two
  implemented source types: Bitcoin Core; Foundry, solc, geth, reth and
  Lighthouse; Agave, Anchor and Surfpool; Gaia, CometBFT and Osmosis; Hermes.
- `block list` shows each tool's ecosystem alongside its source type and
  the commands it provides.
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
- Content-addressed download cache and per-version installs under
  `$BLOCK_HOME`, shared across projects.
- Security: HTTPS-only transport, streaming SHA-256 verification, defensive
  archive extraction (no traversal, no links) and atomic installs.
- Offline atago end-to-end suite driving the real binary against a fake
  GitHub, plus unit tests for every pure package.
