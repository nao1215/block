//go:build !windows

package shim

import "os"

// link points a shim at the block binary. A symlink costs nothing and keeps
// following the binary when it is replaced in place.
func link(self, target string) error {
	return os.Symlink(self, target)
}
