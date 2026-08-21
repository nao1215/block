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
	"testing/fstest"

	"github.com/nao1215/block/internal/fakegh"
	"github.com/nao1215/block/internal/fetch"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/lockfile"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
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

// later moves the fake GitHub from t1 to the latest point in time.
func (h *harness) later() {
	h.Releases = &github.Client{BaseURL: h.srv.URL, HTTP: h.srv.Client()}
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
	// GitHub publishes a digest for foundry's assets: nothing is downloaded.
	if h.stderr.Len() != 0 {
		t.Errorf("stderr = %q", h.stderr)
	}
	first := h.lockText(t)
	for _, forbidden := range []string{"repo =", "asset =", "source ="} {
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
	if h.stderr.Len() != 0 {
		t.Errorf("stderr = %q", h.stderr)
	}
	l, err := lockfile.Load(h.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if tool, _ := l.Tool("foundry"); len(tool.Artifacts) != 2 || !strings.Contains(tool.Artifacts[0].URL, "darwin_arm64") {
		t.Errorf("artifacts = %+v", tool.Artifacts)
	}

	// Upstream publishes 1.7.5: lock moves the pin.
	h.later()
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

func TestLockWithoutUpstreamDigestDownloadsOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools.foo]\nversion = \"1.2\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"example/foo\"\nasset = \"foo_{version}_{os}_{arch}.tar.gz\"\nbin = [\"foo\"]\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(h.stderr.String(), "downloading ") != 1 {
		t.Errorf("stderr = %q", h.stderr)
	}
	if !strings.Contains(h.lockText(t), `source = "sha256:`) {
		t.Error("a project-local tool must record its source fingerprint")
	}
	h.reset()
	if err := h.Lock(ctx, nil, false); err != nil || h.stderr.Len() != 0 {
		t.Errorf("relock = %v, stderr %q (artifact must be reused)", err, h.stderr)
	}
}

