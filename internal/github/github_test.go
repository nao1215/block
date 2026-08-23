package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client(), UserAgent: "block/test", Token: "tok"}
}

func TestTagsPaginates(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("User-Agent") != "block/test" {
			t.Errorf("headers = %v", r.Header)
		}
		if !strings.HasPrefix(r.URL.Path, "/repos/o/r/git/matching-refs/tags/v") {
			t.Errorf("path = %s", r.URL.Path)
		}
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "1":
			var refs []string
			for i := range perPage {
				refs = append(refs, fmt.Sprintf(`{"ref":"refs/tags/v1.0.%d"}`, i))
			}
			fmt.Fprintf(w, "[%s]", strings.Join(refs, ","))
		case "2":
			fmt.Fprint(w, `[{"ref":"refs/tags/v2.0.0"},{"ref":"refs/heads/ignored"}]`)
		default:
			t.Errorf("unexpected page %s", page)
		}
	})
	tags, err := c.Tags(context.Background(), "o/r", "v")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != perPage+1 || tags[len(tags)-1] != "v2.0.0" {
		t.Errorf("Tags() = %d tags, last %q", len(tags), tags[len(tags)-1])
	}
}

// GitHub's matching-refs endpoint answers some repositories with the whole
// ref list whatever ?page says. Paging that blindly repeated every tag once
// per page, and a caller walking the newest tags then spent all its release
// lookups on the same one.
func TestTagsIgnoresAPageThatRepeatsItself(t *testing.T) {
	t.Parallel()
	pages := 0
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		pages++
		w.Header().Set("Content-Type", "application/json")
		var refs []string
		for i := range perPage {
			refs = append(refs, fmt.Sprintf(`{"ref":"refs/tags/v1.0.%d"}`, i))
		}
		fmt.Fprintf(w, "[%s]", strings.Join(refs, ","))
	})
	tags, err := c.Tags(context.Background(), "o/r", "v")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != perPage {
		t.Errorf("Tags() = %d tags, want %d with no repeats", len(tags), perPage)
	}
	if pages != 2 {
		t.Errorf("requested %d pages, want 2: one to read them and one to notice the repeat", pages)
	}
}

func TestTagsNotFound(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	_, err := c.Tags(context.Background(), "o/missing", "v")
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "repository o/missing") {
		t.Errorf("error = %v", err)
	}
}

func TestReleaseByTag(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/tags/v1.0.0":
			fmt.Fprint(w, `{"tag_name":"v1.0.0","draft":false,"prerelease":true,"assets":[{"name":"a.tar.gz","browser_download_url":"https://dl/a.tar.gz","size":3}]}`)
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	})
	rel, err := c.ReleaseByTag(context.Background(), "o/r", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if rel.TagName != "v1.0.0" || !rel.Prerelease || len(rel.Assets) != 1 {
		t.Errorf("release = %+v", rel)
	}
	named := rel.AssetsNamed("a.tar.gz")
	if len(named) != 1 || named[0].BrowserDownloadURL != "https://dl/a.tar.gz" || named[0].Size != 3 {
		t.Errorf("AssetsNamed() = %+v", named)
	}
	if got := rel.AssetsNamed("b.tar.gz"); len(got) != 0 {
		t.Errorf("AssetsNamed() found %+v for a name the release does not carry", got)
	}
	_, err = c.ReleaseByTag(context.Background(), "o/r", "v9.9.9")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v", err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"rate limit", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1700000000")
			http.Error(w, "{}", http.StatusForbidden)
		}, "rate limit exceeded (set GITHUB_TOKEN to raise the limit); resets at 2023-11-14T22:13:20Z"},
		{"rate limit without reset", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, "{}", http.StatusTooManyRequests)
		}, "rate limit exceeded (set GITHUB_TOKEN to raise the limit)"},
		{"forbidden", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "{}", http.StatusForbidden)
		}, "returned 403 Forbidden"},
		{"server error", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "{}", http.StatusBadGateway)
		}, "returned 502 Bad Gateway"},
		{"bad json", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "not json")
		}, "invalid response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newTestClient(t, tt.handler)
			_, err := c.ReleaseByTag(context.Background(), "o/r", "v1.0.0")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestConnectionError(t *testing.T) {
	t.Parallel()
	c := &Client{BaseURL: "http://127.0.0.1:1", HTTP: http.DefaultClient}
	_, err := c.Tags(context.Background(), "o/r", "v")
	if err == nil || !strings.Contains(err.Error(), "github api:") {
		t.Errorf("error = %v", err)
	}
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv(EnvBaseURL, "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	c := NewFromEnv("ua")
	if c.BaseURL != DefaultBaseURL || c.Token != "" || c.UserAgent != "ua" {
		t.Errorf("NewFromEnv() = %+v", c)
	}
	t.Setenv(EnvBaseURL, "http://127.0.0.1:9/api/")
	t.Setenv("GH_TOKEN", "gh")
	c = NewFromEnv("ua")
	if c.BaseURL != "http://127.0.0.1:9/api" || c.Token != "gh" {
		t.Errorf("NewFromEnv() = %+v", c)
	}
	t.Setenv("GITHUB_TOKEN", "primary")
	if NewFromEnv("ua").Token != "primary" {
		t.Error("GITHUB_TOKEN should win over GH_TOKEN")
	}
}

// GitHub's digest is what a lockfile records without downloading, so it has
// to be something the lockfile will read back: 64 lower-case hex characters.
// Anything else is treated as no digest, and the artifact gets hashed after a
// download instead of a value block cannot verify being written to disk.
func TestAssetSHA256AcceptsOnlyAHexDigest(t *testing.T) {
	t.Parallel()
	hex := strings.Repeat("ab", 32)
	tests := []struct {
		digest string
		want   string
	}{
		{"sha256:" + hex, hex},
		{"sha256:" + strings.ToUpper(hex), hex},
		{"", ""},
		{hex, ""},
		{"sha256:", ""},
		{"sha256:" + hex[:63], ""},
		{"sha256:" + hex + "0", ""},
		{"sha256:" + strings.Repeat("zz", 32), ""},
		{"sha512:" + hex, ""},
	}
	for _, tt := range tests {
		if got := (Asset{Digest: tt.digest}).SHA256(); got != tt.want {
			t.Errorf("Asset{Digest: %q}.SHA256() = %q, want %q", tt.digest, got, tt.want)
		}
	}
}

// The matching-refs endpoint answers a repository with many tags by returning
// every one of them in a single response, whatever per_page says — sui has
// nine thousand, in nearly four megabytes. block used to ask for a second
// page anyway, learn nothing from it, and pay a request and another copy of
// that list for every resolution. A response longer than the page asked for
// is the endpoint saying there is no second page.
func TestTagsAsksOnceWhenTheEndpointIgnoresPaging(t *testing.T) {
	t.Parallel()
	const total = perPage*2 + 37
	requests := 0
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests++
		refs := make([]string, 0, total)
		for i := range total {
			refs = append(refs, fmt.Sprintf(`{"ref":"refs/tags/v1.0.%d"}`, i))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", strings.Join(refs, ","))
	})
	tags, err := c.Tags(context.Background(), "o/r", "v")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != total {
		t.Errorf("Tags() = %d tags, want %d", len(tags), total)
	}
	if requests != 1 {
		t.Errorf("Tags() made %d requests, want 1: the first response already held every ref", requests)
	}
}

