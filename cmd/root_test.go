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
	for _, gone := range []string{"init", "update", "outdated", "registry"} {
		code, _, errOut := run(t, dir, gone)
		if code != 1 || !strings.HasPrefix(errOut, `block: unknown command "`+gone+`"`) {
			t.Errorf("%s = %d, %q (must not exist)", gone, code, errOut)
		}
	}
	code, _, errOut := run(t, dir, "lock")
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
