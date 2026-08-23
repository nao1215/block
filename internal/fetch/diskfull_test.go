//go:build unix

package fetch

import (
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"

	"github.com/nao1215/block/internal/diag"
)

// Bytes that arrived and had nowhere to go are a full disk, not a failed
// transfer: "the download failed" would send the reader to look at their
// network for a problem that is entirely local.
func TestCacheErrTellsAFullDiskFromAnUnwritableCache(t *testing.T) {
	t.Parallel()
	f := &Fetcher{Dir: "/home/x/.local/share/block/cache"}
	tests := []struct {
		name string
		err  error
		want diag.Code
	}{
		{name: "no space", err: syscall.ENOSPC, want: diag.DiskFull},
		{name: "over quota", err: syscall.EDQUOT, want: diag.DiskFull},
		{name: "permission", err: syscall.EACCES, want: diag.StoreUnwritable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inner := &fs.PathError{Op: "write", Path: f.Dir + "/.download-1", Err: tt.err}
			got := f.cacheErr(inner)
			if diag.Of(got) != tt.want {
				t.Fatalf("code = %s, want %s (%v)", diag.Of(got), tt.want, got)
			}
			if !strings.Contains(got.Error(), f.Dir) || !errors.Is(got, tt.err) {
				t.Errorf("message = %q", got.Error())
			}
		})
	}
	if msg := f.cacheErr(syscall.ENOSPC).Error(); !strings.Contains(msg, "no room left on the disk") {
		t.Errorf("message = %q", msg)
	}
}
