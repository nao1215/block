// Package store lays out block's per-user directory: a content-addressed
// download cache and the extracted tool installs. It is shared by every
// project on the machine and by every CI job that restores it.
//
//	$BLOCK_HOME/
//	  cache/sha256/<digest>                 downloaded archives
//	  tools/<name>/<version>-<digest12>/    extracted installs
//	  tools/<name>/<version>-<digest12>/.block-installed   completion marker
//
// The marker is written inside the temporary directory that is renamed into
// place, so it appears only with a complete install. A directory without it
// — a half-restored CI cache, an interrupted copy — is not trusted.
package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nao1215/block/internal/archive"
	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/fserr"
	"github.com/nao1215/block/internal/recipe"
)

// EnvHome overrides the store location.
const EnvHome = "BLOCK_HOME"

const (
	dirMode     = 0o755
	shortDigest = 12
)

// markerName marks an install as complete. It is the last file written before
// the atomic rename.
const markerName = ".block-installed"

// tmpInfix is in the name of the directory an install is built in before it
// is renamed into place: "tools/<name>/.<version>-<digest12>.tmp-<random>".
const tmpInfix = ".tmp-"

// Store is rooted at one directory.
type Store struct {
	Root string
}

// Open resolves BLOCK_HOME, and otherwise the platform's own place for
// user-local application data.
func Open() (*Store, error) {
	if root := os.Getenv(EnvHome); root != "" {
		return &Store{Root: root}, nil
	}
	root, err := defaultRoot()
	if err != nil {
		return nil, diag.StoreUnwritable.Errorf("cannot determine the block home directory (set %s): %w", EnvHome, err)
	}
	return &Store{Root: root}, nil
}

// CacheDir holds downloaded archives.
func (s *Store) CacheDir() string { return filepath.Join(s.Root, "cache") }

// InstallDir is where an artifact with the given digest is extracted.
//
// The name and the version reach here from block.lock, which arrives through
// pull requests and hand edits, and the result is a directory this package
// creates, populates and removes. Both are validated upstream — the name by
// [recipe.ValidateName] and the version by [version.Parse], whose alphabet
// admits no separator — and validated again here, because the cost of being
// wrong is os.RemoveAll on a path outside $BLOCK_HOME.
func (s *Store) InstallDir(name, ver, sha string) (string, error) {
	if len(sha) > shortDigest {
		sha = sha[:shortDigest]
	}
	if err := safeComponent("tool name", name); err != nil {
		return "", diag.UnsafeStorePath.Errorf("refusing to install %s %s: %w", name, ver, err)
	}
	if err := safeComponent("version", ver); err != nil {
		return "", diag.UnsafeStorePath.Errorf("refusing to install %s %s: %w", name, ver, err)
	}
	toolsRoot := filepath.Join(s.Root, "tools")
	dir := filepath.Join(toolsRoot, name, ver+"-"+sha)
	if err := containedIn(toolsRoot, dir); err != nil {
		return "", diag.UnsafeStorePath.Errorf("refusing to install %s %s: %w", name, ver, err)
	}
	return dir, nil
}

// safeComponent requires a value to be exactly one path component that names
// something: not empty, not a directory reference, and carrying no separator
// of either flavour. Checking the parts rather than only the joined result
// matters because filepath.Join cleans an absolute component into a relative
// one — "/etc/passwd" as a tool name would land inside the store rather than
// outside it, and would still be nonsense on disk.
func safeComponent(what, v string) error {
	switch {
	case v == "":
		return fmt.Errorf("%s is empty", what)
	case v == "." || v == "..":
		return fmt.Errorf("%s %q is a directory reference", what, v)
	case strings.ContainsRune(v, '/'), strings.ContainsRune(v, '\\'):
		return fmt.Errorf("%s %q contains a path separator", what, v)
	case strings.ContainsRune(v, 0):
		return fmt.Errorf("%s %q contains a NUL", what, v)
	case v != filepath.Clean(v):
		return fmt.Errorf("%s %q is not a plain path component", what, v)
	}
	return nil
}

// containedIn reports whether dir is strictly below root once both are
// cleaned. safeComponent already refuses the ways in, so this is the check
// that has to keep holding if one is ever missed.
func containedIn(root, dir string) error {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(dir))
	if err != nil {
		return fmt.Errorf("install path %q is not under %q", dir, root)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("install path %q escapes %q", dir, root)
	}
	return nil
}

