// Package fserr answers one question about an error the operating system
// returned: did the write fail because there was nowhere to put the bytes?
//
// block writes in three places — the download cache, the directory an artifact
// is unpacked into, and the project's lockfile — and "the disk is full" is the
// same failure in all of them, with the same fix, so it is worth telling apart
// from every other reason a write can fail. The answer here is structural: the
// error number the kernel returned, matched through [errors.Is] all the way
// down a wrapped chain, rather than the sentence it happens to print. The
// sentence is localised, reworded between releases and different on every
// operating system; the number is not.
package fserr

import "errors"

// OutOfSpace reports whether err is the operating system saying a write had
// nowhere to go: the filesystem is full, or the user is over quota.
//
// It is false for a nil error and for every other kind of failure, including
// the ones that look adjacent — a read-only filesystem, a directory the user
// may not write — because those are a different problem with a different fix.
func OutOfSpace(err error) bool {
	if err == nil {
		return false
	}
	for _, target := range outOfSpace {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
