//go:build windows

package shim

import (
	"io"
	"os"
)

// link points a shim at the block binary. Windows symlinks need Developer
// Mode or an elevated prompt, which block will not ask for, so a hard link is
// tried first — it is free and needs no privilege on the same volume — and a
// copy is the fallback for the case where $BLOCK_HOME lives on another drive
// than block itself.
func link(self, target string) error {
	if err := os.Link(self, target); err == nil {
		return nil
	}
	return copyFile(self, target)
}

func copyFile(self, target string) error {
	in, err := os.Open(self)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only

	tmp := target + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Rename last, so a shim never exists half-written.
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
