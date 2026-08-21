---
title: Compared to Docker
description: "Where a container image is the better answer for a blockchain toolchain, where block is, and the numbers behind it."
toc: true
---

A container image is the other way to pin a blockchain toolchain, and for some
jobs it is the right one. block is not trying to replace it: it does a smaller
thing, and where the two overlap the numbers are worth knowing.

## Measured

One Linux machine, warm caches, against the official Foundry image
(`ghcr.io/foundry-rs/foundry:stable`):

| | block | Docker image |
| --- | --- | --- |
| Download for one tool | 94 MB archive | 622 MB image |
| On disk after install | 223 MB | — |
| `forge --version`, five times | 0.031 s total | 1.351 s total (`docker run`) |
| Preparing an already-installed toolchain (13 tools) | 0.005 s | — |

Roughly 6 ms per invocation against roughly 270 ms. Invisible once; tiring in
a `forge test` loop.

## Where block wins

No per-invocation cost: the tool is a local process, not a container start.

Native execution on macOS, where most contract developers work. Docker runs a
Linux VM there, and compile-heavy work over a bind mount is where that shows.
block runs the same official binaries the image ships, directly on the machine.

Composing tools. A repository that needs `forge`, `hermes`, `gaiad` and
`solana` either gets a hand-maintained kitchen-sink image or four containers
with volumes and ports wired together. block puts four binaries on `PATH`.

Local chain state. `anvil`, a devnet, a validator's data directory — ordinary
local processes and ordinary files, with nothing to map.

One artifact, one checksum. A lockfile pins the exact upstream release and its
SHA-256. An image tag pins whatever it pointed at when you pulled it, unless
you pin a digest and maintain that yourself.

## Where Docker wins

block does not compete here, and says so on purpose:

- OS-level isolation, and running untrusted code.
- Anything that is not a CLI: system libraries, databases, services started
  together — the job `docker compose` exists for.
- Reproducing a whole operating system rather than a set of tools.

## They compose

`block sync` inside a `Dockerfile` gives an image whose tools are pinned by
checksum rather than by whatever a base image tag pointed at that day:

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

The same is true of a devcontainer: block is what makes the tools inside it
reproducible, not a reason to stop using one.

## Compared to mise, asdf and aqua

Those are general-purpose version managers, and they are good at what they
do. block deliberately covers less:

- Only blockchain CLIs. It will not manage the Go, Rust, Node or Python
  toolchains, which is exactly where a general manager belongs.
- Two install methods, chosen per tool by the registry, with no arbitrary
  scripts and no run-time fallback.
- Three commands, one direction, and a lockfile that records the URL and the
  digest of every artifact for every platform you declared.

If you already run mise for languages, block sits next to it for the chain
tools; the two do not overlap.
