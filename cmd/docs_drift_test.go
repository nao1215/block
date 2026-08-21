// Documentation drift checks.
//
// They live in package cmd, and reach the repository through ../, so that
// "would block accept this command line?" can be answered by building the
// real root command rather than by exporting a parser nothing else needs.
//
// block's user-facing documentation makes claims a reader will act on: the
// README shows recordings of the tool working, the cookbook quotes commands
// and the exact messages block prints when it refuses, and examples/ holds
// manifests people copy. Every one of those is a thing that can quietly stop
// being true while the code moves.
//
// What these tests can see: that every recording is produced by a tape and
// was rendered after the tape it comes from was last edited; that every
// command a tape types, and every `block …` line the docs show, is one the
// real CLI accepts; that every refusal the cookbook quotes is a string the
// binary can actually produce; and that the files the docs point at exist.
//
// What they cannot see: the pixels. Rendering needs vhs and ffmpeg and is far
// too heavy for CI. Re-rendering from the right tape is what makes a GIF
// current, so the commit-order check is what turns "edited the tape, forgot
// `make demo`" into a failure — not a claim that the frames were read.
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nao1215/block/registry"
)

// docImageRef is one image reference found in a documentation file.
type docImageRef struct {
	path string
	line int
}

// TestDemoAssets_TapesAndRecordingsStayInStep guards the README recordings,
// within what a test can actually see.
func TestDemoAssets_TapesAndRecordingsStayInStep(t *testing.T) {
	t.Parallel()

	tapeGIF, err := tapeOutputGIFs(repoPath("doc/vhs"))
	if err != nil {
		t.Fatalf("scan the tapes: %v", err)
	}
	if len(tapeGIF) == 0 {
		t.Fatal("no tapes found in doc/vhs; the parser or the directory changed")
	}

	// Every tape declares exactly one Output, and that file exists. A tape
	// added without `make demo` being rerun fails here, naming the asset.
	produced := map[string]bool{}
	for tape, gif := range tapeGIF {
		if gif == "" {
			t.Errorf("%s has no Output directive; a tape must declare the GIF it renders", tape)
			continue
		}
		produced[gif] = true
		if _, statErr := os.Stat(repoPath(gif)); statErr != nil {
			t.Errorf("%s declares Output %q, but that file is missing; run \"make demo\"", tape, gif)
		}
		assertRenderedAfter(t, tape, gif)
	}

	// Every recording the documentation embeds must exist and be produced by
	// a tape, so a demo cannot point at an asset nothing regenerates.
	for _, doc := range docsThatEmbedRecordings(t) {
		refs, err := markdownGIFRefs(doc)
		if err != nil {
			t.Fatalf("scan %s: %v", doc, err)
		}
		for _, ref := range refs {
			if _, statErr := os.Stat(repoPath(ref.path)); statErr != nil {
				t.Errorf("%s:%d references %q, which does not exist", doc, ref.line, ref.path)
			}
			if !produced[ref.path] {
				t.Errorf("%s:%d references %q, but no doc/vhs/*.tape produces it; add a tape or fix the reference", doc, ref.line, ref.path)
			}
		}
	}

	// At least one recording has to be on the front page and in the README,
	// or "make demo" is rendering assets nobody sees.
	for _, doc := range docsThatEmbedRecordings(t) {
		refs, err := markdownGIFRefs(doc)
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) == 0 {
			t.Errorf("%s embeds no recording; the README and the front page both lead with one", doc)
		}
	}
}

