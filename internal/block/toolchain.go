package block

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/lockfile"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/store"
)

// Toolchain is a project's installed toolchain: what block.lock pins, checked
// against block.toml and against what is actually on disk. It is what both
// "block exec" and a shim need, and neither of them needs anything else — no
// registry, no network, no resolution.
type Toolchain struct {
	// Dir is the project root: the directory holding block.toml.
	Dir      string
	Platform platform.Platform
	Store    *store.Store
	// commands maps a command name to the executable that provides it.
	commands map[string]string
	// dirs are the directories to put on PATH, in lockfile order.
	dirs []string
}

// OpenToolchain loads the project rooted at dir and verifies, offline, that
// block.lock still matches block.toml and that every locked tool is installed.
// It never resolves, downloads, installs or writes.
func OpenToolchain(dir string, p platform.Platform, st *store.Store) (*Toolchain, error) {
	m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
	if err != nil {
		return nil, err
	}
	l, err := lockfile.Load(filepath.Join(dir, lockfile.FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, diag.LockMissing.Errorf("%s not found; run \"block lock\" and \"block sync\"", lockfile.FileName)
		}
		return nil, err
	}
	if reasons := Check(m, l, []platform.Platform{p}); len(reasons) > 0 {
		return nil, staleError(reasons)
	}
	if err := commandConflict(l); err != nil {
		return nil, err
	}
	t := &Toolchain{Dir: dir, Platform: p, Store: st, commands: map[string]string{}}
	for _, tool := range l.Tools {
		art, ok := tool.Artifact(p)
		if !ok {
			return nil, diag.LockPlatformMissing.Errorf("%s: %s has no artifact for %s; run \"block lock\" and \"block sync\"", tool.Name, lockfile.FileName, p)
		}
		install, err := st.InstallDir(tool.Name, tool.Version, art.SHA256)
		if err != nil {
			return nil, err
		}
		if err := st.Verify(install, tool.Bin); err != nil {
			return nil, fmt.Errorf("%s %s %w; run \"block sync\"", tool.Name, tool.Version, err)
		}
		for _, b := range tool.Bin {
			path, err := store.BinPath(install, b)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", tool.Name, err)
			}
			t.commands[lookupKey(recipe.CommandName(b))] = path
		}
		t.dirs = append(t.dirs, store.BinDirs(install, tool.Bin)...)
	}
	return t, nil
}

// lookupKey is how a command name is matched against the toolchain. Windows
// resolves a command on PATH without regard to case, so `block exec FORGE`
// there must find the same executable `forge` does — otherwise block would
// miss its own toolchain and fall through to whatever PATH offers, which is
// the one thing exec must never do. Elsewhere the match is exact, because
// elsewhere the two really are different commands.
//
// A toolchain cannot contain both spellings: [recipe.CommandKey] refuses that
// on every platform, so folding case here can never make one command mean two
// executables.
func lookupKey(name string) string {
	if store.ExeSuffix == "" {
		return name
	}
	return strings.ToLower(name)
}

// ResolveCommand returns the executable this project's toolchain provides for
// a command name, or false when the toolchain does not provide it. The name
// may carry the platform's executable suffix, however it is spelled.
func (t *Toolchain) ResolveCommand(name string) (string, bool) {
	if store.ExeSuffix != "" && strings.EqualFold(filepath.Ext(name), store.ExeSuffix) {
		name = name[:len(name)-len(store.ExeSuffix)]
	}
	path, ok := t.commands[lookupKey(name)]
	return path, ok
}

// PathDirs are the directories to put in front of PATH so that every tool of
// the toolchain — and anything they call — resolves to this project's
// versions.
func (t *Toolchain) PathDirs() []string {
	out := make([]string, len(t.dirs))
	copy(out, t.dirs)
	return out
}

// Path returns PATH with the toolchain in front of the inherited entries.
func (t *Toolchain) Path() string {
	return strings.Join(append(t.PathDirs(), os.Getenv("PATH")), string(os.PathListSeparator))
}

// Command prepares a command to run inside the toolchain: the project's tools
// come first on PATH, and a command the toolchain provides is run from the
// install directory rather than looked up. Anything else is resolved against
// that same PATH, so "make" or a script finds the pinned tools too.
func (t *Toolchain) Command(ctx context.Context, name string, args []string) (*exec.Cmd, error) {
	path := t.Path()
	bin, ok := t.ResolveCommand(name)
	if !ok {
		var err error
		bin, err = LookPath(name, path)
		if err != nil {
			return nil, diag.CommandNotFound.Errorf("command %q not found in the locked toolchain or on PATH", name)
		}
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // running the user's command is the point
	cmd.Env = append(os.Environ(), "PATH="+path)
	return cmd, nil
}