// IsInstalled reports whether dir holds a complete, usable install: the
// completion marker is present and every declared executable is there and
// executable. Anything else is treated as absent, so a damaged install is
// replaced instead of run.
func (s *Store) IsInstalled(dir string, bins []string) bool {
	if st, err := os.Stat(filepath.Join(dir, markerName)); err != nil || !st.Mode().IsRegular() {
		return false
	}
	return verifyBins(dir, bins) == nil
}

// verifyBins checks that every declared executable exists inside dir and can
// be run.
func verifyBins(dir string, bins []string) error {
	for _, b := range bins {
		target, err := BinPath(dir, b)
		if err != nil {
			return err
		}
		st, err := os.Stat(target)
		if err != nil {
			return diag.ExecutableMissing.Errorf("executable %q is missing", b)
		}
		if !isExecutable(st) {
			return diag.ExecutableMissing.Errorf("%q is not an executable file", b)
		}
	}
	return nil
}

// BinPath resolves an executable path inside dir, refusing anything that
// could leave it, and adds the platform's executable suffix. A lockfile is
// untrusted input, so this is checked again here even though parsing already
// validated it.
func BinPath(dir, bin string) (string, error) {
	if err := recipe.ValidateBin(bin); err != nil {
		return "", err
	}
	target := filepath.Join(dir, filepath.FromSlash(bin)) + ExeSuffix
	rel, err := filepath.Rel(dir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", diag.PathEscape.Errorf("invalid bin entry %q: path escapes the install directory", bin)
	}
	return target, nil
}

// writeFailed names a write into the store that did not happen. A filesystem
// with no room left gets a code of its own: what the reader has to go and free
// is not what they would go and check the permissions of, and telling them to
// do the second when it is the first wastes the trip.
func writeFailed(what string, err error) error {
	if fserr.OutOfSpace(err) {
		return diag.DiskFull.Errorf("%s: there is no room left on the disk, or a quota is exhausted: %w", what, err)
	}
	return diag.StoreUnwritable.Errorf("%s: %w", what, err)
}

// Install places the artifact at src into dir atomically. An archive is
// extracted (dropping strip leading components); a raw executable is copied
// under the single name in bins. Work happens in a sibling temp directory
// that is renamed into place only when every entry succeeded and every
// expected executable exists. A concurrent or earlier install of the same
// dir wins silently.
func (s *Store) Install(src, assetName, dir string, bins []string, strip int) error {
	if s.IsInstalled(dir, bins) {
		return nil
	}
	if len(bins) == 0 {
		return diag.ExecutableMissing.Errorf("no executables declared")
	}
	for _, b := range bins {
		if err := recipe.ValidateBin(b); err != nil {
			return err
		}
	}
	// A directory that exists but does not verify is a damaged install:
	// remove it so the fresh one can take its place.
	if _, err := os.Stat(dir); err == nil {
		if err := os.RemoveAll(dir); err != nil {
			return diag.StoreUnwritable.Errorf("removing the incomplete install %s: %w", dir, err)
		}
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, dirMode); err != nil {
		return writeFailed("creating "+parent, err)
	}
	// Everything is built in a sibling temporary directory and renamed into
	// place at the end, so a failure anywhere below leaves the store exactly
	// as it was — including an install of this very tool that is already
	// there and working.
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+tmpInfix+"*")
	if err != nil {
		return writeFailed("creating a build directory in "+parent, err)
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // best-effort cleanup
	switch {
	case recipe.IsArchiveName(assetName):
		if err := archive.Extract(src, tmp, assetName, strip); err != nil {
			return fmt.Errorf("extract %s: %w", assetName, err)
		}
	case len(bins) == 1:
		target, err := BinPath(tmp, bins[0])
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
			return writeFailed("creating "+filepath.Dir(target), err)
		}
		if err := copyExecutable(src, target); err != nil {
			return fmt.Errorf("install %s: %w", assetName, err)
		}
	default:
		return diag.ExecutableMissing.Errorf("raw executable %s needs exactly one bin name", assetName)
	}
	if err := ensureExecutable(tmp, bins); err != nil {
		return fmt.Errorf("archive %s: %w", assetName, err)
	}
	if err := verifyBins(tmp, bins); err != nil {
		return fmt.Errorf("archive %s: %w", assetName, err)
	}
	// The marker goes in last, so the rename publishes a directory that is
	// complete by construction.
	const markerMode = 0o644
	if err := os.WriteFile(filepath.Join(tmp, markerName), []byte(assetName+"\n"), markerMode); err != nil {
		return writeFailed("marking the install complete", err)
	}
	if err := os.Rename(tmp, dir); err != nil {
		if s.IsInstalled(dir, bins) {
			return nil
		}
		return writeFailed("publishing the install as "+dir, err)
	}
	return nil
}

