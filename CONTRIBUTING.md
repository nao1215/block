## Contributing to block
Thank you for helping lock blockchain toolchains down. Every bug report,
registry recipe, test and review makes `git clone && block sync` work for one
more team.

## Contributing as a Developer
### 1. Start with clear communication
- Bug report: include `block version`, your OS/arch, the `block.toml` and
  `block.lock` involved, the command you ran and what happened.
- New feature: open an issue first so we can agree on direction. block is
  deliberately small — see [Non-goals](./README.md#non-goals) before proposing
  a package manager feature.
- New registry recipe: open a PR adding `registry/<tool>.toml` and its row in
  the table in `registry/registry_test.go`, which pins the artifact each
  supported platform resolves to. See [registry/README.md](./registry/README.md)
  for the schema and the order of preference between install methods.

### 2. Keep the quality bar high
- Add or update unit tests for pure logic (version resolution, manifest and
  lockfile parsing, platform matching, artifact selection, checksums).
- Add or update atago E2E scenarios for anything a user can observe: output,
  exit codes, files written, errors. The E2E suite is block's CLI contract.
- Keep error messages actionable: say what is wrong and which command fixes it.

### 3. Run checks before opening a PR
```shell
make test
make vet
make fmt
make lint   # golangci-lint v2
```

### 4. Run the end-to-end tests
block has an offline end-to-end suite that drives the real `block` binary
against a fake GitHub (`e2e/fakegh`) inside a throwaway temp tree. It never
touches your real `$BLOCK_HOME` and needs no network access. The tests are
plain-YAML specs run by [atago](https://github.com/nao1215/atago).

```shell
go install github.com/nao1215/atago@latest
make e2e
```

The harness lives under `e2e/`: `e2e/run.sh` builds block and the fake
GitHub, then runs the specs in `e2e/atago/`. The same `make e2e` runs in CI
(`.github/workflows/e2e.yml`), where atago is installed by
[setup-atago](https://github.com/nao1215/setup-atago).

The fake GitHub exposes two points in time (`/t1` and the default) so specs can
observe "upstream published a new version" without mutable state. Add new
fixture repositories to `internal/fakegh/fakegh.go` when a scenario needs a
behaviour no existing fixture has.

### 5. Coverage
`make coverage` combines unit-test coverage with the E2E suite's runtime
coverage of the real binary into one `cover.out`. CI publishes it through
octocov.

## Documentation
`README.md` is the single user-facing document. When you change a command's
output or flags, update the README and the E2E specs in the same PR.

## Releasing
Maintainers cut releases by pushing a `v*` tag. GoReleaser builds the
archives, signs them with cosign and attaches an SBOM and build provenance.

## Contributing Outside of Coding
- Give block a GitHub Star
- Share block with teams that run multi-chain repositories
- Open issues with clear reproduction steps
