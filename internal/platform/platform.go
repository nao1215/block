// Package platform names the operating system / CPU pairs block can install
// artifacts for. The vocabulary is Go's (GOOS/GOARCH) because that is what a
// lockfile keys on; recipes map it onto whatever an upstream calls things.
package platform

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
)

// Platform is an OS/architecture pair written as "os/arch" (e.g. linux/amd64).
type Platform struct {
	OS   string
	Arch string
}

// Supported is the closed set of platforms block knows how to install for.
// Windows is intentionally absent from v0.1: none of the targeted toolchains
// are developed or run in CI there, and supporting it well means more than
// swapping the archive format.
var supported = []Platform{ //nolint:gochecknoglobals // immutable table
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
}

// Supported returns a copy of the supported platform list, sorted.
func Supported() []Platform {
	out := make([]Platform, len(supported))
	copy(out, supported)
	return out
}

// Current returns the platform of the running binary. It does not validate
// membership in Supported: callers decide how to report an unsupported host.
func Current() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// Parse parses "os/arch" and rejects anything outside the supported set.
func Parse(s string) (Platform, error) {
	os, arch, ok := strings.Cut(s, "/")
	if !ok || os == "" || arch == "" {
		return Platform{}, fmt.Errorf("invalid platform %q: want os/arch (e.g. linux/amd64)", s)
	}
	p := Platform{OS: os, Arch: arch}
	if !p.IsSupported() {
		return Platform{}, fmt.Errorf("unsupported platform %q: supported platforms are %s", s, strings.Join(Strings(Supported()), ", "))
	}
	return p, nil
}

// IsSupported reports whether p is one of Supported.
func (p Platform) IsSupported() bool {
	for _, s := range supported {
		if s == p {
			return true
		}
	}
	return false
}

// String renders the platform as "os/arch".
func (p Platform) String() string { return p.OS + "/" + p.Arch }

// Strings renders platforms for messages.
func Strings(ps []Platform) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}

// Sort orders platforms by their string form, in place.
func Sort(ps []Platform) {
	sort.Slice(ps, func(i, j int) bool { return ps[i].String() < ps[j].String() })
}
