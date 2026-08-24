package block

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/fetch"
)

// The shim directory is global: it holds what every project on the machine
// ever synced. What `block sync` reports is not that directory — it is this
// project's toolchain, which is the only thing the person running it asked
// about. The two diverge the moment the block binary changes, because every
// shim is then rewritten, and a report built from what was written would name
// tools this project has never heard of.
func TestSyncReportsThisProjectsCommandsAndNotTheRestOfTheMachine(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	ctx := context.Background()

	// Project A locks hermes and syncs it, leaving a global "hermes" shim.
	a := newHarness(t, "/t1")
	a.manifest(t, "[tools]\nhermes = \"1.13\"\n")
	if err := a.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := a.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// Project B shares the store and locks foundry alone.
	b := newHarness(t, "/t1")
	b.Store = a.Store
	b.Fetcher = fetch.New(a.Store.CacheDir(), "block/test")
	b.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	if err := b.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	b.reset()
	if err := b.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	const want = "foundry  1.7.4  installed\ncommands: anvil, cast, chisel, forge\n"
	if b.stdout.String() != want {
		t.Errorf("Sync() stdout = %q, want %q", b.stdout, want)
	}

	// block is upgraded, so every shim on the machine is rewritten — hermes
	// among them. The report is still about project B.
	b.Self = upgradedBlock(t)
	b.reset()
	if err := b.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	const wantCached = "foundry  1.7.4  cached\ncommands: anvil, cast, chisel, forge\n"
	if b.stdout.String() != wantCached {
		t.Errorf("Sync() after an upgrade = %q, want %q", b.stdout, wantCached)
	}

	// Project A's shim was never deleted, and hermes still resolves there.
	if _, err := os.Lstat(filepath.Join(a.Store.Root, "shims", "hermes")); err != nil {
		t.Errorf("project A's shim was removed by project B's sync: %v", err)
	}
	if _, err := a.Which("hermes"); err != nil {
		t.Errorf("Which(hermes) in project A after project B synced = %v", err)
	}
}

// --verbose is where the machine-wide view lives: the directory, what this
// run wrote into it, and every command it serves.
func TestSyncVerboseReportsTheWholeShimDirectory(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	ctx := context.Background()

	a := newHarness(t, "/t1")
	a.manifest(t, "[tools]\nhermes = \"1.13\"\n")
	if err := a.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := a.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	b := newHarness(t, "/t1")
	b.Store = a.Store
	b.Fetcher = fetch.New(a.Store.CacheDir(), "block/test")
	b.Verbose = true
	b.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	if err := b.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	b.reset()
	if err := b.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	out := b.stdout.String()
	for _, want := range []string{
		"commands: anvil, cast, chisel, forge\n",
		"shim directory: " + filepath.Join(a.Store.Root, "shims") + "\n",
		"shims written: anvil, cast, chisel, forge\n",
		"shims present: anvil, cast, chisel, forge, hermes\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Sync(--verbose) stdout = %q, missing %q", out, want)
		}
	}
}

// upgradedBlock is a block binary at a path the shims have never pointed at,
// which is what an upgrade looks like to the shim directory. Nothing runs it:
// the shims are written from it, not executed.
func upgradedBlock(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "block")
	if err := os.WriteFile(path, []byte("not the real binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
