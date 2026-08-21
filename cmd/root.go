// Package cmd defines the block command line.
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

// ExitError carries a process exit status from `block exec`.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// Execute runs the CLI and returns the process exit code.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root := newRootCmd(stdout, stderr)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		var exit *ExitError
		if errors.As(err, &exit) {
			return exit.Code
		}
		fmt.Fprintf(stderr, "%s: %v\n", cmdinfo.Name, err)
		return 1
	}
	return 0
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   cmdinfo.Name,
		Short: "Lock your blockchain toolchain",
		Long: `block pins the blockchain CLI tools a project depends on.

Declare tools in block.toml, resolve them into block.lock, and reproduce the
exact same toolchain on every developer machine and in CI:

  block init                 write a starter block.toml
  block lock                 resolve versions and artifacts into block.lock
  block sync [--locked]      install what block.lock says
  block exec forge test      run a locked tool
  block outdated             show newer upstream versions
  block update [tool...]     move block.lock to the newest matching versions`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(
		newInitCmd(stdout, stderr),
		newLockCmd(stdout, stderr),
		newSyncCmd(stdout, stderr),
		newUpdateCmd(stdout, stderr),
		newOutdatedCmd(stdout, stderr),
		newExecCmd(stdout, stderr),
		newRegistryCmd(stdout, stderr),
		newVersionCmd(stdout),
	)
	return root
}

// newApp builds the App for the project that contains the working directory.
// When needManifest is false the working directory itself is used (init).
func newApp(stdout, stderr io.Writer, needManifest bool) (*block.App, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dir := wd
	if needManifest {
		dir, err = manifest.Find(wd)
		if err != nil {
			return nil, err
		}
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

func newInitCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write a starter block.toml in the current directory",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			app, err := newApp(stdout, stderr, false)
			if err != nil {
				return err
			}
			return app.Init()
		},
	}
}

func newLockCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Resolve block.toml into block.lock",
		Long: `lock resolves every tool in block.toml to an exact upstream release and
records its download URL and SHA-256 per platform in block.lock.

Pins that still satisfy block.toml are kept as they are; only new tools,
changed constraints and newly listed platforms are resolved. Use
"block update" to move existing pins forward.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp(stdout, stderr, true)
			if err != nil {
				return err
			}
			return app.Lock(cmd.Context())
		},
	}
}

func newSyncCmd(stdout, stderr io.Writer) *cobra.Command {
	var locked bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Install the toolchain pinned in block.lock",
		Long: `sync downloads, verifies and installs every tool in block.lock for this
machine. Artifacts are cached under $BLOCK_HOME and shared between projects.

Without --locked, a missing or stale block.lock is resolved first.
With --locked (for CI), sync fails instead of resolving anything when
block.lock is missing, disagrees with block.toml, lacks this platform, or
an artifact's checksum does not match.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp(stdout, stderr, true)
			if err != nil {
				return err
			}
			return app.Sync(cmd.Context(), locked)
		},
	}
	cmd.Flags().BoolVar(&locked, "locked", false, "fail instead of resolving when block.lock is missing or stale")
	return cmd
}

func newUpdateCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "update [tool...]",
		Short: "Move block.lock to the newest versions allowed by block.toml",
		Long: `update re-resolves the named tools (all tools when none are named) to the
newest upstream release that still satisfies block.toml, and rewrites
block.lock. To allow a bigger jump, change the constraint in block.toml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newApp(stdout, stderr, true)
			if err != nil {
				return err
			}
			return app.Update(cmd.Context(), args)
		},
	}
}

func newOutdatedCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "outdated",
		Short: "Show locked tools that have a newer matching upstream release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := newApp(stdout, stderr, true)
			if err != nil {
				return err
			}
			return app.Outdated(cmd.Context())
		},
	}
}

func newExecCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "exec <command> [args...]",
		Short: "Run a command with the locked toolchain on PATH",
		Long: `exec runs a command with every executable from block.lock first on PATH
and exits with the command's status. Tools must be installed with
"block sync" first; exec never downloads anything.

  block exec forge test
  block exec -- cast --version`,
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
				return cmd.Help()
			}
			app, err := newApp(stdout, stderr, true)
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

func newRegistryCmd(stdout, _ io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "registry",
		Short: "List the tools the built-in registry knows",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			reg, err := registry.Builtin()
			if err != nil {
				return err
			}
			for _, n := range reg.Names() {
				rec, _ := reg.Lookup(n)
				fmt.Fprintf(stdout, "%s\t%s\n", n, rec.Source.Repo)
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
