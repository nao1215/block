package block

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
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
		return 0, errors.New("exec needs a command to run, e.g. block exec forge --version")
	}
	dirs, err := a.Env()
	if err != nil {
		return 0, err
	}
	path := strings.Join(append(dirs, os.Getenv("PATH")), string(os.PathListSeparator))
	bin, err := lookPath(args[0], path)
	if err != nil {
		return 0, fmt.Errorf("command %q not found in the locked toolchain or on PATH", args[0])
	}
	cmd := exec.CommandContext(ctx, bin, args[1:]...) //nolint:gosec // running the user's command is the point
	// A cancelled context asks the child to stop the way a SIGTERM would,
	// instead of the default SIGKILL: the tools block runs are nodes and
	// test networks that have shutdown work to do. Without a WaitDelay,
	// block waits for as long as the child needs.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.Env = append(os.Environ(), "PATH="+path)
	cmd.Stdin = stdin
	cmd.Stdout = a.Stdout
	cmd.Stderr = a.Stderr
	cmd.Dir, _ = os.Getwd()
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("exec %s: %w", args[0], err)
	}
	stop := forwardSignals(cmd)
	err = cmd.Wait()
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
		return 0, fmt.Errorf("exec %s: %w", args[0], err)
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

// lookPath resolves name against an explicit PATH value instead of the
// process environment.
func lookPath(name, path string) (string, error) {
	if strings.Contains(name, string(os.PathSeparator)) {
		return exec.LookPath(name)
	}
	for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
		if dir == "" {
			continue
		}
		candidate := dir + string(os.PathSeparator) + name
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 { //nolint:gosec // PATH lookup by design
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}
