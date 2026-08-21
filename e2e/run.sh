#!/usr/bin/env bash
#
# run.sh builds block and the offline fake GitHub server, then runs the atago
# end-to-end suite (e2e/atago/*.atago.yaml) against the real CLI. Nothing
# touches the developer's real $HOME or $BLOCK_HOME and no network access is
# required: every scenario isolates BLOCK_HOME under its own ${workdir} and
# points BLOCK_GITHUB_API_URL at the fake server exported here.
#
# The test DEFINITIONS are atago YAML; this script only bootstraps the
# environment.
#
# Usage: e2e/run.sh [atago args...]        (e.g. e2e/run.sh --filter sync)
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"

if ! command -v atago >/dev/null 2>&1; then
	echo "e2e: atago is not installed. Install it from https://github.com/nao1215/atago" >&2
	echo "e2e: e.g. 'go install github.com/nao1215/atago@latest' (CI uses nao1215/setup-atago)" >&2
	exit 127
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/block-e2e.XXXXXX")"
FAKEGH_PID=""
cleanup() {
	if [ -n "$FAKEGH_PID" ]; then
		kill "$FAKEGH_PID" >/dev/null 2>&1 || true
		wait "$FAKEGH_PID" 2>/dev/null || true
	fi
	rm -rf "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/bin"

# COVER=1 builds a coverage-instrumented block so `make coverage` can collect
# the real CLI's runtime coverage via GOCOVERDIR (exported by the caller and
# passed through by atago to every spec command).
BLOCK_COVER_FLAGS=""
if [ -n "${COVER:-}" ]; then
	BLOCK_COVER_FLAGS="-cover -covermode=atomic -coverpkg=./..."
	echo "e2e: COVER=1 -> building coverage-instrumented block"
fi

echo "e2e: building block and fakegh..."
# shellcheck disable=SC2086 # BLOCK_COVER_FLAGS must word-split into separate args.
(cd "$REPO_ROOT" && go build $BLOCK_COVER_FLAGS -ldflags '-X github.com/nao1215/block/internal/cmdinfo.Version=v0.0.0-e2e' -o "$TMP/bin/block" .)
(cd "$REPO_ROOT" && go build -o "$TMP/bin/fakegh" ./e2e/fakegh)

echo "e2e: starting fake GitHub..."
"$TMP/bin/fakegh" -addr 127.0.0.1:0 -url-file "$TMP/fakegh.url" &
FAKEGH_PID=$!
for _ in $(seq 1 50); do
	[ -s "$TMP/fakegh.url" ] && break
	sleep 0.1
done
if [ ! -s "$TMP/fakegh.url" ]; then
	echo "e2e: fakegh did not start" >&2
	exit 1
fi

# FAKEGH_URL is the root; scenarios derive BLOCK_GITHUB_API_URL from it so
# they can pick the /t1 snapshot or the failure-mode prefixes.
export FAKEGH_URL
FAKEGH_URL="$(cat "$TMP/fakegh.url")"
export BLOCK_GITHUB_API_URL="$FAKEGH_URL"
# Never let a developer's token or real store leak into the suite.
unset GITHUB_TOKEN GH_TOKEN BLOCK_HOME XDG_DATA_HOME

export PATH="$TMP/bin:$PATH"

echo "e2e: BLOCK_GITHUB_API_URL=$BLOCK_GITHUB_API_URL"
atago run "$@" "$SCRIPT_DIR/atago"
