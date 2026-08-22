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

// The vocabulary itself, named so that the table below and any future
// mapping cannot drift apart on a typo.
const (
	osLinux   = "linux"
	osDarwin  = "darwin"
	osWindows = "windows"
	archAMD64 = "amd64"
	archARM64 = "arm64"
)

// Platform is an OS/architecture pair written as "os/arch" (e.g. linux/amd64).
type Platform struct {
	OS   string
	Arch string
}

// supported is the closed set of platforms block knows how to install for.
// A recipe still says which of them its upstream actually ships: most
// blockchain CLIs are Unix-only, and block reports that rather than
// substituting something else.
var supported = []Platform{ //nolint:gochecknoglobals // immutable table
	{OS: osLinux, Arch: archAMD64},
	{OS: osLinux, Arch: archARM64},
	{OS: osDarwin, Arch: archAMD64},
	{OS: osDarwin, Arch: archARM64},
	{OS: osWindows, Arch: archAMD64},
	{OS: osWindows, Arch: archARM64},
}

// Supported returns a copy of the supported platform list, in the order block
// names them: the two Linux platforms, then macOS, then Windows. That is a
// reading order rather than an alphabetical one, and it is what a reader sees
// in the message [Parse] gives for a platform block does not support.
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