// ensureExecutable gives the executable bit to the paths the recipe declared
// as executables, and to nothing else. Some upstreams package a binary 0644
// (Lotus does), and a raw-executable asset arrives with no mode at all — that
// one is already handled on copy, so this makes an archive behave the same
// way. A path that is missing is left to verifyBins, which says so plainly.
func ensureExecutable(dir string, bins []string) error {
	const binMode = 0o755
	for _, b := range bins {
		target, err := BinPath(dir, b)
		if err != nil {
			return err
		}
		st, err := os.Stat(target)
		if err != nil || !st.Mode().IsRegular() || isExecutable(st) {
			continue
		}
		if err := os.Chmod(target, st.Mode().Perm()|binMode); err != nil {
			return diag.StoreUnwritable.Errorf("making %q executable: %w", b, err)
		}
	}
	return nil
}

// copyExecutable copies a raw binary into place with the executable bit set.
//
// A copy has two ends and only one of them is the install directory. Reading
// the cached artifact is not a write into the store, so a failure there keeps
// the plain error it has always had rather than being reported as one; only
// the destination goes through [writeFailed], where a filesystem with no room
// left earns a code of its own.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // cache path
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only
	const execMode = 0o755
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, execMode) //nolint:gosec // inside the temp install dir
	if err != nil {
		return writeFailed("creating "+dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		// io.Copy does not say which end gave out, so only a failure the
		// operating system attributes to the disk is named as the store's.
		if fserr.OutOfSpace(err) {
			return writeFailed("writing "+dst, err)
		}
		return err
	}
	// A close is the destination alone: it is where the last of the buffered
	// bytes are written, which is often where a full disk is first noticed.
	if err := out.Close(); err != nil {
		return writeFailed("writing "+dst, err)
	}
	return nil
}

// BinDirs returns the directories that must be prepended to PATH so that the
// listed executables of an install resolve, without duplicates.
func BinDirs(installDir string, bins []string) []string {
	var dirs []string
	seen := map[string]bool{}
	for _, b := range bins {
		d := filepath.Join(installDir, filepath.Dir(filepath.FromSlash(b)))
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// ErrNotInstalled reports a tool that was never installed, as opposed to one
// whose install is damaged.
var ErrNotInstalled = diag.NotInstalled.Wrap(errors.New("is not installed"))

// Verify reports why an install cannot be used, or nil when it is complete.
func (s *Store) Verify(dir string, bins []string) error {
	if _, err := os.Stat(dir); err != nil {
		return ErrNotInstalled
	}
	if st, err := os.Stat(filepath.Join(dir, markerName)); err != nil || !st.Mode().IsRegular() {
		return diag.InstallDamaged.Errorf("was installed incompletely")
	}
	if err := verifyBins(dir, bins); err != nil {
		return diag.InstallDamaged.Errorf("is damaged: %w", err)
	}
	return nil
}

// SweepTemp removes the build directories interrupted installs left behind —
// a block killed mid-extraction never reaches the deferred cleanup — once
// they are older than olderThan. An install in progress is younger than
// that, so a sweep running beside another sync never takes anything from it.
// Errors are not reported: a sweep is housekeeping, and the install that
// follows says what is wrong with the store if something is.
func (s *Store) SweepTemp(olderThan time.Duration) {
	tools, err := os.ReadDir(filepath.Join(s.Root, "tools"))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, tool := range tools {
		if !tool.IsDir() {
			continue
		}
		dir := filepath.Join(s.Root, "tools", tool.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || !strings.HasPrefix(name, ".") || !strings.Contains(name, tmpInfix) {
				continue
			}
			info, err := e.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			_ = os.RemoveAll(filepath.Join(dir, name))
		}
	}
}
