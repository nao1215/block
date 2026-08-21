// Package registry holds the built-in recipes that tell block how to find a
// tool's releases upstream, and answers which tools exist for a blockchain
// system. Each recipe is a TOML file in this directory, embedded into the
// binary at build time.
//
// The registry is a set of rules, not a version database: a new upstream
// release needs no change here. A recipe only changes when the upstream
// renames its assets or moves repositories. Ecosystem names are recipe data
// too, so a new blockchain system needs no change to block itself. The
// directory is kept free of any other logic so it can later move to its own
// repository unchanged.
package registry

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/nao1215/block/internal/recipe"
)

//go:embed *.toml
var files embed.FS

// Registry is a set of recipes indexed by tool name.
type Registry struct {
	recipes map[string]recipe.Recipe
}

// Builtin loads the recipes embedded in this binary.
func Builtin() (*Registry, error) {
	return Load(files)
}

// Load parses every *.toml in fsys as a recipe. The file name must equal the
// recipe's name so that the directory listing is the index.
func Load(fsys fs.FS) (*Registry, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	r := &Registry{recipes: map[string]recipe.Recipe{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		var rec recipe.Recipe
		md, err := toml.Decode(string(data), &rec)
		if err != nil {
			return nil, fmt.Errorf("registry: %s: %w", e.Name(), err)
		}
		if undecoded := md.Undecoded(); len(undecoded) > 0 {
			return nil, fmt.Errorf("registry: %s: unknown key %q", e.Name(), undecoded[0].String())
		}
		if err := rec.Validate(); err != nil {
			return nil, fmt.Errorf("registry: %s: %w", e.Name(), err)
		}
		if want := strings.TrimSuffix(e.Name(), ".toml"); want != rec.Name {
			return nil, fmt.Errorf("registry: %s: recipe name %q does not match the file name", e.Name(), rec.Name)
		}
		// Normalized at load time so that every listing is deterministic
		// regardless of how a recipe file happens to be written.
		sort.Strings(rec.Ecosystems)
		r.recipes[rec.Name] = rec
	}
	return r, nil
}

// Lookup returns the recipe for a tool.
func (r *Registry) Lookup(name string) (recipe.Recipe, bool) {
	rec, ok := r.recipes[name]
	return rec, ok
}

// Recipes lists every registered recipe, sorted by tool name.
func (r *Registry) Recipes() []recipe.Recipe {
	out := make([]recipe.Recipe, 0, len(r.recipes))
	for _, rec := range r.recipes {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Ecosystems lists every blockchain system some recipe serves, sorted.
func (r *Registry) Ecosystems() []string {
	seen := map[string]bool{}
	for _, rec := range r.recipes {
		for _, e := range rec.Ecosystems {
			seen[e] = true
		}
	}
	out := make([]string, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// ByEcosystem lists the recipes serving one blockchain system, sorted by tool
// name. An ecosystem no recipe serves yields nothing; callers that must tell
// "none" from "unknown" ask Ecosystems.
func (r *Registry) ByEcosystem(ecosystem string) []recipe.Recipe {
	var out []recipe.Recipe
	for _, rec := range r.Recipes() {
		if slices.Contains(rec.Ecosystems, ecosystem) {
			out = append(out, rec)
		}
	}
	return out
}
