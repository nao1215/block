//go:build !unix && !windows

package fserr

// outOfSpace is empty where block has no list for the platform. block ships
// for Linux, macOS and Windows; anywhere else a full disk is reported as
// whatever the operating system said, which is what happened before this
// package existed.
var outOfSpace []error //nolint:gochecknoglobals // the immutable table
