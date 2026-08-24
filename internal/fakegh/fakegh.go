// Package fakegh is an offline stand-in for the GitHub API and release
// downloads. It serves a fixed set of repositories, tags and releases and
// generates every release archive in memory with deterministic contents, so
// checksums are stable across runs. It backs both the atago end-to-end suite
// (through the e2e/fakegh command) and the in-process tests of the block
// use cases.
//
// Two "points in time" are exposed so a test can observe an upstream
// publishing a new version without any mutable state:
//
//	/t1/repos/...   only releases that exist at time 1
//	/repos/...      everything (time 2)
//
// Special prefixes emulate failure modes:
//
//	/ratelimited/repos/...   403 with X-RateLimit-Remaining: 0
//	/downgrade/<name>        302 to plain http on a host that is not loopback
//	/gate/<key>/<n>/<route>  held until n requests wait together (see gate.go)
//
// Besides GitHub, the server plays a vendor download host at /blobs/<name>
// for http-type recipes (modelled on go-ethereum's gethstore).
//
// A private repository is served the way GitHub serves one: the API answers
// "Not Found" unless the request bears [Token], and its release assets are
// had only through the asset endpoint, which redirects to a signed URL on the
// objects host — the same server reached as "localhost" rather than
// "127.0.0.1", because what a signed URL rejects is a request that also
// carries a bearer token, and that is only observable across a host boundary.
package fakegh

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Token is the one bearer token the private fixtures accept.
const Token = "fakegh-secret-token"

// HugeSize is the size the API reports for the assets of a repository
// flagged huge, and the Content-Length its downloads announce: larger than
// block transfers, so nothing is ever actually sent.
const HugeSize = 3 << 30

// release is one fixture release.
type release struct {
	tag string
	// moves marks a tag the upstream retags — Foundry's "nightly". The commit
	// it points at depends on the snapshot, and the release published for
	// that commit is served under "<tag>-<commit>" without being listed:
	// that is what real upstreams do, and it is what block pins.
	moves      bool
	at         int // 1 or 2: the snapshot in which the release appears
	draft      bool
	prerelease bool
	// noRelease marks a tag that exists without a GitHub release.
	noRelease bool
	// assets maps "os/arch" to the asset file name for this tag.
	assets map[string]string
	// bins lists the script executables inside each archive, as archive paths.
	bins []string
	// entries adds raw archive members (for malformed-archive fixtures).
	entries []entry
	// prefix wraps every member in a directory, as versioned tarballs do.
	prefix string
	// twice publishes every asset of this release under its name a second
	// time, at a different URL: the ambiguity block must refuse rather than
	// resolve by taking whichever the API answered with first.
	twice bool
	// unpinnable marks a moving tag the upstream retags without publishing
	// a release for the commit under it: nothing that will not move, so
	// nothing block can pin.
	unpinnable bool
	// pad adds this many directory members named pad/ to every archive of
	// the release, so that an archive can hold more entries than block
	// extracts without listing each one here.
	pad int
}

type entry struct {
	name    string
	link    string
	content string
	mode    int64
	// typ overrides the tar type flag, so a fixture can carry the entries a
	// tool distribution never legitimately has: a device node, a FIFO.
	typ byte
}

// Repo is one fixture repository.
type Repo struct {
	owner, name string
	releases    []release
	// digest makes the API report GitHub's sha256 for every asset, as it
	// does for uploads made since 2025.
	digest bool
	// blobs serves this repository's archives from /blobs/ (a vendor
	// download host) instead of as release assets, named
	// <name>-<os>-<arch>-<version>-<commit8>.tar.gz with a directory prefix.
	blobs bool
	// private hides the repository from any request without [Token] and
	// serves its assets only through the API's asset endpoint.
	private bool
	// huge makes every asset of the repository [HugeSize] bytes, as the API
	// reports it and as the download announces it.
	huge bool
}

const (
	// allPlatforms is every platform block installs for. The fixtures publish
	// an asset for each, so a scenario runs the same wherever the suite does:
	// a fixture that shipped Unix builds only would refuse the whole Windows
	// leg with "unsupported platform" long before it reached what the
	// scenario is about.
	//
	// A fixture that is deliberately narrower — the one that has no build for
	// this machine — names its platforms itself.
	allPlatforms = "linux/amd64,linux/arm64,darwin/amd64,darwin/arm64,windows/amd64,windows/arm64"
)

