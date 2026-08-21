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
//
// Besides GitHub, the server plays a vendor download host at /blobs/<name>
// for http-type recipes (modelled on go-ethereum's gethstore).
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
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// release is one fixture release.
type release struct {
	tag        string
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
}

type entry struct {
	name    string
	link    string
	content string
	mode    int64
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
	return []Repo{
		{owner: "foundry-rs", name: "foundry", digest: true, releases: []release{
			foundry("v1.6.0", 1, false),
			foundry("v1.7.0", 1, false),
			foundry("v1.7.1", 1, false),
			foundry("v1.7.4", 1, false),
			foundry("v1.7.5", 2, false),
			{tag: "v1.7.6", at: 2, noRelease: true},
			foundry("v1.8.0-rc1", 1, true),
			foundry("v1.9.0", 2, true),
			{tag: "nightly-deadbeef", at: 1, prerelease: true, bins: foundryBins,
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
		{owner: "example", name: "linky", releases: []release{
			{tag: "v1.0.0", at: 1, bins: []string{"linky"},
				assets:  platformAssets("linky_1.0.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil),
				entries: []entry{{name: "etc-passwd", link: "/etc/passwd"}}},
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
		{owner: "example", name: "bare", releases: []release{
			// Tags without the "v" prefix, for tag_prefix = "".
			{tag: "2.5.0", at: 1, bins: []string{"bare"},
				assets: platformAssets("bare_2.5.0_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
	}
}

// Server is the fake GitHub handler. Base must be set to the URL clients
// reach it at before serving, because release assets carry absolute URLs.
type Server struct {
	base  string
	repos map[string]Repo
	// blobs caches generated archives by "owner/name/tag/asset".
	mu    sync.Mutex
	blobs map[string][]byte
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
	if rest, ok := strings.CutPrefix(path, "/ratelimited"); ok {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
		_ = rest
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

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request, rest string, snapshot int) {
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	rp, ok := s.repos[parts[0]+"/"+parts[1]]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	switch {
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
				writeJSON(w, http.StatusOK, map[string]string{"sha": commitOf(rel.tag)})
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
	case strings.HasPrefix(parts[2], "releases/tags/"):
		tag := strings.TrimPrefix(parts[2], "releases/tags/")
		for _, rel := range rp.releases {
			if rel.tag != tag || rel.at > snapshot || rel.noRelease || rel.draft {
				continue
			}
			assets := []map[string]any{}
			for _, name := range rel.assets {
				a := map[string]any{
					"name":                 name,
					"browser_download_url": fmt.Sprintf("%s/download/%s/%s/%s/%s", s.base, rp.owner, rp.name, rel.tag, name),
					"size":                 0,
				}
				if rp.digest {
					sum := sha256.Sum256(buildArchive(name, rel))
					a["digest"] = "sha256:" + hex.EncodeToString(sum[:])
				}
				assets = append(assets, a)
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
	for _, rel := range rp.releases {
		if rel.tag != parts[2] {
			continue
		}
		for _, name := range rel.assets {
			if name != parts[3] {
				continue
			}
			s.mu.Lock()
			blob, ok := s.blobs[rest]
			if !ok {
				blob = buildArchive(name, rel)
				s.blobs[rest] = blob
			}
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(blob)
			return
		}
	}
	http.NotFound(w, r)
}

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
// spelled the way Bitcoin Core spells them.
var knownTargets = map[string]bool{ //nolint:gochecknoglobals // immutable table
	"x86_64-linux-gnu": true, "aarch64-linux-gnu": true,
	"x86_64-apple-darwin": true, "arm64-apple-darwin": true,
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
	// A vendor blob has no asset map; its own naming decides.
	return strings.Contains(name, "windows")
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
	members = append(members, rel.entries...)
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
		if m.link != "" {
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = m.link
		} else {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(m.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			log.Fatal(err)
		}
		if m.link == "" {
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
		hdr.SetMode(os.FileMode(m.mode)) //nolint:gosec // fixture modes are small constants
		f, err := zw.CreateHeader(hdr)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := f.Write([]byte(m.content)); err != nil {
			log.Fatal(err)
		}
	}
	_ = zw.Close()
	return buf.Bytes()
}
