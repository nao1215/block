package platform

import (
	"runtime"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"} {
		p, err := Parse(ok)
		if err != nil {
			t.Errorf("Parse(%q) error = %v", ok, err)
		}
		if p.String() != ok || !p.IsSupported() {
			t.Errorf("Parse(%q) = %v", ok, p)
		}
	}
	for _, bad := range []string{"", "linux", "linux/", "/amd64", "windows/amd64", "linux/386", "Linux/amd64", "linux-amd64"} {
		_, err := Parse(bad)
		if err == nil {
			t.Errorf("Parse(%q) accepted", bad)
			continue
		}
		if strings.Contains(bad, "/") && bad != "linux/" && bad != "/amd64" && !strings.Contains(err.Error(), "supported platforms are") {
			t.Errorf("Parse(%q) error %q does not list the supported platforms", bad, err)
		}
	}
}

func TestCurrent(t *testing.T) {
	t.Parallel()
	p := Current()
	if p.OS != runtime.GOOS || p.Arch != runtime.GOARCH {
		t.Errorf("Current() = %v", p)
	}
}

func TestSupportedIsACopy(t *testing.T) {
	t.Parallel()
	s := Supported()
	s[0] = Platform{OS: "plan9", Arch: "mips"}
	if Supported()[0] == s[0] {
		t.Error("Supported() exposed the internal table")
	}
}

func TestSortAndStrings(t *testing.T) {
	t.Parallel()
	ps := []Platform{{OS: "linux", Arch: "arm64"}, {OS: "darwin", Arch: "arm64"}, {OS: "linux", Arch: "amd64"}}
	Sort(ps)
	got := strings.Join(Strings(ps), " ")
	if got != "darwin/arm64 linux/amd64 linux/arm64" {
		t.Errorf("Sort() = %s", got)
	}
}
