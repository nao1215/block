package doc

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/block/registry"
)

// doc/tools.md is generated from the recipes, and a generated file that
// nothing checks is a file that is wrong by the next release.
func TestToolsDocMatchesTheRegistry(t *testing.T) {
	t.Parallel()
	want, err := Tools()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "doc", "tools.md")
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("doc/tools.md is stale; run \"make doc\".\n%s", firstDifference(string(got), string(want)))
	}
}

// Every tool has to appear, or the catalogue quietly loses one when an
// ecosystem name is misspelled in a recipe and its section disappears.
func TestToolsDocNamesEveryRecipe(t *testing.T) {
	t.Parallel()
	page, err := Tools()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range reg.Recipes() {
		if !strings.Contains(string(page), "| `"+rec.Name+"` |") {
			t.Errorf("recipe %q is missing from doc/tools.md", rec.Name)
		}
	}
}

// firstDifference reports the first line that differs, so a failure points at
// the change instead of printing two whole documents.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		gl, wl := "", ""
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return "line " + strconv.Itoa(i+1) + ":\n  committed: " + gl + "\n  generated: " + wl
		}
	}
	return ""
}
