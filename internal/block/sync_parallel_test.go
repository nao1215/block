package block

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/store"
)

// threeTools is a manifest whose three artifacts come from three upstreams,
// so a sync of it is three downloads.
const threeTools = `[tools.foundry]
version = "1.7"
[tools.foundry.source]
type = "github_release"
repo = "foundry-rs/foundry"
asset = "foundry_v{version}_{os}_{arch}.tar.gz"
bin = ["forge", "cast", "anvil", "chisel"]

[tools.hermes]
version = "1.13"
[tools.hermes.source]
type = "github_release"
repo = "informalsystems/hermes"
asset = "hermes-v{version}-{arch}-{os}.tar.gz"
bin = ["hermes"]
[tools.hermes.source.os]
linux = "unknown-linux-gnu"
darwin = "apple-darwin"
windows = "pc-windows-msvc"
[tools.hermes.source.arch]
amd64 = "x86_64"
arm64 = "aarch64"

[tools.cometbft]
version = "1.7"
[tools.cometbft.source]
type = "github_release"
repo = "cometbft/cometbft"
asset = "cometbft_{version}_{os}_{arch}.tar.gz"
bin = ["cometbft"]
`

// gateLock routes every download in the lockfile through a fakegh gate.
func (h *harness) gateLock(t *testing.T, key string, n int) {
	t.Helper()
	lock := strings.ReplaceAll(h.lockText(t), h.srv.URL+"/download/", h.srv.URL+"/gate/"+key+"/"+itoa(n)+"/download/")
	if err := os.WriteFile(h.LockPath(), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string { return string(rune('0' + n)) }

func (h *harness) gateStats(t *testing.T, key string) (arrived, maxInflight int, opened bool) {
	t.Helper()
	resp, err := http.Get(h.srv.URL + "/gate/" + key + "/stats") //nolint:noctx // test
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	var stats struct {
		Arrived     int  `json:"arrived"`
		MaxInflight int  `json:"max_inflight"`
		Opened      bool `json:"opened"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	return stats.Arrived, stats.MaxInflight, stats.Opened
}

// The fake GitHub holds each download at a gate until three are waiting
// together, so a sync that downloaded one tool at a time could never finish:
// the first download would wait for company that only arrives after it is
// done. Three installs and an open gate are the proof that the downloads ran
// side by side — no clock involved.
func TestSyncDownloadsToolsSideBySide(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.fake.GateTimeout = 5 * time.Second
	h.manifest(t, threeTools)
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	h.gateLock(t, "three", 3)
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatalf("Sync() = %v; the downloads did not run side by side", err)
	}
	// The report is in lockfile order whatever order the downloads finished in.
	want := "cometbft  1.7.4   installed\nfoundry   1.7.4   installed\nhermes    1.13.0  installed\ncommands: anvil, cast, chisel, cometbft, forge, hermes\n"
	if h.stdout.String() != want {
		t.Errorf("Sync() stdout = %q, want %q", h.stdout, want)
	}
	if arrived, maxInflight, opened := h.gateStats(t, "three"); arrived != 3 || maxInflight != 3 || !opened {
		t.Errorf("gate saw arrived=%d max_inflight=%d opened=%v, want 3, 3, true", arrived, maxInflight, opened)
	}
	for _, cmd := range []string{"forge", "hermes", "cometbft"} {
		if _, err := h.Which(cmd); err != nil {
			t.Errorf("Which(%s) after the parallel sync = %v", cmd, err)
		}
	}
	// Everything is in the store, nothing is left in flight.
	tools, err := os.ReadDir(filepath.Join(h.Store.Root, "tools"))
	if err != nil || len(tools) != 3 {
		t.Errorf("tools/ = %v, %v", tools, err)
	}
	if !strings.HasPrefix(h.stderr.String(), "note: add ") {
		t.Errorf("stderr = %q", h.stderr)
	}
}

// One artifact that cannot be had fails the sync with that artifact's error,
// and the downloads beside it stop: the other tool's download is parked at a
// gate nothing else will ever reach, so a sync that did not cancel it would
// sit there until the gate gave up.
func TestSyncFailureStopsTheOtherDownloads(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.fake.GateTimeout = 30 * time.Second
	h.manifest(t, threeTools)
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	// foundry's bytes are gone; the other two wait at a gate of three that
	// two can never fill.
	lock := h.lockText(t)
	lock = strings.ReplaceAll(lock, h.srv.URL+"/download/", h.srv.URL+"/gate/stop/3/download/")
	lock = strings.ReplaceAll(lock, "/gate/stop/3/download/foundry-rs/", "/download/foundry-rs/")
	lock = strings.ReplaceAll(lock, "foundry_v1.7.4_linux_amd64.tar.gz", "foundry_v1.7.4_gone.tar.gz")
	if err := os.WriteFile(h.LockPath(), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err := h.Sync(ctx)
	if err == nil || !strings.Contains(err.Error(), "foundry: download ") || !strings.HasSuffix(err.Error(), "404 Not Found") {
		t.Fatalf("Sync() = %v, want foundry's 404", err)
	}
	if diag.Of(err) != diag.DownloadFailed {
		t.Errorf("Sync() error code = %s, want %s", diag.Of(err), diag.DownloadFailed)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Errorf("Sync() took %s: the failing download did not stop the others", took)
	}
	// Nothing was installed and no download was published: the store holds
	// only what a complete install would have put there, which is nothing.
	if _, statErr := os.Stat(filepath.Join(h.Store.Root, "tools")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("tools/ exists after a failed sync: %v", statErr)
	}
	entries, _ := os.ReadDir(filepath.Join(h.Store.Root, "cache", "sha256"))
	if len(entries) != 0 {
		t.Errorf("cache holds %d blobs after a failed sync, want none", len(entries))
	}
}

// A cache hit beside a download: the cached tool is reported as such and
// the other is fetched, and neither is told apart by when it finished.
func TestSyncMixesCachedAndDownloaded(t *testing.T) {
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
	h.manifest(t, threeTools)
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	h.gateLock(t, "mixed", 2)
	h.reset()
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	want := "cometbft  1.7.4   installed\nfoundry   1.7.4   cached\nhermes    1.13.0  installed\ncommands: anvil, cast, chisel, cometbft, forge, hermes\n"
	if h.stdout.String() != want {
		t.Errorf("Sync() stdout = %q, want %q", h.stdout, want)
	}
	if arrived, _, opened := h.gateStats(t, "mixed"); arrived != 2 || !opened {
		t.Errorf("gate saw arrived=%d opened=%v, want the two uncached downloads together", arrived, opened)
	}
}

func TestWhich(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	ctx := context.Background()

	// No project, no lock, not synced: each refusal is the one exec gives.
	if _, err := h.Which("forge"); diag.Of(err) != diag.ManifestMissing {
		t.Errorf("Which(no manifest) = %v, want %s", err, diag.ManifestMissing)
	}
	h.manifest(t, "[tools]\nfoundry = \"1.7\"\n")
	if _, err := h.Which("forge"); diag.Of(err) != diag.LockMissing {
		t.Errorf("Which(no lock) = %v, want %s", err, diag.LockMissing)
	}
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Which("forge"); err == nil || err.Error() != `foundry 1.7.4 is not installed; run "block sync"` {
		t.Errorf("Which(before sync) = %v", err)
	}
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	h.offline()

	got, err := h.Which("forge")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) || !strings.HasPrefix(got, filepath.Join(h.Store.Root, "tools", "foundry", "1.7.4-")) || filepath.Base(got) != "forge"+store.ExeSuffix {
		t.Errorf("Which(forge) = %q, want the forge inside the foundry install", got)
	}
	if st, err := os.Stat(got); err != nil || st.Mode()&0o111 == 0 {
		t.Errorf("Which(forge) = %q: not an executable file (%v)", got, err)
	}
	// The same executable exec runs.
	tc, err := h.Toolchain()
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := tc.Command("forge", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != got {
		t.Errorf("exec runs %q, which prints %q", cmd.Path, got)
	}
	// Every command of the tool, and nothing the tool does not provide — not
	// even a command PATH has.
	for _, c := range []string{"cast", "anvil", "chisel"} {
		if p, err := h.Which(c); err != nil || filepath.Dir(p) != filepath.Dir(got) {
			t.Errorf("Which(%s) = %q, %v", c, p, err)
		}
	}
	for _, c := range []string{"sh", "hermes", "foundry", ""} {
		if p, err := h.Which(c); diag.Of(err) != diag.CommandNotFound || p != "" {
			t.Errorf("Which(%q) = %q, %v; want %s", c, p, err, diag.CommandNotFound)
		}
	}
	if _, err := h.Which("hermes"); err == nil || err.Error() != `block.toml does not lock a tool providing the command "hermes"` {
		t.Errorf("Which(hermes) = %v", err)
	}
	// A stale lock is refused, as exec refuses it.
	h.manifest(t, "[tools]\nfoundry = \"1.6\"\n")
	if _, err := h.Which("forge"); diag.Of(err) != diag.LockStale {
		t.Errorf("Which(stale) = %v, want %s", err, diag.LockStale)
	}
}
