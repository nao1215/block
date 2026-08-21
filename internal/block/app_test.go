package block

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/fakegh"
	"github.com/nao1215/block/internal/fetch"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/lockfile"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/store"
	"github.com/nao1215/block/registry"
)

// harness is an App wired to an in-process fake GitHub, a private store and
// a temp project directory.
type harness struct {
	*App
	srv    *httptest.Server
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func newHarness(t *testing.T, snapshot string) *harness {
	t.Helper()
	fake := fakegh.New(fakegh.Fixtures())
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	fake.SetBase(srv.URL)
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	st := &store.Store{Root: filepath.Join(t.TempDir(), "home")}
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	h := &harness{srv: srv, stdout: out, stderr: errOut}
	h.App = &App{
		Dir:      t.TempDir(),
		Platform: platform.Platform{OS: "linux", Arch: "amd64"},
		Registry: reg,
		Releases: &github.Client{BaseURL: srv.URL + snapshot, HTTP: srv.Client()},
		Fetcher:  fetch.New(st.CacheDir(), "block/test"),
		Store:    st,
		Stdout:   out,
		Stderr:   errOut,
	}
	return h
}

// at switches the fake GitHub to another point in time.
func (h *harness) at(snapshot string) {
	h.Releases = &github.Client{BaseURL: h.srv.URL + snapshot, HTTP: h.srv.Client()}
}

// offline makes any upstream call fail loudly.
func (h *harness) offline() {
	h.Releases = &github.Client{BaseURL: "http://127.0.0.1:1", HTTP: h.srv.Client()}
}

func (h *harness) manifest(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(h.ManifestPath(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) lockText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(h.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (h *harness) reset() {
	h.stdout.Reset()
	h.stderr.Reset()
}

func TestLockResolvesAndRelocks(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if h.stdout.String() != "foundry  locked 1.7.4\nwrote block.lock\n" {
		t.Errorf("stdout = %q", h.stdout)
	}
	if !strings.Contains(h.stderr.String(), "downloading ") {
		t.Errorf("stderr = %q", h.stderr)
	}
	first := h.lockText(t)
	for _, forbidden := range []string{"repo =", "asset ="} {
		if strings.Contains(first, forbidden) {
			t.Errorf("lockfile contains recipe field %q", forbidden)
		}
	}

	// Nothing new upstream: lock is a no-op that touches nothing.
	h.reset()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if h.stdout.String() != "foundry  1.7.4\nblock.lock is up to date\n" || h.stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q", h.stdout, h.stderr)
	}
	if h.lockText(t) != first {
		t.Error("lockfile changed on a no-op lock")
	}

	// A new platform fetches only that artifact from the pinned release.
	h.manifest(t, "platforms = [\"linux/amd64\", \"darwin/arm64\"]\n[tools]\nfoundry = \"1.7\"\n")
	h.reset()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(h.stderr.String(), "downloading ") != 1 || !strings.Contains(h.stderr.String(), "v1.7.4/foundry_v1.7.4_darwin_arm64.tar.gz") {
		t.Errorf("stderr = %q", h.stderr)
	}
	l, err := lockfile.Load(h.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if tool, _ := l.Tool("foundry"); len(tool.Artifacts) != 2 {
		t.Errorf("artifacts = %+v", tool.Artifacts)
	}

	// Upstream publishes 1.7.5: lock moves the pin.
	h.at("")
	h.reset()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout.String(), "foundry  1.7.4 -> 1.7.5\nwrote block.lock") {
		t.Errorf("stdout = %q", h.stdout)
	}

	// A tightened constraint moves the pin back.
	h.manifest(t, "[tools]\nfoundry = \"1.6\"\n")
	h.reset()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout.String(), "foundry  1.7.5 -> 1.6.0") {
		t.Errorf("stdout = %q", h.stdout)
	}
}

func TestLockNamesOnlyThoseTools(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	h.at("")
	h.reset()
	if err := h.Lock(ctx, []string{"hermes"}, false); err != nil {
		t.Fatal(err)
	}
	if h.stdout.String() != "foundry  1.7.4\nhermes   1.13.0 -> 1.13.1\nwrote block.lock\n" {
		t.Errorf("stdout = %q", h.stdout)
	}
	if err := h.Lock(ctx, []string{"geth"}, false); err == nil || err.Error() != `tool "geth" is not declared in block.toml` {
		t.Errorf("Lock(unknown) error = %v", err)
	}
	// A named lock still resolves a tool that was never pinned or whose
	// constraint changed, because the old pin is not reusable.
	h.manifest(t, "[tools]\nfoundry = \"1.6\"\nhermes = \"1.13\"\n")
	h.reset()
	if err := h.Lock(ctx, []string{"hermes"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout.String(), "foundry  1.7.4 -> 1.6.0") {
		t.Errorf("stdout = %q", h.stdout)
	}
}

func TestLockCheck(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n")
	ctx := context.Background()
	// No lock yet: everything is missing.
	err := h.Lock(ctx, nil, true)
	if !errors.Is(err, ErrOutdated) || h.stdout.String() != "foundry  missing 1.7.4\nhermes   missing 1.13.0\n" {
		t.Errorf("check(no lock) = %v, %q", err, h.stdout)
	}
	if _, err := os.Stat(h.LockPath()); err == nil {
		t.Fatal("check wrote block.lock")
	}
	if h.stderr.Len() != 0 {
		t.Errorf("check downloaded something: %q", h.stderr)
	}
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	before := h.lockText(t)
	h.reset()
	if err := h.Lock(ctx, nil, true); err != nil || h.stdout.String() != "foundry  1.7.4 (up-to-date)\nhermes   1.13.0 (up-to-date)\n" {
		t.Errorf("check(current) = %v, %q", err, h.stdout)
	}
	h.at("")
	h.reset()
	err = h.Lock(ctx, nil, true)
	if !errors.Is(err, ErrOutdated) || h.stdout.String() != "foundry  1.7.4 -> 1.7.5\nhermes   1.13.0 -> 1.13.1\n" {
		t.Errorf("check(outdated) = %v, %q", err, h.stdout)
	}
	if h.lockText(t) != before || h.stderr.Len() != 0 {
		t.Error("check modified block.lock or downloaded")
	}
	// A dropped tool and a new platform are changes too.
	h.manifest(t, "platforms = [\"linux/amd64\", \"darwin/arm64\"]\n[tools]\nfoundry = \"1.7.4\"\n")
	h.reset()
	err = h.Lock(ctx, nil, true)
	if !errors.Is(err, ErrOutdated) || !strings.Contains(h.stdout.String(), "hermes  1.13.0 (no longer in block.toml)") {
		t.Errorf("check(dropped) = %v, %q", err, h.stdout)
	}
	if h.lockText(t) != before {
		t.Error("check modified block.lock")
	}
}

func TestLockErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, manifest, want string
	}{
		{"unknown tool", "[tools]\ngeth = \"1\"\n", `unknown tool "geth": it is not in the registry (known tools: foundry, hermes); define [tools.geth.source] in block.toml`},
		{"no match", "[tools]\nfoundry = \"9\"\n", `foundry: no version of foundry-rs/foundry matches "9"`},
		{"prerelease only", "[tools]\nfoundry = \"1.9\"\n", `foundry: no published release of foundry-rs/foundry matches "1.9"`},
		{"unsupported platform", "[tools.maconly]\nversion = \"0.1\"\n[tools.maconly.source]\ntype = \"github_release\"\nrepo = \"example/maconly\"\nasset = \"maconly_{version}_{os}_{arch}.tar.gz\"\nplatforms = [\"darwin/arm64\"]\nbin = [\"maconly\"]\n", "maconly: unsupported platform linux/amd64 (available: darwin/arm64)"},
		{"missing repo", "[tools.ghost]\nversion = \"1\"\n[tools.ghost.source]\ntype = \"github_release\"\nrepo = \"example/ghost\"\nasset = \"ghost_{version}.tar.gz\"\nbin = [\"ghost\"]\n", "ghost: repository example/ghost: not found"},
		{"bad manifest", "[tools]\n", "block.toml: no tools declared"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			h.manifest(t, tt.manifest)
			err := h.Lock(context.Background(), nil, false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Lock() error = %v, want containing %q", err, tt.want)
			}
			if _, err := os.Stat(h.LockPath()); err == nil {
				t.Error("a failed lock wrote block.lock")
			}
		})
	}
	h := newHarness(t, "")
	if err := h.Lock(context.Background(), nil, false); err == nil || err.Error() != "block.toml not found" {
		t.Errorf("Lock(no manifest) error = %v", err)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte("[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n[tools.foo]\nversion = \"1\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"example/foo\"\nasset = \"foo_{version}.tar.gz\"\nbin = [\"foo\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := lockfile.Parse([]byte(`version = 1
[[tools]]
name = "foundry"
constraint = "1.6"
version = "1.6.0"
bin = ["forge"]
source = "sha256:same"
[[tools.artifacts]]
platform = "darwin/arm64"
url = "https://example.com/a.tar.gz"
sha256 = "593c607acd4d8fe57f560298f64779441a0aa7461893223def00eeedc612d0bb"

[[tools]]
name = "foo"
constraint = "1"
version = "1.0.0"
bin = ["foo"]
source = "sha256:old"

[[tools]]
name = "legacy"
constraint = "1"
version = "1.0.0"
bin = ["legacy"]
source = "sha256:x"
`))
	if err != nil {
		t.Fatal(err)
	}
	foo, _ := m.Tool("foo")
	sources := map[string]recipe.Source{"foo": *foo.Source}
	got := Check(m, l, sources, []platform.Platform{{OS: "linux", Arch: "amd64"}})
	want := []string{
		"foo: the source definition changed since block.lock was resolved",
		"foo: block.lock has no artifact for linux/amd64",
		`foundry: block.toml wants "1.7" but block.lock was resolved from "1.6"`,
		"foundry: block.lock has no artifact for linux/amd64",
		"hermes is declared in block.toml but missing from block.lock",
		"legacy is in block.lock but not declared in block.toml",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Check() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if reasons := Check(m, l, nil, nil); len(reasons) != 3 {
		t.Errorf("Check(no sources, no platforms) = %v", reasons)
	}
}

func TestSyncAndExecContract(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	ctx := context.Background()

	if err := h.Sync(ctx); err == nil || err.Error() != `block.lock not found; run "block lock"` {
		t.Errorf("Sync(no lock) error = %v", err)
	}
	if _, err := h.Exec(ctx, []string{"forge"}, nil); err == nil || !strings.Contains(err.Error(), `block.lock not found`) {
		t.Errorf("Exec(no lock) error = %v", err)
	}
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Exec(ctx, []string{"forge"}, nil); err == nil || err.Error() != `foundry 1.7.4 is not installed; run "block sync"` {
		t.Errorf("Exec(before sync) error = %v", err)
	}
	before := h.lockText(t)

	// Upstream moves on and the API goes away: sync must neither notice nor care.
	h.offline()
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if h.stdout.String() != "foundry  1.7.4  installed\n" {
		t.Errorf("Sync() stdout = %q", h.stdout)
	}
	if h.lockText(t) != before {
		t.Error("sync rewrote block.lock")
	}
	h.reset()
	if err := h.Sync(ctx); err != nil || h.stdout.String() != "foundry  1.7.4  cached\n" {
		t.Errorf("Sync(again) = %q, %v", h.stdout, err)
	}
	dirs, err := h.Env()
	if err != nil || len(dirs) != 1 || !strings.HasPrefix(dirs[0], filepath.Join(h.Store.Root, "tools", "foundry", "1.7.4-")) {
		t.Errorf("Env() = %v, %v", dirs, err)
	}
	h.reset()
	code, err := h.Exec(ctx, []string{"forge", "test"}, nil)
	if err != nil || code != 0 || h.stdout.String() != "forge 1.7.4 (fake)\nargs: test\n" {
		t.Errorf("Exec() = %d, %v, stdout %q", code, err, h.stdout)
	}
	code, err = h.Exec(ctx, []string{"cast", "--exit", "7"}, nil)
	if err != nil || code != 7 {
		t.Errorf("Exec(exit 7) = %d, %v", code, err)
	}
	if _, err := h.Exec(ctx, nil, nil); err == nil {
		t.Error("Exec(no args) succeeded")
	}
	if _, err := h.Exec(ctx, []string{"no-such-command-xyz"}, nil); err == nil || !strings.Contains(err.Error(), `command "no-such-command-xyz" not found`) {
		t.Errorf("Exec(missing) error = %v", err)
	}

	// A stale manifest stops sync; nothing is resolved or written.
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n")
	err = h.Sync(ctx)
	if err == nil || err.Error() != "block.lock is stale; run \"block lock\"\n  hermes is declared in block.toml but missing from block.lock" {
		t.Errorf("Sync(stale) error = %v", err)
	}
	if h.lockText(t) != before {
		t.Error("sync modified block.lock")
	}
	// A changed project-local source is stale too.
	h.manifest(t, "[tools.foundry]\nversion = \"1.7\"\n[tools.foundry.source]\ntype = \"github_release\"\nrepo = \"foundry-rs/foundry\"\nasset = \"foundry_v{version}_{os}_{arch}.tar.gz\"\nbin = [\"forge\"]\n")
	err = h.Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), "foundry: the source definition changed since block.lock was resolved") {
		t.Errorf("Sync(source changed) error = %v", err)
	}
}

func TestSyncPlatformHandling(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "platforms = [\"darwin/arm64\"]\n[tools]\nfoundry = \"1.7\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	before := h.lockText(t)
	err := h.Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), "foundry: block.lock has no artifact for linux/amd64") {
		t.Errorf("Sync() error = %v", err)
	}
	if _, err := h.Env(); err == nil || !strings.Contains(err.Error(), "no artifact for linux/amd64") {
		t.Errorf("Env() error = %v", err)
	}
	if h.lockText(t) != before {
		t.Error("sync added a platform on its own")
	}
	h.Platform = platform.Platform{OS: "windows", Arch: "amd64"}
	if err := h.Sync(ctx); err == nil || err.Error() != "unsupported platform windows/amd64" {
		t.Errorf("Sync(windows) error = %v", err)
	}
}

func TestSyncChecksumMismatch(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	l, err := lockfile.Load(h.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	tool, _ := l.Tool("foundry")
	tool.Artifacts[0].SHA256 = strings.Repeat("0", 64)
	if err := lockfile.Write(h.LockPath(), l); err != nil {
		t.Fatal(err)
	}
	err = h.Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), "foundry: checksum mismatch for ") {
		t.Errorf("Sync() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.Store.Root, "tools")); err == nil {
		t.Error("a mismatching artifact was installed")
	}
}

func TestSyncRefusesBadArchives(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, repo, bin, want string
	}{
		{"traversal", "example/evil", "evil", `refusing to extract "../escape": path escapes the destination`},
		{"symlink", "example/linky", "linky", `refusing to extract link "etc-passwd"`},
		{"missing bin", "example/nobin", "nobin", `does not contain executable "nobin"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "")
			name := strings.TrimPrefix(tt.repo, "example/")
			h.manifest(t, "[tools."+name+"]\nversion = \"1\"\n[tools."+name+".source]\ntype = \"github_release\"\nrepo = \""+tt.repo+"\"\nasset = \""+name+"_{version}_{os}_{arch}.tar.gz\"\nbin = [\""+tt.bin+"\"]\n")
			ctx := context.Background()
			if err := h.Lock(ctx, nil, false); err != nil {
				t.Fatal(err)
			}
			err := h.Sync(ctx)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Sync() error = %v, want containing %q", err, tt.want)
			}
			entries, _ := os.ReadDir(filepath.Join(h.Store.Root, "tools", name))
			if len(entries) != 0 {
				t.Errorf("install dir not clean: %v", entries)
			}
		})
	}
}

func TestAssetName(t *testing.T) {
	t.Parallel()
	if got := assetName("https://github.com/o/r/releases/download/v1/foo_v1_linux_amd64.tar.gz?x=1"); got != "foo_v1_linux_amd64.tar.gz" {
		t.Errorf("assetName() = %q", got)
	}
	if got := assetName("://bad"); got != "://bad" {
		t.Errorf("assetName(bad) = %q", got)
	}
}
