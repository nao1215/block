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
	// The mismatching download must not be left under its digest either.
	if _, err := os.Stat(f.Path(sha)); err == nil {
		t.Error("mismatching blob left in cache")
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
		t.Fatal("canceled fetch succeeded")
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
