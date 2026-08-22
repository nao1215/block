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
	"time"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/store"
)

// DirName is the directory inside $BLOCK_HOME that holds the shims.
const DirName = "shims"

// markerName records which block binary the shims point at, so that an
// upgrade can be noticed without inspecting every file.
const markerName = ".block-shims"

// tmpPrefix names the files Ensure builds before renaming them into place.
// It is deliberately not something [CommandName] could return, so a leftover
// is never mistaken for a command the directory serves.
const tmpPrefix = ".block-shim-tmp"

// markerFormat is the first token of a marker block understands. A marker
// without it was written by an older version — the path alone, which cannot
// notice a binary replaced at the same path, or the path and the digest
// without the commands — and what block does with one is described at
// [readMarker].
const markerFormat = "block-shims 3"

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
	// The shims are global: the directory also holds the commands every other
	// project on this machine synced. They are all block's business — a
	// rebuild has to bring back every one of them, not only this project's,
	// or "geth" in the project next door silently stops resolving until it
	// syncs again — so the marker records which commands the directory
	// serves, and this run adds its own to that list.
	known, stale, err := readMarker(dir, self, digest)
	if err != nil {
		return nil, err
	}
	commands = mergeCommands(commands, known)
	var created []string
	for _, command := range commands {
		target := filepath.Join(dir, FileName(command))
		// A shim that is already there points at this binary, so a sync that
		// installs nothing new writes nothing. A rebuild replaces every one
		// of them instead, because the binary they point at has changed.
		if !stale {
			if _, err := os.Lstat(target); err == nil {
				continue
			}
		}
		if err := place(self, target); err != nil {
			if stale {
				// The usual cause on Windows is a shim that is running right
				// now: a build script invoked through "forge" that itself
				// runs "block sync". Say so, because the error does not.
				return created, diag.StoreUnwritable.Errorf("replacing the shim for %q: %w; a shim that is running cannot be replaced while it runs", command, err)
			}
			return created, diag.StoreUnwritable.Errorf("creating the shim for %q: %w", command, err)
		}
		created = append(created, command)
	}
	// The marker is written last, and only once every shim is in place, so a
	// run that dies part-way leaves a directory whose marker is missing or
	// still describes the previous binary — both of which the next run reads
	// as stale and rebuilds from.
	if stale || len(created) > 0 {
		if err := writeMarker(dir, self, digest, commands); err != nil {
			return created, err
		}
	}
	return created, nil
}

// place puts a shim for the block binary at target, replacing whatever is
// there. It is built at a unique path in the same directory and renamed over
// the target, which is atomic and idempotent — so two syncs running at once
// (two projects, or a person and a build script) both succeed, and no command
// is ever missing from the directory in between.
//
// Creating the file at its final path instead would race: the first process
// wins and the second fails with "file exists". Emptying the directory first
// and rebuilding it, which is what block used to do for an upgrade, opens a
// window in which every command on the machine is gone — and takes any file
// that was not a shim with it.
func place(self, target string) error {
	dir := filepath.Dir(target)
	f, err := os.CreateTemp(dir, tmpPrefix+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// link needs a free path: it symlinks on Unix and hard-links on Windows.
	if err := os.Remove(tmp); err != nil {
		return err
	}
	if err := link(self, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := renameOver(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// renameOver moves tmp onto final, retrying briefly.
//
// Windows refuses a rename whose destination another process holds open, with
// "Access is denied", and two syncs replacing the same file at the same
// moment is exactly that: both are writing the same thing, and the one that
// arrives second only has to wait for the first to finish. A handful of
// attempts over a tenth of a second covers it. What outlasts them is a file
// that is genuinely in use — a shim that is running — and is reported.
// Elsewhere a rename over an existing file does not fail this way, so the
// loop costs nothing.
func renameOver(tmp, final string) error {
	const (
		attempts = 10
		maxDelay = 16 * time.Millisecond
	)
	var err error
	for attempt, delay := 0, time.Millisecond; attempt < attempts; attempt++ {
		if err = os.Rename(tmp, final); err == nil {
			return nil
		}
		time.Sleep(delay)
		if delay < maxDelay {
			delay *= 2
		}
	}
	return err
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
		if strings.HasPrefix(name, markerName) || strings.HasPrefix(name, tmpPrefix) || e.IsDir() {
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

// readMarker reports which commands the shim directory serves and whether
// they were built from a different block binary than the one running now — a
// different path, different contents, or a marker too old to say.
//
// A marker block cannot read is one written before this format, or a
// half-restored directory. Both are rebuilt rather than trusted, and the
// commands are recovered by looking at the directory, which is the only thing
// left to go on. That is a migration path, not the normal one: it cannot tell
// a shim from a file somebody else put there, and once this run writes a
// marker of its own there is nothing left to guess.
func readMarker(dir, self, digest string) (commands []string, stale bool, err error) {
	data, readErr := os.ReadFile(filepath.Join(dir, markerName)) //nolint:gosec // block's own store
	switch {
	case readErr == nil:
		if m, ok := parseMarker(string(data)); ok {
			return m.commands, m.path != self || m.digest != digest, nil
		}
	case !os.IsNotExist(readErr):
		return nil, false, readErr
	}
	existing, err := commandsIn(dir)
	if err != nil {
		return nil, false, err
	}
	// An empty directory with no marker is a first sync, not a rebuild.
	return existing, len(existing) > 0 || readErr == nil, nil
}

// marker is what the shim directory records about itself.
type marker struct {
	path     string
	digest   string
	commands []string
}

// parseMarker reads the marker block writes today. It reports !ok for
// anything else, including the formats previous versions wrote.
func parseMarker(data string) (marker, bool) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	const fields = 4
	if len(lines) != fields || strings.TrimSpace(lines[0]) != markerFormat {
		return marker{}, false
	}
	path, okPath := strings.CutPrefix(strings.TrimSpace(lines[1]), "path=")
	digest, okDigest := strings.CutPrefix(strings.TrimSpace(lines[2]), "digest=")
	list, okList := strings.CutPrefix(strings.TrimSpace(lines[3]), "commands=")
	if !okPath || !okDigest || !okList || path == "" || digest == "" {
		return marker{}, false
	}
	var commands []string
	if list != "" {
		commands = strings.Split(list, ",")
	}
	return marker{path: path, digest: digest, commands: commands}, true
}

// writeMarker replaces the marker atomically, so a marker is never read
// half-written and mistaken for one describing a different binary.
func writeMarker(dir, self, digest string, commands []string) error {
	body := fmt.Sprintf("%s\npath=%s\ndigest=%s\ncommands=%s\n", markerFormat, self, digest, strings.Join(commands, ","))
	// A unique temporary name, not a shared one: two syncs writing the marker
	// at the same moment would otherwise overwrite each other's half-written
	// file, and one of them would rename a path the other had already moved.
	f, err := os.CreateTemp(dir, tmpPrefix+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, fileMode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := renameOver(tmp, filepath.Join(dir, markerName)); err != nil {
		_ = os.Remove(tmp)
		return err
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
