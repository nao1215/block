// Package manifest reads block.toml: the human-written statement of which
// tools a project wants and roughly which versions.
//
//	platforms = ["linux/amd64", "darwin/arm64"]   # optional; default: this machine
//
//	[tools]
//	foundry = "1.7"
//
//	[tools.foo]
//	version = "1.2"
//	[tools.foo.source]
//	type = "github_release"
//	repo = "example/foo"
//	asset = "foo_{version}_{os}_{arch}.tar.gz"
//	bin = ["foo"]
package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/internal/version"
)

// FileName is the manifest file name block looks for.
const FileName = "block.toml"

// Manifest is a parsed block.toml.
type Manifest struct {
	// Platforms are the platforms `block lock` resolves artifacts for. Empty
	// means the current platform only.
	Platforms []platform.Platform
	// Tools are sorted by name.
	Tools []Tool
}

// Tool is one [tools] entry.
type Tool struct {
	Name       string
	Constraint version.Constraint
	// Source is set for project-local definitions and nil for registry tools.
	Source *recipe.Source
}

// Tool returns the entry with the given name.
func (m *Manifest) Tool(name string) (Tool, bool) {
	for _, t := range m.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Names lists the tool names in order.
func (m *Manifest) Names() []string {
	out := make([]string, len(m.Tools))
	for i, t := range m.Tools {
		out[i] = t.Name
	}
	return out
}

// EffectivePlatforms returns the declared platforms, or [current] when none
// are declared.
func (m *Manifest) EffectivePlatforms(current platform.Platform) []platform.Platform {
	if len(m.Platforms) == 0 {
		return []platform.Platform{current}
	}
	out := make([]platform.Platform, len(m.Platforms))
	copy(out, m.Platforms)
	return out
}

type rawManifest struct {
	Platforms []string                  `toml:"platforms"`
	Tools     map[string]toml.Primitive `toml:"tools"`
}

type rawTool struct {
	Version string         `toml:"version"`
	Source  *recipe.Source `toml:"source"`
}

// Load reads and parses the manifest at path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the path is the project's own block.toml
	if err != nil {
		return nil, err
	}
	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return m, nil
}

// Parse parses manifest bytes.
func Parse(data []byte) (*Manifest, error) {
	var raw rawManifest
	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	seen := map[string]bool{}
	for _, p := range raw.Platforms {
		pp, err := platform.Parse(p)
		if err != nil {
			return nil, err
		}
		if seen[pp.String()] {
			return nil, fmt.Errorf("platform %q is listed twice", p)
		}
		seen[pp.String()] = true
		m.Platforms = append(m.Platforms, pp)
	}
	platform.Sort(m.Platforms)
	if len(raw.Tools) == 0 {
		return nil, errors.New("no tools declared: add a [tools] table")
	}
	for name, prim := range raw.Tools {
		t, err := parseTool(md, name, prim)
		if err != nil {
			return nil, err
		}
		m.Tools = append(m.Tools, t)
	}
	// Undecoded keys are only known once every tool primitive was decoded.
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown key %q", undecoded[0].String())
	}
	sort.Slice(m.Tools, func(i, j int) bool { return m.Tools[i].Name < m.Tools[j].Name })
	return m, nil
}

func parseTool(md toml.MetaData, name string, prim toml.Primitive) (Tool, error) {
	if err := recipe.ValidateName(name); err != nil {
		return Tool{}, err
	}
	var short string
	if err := md.PrimitiveDecode(prim, &short); err == nil {
		c, err := version.ParseConstraint(short)
		if err != nil {
			return Tool{}, fmt.Errorf("tool %q: %w", name, err)
		}
		return Tool{Name: name, Constraint: c}, nil
	}
	var long rawTool
	if err := md.PrimitiveDecode(prim, &long); err != nil {
		return Tool{}, fmt.Errorf("tool %q: want a version string or a table with version and source: %w", name, err)
	}
	c, err := version.ParseConstraint(long.Version)
	if err != nil {
		return Tool{}, fmt.Errorf("tool %q: %w", name, err)
	}
	if long.Source == nil {
		return Tool{}, fmt.Errorf("tool %q: a [tools.%s] table needs a [tools.%s.source] table (or write %s = %q)", name, name, name, name, long.Version)
	}
	if err := long.Source.Validate(); err != nil {
		return Tool{}, fmt.Errorf("tool %q: %w", name, err)
	}
	return Tool{Name: name, Constraint: c, Source: long.Source}, nil
}

// Find walks from dir upward looking for block.toml and returns the directory
// that holds it.
func Find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, FileName)); err == nil && !st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s not found in the current directory or any parent", FileName)
		}
		dir = parent
	}
}
