package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/nao1215/block/internal/block"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/shim"
	"github.com/nao1215/block/internal/store"
)

// depthEnv guards against two shim directories on PATH handing a command back
// and forth. Nothing else reads it.
const depthEnv = "BLOCK_SHIM_DEPTH"

// maxDepth is how many shims a single command may pass through.
const maxDepth = 8

// Run executes a command through its shim: it finds the project the working
// directory belongs to, reads that project's lockfile, and runs the version
// it pins. It never resolves a version, never downloads, never installs and
// never writes — a project whose tools are not installed is an error that
// says so.
//
// Outside a block project, or for a command this project does not lock, the
// next command of that name on PATH runs instead, so putting the shim
// directory on PATH cannot take a tool away from the rest of the system.
func runShim(ctx context.Context, argv0 string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	command := shim.CommandName(argv0)
	depth, _ := strconv.Atoi(os.Getenv(depthEnv))
	if depth >= maxDepth {
		fmt.Fprintf(stderr, "block: %s: shims are calling each other in a loop; check PATH for more than one block shim directory\n", command)
		return exitFailure
	}
	st, err := store.Open()
	if err != nil {
		fmt.Fprintf(stderr, "block: %v\n", err)
		return exitFailure
	}
	dir, findErr := manifest.Find(workingDir())
	if findErr != nil {
		return fallback(ctx, st, command, args, depth, stdin, stdout, stderr,
			fmt.Sprintf("block: %s: no block project here and no %s elsewhere on PATH", command, command))
	}
	toolchain, err := block.OpenToolchain(dir, platform.Current(), st)
	if err != nil {
		// A project that is stale or not synced is a mistake to report, not
		// one to work around by running some other build of the tool.
		fmt.Fprintf(stderr, "block: %v\n", err)
		return exitFailure
	}
	if _, ok := toolchain.ResolveCommand(command); !ok {
		return fallback(ctx, st, command, args, depth, stdin, stdout, stderr,
			fmt.Sprintf("block: %s: %s does not lock a tool providing %q, and no %s was found elsewhere on PATH",
				command, manifest.FileName, command, command))
	}
	code, err := toolchain.Run(ctx, command, args, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "block: %v\n", err)
		return exitFailure
	}
	return code
}

// fallback runs the next command of this name on PATH, skipping the shim
// directory and this executable so that a shim can never call itself.
func fallback(ctx context.Context, st *store.Store, command string, args []string, depth int,
	stdin io.Reader, stdout, stderr io.Writer, notFound string,
) int {
	bin, ok := next(st, command)
	if !ok {
		fmt.Fprintln(stderr, notFound)
		return exitFailure
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // the command the user typed, found on their own PATH
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", depthEnv, depth+1))
	code, err := block.RunCommand(cmd, command, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "block: %v\n", err)
		return exitFailure
	}
	return code
}

// next finds the command PATH would have resolved to if the shim directory
// were not there.
func next(st *store.Store, command string) (string, bool) {
	shims := shim.Dir(st)
	self, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	selfInfo, _ := os.Stat(self)
	var kept []string
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == "" || shim.SameDir(entry, shims) {
			continue
		}
		kept = append(kept, entry)
	}
	for _, entry := range kept {
		bin, err := block.LookPath(command, entry)
		if err != nil {
			continue
		}
		// A shim copied into another directory is still this binary.
		if selfInfo != nil {
			if info, err := os.Stat(bin); err == nil && os.SameFile(info, selfInfo) { //nolint:gosec // a PATH entry, resolved for comparison only
				continue
			}
		}
		return bin, true
	}
	return "", false
}

// workingDir is where the project search starts.
func workingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
