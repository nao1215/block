---
title: CI
description: "Reproducing a pinned blockchain toolchain on a runner, and keeping it up to date without surprises."
---

```yaml
- uses: actions/checkout@v4
- uses: nao1215/setup-block@v0
  with:
    sync: "true"
- run: block exec forge test
```

[setup-block](https://nao1215.github.io/setup-block/) installs the CLI,
verifies its checksum, exports `$BLOCK_HOME` and caches it on your
`block.lock`. Without the action, the same thing by hand:

```yaml
- uses: actions/cache@v4
  with:
    path: ~/.local/share/block
    key: block-${{ runner.os }}-${{ hashFiles('block.lock') }}
- run: block sync
- run: block exec forge test
```

There is no CI-only flag. `block sync` is always a locked operation: it fails,
rather than resolving anything, when

- `block.lock` is missing;
- `block.toml` and `block.lock` disagree — a tool added or removed, a
  constraint changed, a project-local source changed;
- `block.lock` has no artifact for the runner's platform;
- a downloaded artifact's SHA-256 does not match.

## Lock for every platform you use

`block lock` resolves artifacts for the platforms `block.toml` lists, and for
the machine it runs on when it lists none. A team on macOS whose CI runs Linux
declares both once:

```toml
platforms = ["darwin/arm64", "linux/amd64"]
```

Otherwise the runner stops with "block.lock has no artifact for linux/amd64" —
deliberately, because installing something the lockfile does not name is the
one thing `sync` must never do.

## Keeping up with upstream

Moving a pin is a change to `block.lock`, so it belongs in a pull request, not
in a job. A scheduled workflow can tell you when one is worth making:

```yaml
- uses: nao1215/setup-block@v0
- run: block lock --check   # exit 2 when the lockfile would change
```

## Tokens

`GITHUB_TOKEN` is only relevant to `block lock` and `block lock --check`,
which call the GitHub API to discover versions. `block sync` and `block exec`
never do.
