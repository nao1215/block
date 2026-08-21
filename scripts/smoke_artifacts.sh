#!/usr/bin/env bash
#
# smoke_artifacts.sh validates GoReleaser output before it is published: every
# archive must contain the block binary, and the host (linux/amd64) archive
# must extract and answer `block version`.
#
# Usage: scripts/smoke_artifacts.sh [dist-dir]   (default: dist)
set -euo pipefail

DIST="${1:-dist}"

if [ ! -d "$DIST" ]; then
	echo "smoke: dist directory '$DIST' does not exist (run goreleaser first)" >&2
	exit 1
fi

fail() {
	echo "smoke: FAIL: $*" >&2
	exit 1
}
note() { echo "smoke: $*"; }

shopt -s nullglob
tarballs=("$DIST"/*.tar.gz)
shopt -u nullglob
[ ${#tarballs[@]} -gt 0 ] || fail "no archives (*.tar.gz) found in $DIST"

for a in "${tarballs[@]}"; do
	tar -tzf "$a" | grep -Eq '(^|/)block$' || fail "$a is missing the block binary"
	note "archive contents OK: $(basename "$a")"
done

host_archive=""
for a in "${tarballs[@]}"; do
	case "$a" in
	*_linux_amd64.tar.gz) host_archive="$a" ;;
	esac
done
[ -n "$host_archive" ] || fail "no linux/amd64 archive found to execute"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
tar -xzf "$host_archive" -C "$workdir"
[ -x "$workdir/block" ] || fail "extracted block binary is not executable"
version_out="$("$workdir/block" version)"
echo "$version_out" | grep -q '^block v' || fail "block version output unexpected: $version_out"
note "extracted binary runs: $version_out"

# The registry must be embedded: a lock against an unknown tool lists what
# the binary knows, without touching the network.
mkdir -p "$workdir/proj" && printf '[tools]\nnot-a-tool = "1"\n' > "$workdir/proj/block.toml"
reg_out="$(cd "$workdir/proj" && BLOCK_HOME="$workdir/home" "$workdir/block" lock 2>&1 || true)"
echo "$reg_out" | grep -q 'known tools: foundry' || fail "embedded registry does not list foundry: $reg_out"
note "embedded registry OK"

note "all artifact smoke checks passed"
