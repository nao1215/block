//go:build windows

package store

import (
	"os"
	"path/filepath"
)

// defaultRoot is %LOCALAPPDATA%\block: per-user, not roamed onto every
// machine the account signs in to, which is what a cache of downloaded
// binaries should be. os.UserCacheDir reads the same variable and falls back
// the same way.
func defaultRoot() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "block"), nil
}
