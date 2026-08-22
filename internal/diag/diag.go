// Package diag defines block's diagnostic codes: the stable, searchable names
// for the refusals block reports.
//
// A message alone is a dead end. "block.lock is stale" cannot be searched for,
// linked to, or branched on, and rewording it next release breaks every
// bookmark and every script that matched on the prose. A code survives
// rewording, so the message stays free to improve.
//
// A code is "BLK" followed by four digits, and the thousands digit says which
// kind of problem it is — which is to say, what the reader has to go and fix:
//
//	BLK1xxx  the project's own files: block.toml and block.lock
//	BLK2xxx  resolving a version against an upstream
//	BLK3xxx  downloading an artifact and proving it is the locked one
//	BLK4xxx  installing into the store
//	BLK5xxx  running a command with the locked toolchain
//	BLK6xxx  a refusal on security grounds
//	BLK9xxx  an internal error — a bug in block
//
// The digit is not the exit status: every coded diagnostic exits 1. block's
// other non-zero exit, 2 from `block lock --check`, is a result rather than an
// error — the lockfile would change — and carries no code, because there is
// nothing to look up.
//
// Codes are grouped by what the reader has to fix, never one per call site.
// Two errors share a code when the fix is the same and differ when it differs,
// so the published reference reads as a list of problems a person can act on.
//
// # Why a code cannot go missing
//
// Codes are not constants that a registry then describes; they are what
// registering returns. [register] is the only way to obtain a [Code], and it
// refuses an entry that omits any of the text the published reference needs. A
// code with no documentation is therefore not something to test for — it is
// unconstructable.
package diag

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Prefix is the letters every code starts with.
const Prefix = "BLK"

// Code is one diagnostic's stable identity. The zero value is not a valid
// code; every Code comes from [register], which is unexported, so the set of
// codes is closed and each member is documented by construction.
type Code int

// digits is how many digits a code carries after [Prefix].
const digits = 4

// String renders the code in its published form, e.g. "BLK1005".
func (c Code) String() string { return fmt.Sprintf("%s%0*d", Prefix, digits, int(c)) }

// familySpan is how many codes a family holds, and so the divisor that
// recovers the family from a code.
const familySpan = 1000

// Family is the kind of problem a code names: its thousands digit.
func (c Code) Family() int { return int(c) / familySpan }

// Error is an error carrying a code. The code is deliberately not part of
// [Error.Error]: it is printed once, at the front of the line the user sees,
// rather than buried wherever in a wrapped chain the problem was named.
type Error struct {
	code Code
	err  error
}

func (e *Error) Error() string { return e.err.Error() }

// Unwrap exposes the wrapped error so errors.Is and errors.As keep working
// through a coded error.
func (e *Error) Unwrap() error { return e.err }

// Code is the diagnostic's code.
func (e *Error) Code() Code { return e.code }

// Errorf builds a coded error. It forwards to [fmt.Errorf], so a %w in the
// format still wraps.
//
// Only the site that names the problem carries a code. A wrapper that adds
// context to someone else's error — "%s: %w" — leaves the code to the error it
// wraps, so a message never accumulates two of them.
func (c Code) Errorf(format string, args ...any) error {
	return &Error{code: c, err: fmt.Errorf(format, args...)}
}

// Wrap attaches a code to an existing error, for the sites that already build
// one. An error that already carries a code keeps it.
func (c Code) Wrap(err error) error {
	if err == nil {
		return nil
	}
	if Of(err) != 0 {
		return err
	}
	return &Error{code: c, err: err}
}

// Coder is an error that names its own diagnostic. [Error] is the general
// implementation, used wherever a code is attached at the point the problem is
// named; a package with an error type of its own — one callers match with
// errors.As — implements it directly instead, so the type keeps its identity
// and still says which code it is published under.
type Coder interface {
	error
	Code() Code
}

// Of returns the code an error carries, or 0 when it carries none. The
// outermost code in a wrapped chain wins: it is the one whose message the
// reader is looking at.
func Of(err error) Code {
	var c Coder
	if errors.As(err, &c) {
		return c.Code()
	}
	return 0
}

// Message renders an error the way block prints it: the code, when the error
// carries one, in front of the sentence rather than instead of it.
func Message(err error) string {
	if err == nil {
		return ""
	}
	if c := Of(err); c != 0 {
		return c.String() + ": " + err.Error()
	}
	return err.Error()
}

// Entry is a code's published documentation.
type Entry struct {
	// Code is the diagnostic's identity.
	Code Code
	// Summary is the one-line title the reference lists it under.
	Summary string
	// Detail says what block observed, in prose.
	Detail string
	// Fix says what the reader should do about it.
	Fix string
	// Since is the block version the code first shipped in.
	Since string
}

// registry holds every registered entry, keyed by code.
var registry = map[Code]Entry{} //nolint:gochecknoglobals // the closed set of codes

// register records a code's documentation and returns the code. It is the only
// way to obtain a [Code], so a code that is not documented cannot exist.
//
// It panics rather than returning an error: every call is a package-level
// variable initialiser, so a mistake here is a build-time mistake in every
// sense that matters, and there is no caller to hand an error back to.
// Every code registered so far shipped in the same release, which is why
// since looks constant; it is a parameter because the next one will not.
func register(code int, summary, detail, fix, since string) Code { //nolint:unparam // see above
	c := Code(code)
	switch {
	case c.Family() < 1 || c.Family() > 9:
		panic(fmt.Sprintf("diag: %d is outside the code families", code))
	case strings.TrimSpace(summary) == "":
		panic(fmt.Sprintf("diag: %s has no summary", c))
	case strings.TrimSpace(detail) == "":
		panic(fmt.Sprintf("diag: %s has no detail", c))
	case strings.TrimSpace(fix) == "":
		panic(fmt.Sprintf("diag: %s has no fix", c))
	case strings.TrimSpace(since) == "":
		panic(fmt.Sprintf("diag: %s has no since", c))
	}
	if _, dup := registry[c]; dup {
		panic(fmt.Sprintf("diag: %s is registered twice", c))
	}
	registry[c] = Entry{Code: c, Summary: summary, Detail: detail, Fix: fix, Since: since}
	return c
}

// All returns every registered entry, in code order.
func All() []Entry {
	out := make([]Entry, 0, len(registry))
	for _, e := range registry {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Lookup finds an entry by its printed form, however it is spelled: "BLK1005",
// "blk1005" or the bare "1005" a person retypes from a terminal. The prefix is
// matched without regard to case on purpose — someone reading a code off a
// terminal and typing it back has no reason to be held to the capitalisation —
// while the digits are exact, because they are the code itself.
func Lookup(s string) (Entry, bool) {
	text := strings.TrimSpace(s)
	if len(text) > len(Prefix) && strings.EqualFold(text[:len(Prefix)], Prefix) {
		text = text[len(Prefix):]
	}
	// Digits and nothing else: strconv.Atoi would take "+1005" and "-1005",
	// and neither is a spelling of a code anyone has ever seen printed.
	if text == "" || strings.ContainsFunc(text, func(r rune) bool { return r < '0' || r > '9' }) {
		return Entry{}, false
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return Entry{}, false
	}
	e, ok := registry[Code(n)]
	return e, ok
}
