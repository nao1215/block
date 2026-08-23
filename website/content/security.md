---
title: Security
description: "What block does and does not guarantee when it installs and runs third-party binaries on your machine and in CI."
toc: true
---

block fetches prebuilt binaries from the internet and puts them on your
`PATH`. These are the promises it keeps while doing that, and the ones it does
not make.

## What block installs

block installs artifacts an upstream has already built and published: a GitHub
release asset, or a prebuilt archive on the vendor's own download server. It
does not build a tool from source. `go install`, `cargo install`, `go build`,
`cargo build`, `make`, `cmake`, `forge build` and any other build or install
script an upstream documents are outside what a recipe can ask for, because a
recipe is data and has no field to put a command in.
[Reference](/reference/#registry-and-source-types) has the two source types and
what each may point at.

That is a boundary block draws on purpose rather than a gap in the recipes.
Building from source makes the source tree, the dependency resolver, every
transitive dependency, the build script, the compiler and runtime versions, the
native libraries and the system toolchain part of what runs while a tool is
installed, with network access throughout. Leaving all of it out keeps
installation to one shape, which is the shape the rest of this page is about:

```text
published artifact -> resolve -> download -> digest check -> block.lock -> sync
```

Fewer inputs at install time is worth more in this domain than in most. A
blockchain repository's CI and its developers' machines often hold signing
keys, deployer credentials, RPC keys and cloud tokens, and a tool installation
runs with all of that within reach. block would rather install fewer kinds of
thing than run more code there. General-purpose managers such as
[mise](https://mise.jdx.dev/) and [aqua](https://aquaproj.github.io/) support a
wider set of installation sources, including source builds, which is what a
general-purpose manager is for; block's set is narrower, for a narrower
catalogue.

What this does not claim: a digest says the bytes are the bytes that were
locked, not that they are safe, and not that whoever published them is who they
say they are. An upstream whose account is taken over can publish a malicious
release, and block will pin that release like any other. What `block.lock`
rules out is the artifact changing underneath a lockfile that did not: the same
lockfile installs the same bytes on every machine, and anything else is a
refusal rather than a substitution.

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
