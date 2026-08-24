package shim

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nao1215/block/internal/store"
)

// self stands in for the block binary the shims point at. It has to be a real
// file: Windows links and copies it rather than symlinking.
func self(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "block"+store.ExeSuffix)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCommandName(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ in, want string }{
		{"forge", "forge"},
		{"/usr/local/bin/forge", "forge"},
		{filepath.Join("a", "b"), "b"},
		{"solana-test-validator", "solana-test-validator"},
		{"forge" + store.ExeSuffix, "forge"},
	} {
		if got := CommandName(tt.in); got != tt.want {
			t.Errorf("CommandName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if runtime.GOOS == "windows" {
		// Windows spells the extension however it likes.
		if got := CommandName(`C:\tools\FORGE.EXE`); got != "FORGE" {
			t.Errorf("CommandName(FORGE.EXE) = %q", got)
		}
	}
	if FileName("forge") != "forge"+store.ExeSuffix {
		t.Errorf("FileName() = %q", FileName("forge"))
	}
}

func TestIsShim(t *testing.T) {
	t.Parallel()
	for _, argv0 := range []string{"forge", "/usr/local/bin/cast", "geth" + store.ExeSuffix} {
		if !IsShim(argv0) {
			t.Errorf("IsShim(%q) = false", argv0)
		}
	}
	for _, argv0 := range []string{"block", "/usr/local/bin/block", "block" + store.ExeSuffix} {
		if IsShim(argv0) {
			t.Errorf("IsShim(%q) = true", argv0)
		}
	}
	if runtime.GOOS == "windows" && IsShim(`C:\bin\Block.exe`) {
		t.Error("IsShim ignored Windows' case-insensitive names")
	}
}

// Served answers what a shell would find in the directory: every command
// every project on this machine has synced, and nothing else. `block sync
// --verbose` is what asks.
func TestServedListsTheWholeDirectory(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}

	// Before the first sync there is no directory, which is not an error.
	served, err := Served(st)
	if err != nil || len(served) != 0 {
		t.Fatalf("Served(before any sync) = %v, %v", served, err)
	}

	binary := self(t)
	if _, err := Ensure(st, binary, []string{"forge", "cast"}); err != nil {
		t.Fatal(err)
	}
	// Another project, another tool, one directory.
	if _, err := Ensure(st, binary, []string{"hermes"}); err != nil {
		t.Fatal(err)
	}
	served, err = Served(st)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(served, ",") != "cast,forge,hermes" {
		t.Errorf("Served() = %v, want cast, forge and hermes", served)
	}
	// The marker and any leftover temporary file are block's bookkeeping, not
	// commands.
	for _, name := range served {
		if strings.HasPrefix(name, ".") {
			t.Errorf("Served() named %q, which is not a command", name)
		}
	}
}

func TestEnsureCreatesAndReuses(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	binary := self(t)

	created, err := Ensure(st, binary, []string{"forge", "cast"})
	if err != nil {
		t.Fatal(err)
	}
	// Sorted, not in the order they were handed over: the list is the union
	// of what this project asks for and what the directory already serves, so
	// it has one order rather than the caller's.
	if strings.Join(created, ",") != "cast,forge" {
		t.Errorf("created = %v", created)
	}
	for _, name := range []string{"forge", "cast"} {
		path := filepath.Join(Dir(st), FileName(name))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// However it was placed there — symlink, hard link or copy — it must
		// be the same program block is.
		selfInfo, err := os.Stat(binary)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != selfInfo.Size() {
			t.Errorf("%s is %d bytes, block is %d", name, info.Size(), selfInfo.Size())
		}
	}

	// A second sync of the same tools writes nothing — not even the marker,
	// which is rewritten only when the directory changed.
	before, err := os.Stat(filepath.Join(Dir(st), markerName))
	if err != nil {
		t.Fatal(err)
	}
	created, err = Ensure(st, binary, []string{"forge", "cast"})
	if err != nil || len(created) != 0 {
		t.Errorf("Ensure(again) = %v, %v", created, err)
	}
	after, err := os.Stat(filepath.Join(Dir(st), markerName))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("a sync that placed nothing rewrote the marker")
	}
	// A new tool adds only its own commands.
	created, err = Ensure(st, binary, []string{"forge", "cast", "hermes"})
	if err != nil || strings.Join(created, ",") != "hermes" {
		t.Errorf("Ensure(new tool) = %v, %v", created, err)
	}
}

