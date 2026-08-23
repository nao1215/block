//go:build unix

package fserr

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
	"testing"
)

// The two errno values a Unix kernel has for "nowhere to put this", found
// however deep in a wrapped chain they are — which is where they arrive, since
// os wraps them in a PathError and block wraps that again on the way out.
func TestOutOfSpaceUnix(t *testing.T) {
	t.Parallel()
	for _, errno := range []syscall.Errno{syscall.ENOSPC, syscall.EDQUOT} {
		for _, err := range []error{
			errno,
			&fs.PathError{Op: "write", Path: "/x/forge", Err: errno},
			fmt.Errorf("extract foundry.tar.gz: %w", &fs.PathError{Op: "write", Path: "/x/forge", Err: errno}),
		} {
			if !OutOfSpace(err) {
				t.Errorf("OutOfSpace(%v) = false, want true", err)
			}
		}
	}
	// The neighbours: a write can fail for these too, and neither is fixed by
	// freeing space.
	for _, errno := range []syscall.Errno{syscall.EACCES, syscall.EROFS, syscall.ENOENT, syscall.EEXIST} {
		if OutOfSpace(&fs.PathError{Op: "write", Path: "/x", Err: errno}) {
			t.Errorf("OutOfSpace(%v) = true", errno)
		}
	}
	// errors.Is is what does the matching, so an error that only claims to be
	// ENOSPC through Is is matched as well.
	if !OutOfSpace(fmt.Errorf("wrapped: %w", errors.Join(errors.New("a"), syscall.ENOSPC))) {
		t.Error("a joined ENOSPC was not found")
	}
}
