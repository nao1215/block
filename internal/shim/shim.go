// Package shim makes a project's locked tools runnable by their own names.
//
// One directory holds one small file per command — forge, cast, geth, hermes
// — and the user puts that directory on PATH once. Every one of those files is
// the block binary itself under another name; run as "forge", block looks for
// the project the working directory belongs to, reads its block.lock, and
// executes the version that project pinned.
//
//	$BLOCK_HOME/shims/forge   ->  block   ->  cwd's project  ->  block.lock
//	                                                          ->  tools/foundry/1.7.4-…/forge
//
// The shims are global: one "forge" serves every project, because which
// version it runs is decided per invocation from the working directory, not
// baked into the file. There are no shell hooks, no per-project files, and
// nothing to re-run after switching branches.
package shim

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nao1215/block/internal/store"
)

// DirName is the directory inside $BLOCK_HOME that holds the shims.
const DirName = "shims"

// markerName records which block binary the shims point at, so that an
// upgrade can be noticed without inspecting every file.
const markerName = ".block-shims"

const (
	dirMode  = 0o755
	fileMode = 0o755
)

// IsShim reports whether this process was started through a shim rather than
// as block itself.
func IsShim(argv0 string) bool {
	name := CommandName(argv0)
	return name != "" && !strings.EqualFold(name, "block")
}

// Dir is the shim directory of a store.
func Dir(st *store.Store) string { return filepath.Join(st.Root, DirName) }

// CommandName is the command a shim file stands for: its name without the
// platform's executable suffix.
func CommandName(argv0 string) string {
	base := filepath.Base(argv0)
	if store.ExeSuffix == "" {
		return base
	}
	if strings.EqualFold(filepath.Ext(base), store.ExeSuffix) {
		return base[:len(base)-len(store.ExeSuffix)]
	}
	return base
}

// FileName is the file a command's shim is stored under.
func FileName(command string) string { return command + store.ExeSuffix }

// Ensure makes sure every command has a shim pointing at the block binary at
// self, and returns the commands it had to create. Shims already pointing at
// this binary are left alone, so a sync that installs nothing new writes
// nothing. When self changes — block was upgraded, moved, or reinstalled —
// every shim is rewritten.
func Ensure(st *store.Store, self string, commands []string) ([]string, error) {
	if self == "" {
		return nil, errors.New("the path of the block binary is unknown")
	}
	// One binary can be spelled several ways — macOS reaches the same file
	// through /var and /private/var — and the marker below compares paths,
	// so it is resolved once here rather than rewriting every shim whenever
	// the spelling changes.
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	dir := Dir(st)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, err
	}
	stale, err := markerDiffers(dir, self)
	if err != nil {
		return nil, err
	}
	if stale {
		if err := removeAll(dir); err != nil {
			return nil, err
		}
	}
	var created []string
	for _, command := range commands {
		target := filepath.Join(dir, FileName(command))
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		if err := link(self, target); err != nil {
			return created, fmt.Errorf("creating the shim for %q: %w", command, err)
		}
		created = append(created, command)
	}
	if stale || len(created) > 0 {
		if err := os.WriteFile(filepath.Join(dir, markerName), []byte(self+"\n"), fileMode); err != nil {
			return created, err
		}
	}
	return created, nil
}

// markerDiffers reports whether the shims point at a different binary than
// the one running now.
func markerDiffers(dir, self string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, markerName)) //nolint:gosec // block's own store
	switch {
	case os.IsNotExist(err):
		// A directory with shims but no marker predates the marker, or was
		// half-restored: rewrite it rather than trust it.
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false, err
		}
		return len(entries) > 0, nil
	case err != nil:
		return false, err
	}
	return strings.TrimSpace(string(data)) != self, nil
}

// removeAll empties the shim directory, keeping the directory itself so that
// a PATH entry pointing at it never disappears.
func removeAll(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// OnPath reports whether the shim directory is already on PATH, so that a
// suggestion to add it is printed once rather than every time.
func OnPath(st *store.Store) bool {
	dir := Dir(st)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == "" {
			continue
		}
		if SameDir(entry, dir) {
			return true
		}
	}
	return false
}

// SameDir compares two directory paths the way the running filesystem does.
func SameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	if store.ExeSuffix != "" && strings.EqualFold(a, b) {
		// Windows paths differ only in case.
		return true
	}
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}
