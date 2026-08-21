# Security policy

## Supported versions
Only the latest release of block gets fixes, including security fixes. If you
hit an issue on an older version, please reproduce it on the latest release
first.

## Reporting a vulnerability
Report security issues privately, not through public issues or pull requests.

- Email: n.chika156@gmail.com
- Or use the "Report a vulnerability" button on the repository's Security tab.

block downloads and executes third-party binaries on developer machines and
CI runners, so reports in these areas are especially valuable:

- Checksum verification being bypassed or a lockfile entry being ignored
- Archive extraction writing outside the install directory (path traversal,
  symlinks, hard links)
- `block exec` running something other than the locked toolchain
- Transport downgrade (anything that is not HTTPS reaching a non-loopback host)
- A registry recipe resolving to an artifact its upstream did not publish

Please include enough detail to reproduce:

- block version (`block version`)
- OS and architecture
- The `block.toml` / `block.lock` involved, if you can share them
- The command you ran and what happened

## What to expect
block is maintained by one developer in spare time, so there is no guaranteed
response time. I will acknowledge the report, confirm the issue, and fix it in
a new release. You will be credited in the release notes unless you prefer to
stay anonymous.

## What block does to protect you
- Artifacts are fetched over HTTPS only, including across redirects — a
  redirect to plain HTTP is refused, not followed. (Plain HTTP is accepted
  for loopback addresses so offline test servers can stand in for GitHub.)
- Every download is hashed while streaming and compared with the SHA-256 in
  `block.lock` before anything is extracted, and a cache hit is re-hashed
  rather than trusted for its name.
- Artifacts are unpacked into a temporary directory that is renamed into
  place only after every entry succeeded, every declared executable is there
  and runnable, and a completion marker is written; an install without that
  marker is replaced rather than used. Absolute paths, `..` components,
  symlinks and hard links are refused, including after `strip_components`,
  and executable paths from a lockfile go through the same validation as
  those from a recipe.
- `block sync` never resolves versions, never rewrites `block.lock`, and
  fails on any disagreement between `block.toml` and `block.lock`.
  `block exec` makes the same offline check before running anything, so a
  toolchain that no longer matches the manifest cannot be run by accident.
- Registry recipes are data (TOML), never shell commands.

## Verifying releases
Release artifacts are signed with cosign and ship with an SBOM and build
provenance. See [Verifying a block release](https://nao1215.github.io/block/security/#verifying-a-block-release)
for how to check what you download.
