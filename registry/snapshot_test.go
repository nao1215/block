package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/block/internal/snapshot"
)

// The recipes here are generated: a copy of block-registry at the revision
// SNAPSHOT names. This is the check that says so — an edit made in block
// instead of in block-registry would otherwise ship, and be silently undone
// by the next sync.
func TestVendoredRecipesMatchTheirSnapshot(t *testing.T) {
	t.Parallel()
	s, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Verify(os.DirFS("."), s); err != nil {
		t.Error(err)
	}
	if s.Recipes != len(mustBuiltin(t).Recipes()) {
		t.Errorf("SNAPSHOT records %d recipes but %d load", s.Recipes, len(mustBuiltin(t).Recipes()))
	}
}

// The drift check has to fail on the smallest edit there is, or it is not a
// check at all. A copy of the real recipes with one byte changed stands in
// for someone fixing a recipe in the wrong repository.
func TestDriftIsCaughtByASingleCharacter(t *testing.T) {
	t.Parallel()
	s, err := Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	edited := fstest.MapFS{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var first string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = e.Name()
			data = append(data, ' ')
		}
		edited[e.Name()] = &fstest.MapFile{Data: data}
	}
	err = snapshot.Verify(edited, s)
	if err == nil {
		t.Fatalf("a space appended to %s did not fail the snapshot check", first)
	}
	if !strings.Contains(err.Error(), "make registry-sync") {
		t.Errorf("the drift message must point at the sync:\n%s", err)
	}
}

func mustBuiltin(t *testing.T) *Registry {
	t.Helper()
	r, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
