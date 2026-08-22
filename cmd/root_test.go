package cmd

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/cmdinfo"
	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/fakegh"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/store"
	"github.com/nao1215/block/registry"
)

// run executes the CLI in dir against an in-process fake GitHub.
func run(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	code := Execute(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func setup(t *testing.T) (string, string) {
	t.Helper()
	fake := fakegh.New(fakegh.Fixtures())
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	fake.SetBase(srv.URL)
	t.Setenv(github.EnvBaseURL, srv.URL+"/t1")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv(store.EnvHome, filepath.Join(t.TempDir(), "home"))
	return t.TempDir(), srv.URL
}

func TestVersionAndHelp(t *testing.T) { //nolint:paralleltest // t.Chdir
	dir, _ := setup(t)
	code, out, _ := run(t, dir, "version")
	if code != 0 || !strings.HasPrefix(out, "block "+cmdinfo.Version+"\n") {
		t.Errorf("version = %d, %q", code, out)
	}
	// A binary says which registry snapshot it carries, so a resolution can
	// be traced back to the recipes that produced it.
	snap, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "registry "+snap.Revision[:12]) || !strings.Contains(out, snap.Source) {
		t.Errorf("version does not name the registry snapshot: %q", out)
	}
	code, out, _ = run(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "sync never resolves. exec never installs.") {
		t.Errorf("help = %d, %q", code, out)
	}
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	code, out, _ = run(t, dir, "list")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if code != 0 || !strings.HasPrefix(lines[0], "NAME") || !strings.Contains(lines[0], "ECOSYSTEM") || !strings.Contains(lines[0], "DESCRIPTION") {
		t.Fatalf("list = %d, %q", code, out)
	}
	if len(lines)-1 != len(reg.Recipes()) {
		t.Errorf("list printed %d rows for %d recipes", len(lines)-1, len(reg.Recipes()))
	}
	// Column widths depend on the whole catalogue, so a row is compared by
	// its fields rather than by the spacing of the day.
	row := func(out, name string) (string, bool) {
		for _, line := range strings.Split(out, "\n") {
			if fields := strings.Fields(line); len(fields) > 0 && fields[0] == name {
				return line, true
			}
		}
		return "", false
	}
	for _, want := range []struct{ name, ecosystem, description string }{
		{"bitcoin-core", "bitcoin", "Bitcoin reference implementation"},
		{"foundry", "ethereum", "Fast Ethereum application toolkit"},
		{"hermes", "cosmos, ibc", "IBC relayer"},
	} {
		line, ok := row(out, want.name)
		if !ok || !strings.Contains(line, want.ecosystem) || !strings.Contains(line, want.description) {
			t.Errorf("list is missing a %s row for %q:\n%s", want.name, want.description, out)
		}
	}
	// How a tool is fetched is a registry concern, not something the
	// listing puts in front of a reader.
	if strings.Contains(out, "github_release") {
		t.Errorf("list still shows the source type:\n%s", out)
	}

	// Filtering by ecosystem drops the now-constant column and keeps the
	// rows sorted by tool name.
	code, out, _ = run(t, dir, "list", "ethereum")
	lines = strings.Split(strings.TrimRight(out, "\n"), "\n")
	if code != 0 || !strings.HasPrefix(lines[0], "NAME") || !strings.Contains(lines[0], "COMMANDS") {
		t.Fatalf("list ethereum = %d, %q", code, out)
	}
	if strings.Contains(lines[0], "ECOSYSTEM") {
		t.Errorf("list ethereum keeps the now-constant ecosystem column:\n%s", out)
	}
	if len(lines)-1 != len(reg.ByEcosystem("ethereum")) {
		t.Errorf("list ethereum printed %d rows for %d recipes", len(lines)-1, len(reg.ByEcosystem("ethereum")))
	}
	// The commands a tool provides are what this listing adds over the
	// unfiltered one.
	if line, ok := row(out, "foundry"); !ok || !strings.Contains(line, "forge, cast, anvil, chisel") {
		t.Errorf("list ethereum has no foundry row naming its commands:\n%s", out)
	}
	// A tool serving two ecosystems is listed under each of them.
	for _, ecosystem := range []string{"cosmos", "ibc"} {
		if code, out, _ := run(t, dir, "list", ecosystem); code != 0 {
			t.Errorf("list %s = %d, %q", ecosystem, code, out)
		} else if _, ok := row(out, "hermes"); !ok {
			t.Errorf("list %s does not list hermes:\n%s", ecosystem, out)
		}
	}
	// Every ecosystem the registry knows can be listed.
	for _, ecosystem := range reg.Ecosystems() {
		code, out, errOut := run(t, dir, "list", ecosystem)
		if code != 0 || len(strings.Split(strings.TrimRight(out, "\n"), "\n")) < 2 {
			t.Errorf("list %s = %d, %q, %q", ecosystem, code, out, errOut)
		}
	}
	code, _, errOut := run(t, dir, "list", "etheruem")
	if want := "block: " + diag.UnknownEcosystem.String() + ": unknown ecosystem \"etheruem\"\navailable ecosystems: " + strings.Join(reg.Ecosystems(), ", ") + "\n"; code != 1 || errOut != want {
		t.Errorf("list etheruem = %d, %q, want %q", code, errOut, want)
	}
	if code, _, errOut := run(t, dir, "list", "ethereum", "solana"); code != 1 || !strings.Contains(errOut, "accepts at most 1 arg") {
		t.Errorf("list with two arguments = %d, %q", code, errOut)
	}
	for _, gone := range []string{"init", "update", "outdated", "registry", "search"} {
		code, _, errOut := run(t, dir, gone)
		if code != 1 || !strings.HasPrefix(errOut, `block: unknown command "`+gone+`"`) {
			t.Errorf("%s = %d, %q (must not exist)", gone, code, errOut)
		}
	}
	code, _, errOut = run(t, dir, "lock")
	if code != 1 || !strings.Contains(errOut, "block.toml not found") {
		t.Errorf("lock without manifest = %d, %q", code, errOut)
	}
}

func TestWorkflow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	dir, base := setup(t)
	if err := os.WriteFile(filepath.Join(dir, "block.toml"), []byte("[tools]\nfoundry = \"1.7\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run(t, dir, "sync"); code != 1 || !strings.Contains(errOut, `block.lock not found; run "block lock"`) {
		t.Errorf("sync before lock = %d, %q", code, errOut)
	}
	if code, _, _ := run(t, dir, "lock", "--check"); code != exitNeedsWork {
		t.Errorf("lock --check before lock = %d, want %d", code, exitNeedsWork)
	}
	code, out, _ := run(t, dir, "lock")
	if code != 0 || !strings.Contains(out, "foundry  locked 1.7.4") {
		t.Errorf("lock = %d, %q", code, out)
	}
	if code, out, _ := run(t, dir, "lock", "--check"); code != 0 || out != "foundry  1.7.4 (up-to-date)\n" {
		t.Errorf("lock --check = %d, %q", code, out)
	}
	if code, _, errOut := run(t, dir, "exec", "forge"); code != 1 || !strings.Contains(errOut, `is not installed; run "block sync"`) {
		t.Errorf("exec before sync = %d, %q", code, errOut)
	}
	code, out, _ = run(t, dir, "sync")
	if code != 0 || out != "foundry  1.7.4  installed\nshims: anvil, cast, chisel, forge\n" {
		t.Errorf("sync = %d, %q", code, out)
	}
	code, out, errOut := run(t, dir, "exec", "--", "forge", "--version")
	if code != 0 || out != "forge 1.7.4 (fake)\nargs: --version\n" || errOut != "" {
		t.Errorf("exec = %d, %q, %q", code, out, errOut)
	}
	if code, _, _ := run(t, dir, "exec", "forge", "--exit", "4"); code != 4 {
		t.Errorf("exec exit code = %d, want 4", code)
	}
	if code, out, _ := run(t, dir, "exec", "--help"); code != 0 || !strings.Contains(out, "block exec make test") {
		t.Errorf("exec --help = %d, %q", code, out)
	}
	if code, _, errOut := run(t, dir, "exec"); code != 1 || !strings.Contains(errOut, "requires at least 1 arg") {
		t.Errorf("exec without args = %d, %q", code, errOut)
	}

	// Upstream publishes 1.7.5.
	t.Setenv(github.EnvBaseURL, base)
	code, out, _ = run(t, dir, "lock", "--check")
	if code != exitNeedsWork || out != "foundry  1.7.4 -> 1.7.5\n" {
		t.Errorf("lock --check (outdated) = %d, %q", code, out)
	}
	if code, out, _ := run(t, dir, "sync"); code != 0 || out != "foundry  1.7.4  cached\n" {
		t.Errorf("sync keeps the pin = %d, %q", code, out)
	}
	code, out, _ = run(t, dir, "lock", "foundry")
	if code != 0 || !strings.Contains(out, "foundry  1.7.4 -> 1.7.5") {
		t.Errorf("lock = %d, %q", code, out)
	}
	sub := filepath.Join(dir, "contracts")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(t, sub, "sync"); code != 0 || out != "foundry  1.7.5  installed\n" {
		t.Errorf("sync from subdir = %d, %q", code, out)
	}
	if code, out, _ := run(t, sub, "exec", "forge"); code != 0 || !strings.Contains(out, "forge 1.7.5 (fake)") {
		t.Errorf("exec from subdir = %d, %q", code, out)
	}
}
