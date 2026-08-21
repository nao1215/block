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
)

// run executes the CLI in dir against an in-process fake GitHub.
func run(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	code := Execute(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func setup(t *testing.T) string {
	t.Helper()
	fake := fakegh.New(fakegh.Fixtures())
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	fake.SetBase(srv.URL)
	t.Setenv(github.EnvBaseURL, srv.URL+"/t1")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv(store.EnvHome, filepath.Join(t.TempDir(), "home"))
	return t.TempDir()
}

func TestVersionAndHelp(t *testing.T) { //nolint:paralleltest // t.Chdir
	dir := setup(t)
	code, out, _ := run(t, dir, "version")
	if code != 0 || out != "block "+cmdinfo.Version+"\n" {
		t.Errorf("version = %d, %q", code, out)
	}
	code, out, _ = run(t, dir, "--help")
	if code != 0 || !strings.Contains(out, "block sync [--locked]") {
		t.Errorf("help = %d, %q", code, out)
	}
	code, _, errOut := run(t, dir, "bogus")
	if code != 1 || !strings.HasPrefix(errOut, `block: unknown command "bogus"`) {
		t.Errorf("bogus = %d, %q", code, errOut)
	}
	code, out, _ = run(t, dir, "registry")
	if code != 0 || !strings.Contains(out, "foundry\tfoundry-rs/foundry\n") {
		t.Errorf("registry = %d, %q", code, out)
	}
}

func TestWorkflow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	dir := setup(t)
	code, _, errOut := run(t, dir, "lock")
	if code != 1 || !strings.Contains(errOut, "block.toml not found") {
		t.Errorf("lock without manifest = %d, %q", code, errOut)
	}
	code, out, _ := run(t, dir, "init")
	if code != 0 || out != "created block.toml\n" {
		t.Errorf("init = %d, %q", code, out)
	}
	if code, _, errOut := run(t, dir, "init"); code != 1 || errOut != "block: block.toml already exists\n" {
		t.Errorf("init again = %d, %q", code, errOut)
	}
	code, out, _ = run(t, dir, "lock")
	if code != 0 || !strings.Contains(out, "foundry  locked 1.7.1") {
		t.Errorf("lock = %d, %q", code, out)
	}
	code, out, _ = run(t, dir, "sync", "--locked")
	if code != 0 || out != "foundry  1.7.1  installed\n" {
		t.Errorf("sync = %d, %q", code, out)
	}
	code, out, errOut = run(t, dir, "exec", "--", "forge", "--version")
	if code != 0 || out != "forge 1.7.1 (fake)\nargs: --version\n" || errOut != "" {
		t.Errorf("exec = %d, %q, %q", code, out, errOut)
	}
	if code, _, _ := run(t, dir, "exec", "forge", "--exit", "4"); code != 4 {
		t.Errorf("exec exit code = %d, want 4", code)
	}
	if code, out, _ := run(t, dir, "exec", "--help"); code != 0 || !strings.Contains(out, "block exec forge test") {
		t.Errorf("exec --help = %d, %q", code, out)
	}
	if code, _, errOut := run(t, dir, "exec"); code != 1 || !strings.Contains(errOut, "requires at least 1 arg") {
		t.Errorf("exec without args = %d, %q", code, errOut)
	}
	t.Setenv(github.EnvBaseURL, strings.TrimSuffix(os.Getenv(github.EnvBaseURL), "/t1"))
	code, out, _ = run(t, dir, "outdated")
	if code != 0 || out != "foundry  1.7.1 -> 1.7.4\n" {
		t.Errorf("outdated = %d, %q", code, out)
	}
	code, out, _ = run(t, dir, "update", "foundry")
	if code != 0 || !strings.Contains(out, "foundry  1.7.1 -> 1.7.4") {
		t.Errorf("update = %d, %q", code, out)
	}
	sub := filepath.Join(dir, "contracts")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	code, out, _ = run(t, sub, "sync")
	if code != 0 || out != "foundry  1.7.4  installed\n" {
		t.Errorf("sync from subdir = %d, %q", code, out)
	}
}

func TestUnsupportedHostPlatform(t *testing.T) { //nolint:paralleltest // t.Chdir
	dir := setup(t)
	if err := os.WriteFile(filepath.Join(dir, "block.toml"), []byte("[tools]\nfoundry = \"1.7\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		code, _, errOut := run(t, dir, "sync")
		if code != 1 || !strings.Contains(errOut, "unsupported platform windows/") {
			t.Errorf("sync on windows = %d, %q", code, errOut)
		}
	}
}
