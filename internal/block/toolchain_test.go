package block

import (
	"testing"

	"github.com/nao1215/block/internal/store"
)

// Windows resolves a command on PATH without regard to case, so `block exec
// FORGE` there has to find the same executable `forge` does. Missing it would
// send exec through to whatever PATH offers, which is the one thing it must
// never do. Elsewhere the two are different commands and the match is exact.
func TestLookupKeyFollowsThePlatform(t *testing.T) {
	t.Parallel()
	got := lookupKey("Forge")
	if store.ExeSuffix == "" {
		if got != "Forge" {
			t.Errorf("lookupKey(%q) = %q, want the name unchanged where case matters", "Forge", got)
		}
		return
	}
	if got != "forge" {
		t.Errorf("lookupKey(%q) = %q, want it folded where PATH folds", "Forge", got)
	}
}

// And the suffix a caller types is stripped however it is spelled.
func TestResolveCommandAcceptsTheExecutableSuffix(t *testing.T) {
	t.Parallel()
	tc := &Toolchain{commands: map[string]string{lookupKey("forge"): "/tools/forge"}}
	names := []string{"forge"}
	if store.ExeSuffix != "" {
		names = append(names, "forge.exe", "FORGE.EXE", "Forge", "FORGE")
	}
	for _, name := range names {
		if path, ok := tc.ResolveCommand(name); !ok || path != "/tools/forge" {
			t.Errorf("ResolveCommand(%q) = %q, %v; want the locked executable", name, path, ok)
		}
	}
	if _, ok := tc.ResolveCommand("cast"); ok {
		t.Error("ResolveCommand found a command the toolchain does not provide")
	}
}
