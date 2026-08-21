// Package cmdinfo carries the build-time identity of the binary.
//
// A version reaches block by one of two routes, and it has to work by both.
// A release is built by GoReleaser, which stamps [Version] through -ldflags.
// A `go install github.com/nao1215/block@latest` is not — no ldflags are
// passed — but the Go toolchain records the module version it resolved in the
// binary's build info, so that is where the version comes from instead.
// Neither is available in a build from a local checkout, which is the only
// case that should say "dev".
package cmdinfo

import (
	"runtime/debug"
	"sync"
)

// Name is the command name.
const Name = "block"

// devVersion is what a build from a local working tree reports. The Go
// toolchain also writes it into build info for such a build, which is why
// [resolveVersion] treats it as "no version here" rather than as an answer.
const devVersion = "dev"

// Version is injected with -ldflags "-X github.com/nao1215/block/internal/cmdinfo.Version=v1.2.3".
// Read it through [Resolve]; it is the first source, not the only one.
var Version = devVersion //nolint:gochecknoglobals // set by the linker

// resolved caches the answer. Build info is read from the binary's own
// headers, which is cheap but not free, and the value cannot change while the
// process runs.
var resolved = sync.OnceValue(func() string { //nolint:gochecknoglobals // memoized constant
	return resolveVersion(Version, debug.ReadBuildInfo)
})

// Resolve reports the version this binary should call itself.
func Resolve() string { return resolved() }

// resolveVersion picks a version from the two places one can be, in order of
// how much they are worth trusting. It takes its inputs rather than reading
// globals so the ordering can be tested without building binaries.
func resolveVersion(injected string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	// A release: GoReleaser stamped it, and it is exactly the tag.
	if injected != "" && injected != devVersion {
		return injected
	}
	// A `go install`: no ldflags ran, but the toolchain recorded the module
	// version it resolved. "(devel)" is what it writes for a build from a
	// local checkout, which is the one case that really is "dev".
	if info, ok := readBuildInfo(); ok && info != nil {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return devVersion
}

// UserAgent identifies block to upstream servers.
func UserAgent() string { return Name + "/" + Resolve() }
