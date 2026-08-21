---
title: CI
description: "Reproducing a pinned blockchain toolchain on a runner — GitHub Actions, GitLab CI, CircleCI and Docker — and keeping it up to date without surprises."
toc: true
---

```yaml
- uses: actions/checkout@v6
- uses: nao1215/setup-block@v0
  with:
    sync: "true"
- run: block exec forge test
```

[setup-block](https://nao1215.github.io/setup-block/) installs the CLI,
verifies its checksum, exports `$BLOCK_HOME`, caches the store on your
`block.lock` and runs `sync`. Without the action, the same thing by hand:

```yaml
- uses: actions/checkout@v6
- name: Install block
  env:
    BLOCK_VERSION: 0.1.0
  run: |
    curl -sSfL "https://github.com/nao1215/block/releases/download/v${BLOCK_VERSION}/block_${BLOCK_VERSION}_linux_amd64.tar.gz" | tar xz
    sudo install -m 0755 block /usr/local/bin/block
- uses: actions/cache@v4
  with:
    path: ~/.local/share/block
    key: block-${{ runner.os }}-${{ hashFiles('block.lock') }}
- run: block sync
- run: block exec forge test
```

There is no CI-only flag and no CI mode. `block sync` means the same thing on a
runner as on a laptop, and it fails rather than resolving anything when

- `block.lock` is missing;
- `block.toml` and `block.lock` disagree — a tool added or removed, a
  constraint changed, a project-local source changed;
- `block.lock` has no artifact for the runner's platform;
- a downloaded artifact's SHA-256 does not match.

Each of those is a message that names the command that fixes it. The
[cookbook](/cookbook/#read-a-refusal) has them side by side.

## Lock for every platform you use

`block lock` resolves artifacts for the platforms `block.toml` lists, and for
the machine it runs on when it lists none. A team on macOS whose CI runs Linux
declares both once:

```toml
platforms = ["darwin/arm64", "linux/amd64"]
```

Otherwise the runner stops with

```text
block: block.lock is stale; run "block lock"
  foundry: block.lock has no artifact for linux/amd64
```

deliberately, because installing something the lockfile does not name is the
one thing `sync` must never do.

A matrix needs every platform in that list, and nothing else:

```yaml
strategy:
  matrix:
    os: [ubuntu-latest, macos-latest]
runs-on: ${{ matrix.os }}
steps:
  - uses: actions/checkout@v6
  - uses: nao1215/setup-block@v0
    with:
      sync: "true"
  - run: block exec forge test
```

```toml
platforms = ["linux/amd64", "darwin/arm64"]
```

Whether a given tool actually has a build for each of them is the upstream's
decision, recorded in the registry — the Platforms column in
[Tools](/tools/). A job on a platform an upstream does not ship for is a
lockfile problem you find at `block lock` time, on your own machine, rather
than at 3 a.m. on a runner.

## Caching

Key the cache on `block.lock`: that file names every artifact and every digest
the job will use, so a hit means the store already holds exactly what `sync`
wants and the job downloads nothing.

```yaml
- uses: actions/cache@v4
  with:
    path: ~/.local/share/block
    key: block-${{ runner.os }}-${{ hashFiles('block.lock') }}
```

A partially restored cache is not a hazard. The store is content-addressed and
a cache hit is re-hashed before it is used, so a truncated archive is discarded
and fetched again rather than installed.

On a runner whose cache action wants a path inside the workspace, move the
store there:

```yaml
env:
  BLOCK_HOME: ${{ github.workspace }}/.block
```

## Guarding the lockfile

A pull request that edits `block.toml` without re-locking already fails on the
`sync` step. Giving that its own step says so earlier and more clearly:

```yaml
- name: block.lock matches block.toml
  run: block sync
```

To also learn when the pins have fallen behind upstream, without failing the
build for it:

```yaml
- name: Report toolchain updates
  continue-on-error: true
  run: block lock --check
```

`block lock --check` resolves and writes nothing. It exits 0 when the lockfile
is current, 2 when it would change, 1 on error. Keep it out of the required
checks: a release upstream is news, not a broken build.

## Keeping up with upstream

Moving a pin is a change to `block.lock`, so it belongs in a pull request, not
in a job. A weekly workflow can open that pull request for you:

```yaml
name: Toolchain updates
on:
  schedule: [{ cron: "0 6 * * 1" }]
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  bump:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: nao1215/setup-block@v0
      - id: check
        run: block lock --check || echo "stale=${?}" >> "$GITHUB_OUTPUT"
      - if: steps.check.outputs.stale == '2'
        run: block lock
      - if: steps.check.outputs.stale == '2'
        uses: peter-evans/create-pull-request@v7
        with:
          title: "chore: move the toolchain pins forward"
          branch: block/toolchain-update
```

The diff is `block.lock`, and it is reviewable: exact versions in, exact
versions out, with a URL and a digest per platform.

## Other CI systems

The install is a tarball and the commands are the same everywhere.

GitLab CI:

```yaml
variables:
  BLOCK_HOME: "$CI_PROJECT_DIR/.block"

toolchain:
  cache:
    key:
      files: [block.lock]
    paths: [".block"]
  before_script:
    - curl -sSfL "https://github.com/nao1215/block/releases/download/v0.1.0/block_0.1.0_linux_amd64.tar.gz" | tar xz -C /usr/local/bin block
    - block sync
  script:
    - block exec forge test
```

CircleCI:

```yaml
steps:
  - checkout
  - restore_cache: { keys: ["block-{{ checksum \"block.lock\" }}"] }
  - run: curl -sSfL "https://github.com/nao1215/block/releases/download/v0.1.0/block_0.1.0_linux_amd64.tar.gz" | sudo tar xz -C /usr/local/bin block
  - run: block sync
  - save_cache:
      key: block-{{ checksum "block.lock" }}
      paths: ["~/.local/share/block"]
  - run: block exec forge test
```

Anything else — Jenkins, Buildkite, a self-hosted runner — is the same three
lines: fetch the binary, `block sync`, `block exec`.

## Inside a container image

Copying `block.toml` and `block.lock` before the rest of the source makes the
toolchain a cacheable layer that only changes when the lockfile does — the same
trick as `go.mod`/`go.sum`:

```dockerfile
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*
RUN curl -sSfL https://github.com/nao1215/block/releases/download/v0.1.0/block_0.1.0_linux_amd64.tar.gz \
  | tar xz -C /usr/local/bin block

WORKDIR /src
COPY block.toml block.lock ./
RUN block sync
COPY . .
RUN block exec forge build
```

## Tokens

`GITHUB_TOKEN` is only relevant to `block lock` and `block lock --check`, which
call the GitHub API to discover versions; an unauthenticated runner gets 60
calls an hour, which a re-lock can exhaust.

```yaml
- run: block lock --check
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

`block sync` and `block exec` never call the API, so a job that only builds and
tests needs no token at all — and cannot be broken by a rate limit.
