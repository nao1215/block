# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `block init`, `block lock`, `block sync [--locked]`, `block update [tool...]`,
  `block outdated`, `block exec <command>`, `block registry` and `block version`.
- `block.toml` manifest with dotted-prefix version constraints (`"1"`, `"1.7"`,
  `"1.7.4"`), an optional `platforms` list and project-local
  `[tools.<name>.source]` definitions.
- `block.lock` lockfile recording the exact version, source recipe, and the
  download URL plus SHA-256 of every artifact per platform.
- Built-in registry recipes for Foundry (`forge`, `cast`, `anvil`, `chisel`)
  and Hermes.
- GitHub Releases source: versions are discovered from git tags and artifacts
  from release assets; drafts, pre-releases and non-semver tags are skipped.
- Content-addressed download cache and per-version installs under
  `$BLOCK_HOME`, shared across projects.
- Security: HTTPS-only transport, streaming SHA-256 verification, defensive
  archive extraction (no traversal, no links) and atomic installs.
- Offline atago end-to-end suite driving the real binary against a fake
  GitHub, plus unit tests for every pure package.
