//go:build unix

package block

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// lockedBuffer is a bytes.Buffer the child may write to while the test reads.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A signal block receives reaches the child once, as the signal it was, and
// the child's own exit status is what block reports — including 0 after a
// clean shutdown.
func TestRunCommandForwardsASignalOnceAndKeepsACleanExit(t *testing.T) { //nolint:paralleltest // signals the whole test process
	// A handler sets a flag the loop reads, rather than ending a background
	// sleep: every sh runs a trap once the foreground command returns, and
	// nothing is left behind holding the pipes Wait reads to their end.
	script := `trap 'echo got INT; stop=1' INT; trap 'echo got TERM; stop=1' TERM; echo ready; while [ -z "$stop" ]; do sleep 0.1; done; echo bye; exit 0`
	cmd := exec.Command("sh", "-c", script) //nolint:noctx // what Toolchain.Command builds: unbound from any context
	var out lockedBuffer
	done := make(chan struct {
		code int
		err  error
	}, 1)
	go func() {
		code, err := RunCommand(cmd, "sh", nil, &out, &out)
		done <- struct {
			code int
			err  error
		}{code, err}
	}()
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(out.String(), "ready") {
		if time.Now().After(deadline) {
			t.Fatalf("child never became ready: %q", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The signal block itself would get from a Ctrl-C; forwardSignals is
	// what keeps the test process alive through it.
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		if r.err != nil || r.code != 0 {
			t.Fatalf("RunCommand = %d, %v; want 0, nil (output %q)", r.code, r.err, out.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("child did not exit after SIGINT: %q", out.String())
	}
	if got, want := out.String(), "ready\ngot INT\nbye\n"; got != want {
		t.Fatalf("child saw %q, want %q", got, want)
	}
}