func TestEnsureRewritesAfterAnUpgrade(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	if _, err := Ensure(st, self(t), []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	// block was reinstalled somewhere else: the shims point at the old
	// binary, which on Windows is a copy that would never update.
	upgraded := self(t)
	created, err := Ensure(st, upgraded, []string{"forge"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created, ",") != "forge" {
		t.Errorf("created = %v, want the shim rewritten", created)
	}
	marker, err := os.ReadFile(filepath.Join(Dir(st), markerName))
	if err != nil {
		t.Fatal(err)
	}
	// The marker holds the resolved path, so that the same binary reached by
	// another spelling — /var against /private/var on macOS, an 8.3 short
	// name on Windows — is recognised as the same binary.
	want, err := filepath.EvalSymlinks(upgraded)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := parseMarker(string(marker))
	if !ok {
		t.Fatalf("marker = %q, which block cannot read back", marker)
	}
	if got.path != want {
		t.Errorf("marker path = %q, want %q", got.path, want)
	}
	if !strings.HasPrefix(got.digest, "sha256:") {
		t.Errorf("marker digest = %q, want a sha256", got.digest)
	}
	if strings.Join(got.commands, ",") != "forge" {
		t.Errorf("marker commands = %v, want the commands the directory serves", got.commands)
	}
	// A shim directory left without a marker is rebuilt rather than trusted.
	if err := os.Remove(filepath.Join(Dir(st), markerName)); err != nil {
		t.Fatal(err)
	}
	created, err = Ensure(st, upgraded, []string{"forge"})
	if err != nil || strings.Join(created, ",") != "forge" {
		t.Errorf("Ensure(no marker) = %v, %v", created, err)
	}
}

func TestEnsureNeedsTheBinaryPath(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	if _, err := Ensure(st, "", []string{"forge"}); err == nil {
		t.Error("Ensure accepted an unknown binary path")
	}
}

func TestOnPath(t *testing.T) {
	st := &store.Store{Root: t.TempDir()}
	if err := os.MkdirAll(Dir(st), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join("/usr", "bin"))
	if OnPath(st) {
		t.Error("OnPath() = true without the directory on PATH")
	}
	t.Setenv("PATH", strings.Join([]string{filepath.Join("/usr", "bin"), Dir(st)}, string(os.PathListSeparator)))
	if !OnPath(st) {
		t.Error("OnPath() = false with the directory on PATH")
	}
	// The same directory spelled differently still counts.
	t.Setenv("PATH", filepath.Join(Dir(st), "..", DirName))
	if !OnPath(st) {
		t.Error("OnPath() did not recognise an equivalent path")
	}
}

func TestSameDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if !SameDir(dir, filepath.Join(dir, ".")) {
		t.Error("SameDir() = false for the same directory")
	}
	if SameDir(dir, t.TempDir()) {
		t.Error("SameDir() = true for different directories")
	}
	if SameDir(dir, filepath.Join(dir, "missing")) {
		t.Error("SameDir() = true for a missing directory")
	}
}

func TestEnsureIgnoresHowTheBinaryIsSpelled(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	binary := self(t)
	if _, err := Ensure(st, binary, []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	// The same file reached through a symlinked directory — which is how
	// macOS presents /var — must not look like a different block.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Dir(binary), link); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	created, err := Ensure(st, filepath.Join(link, filepath.Base(binary)), []string{"forge"})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 0 {
		t.Errorf("created = %v, want the shims left alone", created)
	}
}

// `go install` replaces block at the path it already lives at, by writing a
// new file and renaming it over the old one. On Windows a shim is a hard link
// or a copy, so it holds the old contents; the marker used to record the path
// alone, agreed with itself, and left every shim running the previous build.
func TestEnsureRewritesWhenTheBinaryIsReplacedInPlace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binary := filepath.Join(dir, "block"+store.ExeSuffix)
	writeBinary := func(body string) {
		t.Helper()
		// Written beside and renamed over, the way an installer replaces a
		// running program: the path is unchanged and the contents are not.
		tmp := binary + ".new"
		if err := os.WriteFile(tmp, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, binary); err != nil {
			t.Fatal(err)
		}
	}
	st := &store.Store{Root: t.TempDir()}
	commands := []string{"cast", "forge"}

	writeBinary("#!/bin/sh\necho old\n")
	created, err := Ensure(st, binary, commands)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created, ",") != "cast,forge" {
		t.Fatalf("first Ensure created %v, want both shims", created)
	}

	// Same path, different program.
	writeBinary("#!/bin/sh\necho new\n")
	created, err = Ensure(st, binary, commands)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created, ",") != "cast,forge" {
		t.Errorf("created = %v, want every shim rewritten after an in-place upgrade", created)
	}
	for _, command := range commands {
		got, err := os.ReadFile(filepath.Join(Dir(st), FileName(command)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "echo new") {
			t.Errorf("the %s shim still runs the old binary: %q", command, got)
		}
	}

	// And a third run with nothing changed writes nothing: noticing an
	// upgrade must not mean rebuilding on every sync.
	created, err = Ensure(st, binary, commands)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 0 {
		t.Errorf("created = %v, want nothing rebuilt when the binary is unchanged", created)
	}
}

