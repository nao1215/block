# Demo fixtures

The projects the README GIFs are recorded in. Nothing here is part of block;
`make demo` uses them and then throws the store away.

| Directory | Recorded by | Shows |
|:--|:--|:--|
| `project/` | [`doc/vhs/demo.tape`](../vhs/demo.tape) | lock → sync → exec, on a two-tool manifest |
| `defi/`, `bridge/` | [`doc/vhs/shims.tape`](../vhs/shims.tape) | two repositories, two Foundry versions, no switching |

`doc/vhs/list.tape` needs no fixture: `block list` reads the registry embedded
in the binary.

The lockfiles are generated rather than committed, because they pin exact
upstream versions and a committed one here would be stale within a release.
`make demo` writes them, and the recording of `block lock` is what produces
the one in `project/`.

The store lives in `/tmp/block-demo` — a short path, so it does not put a
maintainer's home directory in a published GIF, and a throwaway one, so the
recording never touches your real `$BLOCK_HOME`.
