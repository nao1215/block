//go:build unix

package fserr

import "syscall"

// outOfSpace are the two errno values a Unix kernel returns when a write has
// nowhere to go: the filesystem itself is full (ENOSPC), or this user has
// reached the limit set for them on a filesystem that is not (EDQUOT). They
// are separate numbers because they are separate things to fix, and block
// reports them together because it cannot fix either.
var outOfSpace = []error{syscall.ENOSPC, syscall.EDQUOT} //nolint:gochecknoglobals // the immutable table
