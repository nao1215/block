// Command registry-snapshot writes and checks registry/SNAPSHOT, the record
// of which block-registry revision block's vendored recipes came from.
//
//	registry-snapshot -verify                       # offline; used by CI
//	registry-snapshot -write -revision <full sha>   # run by registry-sync.sh
//
// It is a maintenance tool, not part of block: nothing the CLI does at run
// time reads it, and `go install` of block does not build it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nao1215/block/internal/snapshot"
)

// defaultSource is the repository recipes are vendored from.
const defaultSource = "https://github.com/nao1215/block-registry"

func main() {
	dir := flag.String("dir", "registry", "directory holding the vendored recipes")
	write := flag.Bool("write", false, "rewrite SNAPSHOT for the recipes now in -dir")
	verify := flag.Bool("verify", false, "check the recipes in -dir against SNAPSHOT")
	revision := flag.String("revision", "", "full commit SHA the recipes were taken from (with -write)")
	source := flag.String("source", defaultSource, "repository the recipes were taken from (with -write)")
	flag.Parse()

	if err := run(*dir, *source, *revision, *write, *verify); err != nil {
		fmt.Fprintf(os.Stderr, "registry-snapshot: %v\n", err)
		os.Exit(1)
	}
}

func run(dir, source, revision string, write, verify bool) error {
	switch {
	case write == verify:
		return errors.New("choose one of -write or -verify")
	case write:
		return writeSnapshot(dir, source, revision)
	default:
		return verifySnapshot(dir)
	}
}

func writeSnapshot(dir, source, revision string) error {
	digest, count, err := snapshot.Digest(os.DirFS(dir))
	if err != nil {
		return err
	}
	s := snapshot.Snapshot{Source: source, Revision: revision, Recipes: count, Digest: digest}
	// Parsed before it is written, so a bad -revision fails here rather
	// than at the next verify.
	if _, err := snapshot.Parse(s.Format()); err != nil {
		return err
	}
	const fileMode = 0o644
	if err := os.WriteFile(filepath.Join(dir, snapshot.FileName), s.Format(), fileMode); err != nil {
		return err
	}
	fmt.Printf("%s: %d recipes from %s at %s\n", filepath.Join(dir, snapshot.FileName), count, source, revision[:12])
	return nil
}

func verifySnapshot(dir string) error {
	path := filepath.Join(dir, snapshot.FileName)
	data, err := os.ReadFile(path) //nolint:gosec // the repository's own file
	if err != nil {
		return err
	}
	s, err := snapshot.Parse(data)
	if err != nil {
		return err
	}
	if err := snapshot.Verify(os.DirFS(dir), s); err != nil {
		return err
	}
	fmt.Printf("%s: %d recipes match block-registry at %s\n", dir, s.Recipes, s.Revision[:12])
	return nil
}
