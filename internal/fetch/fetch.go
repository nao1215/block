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
	"strings"
	"time"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/fserr"
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

// MaxBytes is the most one download may hold. A transfer is stopped at this
// size whatever the upstream announced, so a response with no Content-Length
// — or a lying one — cannot fill the disk through the cache. The bound is far
// above any real toolchain artifact: the largest in the registry is a few
// hundred megabytes.
const MaxBytes = 2 << 30

// Credential is a bearer token and the one host it may be sent to. The host
// is the GitHub API's, which already saw the token when the release was
// resolved; a download anywhere else — a release CDN, a vendor server, a
// mirror a redirect points at — is made without it.
type Credential struct {
	Host  string
	Token string
}

// Allows reports whether rawURL is one the token may be sent to: the
// credential's own host, compared whole (name and port) and without regard
// to case. An empty credential allows nothing.
func (c Credential) Allows(rawURL string) bool {
	if c.Token == "" || c.Host == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, c.Host)
}

// Fetcher downloads into Dir.
type Fetcher struct {
	Dir       string
	HTTP      *http.Client
	UserAgent string
	// Credential is sent with a request to its host and to nothing else.
	Credential Credential
	// MaxBytes caps one download; New sets it to [MaxBytes].
	MaxBytes int64
}

// New returns a Fetcher storing blobs under dir. Its client enforces the
// transport policy on every redirect hop, so an https artifact cannot be
// answered by a plain-http location, and it drops the credential from a hop
// that leaves the credential's host, so a release served through a signed
// CDN URL never sees the token.
func New(dir, userAgent string) *Fetcher {
	const timeout = 10 * time.Minute
	f := &Fetcher{Dir: dir, UserAgent: userAgent, MaxBytes: MaxBytes}
	f.HTTP = &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			const maxRedirects = 10
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if err := CheckURL(req.URL.String()); err != nil {
				return err
			}
			// The client copies the headers of the first request onto each
			// hop. The token is among them, and it is for one host only.
			if !f.Credential.Allows(req.URL.String()) {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
	return f
}

// CheckSize refuses an artifact that is known, before any transfer, to be
// larger than the Fetcher will download — the size an upstream's API reports
// for it.
func (f *Fetcher) CheckSize(rawURL string, size int64) error {
	if size > f.MaxBytes {
		return f.tooLarge(rawURL)
	}
	return nil
}

func (f *Fetcher) tooLarge(rawURL string) error {
	return diag.DownloadTooLarge.Errorf("refusing to download %s: it is larger than the %d bytes block transfers", rawURL, f.MaxBytes)
}

// cacheErr names a failure to write the download cache. A filesystem with no
// room left is its own diagnostic: "the store could not be written" would send
// the reader to check the permissions of a directory whose permissions are
// fine, and an artifact is written twice on its way in — once as a temporary
// file, once under its digest — so the cache is where a tight disk shows first.
func (f *Fetcher) cacheErr(err error) error {
	if fserr.OutOfSpace(err) {
		return diag.DiskFull.Errorf("cache %s: there is no room left on the disk, or a quota is exhausted: %w", f.Dir, err)
	}
	return diag.StoreUnwritable.Errorf("cache %s: %w", f.Dir, err)
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
	sha, err = f.download(ctx, rawURL, want)
	if err != nil {
		return "", "", false, err
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
// digest. With want set, bytes that hash to anything else never reach the
// cache: the mismatch is decided on the temporary file, so nothing is
// published and then withdrawn, and a blob another fetch is verifying at the
// same moment cannot disappear under it. A download whose bytes are already
// cached under the same digest replaces nothing.
func (f *Fetcher) download(ctx context.Context, rawURL, want string) (string, error) {
	const dirMode = 0o755
	if err := os.MkdirAll(filepath.Join(f.Dir, "sha256"), dirMode); err != nil {
		return "", f.cacheErr(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	if f.Credential.Allows(rawURL) {
		// A private release asset is served by the API, which hands over
		// the bytes — or a signed redirect to them — only when asked for
		// the binary rather than the JSON that describes it.
		req.Header.Set("Authorization", "Bearer "+f.Credential.Token)
		req.Header.Set("Accept", "application/octet-stream")
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		// Wrap rather than Errorf: a redirect this client refused already
		// carries the code for why, and "the download failed" is the less
		// useful of the two answers.
		return "", diag.DownloadFailed.Wrap(fmt.Errorf("download %s: %w", rawURL, err))
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode != http.StatusOK {
		return "", diag.DownloadFailed.Errorf("download %s: %s", rawURL, resp.Status)
	}
	// What the response announces is checked first, so an artifact that says
	// how big it is costs nothing to refuse. What it announces is not
	// trusted, though: the copy below stops at the same bound whether or
	// not a Content-Length was given, and whatever it said.
	if resp.ContentLength > f.MaxBytes {
		return "", f.tooLarge(rawURL)
	}
	tmp, err := os.CreateTemp(f.Dir, downloadPrefix+"*")
	if err != nil {
		return "", f.cacheErr(err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = tmp.Close(); _ = os.Remove(tmpName) }
	h := sha256.New()
	// One byte past the limit, so a body that ends exactly at it is told
	// apart from one that was cut there.
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, f.MaxBytes+1))
	if err != nil {
		cleanup()
		// A copy has two ends, and only one of them is the network: bytes
		// that arrived and had nowhere to go are a full disk, not a failed
		// transfer, and saying "the download failed" would send the reader
		// to look at their proxy.
		if fserr.OutOfSpace(err) {
			return "", f.cacheErr(err)
		}
		return "", diag.DownloadFailed.Errorf("download %s: %w", rawURL, err)
	}
	if n > f.MaxBytes {
		cleanup()
		return "", f.tooLarge(rawURL)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", f.cacheErr(err)
	}
	sha := hex.EncodeToString(h.Sum(nil))
	if want != "" && sha != want {
		// Decided here, on the temporary file, so the cache is left exactly
		// as it was: the blob the lockfile names, and any other tool's blob
		// these bytes happen to equal, are untouched.
		_ = os.Remove(tmpName)
		return "", &ChecksumError{URL: rawURL, Want: want, Got: sha}
	}
	// The same bytes may already be cached under this digest — the artifact
	// of another tool, or a previous download of this one. Content-addressed
	// means they are the same file, so the existing one stays and the copy
	// just made is discarded.
	if st, err := os.Stat(f.Path(sha)); err == nil && st.Mode().IsRegular() {
		_ = os.Remove(tmpName)
		return sha, nil
	}
	if err := os.Rename(tmpName, f.Path(sha)); err != nil {
		_ = os.Remove(tmpName)
		return "", f.cacheErr(err)
	}
	return sha, nil
}

// downloadPrefix names a download in progress. It lives in the cache
// directory itself, beside sha256/, so nothing under sha256/ is ever a
// partial blob.
const downloadPrefix = ".download-"

// Sweep removes downloads that were interrupted — a block killed mid-transfer
// leaves its temporary file behind, and nothing else ever looks at it — once
// they are older than olderThan. A download still in progress is younger than
// that: its file is written to until it is renamed into place, so a sweep
// running beside another fetch never takes anything from it. Errors are not
// reported: a sweep is housekeeping, and the fetch that follows says what is
// wrong with the cache if something is.
func (f *Fetcher) Sweep(olderThan time.Duration) {
	entries, err := os.ReadDir(f.Dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), downloadPrefix) || e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(f.Dir, e.Name()))
	}
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
