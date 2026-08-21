package cmdinfo

import (
	"runtime/debug"
	"strings"
	"testing"
)

// A `go install github.com/nao1215/block@latest` runs no ldflags, so before
// this the binary called itself "dev" and told upstreams so in its
// User-Agent. The Go toolchain records the module version it resolved; that
// is where the version comes from when the linker did not supply one.
func TestResolveVersion(t *testing.T) {
	t.Parallel()
	buildInfo := func(v string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Path: "github.com/nao1215/block", Version: v}}, true
		}
	}
	none := func() (*debug.BuildInfo, bool) { return nil, false }

	tests := []struct {
		name      string
		injected  string
		buildInfo func() (*debug.BuildInfo, bool)
		want      string
	}{
		{
			name:      "a GoReleaser build uses the stamped tag",
			injected:  "v1.2.3",
			buildInfo: buildInfo("v9.9.9"), // must not win over the linker
			want:      "v1.2.3",
		},
		{
			name:      "go install falls back to the resolved module version",
			injected:  devVersion,
			buildInfo: buildInfo("v1.2.3"),
			want:      "v1.2.3",
		},
		{
			name:      "a pseudo-version is still a version",
			injected:  devVersion,
			buildInfo: buildInfo("v0.0.0-20260821134500-abcdef012345"),
			want:      "v0.0.0-20260821134500-abcdef012345",
		},
		{
			name:      "a build from a local checkout says dev",
			injected:  devVersion,
			buildInfo: buildInfo("(devel)"),
			want:      devVersion,
		},
		{
			name:      "an empty module version says dev",
			injected:  devVersion,
			buildInfo: buildInfo(""),
			want:      devVersion,
		},
		{
			name:      "no build info at all says dev",
			injected:  devVersion,
			buildInfo: none,
			want:      devVersion,
		},
		{
			name:      "an empty ldflags value is not a version",
			injected:  "",
			buildInfo: buildInfo("v1.2.3"),
			want:      "v1.2.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveVersion(tt.injected, tt.buildInfo); got != tt.want {
				t.Errorf("resolveVersion(%q) = %q, want %q", tt.injected, got, tt.want)
			}
		})
	}
}

// Whatever the version turns out to be, it is what upstreams are told.
func TestUserAgentCarriesTheResolvedVersion(t *testing.T) {
	t.Parallel()
	ua := UserAgent()
	if !strings.HasPrefix(ua, Name+"/") {
		t.Errorf("UserAgent() = %q, want it to start with %q", ua, Name+"/")
	}
	if got := strings.TrimPrefix(ua, Name+"/"); got != Resolve() {
		t.Errorf("UserAgent() carries %q but Resolve() is %q", got, Resolve())
	}
}