func TestLockHTTPSourceAndRawExecutable(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools.geth]\nversion = \"1.17\"\n[tools.geth.source]\ntype = \"http\"\nrepo = \"ethereum/go-ethereum\"\nurl = \""+h.srv.URL+"/blobs/geth-{os}-{arch}-{version}-{commit}.tar.gz\"\nstrip_components = 1\nbin = [\"geth\"]\n\n[tools.rawbin]\nversion = \"1\"\n[tools.rawbin.source]\ntype = \"github_release\"\nrepo = \"example/rawbin\"\nasset = \"rawbin-{os}-{arch}\"\nbin = [\"rawbin\"]\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	lock := h.lockText(t)
	if !strings.Contains(lock, "/blobs/geth-linux-amd64-1.17.4-") || !strings.Contains(lock, "strip_components = 1") || !strings.Contains(lock, "/rawbin-linux-amd64\"") {
		t.Errorf("lock = %s", lock)
	}
	h.offline()
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	h.reset()
	if code, err := h.Exec(ctx, []string{"geth", "version"}, nil); err != nil || code != 0 || !strings.Contains(h.stdout.String(), "geth 1.17.4 (fake)") {
		t.Errorf("Exec(geth) = %d, %v, %q", code, err, h.stdout)
	}
	h.reset()
	if code, err := h.Exec(ctx, []string{"rawbin"}, nil); err != nil || code != 0 || !strings.Contains(h.stdout.String(), "rawbin 1.0.0 (fake)") {
		t.Errorf("Exec(rawbin) = %d, %v, %q", code, err, h.stdout)
	}
	// T2: geth 1.17.5 is tagged; the commit-named blob resolves too.
	h.later()
	h.reset()
	err := h.Lock(ctx, nil, true)
	if !errors.Is(err, ErrOutdated) || !strings.Contains(h.stdout.String(), "geth    1.17.4 -> 1.17.5") {
		t.Errorf("check = %v, %q", err, h.stdout)
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
	h.later()
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
	// A kept pin still gets an artifact for a newly declared platform: the
	// pinned version is resolved exactly, not moved.
	h.manifest(t, "platforms = [\"linux/amd64\", \"darwin/arm64\"]\n[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n")
	h.reset()
	if err := h.Lock(ctx, []string{"hermes"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout.String(), "foundry  1.7.4 (artifact for darwin/arm64 added)") {
		t.Errorf("stdout = %q (foundry must keep its pin and gain the platform)", h.stdout)
	}
	l, err := lockfile.Load(h.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	foundry, _ := l.Tool("foundry")
	if len(foundry.Artifacts) != 2 || foundry.Version != "1.7.4" {
		t.Errorf("foundry = %+v", foundry)
	}
	if a, ok := foundry.Artifact(platform.Platform{OS: "darwin", Arch: "arm64"}); !ok || !strings.Contains(a.URL, "v1.7.4/foundry_v1.7.4_darwin_arm64") {
		t.Errorf("darwin artifact = %+v, %v", a, ok)
	}
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n")

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
	h.later()
	h.reset()
	err = h.Lock(ctx, nil, true)
	if !errors.Is(err, ErrOutdated) || h.stdout.String() != "foundry  1.7.4 -> 1.7.5\nhermes   1.13.0 -> 1.13.1\n" {
		t.Errorf("check(outdated) = %v, %q", err, h.stdout)
	}
	if h.lockText(t) != before || h.stderr.Len() != 0 {
		t.Error("check modified block.lock or downloaded")
	}
	// check never downloads, even for tools without an upstream digest.
	h.manifest(t, "[tools.foo]\nversion = \"1.2\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"example/foo\"\nasset = \"foo_{version}_{os}_{arch}.tar.gz\"\nbin = [\"foo\"]\n")
	h.reset()
	if err := h.Lock(ctx, nil, true); !errors.Is(err, ErrOutdated) || h.stderr.Len() != 0 {
		t.Errorf("check(no digest) = %v, stderr %q", err, h.stderr)
	}
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n")
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

// setRegistry replaces the built-in registry with one recipe, so that a test
// can change what the registry says about a tool the way a registry update
// would.
func (h *harness) setRegistry(t *testing.T, body string) {
	t.Helper()
	reg, err := registry.Load(fstest.MapFS{"foundry.toml": {Data: []byte(body)}})
	if err != nil {
		t.Fatal(err)
	}
	h.Registry = reg
}

const foundryRecipe = `name = "foundry"
ecosystems = ["ethereum"]
description = "Fast Ethereum application toolkit"
[source]
type = "github_release"
repo = "foundry-rs/foundry"
asset = "foundry_v{version}_{os}_{arch}.tar.gz"
bin = ["forge", "cast", "anvil", "chisel"]
`

// TestLockCheckDetectsMetadataChanges covers what a version comparison alone
// misses: block lock would rewrite block.lock for any of these, so
// lock --check has to say so.
func TestLockCheckDetectsMetadataChanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// change is applied after the initial lock; none of them moves the
		// resolved version.
		change func(t *testing.T, h *harness)
		want   string
	}{
		{
			name: "constraint narrowed to the version it already resolved",
			change: func(t *testing.T, h *harness) {
				h.manifest(t, "[tools]\nfoundry = \"1.7.4\"\n")
			},
			want: "foundry  1.7.4 (constraint)",
		},
		{
			name: "registry recipe renamed the executables",
			change: func(t *testing.T, h *harness) {
				h.setRegistry(t, strings.Replace(foundryRecipe, `bin = ["forge", "cast", "anvil", "chisel"]`, `bin = ["forge"]`, 1))
			},
			want: "foundry  1.7.4 (bin)",
		},
		{
			name: "registry recipe changed how the archive is unpacked",
			change: func(t *testing.T, h *harness) {
				h.setRegistry(t, foundryRecipe+"strip_components = 1\n")
			},
			want: "foundry  1.7.4 (strip_components)",
		},
		{
			name: "registry recipe selects a different asset",
			change: func(t *testing.T, h *harness) {
				// The upstream renamed what it calls this architecture.
				h.setRegistry(t, foundryRecipe+"[source.arch]\namd64 = \"arm64\"\n")
			},
			want: "foundry  1.7.4 (artifact for linux/amd64)",
		},
		{
			name: "the tool moved to a project-local source",
			change: func(t *testing.T, h *harness) {
				h.manifest(t, "[tools.foundry]\nversion = \"1.7\"\n[tools.foundry.source]\ntype = \"github_release\"\nrepo = \"foundry-rs/foundry\"\nasset = \"foundry_v{version}_{os}_{arch}.tar.gz\"\nbin = [\"forge\"]\n")
			},
			want: "foundry  1.7.4 (bin, source)",
		},
		{
			name: "a platform was added",
			change: func(t *testing.T, h *harness) {
				h.manifest(t, "platforms = [\"linux/amd64\", \"darwin/arm64\"]\n[tools]\nfoundry = \"1.7\"\n")
			},
			want: "foundry  1.7.4 (artifact for darwin/arm64 added)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "/t1")
			h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
			ctx := context.Background()
			if err := h.Lock(ctx, nil, false); err != nil {
				t.Fatal(err)
			}
			before := h.lockText(t)
			h.reset()
			// Nothing has changed yet: check must be quiet.
			if err := h.Lock(ctx, nil, true); err != nil {
				t.Fatalf("check before the change = %v, %q", err, h.stdout)
			}

			tt.change(t, h)
			h.reset()
			err := h.Lock(ctx, nil, true)
			if !errors.Is(err, ErrOutdated) {
				t.Fatalf("check = %v, want ErrOutdated; stdout %q", err, h.stdout)
			}
			if got := strings.TrimRight(h.stdout.String(), "\n"); got != tt.want {
				t.Errorf("check stdout = %q, want %q", got, tt.want)
			}
			if h.lockText(t) != before {
				t.Error("check rewrote block.lock")
			}
			if h.stderr.Len() != 0 {
				t.Errorf("check downloaded something: %q", h.stderr)
			}

			// And the change really does land when lock runs for real.
			h.reset()
			if err := h.Lock(ctx, nil, false); err != nil {
				t.Fatal(err)
			}
			if h.lockText(t) == before {
				t.Error("lock did not apply the change check reported")
			}
			h.reset()
			if err := h.Lock(ctx, nil, true); err != nil {
				t.Errorf("check after lock = %v, %q", err, h.stdout)
			}
		})
	}
}

