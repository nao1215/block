//go:build live

// Live registry validation. This is the check that keeps recipes honest as
// upstreams move: it takes each recipe, finds the newest stable release the
// way block does, resolves the artifact for every platform the recipe claims,
// and — for the platform it runs on — downloads it, verifies the checksum,
// unpacks it, and runs the executables the recipe promises.
//
// It only ever fully exercises the platform it is running on, because running
// a binary is the point and a Linux runner cannot run a macOS build. The
// Registry (live) workflow therefore runs this on one runner per platform
// block supports, so every platform in the registry is covered by a real
// download, a real unpack and a real execution rather than by a HEAD request.
//
// It talks to the real internet and downloads hundreds of megabytes, so it is
// behind a build tag and never runs with the unit tests:
//
//	make registry-live                                  # every recipe
//	go test -tags=live -run 'TestLiveRegistry/foundry' ./registry/
//
// A failure here means a recipe stopped matching reality — a renamed asset, a
// moved repository, a dropped platform — which is the only time a human needs
// to touch the registry.
package registry_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/block/internal/fetch"
	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/resolver"
	"github.com/nao1215/block/internal/store"
	"github.com/nao1215/block/internal/version"
	"github.com/nao1215/block/registry"
)

const (
	liveTimeout  = 20 * time.Minute
	probeTimeout = 60 * time.Second
)

func TestLiveRegistry(t *testing.T) {
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	gh := github.NewFromEnv("block/live-test")
	if gh.Token == "" {
		t.Log("GITHUB_TOKEN is unset: the GitHub API rate limit may end this run early")
	}
	// One store for the whole run, so re-running a single tool is cheap.
	root := os.Getenv("BLOCK_LIVE_HOME")
	if root == "" {
		root = filepath.Join(t.TempDir(), "home")
	}
	st := &store.Store{Root: root}
	fetcher := fetch.New(st.CacheDir(), "block/live-test")

	for _, rec := range reg.Recipes() {
		t.Run(rec.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), liveTimeout)
			defer cancel()

			majors, err := majorLines(ctx, gh, rec.Source)
			if err != nil {
				t.Fatalf("version discovery: %v", err)
			}
			t.Logf("major lines in the repository's tags: %v", majors)

			// Resolved the way `block lock` would for a user who pinned a
			// major line, rather than by demanding the newest tag exactly.
			// The newest line is tried first and older ones after it, because
			// "the newest line has nothing installable yet" is a normal state
			// an upstream passes through — a tag pushed before its release,
			// or a new major published as a pre-release, which block skips by
			// design. A recipe is broken when NO line resolves, and that is
			// what fails here.
			res, c, err := resolveNewestLine(ctx, gh, rec.Source, majors)
			if err != nil {
				t.Fatalf("no major line of %s resolves: %v", rec.Source.Repo, err)
			}
			if c.String() != strconv.Itoa(majors[0]) {
				t.Logf("the %d line has no installable release yet; checking %q instead", majors[0], c)
			}
			v := res.Version

			// Every platform the recipe claims must have an artifact that
			// really exists upstream.
			here := platform.Current()
			var mine resolver.Artifact
			for _, p := range rec.Source.SupportedPlatforms() {
				art, err := resolver.ArtifactFor(res, rec.Source, p)
				if err != nil {
					t.Errorf("%s: %v", p, err)
					continue
				}
				if p == here {
					mine = art
				}
				if err := head(ctx, art.URL); err != nil {
					t.Errorf("%s: %v", p, err)
					continue
				}
				t.Logf("%s: %s (digest published: %v)", p, art.URL, art.SHA256 != "")
			}
			if mine.URL == "" {
				// Not a gap: the recipe says this upstream ships nothing
				// here, and another leg of the matrix runs on a platform it
				// does ship for. The artifacts for those platforms were
				// checked above.
				t.Skipf("%s ships nothing for %s; it is checked on %s",
					rec.Name, here, strings.Join(platform.Strings(rec.Source.SupportedPlatforms()), ", "))
			}

			// And the artifact for this machine must download, verify,
			// unpack and run.
			var blob, sha string
			var cached bool
			if err := retry(func() error {
				var err error
				blob, sha, cached, err = fetcher.Fetch(ctx, mine.URL, mine.SHA256)
				return err
			}); err != nil {
				t.Fatalf("downloading %s: %v", mine.URL, err)
			}
			t.Logf("downloaded %s (sha256:%s, cached: %v)", filepath.Base(mine.URL), sha, cached)

			dir, err := st.InstallDir(rec.Name, v.String(), sha)
			if err != nil {
				t.Fatalf("install directory: %v", err)
			}
			if err := st.Install(blob, filepath.Base(mine.URL), dir, rec.Source.Bin, rec.Source.StripComponents); err != nil {
				t.Fatalf("installing: %v", err)
			}
			if err := st.Verify(dir, rec.Source.Bin); err != nil {
				t.Fatalf("verifying the install: %v", err)
			}
			// And every executable it promises has to start. This is the
			// part a HEAD request cannot make: the file is here, unpacked,
			// and it is the program the recipe said it would be.
			for _, b := range rec.Source.Bin {
				bin, err := store.BinPath(dir, b)
				if err != nil {
					t.Errorf("bin %q: %v", b, err)
					continue
				}
				probe(t, bin)
			}
		})
	}
}

