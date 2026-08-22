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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/store"
)

// DirName is the directory inside $BLOCK_HOME that holds the shims.
const DirName = "shims"

// markerName records which block binary the shims point at, so that an
// upgrade can be noticed without inspecting every file.
const markerName = ".block-shims"

// markerFormat is the first token of a marker block understands. A marker
// without it was written by a version that recorded the path alone, which is
// not enough to notice a binary replaced at the same path, so it is treated
// as stale rather than trusted.
const markerFormat = "block-shims 2"

const (
	dirMode  = 0o755
	fileMode = 0o755
)

// IsShim reports whether this process was started through a shim rather than
// as block itself.
//
// An argv[0] that names no command — empty, or a path whose last element is
// "." or ".." — is not a shim. filepath.Base turns all of those into "." or
// "..", which would otherwise be taken as the name of a command to look for,
// and the answer for an invocation block cannot identify is to be block.
func IsShim(argv0 string) bool {
	name := CommandName(argv0)
	switch name {
	case "", ".", "..", string(filepath.Separator):
		return false
	}
	return !strings.EqualFold(name, "block")
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
		return nil, diag.Internal.Errorf("the path of the block binary is unknown")
	}
	// One binary can be spelled several ways — macOS reaches the same file
	// through /var and /private/var — and the marker below compares paths,
	// so it is resolved once here rather than rewriting every shim whenever
	// the spelling changes.
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	// On Windows a shim is a hard link or a copy, so it holds the binary's
	// contents rather than following its path: an upgrade that replaces
	// block.exe at the same path — which is what `go install` does — leaves
	// every shim executing the old build. The marker therefore records what
	// the binary *is*, not only where it is.
	digest, err := fileDigest(self)
	if err != nil {
		return nil, diag.StoreUnwritable.Errorf("identifying the block binary at %s: %w", self, err)
	}
	dir := Dir(st)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, diag.StoreUnwritable.Wrap(err)
	}
	stale, err := markerDiffers(dir, self, digest)
	if err != nil {
		return nil, err
	}
	if stale {
		// The shims are global: the directory also holds the commands every
		// other project on this machine synced. Rebuilding for an upgrade
		// must rebuild all of them, not only this project's, or "geth" in the
		// project next door silently stops resolving until it syncs again.
		existing, err := commandsIn(dir)
		if err != nil {
			return nil, err
		}
		commands = mergeCommands(commands, existing)
		if err := removeAll(dir); err != nil {
			// The usual cause is a shim that is running right now: a build
			// script invoked through "forge" that itself runs "block sync".
			// Say so, because the file name alone does not.
			return nil, diag.StoreUnwritable.Errorf("replacing the shims in %s: %w; a shim that is running cannot be replaced while it runs", dir, err)
		}
	}
	var created []string
	for _, command := range commands {
		target := filepath.Join(dir, FileName(command))
		if _, err := os.Lstat(target); err == nil {
			continue
		}
		if err := link(self, target); err != nil {
			return created, diag.StoreUnwritable.Errorf("creating the shim for %q: %w", command, err)
		}
		created = append(created, command)
	}
	// The marker is written last, and only once every shim is in place, so a
	// run that dies part-way leaves a directory whose marker is missing or
	// still describes the previous binary — both of which the next run reads
	// as stale and rebuilds from.
	if stale || len(created) > 0 {
		if err := writeMarker(dir, self, digest); err != nil {
			return created, err
		}
	}
	return created, nil
}

// commandsIn lists the commands the shim directory already serves, so that a
// rebuild can recreate them.
func commandsIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if name == markerName || strings.HasPrefix(name, markerName) || e.IsDir() {
			continue
		}
		out = append(out, CommandName(name))
	}
	return out, nil
}

// mergeCommands unions two command lists, sorted and without duplicates.
func mergeCommands(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range append(append([]string(nil), a...), b...) {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// fileDigest is the SHA-256 of the file at path. It is what tells one block
// binary from another when both live at the same path.
func fileDigest(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // the running binary
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// markerDiffers reports whether the shims were built from a different block
// binary than the one running now — a different path, different contents, or
// a marker too old to say.
func markerDiffers(dir, self, digest string) (bool, error) {
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
	wantPath, wantDigest, ok := parseMarker(string(data))
	if !ok {
		// Written by a version that recorded the path alone. It cannot say
		// whether the binary at that path is still the same one, so rebuild.
		return true, nil
	}
	return wantPath != self || wantDigest != digest, nil
}

// parseMarker reads the marker block writes today. It reports !ok for
// anything else, including the single-line path a previous version wrote.
func parseMarker(data string) (path, digest string, ok bool) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	if len(lines) != 3 || strings.TrimSpace(lines[0]) != markerFormat {
		return "", "", false
	}
	path, okPath := strings.CutPrefix(strings.TrimSpace(lines[1]), "path=")
	digest, okDigest := strings.CutPrefix(strings.TrimSpace(lines[2]), "digest=")
	if !okPath || !okDigest || path == "" || digest == "" {
		return "", "", false
	}
	return path, digest, true
}

// writeMarker replaces the marker atomically, so a marker is never read
// half-written and mistaken for one describing a different binary.
func writeMarker(dir, self, digest string) error {
	body := fmt.Sprintf("%s\npath=%s\ndigest=%s\n", markerFormat, self, digest)
	final := filepath.Join(dir, markerName)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), fileMode); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
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
