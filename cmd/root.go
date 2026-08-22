// Package cmd defines the block command line: lock, sync, exec.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nao1215/block/internal/block"
	"github.com/nao1215/block/internal/cmdinfo"
	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/fetch"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/shim"
	"github.com/nao1215/block/internal/store"
	"github.com/nao1215/block/registry"
)

// Exit codes. Errors exit 1 everywhere; 2 is not an error but a result — the
// lockfile would change, or the toolchain is not ready — so that CI can tell
// "there is something to do" from "block could not do its job".
const (
	exitOK        = 0
	exitFailure   = 1
	exitNeedsWork = 2
)

// ExitError carries a process exit status from `block exec`.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// Main is the process entry point: it decides whether this invocation is
// block itself or one of its shims. A shim is the same binary under a
// command's name — forge, cast, geth — and runs that command from the
// toolchain the working directory's project locked.
func Main(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(argv) > 0 && shim.IsShim(argv[0]) {
		return runShim(ctx, argv[0], argv[1:], stdin, stdout, stderr)
	}
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}
	return Execute(ctx, args, stdout, stderr)
}

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
	case errors.Is(err, block.ErrOutdated), errors.Is(err, block.ErrNotReady):
		return exitNeedsWork
	}
	// The code goes at the front of the line, once, rather than wherever in a
	// wrapped chain the problem was named: it is what a reader searches for
	// and what a reviewer quotes.
	fmt.Fprintf(stderr, "%s: %s\n", cmdinfo.Name, diag.Message(err))
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
can move a pin.

  block status  shows what block.toml, block.lock and the store say
  block list    shows the tools block supports, all or by ecosystem
  block explain names what one of block's BLK error codes means

` + Links,
		// The same answer as `block version`, printed by the same code: a
		// binary that reports one version to a flag and another to a
		// subcommand is a binary nobody can quote.
		Version:       cmdinfo.Resolve(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	// cobra renders the version template with text/template, and this text is
	// data — a version string and a registry revision — not a template. The
	// one sequence that would turn it into one is escaped, so `--version` and
	// `version` cannot diverge over a brace.
	root.SetVersionTemplate(strings.ReplaceAll(versionText(), "{{", `{{"{{"}}`))
	root.AddCommand(
		newLockCmd(stdout, stderr),
		newSyncCmd(stdout, stderr),
		newExecCmd(stdout, stderr),
		newStatusCmd(stdout, stderr),
		newListCmd(stdout),
		newExplainCmd(stdout),
		newVersionCmd(stdout),
	)
	return root
}

// Links is the footer every help screen ends with. A CLI that refuses is the
// place a person is standing when they need the documentation, a way to
// report what went wrong, or a reason to believe the project is maintained,
// so the addresses are in the binary rather than only in a README they are
// not reading.
const Links = `Documentation:   https://nao1215.github.io/block/
Error codes:     https://nao1215.github.io/block/errors/
Report a bug:    https://github.com/nao1215/block/issues/new
GitHub Sponsors: https://github.com/sponsors/nao1215`