// majorLines lists the major versions the recipe's tags expose, newest first.
// Pre-release tags are ignored, the way block ignores them.
func majorLines(ctx context.Context, gh *github.Client, src recipe.Source) ([]int, error) {
	tags, err := gh.Tags(ctx, src.Repo, src.EffectiveTagPrefix())
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	var majors []int
	for _, tag := range tags {
		v, ok := src.ParseTag(tag)
		if !ok || v.IsPrerelease() || seen[v.Major] {
			continue
		}
		seen[v.Major] = true
		majors = append(majors, v.Major)
	}
	if len(majors) == 0 {
		return nil, errNoStableRelease
	}
	sort.Sort(sort.Reverse(sort.IntSlice(majors)))
	return majors, nil
}

// resolveNewestLine resolves the newest major line that has something
// installable, and reports which constraint that was.
func resolveNewestLine(ctx context.Context, gh *github.Client, src recipe.Source, majors []int) (resolver.Resolution, version.Constraint, error) {
	var last error
	for _, major := range majors {
		c := version.MustParseConstraint(strconv.Itoa(major))
		res, err := resolver.Resolve(ctx, gh, src, c)
		if err == nil {
			return res, c, nil
		}
		last = err
	}
	return resolver.Resolution{}, version.Constraint{}, last
}

var errNoStableRelease = errors.New("no stable release found in the repository's tags")

// head checks that an artifact really exists at the URL the recipe renders,
// without downloading all of it. A failure here should mean "this recipe is wrong",
// so the transient failures of a large CDN are retried first.
func head(ctx context.Context, rawURL string) error {
	if err := fetch.CheckURL(rawURL); err != nil {
		return err
	}
	return retry(func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		// A one-byte ranged GET rather than HEAD: GitHub's asset CDN answers
		// some HEAD requests with a gateway timeout even though the object
		// downloads fine.
		req.Header.Set("Range", "bytes=0-0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("%s: %w", rawURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return fmt.Errorf("%s: %s", rawURL, resp.Status)
		}
		return nil
	})
}

// retry repeats an upstream call a few times, because a gateway timeout from
// a release CDN is not a broken recipe.
func retry(fn func() error) error {
	const attempts = 4
	var err error
	for i := range attempts {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
	}
	return err
}

// probe runs an installed executable to confirm it is the program the recipe
// promised and that it can start at all. Upstreams disagree about how to ask
// for a version, so the usual spellings are tried in turn, and --help last:
// a supervisor like cosmovisor or a one-shot tool like rlpdump has no version
// to report, and refusing to install it over that would be pedantry rather
// than a check on the recipe.
func probe(t *testing.T, bin string) {
	t.Helper()
	for _, args := range [][]string{{"--version"}, {"version"}, {"-version"}, {"--help"}} {
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
		cancel()
		if err == nil {
			t.Logf("%s %v: %s", filepath.Base(bin), args, firstLine(out))
			return
		}
	}
	t.Errorf("%s: none of --version, version, -version or --help worked", filepath.Base(bin))
}

func firstLine(b []byte) string {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[:i]
	}
	return string(b)
}