// A marker written by a version that recorded only the path cannot say
// whether the binary at that path is still the same one, so it is not
// trusted.
func TestEnsureRebuildsFromAnUnreadableMarker(t *testing.T) {
	t.Parallel()
	binary := self(t)
	tests := []struct {
		name   string
		marker string
	}{
		{"the previous format, a bare path", "PATH\n"},
		{"the format before the command list", "block-shims 2\npath=PATH\ndigest=sha256:00\n"},
		{"truncated mid-write", "block-shims 3\npath=PATH\ndigest=sha256:00\n"},
		{"a format from the future", "block-shims 9\npath=PATH\ndigest=sha256:00\ncommands=forge\n"},
		{"an empty digest", "block-shims 3\npath=PATH\ndigest=\ncommands=forge\n"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := &store.Store{Root: t.TempDir()}
			if _, err := Ensure(st, binary, []string{"forge"}); err != nil {
				t.Fatal(err)
			}
			resolved, err := filepath.EvalSymlinks(binary)
			if err != nil {
				t.Fatal(err)
			}
			body := strings.ReplaceAll(tt.marker, "PATH", resolved)
			if err := os.WriteFile(filepath.Join(Dir(st), markerName), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			created, err := Ensure(st, binary, []string{"forge"})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(created, ",") != "forge" {
				t.Errorf("created = %v, want the shims rebuilt from a marker block cannot read", created)
			}
		})
	}
}

