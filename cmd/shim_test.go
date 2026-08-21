package cmd_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nao1215/block/internal/shim"
	"github.com/nao1215/block/internal/store"
)

// These tests run the real thing: a block binary placed under a tool's name,
// started as its own process, on whatever operating system the tests run on.
// That is the only way to cover what a shim actually is — argv[0] dispatch,
// Windows links or copies instead of symlinks, exit codes crossing a process
// boundary — so they build binaries rather than calling functions.

// fakeToolSource is a stand-in for a locked tool: it says which version it is
// and does what its arguments ask, and it is a real executable on every
// platform.
const fakeToolSource = `package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	fmt.Println("faketool", version)
	if len(args) > 0 {
		fmt.Println("args:", strings.Join(args, " "))
	}
	if len(args) == 2 && args[0] == "--exit" {
		code, err := strconv.Atoi(args[1])
		if err == nil {
			os.Exit(code)
		}
	}
}
`

//nolint:gochecknoglobals // built once for the whole package
var (
	buildOnce sync.Once
	blockBin  string
	errBuild  error
	buildDir  string
)

// blockBinary builds block itself, once, and returns the path to it.
func blockBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		buildDir, errBuild = os.MkdirTemp("", "block-shim-test-*")
		if errBuild != nil {
			return
		}
		blockBin = filepath.Join(buildDir, "block"+store.ExeSuffix)
		out, err := exec.CommandContext(context.Background(), "go", "build", "-o", blockBin, "..").CombinedOutput()
		if err != nil {
			errBuild = fmt.Errorf("building block: %w: %s", err, out)
		}
	})
	if errBuild != nil {
		t.Fatal(errBuild)
	}
	return blockBin
}

func TestMain(m *testing.M) {
	code := m.Run()
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	os.Exit(code)
}

// buildFakeTool compiles the stand-in tool with a version baked in.
func buildFakeTool(t *testing.T, version string) string {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(fakeToolSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module faketool\n\ngo 1.25.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "faketool"+store.ExeSuffix)
	cmd := exec.CommandContext(t.Context(), "go", "build", "-ldflags", "-X main.version="+version, "-o", bin, ".")
	cmd.Dir = src
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the fake tool: %v: %s", err, out)
	}
	return bin
}

