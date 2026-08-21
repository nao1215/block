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
	if code != 0 || out != "block "+cmdinfo.Version+"\n" {
		t.Errorf("version = %d, %q", code, out)
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
	for _, row := range []string{
		"bitcoin-core   bitcoin       Bitcoin reference implementation",
		"foundry        ethereum      Fast Ethereum application toolkit",
		"hermes         cosmos, ibc   IBC relayer",
	} {
		if !strings.Contains(out, row) {
			t.Errorf("list is missing %q:\n%s", row, out)
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
	if code != 0 || out != "NAME         COMMANDS                     DESCRIPTION\n"+
		"foundry      forge, cast, anvil, chisel   Fast Ethereum application toolkit: build, test, deploy and inspect contracts\n"+
		"geth         geth                         go-ethereum, the Go implementation of an Ethereum execution client\n"+
		"lighthouse   lighthouse                   Ethereum consensus (beacon chain) client written in Rust\n"+
		"reth         reth                         Modular Ethereum execution client written in Rust\n"+
		"solc         solc                         The Solidity smart-contract compiler\n" {
		t.Errorf("list ethereum = %d, %q", code, out)
	}
	// A tool serving two ecosystems is listed under each of them. Column
	// widths differ per listing, so the row is compared by its fields.
	hasRow := func(out, name string) bool {
		for _, line := range strings.Split(out, "\n") {
			if fields := strings.Fields(line); len(fields) > 0 && fields[0] == name {
				return true
			}
		}
		return false
	}
	for _, ecosystem := range []string{"cosmos", "ibc"} {
		if code, out, _ := run(t, dir, "list", ecosystem); code != 0 || !hasRow(out, "hermes") {
			t.Errorf("list %s = %d, %q", ecosystem, code, out)
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
	if code != 1 || errOut != "block: unknown ecosystem \"etheruem\"\navailable ecosystems: bitcoin, cosmos, ethereum, ibc, solana\n" {
		t.Errorf("list etheruem = %d, %q", code, errOut)
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
	if code, _, _ := run(t, dir, "lock", "--check"); code != exitOutdated {
		t.Errorf("lock --check before lock = %d, want %d", code, exitOutdated)
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
	if code != 0 || out != "foundry  1.7.4  installed\n" {
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
	if code != exitOutdated || out != "foundry  1.7.4 -> 1.7.5\n" {
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
