---
title: Security
description: "What block guarantees when it downloads and runs third-party binaries on your machine and in CI."
toc: true
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

One command name may not mean two executables — across tools or inside one.
Which of them runs would otherwise depend on how it was called: a shim
resolves a command through the lockfile, `PATH` resolves it by directory
order, and the two can disagree. Names are compared without regard to case on
every platform, because Windows resolves `PATH` that way and a lockfile is
committed and read everywhere; block refuses the ambiguity where it is
written rather than where it happens to break.

## Boundaries

`sync` never resolves and `exec` never installs. `exec` re-checks offline that
`block.lock` still matches `block.toml`. Nothing falls back to an artifact the
lockfile does not name, and no registry recipe can run a command: a recipe is
data, and there is no `install = "curl … | bash"` escape hatch.

## Verifying a block release

Every published artifact — the archives and the `.deb`, `.rpm` and `.apk`
packages — is listed in `checksums.txt`, ships with an SPDX SBOM beside it,
and carries SLSA build provenance attested through GitHub OIDC.

`checksums.txt` is the file cosign signs, keyless, and it is what the
signature covers the rest through: verify the signature over the list, then
check your download against the line that names it. Build provenance is
attested per artifact, so `gh attestation verify` works directly on whichever
file you downloaded.


```shell
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nao1215/block/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

## Reading a refusal

Every refusal on this page carries a code, and the ones in this section are the
`BLK6xxx` family: block declining, rather than failing. `BLK6001` is a URL it
will not fetch, `BLK6002` an entry that would be written outside its directory,
`BLK6003` a link or a device node in an archive, `BLK6004` a member larger than
block will extract, `BLK6005` a name or version from `block.lock` that is not a
path component.

```shell
block explain BLK6002
```

Every code, and what to do about each, is at [Error codes](/errors/).

## Reporting a vulnerability

Privately, by email to n.chika156@gmail.com or through the repository's
Security tab. See
[SECURITY.md](https://github.com/nao1215/block/blob/main/SECURITY.md).
