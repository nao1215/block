#!/usr/bin/env bash
#
# smoke_artifacts.sh validates GoReleaser output before it is published: every
# archive must contain the block binary, the host (linux/amd64) archive must
# extract and answer `block version`, and every distributable artifact must
# carry the integrity metadata the README promises for it.
#
# The release workflow runs this against a snapshot built with
# --skip=publish,sign,sbom, so the signature and the SBOMs do not exist yet at
# that point. What it can check there is coverage: that checksums.txt names
# every file a user could download, since one cosign signature over that file
# is what covers all of them. With SBOMs present — a full snapshot, or a local
# run with syft — it checks those too.
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
zips=("$DIST"/*.zip)
shopt -u nullglob
[ ${#tarballs[@]} -gt 0 ] || fail "no archives (*.tar.gz) found in $DIST"
[ ${#zips[@]} -gt 0 ] || fail "no Windows archives (*.zip) found in $DIST"

for a in "${tarballs[@]}"; do
	tar -tzf "$a" | grep -Eq '(^|/)block$' || fail "$a is missing the block binary"
	note "archive contents OK: $(basename "$a")"
done

for a in "${zips[@]}"; do
	unzip -Z1 "$a" | grep -Eq '(^|/)block\.exe$' || fail "$a is missing block.exe"
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

# The registry snapshot must be embedded: list is offline and needs nothing else.
"$workdir/block" list | grep -q '^foundry ' || fail "embedded registry does not list foundry"
note "embedded registry OK"

# --- integrity metadata -----------------------------------------------------
#
# Every artifact a user can download has to be listed in checksums.txt. The
# release signs that one file with cosign, and the README says the signature
# covers the rest through it; a package missing from the list would be
# published with nothing standing behind it.
[ -f "$DIST/checksums.txt" ] || fail "checksums.txt was not produced"

shopt -s nullglob
distributables=("$DIST"/*.tar.gz "$DIST"/*.zip "$DIST"/*.deb "$DIST"/*.rpm "$DIST"/*.apk)
sboms=("$DIST"/*.sbom.json)
shopt -u nullglob
[ ${#distributables[@]} -gt 0 ] || fail "no distributable artifacts found in $DIST"

for a in "${distributables[@]}"; do
	name="$(basename "$a")"
	grep -Fq "  $name" "$DIST/checksums.txt" || fail "$name is not listed in checksums.txt"
done
note "checksums.txt covers all ${#distributables[@]} distributable artifacts"

# nfpms packages are built from the same binaries as the archives, and both
# are published, so both need an SBOM. Skipped when the snapshot was built
# without syft, which is how the pre-publish smoke run is built.
if [ ${#sboms[@]} -gt 0 ]; then
	for a in "${distributables[@]}"; do
		name="$(basename "$a")"
		[ -f "$DIST/$name.sbom.json" ] || fail "$name has no SBOM beside it"
	done
	note "every distributable artifact has an SBOM"
else
	note "no SBOMs in $DIST (built with --skip=sbom); skipping the SBOM check"
fi

note "all artifact smoke checks passed"
