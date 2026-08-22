//go:build !windows

package block

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLookPathSearchesTheGivenPathOnly(t *testing.T) {
	t.Parallel()
	first := t.TempDir()
	second := t.TempDir()
	want := filepath.Join(second, "forge")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A file without the executable bit, and a directory of the name, are
	// both things PATH lookup passes over.
	if err := os.WriteFile(filepath.Join(first, "forge"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	third := t.TempDir()
	if err := os.Mkdir(filepath.Join(third, "cast"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := first + string(os.PathListSeparator) + "" + string(os.PathListSeparator) + third + string(os.PathListSeparator) + second
	got, err := LookPath("forge", path)
	if err != nil || got != want {
		t.Fatalf("LookPath(forge) = %q, %v; want %q", got, err, want)
	}
	if _, err := LookPath("cast", path); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("LookPath(cast) err = %v, want ErrNotFound", err)
	}
	// The process's own PATH is not consulted: "sh" is on it and not here.
	if _, err := LookPath("sh", path); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("LookPath(sh) err = %v, want ErrNotFound", err)
	}
}

func TestLookPathWithASeparatorIsTheNameItself(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := filepath.Join(dir, "forge")
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := LookPath(want, "")
	if err != nil || got != want {
		t.Fatalf("LookPath(%q) = %q, %v", want, got, err)
	}
}