// The marker is written by rename, so a reader never sees half of one.
func TestEnsureLeavesNoTemporaryMarker(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	if _, err := Ensure(st, self(t), []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(Dir(st))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}

// The shim directory is shared by every project on the machine. An upgrade
// rebuilds it, and the rebuild has to bring back the commands other projects
// synced — not only the ones the project that noticed the upgrade locks —
// or "geth" next door stops resolving until that project happens to sync.
func TestEnsureKeepsOtherProjectsShimsAcrossAnUpgrade(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	old := self(t)
	if _, err := Ensure(st, old, []string{"forge", "cast"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(st, old, []string{"geth"}); err != nil {
		t.Fatal(err)
	}
	// block was upgraded; a project that locks only hermes syncs first.
	upgraded := self(t)
	created, err := Ensure(st, upgraded, []string{"hermes"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(created, ","); got != "cast,forge,geth,hermes" {
		t.Errorf("created = %q, want every shim rebuilt, not only this project's", got)
	}
	for _, command := range []string{"forge", "cast", "geth", "hermes"} {
		if _, err := os.Stat(filepath.Join(Dir(st), FileName(command))); err != nil {
			t.Errorf("shim %s is gone after the upgrade: %v", command, err)
		}
	}
	// And nothing is rebuilt on the next sync from another project.
	created, err = Ensure(st, upgraded, []string{"forge"})
	if err != nil || len(created) != 0 {
		t.Errorf("Ensure after rebuild = %v, %v, want nothing created", created, err)
	}
}

// filepath.Base turns an empty argv[0] into ".", so the guard that was meant
// to catch it never fired and block ran as a shim for a command called ".".
// An invocation block cannot identify is block.
func TestIsShimIgnoresAnArgv0ThatNamesNoCommand(t *testing.T) {
	t.Parallel()
	for _, argv0 := range []string{"", ".", "..", "./", "../", string(filepath.Separator), "some/dir/.", "some/dir/.."} {
		if IsShim(argv0) {
			t.Errorf("IsShim(%q) = true; an argv[0] naming no command is block itself", argv0)
		}
	}
	for _, argv0 := range []string{"forge", "./forge", "/usr/local/bin/cast", "hermes" + store.ExeSuffix} {
		if !IsShim(argv0) {
			t.Errorf("IsShim(%q) = false, want true", argv0)
		}
	}
	for _, argv0 := range []string{"block", "BLOCK", "/usr/bin/block", "block" + store.ExeSuffix} {
		if IsShim(argv0) {
			t.Errorf("IsShim(%q) = true, want false", argv0)
		}
	}
}

// The shim directory is shared by every project on the machine, so two syncs
// at once — two projects, or a person and a build script — is ordinary. Both
// used to fail: whoever lost the race to create a file got "file exists",
// both wrote the marker through one shared temporary name so one of them
// renamed a path the other had already moved, and the rebuild emptied the
// directory first, so a command another project had just put there was gone.
func TestEnsureSurvivesTwoSyncsAtOnce(t *testing.T) {
	t.Parallel()
	for attempt := range 20 {
		st := &store.Store{Root: t.TempDir()}
		old := self(t)
		if _, err := Ensure(st, old, []string{"forge", "cast"}); err != nil {
			t.Fatal(err)
		}
		if _, err := Ensure(st, old, []string{"geth"}); err != nil {
			t.Fatal(err)
		}
		// Both projects notice the same upgrade at the same moment.
		upgraded := self(t)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		sets := [][]string{{"forge", "cast"}, {"hermes"}}
		for i := range sets {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = Ensure(st, upgraded, sets[i])
			}()
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("attempt %d: concurrent Ensure %d = %v", attempt, i, err)
			}
		}
		for _, command := range []string{"forge", "cast", "geth", "hermes"} {
			if _, err := os.Stat(filepath.Join(Dir(st), FileName(command))); err != nil {
				t.Errorf("attempt %d: %s is missing after two syncs at once: %v", attempt, command, err)
			}
		}
		// Whatever order they finished in, the directory has settled: a third
		// sync of everything creates nothing.
		created, err := Ensure(st, upgraded, []string{"forge", "cast", "geth", "hermes"})
		if err != nil || len(created) != 0 {
			t.Errorf("attempt %d: Ensure after the race = %v, %v", attempt, created, err)
		}
		entries, err := os.ReadDir(Dir(st))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), tmpPrefix) {
				t.Errorf("attempt %d: a half-built shim was left behind: %s", attempt, e.Name())
			}
		}
	}
}

// A rebuild recreates the commands the marker records, so a file block did
// not put in the directory is not turned into a shim — and, unlike the
// rebuild that emptied the directory first, not deleted either.
func TestEnsureLeavesAFileItDidNotPutThere(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	if _, err := Ensure(st, self(t), []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(Dir(st), "notes.txt")
	if err := os.WriteFile(stray, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := Ensure(st, self(t), []string{"forge"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created, ",") != "forge" {
		t.Errorf("created = %v, want only the command the marker records", created)
	}
	body, err := os.ReadFile(stray)
	if err != nil || string(body) != "mine\n" {
		t.Errorf("the stray file is now %q (%v), want it untouched", body, err)
	}
}

// The marker carries the commands, so a directory that has been rebuilt no
// longer has to be scanned to find out what it serves.
func TestMarkerRecordsEveryCommandTheDirectoryServes(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	binary := self(t)
	if _, err := Ensure(st, binary, []string{"forge", "cast"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(st, binary, []string{"geth"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(Dir(st), markerName))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := parseMarker(string(data))
	if !ok {
		t.Fatalf("marker = %q, which block cannot read back", data)
	}
	if strings.Join(m.commands, ",") != "cast,forge,geth" {
		t.Errorf("marker commands = %v, want every command the directory serves", m.commands)
	}
}

// A marker that describes another build of block is refreshed even by a
// sync that has no commands of its own to place, so the next sync does not
// rebuild again for nothing.
func TestEnsureRefreshesAStaleMarkerWithNothingToPlace(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	binary := self(t)
	if _, err := Ensure(st, binary, []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	// The same path, other contents: what an upgrade in place looks like.
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(st, binary, nil); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := fileDigest(resolved)
	if err != nil {
		t.Fatal(err)
	}
	commands, stale, err := readMarker(Dir(st), resolved, digest)
	if err != nil || stale || strings.Join(commands, ",") != "forge" {
		t.Fatalf("readMarker = %v, stale=%v, %v", commands, stale, err)
	}
}
