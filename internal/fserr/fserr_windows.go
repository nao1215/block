//go:build windows

package fserr

import "syscall"

// Windows reports a full volume with its own error codes, and Go's syscall
// package does not name them — syscall.ENOSPC there is a placeholder in the
// APPLICATION_ERROR range that no API ever returns, so matching it would match
// nothing. These are the values from winerror.h, which is where they are
// stable. An NTFS disk quota is reported as ERROR_DISK_FULL as well: to a
// process that has reached one, the volume is full.
const (
	errorHandleDiskFull syscall.Errno = 39
	errorDiskFull       syscall.Errno = 112
)

// outOfSpace are the error codes a Windows write returns when the volume has
// no room left.
var outOfSpace = []error{errorHandleDiskFull, errorDiskFull} //nolint:gochecknoglobals // the immutable table
