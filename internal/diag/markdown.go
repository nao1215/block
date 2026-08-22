package diag

import (
	"fmt"
	"strings"
)

// families describes each code range on the generated reference page, in the
// order the page presents them.
var families = []struct { //nolint:gochecknoglobals // immutable table
	digit   int
	label   string
	meaning string
}{
	{1, "BLK1xxx", "what was asked for — block.toml, block.lock, and the names typed on the command line"},
	{2, "BLK2xxx", "resolving a version against an upstream, which only `block lock` ever does"},
	{3, "BLK3xxx", "downloading an artifact and proving it is the one block.lock names"},
	{4, "BLK4xxx", "installing into the store under `$BLOCK_HOME`"},
	{5, "BLK5xxx", "running a command with the locked toolchain, through `block exec` or a shim"},
	{6, "BLK6xxx", "a refusal on security grounds — block declining, rather than failing"},
	{9, "BLK9xxx", "an internal error: a bug in block"},
}

// Markdown renders the published error reference from the registry. The
// committed doc/errors.md is this output, and a drift test fails when the two
// disagree, so a code cannot be added, reworded, or removed without its
// documentation following.
func Markdown() []byte {
	var b strings.Builder
	all := All()

	b.WriteString("# Error codes\n\n")
	b.WriteString("A coded error carries a name that can be searched for, linked to, and branched on, so the message beside it stays free to improve without breaking anyone.\n\n")
	b.WriteString("A code is `" + Prefix + "` followed by four digits, and the thousands digit says what kind of problem it is — which is to say, where the fix lives.\n\n")

	b.WriteString("| Codes | Kind | Codes assigned |\n|---|---|---|\n")
	for _, f := range families {
		fmt.Fprintf(&b, "| `%s` | %s | %d |\n", f.label, f.meaning, len(entriesOf(all, f.digit)))
	}
	b.WriteString("\n")
	b.WriteString("The digit is not the exit status. Every coded error exits 1. block's other non-zero exit, 2 from `block lock --check` and `block status`, is a result rather than an error — the lockfile would change, or the toolchain is not ready — and carries no code, because there is nothing to look up.\n\n")
	b.WriteString("Codes are grouped by what you have to fix rather than by where inside block the error was raised, so one code can be reported from several places when the answer is the same in all of them.\n\n")
	b.WriteString("Not every message carries one. A command line block cannot parse at all — an unknown command, an unknown flag, the wrong number of arguments — is reported by the parser, and there is nothing to look up: the message already names what you typed.\n\n")
	b.WriteString("Look a code up from the terminal with `block explain " + all[0].Code.String() + "`, which prints this same text without a browser.\n\n")

	b.WriteString("## Every code\n\n")
	b.WriteString("| Code | Meaning | Since |\n|---|---|---|\n")
	for _, e := range all {
		fmt.Fprintf(&b, "| [`%s`](#%s) | %s | %s |\n", e.Code, anchor(e), e.Summary, e.Since)
	}
	b.WriteString("\n")

	for _, f := range families {
		section := entriesOf(all, f.digit)
		if len(section) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s — %s\n\n", f.label, f.meaning)
		for _, e := range section {
			fmt.Fprintf(&b, "### %s — %s\n\n", e.Code, e.Summary)
			b.WriteString(e.Detail)
			b.WriteString("\n\n")
			fmt.Fprintf(&b, "Fix: %s\n\n", e.Fix)
			fmt.Fprintf(&b, "Exits 1. Since %s.\n\n", e.Since)
		}
	}
	return []byte(b.String())
}

// wrapWidth is the column prose is broken at for a terminal.
const wrapWidth = 76

// Text renders one entry the way `block explain` prints it: the same registry
// the published page is generated from, so a code cannot mean one thing in a
// terminal and another in a browser.
func Text(e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n\n", e.Code, e.Summary)
	b.WriteString(wrap(e.Detail, wrapWidth))
	b.WriteString("\n\nFix\n")
	b.WriteString(wrap(e.Fix, wrapWidth))
	fmt.Fprintf(&b, "\n\nExits 1. Since %s.\n", e.Since)
	fmt.Fprintf(&b, "https://nao1215.github.io/block/errors/#%s\n", anchor(e))
	return b.String()
}

// wrap breaks prose at width for a terminal. The registry stores each field as
// one paragraph, since that is what Markdown wants; a terminal wants lines.
func wrap(s string, width int) string {
	var b strings.Builder
	col := 0
	for i, word := range strings.Fields(s) {
		switch {
		case i == 0:
			col = len(word)
		case col+1+len(word) > width:
			b.WriteByte('\n')
			col = len(word)
		default:
			b.WriteByte(' ')
			col += 1 + len(word)
		}
		b.WriteString(word)
	}
	return b.String()
}

// entriesOf returns the entries belonging to one family, in order.
func entriesOf(all []Entry, digit int) []Entry {
	var out []Entry
	for _, e := range all {
		if e.Code.Family() == digit {
			out = append(out, e)
		}
	}
	return out
}

// anchor is the heading anchor GitHub and Hugo both derive from an entry's
// "### CODE — summary" heading, so a link works in either renderer.
func anchor(e Entry) string {
	heading := fmt.Sprintf("%s — %s", e.Code, e.Summary)
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}
