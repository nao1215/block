package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nao1215/block/internal/diag"
)

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestFetchDownloadsVerifiesAndCaches(t *testing.T) {
	t.Parallel()
	body := []byte("archive bytes")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("User-Agent") != "block/test" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		switch r.URL.Path {
		case "/a.tar.gz":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := New(t.TempDir(), "block/test")
	ctx := context.Background()

	// First fetch without an expected digest: lock-time behaviour.
	path, sha, cached, err := f.Fetch(ctx, srv.URL+"/a.tar.gz", "")
	if err != nil || cached || sha != digest(body) || path != f.Path(sha) {
		t.Fatalf("Fetch() = %s, %s, %v, %v", path, sha, cached, err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(body) {
		t.Errorf("cached bytes = %q", got)
	}
	// Second fetch with the digest is served from the cache.
	_, _, cached, err = f.Fetch(ctx, srv.URL+"/a.tar.gz", sha)
	if err != nil || !cached {
		t.Fatalf("Fetch(cached) = %v, %v", cached, err)
	}
	if hits.Load() != 1 {
		t.Errorf("server hits = %d, want 1", hits.Load())
	}
	// A wrong expected digest re-downloads and fails.
	wrong := strings.Repeat("0", 64)
	_, _, _, err = f.Fetch(ctx, srv.URL+"/a.tar.gz", wrong)
	var cerr *ChecksumError
	if !errors.As(err, &cerr) || cerr.Want != wrong || cerr.Got != sha {
		t.Fatalf("Fetch(wrong digest) error = %v", err)
	}
	if !strings.Contains(cerr.Error(), "checksum mismatch for "+srv.URL+"/a.tar.gz") {
		t.Errorf("Error() = %q", cerr.Error())
	}
	// The bytes it hashed to were already in the cache, verified, under
	// their own digest: a bad expectation must not cost that blob.
	if _, err := os.Stat(f.Path(sha)); err != nil {
		t.Error("a mismatch discarded a blob that was already cached and verified")
	}
	entries, _ := os.ReadDir(f.Dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".download-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	// 404.
	_, _, _, err = f.Fetch(ctx, srv.URL+"/missing", "")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("Fetch(404) error = %v", err)
	}
}

func TestFetchCanceled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := New(t.TempDir(), "").Fetch(ctx, srv.URL+"/x", "")
	if err == nil {
		t.Fatal("cancelled fetch succeeded")
	}
}

func TestCheckURL(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"https://github.com/x.tar.gz", "http://127.0.0.1:8080/x", "http://localhost/x", "http://[::1]:1/x"} {
		if err := CheckURL(ok); err != nil {
			t.Errorf("CheckURL(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"http://github.com/x", "http://10.0.0.1/x", "ftp://x/y", "file:///etc/passwd", "://bad"} {
		if err := CheckURL(bad); err == nil {
			t.Errorf("CheckURL(%q) accepted", bad)
		}
	}
	err := CheckURL("http://example.com/a")
	if err == nil || !strings.Contains(err.Error(), "only https is allowed") {
		t.Errorf("error = %v", err)
	}
	// Policy is enforced before any network access.
	_, _, _, err = New(t.TempDir(), "").Fetch(context.Background(), "http://example.com/a", "")
	if err == nil || !strings.Contains(err.Error(), "refusing insecure url") {
		t.Errorf("Fetch(insecure) error = %v", err)
	}
}

func TestSHA256File(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := SHA256File(p)
	if err != nil || got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Errorf("SHA256File() = %s, %v", got, err)
	}
	if _, err := SHA256File(p + ".missing"); err == nil {
		t.Error("missing file hashed")
	}
}

func TestFetchRejectsCorruptCache(t *testing.T) {
	t.Parallel()
	body := []byte("archive bytes")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	f := New(t.TempDir(), "block/test")
	ctx := context.Background()
	path, sha, _, err := f.Fetch(ctx, srv.URL+"/a.tar.gz", "")
	if err != nil {
		t.Fatal(err)
	}
	// A cache restored half-way, or a blob damaged on disk: the name still
	// says sha, the bytes no longer do.
	if err := os.WriteFile(path, []byte("truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, cached, err := f.Fetch(ctx, srv.URL+"/a.tar.gz", sha)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if cached {
		t.Error("a corrupt blob was served from the cache")
	}
	if hits.Load() != 2 {
		t.Errorf("server hits = %d, want a re-download", hits.Load())
	}
	if data, _ := os.ReadFile(got); string(data) != string(body) {
		t.Errorf("cached bytes = %q", data)
	}
	// And a healthy cache is still served without touching the network.
	if _, _, cached, err = f.Fetch(ctx, srv.URL+"/a.tar.gz", sha); err != nil || !cached || hits.Load() != 2 {
		t.Errorf("Fetch(healthy cache) = %v, %v, hits %d", cached, err, hits.Load())
	}
}

func TestFetchRefusesInsecureRedirect(t *testing.T) {
	t.Parallel()
	// A plain-http destination that must never be reached.
	var reached atomic.Bool
	insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		_, _ = w.Write([]byte("payload"))
	}))
	defer insecure.Close()
	// httptest serves on 127.0.0.1, which the transport policy allows so
	// that offline tests can stand in for GitHub; rewrite the redirect to a
	// routable host to model the real downgrade.
	target := strings.Replace(insecure.URL, "127.0.0.1", "mirror.example.com", 1)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer redirector.Close()

	_, _, _, err := New(t.TempDir(), "block/test").Fetch(context.Background(), redirector.URL+"/a.tar.gz", "")
	if err == nil || !strings.Contains(err.Error(), "only https is allowed") {
		t.Fatalf("Fetch() error = %v, want the redirect refused", err)
	}
	if reached.Load() {
		t.Error("the insecure destination was contacted")
	}
}

