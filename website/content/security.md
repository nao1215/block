---
title: Security
description: "What block guarantees when it downloads and runs third-party binaries on your machine and in CI."
---

block fetches prebuilt binaries from the internet and puts them on your
`PATH`. These are the promises it keeps while doing that.

## Transport

Artifacts are fetched over HTTPS only, and every redirect is held to the same
rule: an `https` URL that redirects to plain `http` is refused rather than
followed. Plain HTTP is accepted only for loopback addresses, so offline test
servers can stand in for GitHub.

## Checksums

Every download is hashed while streaming and must match the SHA-256 in
`block.lock` before anything is extracted. The digest comes from the upstream
when it publishes one — GitHub records a SHA-256 for release assets uploaded
since 2025 — and from the first download otherwise.

The cache is content-addressed, and the name is not taken as proof: a cache
hit is re-hashed before it is used, so a truncated download or a half-restored
CI cache is discarded and fetched again instead of installed.

## Unpacking

Archives are extracted defensively. Absolute paths, `..` components, symlinks
and hard links are refused — also after `strip_components` — and only the
executable bit is preserved from archive permissions. Executable paths are
validated the same way in a recipe and in a lockfile, because a lockfile
arrives through pull requests and hand edits too.

## Installing

Installs are atomic and self-attesting: everything is unpacked into a
temporary directory, every declared executable is confirmed to be present and
runnable, a completion marker is written, and only then is the directory
renamed into place. An install without that marker — or missing one of its
executables — is replaced, never run.

Two tools that provide the same command name are a conflict block reports, not
something it resolves by `PATH` order.

## Boundaries

`sync` never resolves and `exec` never installs. `exec` re-checks offline that
`block.lock` still matches `block.toml`. Nothing falls back to an artifact the
lockfile does not name, and no registry recipe can run a command: a recipe is
data, and there is no `install = "curl … | bash"` escape hatch.

## Verifying a block release

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

## Reporting a vulnerability

Privately, by email to n.chika156@gmail.com or through the repository's
Security tab. See
[SECURITY.md](https://github.com/nao1215/block/blob/main/SECURITY.md).
