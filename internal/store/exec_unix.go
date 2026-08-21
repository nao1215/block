//go:build !windows

package store

import (
	"io/fs"
)

// ExeSuffix is what an executable file name ends with on this platform.
const ExeSuffix = ""

// isExecutable reports whether a file can be run. On Unix that is the
// executable bit; a tool that unpacked without it is broken, not usable.
func isExecutable(fi fs.FileInfo) bool {
	return fi.Mode().IsRegular() && fi.Mode()&0o100 != 0
}
