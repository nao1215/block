package fserr

import (
	"errors"
	"io/fs"
	"testing"
)

// Everything that is not the operating system saying "nowhere to put this" is
// a different problem with a different fix, and must not be read as this one.
func TestOutOfSpaceIsFalseForEverythingElse(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		nil,
		errors.New("boom"),
		fs.ErrPermission,
		fs.ErrNotExist,
		&fs.PathError{Op: "open", Path: "/x", Err: fs.ErrPermission},
		// The sentence, without the error number behind it. block reads the
		// number, so a message that merely says the words is not enough —
		// and that is deliberate: an upstream's prose is not an interface.
		errors.New("write /x: no space left on device"),
	} {
		if OutOfSpace(err) {
			t.Errorf("OutOfSpace(%v) = true", err)
		}
	}
}
