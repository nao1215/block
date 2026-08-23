package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/store"
)

func TestWhich(t *testing.T) { // t.Chdir and t.Setenv
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	dir, _ := setup(t)
	if err := os.WriteFile(filepath.Join(dir, "block.toml"), []byte("[tools]\nfoundry = \"1.7\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run(t, dir, "which", "forge"); code != 1 || !strings.Contains(errOut, "block.lock not found") {
		t.Errorf("which before lock = %d, %q", code, errOut)
	}
	if code, _, _ := run(t, dir, "lock"); code != 0 {
		t.Fatal("lock failed")
	}
	if code, _, errOut := run(t, dir, "which", "forge"); code != 1 || !strings.Contains(errOut, `is not installed; run "block sync"`) {
		t.Errorf("which before sync = %d, %q", code, errOut)
	}
	if code, _, _ := run(t, dir, "sync"); code != 0 {
		t.Fatal("sync failed")
	}
	// A forge of its own on PATH does not change the answer.
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "forge"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", other+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, out, errOut := run(t, dir, "which", "forge")
	path := strings.TrimSuffix(out, "\n")
	home := os.Getenv(store.EnvHome)
	if code != 0 || errOut != "" || !strings.HasPrefix(path, filepath.Join(home, "tools", "foundry", "1.7.4-")) || filepath.Base(path) != "forge" {
		t.Errorf("which forge = %d, %q, %q", code, out, errOut)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("which prints %q, want exactly one line", out)
	}
	// The same answer from a nested directory.
	sub := filepath.Join(dir, "contracts", "deep")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if code, nested, _ := run(t, sub, "which", "forge"); code != 0 || nested != out {
		t.Errorf("which from a nested directory = %d, %q, want %q", code, nested, out)
	}
	if code, _, errOut := run(t, dir, "which", "nope"); code != 1 || errOut != "block: "+diag.CommandNotFound.String()+": block.toml does not lock a tool providing the command \"nope\"\n" {
		t.Errorf("which nope = %d, %q", code, errOut)
	}
	if code, _, errOut := run(t, dir, "which"); code != 1 || !strings.Contains(errOut, "accepts 1 arg") {
		t.Errorf("which without args = %d, %q", code, errOut)
	}
	if code, _, errOut := run(t, dir, "which", "forge", "cast"); code != 1 || !strings.Contains(errOut, "accepts 1 arg") {
		t.Errorf("which with two args = %d, %q", code, errOut)
	}
	if code, out, _ := run(t, dir, "which", "--help"); code != 0 || !strings.Contains(out, "does not look at PATH") {
		t.Errorf("which --help = %d, %q", code, out)
	}

	// Shell completion offers what the project locks, from the lockfile and
	// nothing else.
	code, out, _ = run(t, sub, "__complete", "which", "")
	if code != 0 || !strings.HasPrefix(out, "anvil\ncast\nchisel\nforge\n:4\n") {
		t.Errorf("__complete which = %d, %q", code, out)
	}
	code, out, _ = run(t, dir, "__complete", "lock", "")
	if code != 0 || !strings.HasPrefix(out, "foundry\n:4\n") {
		t.Errorf("__complete lock = %d, %q", code, out)
	}
	// Outside a project there is nothing to offer, and no complaint either.
	if code, out, _ := run(t, t.TempDir(), "__complete", "which", ""); code != 0 || !strings.HasPrefix(out, ":4\n") {
		t.Errorf("__complete which outside a project = %d, %q", code, out)
	}
}

func TestCompletion(t *testing.T) { //nolint:paralleltest // t.Chdir
	dir, _ := setup(t)
	for shell, marker := range map[string]string{
		"bash": "__block_init_completion",
		"zsh":  "#compdef block",
		"fish": "complete -c block",
	} {
		code, out, errOut := run(t, dir, "completion", shell)
		if code != 0 || errOut != "" || !strings.Contains(out, marker) || !strings.Contains(out, "__complete") {
			t.Errorf("completion %s = %d, %q, %q; want a script naming %q", shell, code, errOut, out, marker)
		}
	}
	if code, _, errOut := run(t, dir, "completion", "tcsh"); code != 1 || !strings.Contains(errOut, "tcsh") {
		t.Errorf("completion tcsh = %d, %q", code, errOut)
	}
	if code, out, _ := run(t, dir, "completion", "--help"); code != 0 || !strings.Contains(out, "source <(block completion bash)") {
		t.Errorf("completion --help = %d, %q", code, out)
	}
	if code, out, _ := run(t, dir, "--help"); code != 0 || !strings.Contains(out, "completion  Print a shell completion script") {
		t.Errorf("--help does not list completion: %d, %q", code, out)
	}
	// The static candidates: every subcommand, every ecosystem, every code.
	code, out, _ := run(t, dir, "__complete", "")
	for _, sub := range []string{"lock", "sync", "exec", "which", "status", "list", "explain", "version", "completion"} {
		if code != 0 || !strings.Contains(out, "\n"+sub+"\t") && !strings.HasPrefix(out, sub+"\t") {
			t.Errorf("__complete '' = %d, %q; missing %s", code, out, sub)
		}
	}
	if code, out, _ := run(t, dir, "__complete", "list", ""); code != 0 || !strings.Contains(out, "\nethereum\n") {
		t.Errorf("__complete list = %d, %q", code, out)
	}
	if code, out, _ := run(t, dir, "__complete", "explain", ""); code != 0 || !strings.Contains(out, diag.LockMissing.String()+"\t") {
		t.Errorf("__complete explain = %d, %q", code, out)
	}
	if code, out, _ := run(t, dir, "__complete", "status", "--"); code != 0 || !strings.Contains(out, "--json") {
		t.Errorf("__complete status -- = %d, %q", code, out)
	}
}
