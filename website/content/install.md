---
title: Install
description: Install block with go install, Homebrew, Scoop, a prebuilt .deb/.rpm/.apk package or a release archive, put the shims on PATH, and verify the release signatures.
toc: true
---

block is a single static binary with no runtime dependencies. It does not need
Go, Docker, or a package manager to do its job — those are only ways to get the
binary itself.

## go install

```shell
go install github.com/nao1215/block@latest
```

Building from source needs Go 1.25 or newer. On an older Go, take a prebuilt
binary or a package below.

## Package managers

macOS and Linux, through Homebrew:

```shell
brew install --cask nao1215/tap/block
```

Windows, through Scoop:

```shell
scoop bucket add nao1215 https://github.com/nao1215/block
scoop install nao1215/block
```

## Prebuilt packages and binaries

The [releases page](https://github.com/nao1215/block/releases) carries `.deb`,
`.rpm` and `.apk` packages plus archives for Linux, macOS and Windows on both
x86-64 and arm64.

```shell
curl -sSfL https://github.com/nao1215/block/releases/download/v0.1.0/block_0.1.0_linux_amd64.tar.gz | tar xz
sudo install -m 0755 block /usr/local/bin/block
```

## In GitHub Actions

```yaml
- uses: nao1215/setup-block@v0
  with:
    sync: "true"
```

[setup-block](https://nao1215.github.io/setup-block/) installs the CLI,
verifies its checksum, exports `$BLOCK_HOME` and caches the store on your
`block.lock`. See [CI](/ci/) for runners that are not GitHub Actions.

## Put the shims on PATH

Optional, and done once. `block sync` writes one file per command into
`$BLOCK_HOME/shims`; with that directory on `PATH`, `forge` and friends run as
themselves and follow whichever project you are standing in.

```shell
# Unix, in ~/.bashrc or ~/.zshrc
export PATH="$HOME/.local/share/block/shims:$PATH"
```

```powershell
# Windows, once
[Environment]::SetEnvironmentVariable(
  "Path", "$env:LOCALAPPDATA\block\shims;$env:Path", "User")
```

block writes nothing to your startup files and installs no shell hook. Skipping
this step costs you nothing but four extra characters: `block exec forge test`.

## Check what you got

```shell
block version
```

```text
block v0.1.0
registry 40fe35d71da6 (47 recipes from https://github.com/nao1215/block-registry)
```

The second line is the [block-registry](https://github.com/nao1215/block-registry)
revision whose recipes are compiled into that binary, so a resolution can always
be traced back to a reviewed recipe.

## Verify what you downloaded

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
  --certificate-identity-regexp 'https://github.com/nao1215/block/\.github/workflows/release\.yml@refs/tags/.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

```shell
gh attestation verify block_0.1.0_linux_amd64.tar.gz --repo nao1215/block
```

## Uninstall

Remove the binary the way you installed it, then delete the store:

```shell
rm -rf ~/.local/share/block          # %LOCALAPPDATA%\block on Windows
```

Nothing else on your system points into it, and no startup file was touched.
