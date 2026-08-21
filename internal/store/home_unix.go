//go:build !windows

package store

import (
	"os"
	"path/filepath"
)

// defaultRoot is $XDG_DATA_HOME/block, or ~/.local/share/block — where the
// XDG base directory specification says user-local application data goes.
func defaultRoot() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "block"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "block"), nil
}
