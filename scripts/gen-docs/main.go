// Command gen-docs writes the documentation block generates rather than
// writes by hand: doc/tools.md from the recipes embedded in this repository,
// and doc/errors.md from the diagnostic-code registry. Both are published as
// pages of the website, so generating them here keeps one source of truth for
// each instead of a second list that drifts away from it.
//
//	gen-docs            # rewrite the generated documentation
//	gen-docs -check     # fail if any of it is stale
//
// It is a maintenance tool, not part of block: nothing the CLI does at run
// time reads it, and `go install` of block does not build it.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/doc"
)

// docPerm is the mode a generated documentation file is written with.
const docPerm = 0o600

// generated pairs each generated file with the function that renders it.
var generated = []struct { //nolint:gochecknoglobals // the immutable list of outputs
	path   string
	render func() ([]byte, error)
}{
	{filepath.Join("doc", "tools.md"), doc.Tools},
	{filepath.Join("doc", "errors.md"), func() ([]byte, error) { return diag.Markdown(), nil }},
}

func main() {
	dir := flag.String("dir", ".", "repository root to write into")
	check := flag.Bool("check", false, "compare instead of writing, and exit 1 on a difference")
	flag.Parse()

	if err := run(*dir, *check); err != nil {
		fmt.Fprintf(os.Stderr, "gen-docs: %v\n", err)
		os.Exit(1)
	}
}

func run(dir string, check bool) error {
	for _, g := range generated {
		want, err := g.render()
		if err != nil {
			return err
		}
		path := filepath.Join(dir, g.path)
		if !check {
			if err := os.WriteFile(filepath.Clean(path), want, docPerm); err != nil {
				return err
			}
			continue
		}
		got, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("%s is stale; run \"make doc\"", g.path)
		}
	}
	return nil
}
