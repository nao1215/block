# Benchmarks

block is measured the way a user or CI runs it, one process per call from start to exit, with [himorime](https://github.com/nao1215/himorime). himorime builds block, runs each command in interleaved rounds, and reports latency, CPU time, peak RSS and, for the large archive, throughput.

```console
$ go install github.com/nao1215/himorime@latest
$ make bench            # himorime run bench
$ make bench-compare    # himorime compare --against main bench
```

## Regression suite: himorime.yaml

`make bench-compare` checks main out into a temporary Git worktree, builds it and your working tree, and measures both in the same rounds, so a background job slows both revisions instead of one. `BASE=v0.7.1 make bench-compare` compares against another revision. `himorime run --filter 'sync 32MiB' bench` measures one benchmark.

On a pull request, `.github/workflows/bench.yml` runs `himorime ci bench` the same way, with the base of the pull request as the base, on one runner. The job fails when a command is slower, uses more CPU time or more memory than the base beyond the tolerance in `himorime.yaml` with 95% confidence. A difference too close to call is reported as inconclusive and does not fail the job. The comparison is on the job summary page and the JSON report is kept as an artifact.

Nothing is measured against the network. Before measuring, `gen.sh` writes a project of one tool and locks and syncs it with the fake GitHub of the E2E suite (`e2e/fakegh`), started on a loopback port for that step only. Both revisions run the working tree's `gen.sh` and fake GitHub, and each revision locks with its own block. The measured commands then find everything in the store and the download cache: the GitHub API they are given is a closed port, and no token is set. The store, `HOME` and the XDG directories are in the benchmark's own directory.

block tells itself from its shims by the name it was started as, so every benchmark runs the built binary through a link named `block`.

| Benchmark | Commands | Measures |
|-----------|----------|----------|
| `version` | `block version` | starting block |
| `sync 1` | `block sync` | installing one locked tool from the download cache: checking the sha256, extracting the archive and writing the shims; the install is removed before every run |
| `sync 1 installed` | `block sync` | a sync with the tool already in the store, as in CI after the store is restored |
| `sync 32MiB` | `block sync` | installing one tool whose archive holds an executable of 32 MiB from the download cache |
| `exec 1` | `block exec foo --version` | finding a tool from `block.lock` and the store and running it |
| `sync error` | `block sync` without `block.lock` | the error path, BLK1003, exit 1 |

Not measured:

| Command | Why |
|---------|-----|
| `block lock`, and `block sync` when the archive is not in the cache | They talk to the GitHub API and download releases. The local substitute, the fake GitHub, is a server, and himorime stops a process a hook leaves running when the hook exits, so no server can outlive the setup to answer a measured command. |

Numbers from different machines are not comparable; compare revisions on one machine, as `make bench-compare` and CI do.
