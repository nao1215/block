//go:build windows

package fserr

import (
	"io/fs"
	"syscall"
	"testing"
)

// Windows names a full volume with its own codes, and syscall.ENOSPC there is
// a placeholder no API returns — so matching it would match nothing, and this
// pins that the real ones are what is matched.
func TestOutOfSpaceWindows(t *testing.T) {
	t.Parallel()
	for _, errno := range []syscall.Errno{errorHandleDiskFull, errorDiskFull} {
		if !OutOfSpace(&fs.PathError{Op: "write", Path: `C:\x`, Err: errno}) {
			t.Errorf("OutOfSpace(%d) = false, want true", uintptr(errno))
		}
	}
	if OutOfSpace(&fs.PathError{Op: "write", Path: `C:\x`, Err: syscall.ERROR_ACCESS_DENIED}) {
		t.Error("access denied was read as a full disk")
	}
}
