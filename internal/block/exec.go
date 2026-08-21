package block

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Exec runs args[0] with the locked toolchain first on PATH and returns its
// exit status. Signals and standard streams are passed straight through.
func (a *App) Exec(ctx context.Context, args []string, stdin *os.File) (int, error) {
	if len(args) == 0 {
		return 0, errors.New("exec needs a command to run, e.g. block exec forge --version")
	}
	dirs, err := a.Env()
	if err != nil {
		return 0, err
	}
	path := strings.Join(append(dirs, os.Getenv("PATH")), string(os.PathListSeparator))
	env := append(os.Environ(), "PATH="+path)
	bin, err := lookPath(args[0], path)
	if err != nil {
		return 0, fmt.Errorf("command %q not found in the locked toolchain or on PATH", args[0])
	}
	cmd := exec.CommandContext(ctx, bin, args[1:]...) //nolint:gosec // running the user's command is the point
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = a.Stdout
	cmd.Stderr = a.Stderr
	cmd.Dir, _ = os.Getwd()
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), nil
	default:
		return 0, fmt.Errorf("exec %s: %w", args[0], err)
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
