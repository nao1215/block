//go:build unix

package block

import (
	"os/exec"
	"syscall"
)

// signalOf reports the signal that killed a process, if one did.
func signalOf(err *exec.ExitError) (syscall.Signal, bool) {
	ws, ok := err.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return 0, false
	}
	return ws.Signal(), true
}
