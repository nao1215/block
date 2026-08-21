#!/usr/bin/env bash
#
# registry-sync.sh vendors block-registry's recipes into registry/.
#
# block-registry is where recipes are written and reviewed; registry/ here is
# a copy of one revision of it, embedded into the binary so that `block list`
# and `block lock` work offline and a block version always pairs with a
# registry it was tested against. This script is the only supported way to
# change registry/*.toml.
#
# What is NOT vendored, and why: policy/hosts.toml, schema/, cmd/registry-lint
# and the catalog website stay in block-registry. They are how a recipe is
# reviewed before it is merged there — the rule about which hosts a recipe may
# download from is enforced at that gate, on every push. block executes the
# source a recipe names; it never chooses one, and a project-local
# [tools.<name>.source] in block.toml is deliberately free to point wherever
# its author needs. Copying the policy here would put a file in block that
# nothing in block can act on, and give the rule a second home to drift in.
# What block owns is the other check: that a recipe still resolves, downloads
# and runs against the real upstream (make registry-live).
#
# Usage:
#   scripts/registry-sync.sh                 # newest main
#   REVISION=<sha or ref> scripts/registry-sync.sh
#   REPO=owner/name scripts/registry-sync.sh
set -euo pipefail

REPO="${REPO:-nao1215/block-registry}"
REVISION="${REVISION:-}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
DEST="$REPO_ROOT/registry"

if ! command -v gh >/dev/null; then
	echo "registry-sync: gh is required (it authenticates to private repositories)" >&2
	echo "registry-sync: install it from https://cli.github.com and run 'gh auth login'" >&2
	exit 127
fi

# A ref is resolved to the commit it points at, so SNAPSHOT records what was
# actually vendored rather than a name that moves.
if [ -z "$REVISION" ]; then
	REVISION="$(gh api "repos/$REPO/commits/HEAD" --jq .sha)"
else
	REVISION="$(gh api "repos/$REPO/commits/$REVISION" --jq .sha)"
fi
echo "registry-sync: $REPO at $REVISION"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/block-registry-sync.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# The tarball goes through gh, so this works for a private block-registry with
# no git credentials configured.
gh api "repos/$REPO/tarball/$REVISION" > "$TMP/registry.tar.gz"
tar -xzf "$TMP/registry.tar.gz" -C "$TMP"
SRC="$(find "$TMP" -mindepth 2 -maxdepth 2 -type d -name registry -print -quit)"
if [ -z "$SRC" ]; then
	echo "registry-sync: $REPO at $REVISION has no registry/ directory" >&2
	exit 1
fi
count="$(find "$SRC" -maxdepth 1 -name '*.toml' | wc -l | tr -d ' ')"
if [ "$count" -eq 0 ]; then
	echo "registry-sync: $REPO at $REVISION has no recipes; refusing to empty registry/" >&2
	exit 1
fi

# Replaced wholesale rather than merged: a recipe deleted upstream must
# disappear here too, or block would keep installing a tool nobody maintains.
rm -f "$DEST"/*.toml
cp "$SRC"/*.toml "$DEST/"

go run "$REPO_ROOT/scripts/registry-snapshot" -dir "$DEST" -write -revision "$REVISION" \
	-source "https://github.com/$REPO"

# The sync is not done until the snapshot it produced passes the checks the
# rest of the tree applies to it. A recipe that block cannot even load is a
# failed sync, not something to commit and find out about later.
echo "registry-sync: checking the snapshot"
(cd "$REPO_ROOT" && go run ./scripts/registry-snapshot -verify && go test ./registry/)

echo "registry-sync: registry/ is now $REPO at $REVISION ($count recipes)"
echo "registry-sync: commit it as \"chore(registry): sync to ${REVISION:0:12}\""
