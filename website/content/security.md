---
title: Security
description: "What block does and does not guarantee when it installs and runs third-party binaries on your machine and in CI."
toc: true
---

block fetches prebuilt binaries from the internet and puts them on your
`PATH`. This page lists what it guarantees while doing that, and what it does
not.

## What block installs

block installs artifacts an upstream has already built and published: a GitHub
release asset, or a prebuilt archive on the vendor's own download server. It
does not build a tool from source. `go install`, `cargo install`, `go build`,
`cargo build`, `make`, `cmake`, `forge build` and any other build or install
script an upstream documents are outside what a recipe can ask for, because a
recipe is data and has no field to put a command in.
[Reference](/reference/#registry-and-source-types) has the two source types and
what each may point at.

This is deliberate. Building from source would make the source tree, the
dependency resolver, every transitive dependency, the build script, the
compiler and runtime versions, the native libraries and the system toolchain
part of what runs during an install, with network access throughout. Without
them, every installation has one shape:

```text
published artifact -> resolve -> download -> digest check -> block.lock -> sync
```

A blockchain repository's CI and its developers' machines often hold signing
keys, deployer credentials, RPC keys and cloud tokens, and a tool installation
runs with all of that within reach, so block installs fewer kinds of thing
instead of running more code there. General-purpose managers such as
[mise](https://mise.jdx.dev/) and [aqua](https://aquaproj.github.io/) support
more installation sources, including source builds, for a wider catalogue.

A digest only says the bytes match what was locked. It does not say they are
safe or that the publisher is who they claim to be. An upstream whose account
is taken over can publish a malicious release, and block will pin it like any
other. What `block.lock` rules out is the artifact changing while the lockfile
stays the same: one lockfile installs the same bytes on every machine, and any
mismatch is refused.

## Transport

Artifacts are fetched over HTTPS only, and every redirect is held to the same
rule: block refuses an `https` URL that redirects to plain `http`. Plain HTTP is accepted only for loopback addresses, so offline test
servers can stand in for GitHub.

## Checksums

Every download is hashed while streaming and must match the SHA-256 in
`block.lock` before anything is extracted. The digest comes from the upstream
when it publishes one (GitHub records a SHA-256 for release assets uploaded
since 2025), and from the first download otherwise.

The cache is content-addressed, and a cache hit is re-hashed before it is used,
so a truncated download or a half-restored CI cache is discarded and fetched
again.

## Unpacking

block refuses absolute paths, `..` components, symlinks and hard links in an
archive, including after `strip_components`, and keeps only the executable bit
from archive permissions. Executable paths are
validated the same way in a recipe and in a lockfile, because a lockfile
arrives through pull requests and hand edits too.

## Installing

Installs are atomic. block unpacks into a temporary directory, confirms every
declared executable is present and runnable, writes a completion marker, and
only then renames the directory into place. An install without that marker, or
missing one of its executables, is replaced and never run.

One command name may not map to two executables, across tools or inside one.
Otherwise which one runs would depend on how it was called: a shim resolves a
command through the lockfile, `PATH` resolves it by directory order, and the
two can disagree. Names are compared case-insensitively on every platform,
because Windows resolves `PATH` that way and a lockfile is read everywhere.
block refuses the clash when the lockfile is written.

## Boundaries

`sync` never resolves and `exec` never installs. `exec` re-checks offline that
`block.lock` still matches `block.toml`. Nothing falls back to an artifact the
lockfile does not name, and no registry recipe can run a command: a recipe is
data, and there is no `install = "curl … | bash"` escape hatch.

## Verifying a block release

Every published artifact is listed in `checksums.txt`, which cosign signs
keyless, and carries an SPDX SBOM and SLSA build provenance. Verify the
signature, then check your download against its line.
[Install](/install/#verify-what-you-downloaded) also covers
`gh attestation verify`.

```shell
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp 'https://github.com/nao1215/block/.github/workflows/release.yml@refs/tags/v.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

Releases from v0.7.3 on also attach the provenance as `multiple.intoto.jsonl`,
which [slsa-verifier](https://github.com/slsa-framework/slsa-verifier) checks
an archive against. Replace 0.8.0 with the version you downloaded:

```shell
slsa-verifier verify-artifact block_0.8.0_linux_amd64.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/nao1215/block \
  --source-tag v0.8.0
```

## Reading a refusal

Every refusal on this page carries a code in the `BLK6xxx` family, which block
uses when it declines to do something. `BLK6001` is a URL it
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
