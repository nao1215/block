package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/platform"
)

func TestParseShortAndLongForms(t *testing.T) {
	t.Parallel()
	m, err := Parse([]byte(`
platforms = ["linux/amd64", "darwin/arm64"]

[tools]
foundry = "1.7"
hermes = "1"

[tools.foo]
version = "1.2.3"

[tools.foo.source]
type = "github_release"
repo = "example/foo"
tag_prefix = ""
asset = "foo_{version}_{os}_{arch}.zip"
bin = ["bin/foo"]
platforms = ["linux/amd64"]

[tools.foo.source.arch]
amd64 = "x86_64"
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Names(), ","); got != "foo,foundry,hermes" {
		t.Errorf("Names() = %s (must be sorted)", got)
	}
	if got := strings.Join(platform.Strings(m.Platforms), ","); got != "darwin/arm64,linux/amd64" {
		t.Errorf("Platforms = %s (must be sorted)", got)
	}
	foundry, ok := m.Tool("foundry")
	if !ok || foundry.Constraint.String() != "1.7" || foundry.Source != nil {
		t.Errorf("foundry = %+v", foundry)
	}
	foo, _ := m.Tool("foo")
	if foo.Source == nil || foo.Source.Repo != "example/foo" || foo.Source.EffectiveTagPrefix() != "" || foo.Source.Arch["amd64"] != "x86_64" {
		t.Errorf("foo.Source = %+v", foo.Source)
	}
	if !foo.Constraint.IsExact() {
		t.Error("foo constraint should be exact")
	}
	if _, ok := m.Tool("geth"); ok {
		t.Error("Tool(geth) found")
	}
}

func TestEffectivePlatforms(t *testing.T) {
	t.Parallel()
	here := platform.Platform{OS: "linux", Arch: "amd64"}
	m := &Manifest{}
	if got := m.EffectivePlatforms(here); len(got) != 1 || got[0] != here {
		t.Errorf("EffectivePlatforms() = %v", got)
	}
	m.Platforms = []platform.Platform{{OS: "darwin", Arch: "arm64"}}
	got := m.EffectivePlatforms(here)
	if len(got) != 1 || got[0].OS != "darwin" {
		t.Errorf("EffectivePlatforms() = %v", got)
	}
	got[0] = here
	if m.Platforms[0] == here {
		t.Error("EffectivePlatforms() must return a copy")
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"empty", ``, "no tools declared"},
		{"empty tools", "[tools]\n", "no tools declared"},
		{"toml syntax", "[tools\n", "expected"},
		{"unknown top key", "tool = 1\n[tools]\nfoundry = \"1\"\n", `unknown key "tool"`},
		{"unknown tool key", "[tools.foo]\nversion = \"1\"\nrepo = \"x/y\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"x/y\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\n", `unknown key "tools.foo.repo"`},
		{"tool is a list", "[tools]\nfoo = [1]\n", "want a version string or a table"},
		{"bad name", "[tools]\nFoundry = \"1\"\n", `invalid tool name "Foundry"`},
		{"bad constraint", "[tools]\nfoundry = \"^1\"\n", `tool "foundry": invalid version constraint "^1"`},
		{"not a string", "[tools]\nfoundry = 1\n", `tool "foundry"`},
		{"table without source", "[tools.foo]\nversion = \"1\"\n", "needs a [tools.foo.source] table"},
		{"table bad constraint", "[tools.foo]\nversion = \"x\"\n[tools.foo.source]\ntype = \"github_release\"\n", `tool "foo": invalid version constraint`},
		{"bad source", "[tools.foo]\nversion = \"1\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"x/y\"\nasset = \"a_{version}.tar.gz\"\n", `tool "foo": bin must list`},
		{"unknown source key", "[tools.foo]\nversion = \"1\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"x/y\"\nasset = \"a_{version}.tar.gz\"\nbin = [\"a\"]\nmirror = \"x\"\n", `unknown key "tools.foo.source.mirror"`},
		{"bad platform", "platforms = [\"plan9/amd64\"]\n[tools]\nfoundry = \"1\"\n", `unsupported platform "plan9/amd64"`},
		{"dup platform", "platforms = [\"linux/amd64\", \"linux/amd64\"]\n[tools]\nfoundry = \"1\"\n", "listed twice"},
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

func TestLoadAndFind(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := Load(filepath.Join(root, FileName)); !os.IsNotExist(err) {
		t.Errorf("Load(missing) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("[tools]\nfoundry = \"1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Load(filepath.Join(root, FileName))
	if err != nil || len(m.Tools) != 1 {
		t.Fatalf("Load() = %v, %v", m, err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	got, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(root)
	gotReal, _ := filepath.EvalSymlinks(got)
	if gotReal != want {
		t.Errorf("Find() = %s, want %s", got, want)
	}
	if _, err := Find(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not found in the current directory or any parent") {
		t.Errorf("Find(no manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.toml"), []byte("[tools\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(root, "bad.toml")); err == nil || !strings.HasPrefix(err.Error(), "bad.toml: ") {
		t.Errorf("Load(bad) error = %v, want file-prefixed", err)
	}
}

// Parse takes a hand-written file, so it must never panic, and what it
// accepts must be a manifest the rest of block can act on: sorted unique
// tool names, a parsed constraint for each, and a validated source for every
// project-local tool.
func FuzzParse(f *testing.F) {
	f.Add("[tools]\nfoundry = \"1.7\"\n")
	f.Add("platforms = [\"linux/amd64\"]\n[tools]\nfoundry = \"1.7\"\nhermes = \"1\"\n")
	f.Add("[tools.foo]\nversion = \"1.2\"\n[tools.foo.source]\ntype = \"github_release\"\nrepo = \"example/foo\"\nasset = \"foo_{version}_{os}_{arch}.tar.gz\"\nbin = [\"foo\"]\n")
	f.Add("[tools]\nfoundry = 1\n")
	f.Add("[tools]\n")
	f.Add("")
	f.Add("platforms = [\"linux/amd64\", \"linux/amd64\"]\n[tools]\nx = \"1\"\n")
	f.Fuzz(func(t *testing.T, data string) {
		m, err := Parse([]byte(data))
		if err != nil {
			return
		}
		if len(m.Tools) == 0 {
			t.Fatal("Parse accepted a manifest with no tools")
		}
		for i, tool := range m.Tools {
			if tool.Constraint.IsZero() {
				t.Fatalf("tool %q has no constraint", tool.Name)
			}
			if i > 0 && m.Tools[i-1].Name >= tool.Name {
				t.Fatalf("tools are not sorted and unique: %q, %q", m.Tools[i-1].Name, tool.Name)
			}
			if tool.Source != nil {
				if err := tool.Source.Validate(); err != nil {
					t.Fatalf("tool %q: accepted source does not validate: %v", tool.Name, err)
				}
			}
		}
		for i, p := range m.Platforms {
			if !p.IsSupported() {
				t.Fatalf("platform %s is not supported", p)
			}
			if i > 0 && m.Platforms[i-1].String() >= p.String() {
				t.Fatalf("platforms are not sorted and unique: %s, %s", m.Platforms[i-1], p)
			}
		}
	})
}
