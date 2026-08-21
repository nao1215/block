package block

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/nao1215/block/internal/diag"
)

// signalExit is the shell convention for a process killed by a signal.
const signalExit = 128

// Exec runs args[0] with the locked toolchain first on PATH and returns its
// exit status. Standard streams are passed straight through, and SIGINT and
// SIGTERM are forwarded to the child rather than acted on here: a node, a
// validator or a local test network must get the chance to shut down
// cleanly, and its own exit status is what block reports.
func (a *App) Exec(ctx context.Context, args []string, stdin *os.File) (int, error) {
	if len(args) == 0 {
		return 0, diag.CommandNotFound.Errorf("exec needs a command to run, e.g. block exec forge --version")
	}
	t, err := a.Toolchain()
	if err != nil {
		return 0, err
	}
	return t.Run(ctx, args[0], args[1:], stdin, a.Stdout, a.Stderr)
}

// Run executes one command inside the toolchain and returns its exit status.
// It is what both "block exec" and a shim end in, so the two cannot drift.
func (t *Toolchain) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd, err := t.Command(ctx, name, args)
	if err != nil {
		return 0, err
	}
	return RunCommand(cmd, name, stdin, stdout, stderr)
}

// RunCommand starts a prepared command, hands it the standard streams and the
// signals block receives, and reports its exit status.
func RunCommand(cmd *exec.Cmd, name string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	// A cancelled context asks the child to stop the way a SIGTERM would,
	// instead of the default SIGKILL: the tools block runs are nodes and
	// test networks that have shutdown work to do. Without a WaitDelay,
	// block waits for as long as the child needs.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if cmd.Dir == "" {
		cmd.Dir, _ = os.Getwd()
	}
	if err := cmd.Start(); err != nil {
		return 0, diag.CommandFailedToStart.Errorf("exec %s: %w", name, err)
	}
	stop := forwardSignals(cmd)
	err := cmd.Wait()
	stop()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exitErr):
		if sig, ok := signalOf(exitErr); ok {
			// 128+signal, as a shell reports it — not the -1 that ExitCode
			// returns for a signalled process.
			return signalExit + int(sig), nil
		}
		return exitErr.ExitCode(), nil
	default:
		return 0, diag.CommandFailedToStart.Errorf("exec %s: %w", name, err)
	}
}

// forwardSignals relays the signals block receives to the child until the
// returned function is called, so that stopping block stops the tool it is
// running rather than orphaning or killing it outright.
func forwardSignals(cmd *exec.Cmd) func() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-sigs:
				_ = cmd.Process.Signal(s)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(sigs)
		close(done)
	}
}
