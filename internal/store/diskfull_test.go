//go:build unix

package store

import (
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"

	"github.com/nao1215/block/internal/diag"
)

// A store that cannot be written has two very different causes, and the fix
// for one is no use against the other: BLK4005 sends the reader to look at
// permissions, BLK4006 to free space.
func TestWriteFailedTellsAFullDiskFromAnUnwritableStore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want diag.Code
	}{
		{name: "no space", err: syscall.ENOSPC, want: diag.DiskFull},
		{name: "over quota", err: syscall.EDQUOT, want: diag.DiskFull},
		{name: "permission", err: syscall.EACCES, want: diag.StoreUnwritable},
		{name: "read-only filesystem", err: syscall.EROFS, want: diag.StoreUnwritable},
		{name: "not a directory", err: syscall.ENOTDIR, want: diag.StoreUnwritable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inner := &fs.PathError{Op: "mkdir", Path: "/home/x/.local/share/block/tools/gaia", Err: tt.err}
			got := writeFailed("creating /home/x/.local/share/block/tools/gaia", inner)
			if diag.Of(got) != tt.want {
				t.Fatalf("code = %s, want %s (%v)", diag.Of(got), tt.want, got)
			}
			// Whichever it is, the message names the step that failed and
			// keeps what the operating system said.
			if !strings.Contains(got.Error(), "creating /home/x/.local/share/block/tools/gaia") ||
				!strings.Contains(got.Error(), inner.Error()) {
				t.Errorf("message = %q", got.Error())
			}
			if !errors.Is(got, tt.err) {
				t.Error("the errno is no longer reachable through the wrapper")
			}
		})
	}
	// The disk-space reading says so in words as well as in the code, so a
	// reader who does not look the code up still knows what to free.
	msg := writeFailed("marking the install complete", syscall.ENOSPC).Error()
	if !strings.Contains(msg, "no room left on the disk") || !strings.Contains(msg, "quota") {
		t.Errorf("message = %q", msg)
	}
}
