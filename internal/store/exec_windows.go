//go:build windows

package store

import (
	"io/fs"
)

// ExeSuffix is what an executable file name ends with on this platform.
// Recipes and lockfiles name executables without it — "geth", not "geth.exe" —
// so that one lockfile describes every platform.
const ExeSuffix = ".exe"

// isExecutable reports whether a file can be run. Windows has no executable
// bit: what makes a file runnable is its extension, which ExeSuffix already
// pinned when the path was built.
func isExecutable(fi fs.FileInfo) bool {
	return fi.Mode().IsRegular()
}
