// Package examples_test holds the checks that keep examples/*.toml honest.
//
// An example manifest is documentation people copy, so it has to be a file
// block would actually accept: every tool has to exist in the registry (or
// bring its own source), every constraint has to parse, and every platform has
// to be one block installs for. What the test cannot check offline is whether
// a version still exists upstream — that is what "make registry-live" is for.
package examples_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/registry"
)

func TestEveryExampleIsAManifestBlockAccepts(t *testing.T) {
	t.Parallel()
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range examples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			m, err := manifest.Load(path)
			if err != nil {
				t.Fatalf("block would refuse this file: %v", err)
			}
			if len(m.Tools) == 0 {
				t.Fatal("an example with no tools teaches nothing")
			}
			for _, tool := range m.Tools {
				if tool.Source != nil {
					// A project-local source is the point of that example;
					// it must still be a source block can execute.
					if err := tool.Source.Validate(); err != nil {
						t.Errorf("%s: %v", tool.Name, err)
					}
					continue
				}
				if _, ok := reg.Lookup(tool.Name); !ok {
					t.Errorf("%s is not in the registry; run \"block list\" for the names that are", tool.Name)
				}
			}
		})
	}
}

// A manifest that names platforms its tools have no build for would fail at
// "block lock", which is a poor thing to hand someone as an example.
func TestEveryExampleDeclaresPlatformsItsToolsShip(t *testing.T) {
	t.Parallel()
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range examples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			m, err := manifest.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Platforms) == 0 {
				t.Fatal("an example should declare its platforms: that is the part people get wrong")
			}
			for _, tool := range m.Tools {
				rec, ok := reg.Lookup(tool.Name)
				if !ok {
					continue // a project-local source; covered above
				}
				for _, p := range m.Platforms {
					if !rec.Source.Supports(p) {
						t.Errorf("%s has no %s build upstream, so \"block lock\" would fail here", tool.Name, p)
					}
				}
			}
		})
	}
}

// The README table is the index people read first, so every example has to be
// in it and every entry has to point at a file that exists.
func TestTheREADMEListsEveryExample(t *testing.T) {
	t.Parallel()
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range examples(t) {
		name := filepath.Base(path)
		if !strings.Contains(string(readme), "["+"`"+name+"`"+"](./"+name+")") {
			t.Errorf("examples/README.md does not list %s", name)
		}
	}
}

func examples(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob("*.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no examples found")
	}
	return paths
}
