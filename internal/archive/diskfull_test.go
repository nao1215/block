//go:build unix

package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"
	"testing"

	"github.com/nao1215/block/internal/diag"
)

// A compressed artifact becomes very much larger than itself, so extraction is
// where a tight filesystem usually gives out. That failure is the store's, not
// the archive's, and it gets the code that says so.
func TestExtractionOutOfSpaceIsCoded(t *testing.T) {
	t.Parallel()
	for _, errno := range []syscall.Errno{syscall.ENOSPC, syscall.EDQUOT} {
		inner := &fs.PathError{Op: "write", Path: "/home/x/.local/share/block/tools/foundry/.tmp/forge", Err: errno}
		err := diskFull(inner)
		if diag.Of(err) != diag.DiskFull {
			t.Fatalf("diskFull(%v) code = %s, want %s", errno, diag.Of(err), diag.DiskFull)
		}
		// The message says which processing failed and why, and keeps the
		// operating system's own sentence — which names the file.
		msg := err.Error()
		if !strings.Contains(msg, "unpacking the archive") || !strings.Contains(msg, "no room left on the disk") {
			t.Errorf("message = %q", msg)
		}
		if !strings.Contains(msg, inner.Error()) {
			t.Errorf("message %q dropped the underlying error", msg)
		}
		if !errors.Is(err, errno) {
			t.Error("the errno is no longer reachable through the wrapper")
		}
	}
}

// Everything else keeps the code extraction already gave it: an archive block
// refuses is a different report from a disk block cannot write to.
func TestDiskFullLeavesOtherFailuresAlone(t *testing.T) {
	t.Parallel()
	if err := diskFull(nil); err != nil {
		t.Errorf("diskFull(nil) = %v", err)
	}
	refusal := diag.PathEscape.Errorf("refusing to extract %q: path escapes the destination", "../x")
	if got := diskFull(refusal); !errors.Is(got, refusal) || diag.Of(got) != diag.PathEscape {
		t.Errorf("diskFull() changed an unrelated refusal: %v", got)
	}
	plain := fmt.Errorf("read: %w", &fs.PathError{Op: "read", Path: "/x", Err: syscall.EACCES})
	if got := diskFull(plain); !errors.Is(got, plain) || diag.Of(got) != 0 {
		t.Errorf("diskFull() changed a permission failure: %v", got)
	}
}