// installTool puts a fake tool into the store the way block would: an archive
// with the executable inside, installed through the store's own code so the
// completion marker and the layout are real.
func installTool(t *testing.T, st *store.Store, tool, command, version string) string {
	t.Helper()
	binary, err := os.ReadFile(buildFakeTool(t, version))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name:     command + store.ExeSuffix,
		Mode:     0o755,
		Size:     int64(len(binary)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), tool+".tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	digest := hex.EncodeToString(sum[:])
	dir, err := st.InstallDir(tool, version, digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Install(archive, tool+".tar.gz", dir, []string{command}, 0); err != nil {
		t.Fatal(err)
	}
	return digest
}

// project writes a block.toml and a block.lock pinning one tool at one
// version, as "block lock" would have.
func project(t *testing.T, dir, tool, command, version, digest string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("[tools]\n%s = \"%s\"\n", tool, majorMinor(version))
	if err := os.WriteFile(filepath.Join(dir, "block.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := fmt.Sprintf(`version = 1

[[tools]]
name = %q
constraint = %q
version = %q
bin = [%q]

[[tools.artifacts]]
platform = %q
url = "https://example.invalid/%s.tar.gz"
sha256 = %q
`, tool, majorMinor(version), version, command, currentPlatform(), tool, digest)
	if err := os.WriteFile(filepath.Join(dir, "block.lock"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
}

func majorMinor(version string) string {
	const components = 3
	parts := strings.SplitN(version, ".", components)
	return parts[0] + "." + parts[1]
}

func currentPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// run executes a shim in a directory and returns its exit code and output.
func run(t *testing.T, dir, shimPath, blockHome string, extraPath []string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), shimPath, args...)
	cmd.Dir = dir
	path := strings.Join(append(extraPath, os.Getenv("PATH")), string(os.PathListSeparator))
	cmd.Env = append(os.Environ(), "BLOCK_HOME="+blockHome, "PATH="+path)
	out, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if err != nil {
		if !errors.As(err, &exitErr) {
			t.Fatalf("running %s: %v: %s", shimPath, err, out)
		}
		code = exitErr.ExitCode()
	}
	return code, string(out)
}

func TestShimRunsThePinnedVersionOfEachProject(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	st := &store.Store{Root: filepath.Join(root, "home")}

	// Two projects, two versions of the same tool, both installed.
	digestA := installTool(t, st, "foundry", "forge", "1.7.4")
	digestB := installTool(t, st, "foundry", "forge", "1.8.1")
	projectA := filepath.Join(root, "project-a")
	projectB := filepath.Join(root, "project-b")
	project(t, projectA, "foundry", "forge", "1.7.4", digestA)
	project(t, projectB, "foundry", "forge", "1.8.1", digestB)

	if _, err := shim.Ensure(st, blockBinary(t), []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	forge := filepath.Join(shim.Dir(st), shim.FileName("forge"))

	// The same shim, two directories, two versions — with nothing to switch.
	for dir, want := range map[string]string{
		projectA: "faketool 1.7.4",
		projectB: "faketool 1.8.1",
	} {
		code, out := run(t, dir, forge, st.Root, nil, "--version")
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("in %s: exit %d, output %q, want %q", filepath.Base(dir), code, out, want)
		}
	}

	// A nested directory belongs to the project above it.
	nested := filepath.Join(projectA, "contracts", "test")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, nested, forge, st.Root, nil, "--version"); code != 0 || !strings.Contains(out, "faketool 1.7.4") {
		t.Errorf("nested: exit %d, output %q", code, out)
	}

	// Arguments and exit codes cross the shim untouched.
	code, out := run(t, projectA, forge, st.Root, nil, "--exit", "7")
	if code != 7 {
		t.Errorf("exit code = %d, want 7; output %q", code, out)
	}
	if !strings.Contains(out, "args: --exit 7") {
		t.Errorf("arguments were not passed through: %q", out)
	}
}

func TestShimRefusesWhatItMustNotFixItself(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	st := &store.Store{Root: filepath.Join(root, "home")}
	digest := installTool(t, st, "foundry", "forge", "1.7.4")
	if _, err := shim.Ensure(st, blockBinary(t), []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	forge := filepath.Join(shim.Dir(st), shim.FileName("forge"))

	// Locked but never installed: the shim says what to run, and does not
	// run it.
	notSynced := filepath.Join(root, "not-synced")
	project(t, notSynced, "foundry", "forge", "1.7.4", strings.Repeat("a", 64))
	code, out := run(t, notSynced, forge, st.Root, nil, "--version")
	if code == 0 || !strings.Contains(out, `run "block sync"`) {
		t.Errorf("not synced: exit %d, output %q", code, out)
	}
	if strings.Contains(out, "faketool") {
		t.Error("the shim ran a tool the project had not installed")
	}

	// A manifest that moved on: the shim reports it instead of resolving.
	stale := filepath.Join(root, "stale")
	project(t, stale, "foundry", "forge", "1.7.4", digest)
	manifest := filepath.Join(stale, "block.toml")
	if err := os.WriteFile(manifest, []byte("[tools]\nfoundry = \"1.6\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out = run(t, stale, forge, st.Root, nil, "--version")
	if code == 0 || !strings.Contains(out, `block.lock is stale; run "block lock"`) {
		t.Errorf("stale: exit %d, output %q", code, out)
	}
	before, err := os.ReadFile(filepath.Join(stale, "block.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), `constraint = "1.7"`) {
		t.Error("the shim rewrote the lockfile")
	}
}

func TestShimOutsideAProjectStepsAside(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	st := &store.Store{Root: filepath.Join(root, "home")}
	if _, err := shim.Ensure(st, blockBinary(t), []string{"forge"}); err != nil {
		t.Fatal(err)
	}
	forge := filepath.Join(shim.Dir(st), shim.FileName("forge"))
	outside := t.TempDir()

	// Nothing else provides the command: a plain explanation, not a crash.
	code, out := run(t, outside, forge, st.Root, nil, "--version")
	if code == 0 || !strings.Contains(out, "no block project") {
		t.Errorf("outside a project: exit %d, output %q", code, out)
	}

	// Something else does: block gets out of the way rather than taking the
	// command away from the rest of the system.
	systemDir := t.TempDir()
	systemForge := filepath.Join(systemDir, "forge"+store.ExeSuffix)
	if err := copyFile(buildFakeTool(t, "system"), systemForge); err != nil {
		t.Fatal(err)
	}
	code, out = run(t, outside, forge, st.Root, []string{shim.Dir(st), systemDir}, "--version")
	if code != 0 || !strings.Contains(out, "faketool system") {
		t.Errorf("fallback: exit %d, output %q", code, out)
	}

	// And a project that does not lock the command behaves the same way.
	other := filepath.Join(root, "other")
	digest := installTool(t, st, "hermes", "hermes", "1.13.0")
	project(t, other, "hermes", "hermes", "1.13.0", digest)
	code, out = run(t, other, forge, st.Root, []string{shim.Dir(st), systemDir}, "--version")
	if code != 0 || !strings.Contains(out, "faketool system") {
		t.Errorf("unlocked command: exit %d, output %q", code, out)
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}
