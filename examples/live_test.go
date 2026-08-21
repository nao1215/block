//go:build live

// Live validation of the example manifests. examples_test.go proves each file
// is one block would accept; this proves the versions in it still resolve to
// something a user could install today.
//
// It talks to the real GitHub API, so it is behind a build tag and never runs
// with the unit tests:
//
//	make examples-live
//	go test -tags=live -run 'TestLiveExamples/evm-contracts' ./examples/
//
// A failure means an example has aged out — an upstream that stopped
// publishing the line it pins, or a repository that moved — which is a
// documentation bug worth the same attention as a broken recipe.
package examples_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nao1215/block/internal/github"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/resolver"
	"github.com/nao1215/block/registry"
)

const liveTimeout = 10 * time.Minute

func TestLiveExamples(t *testing.T) {
	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	gh := github.NewFromEnv("block/live-test")
	if gh.Token == "" {
		t.Log("GITHUB_TOKEN is unset: the GitHub API rate limit may end this run early")
	}

	for _, path := range examples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			m, err := manifest.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), liveTimeout)
			defer cancel()

			for _, tool := range m.Tools {
				// byo-tool.toml points at a repository that deliberately does
				// not exist: it teaches the shape of a project-local source,
				// it does not name a real tool.
				if tool.Source != nil {
					t.Logf("%s: project-local source, not resolved", tool.Name)
					continue
				}
				rec, ok := reg.Lookup(tool.Name)
				if !ok {
					t.Errorf("%s is not in the registry", tool.Name)
					continue
				}
				src := rec.Source
				res, err := resolver.Resolve(ctx, gh, src, tool.Constraint)
				if err != nil {
					t.Errorf("%s = %q no longer resolves: %v", tool.Name, tool.Constraint, err)
					continue
				}
				// The manifest declares the platforms a reader will lock for,
				// so every one of them must have an artifact that exists.
				for _, p := range m.Platforms {
					if _, err := resolver.ArtifactFor(res, src, p); err != nil {
						t.Errorf("%s %s: %v", tool.Name, p, err)
					}
				}
				t.Logf("%s = %q resolves to %s", tool.Name, tool.Constraint, res.Version)
			}
		})
	}
}
