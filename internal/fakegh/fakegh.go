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
package fakegh

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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
}

const (
	// version strings are derived from tags by stripping "v".
	allPlatforms = "linux/amd64,linux/arm64,darwin/amd64,darwin/arm64"
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
				map[string]string{"linux": "unknown-linux-gnu", "darwin": "apple-darwin"},
				map[string]string{"amd64": "x86_64", "arm64": "aarch64"})}
	}
	example := func(tag string, at int, bin, platforms, ext string) release {
		base := bin[strings.LastIndex(bin, "/")+1:]
		return release{tag: tag, at: at, bins: []string{bin},
			assets: platformAssets(base+"_"+strings.TrimPrefix(tag, "v")+"_{os}_{arch}"+ext, platforms, nil, nil)}
	}
	return []Repo{
		{owner: "foundry-rs", name: "foundry", releases: []release{
			foundry("v1.6.0", 1, false),
			foundry("v1.7.0", 1, false),
			foundry("v1.7.1", 1, false),
			foundry("v1.7.4", 2, false),
			{tag: "v1.7.5", at: 2, noRelease: true},
			foundry("v1.8.0-rc1", 1, true),
			foundry("v1.9.0", 2, true),
			{tag: "nightly-deadbeef", at: 1, prerelease: true, bins: foundryBins,
				assets: platformAssets("foundry_nightly_{os}_{arch}.tar.gz", allPlatforms, nil, nil)},
		}},
		{owner: "informalsystems", name: "hermes", releases: []release{
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
	case strings.HasPrefix(parts[2], "releases/tags/"):
		tag := strings.TrimPrefix(parts[2], "releases/tags/")
		for _, rel := range rp.releases {
			if rel.tag != tag || rel.at > snapshot || rel.noRelease || rel.draft {
				continue
			}
			assets := []map[string]any{}
			for _, name := range rel.assets {
				assets = append(assets, map[string]any{
					"name":                 name,
					"browser_download_url": fmt.Sprintf("%s/download/%s/%s/%s/%s", s.base, rp.owner, rp.name, rel.tag, name),
					"size":                 0,
				})
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
esac
echo "%s %s (fake)"
[ $# -gt 0 ] && echo "args: $*"
exit 0
`, name, ver)
}

func buildArchive(name string, rel release) []byte {
	ver := strings.TrimPrefix(rel.tag, "v")
	var members []entry
	for _, b := range rel.bins {
		members = append(members, entry{name: b, content: script(b, ver), mode: 0o755})
	}
	// The asset name goes into the README so every platform's archive hashes
	// differently, as real per-platform builds do.
	members = append(members, entry{name: "README.md", content: "fake release " + rel.tag + " " + name + "\n", mode: 0o644})
	members = append(members, rel.entries...)
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
