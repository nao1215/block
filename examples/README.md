# Examples

Real `block.toml` files for the kinds of repository block was written for.
Copy one next to your project, adjust the versions, and run:

```shell
block lock
block sync
block exec <tool> --version
```

Every manifest here is parsed and checked by `examples_test.go` on each push:
the tool names resolve against the embedded registry, the commands are the ones
those tools actually provide, and the platforms are ones block supports. An
example that stopped being true would fail the build rather than mislead a
reader.

None of them carry a `block.lock`. A lockfile records exact versions, URLs and
digests resolved at one moment, and committing one here would go stale by the
next upstream release. Yours belongs in your repository, beside your
`block.toml`, committed.

| File | For a repository that |
|:--|:--|
| [`evm-contracts.toml`](./evm-contracts.toml) | develops and tests Solidity contracts |
| [`evm-node.toml`](./evm-node.toml) | runs an Ethereum execution and consensus client pair |
| [`cosmos-appchain.toml`](./cosmos-appchain.toml) | builds a Cosmos SDK chain and relays IBC |
| [`solana-program.toml`](./solana-program.toml) | writes Solana programs with Anchor |
| [`bitcoin.toml`](./bitcoin.toml) | drives a Bitcoin node from tests |
| [`starknet.toml`](./starknet.toml) | writes Cairo contracts for Starknet |
| [`multi-chain.toml`](./multi-chain.toml) | spans EVM, Cosmos and Solana in one tree |
| [`byo-tool.toml`](./byo-tool.toml) | needs a tool the registry does not carry |

Which tools exist for a chain you do not see here:

```shell
block list                 # every tool, with the systems it serves
block list starknet        # one system, with the commands each tool provides
```

[doc/tools.md](../doc/tools.md) is the same catalogue as a page.
