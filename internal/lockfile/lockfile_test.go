package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/platform"
)

const (
	sha    = "593c607acd4d8fe57f560298f64779441a0aa7461893223def00eeedc612d0bb"
	source = "sha256:0000000000000000000000000000000000000000000000000000000000000001"
)

func sample() *Lock {
	return &Lock{Version: FormatVersion, Tools: []Tool{
		{
			Name: "hermes", Constraint: "1.13", Version: "1.13.0", Bin: []string{"hermes"}, Source: source,
			Artifacts: []Artifact{{Platform: "linux/amd64", URL: "https://example.com/h.tar.gz", SHA256: sha}},
		},
		{
			Name: "foundry", Constraint: "1.7", Version: "1.7.1", Bin: []string{"forge", "cast"}, Source: source,
			Artifacts: []Artifact{
				{Platform: "linux/amd64", URL: "https://example.com/l.tar.gz", SHA256: sha},
				{Platform: "darwin/arm64", URL: "https://example.com/d.tar.gz", SHA256: sha},
			},
		},
	}}
}

func TestMarshalRoundTripIsDeterministicAndSorted(t *testing.T) {
	t.Parallel()
	data, err := Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), header) {
		t.Errorf("missing header:\n%s", data)
	}
	if strings.Index(string(data), `name = "foundry"`) > strings.Index(string(data), `name = "hermes"`) {
		t.Error("tools are not sorted by name")
	}
	if strings.Index(string(data), `platform = "darwin/arm64"`) > strings.Index(string(data), `platform = "linux/amd64"`) {
		t.Error("artifacts are not sorted by platform")
	}
	// The lock holds facts, not the recipe.
	for _, forbidden := range []string{"repo", "asset", "[tools.source]"} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("lockfile leaks recipe field %q:\n%s", forbidden, data)
		}
	}
	again, err := Marshal(sample())
	if err != nil || string(again) != string(data) {
		t.Fatal("Marshal is not deterministic")
	}
	l, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(Marshal()) error = %v\n%s", err, data)
	}
	back, err := Marshal(l)
	if err != nil || string(back) != string(data) {
		t.Errorf("round trip differs:\n%s\n---\n%s", data, back)
	}
	if _, ok := l.Tool("nope"); ok {
		t.Error("Tool(nope) found")
	}
}

