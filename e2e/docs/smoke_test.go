//go:build docs

// A documentation smoke test: on a machine with nothing on it, do the steps
// the front pages promise actually take a reader from an empty directory to a
// blockchain tool running?
//
// The steps are not retyped here. They are read out of README.md and the
// website's front page, so the test runs whatever those pages currently say —
// and fails when a page starts promising something that does not work. That
// is the whole point: a quickstart nobody executes is a quickstart that is
// wrong by the next release.
//
// It installs real tools from the real internet, so it is behind a build tag
// and never runs with the unit or E2E suites:
//
//	go test -tags=docs -v -timeout 20m ./e2e/docs/
//
// The Documentation smoke workflow runs it on a clean runner of every platform
// block supports.
package docs_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const stepTimeout = 10 * time.Minute

// pages are the documents that make the promise, and both have to keep it.
// They are checked separately rather than compared, because they are allowed
// to word it differently — what they may not do is differ in what happens.
var pages = []string{
	filepath.Join("..", "..", "README.md"),
	filepath.Join("..", "..", "website", "content", "_index.md"),
}

// quickstart is one runnable block of shell from a page: the manifest it
// writes, and the block invocations that follow.
type quickstart struct {
	page     string
	line     int
	manifest string
	commands [][]string
}

func TestQuickstartsWork(t *testing.T) {
	block := buildBlock(t)

	for _, page := range pages {
		starts := quickstartsIn(t, page)
		if len(starts) == 0 {
			t.Errorf("%s has no quickstart: no fenced shell block writes a block.toml and then runs block", page)
			continue
		}
		ran := 0
		for _, q := range starts {
			if !supportedHere(t, block, q) {
				t.Logf("%s:%d: skipping, its tools ship nothing for %s/%s", q.page, q.line, runtime.GOOS, runtime.GOARCH)
				continue
			}
			t.Run(shortName(q), func(t *testing.T) { runQuickstart(t, block, q) })
			ran++
		}
		// A page whose every quickstart is unrunnable here is a page a reader
		// on this platform cannot follow, which is the failure this test
		// exists to find.
		if ran == 0 {
			t.Errorf("%s offers no quickstart a reader on %s/%s can follow", page, runtime.GOOS, runtime.GOARCH)
		}
	}
}

