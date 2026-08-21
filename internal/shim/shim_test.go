package shim

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestEnsureCreatesAndReuses(t *testing.T) {
	t.Parallel()
	st := &store.Store{Root: t.TempDir()}
	binary := self(t)

	created, err := Ensure(st, binary, []string{"forge", "cast"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(created, ",") != "forge,cast" {
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

	// A second sync of the same tools writes nothing.
	created, err = Ensure(st, binary, []string{"forge", "cast"})
	if err != nil || len(created) != 0 {
		t.Errorf("Ensure(again) = %v, %v", created, err)
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
	if strings.TrimSpace(string(marker)) != upgraded {
		t.Errorf("marker = %q, want %q", marker, upgraded)
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