func TestFetchFollowsAllowedRedirect(t *testing.T) {
	t.Parallel()
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer final.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/real.tar.gz", http.StatusFound)
	}))
	defer redirector.Close()
	path, sha, _, err := New(t.TempDir(), "block/test").Fetch(context.Background(), redirector.URL+"/a.tar.gz", "")
	if err != nil || sha != digest([]byte("payload")) {
		t.Fatalf("Fetch() = %q, %q, %v", path, sha, err)
	}
}

// A download whose bytes are not what the lockfile names is discarded — but
// only when those bytes were not already in the cache. The same digest may be
// another tool's artifact, and content-addressed storage means that blob is
// exactly as valid after the mismatch as before it.
func TestFetchMismatchKeepsBlobsThatWereAlreadyCached(t *testing.T) {
	t.Parallel()
	body := []byte("shared bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	f := New(filepath.Join(t.TempDir(), "cache"), "block/test")
	ctx := context.Background()
	wrong := strings.Repeat("1", 64)

	// Fresh mismatching download: nothing of it may stay.
	_, _, _, err := f.Fetch(ctx, srv.URL+"/x.tar.gz", wrong)
	var cerr *ChecksumError
	if !errors.As(err, &cerr) {
		t.Fatalf("Fetch(wrong) error = %v", err)
	}
	if _, err := os.Stat(f.Path(digest(body))); err == nil {
		t.Error("a fresh mismatching download was left in the cache")
	}

	// Cache the same bytes legitimately, then mismatch again.
	if _, _, _, err := f.Fetch(ctx, srv.URL+"/x.tar.gz", digest(body)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := f.Fetch(ctx, srv.URL+"/y.tar.gz", wrong); !errors.As(err, &cerr) {
		t.Fatalf("Fetch(wrong again) error = %v", err)
	}
	if got, err := os.ReadFile(f.Path(digest(body))); err != nil || string(got) != string(body) {
		t.Errorf("the verified blob was lost to a mismatch: %q, %v", got, err)
	}
	entries, _ := os.ReadDir(f.Dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".download-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestSweepRemovesOnlyStaleDownloads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := New(dir, "block/test")
	stale := filepath.Join(dir, ".download-stale")
	fresh := filepath.Join(dir, ".download-fresh")
	blob := filepath.Join(dir, "sha256", "deadbeef")
	if err := os.MkdirAll(filepath.Dir(blob), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{stale, fresh, blob} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(blob, old, old); err != nil {
		t.Fatal(err)
	}
	f.Sweep(24 * time.Hour)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale download still there: %v", err)
	}
	for _, p := range []string{fresh, blob} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s was swept: %v", p, err)
		}
	}
	// A cache directory that does not exist yet is nothing to sweep.
	New(filepath.Join(dir, "nope"), "block/test").Sweep(0)
}

func TestDownloadReportsAnUnwritableCache(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("a read-only directory is not refused here")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("bytes"))
	}))
	defer srv.Close()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	f := New(dir, "block/test")
	_, _, _, err := f.Fetch(context.Background(), srv.URL+"/a.tar.gz", "")
	if err == nil {
		t.Fatal("fetch into a read-only cache succeeded")
	}
	if diag.Of(err) != diag.StoreUnwritable {
		t.Fatalf("err = %v, want %s", err, diag.StoreUnwritable)
	}
}

