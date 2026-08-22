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
	"strings"
	"sync/atomic"
	"testing"
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
