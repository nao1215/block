//go:build unix

package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// A raw executable is copied from the download cache into the store, and only
// one end of that copy is the store. A cached artifact that has gone missing
// is the cache's problem, and reporting it as a store that cannot be written
// would send the reader to check the permissions of a directory block had no
// trouble with.
func TestInstallDoesNotBlameTheStoreForAnUnreadableSource(t *testing.T) {
	t.Parallel()
	s := &Store{Root: t.TempDir()}
	dir, err := s.InstallDir("solc", "0.8.30", "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	err = s.Install(filepath.Join(t.TempDir(), "gone"), "solc-static-linux", dir, []string{"solc"}, 0)
	if err == nil {
		t.Fatal("Install() accepted a source that is not there")
	}
	if code := diag.Of(err); code == diag.StoreUnwritable || code == diag.DiskFull {
		t.Errorf("code = %s, want the store not to be blamed for the cache (%v)", code, err)
	}
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "install solc-static-linux") {
		t.Errorf("err = %v, want the missing source named", err)
	}
	// And nothing was published: the install directory is not there.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("a failed install left %s behind: %v", dir, err)
	}
}