// A server that redirects forever is given up on after ten hops, and the
// failure says so rather than spinning until the client's timeout.
func TestFetchGivesUpOnARedirectLoop(t *testing.T) {
	t.Parallel()
	var hops atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	defer srv.Close()
	f := New(t.TempDir(), "block/test")
	_, _, _, err := f.Fetch(context.Background(), srv.URL+"/a.tar.gz", "")
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("err = %v", err)
	}
	if diag.Of(err) != diag.DownloadFailed {
		t.Fatalf("err = %v, want %s", err, diag.DownloadFailed)
	}
	if n := hops.Load(); n > 11 {
		t.Fatalf("followed %d hops", n)
	}
}

func TestCredentialAllows(t *testing.T) {
	t.Parallel()
	c := Credential{Host: "api.github.com", Token: "tok"}
	tests := []struct {
		name string
		cred Credential
		url  string
		want bool
	}{
		{"the credential's host", c, "https://api.github.com/repos/o/r/releases/assets/1", true},
		{"the host in another case", c, "https://API.GitHub.com/repos/o/r/releases/assets/1", true},
		{"the release CDN", c, "https://objects.githubusercontent.com/x?X-Amz-Signature=y", false},
		{"the web host", c, "https://github.com/o/r/releases/download/v1/a.tar.gz", false},
		{"a subdomain", c, "https://evil.api.github.com/", false},
		{"the host on another port", c, "https://api.github.com:8443/", false},
		{"the host with the port it was given", Credential{Host: "127.0.0.1:8080", Token: "t"}, "http://127.0.0.1:8080/repos/o/r", true},
		{"no token", Credential{Host: "api.github.com"}, "https://api.github.com/", false},
		{"no host", Credential{Token: "tok"}, "https://api.github.com/", false},
		{"not a url", c, "://", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cred.Allows(tt.url); got != tt.want {
				t.Errorf("Allows(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// A private release asset is had from the API with the token — and from
// the signed URL the API redirects to without it. The objects host refuses a
// request that carries both, so the token leaking onto the hop is not a
// theoretical concern: it is the download failing.
func TestFetchSendsTheTokenToItsHostAndDropsItOnTheHop(t *testing.T) {
	t.Parallel()
	payload := []byte("private bytes")
	objects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "Only one auth mechanism allowed", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer objects.Close()
	var seen atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("Accept") != "application/octet-stream" {
			t.Errorf("the API was asked without the token or for the wrong type: %v", r.Header)
		}
		// Two httptest servers are two ports on one loopback address, so
		// the hop is to a different host the way a CDN is.
		http.Redirect(w, r, objects.URL+"/signed?sig=1", http.StatusFound)
	}))
	defer api.Close()
	f := New(t.TempDir(), "block/test")
	f.Credential = Credential{Host: strings.TrimPrefix(api.URL, "http://"), Token: "tok"}
	_, sha, _, err := f.Fetch(context.Background(), api.URL+"/repos/o/r/releases/assets/1", "")
	if err != nil || sha != digest(payload) {
		t.Fatalf("Fetch() = %q, %v, want the asset through the redirect", sha, err)
	}
	if seen.Load() != 1 {
		t.Errorf("the API was asked %d times", seen.Load())
	}
}

// Whatever host the artifact is on, the token goes to the credential's host
// and nowhere else — not to a vendor download server, and not to GitHub's
// own web host either.
func TestFetchDoesNotSendTheTokenElsewhere(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("a host the credential is not for was sent %q", auth)
		}
		_, _ = w.Write([]byte("public"))
	}))
	defer srv.Close()
	f := New(t.TempDir(), "block/test")
	f.Credential = Credential{Host: "api.github.com", Token: "tok"}
	if _, _, _, err := f.Fetch(context.Background(), srv.URL+"/a.tar.gz", ""); err != nil {
		t.Fatal(err)
	}
}

