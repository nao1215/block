// Package fetch downloads artifacts into a content-addressed cache and
// verifies their SHA-256 digests. A blob is stored under its own digest, so
// the same artifact pulled by two projects lands in one file. The name alone
// is not taken as proof: a cache hit is re-hashed before it is used, because
// a restored CI cache can be truncated or corrupt.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/nao1215/block/internal/diag"
)

// ChecksumError reports a digest mismatch between lockfile and download.
type ChecksumError struct {
	URL  string
	Want string
	Got  string
}

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("checksum mismatch for %s: want sha256 %s, got %s", e.URL, e.Want, e.Got)
}

// Code names the diagnostic this refusal is published under.
func (e *ChecksumError) Code() diag.Code { return diag.ChecksumMismatch }

// Fetcher downloads into Dir.
type Fetcher struct {
	Dir       string
	HTTP      *http.Client
	UserAgent string
}

// New returns a Fetcher storing blobs under dir. Its client enforces the
// transport policy on every redirect hop, so an https artifact cannot be
// answered by a plain-http location.
func New(dir, userAgent string) *Fetcher {
	const timeout = 10 * time.Minute
	return &Fetcher{
		Dir: dir,
		HTTP: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				const maxRedirects = 10
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return CheckURL(req.URL.String())
			},
		},
		UserAgent: userAgent,
	}
}

// Path returns where a blob with the given digest lives in the cache.
func (f *Fetcher) Path(sha string) string {
	return filepath.Join(f.Dir, "sha256", sha)
}

// Fetch returns the cached path of rawURL's content. When want is non-empty
// the cache is consulted first — and re-hashed, so a corrupt blob is discarded
// rather than used — and the download must hash to want; when want is empty
// the artifact is downloaded and its digest reported. Fetch returns the blob
// path, its digest and whether the cache served it.
func (f *Fetcher) Fetch(ctx context.Context, rawURL, want string) (path, sha string, cached bool, err error) {
	if want != "" {
		ok, err := f.verifyCached(want)
		if err != nil {
			return "", "", false, err
		}
		if ok {
			return f.Path(want), want, true, nil
		}
	}
	if err := CheckURL(rawURL); err != nil {
		return "", "", false, err
	}
	sha, fresh, err := f.download(ctx, rawURL)
	if err != nil {
		return "", "", false, err
	}
	if want != "" && sha != want {
		// The mismatching bytes are removed from the cache, and every blob
		// that was already there is left exactly as it was: a bad download
		// must not cost an artifact that was already verified — neither the
		// one the lockfile names nor another tool's, which is what the
		// download hashed to when fresh is false.
		if fresh {
			_ = os.Remove(f.Path(sha))
		}
		return "", "", false, &ChecksumError{URL: rawURL, Want: want, Got: sha}
	}
	return f.Path(sha), sha, false, nil
}

// verifyCached reports whether the cache holds a blob that really hashes to
// want. A blob whose bytes do not match its name is deleted: it is either a
// truncated download or a damaged cache restore, and either way the artifact
// must be fetched again.
func (f *Fetcher) verifyCached(want string) (bool, error) {
	path := f.Path(want)
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return false, nil //nolint:nilerr // a missing blob is a normal cache miss
	}
	got, err := SHA256File(path)
	if err != nil {
		return false, err
	}
	if got == want {
		return true, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, diag.CacheUnusable.Errorf("cached artifact %s is corrupt and could not be removed: %w", path, err)
	}
	return false, nil
}

// download fetches rawURL into the cache under its digest and reports that
// digest, plus whether the blob is new to the cache: a download whose bytes
// were already cached under the same digest replaces nothing.
func (f *Fetcher) download(ctx context.Context, rawURL string) (sha string, fresh bool, err error) {
	const dirMode = 0o755
	if err := os.MkdirAll(filepath.Join(f.Dir, "sha256"), dirMode); err != nil {
		return "", false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", false, err
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		// Wrap rather than Errorf: a redirect this client refused already
		// carries the code for why, and "the download failed" is the less
		// useful of the two answers.
		return "", false, diag.DownloadFailed.Wrap(fmt.Errorf("download %s: %w", rawURL, err))
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode != http.StatusOK {
		return "", false, diag.DownloadFailed.Errorf("download %s: %s", rawURL, resp.Status)
	}
	tmp, err := os.CreateTemp(f.Dir, ".download-*")
	if err != nil {
		return "", false, err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		cleanup()
		return "", false, diag.DownloadFailed.Errorf("download %s: %w", rawURL, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", false, err
	}
	sha = hex.EncodeToString(h.Sum(nil))
	// The same bytes may already be cached under this digest — the artifact
	// of another tool, or a previous download of this one. Content-addressed
	// means they are the same file, so the existing one stays and the copy
	// just made is discarded.
	if st, err := os.Stat(f.Path(sha)); err == nil && st.Mode().IsRegular() {
		_ = os.Remove(tmpName)
		return sha, false, nil
	}
	if err := os.Rename(tmpName, f.Path(sha)); err != nil {
		_ = os.Remove(tmpName)
		return "", false, err
	}
	return sha, true, nil
}

// CheckURL enforces block's transport policy: HTTPS everywhere, with plain
// HTTP tolerated only for loopback addresses so test servers can stand in
// for GitHub.
func CheckURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return diag.InsecureURL.Errorf("invalid url %q: %w", rawURL, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopback(u.Hostname()) {
			return nil
		}
		return diag.InsecureURL.Errorf("refusing insecure url %s: only https is allowed", rawURL)
	default:
		return diag.InsecureURL.Errorf("unsupported url scheme in %s: only https is allowed", rawURL)
	}
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// SHA256File hashes a file on disk.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // caller-controlled cache path
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