func TestArtifactHelpers(t *testing.T) {
	t.Parallel()
	tool := sample().Tools[1]
	a, ok := tool.Artifact(platform.Platform{OS: "darwin", Arch: "arm64"})
	if !ok || a.URL != "https://example.com/d.tar.gz" {
		t.Errorf("Artifact() = %+v, %v", a, ok)
	}
	if _, ok := tool.Artifact(platform.Platform{OS: "linux", Arch: "arm64"}); ok {
		t.Error("missing platform found")
	}
	tool.SetArtifact(Artifact{Platform: "linux/arm64", URL: "u", SHA256: sha})
	tool.SetArtifact(Artifact{Platform: "linux/amd64", URL: "replaced", SHA256: sha})
	if len(tool.Artifacts) != 3 || tool.Artifacts[0].Platform != "darwin/arm64" || tool.Artifacts[1].URL != "replaced" || tool.Artifacts[2].Platform != "linux/arm64" {
		t.Errorf("SetArtifact() = %+v", tool.Artifacts)
	}
	v, err := tool.ParsedVersion()
	if err != nil || v.String() != "1.7.1" {
		t.Errorf("ParsedVersion() = %v, %v", v, err)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	valid := func(edit func(string) string) string {
		data, _ := Marshal(sample())
		return edit(string(data))
	}
	tests := []struct {
		name, in, want string
	}{
		{"syntax", "version = \n", "expected"},
		{"unknown key", "version = 1\nextra = true\n", `unknown key "extra"`},
		{"version 0", "version = 0\n", "unsupported lockfile version 0"},
		{"version 2", "version = 2\n", "unsupported lockfile version 2"},
		{"bad name", valid(func(s string) string { return strings.Replace(s, `name = "hermes"`, `name = "Hermes"`, 1) }), `invalid tool name "Hermes"`},
		{"dup tool", valid(func(s string) string { return strings.Replace(s, `name = "hermes"`, `name = "foundry"`, 1) }), `tool "foundry" is locked twice`},
		{"bad constraint", valid(func(s string) string { return strings.Replace(s, `constraint = "1.7"`, `constraint = "~1.7"`, 1) }), `tool "foundry": invalid version constraint`},
		{"bad version", valid(func(s string) string { return strings.Replace(s, `version = "1.7.1"`, `version = "v1.7.1"`, 1) }), `tool "foundry": invalid version "v1.7.1"`},
		{"no bin", valid(func(s string) string { return strings.Replace(s, `bin = ["hermes"]`, `bin = []`, 1) }), `tool "hermes": bin is empty`},
		{"negative strip", "version = 1\n[[tools]]\nname = \"hermes\"\nconstraint = \"1\"\nversion = \"1.0.0\"\nbin = [\"hermes\"]\nstrip_components = -1\n", `tool "hermes": strip_components must not be negative`},
		{"bad platform", valid(func(s string) string {
			return strings.Replace(s, `platform = "darwin/arm64"`, `platform = "plan9/arm64"`, 1)
		}), `tool "foundry": unsupported platform`},
		{"dup platform", valid(func(s string) string {
			return strings.Replace(s, `platform = "darwin/arm64"`, `platform = "linux/amd64"`, 1)
		}), `platform linux/amd64 is locked twice`},
		{"empty url", valid(func(s string) string {
			return strings.Replace(s, `url = "https://example.com/d.tar.gz"`, `url = ""`, 1)
		}), `tool "foundry" (darwin/arm64): url is empty`},
		{"short sha", valid(func(s string) string { return strings.Replace(s, sha, "abc", 1) }), `sha256 "abc" is not a 64-character hex digest`},
		{"upper sha", valid(func(s string) string { return strings.Replace(s, sha, strings.ToUpper(sha), 1) }), "is not a 64-character hex digest"},

		// A lockfile arrives through pull requests and hand edits. Its
		// version becomes a directory name under $BLOCK_HOME, so anything
		// that could be read as a path is refused before a download starts.
		{"version escapes with unix separators", valid(func(s string) string {
			return strings.Replace(s, `version = "1.7.1"`, `version = "1.7/../../outside"`, 1)
		}), `tool "foundry": invalid version "1.7/../../outside"`},
		{"version escapes with windows separators", valid(func(s string) string {
			return strings.Replace(s, `version = "1.7.1"`, `version = '1.7\..\..\outside'`, 1)
		}), `tool "foundry": invalid version`},
		{"version hides a separator in the pre-release", valid(func(s string) string {
			return strings.Replace(s, `version = "1.7.1"`, `version = "1.7.1-rc/../../outside"`, 1)
		}), `tool "foundry": invalid version`},
		{"version carries a control character", valid(func(s string) string {
			return strings.Replace(s, `version = "1.7.1"`, `version = "1.7.1-rc\u0000"`, 1)
		}), `tool "foundry": invalid version`},
		{"version has an empty pre-release identifier", valid(func(s string) string {
			return strings.Replace(s, `version = "1.7.1"`, `version = "1.7.1-a..b"`, 1)
		}), `empty pre-release identifier`},

		// And the pin has to be one the constraint beside it could have
		// chosen: "resolved 1.7 to 9.9.9" installs what nobody asked for.
		{"version does not satisfy its constraint", valid(func(s string) string {
			return strings.Replace(s, `version = "1.7.1"`, `version = "9.9.9"`, 1)
		}), `tool "foundry": version 9.9.9 does not satisfy the constraint "1.7"`},
		{"version is a pre-release its constraint excludes", valid(func(s string) string {
			return strings.Replace(s, `version = "1.7.1"`, `version = "1.7.2-rc.1"`, 1)
		}), `does not satisfy the constraint`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tt.in))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWriteAndLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if _, err := Load(path); !os.IsNotExist(err) {
		t.Errorf("Load(missing) error = %v", err)
	}
	if err := Write(path, sample()); err != nil {
		t.Fatal(err)
	}
	l, err := Load(path)
	if err != nil || len(l.Tools) != 2 {
		t.Fatalf("Load() = %v, %v", l, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
	if err := os.WriteFile(path, []byte("version = 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.HasPrefix(err.Error(), FileName+": ") {
		t.Errorf("Load(bad) error = %v", err)
	}
	if err := Write(filepath.Join(dir, "missing", FileName), sample()); err == nil {
		t.Error("Write into a missing directory succeeded")
	}
}
