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

	"github.com/nao1215/block/internal/archive"
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

// Store is rooted at one directory.
type Store struct {
	Root string
}

// Open resolves BLOCK_HOME, then $XDG_DATA_HOME/block, then ~/.local/share/block.
func Open() (*Store, error) {
	if root := os.Getenv(EnvHome); root != "" {
		return &Store{Root: root}, nil
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return &Store{Root: filepath.Join(xdg, "block")}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine the block home directory (set %s): %w", EnvHome, err)
	}
	return &Store{Root: filepath.Join(home, ".local", "share", "block")}, nil
}

// CacheDir holds downloaded archives.
func (s *Store) CacheDir() string { return filepath.Join(s.Root, "cache") }

// InstallDir is where an artifact with the given digest is extracted.
func (s *Store) InstallDir(name, ver, sha string) string {
	if len(sha) > shortDigest {
		sha = sha[:shortDigest]
	}
	return filepath.Join(s.Root, "tools", name, ver+"-"+sha)
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
		target, err := binPath(dir, b)
		if err != nil {
			return err
		}
		st, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("executable %q is missing", b)
		}
		if !st.Mode().IsRegular() || st.Mode()&0o100 == 0 {
			return fmt.Errorf("%q is not an executable file", b)
		}
	}
	return nil
}

// binPath resolves an executable path inside dir, refusing anything that
// could leave it. A lockfile is untrusted input, so this is checked again
// here even though parsing already validated it.
func binPath(dir, bin string) (string, error) {
	if err := recipe.ValidateBin(bin); err != nil {
		return "", err
	}
	target := filepath.Join(dir, filepath.FromSlash(bin))
	rel, err := filepath.Rel(dir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid bin entry %q: path escapes the install directory", bin)
	}
	return target, nil
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
		return errors.New("no executables declared")
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
			return fmt.Errorf("removing the incomplete install %s: %w", dir, err)
		}
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, dirMode); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // best-effort cleanup
	switch {
	case recipe.IsArchiveName(assetName):
		if err := archive.Extract(src, tmp, assetName, strip); err != nil {
			return fmt.Errorf("extract %s: %w", assetName, err)
		}
	case len(bins) == 1:
		target, err := binPath(tmp, bins[0])
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
			return err
		}
		if err := copyExecutable(src, target); err != nil {
			return fmt.Errorf("install %s: %w", assetName, err)
		}
	default:
		return fmt.Errorf("raw executable %s needs exactly one bin name", assetName)
	}
	if err := verifyBins(tmp, bins); err != nil {
		return fmt.Errorf("archive %s: %w", assetName, err)
	}
	// The marker goes in last, so the rename publishes a directory that is
	// complete by construction.
	const markerMode = 0o644
	if err := os.WriteFile(filepath.Join(tmp, markerName), []byte(assetName+"\n"), markerMode); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		if s.IsInstalled(dir, bins) {
			return nil
		}
		return err
	}
	return nil
}

// copyExecutable copies a raw binary into place with the executable bit set.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // cache path
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only
	const execMode = 0o755
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, execMode) //nolint:gosec // inside the temp install dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
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
var ErrNotInstalled = errors.New("is not installed")

// Verify reports why an install cannot be used, or nil when it is complete.
func (s *Store) Verify(dir string, bins []string) error {
	if _, err := os.Stat(dir); err != nil {
		return ErrNotInstalled
	}
	if st, err := os.Stat(filepath.Join(dir, markerName)); err != nil || !st.Mode().IsRegular() {
		return errors.New("was installed incompletely")
	}
	if err := verifyBins(dir, bins); err != nil {
		return fmt.Errorf("is damaged: %w", err)
	}
	return nil
}