// The install page tells a reader to put $BLOCK_HOME/shims on PATH once and
// then type the tool's own name. That is the step where a promise about
// PATH, executable suffixes and shim files either holds on this platform or
// does not, so it is done for real rather than described.
func TestShimsOnPathRunTheTool(t *testing.T) {
	block := buildBlock(t)

	var ran bool
	for _, page := range pages {
		for _, q := range quickstartsIn(t, page) {
			if ran || !supportedHere(t, block, q) {
				continue
			}
			dir := t.TempDir()
			home := filepath.Join(dir, "store")
			if err := os.WriteFile(filepath.Join(dir, "block.toml"), []byte(q.manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, args := range q.commands[:len(q.commands)-1] {
				if out, err := runBlock(t, block, dir, home, args...); err != nil {
					t.Fatalf("block %s: %v\n%s", strings.Join(args, " "), err, out)
				}
			}
			// The last documented command is "exec <tool> …"; without the
			// "exec" it is what a reader with the shims on PATH types.
			last := q.commands[len(q.commands)-1]
			if len(last) < 2 || last[0] != "exec" {
				t.Fatalf("the quickstart's last step is %q, which is not an exec; the test cannot derive the bare command", last)
			}
			shims := filepath.Join(home, "shims")
			if _, err := os.Stat(shims); err != nil {
				t.Fatalf("sync did not create %s, which the install page tells the reader to put on PATH: %v", shims, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
			cmd := exec.CommandContext(ctx, filepath.Join(shims, last[1]+exeSuffix()), last[2:]...) //nolint:gosec // the tool the page names
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "BLOCK_HOME="+home, "PATH="+shims+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			cancel()
			t.Logf("$ %s %s\n%s", last[1], strings.Join(last[2:], " "), out)
			if err != nil {
				t.Fatalf("%s through its shim: %v", last[1], err)
			}
			ran = true
		}
	}
	if !ran {
		t.Skipf("no documented quickstart resolves on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// TestVersionSaysWhatTheInstallPageShows checks the one output the install
// page quotes verbatim: the two lines that pair a block binary with the
// registry revision it carries.
func TestVersionSaysWhatTheInstallPageShows(t *testing.T) {
	block := buildBlock(t)
	dir := t.TempDir()
	out, err := runBlock(t, block, dir, filepath.Join(dir, "store"), "version")
	if err != nil {
		t.Fatalf("block version: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("block version printed %d lines, the install page shows 2:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "block ") {
		t.Errorf("the first line is %q, the page shows \"block <version>\"", lines[0])
	}
	if !regexp.MustCompile(`^registry [0-9a-f]{12} \(\d+ recipes from https://github\.com/nao1215/block-registry\)$`).MatchString(lines[1]) {
		t.Errorf("the second line is %q, which is not the shape the install page shows", lines[1])
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// runQuickstart does what the page says, in an empty directory, with a store
// of its own — the closest a test gets to a machine that has never seen block.
func runQuickstart(t *testing.T, block string, q quickstart) {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "store")
	if err := os.WriteFile(filepath.Join(dir, "block.toml"), []byte(q.manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s:%d: block.toml is\n%s", q.page, q.line, q.manifest)

	for _, args := range q.commands {
		out, err := runBlock(t, block, dir, home, args...)
		t.Logf("$ block %s\n%s", strings.Join(args, " "), out)
		if err != nil {
			t.Fatalf("the page says to run \"block %s\", and it failed: %v", strings.Join(args, " "), err)
		}
	}
	// The last documented command runs a tool. A quickstart that ends in
	// silence has not shown the reader anything.
	last := q.commands[len(q.commands)-1]
	out, err := runBlock(t, block, dir, home, last...)
	if err != nil || strings.TrimSpace(out) == "" {
		t.Errorf("the last step printed nothing; a quickstart has to end by showing the tool running (%v)", err)
	}
}

// supportedHere reports whether every tool the quickstart pins ships for this
// machine, asked of block itself rather than guessed.
func supportedHere(t *testing.T, block string, q quickstart) bool {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "block.toml"), []byte(q.manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runBlock(t, block, dir, filepath.Join(dir, "store"), "lock", "--check")
	// --check exits 0 when current and 2 when the lock would change; both
	// mean the toolchain resolves here. Anything else is a refusal, and the
	// only refusal that is not a documentation bug is "no build for this
	// platform".
	if err == nil || exitCode(err) == 2 {
		return true
	}
	if strings.Contains(out, "unsupported platform") {
		return false
	}
	t.Errorf("%s:%d: the documented manifest does not resolve here: %s", q.page, q.line, strings.TrimSpace(out))
	return false
}

func runBlock(t *testing.T, block, dir, home string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, block, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BLOCK_HOME="+home)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func exitCode(err error) int {
	var e *exec.ExitError
	if errors.As(err, &e) {
		return e.ExitCode()
	}
	return -1
}

// buildBlock builds the binary under test. The pages tell a reader to
// `go install github.com/nao1215/block@latest`; what is checked here is the
// revision in hand, because a smoke test that installed the last release
// would pass while this one is broken.
func buildBlock(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "block")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join("..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building block: %v\n%s", err, b)
	}
	return out
}

// fence opens or closes a fenced block and names its language.
var fence = regexp.MustCompile("^\\s*```([a-zA-Z]*)")

// writesManifest matches the one line every quickstart starts with: the
// printf that writes block.toml.
var writesManifest = regexp.MustCompile(`^printf\s+'(.*)'\s*>\s*block\.toml$`)

// invokesBlock matches a line that runs block, however the page spells the
// binary — `block`, or `go run github.com/nao1215/block@latest`.
var invokesBlock = regexp.MustCompile(`^(?:go run github\.com/nao1215/block@\S+|block)\s+(.*)$`)

// quickstartsIn finds every fenced shell block that writes a block.toml and
// then runs block against it.
func quickstartsIn(t *testing.T, page string) []quickstart {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(page))
	if err != nil {
		t.Fatal(err)
	}
	var out []quickstart
	var current *quickstart
	inShell, start := false, 0
	for i, line := range strings.Split(string(data), "\n") {
		if m := fence.FindStringSubmatch(line); m != nil {
			if inShell {
				if current != nil && len(current.commands) > 0 {
					out = append(out, *current)
				}
				current, inShell = nil, false
				continue
			}
			switch m[1] {
			case "shell", "sh", "bash":
				inShell, start = true, i+2
			}
			continue
		}
		if !inShell {
			continue
		}
		text := strings.TrimSpace(stripComment(line))
		switch {
		case writesManifest.MatchString(text):
			current = &quickstart{page: page, line: start, manifest: unescape(writesManifest.FindStringSubmatch(text)[1])}
		case current != nil && invokesBlock.MatchString(text):
			current.commands = append(current.commands, strings.Fields(invokesBlock.FindStringSubmatch(text)[1]))
		}
	}
	return out
}

// stripComment drops a trailing "# …" annotation, which the pages use to say
// what each step does.
func stripComment(line string) string {
	if i := strings.Index(line, " #"); i >= 0 {
		return line[:i]
	}
	return line
}

// unescape turns the printf argument the page shows into the file it writes.
func unescape(s string) string { return strings.ReplaceAll(s, `\n`, "\n") }

func shortName(q quickstart) string {
	return strings.NewReplacer("/", "_", " ", "_", "\n", "_").Replace(filepath.Base(q.page)) + "_" + strconv.Itoa(q.line)
}