func platformAssets(template, platforms string, osMap, archMap map[string]string) map[string]string {
	out := map[string]string{}
	for _, p := range strings.Split(platforms, ",") {
		osName, arch, _ := strings.Cut(p, "/")
		o, a := osName, arch
		if m, ok := osMap[osName]; ok {
			o = m
		}
		if m, ok := archMap[arch]; ok {
			a = m
		}
		out[p] = strings.NewReplacer("{os}", o, "{arch}", a).Replace(template)
	}
	return out
}

// Fixtures returns the fixture repositories shared by every test suite.
func Fixtures() []Repo {
	foundryBins := []string{"forge", "cast", "anvil", "chisel"}
	foundry := func(tag string, at int, pre bool) release {
		return release{tag: tag, at: at, prerelease: pre, bins: foundryBins,
			assets: platformAssets("foundry_"+tag+"_{os}_{arch}.tar.gz", allPlatforms, nil, nil)}
	}
	hermes := func(tag string, at int) release {
		return release{tag: tag, at: at, bins: []string{"hermes"},
			assets: platformAssets("hermes-"+tag+"-{arch}-{os}.tar.gz", allPlatforms,
				map[string]string{"linux": "unknown-linux-gnu", "darwin": "apple-darwin", "windows": "pc-windows-msvc"},
				map[string]string{"amd64": "x86_64", "arm64": "aarch64"})}
	}
	example := func(tag string, at int, bin, platforms, ext string) release {
		base := bin[strings.LastIndex(bin, "/")+1:]
		return release{tag: tag, at: at, bins: []string{bin},
			assets: platformAssets(base+"_"+strings.TrimPrefix(tag, "v")+"_{os}_{arch}"+ext, platforms, nil, nil)}
	}
	// cometbft is the registry tool the suite uses where a scenario is about
	// the registry itself rather than about resolution: its recipe is the one
	// in block-registry that ships for every platform block supports, so a
	// scenario written with it runs the same everywhere. foundry and hermes
	// below publish no Windows build, which is upstream's choice and a thing
	// the suite asserts once, deliberately.
	cometbft := func(tag string, at int, pre bool) release {
		return release{tag: tag, at: at, prerelease: pre, bins: []string{"cometbft"},
			assets: platformAssets("cometbft_"+strings.TrimPrefix(tag, "v")+"_{os}_{arch}.tar.gz", allPlatforms, nil, nil)}
	}
	return []Repo{
		{owner: "cometbft", name: "cometbft", digest: true, releases: []release{
			cometbft("v1.6.0", 1, false),
			cometbft("v1.7.0", 1, false),
			cometbft("v1.7.1", 1, false),
			cometbft("v1.7.4", 1, false),
			cometbft("v1.7.5", 2, false),
			cometbft("v1.8.0-rc1", 1, true),
			cometbft("v1.9.0", 2, true),
		}},
		{owner: "foundry-rs", name: "foundry", digest: true, releases: []release{
			foundry("v1.6.0", 1, false),
			foundry("v1.7.0", 1, false),
			foundry("v1.7.1", 1, false),
			foundry("v1.7.4", 1, false),
			foundry("v1.7.5", 2, false),
			{tag: "v1.7.6", at: 2, noRelease: true},
			foundry("v1.8.0-rc1", 1, true),
			foundry("v1.9.0", 2, true),
			// The moving tag, and the assets it carries. Every night it
			// points somewhere new, which is what the snapshot changes here.
			{tag: "nightly", at: 1, moves: true, prerelease: true, bins: foundryBins,
				assets: platformAssets("foundry_nightly_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "informalsystems", name: "hermes", digest: true, releases: []release{
			hermes("v1.13.0", 1),
			hermes("v1.13.1", 2),
		}},
		{owner: "example", name: "foo", releases: []release{
			example("v1.2.0", 1, "foo", allPlatforms, ".tar.gz"),
			example("v1.2.3", 2, "foo", allPlatforms, ".tar.gz"),
		}},
		{owner: "example", name: "maconly", releases: []release{
			example("v0.1.0", 1, "maconly", "darwin/arm64", ".tar.gz"),
		}},
		{owner: "example", name: "zipper", releases: []release{
			example("v3.0.0", 1, "zipper", allPlatforms, ".zip"),
		}},
		{owner: "example", name: "nested", releases: []release{
			example("v1.0.0", 1, "bin/nested", allPlatforms, ".tar.gz"),
		}},
		{owner: "example", name: "evil", releases: []release{
			{tag: "v1.0.0", at: 1, bins: []string{"evil"},
				assets:  platformAssets("evil_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: "../escape", content: "outside\n", mode: 0o644}}},
		}},
		{owner: "example", name: "linked", releases: []release{
			// A link beside the executable it points at, which is how real
			// distributions ship a second name for one program: Nethermind
			// publishes Nethermind.Runner this way.
			{tag: "v1.0.0", at: 1, bins: []string{"linked"},
				assets:  platformAssets("linked_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: "Runner", link: "linked"}}},
		}},
		{owner: "example", name: "linky", releases: []release{
			{tag: "v1.0.0", at: 1, bins: []string{"linky"},
				assets:  platformAssets("linky_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: "etc-passwd", link: "/etc/passwd"}}},
		}},
		{owner: "example", name: "devnode", releases: []release{
			// A character device inside a release archive. No tool
			// distribution has one; an archive that does is asking for
			// something other than files on disk.
			{tag: "v1.0.0", at: 1, bins: []string{"devnode"},
				assets:  platformAssets("devnode_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: "null", typ: tar.TypeChar, mode: 0o666}}},
		}},
		{owner: "example", name: "winpath", releases: []release{
			// A member naming a Windows drive. It is refused wherever block
			// runs, not only where it would have meant something.
			{tag: "v1.0.0", at: 1, bins: []string{"winpath"},
				assets:  platformAssets("winpath_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: `C:\Windows\System32\evil`, content: "outside\n", mode: 0o644}}},
		}},
		{owner: "example", name: "unc", releases: []release{
			{tag: "v1.0.0", at: 1, bins: []string{"unc"},
				assets:  platformAssets("unc_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: `\\server\share\evil`, content: "outside\n", mode: 0o644}}},
		}},
		{owner: "example", name: "twofiles", releases: []release{
			// One name, two members: what lands on disk would depend on
			// extraction order.
			{tag: "v1.0.0", at: 1, bins: []string{"twofiles"},
				assets: platformAssets("twofiles_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{
					{name: "README.md", content: "second\n", mode: 0o644},
				}},
		}},
		{owner: "example", name: "dupasset", releases: []release{
			// The same asset name published twice, at two URLs.
			{tag: "v1.0.0", at: 1, twice: true, bins: []string{"dupasset"},
				assets: platformAssets("dupasset_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "example", name: "noassets", releases: []release{
			// A published release with nothing attached: a project that is
			// built from source and tags releases without shipping binaries.
			{tag: "v1.0.0", at: 1},
		}},
		{owner: "example", name: "nobin", releases: []release{
			{tag: "v1.0.0", at: 1, bins: []string{"something-else"},
				assets: platformAssets("nobin_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "example", name: "bundle", digest: true, releases: []release{
			// Tools under a versioned directory, like Agave's release
			// archives (whose real compression, bzip2, is covered by the
			// archive package's unit tests: Go has no bzip2 writer).
			{tag: "v2.0.0", at: 1, bins: []string{"bin/bundle-cli", "bin/bundle-node"}, prefix: "bundle-release",
				assets: platformAssets("bundle-release-{os}-{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "example", name: "quirky", blobs: true, releases: []release{
			// A vendor host whose platform strings are not a product of
			// os and arch, like Bitcoin Core's.
			{tag: "v29.4", at: 1, bins: []string{"bin/quirkyd"}, prefix: "quirky-29.4"},
			{tag: "v29.5", at: 2, bins: []string{"bin/quirkyd"}, prefix: "quirky-29.5"},
		}},
		{owner: "example", name: "rawbin", releases: []release{
			// A single raw executable per platform, no archive.
			{tag: "v1.0.0", at: 1, bins: []string{"rawbin"},
				assets: platformAssets("rawbin-{os}-{arch}", allPlatforms, nil, nil)},
		}},
		{owner: "ethereum", name: "go-ethereum", blobs: true, releases: []release{
			{tag: "v1.17.4", at: 1, bins: []string{"geth"}},
			{tag: "v1.17.5", at: 2, bins: []string{"geth"}},
		}},
		{owner: "example", name: "drifter", releases: []release{
			// A moving tag and nothing else: the upstream retags "nightly"
			// but never publishes a release for the commit beneath it.
			{tag: "nightly", at: 1, moves: true, unpinnable: true, prerelease: true, bins: []string{"drifter"},
				assets: platformAssets("drifter_nightly_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "example", name: "stamped", releases: []release{
			// A release line whose name carries a hyphen, stamping the commit
			// into the asset name the way vyper and Nethermind do. Pinning
			// it has to keep the whole commit, not what follows the first
			// hyphen of the tag.
			{tag: "pre-release", at: 1, moves: true, prerelease: true, bins: []string{"stamped"},
				assets: platformAssets("stamped_{commit}_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "example", name: "bare", releases: []release{
			// Tags without the "v" prefix, for tag_prefix = "".
			{tag: "2.5.0", at: 1, bins: []string{"bare"},
				assets: platformAssets("bare_2.5.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "example", name: "secret", private: true, digest: true, releases: []release{
			// A private repository: resolvable and downloadable with the
			// token, invisible without it.
			example("v1.0.0", 1, "secret", allPlatforms, ".tar.gz"),
		}},
		{owner: "example", name: "huge", huge: true, releases: []release{
			// Assets the API says are larger than block will download.
			example("v1.0.0", 1, "huge", allPlatforms, ".tar.gz"),
		}},
		{owner: "example", name: "crowded", releases: []release{
			// More members than block extracts, almost all of them
			// directories and the last two links: an archive that counted
			// only its files would be under the limit.
			{tag: "v1.0.0", at: 1, bins: []string{"crowded"}, pad: crowdPad - 2,
				assets:  platformAssets("crowded_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: "Runner", link: "crowded"}, {name: "Alias", link: "crowded", typ: tar.TypeLink}}},
		}},
		{owner: "example", name: "crowdedzip", releases: []release{
			// The same in a zip, which has no hard links.
			{tag: "v1.0.0", at: 1, bins: []string{"crowdedzip"}, pad: crowdPad - 1,
				assets:  platformAssets("crowdedzip_1.0.0_{os}_{arch}.zip", allPlatforms, nil, nil),
				entries: []entry{{name: "Runner", link: "crowdedzip"}}},
		}},
		{owner: "example", name: "longlink", releases: []release{
			// A zip symlink whose target is one byte longer than block
			// reads: cut short, it would point at some path nobody wrote.
			{tag: "v1.0.0", at: 1, bins: []string{"longlink"},
				assets:  platformAssets("longlink_1.0.0_{os}_{arch}.zip", allPlatforms, nil, nil),
				entries: []entry{{name: "Runner", link: strings.Repeat("a", longLinkBytes)}}},
		}},
	}
}

// crowdPad is how many members beyond the executable and README it takes to
// exceed the number of entries block extracts: one past 200,000.
const crowdPad = 200_001 - 2

// longLinkBytes is one more than the zip link target block reads.
const longLinkBytes = 4096 + 1

// Server is the fake GitHub handler. Base must be set to the URL clients
// reach it at before serving, because release assets carry absolute URLs.
type Server struct {
	base  string
	repos map[string]Repo
	// blobs caches generated archives by "owner/name/tag/asset".
	mu    sync.Mutex
	blobs map[string][]byte
	// gates are the barriers of [gate], by key.
	gates map[string]*gate
	// GateTimeout is how long a gated request waits before it is refused;
	// zero means [DefaultGateTimeout].
	GateTimeout time.Duration
}

// New builds a Server for the given repositories.
func New(repos []Repo) *Server {
	s := &Server{repos: map[string]Repo{}, blobs: map[string][]byte{}}
	for _, r := range repos {
		s.repos[r.owner+"/"+r.name] = r
	}
	return s
}

// SetBase records the URL clients reach the server at.
func (s *Server) SetBase(base string) { s.base = strings.TrimRight(base, "/") }

// ServeHTTP dispatches API, download and failure-mode routes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	snapshot := 2
	if rest, ok := strings.CutPrefix(path, "/t1"); ok {
		snapshot, path = 1, rest
	}
	if rest, ok := strings.CutPrefix(path, "/gate/"); ok {
		s.serveGate(w, r, rest)
		return
	}
	if rest, ok := strings.CutPrefix(path, "/ratelimited"); ok {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
		_ = rest
		return
	}
	if rest, ok := strings.CutPrefix(path, "/downgrade/"); ok {
		// A locked https (or loopback-http) URL whose host answers with a
		// redirect to plain http somewhere else. block has to refuse the hop
		// rather than follow it.
		// A fixed destination, not one derived from the request: this is a
		// test server, and the point is only that block sees an https (or
		// loopback-http) URL answered by a plain-http Location somewhere
		// else.
		_ = rest
		w.Header().Set("Location", "http://mirror.example.com/downgraded.tar.gz")
		w.WriteHeader(http.StatusFound)
		return
	}
	if rest, ok := strings.CutPrefix(path, "/download/"); ok {
		s.serveDownload(w, r, rest)
		return
	}
	if rest, ok := strings.CutPrefix(path, "/blobs/"); ok {
		s.serveBlob(w, r, rest)
		return
	}
	if rest, ok := strings.CutPrefix(path, "/repos/"); ok {
		s.serveAPI(w, r, rest, snapshot)
		return
	}
	http.NotFound(w, r)
}

// authorized reports whether a request may see a repository: any request
// may see a public one, and a private one shows only to [Token].
func authorized(rp Repo, r *http.Request) bool {
	return !rp.private || r.Header.Get("Authorization") == "Bearer "+Token
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request, rest string, snapshot int) {
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	rp, ok := s.repos[parts[0]+"/"+parts[1]]
	if !ok || !authorized(rp, r) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	if len(parts) == 2 {
		// The repository itself, which is where its visibility is said.
		writeJSON(w, http.StatusOK, map[string]any{"full_name": parts[0] + "/" + parts[1], "private": rp.private})
		return
	}
	switch {
	case strings.HasPrefix(parts[2], "releases/assets/"):
		s.serveAsset(w, r, rp, strings.TrimPrefix(parts[2], "releases/assets/"))
	case strings.HasPrefix(parts[2], "git/matching-refs/tags/"):
		prefix := strings.TrimPrefix(parts[2], "git/matching-refs/tags/")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		if page < 1 {
			page = 1
		}
		if perPage < 1 {
			perPage = 30
		}
		var refs []map[string]string
		for _, rel := range rp.releases {
			if rel.at <= snapshot && strings.HasPrefix(rel.tag, prefix) {
				refs = append(refs, map[string]string{"ref": "refs/tags/" + rel.tag})
			}
		}
		start := (page - 1) * perPage
		if start > len(refs) {
			start = len(refs)
		}
		end := start + perPage
		if end > len(refs) {
			end = len(refs)
		}
		if refs == nil {
			refs = []map[string]string{}
		}
		writeJSON(w, http.StatusOK, refs[start:end])
	case strings.HasPrefix(parts[2], "commits/"):
		ref := strings.TrimPrefix(parts[2], "commits/")
		for _, rel := range rp.releases {
			if rel.tag == ref && rel.at <= snapshot {
				writeJSON(w, http.StatusOK, map[string]string{"sha": commitAt(rel, snapshot)})
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	case strings.HasPrefix(parts[2], "releases/tags/"):
		tag := strings.TrimPrefix(parts[2], "releases/tags/")
		for _, rel := range releasesFor(rp, tag, snapshot) {
			if rel.tag != tag || rel.at > snapshot || rel.noRelease || rel.draft {
				continue
			}
			assets := []map[string]any{}
			for _, name := range rel.assets {
				key := assetKey(rp, rel.tag, name)
				a := map[string]any{
					"id":                   assetID(key),
					"url":                  fmt.Sprintf("%s/repos/%s/%s/releases/assets/%d", s.base, rp.owner, rp.name, assetID(key)),
					"name":                 name,
					"browser_download_url": fmt.Sprintf("%s/download/%s", s.base, key),
					"size":                 int64(0),
				}
				if rp.huge {
					a["size"] = int64(HugeSize)
				}
				if rp.digest {
					sum := sha256.Sum256(s.archive(key, name, rel))
					a["digest"] = "sha256:" + hex.EncodeToString(sum[:])
					a["size"] = int64(len(s.archive(key, name, rel)))
				}
				assets = append(assets, a)
				if rel.twice {
					dup := map[string]any{}
					for k, v := range a {
						dup[k] = v
					}
					dup["browser_download_url"] = fmt.Sprintf("%s/download/%s/%s/%s/mirror/%s", s.base, rp.owner, rp.name, rel.tag, name)
					assets = append(assets, dup)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"tag_name": rel.tag, "draft": rel.draft, "prerelease": rel.prerelease, "assets": assets,
			})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	default:
		http.NotFound(w, r)
	}
}

// assetKey names one asset: "owner/name/tag/asset", which is also its path
// under /download/.
func assetKey(rp Repo, tag, name string) string {
	return fmt.Sprintf("%s/%s/%s/%s", rp.owner, rp.name, tag, name)
}

// assetID is the stable numeric id an asset has in the API.
func assetID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64() >> 1) // halved, so it fits
}

// signature is what the objects host wants in place of a token: a stamp a
// real CDN derives from a secret, and here from the key alone.
func signature(key string) string {
	sum := sha256.Sum256([]byte("sig:" + key))
	return hex.EncodeToString(sum[:8])
}

// objectsBase is the URL of the objects host: this server, reached by the
// other loopback name, so that a hop there crosses a host boundary.
func (s *Server) objectsBase() string {
	if strings.Contains(s.base, "127.0.0.1") {
		return strings.Replace(s.base, "127.0.0.1", "localhost", 1)
	}
	return strings.Replace(s.base, "localhost", "127.0.0.1", 1)
}

// archive returns the (cached) bytes of one asset.
func (s *Server) archive(key, name string, rel release) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	blob, ok := s.blobs[key]
	if !ok {
		blob = buildArchive(name, rel)
		s.blobs[key] = blob
	}
	return blob
}

// findAsset resolves an asset id to the key, name and release it belongs to.
func (s *Server) findAsset(rp Repo, id string) (key, name string, rel release, ok bool) {
	for _, rel := range allReleases(rp) {
		for _, name := range rel.assets {
			key := assetKey(rp, rel.tag, name)
			if strconv.FormatInt(assetID(key), 10) == id {
				return key, name, rel, true
			}
		}
	}
	return "", "", release{}, false
}

// allReleases lists the releases a repository can serve, the pinned
// "<moving tag>-<commit>" ones included.
func allReleases(rp Repo) []release {
	out := append([]release(nil), rp.releases...)
	for _, rel := range rp.releases {
		if !rel.moves || rel.unpinnable {
			continue
		}
		for at := 1; at <= latestSnapshot; at++ {
			tag := rel.tag + "-" + commitAt(rel, at)
			for _, pinned := range releasesFor(rp, tag, latestSnapshot) {
				if pinned.tag == tag {
					out = append(out, pinned)
				}
			}
		}
	}
	return out
}

// serveAsset is the API's asset endpoint. Asked for application/octet-stream
// it answers, as GitHub does, with a redirect to a signed URL on the objects
// host; asked for anything else it describes the asset.
func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request, rp Repo, id string) {
	key, name, _, ok := s.findAsset(rp, id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	if r.Header.Get("Accept") != "application/octet-stream" {
		writeJSON(w, http.StatusOK, map[string]any{"id": assetID(key), "name": name})
		return
	}
	w.Header().Set("Location", fmt.Sprintf("%s/download/%s?sig=%s", s.objectsBase(), key, signature(key)))
	w.WriteHeader(http.StatusFound)
}

func (s *Server) serveDownload(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) != 4 {
		http.NotFound(w, r)
		return
	}
	rp, ok := s.repos[parts[0]+"/"+parts[1]]
	if !ok {
		http.NotFound(w, r)
		return
	}
	// A signed URL is the objects host's, and it accepts one credential only:
	// a request that also carries a token is refused, as S3 refuses it. The
	// browser URL of a private asset, unsigned, is not for a program.
	if sig := r.URL.Query().Get("sig"); sig != "" {
		if sig != signature(rest) {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "Only one auth mechanism allowed", http.StatusBadRequest)
			return
		}
	} else if rp.private {
		http.NotFound(w, r)
		return
	}
	for _, rel := range releasesFor(rp, parts[2], latestSnapshot) {
		if rel.tag != parts[2] {
			continue
		}
		for _, name := range rel.assets {
			if name != parts[3] {
				continue
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			if rp.huge {
				// Announced, never sent: a client that reads the header
				// refuses before the body, and one that does not would wait
				// for bytes that are not coming.
				w.Header().Set("Content-Length", strconv.Itoa(HugeSize))
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = w.Write(s.archive(rest, name, rel))
			return
		}
	}
	http.NotFound(w, r)
}

// latestSnapshot is the point in time a download is served from. Artifacts are
// content-addressed by the tag in them, so which snapshot a pinned tag was
// resolved at does not change what its bytes are.
const latestSnapshot = 2

// commitAt is the commit a tag points at. A tag that moves points somewhere
// new in the later snapshot, which is what makes a nightly a nightly.
func commitAt(rel release, snapshot int) string {
	if !rel.moves {
		return commitOf(rel.tag)
	}
	return commitOf(fmt.Sprintf("%s@%d", rel.tag, snapshot))
}

// releasesFor returns the releases a tag can name: the ones the fixture lists,
// plus — for "<moving tag>-<commit>" — the release the upstream publishes for
// the commit under a tag that moves. Those are not listed, exactly as they are
// not something a fixture can enumerate: there is one per night.
func releasesFor(rp Repo, tag string, snapshot int) []release {
	for _, rel := range rp.releases {
		if rel.tag == tag {
			return rp.releases
		}
	}
	for _, rel := range rp.releases {
		// The moving tag itself may carry a hyphen ("pre-release"), so the
		// commit is what follows the whole tag, not the first hyphen.
		commit, ok := strings.CutPrefix(tag, rel.tag+"-")
		if !ok || !rel.moves || rel.unpinnable || len(commit) != commitHexLen || rel.at > snapshot {
			continue
		}
		// Only the commit this tag really points at, at some snapshot: a
		// pinned tag block never resolved does not exist.
		for at := 1; at <= latestSnapshot; at++ {
			if commitAt(rel, at) != commit {
				continue
			}
			pinned := rel
			pinned.tag = tag
			pinned.moves = false
			// An upstream that stamps the commit into its asset names
			// publishes this night's under this night's commit.
			pinned.assets = make(map[string]string, len(rel.assets))
			for p, name := range rel.assets {
				pinned.assets[p] = strings.ReplaceAll(name, "{commit}", commit[:8])
			}
			return append(rp.releases, pinned)
		}
	}
	return rp.releases
}

// commitHexLen is how long the fake commit SHAs are.
const commitHexLen = 40

// commitOf derives a stable fake commit SHA for a tag.
func commitOf(tag string) string {
	sum := sha256.Sum256([]byte("commit:" + tag))
	return hex.EncodeToString(sum[:])[:40]
}

// serveBlob plays a vendor download host for repositories flagged blobs.
// Two shapes are served:
//
//	/blobs/<tool>-<os>-<arch>-<version>-<commit8>.tar.gz   (go-ethereum style)
//	/blobs/<tool>-<version>/<tool>-<version>-<target>.tar.gz  (Bitcoin Core style)
func (s *Server) serveBlob(w http.ResponseWriter, r *http.Request, name string) {
	if dir, file, ok := strings.Cut(name, "/"); ok {
		s.serveVersionedBlob(w, r, dir, file)
		return
	}
	base, ok := strings.CutSuffix(name, ".tar.gz")
	if !ok {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(base, "-")
	if len(parts) != 5 {
		http.NotFound(w, r)
		return
	}
	tool, ver, commit8 := parts[0], parts[3], parts[4]
	for _, rp := range s.repos {
		if !rp.blobs || rp.name != tool && rp.name != "go-ethereum" {
			continue
		}
		for _, rel := range rp.releases {
			if rel.tag != "v"+ver || !strings.HasPrefix(commitOf(rel.tag), commit8) {
				continue
			}
			wrapped := rel
			wrapped.prefix = base
			s.writeArchive(w, name, wrapped)
			return
		}
	}
	http.NotFound(w, r)
}

// serveVersionedBlob answers a per-version directory listing one file per
// upstream platform string, whatever that string looks like.
func (s *Server) serveVersionedBlob(w http.ResponseWriter, r *http.Request, dir, file string) {
	tool, ver, ok := strings.Cut(dir, "-")
	if !ok || !strings.HasPrefix(file, tool+"-"+ver+"-") || !strings.HasSuffix(file, ".tar.gz") {
		http.NotFound(w, r)
		return
	}
	target := strings.TrimSuffix(strings.TrimPrefix(file, tool+"-"+ver+"-"), ".tar.gz")
	if !knownTargets[target] {
		http.NotFound(w, r)
		return
	}
	for _, rp := range s.repos {
		if !rp.blobs || rp.name != tool {
			continue
		}
		for _, rel := range rp.releases {
			if rel.tag == "v"+ver {
				s.writeArchive(w, file, rel)
				return
			}
		}
	}
	http.NotFound(w, r)
}

// knownTargets are the upstream platform strings the quirky fixture ships,
// spelled the way Bitcoin Core spells them — plus the two Windows ones, so a
// scenario about target maps can install and run on every platform the suite
// runs on rather than only where this particular upstream happens to build.
var knownTargets = map[string]bool{ //nolint:gochecknoglobals // immutable table
	"x86_64-linux-gnu": true, "aarch64-linux-gnu": true,
	"x86_64-apple-darwin": true, "arm64-apple-darwin": true,
	"win64": true, "win64-arm": true,
}

func (s *Server) writeArchive(w http.ResponseWriter, name string, rel release) {
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(buildArchive(name, rel)) //nolint:gosec // generated archive bytes, not user input
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// script is the fake executable: it prints its identity, echoes arguments
// and can be told to exit with a given status.
func script(bin, ver string) string {
	name := bin[strings.LastIndex(bin, "/")+1:]
	return fmt.Sprintf(`#!/bin/sh
case "$1" in
  --exit) exit "$2" ;;
  # A long-running process that shuts down cleanly, like a node or a local
  # test network: "sleep & wait" keeps the shell interruptible so the trap
  # runs.
  --serve) trap 'echo "%[1]s stopping"; exit 7' TERM INT; echo "%[1]s ready"; sleep 30 & wait ;;
  # The same, with no handler at all: the signal itself ends it.
  --hang) echo "%[1]s ready"; sleep 30 & wait ;;
esac
echo "%[1]s %[2]s (fake)"
[ $# -gt 0 ] && echo "args: $*"
exit 0
`, name, ver)
}

// tool is the compiled stand-in executable, when the caller supplied one.
// Windows cannot run the shell script this file otherwise serves, so the
// runner builds e2e/faketool for the host and hands the bytes over; the
// program behaves identically, reading its name and version from where block
// installed it. Empty means "serve the script", which is what every Unix run
// does.
var tool []byte //nolint:gochecknoglobals // fixture state, set once before serving

// SetTool makes every served executable a copy of the compiled stand-in.
func SetTool(binary []byte) { tool = binary }

// windowsAsset reports whether an asset is the Windows build of a release, by
// finding the platform it was published for. The name alone is not enough:
// a recipe may rename the platform (Rust spells it "pc-windows-msvc") or
// replace the pair entirely with a target string.
func windowsAsset(rel release, name string) bool {
	for p, asset := range rel.assets {
		if asset == name {
			return strings.HasPrefix(p, "windows/")
		}
	}
	// A vendor blob has no asset map; its own naming decides. The quirky
	// fixture spells Windows the way a vendor host does — "win64", not
	// "windows" — which is the whole reason that fixture exists.
	return strings.Contains(name, "windows") || strings.Contains(name, "win64")
}

// executable is the body of one fake tool inside an archive.
func executable(bin, ver string) entry {
	if len(tool) > 0 {
		// A Go string holds arbitrary bytes, so the archive writers need no
		// second field for a binary member.
		return entry{name: bin, content: string(tool), mode: 0o755}
	}
	return entry{name: bin, content: script(bin, ver), mode: 0o755}
}

func buildArchive(name string, rel release) []byte {
	ver := strings.TrimPrefix(rel.tag, "v")
	if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".zip") {
		// A raw executable: the stand-in itself, platform-flavoured.
		if len(tool) > 0 {
			return tool
		}
		return []byte(script(rel.bins[0], ver) + "# " + name + "\n")
	}
	// A real upstream's Windows archive names its executables with the
	// suffix Windows needs to run them, and block looks for exactly that.
	// A fixture that shipped a bare "forge" inside a .zip would fail every
	// Windows install for a reason no upstream has.
	suffix := ""
	if windowsAsset(rel, name) {
		suffix = ".exe"
	}
	var members []entry
	for _, b := range rel.bins {
		members = append(members, executable(b+suffix, ver))
	}
	// The asset name goes into the README so every platform's archive hashes
	// differently, as real per-platform builds do.
	members = append(members, entry{name: "README.md", content: "fake release " + rel.tag + " " + name + "\n", mode: 0o644})
	// An entry that links to one of the executables has to follow it: where
	// the archive names the executable "linked.exe", a link to "linked"
	// points at nothing, which is a broken fixture rather than the thing the
	// scenario is about.
	// The padding goes before the listed entries, so a fixture whose point
	// is the entry past the limit gets to say which member that is.
	for range rel.pad {
		members = append(members, entry{name: "pad/", mode: 0o755})
	}
	for _, e := range rel.entries {
		if e.link != "" && slices.Contains(rel.bins, e.link) {
			e.link += suffix
		}
		members = append(members, e)
	}
	if rel.prefix != "" {
		for i := range members {
			members[i].name = rel.prefix + "/" + members[i].name
		}
	}
	if strings.HasSuffix(name, ".zip") {
		return buildZip(members)
	}
	return buildTarGz(members)
}

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // fixed mtime

func buildTarGz(members []entry) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		hdr := &tar.Header{Name: m.name, Mode: m.mode, ModTime: epoch}
		switch {
		case m.typ != 0:
			hdr.Typeflag = m.typ
			hdr.Linkname = m.link
		case strings.HasSuffix(m.name, "/"):
			hdr.Typeflag = tar.TypeDir
		case m.link != "":
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = m.link
		default:
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(m.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			log.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(m.content)); err != nil {
				log.Fatal(err)
			}
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func buildZip(members []entry) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range members {
		hdr := &zip.FileHeader{Name: m.name, Method: zip.Deflate, Modified: epoch}
		mode, content := os.FileMode(m.mode), m.content //nolint:gosec // fixture modes are small constants
		switch {
		case strings.HasSuffix(m.name, "/"):
			mode |= os.ModeDir
		case m.link != "":
			// A zip symlink is a file whose contents are the target.
			mode, content = mode|os.ModeSymlink|0o777, m.link
		}
		hdr.SetMode(mode)
		f, err := zw.CreateHeader(hdr)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			log.Fatal(err)
		}
	}
	_ = zw.Close()
	return buf.Bytes()
}