// The social card is a still frame of the hero recording, cut by "make demo".
// If the recording moves and the card does not, every link preview shows a
// screen the tool no longer produces.
func TestSocialCard_IsRenderedFromTheHeroRecording(t *testing.T) {
	t.Parallel()
	const (
		hero = "doc/img/demo.gif"
		card = "doc/img/social.png"
	)
	if _, err := os.Stat(repoPath(card)); err != nil {
		t.Fatalf("%s is missing; run \"make demo\"", card)
	}
	assertRenderedAfter(t, hero, card)

	// And the site has to be pointing at it, or the card is decoration.
	layout, err := os.ReadFile(repoPath(filepath.Join("website", "layouts", "baseof.html")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(layout), "social.png") {
		t.Error("website/layouts/baseof.html does not derive its social card from doc/img/social.png")
	}
}

// Every tape drives the real CLI. A tape that types a command block does not
// have renders a demo of an error, and nothing else would notice.
func TestDemoTapes_TypeOnlyCommandsBlockHas(t *testing.T) {
	t.Parallel()
	tapes, err := filepath.Glob(repoPath(filepath.Join("doc", "vhs", "*.tape")))
	if err != nil {
		t.Fatal(err)
	}
	if len(tapes) == 0 {
		t.Fatal("no tapes found")
	}
	for _, tape := range tapes {
		data, err := os.ReadFile(filepath.Clean(tape))
		if err != nil {
			t.Fatal(err)
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			typed, ok := tapeTypedCommand(line)
			if !ok {
				continue
			}
			for _, invocation := range blockInvocations(typed) {
				if err := acceptsInvocation(invocation); err != nil {
					t.Errorf("%s:%d types %q, which block rejects: %v", tape, lineNo+1, invocation, err)
				}
			}
		}
	}
}

// A tape that cds into a fixture that is not there records nothing useful.
func TestDemoTapes_FixturesExist(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{"project", "defi", "bridge"} {
		path := repoPath(filepath.Join("doc", "demo", dir, "block.toml"))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("the demo fixture %s is missing: %v", path, err)
		}
	}
	// The lockfiles are deliberately not committed — they pin exact upstream
	// versions and would be stale within a release — so the ignore rule has
	// to stay, or one gets committed by accident.
	ignore, err := os.ReadFile(repoPath(".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ignore), "/doc/demo/*/block.lock") {
		t.Error(".gitignore no longer excludes the demo lockfiles; make demo would commit stale pins")
	}
}

// Every `block …` line the cookbook and the site show has to be one the real
// CLI accepts. A renamed flag or a dropped subcommand would otherwise live on
// in copyable documentation.
func TestDocs_EveryDocumentedInvocationIsAccepted(t *testing.T) {
	t.Parallel()
	for _, doc := range documentedFiles(t) {
		data, err := os.ReadFile(filepath.Clean(doc))
		if err != nil {
			t.Fatal(err)
		}
		for _, cmdLine := range shellLinesIn(string(data)) {
			for _, invocation := range blockInvocations(cmdLine.text) {
				if err := acceptsInvocation(invocation); err != nil {
					t.Errorf("%s:%d documents %q, which block rejects: %v", doc, cmdLine.line, invocation, err)
				}
			}
		}
	}
}

// The cookbook's "Read a refusal" section is its most quoted part, and a
// message that has been reworded is worse than no message at all: someone
// searches for the text they saw and finds a page saying something else.
// Every block: … the documentation quotes has to still be producible by a
// format string in the source. That catches a message that was reworded,
// truncated, or never existed.
//
// What it does not catch is a rewording inside a value another message
// interpolates: a printf verb stands for anything, so "%s is stale" matches
// whatever the first half says. The exact wording of every message a user can
// see is pinned by the E2E suite instead, against the real binary — see
// e2e/atago. This check is the other half: that the documentation quotes
// something the binary could still print.
func TestDocs_EveryQuotedRefusalStillExists(t *testing.T) {
	t.Parallel()
	patterns := messagePatterns(t)
	if len(patterns) < 50 {
		t.Fatalf("only %d message patterns were collected from the source; the collector is broken", len(patterns))
	}
	for _, doc := range documentedFiles(t) {
		data, err := os.ReadFile(filepath.Clean(doc))
		if err != nil {
			t.Fatal(err)
		}
		for _, quoted := range quotedRefusals(string(data)) {
			if !matchesAny(patterns, quoted.text) {
				t.Errorf("%s:%d quotes a message the source can no longer produce:\n%s", doc, quoted.line, quoted.text)
			}
		}
	}
}

// quotedRefusals returns every fenced block that shows block refusing —
// whole, not line by line, because several messages span two lines and the
// second half is the part that says what to do.
func quotedRefusals(doc string) []docLine {
	var out []docLine
	var current []string
	start, inFence := 0, false
	for i, line := range strings.Split(doc, "\n") {
		if fence.MatchString(line) {
			if inFence {
				body := strings.Join(current, "\n")
				if strings.Contains(body, "block: ") {
					out = append(out, docLine{text: body, line: start})
				}
				current, inFence = nil, false
				continue
			}
			inFence, start = true, i+2
			continue
		}
		if inFence {
			current = append(current, line)
		}
	}
	return out
}

// matchesAny reports whether some source format string could have produced
// any part of the quoted text.
func matchesAny(patterns []*regexp.Regexp, quoted string) bool {
	for _, p := range patterns {
		if p.MatchString(quoted) {
			return true
		}
	}
	return false
}

// verb matches a printf verb, including a flagged or width-qualified one.
var verb = regexp.MustCompile(`%[-+# 0-9.*\[\]]*[a-zA-Z]`)

// messagePatterns turns every string literal in the non-test Go source into a
// regexp: the literal text has to appear as written, and each printf verb
// stands for whatever the binary interpolates there. A documented message
// matches when some literal could have produced it.
//
// Literals whose fixed text is too short to be distinctive are dropped —
// "%s: %w" would otherwise match anything at all.
func messagePatterns(t *testing.T) []*regexp.Regexp {
	t.Helper()
	const minFixed = 16
	var out []*regexp.Regexp
	fset := token.NewFileSet()
	err := filepath.WalkDir(repoPath("."), func(path string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == ".git" || d.Name() == "website" || d.Name() == "dist" || d.Name() == "doc"):
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go"):
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, unquoteErr := strconv.Unquote(lit.Value)
			if unquoteErr != nil {
				return true
			}
			if len(verb.ReplaceAllString(text, "")) < minFixed {
				return true
			}
			var b strings.Builder
			last := 0
			for _, loc := range verb.FindAllStringIndex(text, -1) {
				b.WriteString(regexp.QuoteMeta(text[last:loc[0]]))
				b.WriteString(`.*?`)
				last = loc[1]
			}
			b.WriteString(regexp.QuoteMeta(text[last:]))
			if re, compileErr := regexp.Compile("(?s)" + b.String()); compileErr == nil {
				out = append(out, re)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// examples/ is documentation people copy. examples_test.go checks the
// manifests themselves; this checks that the places a reader starts from
// actually point at them.
func TestExamples_AreReachableFromTheDocs(t *testing.T) {
	t.Parallel()
	manifests, err := filepath.Glob(repoPath(filepath.Join("examples", "*.toml")))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Fatal("no examples found")
	}
	readme, err := os.ReadFile(repoPath("README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "](./examples)") {
		t.Error("README.md does not link to examples/")
	}
	cookbook, err := os.ReadFile(repoPath(filepath.Join("doc", "cookbook.md")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cookbook), "examples/") {
		t.Error("doc/cookbook.md does not mention examples/")
	}
}

// docsThatEmbedRecordings are the two front doors: the README and the site's
// front page. Both lead with a recording.
func docsThatEmbedRecordings(t *testing.T) []string {
	t.Helper()
	return []string{repoPath("README.md"), repoPath(filepath.Join("website", "content", "_index.md"))}
}

// documentedFiles are every user-facing document that shows commands.
func documentedFiles(t *testing.T) []string {
	t.Helper()
	files := []string{repoPath("README.md"), repoPath(filepath.Join("doc", "cookbook.md"))}
	pages, err := filepath.Glob(repoPath(filepath.Join("website", "content", "*.md")))
	if err != nil {
		t.Fatal(err)
	}
	return append(files, pages...)
}

// tapeOutputGIFs maps each tape to the file its Output directive names.
func tapeOutputGIFs(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	output := regexp.MustCompile(`^Output\s+"?([^"\s]+)"?`)
	result := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tape") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, readErr := os.ReadFile(filepath.Clean(path))
		if readErr != nil {
			return nil, readErr
		}
		result[filepath.ToSlash(path)] = "" // record the tape even when it declares no Output
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			if m := output.FindStringSubmatch(strings.TrimSpace(scanner.Text())); m != nil {
				result[filepath.ToSlash(path)] = m[1]
				break
			}
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return nil, scanErr
		}
	}
	return result, nil
}

// markdownGIFRefs finds the recordings a markdown file embeds, by repository
// path or by site path.
func markdownGIFRefs(path string) ([]docImageRef, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	image := regexp.MustCompile(`!\[[^\]]*\]\((?:\./)?(?:doc/img|/img)/([^)]+\.gif)\)`)
	var refs []docImageRef
	lineNo := 0
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		lineNo++
		for _, m := range image.FindAllStringSubmatch(scanner.Text(), -1) {
			// Slash-separated on every platform: it is compared against the
			// path a tape's Output directive writes, which is a tape file's
			// text and not a filesystem path.
			refs = append(refs, docImageRef{path: "doc/img/" + m[1], line: lineNo})
		}
	}
	return refs, scanner.Err()
}

// tapeTypedCommand returns the text a `Type "…"` line types.
var tapeType = regexp.MustCompile(`^\s*Type\s+"(.*)"\s*$`)

func tapeTypedCommand(line string) (string, bool) {
	m := tapeType.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// docLine is one line of a fenced code block, with its position in the file.
type docLine struct {
	text string
	line int
}

// fence opens or closes a markdown code block, and names its language.
var fence = regexp.MustCompile("^\\s*```([a-zA-Z]*)")

// shellLinesIn returns the lines of every fenced block that holds commands
// rather than output or configuration. Prose is excluded on purpose: "block
// is the unit a chain is made of" is a sentence, not an invocation, and no
// amount of pattern-matching separates the two reliably outside a fence.
func shellLinesIn(doc string) []docLine {
	var out []docLine
	inFence, isShell, prompted := false, false, false
	for i, line := range strings.Split(doc, "\n") {
		if m := fence.FindStringSubmatch(line); m != nil {
			if inFence {
				inFence, isShell, prompted = false, false, false
				continue
			}
			inFence = true
			switch m[1] {
			case "shell", "bash", "sh", "":
				isShell, prompted = true, false
			case "console":
				isShell, prompted = true, true
			default:
				isShell, prompted = false, false
			}
			continue
		}
		if !inFence || !isShell {
			continue
		}
		// A console block interleaves commands with their output, and only a
		// prompted line is a command: "block v0.1.0" under "$ block version"
		// is what block printed, not something to run.
		if prompted && !strings.HasPrefix(strings.TrimSpace(line), "$ ") {
			continue
		}
		out = append(out, docLine{text: line, line: i + 1})
	}
	return out
}

// blockInvocation matches a `block …` command at a position where a command
// can start: the beginning of a line, after a `$ ` prompt, or after a shell
// operator. It stops at a pipe, a redirect, a comment or a chained command.
var blockInvocation = regexp.MustCompile(`(?:^|\$\s|&&\s|\|\|\s|;\s)(block\s[^|&;>#\n]*)`)

// blockInvocations extracts every block command a shell line contains.
func blockInvocations(line string) []string {
	var out []string
	for _, m := range blockInvocation.FindAllStringSubmatch(strings.TrimSpace(line), -1) {
		invocation := strings.TrimSpace(m[1])
		// A placeholder is documentation, not a command: <cmd>, [tool...].
		if strings.ContainsAny(invocation, "<>[]{}") {
			continue
		}
		out = append(out, invocation)
	}
	return out
}

// acceptsInvocation runs one documented command through the real CLI in a way
// that parses it and does nothing else: an unknown subcommand or an unknown
// flag is an error, and anything past parsing is not reached.
func acceptsInvocation(invocation string) error {
	fields := strings.Fields(invocation)
	if len(fields) < 2 {
		return nil // bare "block"
	}
	args := fields[1:]
	// `block exec <command> …` passes everything after the command to the
	// child, so only the subcommand itself is block's to accept.
	if args[0] == "exec" {
		args = args[:1]
	}
	return acceptsArgs(args)
}

// acceptsArgs asks the real root command whether it would accept these
// arguments: the subcommand must exist and every flag must be one it defines.
// Nothing past parsing runs, so no manifest is read and no network is touched.
func acceptsArgs(args []string) error {
	root := newRootCmd(io.Discard, io.Discard)
	target, remaining, err := root.Find(args)
	if err != nil {
		return err
	}
	if target == root && len(args) > 0 {
		return fmt.Errorf("unknown command %q", args[0])
	}
	return target.ParseFlags(remaining)
}

// assertRenderedAfter fails when source was last changed after derived was
// rendered, which is what "edited it and forgot `make demo`" looks like in
// history. Commit timestamps rather than file mtimes, because a checkout
// gives every file the same arbitrary mtime. When either path has no commit
// yet — both staged in one change, or a shallow clone — there is nothing to
// compare and the check passes.
func assertRenderedAfter(t *testing.T, source, derived string) {
	t.Helper()
	sourceTime, ok := lastCommitUnix(source)
	if !ok {
		return
	}
	derivedTime, ok := lastCommitUnix(derived)
	if !ok {
		return
	}
	if derivedTime < sourceTime {
		t.Errorf("%s was last changed after %s was rendered; run \"make demo\" and commit the result", source, derived)
	}
}

// repoPath makes a repository-relative path usable from inside cmd/.
func repoPath(rel string) string { return filepath.Join("..", rel) }

func lastCommitUnix(path string) (int64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "log", "-1", "--format=%ct", "--", repoPath(path)).Output()
	if err != nil {
		return 0, false
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, false
	}
	seconds, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, false
	}
	return seconds, true
}

// The front pages count the catalogue out loud — "45 tools across 17
// blockchain systems" — and that sentence is the one thing about the registry
// a reader takes on trust without opening doc/tools.md. It goes stale the
// moment a recipe is added, and nothing else notices, because the count is
// prose rather than generated text. So it is checked against the registry the
// binary actually carries.
func TestDocs_CatalogueCountsMatchTheRegistry(t *testing.T) {
	t.Parallel()

	reg, err := registry.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	tools, ecosystems := len(reg.Recipes()), len(reg.Ecosystems())
	if tools == 0 || ecosystems == 0 {
		t.Fatal("the embedded registry is empty; the loader changed")
	}

	// Every phrasing of the claim across the documentation, so a page cannot
	// escape the check by wording it differently.
	claim := regexp.MustCompile(`(\d+) (?:tools|CLIs|recipes) across (\d+) blockchain systems`)
	found := 0
	for _, doc := range append(documentedFiles(t), repoPath(filepath.Join("doc", "tools.md"))) {
		data, readErr := os.ReadFile(filepath.Clean(doc))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, line := range strings.Split(string(data), "\n") {
			m := claim.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			found++
			gotTools, _ := strconv.Atoi(m[1])
			gotEcosystems, _ := strconv.Atoi(m[2])
			if gotTools != tools || gotEcosystems != ecosystems {
				t.Errorf("%s says %q, but the embedded registry has %d tools across %d blockchain systems",
					doc, m[0], tools, ecosystems)
			}
			if strings.Contains(m[0], "CLIs") {
				t.Errorf("%s says %q; a registry entry is a tool, and one tool can provide several CLIs (foundry provides four)", doc, m[0])
			}
		}
	}
	if found == 0 {
		t.Error("no page states how large the catalogue is; the check has nothing to guard")
	}
}

// The documentation is a web: the README points at the cookbook, the cookbook
// at examples/, SECURITY.md at the security page, CONTRIBUTING at the
// registry's own repository. A link that stops resolving is a reader who
// stops. Every relative link in every user-facing document — and every anchor
// it names inside one — has to exist.
//
// Absolute links are left alone: checking them would put the network in the
// unit suite, and a page on another site is not something this repository can
// keep true anyway.
func TestDocs_EveryRelativeLinkResolves(t *testing.T) {
	t.Parallel()

	files := docFiles(t)
	if len(files) < 10 {
		t.Fatalf("only %d documents were found; the collector is broken", len(files))
	}
	for _, doc := range files {
		data, err := os.ReadFile(filepath.Clean(doc))
		if err != nil {
			t.Fatal(err)
		}
		for lineNo, line := range strings.Split(string(data), "\n") {
			for _, m := range markdownLink.FindAllStringSubmatch(line, -1) {
				checkLink(t, doc, lineNo+1, m[1])
			}
		}
	}
}

var markdownLink = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)

// heading matches an ATX heading, whose text becomes an anchor.
var heading = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*$`)

// anchorChars keeps what GitHub and Hugo keep when they derive an anchor.
var anchorChars = regexp.MustCompile(`[^a-z0-9 -]`)

func checkLink(t *testing.T, doc string, line int, target string) {
	t.Helper()
	switch {
	case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"), strings.HasPrefix(target, "mailto:"):
		return
	// A site-absolute link is a Hugo route ("/commands/"), resolved by the
	// site rather than by the filesystem.
	case strings.HasPrefix(target, "/"):
		return
	}
	path, fragment, _ := strings.Cut(target, "#")
	if path == "" {
		if fragment != "" && !hasAnchor(t, doc, fragment) {
			t.Errorf("%s:%d links to #%s, which is not a heading in this file", doc, line, fragment)
		}
		return
	}
	resolved := filepath.Join(filepath.Dir(doc), filepath.FromSlash(path))
	if _, err := os.Stat(resolved); err != nil {
		t.Errorf("%s:%d links to %q, which does not exist", doc, line, target)
		return
	}
	if fragment != "" && strings.HasSuffix(resolved, ".md") && !hasAnchor(t, resolved, fragment) {
		t.Errorf("%s:%d links to %q, whose anchor is not a heading there", doc, line, target)
	}
}

// hasAnchor reports whether a document has a heading that renders as anchor.
func hasAnchor(t *testing.T, doc, anchor string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(doc))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		m := heading.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		text := strings.ToLower(m[1])
		text = anchorChars.ReplaceAllString(text, "")
		if strings.ReplaceAll(text, " ", "-") == anchor {
			return true
		}
	}
	return false
}

// docFiles are every markdown document this repository publishes: the ones at
// the root, the hand-written and generated pages under doc/, and the website's
// content.
func docFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, pattern := range []string{
		"*.md",
		filepath.Join("doc", "*.md"),
		filepath.Join("website", "content", "*.md"),
		filepath.Join("registry", "*.md"),
		filepath.Join("examples", "*.md"),
	} {
		matches, err := filepath.Glob(repoPath(pattern))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, matches...)
	}
	return out
}
