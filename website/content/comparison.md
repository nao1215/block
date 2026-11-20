---
title: Compared to Docker
description: "Where a container image is the better answer for a blockchain toolchain, where block is, and the numbers behind it."
toc: true
---

A container image is the other common way to pin a blockchain toolchain, and
for some jobs it is the better one. block does less, and where the two overlap
they can be measured against each other.

## Measured

One Linux machine, warm caches, against the official Foundry image
(`ghcr.io/foundry-rs/foundry:stable`):

| | block | Docker image |
| --- | --- | --- |
| Download for one tool | 94 MB archive | 622 MB image |
| On disk after install | 223 MB | n/a |
| `forge --version`, five times | 0.031 s total | 1.351 s total (`docker run`) |
| Preparing an already-installed toolchain (13 tools) | 0.005 s | n/a |

That is roughly 6 ms per invocation against roughly 270 ms, which you notice in
a `forge test` loop.

## Where block wins

Each invocation starts a local process, with no container to start.

On macOS, where most contract developers work, Docker runs a Linux VM, and
compile-heavy work over a bind mount is slower for it. block runs the same
official binaries the image ships, directly on the machine.

A repository that needs `forge`, `hermes`, `gaiad` and `solana` would otherwise
need a hand-maintained image with everything in it, or four containers with
volumes and ports wired together. block puts four binaries on `PATH`.

Local chain state such as `anvil`, a devnet or a validator's data directory is
ordinary local processes and files, with nothing to map.

A lockfile pins the exact upstream release and its SHA-256. An image tag pins whatever it pointed at when you pulled it, unless
you pin a digest and maintain that yourself.

## Where Docker wins

- OS-level isolation, and running untrusted code.
- Anything that is not a CLI: system libraries, databases, and services started
  together with `docker compose`.
- Reproducing a whole operating system rather than a set of tools.

## They compose

`block sync` inside a `Dockerfile` gives an image whose tools are pinned by
checksum instead of by whatever a base image tag pointed at that day:

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

A devcontainer works the same way, with block pinning the tools inside it.

## Compared to mise, asdf and aqua

Those are general-purpose version managers. block covers less:

- Only blockchain CLIs. It does not manage the Go, Rust, Node or Python
  toolchains; use a general manager for those.
- Two install methods, chosen per tool by the registry, with no arbitrary
  scripts and no run-time fallback.
- Three lifecycle commands, one direction, and a lockfile that records the URL and the
  digest of every artifact for every platform you declared.

If you already run mise for languages, block can sit next to it for the chain
tools without overlap.
