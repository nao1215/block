// Package cmd defines the block command line: lock, sync, exec.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/nao1215/block/internal/block"
	"github.com/nao1215/block/internal/cmdinfo"
	"github.com/nao1215/block/internal/fetch"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/store"
	"github.com/nao1215/block/registry"
)

// Exit codes. Errors exit 1 everywhere; `lock --check` additionally uses 2
// for "block.lock would change" so CI can tell the two apart.
const (
	exitOK       = 0
	exitFailure  = 1
	exitOutdated = 2
)

// ExitError carries a process exit status from `block exec`.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// Execute runs the CLI and returns the process exit code.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	var exit *ExitError
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &exit):
		return exit.Code
	case errors.Is(err, block.ErrOutdated):
		return exitOutdated
	}
	fmt.Fprintf(stderr, "%s: %v\n", cmdinfo.Name, err)
	return exitFailure
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   cmdinfo.Name,
		Short: "Lock your blockchain toolchain",
		Long: `block pins the blockchain CLI tools a project depends on.

  block lock    resolves block.toml into block.lock
  block sync    installs the toolchain pinned in block.lock
  block exec    runs a command with the installed toolchain

sync never resolves. exec never installs. lock is the only operation that
can move a pin.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(
		newLockCmd(stdout, stderr),
		newSyncCmd(stdout, stderr),
		newExecCmd(stdout, stderr),
		newVersionCmd(stdout),
	)
	return root
}

// newApp builds the App for the project that contains the working directory.
func newApp(stdout, stderr io.Writer) (*block.App, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dir, err := manifest.Find(wd)
	if err != nil {
		return nil, err
	}
	reg, err := registry.Builtin()
	if err != nil {
		return nil, err
	}
	st, err := store.Open()
	if err != nil {
		return nil, err
	}
	ua := cmdinfo.UserAgent()
	return &block.App{
		Dir:      dir,
		Platform: platform.Current(),
		Registry: reg,
		Releases: github.NewFromEnv(ua),
		Fetcher:  fetch.New(st.CacheDir(), ua),
		Store:    st,
		Stdout:   stdout,
		Stderr:   stderr,
	}, nil
}

func newLockCmd(stdout, stderr io.Writer) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "lock [tool...]",
		Short: "Resolve block.toml into block.lock",
		Long: `lock resolves every tool in block.toml to the newest upstream release its
constraint allows and records the download URL and SHA-256 per platform in
block.lock. Naming tools re-resolves only those and keeps the other pins.

lock is the only command that moves a pin. Commit block.lock.

With --check, lock performs the same resolution but writes nothing:
exit 0 when block.lock is current, 2 when it would change, 1 on error.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(stdout, stderr)
			if err != nil {
				return err
			}
			return app.Lock(cmd.Context(), args, check)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "report what lock would change without writing block.lock")
	return cmd
}

func newSyncCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Install the toolchain pinned in block.lock",
		Long: `sync downloads, verifies and installs every artifact block.lock names for
this machine. Artifacts are cached under $BLOCK_HOME and shared between
projects, so a warm cache makes sync an offline operation.

sync never resolves a version and never writes block.lock. It fails when
block.lock is missing, disagrees with block.toml, lacks this platform, or an
artifact's checksum does not match. The behaviour is identical locally and
in CI.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp(stdout, stderr)
			if err != nil {
				return err
			}
			return app.Sync(cmd.Context())
		},
	}
}

func newExecCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "exec <command> [args...]",
		Short: "Run a command with the installed toolchain on PATH",
		Long: `exec runs a command with every executable from block.lock first on PATH
and exits with the command's status. Any command works, not only the locked
tools, so build scripts that call them see the locked versions:

  block exec forge test
  block exec make test
  block exec ./scripts/integration-test.sh

exec never downloads, installs or resolves anything; run "block sync" first.`,
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
				return cmd.Help()
			}
			app, err := newApp(stdout, stderr)
			if err != nil {
				return err
			}
			code, err := app.Exec(cmd.Context(), args, os.Stdin)
			if err != nil {
				return err
			}
			if code != 0 {
				return &ExitError{Code: code}
			}
			return nil
		},
	}
}

func newVersionCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the block version",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Fprintf(stdout, "%s %s\n", cmdinfo.Name, cmdinfo.Version)
			return nil
		},
	}
}
