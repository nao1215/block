#!/bin/sh
# Writes the inputs of the himorime suite in bench/. Both revisions of a
# comparison run the working tree's copy (${head_root}/gen.sh). Only sh, awk,
# tar, gzip and sha256sum (or shasum) are needed.
#
#   sh gen.sh store DIR BLOCK FAKEGH
#       DIR/block.toml of one tool (example/foo of the fake GitHub in
#       internal/fakegh), locked and synced with the block binary BLOCK while
#       the fake GitHub FAKEGH serves it on a loopback port, and stopped
#       again. The store is DIR/home. Nothing leaves the machine.
#
#   sh gen.sh payload MIB DIR
#       Replaces the archive DIR/block.lock pins with a tar.gz of an
#       executable of MIB MiB, and puts it in the download cache of
#       DIR/home under its sha256, so `block sync` installs it without
#       downloading anything. The content is the same bytes on every run.
set -eu

sha256() {
	if command -v sha256sum >/dev/null; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

case "$1" in
store)
	dir=$2 block=$3 fakegh=$4
	mkdir -p "$dir"
	cat > "$dir/block.toml" <<'EOF'
[tools.foo]
version = "1.2"

[tools.foo.source]
type = "github_release"
repo = "example/foo"
asset = "foo_{version}_{os}_{arch}.tar.gz"
bin = ["foo"]
EOF
	"$fakegh" -addr 127.0.0.1:0 -url-file "$dir/fakegh.url" 2> "$dir/fakegh.log" &
	pid=$!
	trap 'kill "$pid"' EXIT
	i=0
	while [ ! -s "$dir/fakegh.url" ]; do
		i=$((i + 1))
		if [ "$i" -gt 100 ]; then
			echo "gen.sh: the fake GitHub did not start" >&2
			exit 1
		fi
		sleep 0.05
	done
	(
		cd "$dir"
		BLOCK_HOME="$dir/home" BLOCK_GITHUB_API_URL="$(cat fakegh.url)" GITHUB_TOKEN='' GH_TOKEN='' "$block" lock > /dev/null
		BLOCK_HOME="$dir/home" BLOCK_GITHUB_API_URL="$(cat fakegh.url)" GITHUB_TOKEN='' GH_TOKEN='' "$block" sync > /dev/null
	)
	;;
payload)
	mib=$2 dir=$3
	mkdir -p "$dir/payload"
	# A script that exits 0, padded with hexadecimal text to MIB MiB: about
	# as compressible as an executable, and the same bytes on every run.
	awk -v n="$mib" 'BEGIN {
		print "#!/bin/sh"
		print "exit 0"
		srand(1)
		for (i = 0; i < n * 1048576 / 17; i++)
			printf "%08x%08x\n", int(rand() * 4294967295), int(rand() * 4294967295)
	}' > "$dir/payload/foo"
	chmod 755 "$dir/payload/foo"
	tar -C "$dir/payload" -cf - foo | gzip -n > "$dir/payload.tar.gz"
	new=$(sha256 "$dir/payload.tar.gz")
	old=$(sed -n 's/^sha256 = "\(.*\)"$/\1/p' "$dir/block.lock")
	if [ -z "$old" ]; then
		echo "gen.sh: no artifact sha256 in $dir/block.lock" >&2
		exit 1
	fi
	sed "s/$old/$new/" "$dir/block.lock" > "$dir/block.lock.new"
	mv "$dir/block.lock.new" "$dir/block.lock"
	mkdir -p "$dir/home/cache/sha256"
	cp "$dir/payload.tar.gz" "$dir/home/cache/sha256/$new"
	rm -rf "$dir/payload" "$dir/home/tools" "$dir/home/shims"
	;;
*)
	echo "gen.sh: unknown kind $1" >&2
	exit 2
	;;
esac