// TestLockAppliesRegistryRecipeChangesAtTheSameVersion is the other half of
// that contract: a recipe fix has to reach the lockfile even when the
// upstream version is unchanged, and sync must keep working from the old
// lockfile until it does.
func TestLockAppliesRegistryRecipeChangesAtTheSameVersion(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	before := h.lockText(t)

	// The registry now names a different asset for this architecture, and
	// says the release ships only forge.
	h.setRegistry(t, strings.Replace(foundryRecipe,
		`bin = ["forge", "cast", "anvil", "chisel"]`, `bin = ["forge"]`, 1)+"[source.arch]\namd64 = \"arm64\"\n")

	// sync is unaffected: it installs the artifacts the lockfile records.
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatalf("a registry change must not stale an existing lock: %v", err)
	}
	if h.stdout.String() != "foundry  1.7.4  cached\n" || h.lockText(t) != before {
		t.Errorf("sync = %q, lock changed = %v", h.stdout, h.lockText(t) != before)
	}

	// lock picks the new recipe up, at the same version.
	h.reset()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stdout.String(), "foundry  1.7.4 (bin, artifact for linux/amd64)") {
		t.Errorf("lock stdout = %q", h.stdout)
	}
	l, err := lockfile.Load(h.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	tool, _ := l.Tool("foundry")
	art, _ := tool.Artifact(h.Platform)
	if !strings.HasSuffix(art.URL, "foundry_v1.7.4_linux_arm64.tar.gz") || strings.Join(tool.Bin, ",") != "forge" {
		t.Errorf("lock = %+v", tool)
	}
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if h.stdout.String() != "foundry  1.7.4  installed\n" {
		t.Errorf("sync = %q", h.stdout)
	}
}

// TestLockReusesDigestForAnUnchangedURL keeps the download budget honest: a
// re-lock that resolves the same artifact must not fetch it again.
func TestLockReusesDigestForAnUnchangedURL(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	// example/foo publishes no digest, so the first lock has to download.
	h.manifest(t, "[tools.foo]\nversion = \"1.2\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"example/foo\"\nasset = \"foo_{version}_{os}_{arch}.tar.gz\"\nbin = [\"foo\"]\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if strings.Count(h.stderr.String(), "downloading ") != 1 {
		t.Fatalf("first lock stderr = %q", h.stderr)
	}
	h.reset()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("re-locking downloaded again: %q", h.stderr)
	}
}

func TestLockErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, manifest, want string
	}{
		{"unknown tool", "[tools]\nnot-a-tool = \"1\"\n", `unknown tool "not-a-tool": it is not in the registry (run "block list" to see the supported tools); define [tools.not-a-tool.source] in block.toml`},
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
	got := Check(m, l, []platform.Platform{{OS: "linux", Arch: "amd64"}})
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
	if reasons := Check(m, l, nil); len(reasons) != 4 {
		t.Errorf("Check(no platforms) = %v", reasons)
	}
	// A registry recipe change never stales a lock: only local sources carry
	// a fingerprint.
	m2, _ := manifest.Parse([]byte("[tools]\nfoundry = \"1.6\"\n"))
	l2, _ := lockfile.Parse([]byte("version = 1\n[[tools]]\nname = \"foundry\"\nconstraint = \"1.6\"\nversion = \"1.6.0\"\nbin = [\"forge\"]\n"))
	if reasons := Check(m2, l2, nil); len(reasons) != 0 {
		t.Errorf("Check(registry tool without source) = %v", reasons)
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
	if h.stdout.String() != "foundry  1.7.4  installed\nshims: anvil, cast, chisel, forge\n" {
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

// TestExecRefusesAStaleLock: running the previous toolchain after block.toml
// changed is exactly the reproducibility hole block exists to close.
func TestExecRefusesAStaleLock(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	before := h.lockText(t)

	// The subtests share one project on purpose: each edits the manifest of
	// the toolchain the previous one left installed.
	for _, tt := range []struct { //nolint:paralleltest // shared project state
		name, manifest, want string
	}{
		{"a tool was added", "[tools]\nfoundry = \"1.7\"\nhermes = \"1.13\"\n", "hermes is declared in block.toml but missing from block.lock"},
		{"a constraint changed", "[tools]\nfoundry = \"1.6\"\n", `foundry: block.toml wants "1.6" but block.lock was resolved from "1.7"`},
		{"a tool was removed", "[tools]\nhermes = \"1.13\"\n", "foundry is in block.lock but not declared in block.toml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h.manifest(t, tt.manifest)
			h.reset()
			_, err := h.Exec(ctx, []string{"forge"}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Exec() error = %v, want containing %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), `block.lock is stale; run "block lock"`) {
				t.Errorf("the error should say how to fix it: %v", err)
			}
			if h.stdout.Len() != 0 {
				t.Errorf("the tool ran anyway: %q", h.stdout)
			}
			// Refusing must stay a read-only, offline verdict.
			if h.lockText(t) != before || h.stderr.Len() != 0 {
				t.Error("exec wrote the lockfile or reached the network")
			}
		})
	}
}

// TestCommandConflict: two tools providing the same command would leave the
// choice to PATH order, which is not something a project can depend on.
func TestCommandConflict(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	// example/rawbin ships a "rawbin" executable; declare a second tool that
	// claims the same command name.
	const twoTools = `[tools.rawbin]
version = "1"
[tools.rawbin.source]
type = "github_release"
repo = "example/rawbin"
asset = "rawbin-{os}-{arch}"
bin = ["rawbin"]

[tools.clash]
version = "1"
[tools.clash.source]
type = "github_release"
repo = "example/rawbin"
asset = "rawbin-{os}-{arch}"
bin = ["rawbin"]
`
	h := newHarness(t, "/t1")
	h.manifest(t, twoTools)
	ctx := context.Background()
	const want = `tools "clash" and "rawbin" both provide the command "rawbin"; remove one from block.toml`
	err := h.Lock(ctx, nil, false)
	if err == nil || err.Error() != want {
		t.Fatalf("Lock() error = %v, want %q", err, want)
	}
	if _, statErr := os.Stat(h.LockPath()); statErr == nil {
		t.Error("a conflicting toolchain was written to block.lock")
	}

	// A lockfile that already contains the conflict — hand-written, or
	// merged from a branch — is refused by sync and exec too.
	h.manifest(t, "[tools.rawbin]\nversion = \"1\"\n[tools.rawbin.source]\ntype = \"github_release\"\nrepo = \"example/rawbin\"\nasset = \"rawbin-{os}-{arch}\"\nbin = [\"rawbin\"]\n")
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	l, err := lockfile.Load(h.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	clash := l.Tools[0]
	clash.Name = "clash"
	l.Tools = append(l.Tools, clash)
	if err := lockfile.Write(h.LockPath(), l); err != nil {
		t.Fatal(err)
	}
	h.manifest(t, twoTools)
	if err := h.Sync(ctx); err == nil || err.Error() != want {
		t.Errorf("Sync() error = %v, want %q", err, want)
	}
	if _, err := h.Exec(ctx, []string{"rawbin"}, nil); err == nil || err.Error() != want {
		t.Errorf("Exec() error = %v, want %q", err, want)
	}
}

// The other half of the same rule: a command name that means two executables
// inside one tool. block used to allow it, and then disagreed with itself —
// the toolchain's command map took the last entry, PATH took the first.
func TestCommandConflictWithinOneTool(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	tests := []struct {
		name string
		bin  string
		want string
	}{
		{
			name: "the same command in two directories",
			bin:  `bin = ["a/foo", "b/foo"]`,
			want: `bin "a/foo" and "b/foo" would both provide the command "foo"`,
		},
		{
			// Windows resolves a command on PATH without regard to case, so
			// this toolchain would install on Linux and collide there. block
			// refuses it on every platform rather than shipping a lockfile
			// that only works on some of them.
			name: "the same command in two spellings",
			bin:  `bin = ["foo", "Foo"]`,
			want: `bin "foo" and "Foo" would both provide the command "Foo"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, "/t1")
			// An archive, so the raw-executable rule ("exactly one bare bin
			// name") is not what refuses this; the ambiguity is.
			h.manifest(t, `[tools.foo]
version = "1"
[tools.foo.source]
type = "github_release"
repo = "example/foo"
asset = "foo_{version}_{os}_{arch}.tar.gz"
`+tt.bin+"\n")
			err := h.Lock(context.Background(), nil, false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Lock() error = %v, want it to contain %q", err, tt.want)
			}
			if _, statErr := os.Stat(h.LockPath()); statErr == nil {
				t.Error("an ambiguous toolchain was written to block.lock")
			}
		})
	}
}

// And a lockfile that already carries the ambiguity — hand-edited, or merged
// from a branch — is refused by lock, sync and exec alike.
func TestAmbiguousLockfileIsRefusedEverywhere(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
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
	// foundry ships forge; add a second path that is the same command.
	l.Tools[0].Bin = append(l.Tools[0].Bin, "extra/FORGE")
	if err := lockfile.Write(h.LockPath(), l); err != nil {
		t.Fatal(err)
	}
	const want = `are both the command "FORGE"`
	if err := h.Sync(ctx); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Sync() error = %v, want it to contain %q", err, want)
	}
	if _, err := h.Exec(ctx, []string{"forge"}, nil); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Exec() error = %v, want it to contain %q", err, want)
	}
	if err := h.Lock(ctx, nil, false); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Lock() error = %v, want it to contain %q", err, want)
	}
}

// TestSyncAndExecRejectDamagedInstalls: a restored CI cache can be truncated
// or partial, and a directory that merely exists proves nothing.
func TestSyncAndExecRejectDamagedInstalls(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	dirs, err := h.Env()
	if err != nil {
		t.Fatal(err)
	}
	installed := dirs[0]

	// Damage it the way a half-restored cache would.
	if err := os.Remove(filepath.Join(installed, "cast")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Env(); err == nil || !strings.Contains(err.Error(), `run "block sync"`) {
		t.Errorf("Env() error = %v, want it to point at sync", err)
	}
	if _, err := h.Exec(ctx, []string{"forge"}, nil); err == nil {
		t.Error("exec ran from a damaged install")
	}
	// sync repairs it rather than calling it cached.
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if h.stdout.String() != "foundry  1.7.4  installed\n" {
		t.Errorf("sync = %q, want a reinstall", h.stdout)
	}
	if _, err := h.Env(); err != nil {
		t.Errorf("Env() after repair = %v", err)
	}

	// A corrupt cached blob is re-downloaded rather than trusted.
	blobs, err := os.ReadDir(filepath.Join(h.Store.CacheDir(), "sha256"))
	if err != nil || len(blobs) == 0 {
		t.Fatalf("cache = %v, %v", blobs, err)
	}
	blob := filepath.Join(h.Store.CacheDir(), "sha256", blobs[0].Name())
	if err := os.WriteFile(blob, []byte("truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(installed); err != nil {
		t.Fatal(err)
	}
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatalf("sync with a corrupt cache = %v", err)
	}
	if _, err := h.Env(); err != nil {
		t.Errorf("Env() = %v", err)
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
	h.Platform = platform.Platform{OS: "plan9", Arch: "amd64"}
	if err := h.Sync(ctx); err == nil || err.Error() != "unsupported platform plan9/amd64" {
		t.Errorf("Sync(plan9) error = %v", err)
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
		{"missing bin", "example/nobin", "nobin", `executable "nobin" is missing`},
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