// versionText is what both `block version` and `block --version` print. It
// doubles as the cobra version template, which is why it ends in a newline
// and carries no template actions.
func versionText() string {
	text := fmt.Sprintf("%s %s\n", cmdinfo.Name, cmdinfo.Resolve())
	// The recipes are vendored, so "which block" does not by itself answer
	// "which registry". A binary that cannot say which revision it was built
	// from cannot be matched to the recipes it resolves with, which is the
	// pairing the snapshot exists for.
	if s, err := registry.Snapshot(); err == nil {
		text += fmt.Sprintf("registry %s (%d recipes from %s)\n", s.Revision[:shortRevision], s.Recipes, s.Source)
	}
	return text
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

func newStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show what block.toml, block.lock and the store say",
		Long: `status reports one line per tool: the constraint block.toml asks for, the
version block.lock pins, whether that version is installed for this machine,
and what to do about the difference.

  ok        installed and matching block.toml
  missing   locked, not installed          -> block sync
  damaged   installed but not usable       -> block sync
  stale     block.lock does not match block.toml -> block lock

It changes nothing: no resolution, no network, no download, no install, no
shims, no lockfile. It answers the same way with the network unplugged, and a
project it reports on is in exactly the state it was before.

Exit 0 when every tool is ok, 2 when something needs doing, 1 on error.
With --json, the same report as an object for CI and other tools.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			app, err := newApp(stdout, stderr)
			if err != nil {
				return err
			}
			report, err := app.Status()
			if err != nil {
				return err
			}
			if err := printStatus(stdout, stderr, report, asJSON); err != nil {
				return err
			}
			if !report.Ready {
				return block.ErrNotReady
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the report as a JSON object instead of a table")
	return cmd
}

// printStatus renders a report. The table is the whole of stdout, so that a
// reader and a `grep` see the same thing; what is not about one tool — a
// manifest that does not name this machine — goes to stderr, and the JSON
// form carries it in the report instead.
func printStatus(stdout, stderr io.Writer, report *block.Status, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0) //nolint:mnd // column padding
	fmt.Fprintln(tw, "TOOL\tWANTED\tLOCKED\tINSTALLED\tSTATE")
	for _, t := range report.Tools {
		state := string(t.State)
		if t.Detail != "" {
			state += " (" + t.Detail + ")"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.Name, orDash(t.Wanted), orDash(t.Locked), orDash(t.Installed), state)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if hint := report.Hint(); hint != "" {
		fmt.Fprintf(stdout, "\n%s\n", hint)
	}
	for _, note := range report.Notes {
		fmt.Fprintf(stderr, "note: %s\n", note)
	}
	return nil
}

// orDash renders an empty column the way a table says "there is none".
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newListCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list [ecosystem]",
		Short: "List the tools block supports",
		Long: `List tools supported by block.

Without an argument, lists every tool with the blockchain systems it serves.
With an ecosystem, lists that system's tools and the commands each provides:

  block list
  block list ethereum

Tools are discovered here and chosen by you: listing an ecosystem never adds
anything to block.toml. list is read-only and offline — it reads the registry
snapshot embedded in this binary, resolves nothing, downloads nothing, and
needs neither block.toml nor block.lock. Project-local tools are not listed;
a project's own toolchain is its block.toml and block.lock.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			reg, err := registry.Builtin()
			if err != nil {
				return err
			}
			recipes := reg.Recipes()
			byEcosystem := len(args) == 1
			if byEcosystem {
				if !slices.Contains(reg.Ecosystems(), args[0]) {
					return diag.UnknownEcosystem.Errorf("unknown ecosystem %q\navailable ecosystems: %s", args[0], strings.Join(reg.Ecosystems(), ", "))
				}
				recipes = reg.ByEcosystem(args[0])
			}
			tw := tabwriter.NewWriter(stdout, 0, 0, 3, ' ', 0) //nolint:mnd // column padding
			// How a tool is obtained is a registry concern, so the columns
			// answer the two questions a reader has instead: what is this,
			// and what do I get. Narrowing to one ecosystem trades that
			// column — every row would repeat it — for the commands.
			if byEcosystem {
				fmt.Fprintln(tw, "NAME\tCOMMANDS\tDESCRIPTION")
			} else {
				fmt.Fprintln(tw, "NAME\tECOSYSTEM\tDESCRIPTION")
			}
			for _, rec := range recipes {
				if byEcosystem {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", rec.Name, strings.Join(commandNames(rec.Source.Bin), ", "), rec.Description)
					continue
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", rec.Name, strings.Join(rec.Ecosystems, ", "), rec.Description)
			}
			return tw.Flush()
		},
	}
}

// commandNames reduces archive-relative executable paths to the command
// names a user types. What "the command name" is belongs to the recipe
// package, which is where every other caller asks.
func commandNames(bins []string) []string {
	out := make([]string, len(bins))
	for i, b := range bins {
		out[i] = recipe.CommandName(b)
	}
	return out
}

func newVersionCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the block version and the registry snapshot it carries",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Fprint(stdout, versionText())
			return nil
		},
	}
}

// shortRevision is how much of the registry revision `block version` prints:
// enough to find the commit, short enough to sit on one line.
const shortRevision = 12
