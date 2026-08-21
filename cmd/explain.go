package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nao1215/block/internal/diag"
)

// newExplainCmd looks a diagnostic code up without a browser. It reads the
// same registry the published reference is generated from, so a code cannot
// mean one thing in a terminal and another on the website.
func newExplainCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "explain <code>",
		Short: "Explain one of block's " + diag.Prefix + " error codes",
		Long: `explain prints what a diagnostic code means, what block observed, and what
to do about it.

  block explain ` + diag.All()[0].Code.String() + `
  block explain ` + strings.ToLower(diag.All()[0].Code.String()) + `

Every code is listed at https://nao1215.github.io/block/errors/. Like list,
explain is offline and read-only: it needs no block.toml, no block.lock and no
network.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			e, ok := diag.Lookup(args[0])
			if !ok {
				// Uncoded on purpose: mistyping an argument is not one of the
				// problems the codes are for, and a code here would be a
				// number to look up that only says "look up a number".
				return fmt.Errorf("unknown error code %q\nevery code is listed at https://nao1215.github.io/block/errors/", args[0])
			}
			fmt.Fprint(stdout, diag.Text(e))
			return nil
		},
	}
}
