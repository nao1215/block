//go:build windows

package block

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookPathTriesPathExtInOrder(t *testing.T) { //nolint:paralleltest // sets PATHEXT
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT")
	first := t.TempDir()
	second := t.TempDir()
	for _, name := range []string{"forge.exe", "forge.bat"} {
		if err := os.WriteFile(filepath.Join(first, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(second, "forge.com"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := first + ";" + second
	// The first directory wins even though the second has an earlier
	// extension; within a directory PATHEXT order decides.
	got, err := LookPath("forge", path)
	if err != nil || !strings.EqualFold(got, filepath.Join(first, "forge.exe")) {
		t.Fatalf("LookPath(forge) = %q, %v", got, err)
	}
	// A name that already carries an extension is taken as it is.
	got, err = LookPath("forge.bat", path)
	if err != nil || !strings.EqualFold(got, filepath.Join(first, "forge.bat")) {
		t.Fatalf("LookPath(forge.bat) = %q, %v", got, err)
	}
	// Windows resolves names without regard to case.
	got, err = LookPath("FORGE", path)
	if err != nil || !strings.EqualFold(got, filepath.Join(first, "forge.exe")) {
		t.Fatalf("LookPath(FORGE) = %q, %v", got, err)
	}
	// An extension PATHEXT does not list is not executable.
	if err := os.WriteFile(filepath.Join(first, "cast.ps1"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LookPath("cast", path); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("LookPath(cast) err = %v, want ErrNotFound", err)
	}
	// A directory of the name is passed over.
	if err := os.Mkdir(filepath.Join(first, "anvil.exe"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LookPath("anvil", path); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("LookPath(anvil) err = %v, want ErrNotFound", err)
	}
}

func TestLookPathFallsBackToTheDefaultPathExt(t *testing.T) { //nolint:paralleltest // sets PATHEXT
	t.Setenv("PATHEXT", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "forge.cmd"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LookPath("forge", dir)
	if err != nil || !strings.EqualFold(got, filepath.Join(dir, "forge.cmd")) {
		t.Fatalf("LookPath(forge) = %q, %v", got, err)
	}
}
