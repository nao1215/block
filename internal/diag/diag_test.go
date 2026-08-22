package diag_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/diag"
)

// A published reference is only worth linking to if every code in it is
// complete. register refuses an entry that omits any field, so this is the
// other half: that the set as a whole is one a reader can navigate.
func TestEveryCodeIsPublishable(t *testing.T) {
	t.Parallel()

	all := diag.All()
	if len(all) == 0 {
		t.Fatal("no codes are registered")
	}
	anchors := map[string]diag.Code{}
	for _, e := range all {
		if got := e.Code.String(); !regexp.MustCompile(`^BLK[1-9]\d{3}$`).MatchString(got) {
			t.Errorf("%d renders as %q, which is not a published code", int(e.Code), got)
		}
		// A summary is a heading. It has to read as one: no trailing stop, no
		// capital that turns it into a sentence fragment mid-table.
		if strings.HasSuffix(e.Summary, ".") {
			t.Errorf("%s: the summary ends in a full stop: %q", e.Code, e.Summary)
		}
		for _, field := range []struct{ name, text string }{
			{"detail", e.Detail}, {"fix", e.Fix},
		} {
			if !strings.HasSuffix(strings.TrimSpace(field.text), ".") {
				t.Errorf("%s: the %s is not a sentence: %q", e.Code, field.name, field.text)
			}
			if strings.Contains(field.text, "\n") {
				t.Errorf("%s: the %s spans lines; the registry stores one paragraph per field", e.Code, field.name)
			}
		}
		// Two entries sharing an anchor would make one of them unlinkable.
		anchor := anchorOf(e)
		if first, dup := anchors[anchor]; dup {
			t.Errorf("%s and %s both anchor at %q", first, e.Code, anchor)
		}
		anchors[anchor] = e.Code
	}
}

// A code a reader retypes from a terminal has to find its way back to the
// entry, however they spell it.
func TestLookupAcceptsEverySpelling(t *testing.T) {
	t.Parallel()

	want := diag.All()[0]
	for _, spelling := range []string{
		want.Code.String(),
		strings.ToLower(want.Code.String()),
		strings.TrimPrefix(want.Code.String(), diag.Prefix),
		"  " + want.Code.String() + "  ",
	} {
		got, ok := diag.Lookup(spelling)
		if !ok || got.Code != want.Code {
			t.Errorf("Lookup(%q) = %v, %v; want %s", spelling, got.Code, ok, want.Code)
		}
	}
	for _, spelling := range []string{"", "BLK", "nonsense", "BLK9999", "1"} {
		if _, ok := diag.Lookup(spelling); ok {
			t.Errorf("Lookup(%q) found an entry; it names no registered code", spelling)
		}
	}
}

// The code has to survive being wrapped: block adds context to an error as it
// travels up ("foundry: %w"), and the reader still needs to be told which
// problem they are looking at.
func TestCodeSurvivesWrapping(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("underlying")
	coded := diag.ChecksumMismatch.Errorf("checksum mismatch: %w", sentinel)
	wrapped := fmt.Errorf("foundry: %w", coded)

	if got := diag.Of(wrapped); got != diag.ChecksumMismatch {
		t.Errorf("Of(wrapped) = %s, want %s", got, diag.ChecksumMismatch)
	}
	if !errors.Is(wrapped, sentinel) {
		t.Error("wrapping through a coded error broke errors.Is")
	}
	if got, want := diag.Message(wrapped), "BLK3002: foundry: checksum mismatch: underlying"; got != want {
		t.Errorf("Message(wrapped) = %q, want %q", got, want)
	}
	if got := diag.Message(sentinel); got != "underlying" {
		t.Errorf("an uncoded error should print as itself, got %q", got)
	}
	if got := diag.Message(nil); got != "" {
		t.Errorf("Message(nil) = %q, want the empty string", got)
	}
}

// Wrap leaves an existing code alone: the site that named the problem owns it,
// and a message must never accumulate two.
func TestWrapKeepsTheInnerCode(t *testing.T) {
	t.Parallel()

	inner := diag.LockStale.Errorf("stale")
	if got := diag.Of(diag.Internal.Wrap(inner)); got != diag.LockStale {
		t.Errorf("Wrap replaced %s with %s", diag.LockStale, got)
	}
	if diag.Internal.Wrap(nil) != nil {
		t.Error("Wrap(nil) should stay nil")
	}
}

// doc/errors.md is generated, and a generated file that is committed is a file
// that goes stale. The published reference has to be what the registry says.
func TestErrorsDocMatchesTheRegistry(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "doc", "errors.md")
	got, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	if want := diag.Markdown(); string(got) != string(want) {
		t.Error("doc/errors.md is stale; run \"make doc\".")
	}
}

// Every registered code has to appear on the page, and the page must name no
// code that does not exist.
func TestErrorsDocListsEveryCode(t *testing.T) {
	t.Parallel()

	page := string(diag.Markdown())
	for _, e := range diag.All() {
		if !strings.Contains(page, e.Code.String()) {
			t.Errorf("%s is registered but does not appear on the page", e.Code)
		}
	}
	registered := map[string]bool{}
	for _, e := range diag.All() {
		registered[e.Code.String()] = true
	}
	for _, m := range regexp.MustCompile(`BLK\d{4}`).FindAllString(page, -1) {
		if !registered[m] {
			t.Errorf("the page names %s, which is not a registered code", m)
		}
	}
}

// anchorOf mirrors the anchor the generator emits, so the uniqueness check
// above does not depend on an unexported helper.
func anchorOf(e diag.Entry) string {
	var b strings.Builder
	for _, r := range strings.ToLower(fmt.Sprintf("%s — %s", e.Code, e.Summary)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// Lookup accepts the three spellings a person types — "BLK1005", "blk1005"
// and the bare "1005" — and nothing else. strconv.Atoi would also have taken
// "+1005" and "-1005", neither of which block has ever printed.
func TestLookupAcceptsOnlyTheSpellingsBlockPrints(t *testing.T) {
	t.Parallel()
	want := diag.LockStale
	for _, s := range []string{"BLK1005", "blk1005", "Blk1005", "1005", " 1005 ", "\t1005\n"} {
		e, ok := diag.Lookup(s)
		if !ok || e.Code != want {
			t.Errorf("Lookup(%q) = %v, %v; want %s", s, e.Code, ok, want)
		}
	}
	for _, s := range []string{"", "+1005", "-1005", "1005.0", "1_005", "0x3ed", "BLK+1005", "BLK", "BLK1005x", "１００５"} {
		if e, ok := diag.Lookup(s); ok {
			t.Errorf("Lookup(%q) = %v, want no match", s, e.Code)
		}
	}
}
