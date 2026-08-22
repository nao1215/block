// Package github is the minimal GitHub REST client block needs: list the tags
// of a repository and fetch one release by tag. It deliberately avoids
// paging through /releases, which for projects that publish nightlies (Foundry
// has hundreds) would cost many requests per resolution.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nao1215/block/internal/diag"
)

// DefaultBaseURL is the public GitHub API.
const DefaultBaseURL = "https://api.github.com"

// EnvBaseURL overrides the API base URL (used by tests and GitHub Enterprise).
const EnvBaseURL = "BLOCK_GITHUB_API_URL"

const (
	perPage      = 100
	maxTagPages  = 20
	maxBodyBytes = 8 << 20
)

// ErrNotFound reports a missing repository, tag or release.
var ErrNotFound = diag.UpstreamNotFound.Wrap(errors.New("not found"))

// Client talks to the GitHub REST API.
type Client struct {
	BaseURL   string
	Token     string
	HTTP      *http.Client
	UserAgent string
}

// NewFromEnv builds a client from BLOCK_GITHUB_API_URL and GITHUB_TOKEN/GH_TOKEN.
func NewFromEnv(userAgent string) *Client {
	base := os.Getenv(EnvBaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	const timeout = 60 * time.Second
	return &Client{BaseURL: strings.TrimRight(base, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}, UserAgent: userAgent}
}

// Release is the subset of a GitHub release block consumes.
type Release struct {
	TagName    string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Asset is one downloadable release file.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	// Digest is GitHub's own checksum of the upload, "sha256:<hex>", or ""
	// for assets uploaded before GitHub started recording it.
	Digest string `json:"digest"`
}

// SHA256 returns the lower-case hex digest when GitHub recorded a sha256 for
// the asset, and "" otherwise — including when the field is present but is
// not a digest block could write into a lockfile and read back. A value that
// is not 64 hex characters is treated as absent rather than trusted, so that
// the caller downloads and hashes the artifact itself.
func (a Asset) SHA256() string {
	hex, ok := strings.CutPrefix(a.Digest, "sha256:")
	if !ok {
		return ""
	}
	hex = strings.ToLower(hex)
	if !isHexDigest(hex) {
		return ""
	}
	return hex
}

// isHexDigest reports whether s is a sha256 written as 64 lower-case hex
// characters, which is the only spelling a lockfile accepts.
func isHexDigest(s string) bool {
	const hexLen = 64
	if len(s) != hexLen {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// AssetsNamed returns every asset of the release with this file name.
//
// It returns a list rather than "the" asset because a release carrying two
// files of one name is not something to resolve by taking the first: the two
// are different downloads, and which one a lockfile pinned would depend on the
// order the API happened to answer in. The caller refuses that instead.
func (r *Release) AssetsNamed(name string) []Asset {
	var out []Asset
	for _, a := range r.Assets {
		if a.Name == name {
			out = append(out, a)
		}
	}
	return out
}

type ref struct {
	Ref string `json:"ref"`
}

// Tags lists the repository's tag names starting with prefix, once each, in
// the order GitHub returns them (lexically ordered refs).
//
// The matching-refs endpoint does not always honour ?page: for some
// repositories it answers every page with the same full list, so paging
// blindly would repeat each tag as many times as there are pages — and the
// caller would then spend its release lookups on the same tag over and over.
// A page that contributes no new tag therefore ends the walk.
func (c *Client) Tags(ctx context.Context, repo, prefix string) ([]string, error) {
	var tags []string
	seen := map[string]bool{}
	for page := 1; page <= maxTagPages; page++ {
		endpoint := fmt.Sprintf("%s/repos/%s/git/matching-refs/tags/%s?per_page=%d&page=%d", c.BaseURL, repo, url.PathEscape(prefix), perPage, page)
		var refs []ref
		if err := c.getJSON(ctx, endpoint, &refs); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("repository %s: %w", repo, ErrNotFound)
			}
			return nil, err
		}
		added := 0
		for _, r := range refs {
			name, ok := strings.CutPrefix(r.Ref, "refs/tags/")
			if !ok || seen[name] {
				continue
			}
			seen[name] = true
			tags = append(tags, name)
			added++
		}
		// A page shorter than the one asked for is the last one. A page
		// *longer* than the one asked for is the endpoint ignoring per_page
		// and handing back every matching ref at once, which is what it does
		// for the repositories that have many: there is no second page to
		// ask for, and asking anyway costs one wasted request per resolution
		// — a second copy of a list that can run to several megabytes.
		if added == 0 || len(refs) != perPage {
			break
		}
	}
	return tags, nil
}

// ReleaseByTag fetches the release published for tag. A tag without a
// release yields ErrNotFound.
func (c *Client) ReleaseByTag(ctx context.Context, repo, tag string) (*Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/releases/tags/%s", c.BaseURL, repo, url.PathEscape(tag))
	var rel Release
	if err := c.getJSON(ctx, endpoint, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

type commit struct {
	SHA string `json:"sha"`
}

// Commit returns the full SHA of the commit a tag (or any ref) points at.
// GitHub dereferences annotated tags for this endpoint.
func (c *Client) Commit(ctx context.Context, repo, ref string) (string, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/commits/%s", c.BaseURL, repo, url.PathEscape(ref))
	var cm commit
	if err := c.getJSON(ctx, endpoint, &cm); err != nil {
		return "", err
	}
	if cm.SHA == "" {
		return "", diag.UpstreamError.Errorf("github api: no commit for %s@%s", repo, ref)
	}
	return cm.SHA, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return diag.UpstreamError.Errorf("github api: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	// One byte past the limit, so that a response block refused to read whole
	// is told apart from one that ended there. Without that, a truncated body
	// reached the decoder and was reported as "invalid response from the API"
	// — which blamed GitHub for a cut block made itself.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return diag.UpstreamError.Errorf("github api: %w", err)
	}
	if len(body) > maxBodyBytes {
		return diag.UpstreamError.Errorf("github api: the response from %s is larger than the %d bytes block reads", endpoint, maxBodyBytes)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return rateLimitError(resp.Header.Get("X-RateLimit-Reset"))
		}
		return diag.UpstreamError.Errorf("github api: %s returned %s", endpoint, resp.Status)
	case resp.StatusCode/100 != 2: //nolint:mnd // HTTP status class
		return diag.UpstreamError.Errorf("github api: %s returned %s", endpoint, resp.Status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return diag.UpstreamError.Errorf("github api: invalid response from %s: %w", endpoint, err)
	}
	return nil
}

func rateLimitError(reset string) error {
	msg := "github api: rate limit exceeded (set GITHUB_TOKEN to raise the limit)"
	if secs, err := strconv.ParseInt(reset, 10, 64); err == nil {
		msg += "; resets at " + time.Unix(secs, 0).UTC().Format(time.RFC3339)
	}
	return diag.RateLimited.Errorf("%s", msg)
}
