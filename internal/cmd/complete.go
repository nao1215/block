package cmd

import (
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/lockfile"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/recipe"
	"github.com/nao1215/block/registry"
)

// Dynamic completion: what a shell offers after `block lock `, `block which `,
// `block list ` and `block explain `. Every candidate list here is read from
// something already on this machine — the project's own two files, the
// registry snapshot in the binary, the diagnostic table — and none of them
// resolves, downloads or installs anything. A completion that cannot answer
// (no project here, a file that will not parse) offers nothing rather than an
// error: a shell prompt is no place for a diagnostic.

// noFiles is the directive every completion here ends with: the candidates
// are names block knows, and a file name is never one of them.
const noFiles = cobra.ShellCompDirectiveNoFileComp

// completeManifestTools offers the tools block.toml declares.
func completeManifestTools(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	m, err := loadProjectManifest()
	if err != nil {
		return nil, noFiles
	}
	var names []string
	for _, t := range m.Tools {
		if !slices.Contains(args, t.Name) {
			names = append(names, t.Name)
		}
	}
	return names, noFiles
}

// completeLockedCommands offers the commands block.lock provides, which are
// exactly the names `block which` and `block exec` can answer for.
func completeLockedCommands(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	dir, err := projectDir()
	if err != nil {
		return nil, noFiles
	}
	l, err := lockfile.Load(dir + string(os.PathSeparator) + lockfile.FileName)
	if err != nil {
		return nil, noFiles
	}
	var names []string
	for _, t := range l.Tools {
		names = append(names, commandNames(t.Bin)...)
	}
	slices.Sort(names)
	return slices.Compact(names), noFiles
}

// completeEcosystems offers the ecosystems the embedded registry knows.
func completeEcosystems(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	reg, err := registry.Builtin()
	if err != nil {
		return nil, noFiles
	}
	return reg.Ecosystems(), noFiles
}

// completeCodes offers every diagnostic code, each with its summary, so the
// shell can show what a code means before it is typed.
func completeCodes(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, noFiles
	}
	entries := diag.All()
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Code.String()+"\t"+e.Summary)
	}
	return out, noFiles
}

func projectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return manifest.Find(wd)
}

func loadProjectManifest() (*manifest.Manifest, error) {
	dir, err := projectDir()
	if err != nil {
		return nil, err
	}
	return manifest.Load(dir + string(os.PathSeparator) + manifest.FileName)
}

// commandNames reduces archive-relative executable paths to the command
// names a user types. What "the command name" is belongs to the recipe
// package, which is where every other caller asks.
func commandNames(bins []string) []string {
	out := make([]string, len(bins))
	for i, b := range bins {
		out[i] = recipe.CommandName(b)
	}
	return out
}
