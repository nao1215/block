//go:build !unix

package block

import (
	"os/exec"
	"syscall"
)

// signalOf has no meaning where processes are not killed by signals.
func signalOf(*exec.ExitError) (syscall.Signal, bool) { return 0, false }