// An artifact that announces a size past the limit is refused before a byte
// of it is read, and one that announces nothing is stopped at the limit
// while it is being read. Either way nothing is kept: not in the cache, and
// not as a temporary file beside it.
func TestFetchRefusesADownloadLargerThanItTransfers(t *testing.T) {
	t.Parallel()
	nothingKept := func(t *testing.T, dir string) {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), downloadPrefix) {
				t.Errorf("%s was left behind", e.Name())
			}
		}
		blobs, _ := os.ReadDir(filepath.Join(dir, "sha256"))
		if len(blobs) != 0 {
			t.Errorf("the cache holds %d blobs from a refused download", len(blobs))
		}
	}
	t.Run("announced by Content-Length", func(t *testing.T) {
		t.Parallel()
		var body atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "3221225472")
			w.WriteHeader(http.StatusOK)
			body.Add(1)
		}))
		defer srv.Close()
		f := New(t.TempDir(), "block/test")
		_, _, _, err := f.Fetch(context.Background(), srv.URL+"/huge.tar.gz", "")
		if err == nil || diag.Of(err) != diag.DownloadTooLarge || !strings.Contains(err.Error(), "larger than the") {
			t.Fatalf("Fetch() error = %v (%v), want BLK3004", err, diag.Of(err))
		}
		nothingKept(t, f.Dir)
	})
	t.Run("unannounced", func(t *testing.T) {
		t.Parallel()
		const limit = 1024
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			// Flushing first means no Content-Length: the body arrives in
			// chunks and its size is known only when it ends — or does not.
			_ = http.NewResponseController(w).Flush()
			chunk := []byte(strings.Repeat("x", limit))
			for range 4 {
				if _, err := w.Write(chunk); err != nil {
					return
				}
				_ = http.NewResponseController(w).Flush()
			}
		}))
		defer srv.Close()
		f := New(t.TempDir(), "block/test")
		f.MaxBytes = limit
		_, _, _, err := f.Fetch(context.Background(), srv.URL+"/endless.tar.gz", "")
		if err == nil || diag.Of(err) != diag.DownloadTooLarge {
			t.Fatalf("Fetch() error = %v (%v), want BLK3004", err, diag.Of(err))
		}
		nothingKept(t, f.Dir)
	})
	t.Run("exactly the limit is not too large", func(t *testing.T) {
		t.Parallel()
		const limit = 1024
		payload := []byte(strings.Repeat("y", limit))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = http.NewResponseController(w).Flush()
			_, _ = w.Write(payload)
		}))
		defer srv.Close()
		f := New(t.TempDir(), "block/test")
		f.MaxBytes = limit
		_, sha, _, err := f.Fetch(context.Background(), srv.URL+"/exact.tar.gz", "")
		if err != nil || sha != digest(payload) {
			t.Fatalf("Fetch() = %q, %v", sha, err)
		}
	})
}

func TestCheckSize(t *testing.T) {
	t.Parallel()
	f := New(t.TempDir(), "block/test")
	if err := f.CheckSize("https://example.com/a", MaxBytes); err != nil {
		t.Errorf("CheckSize(at the limit) = %v", err)
	}
	if err := f.CheckSize("https://example.com/a", 0); err != nil {
		t.Errorf("CheckSize(unknown) = %v", err)
	}
	err := f.CheckSize("https://example.com/a", MaxBytes+1)
	if err == nil || diag.Of(err) != diag.DownloadTooLarge || !strings.Contains(err.Error(), "https://example.com/a") {
		t.Errorf("CheckSize(past the limit) = %v (%v)", err, diag.Of(err))
	}
}
