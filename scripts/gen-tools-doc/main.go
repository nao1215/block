// Command gen-tools-doc writes doc/tools.md from the recipes embedded in this
// repository, so the published tool catalogue is the registry rather than a
// second list that drifts away from it.
//
//	gen-tools-doc            # rewrite doc/tools.md
//	gen-tools-doc -check     # fail if doc/tools.md is not what the recipes say
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

	"github.com/nao1215/block/internal/doc"
)

// docPerm is the mode the generated documentation file is written with.
const docPerm = 0o600

func main() {
	out := flag.String("out", filepath.Join("doc", "tools.md"), "file to write")
	check := flag.Bool("check", false, "compare instead of writing, and exit 1 on a difference")
	flag.Parse()

	if err := run(*out, *check); err != nil {
		fmt.Fprintf(os.Stderr, "gen-tools-doc: %v\n", err)
		os.Exit(1)
	}
}

func run(out string, check bool) error {
	want, err := doc.Tools()
	if err != nil {
		return err
	}
	if !check {
		return os.WriteFile(filepath.Clean(out), want, docPerm)
	}
	got, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%s is stale; run \"make doc\"", out)
	}
	return nil
}
