// Package store lays out block's per-user directory: a content-addressed
// download cache and the extracted tool installs. It is shared by every
// project on the machine and by every CI job that restores it.
//
//	$BLOCK_HOME/
//	  cache/sha256/<digest>                 downloaded archives
//	  tools/<name>/<version>-<digest12>/    extracted installs
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nao1215/block/internal/archive"
)

// EnvHome overrides the store location.
const EnvHome = "BLOCK_HOME"

const (
	dirMode     = 0o755
	shortDigest = 12
)

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

// IsInstalled reports whether dir holds a completed install.
func (s *Store) IsInstalled(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

// Install extracts the archive at src into dir atomically: extraction happens
// in a sibling temp directory that is renamed into place only when every
// entry succeeded and every expected executable exists. A concurrent or
// earlier install of the same dir wins silently.
func (s *Store) Install(src, assetName, dir string, bins []string) error {
	if s.IsInstalled(dir) {
		return nil
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
	if err := archive.Extract(src, tmp, assetName); err != nil {
		return fmt.Errorf("extract %s: %w", assetName, err)
	}
	for _, b := range bins {
		st, err := os.Stat(filepath.Join(tmp, filepath.FromSlash(b)))
		if err != nil {
			return fmt.Errorf("archive %s does not contain executable %q", assetName, b)
		}
		if !st.Mode().IsRegular() || st.Mode()&0o100 == 0 {
			return fmt.Errorf("archive %s: %q is not an executable file", assetName, b)
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		if s.IsInstalled(dir) {
			return nil
		}
		return err
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

// ErrNotInstalled reports a lockfile entry that has not been synced.
var ErrNotInstalled = errors.New("not installed")
