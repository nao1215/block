#!/usr/bin/env bash
#
# coverage.sh combines unit-test coverage with self-hosted E2E coverage into a
# single cover.out. Unit tests never exercise the real block binary the way an
# end user does; the atago-driven E2E specs do. `go build -cover` instruments
# the binary and GOCOVERDIR collects its runtime coverage, so both views merge
# into one honest number.
#
# Everything lands under .coverage/ (gitignored) except the final cover.out /
# cover.html, the same artifacts `make test` produces.
set -euo pipefail

REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

COV="$REPO_ROOT/.coverage"
rm -rf "$COV"
mkdir -p "$COV/unit" "$COV/e2e" "$COV/merged"

echo ">> unit coverage -> $COV/unit"
go test -count=1 -cover -covermode=atomic -coverpkg=./... ./... \
	-args -test.gocoverdir="$COV/unit"

echo ">> e2e coverage -> $COV/e2e"
COVER=1 GOCOVERDIR="$COV/e2e" "$REPO_ROOT/e2e/run.sh"

echo ">> merging unit + e2e covdata -> cover.out"
go tool covdata merge -i="$COV/unit,$COV/e2e" -o="$COV/merged"
go tool covdata textfmt -i="$COV/merged" -o="$REPO_ROOT/cover.out"

go tool cover -func=cover.out | tail -n 1
go tool cover -html=cover.out -o cover.html
echo ">> wrote cover.out and cover.html (unit + e2e combined)"