// A body block declines to read whole is block's own limit, not a broken API.
// Reported as "invalid response", it sent a reader to look at GitHub for a
// cut block made itself.
func TestGetJSONNamesItsOwnSizeLimit(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("["))
		ref := `{"ref":"refs/tags/v1.0.0"},`
		const chunkRefs = 4096
		chunk := []byte(strings.Repeat(ref, chunkRefs))
		for written := 0; written <= maxBodyBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
		_, _ = w.Write([]byte(`{"ref":"refs/tags/v1.0.1"}]`))
	})
	_, err := c.Tags(context.Background(), "o/r", "v")
	if err == nil {
		t.Fatal("Tags() accepted a response larger than block reads")
	}
	if !strings.Contains(err.Error(), "larger than the") {
		t.Errorf("error = %q, want it to name block's own size limit", err)
	}
	if strings.Contains(err.Error(), "invalid response") {
		t.Errorf("error = %q, which blames the API for block's own cut", err)
	}
}

// Whether a repository is private is asked of the API only when there is a
// token to ask with: without one the API shows public repositories only, so
// a repository that resolved at all is public, and a request would be spent
// on a question already answered.
func TestPrivateAsksOnlyWithAToken(t *testing.T) {
	t.Parallel()
	t.Run("no token, no request", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
			t.Errorf("the API was asked %s without a token", r.URL)
		})
		c.Token = ""
		private, err := c.Private(context.Background(), "o/r")
		if err != nil || private {
			t.Errorf("Private() = %v, %v, want public without a request", private, err)
		}
	})
	t.Run("a private repository", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/repos/o/r" || r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("asked %s with %v", r.URL, r.Header)
			}
			_, _ = w.Write([]byte(`{"full_name":"o/r","private":true}`))
		})
		private, err := c.Private(context.Background(), "o/r")
		if err != nil || !private {
			t.Errorf("Private() = %v, %v", private, err)
		}
	})
	t.Run("a public repository", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"full_name":"o/r","private":false}`))
		})
		private, err := c.Private(context.Background(), "o/r")
		if err != nil || private {
			t.Errorf("Private() = %v, %v", private, err)
		}
	})
	t.Run("a repository the token cannot see", func(t *testing.T) {
		t.Parallel()
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		})
		_, err := c.Private(context.Background(), "o/r")
		if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "repository o/r") {
			t.Errorf("Private() error = %v, want not found naming the repository", err)
		}
	})
}

func TestHost(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		DefaultBaseURL:                   "api.github.com",
		"https://ghe.example.com/api/v3": "ghe.example.com",
		"http://127.0.0.1:8080/t1":       "127.0.0.1:8080",
		"://":                            "",
	}
	for base, want := range tests {
		if got := (&Client{BaseURL: base}).Host(); got != want {
			t.Errorf("Host(%q) = %q, want %q", base, got, want)
		}
	}
}

// The asset's API URL is what a private asset is downloaded from, so it has
// to survive the trip through the release JSON.
func TestReleaseByTagKeepsTheAssetAPIURL(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.0.0","assets":[{"name":"a.tar.gz","url":"https://api.example.com/repos/o/r/releases/assets/7","browser_download_url":"https://example.com/o/r/releases/download/v1.0.0/a.tar.gz","size":42}]}`))
	})
	rel, err := c.ReleaseByTag(context.Background(), "o/r", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	got := rel.Assets[0]
	if got.URL != "https://api.example.com/repos/o/r/releases/assets/7" || got.Size != 42 {
		t.Errorf("asset = %+v", got)
	}
}
