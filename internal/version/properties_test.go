package version

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/mod/semver"
	"pgregory.net/rapid"
)

// semverID draws one pre-release identifier in semver's own grammar: a
// numeric identifier without a leading zero, or an alphanumeric one that may
// carry a hyphen. Numbers are sometimes longer than an int to reach the
// comparison by length.
func semverID() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.Custom(func(t *rapid.T) string {
			return strconv.Itoa(rapid.IntRange(0, 30).Draw(t, "n"))
		}),
		rapid.StringMatching(`[1-9][0-9]{18,22}`),
		rapid.StringMatching(`[0-9A-Za-z-]{0,4}[A-Za-z-][0-9A-Za-z-]{0,4}`),
	)
}

// semverText draws a strict semver version without build metadata, with small
// numbers so that equal cores and shared pre-release prefixes are common.
func semverText() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		core := strconv.Itoa(rapid.IntRange(0, 3).Draw(t, "major")) + "." +
			strconv.Itoa(rapid.IntRange(0, 3).Draw(t, "minor")) + "." +
			strconv.Itoa(rapid.IntRange(0, 3).Draw(t, "patch"))
		ids := rapid.SliceOfN(semverID(), 0, 3).Draw(t, "pre")
		if len(ids) == 0 {
			return core
		}
		return core + "-" + strings.Join(ids, ".")
	})
}

// TestDifferentialCompareMatchesXModSemver compares Compare with
// golang.org/x/mod/semver, the Go team's implementation of the same
// precedence rules, on strict semver input. block accepts more spellings than
// semver ("29.0", "29.1rc1"), so only what both accept is compared.
func TestDifferentialCompareMatchesXModSemver(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a, b := semverText().Draw(t, "a"), semverText().Draw(t, "b")
		if !semver.IsValid("v"+a) || !semver.IsValid("v"+b) {
			t.Skip("not strict semver")
		}
		va, err := Parse(a)
		if err != nil {
			t.Fatalf("Parse(%q): %v", a, err)
		}
		vb, err := Parse(b)
		if err != nil {
			t.Fatalf("Parse(%q): %v", b, err)
		}
		if got, want := Compare(va, vb), semver.Compare("v"+a, "v"+b); got != want {
			t.Fatalf("Compare(%s, %s) = %d, x/mod/semver says %d", a, b, got, want)
		}
	})
}

// TestPBTParseKeepsTheSpelling checks that String gives back exactly the text
// Parse read, which is what URLs, directory names and block.lock are built
// from.
func TestPBTParseKeepsTheSpelling(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := semverText().Draw(t, "v")
		v, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q): %v", s, err)
		}
		if v.String() != s {
			t.Fatalf("Parse(%q).String() = %q", s, v.String())
		}
		again, err := Parse(v.String())
		if err != nil || !Equal(again, v) {
			t.Fatalf("Parse(String()) of %q = %v, %v", s, again, err)
		}
	})
}

// TestPBTCompareIsATotalPreorder checks the laws Sort and Latest rely on:
// Compare is antisymmetric and transitive, and Equal agrees with it.
func TestPBTCompareIsATotalPreorder(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a := MustParse(semverText().Draw(t, "a"))
		b := MustParse(semverText().Draw(t, "b"))
		c := MustParse(semverText().Draw(t, "c"))
		if Compare(a, b) != -Compare(b, a) {
			t.Fatalf("Compare(%s, %s) = %d but Compare(%s, %s) = %d", a, b, Compare(a, b), b, a, Compare(b, a))
		}
		if Equal(a, b) != (Compare(a, b) == 0) {
			t.Fatalf("Equal(%s, %s) disagrees with Compare", a, b)
		}
		if Compare(a, b) <= 0 && Compare(b, c) <= 0 && Compare(a, c) > 0 {
			t.Fatalf("%s <= %s <= %s but Compare(%s, %s) = %d", a, b, c, a, c, Compare(a, c))
		}
	})
}

// TestPBTLatestIsTheMaximumThatMatches checks Latest against its definition:
// the answer matches the constraint, nothing that matches is higher, and the
// order the tags arrive in does not change the answer.
func TestPBTLatestIsTheMaximumThatMatches(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		texts := rapid.SliceOfN(semverText(), 1, 12).Draw(t, "tags")
		vs := make([]Version, len(texts))
		for i, s := range texts {
			vs[i] = MustParse(s)
		}
		pick := vs[rapid.IntRange(0, len(vs)-1).Draw(t, "pick")]
		prefix := rapid.IntRange(1, 3).Draw(t, "parts")
		parts := []int{pick.Major, pick.Minor, pick.Patch}[:prefix]
		strs := make([]string, len(parts))
		for i, p := range parts {
			strs[i] = strconv.Itoa(p)
		}
		c := MustParseConstraint(strings.Join(strs, "."))

		got, ok := Latest(vs, c)
		matching := slices.ContainsFunc(vs, c.Matches)
		if ok != matching {
			t.Fatalf("Latest found=%v, but some tag matches %q: %v", ok, c, matching)
		}
		if !ok {
			return
		}
		if !c.Matches(got) {
			t.Fatalf("Latest(%v, %q) = %s, which does not match", vs, c, got)
		}
		for _, v := range vs {
			if c.Matches(v) && Compare(v, got) > 0 {
				t.Fatalf("Latest(%v, %q) = %s, but %s matches and is higher", vs, c, got, v)
			}
		}
		shuffled := slices.Clone(vs)
		slices.Reverse(shuffled)
		if again, _ := Latest(shuffled, c); again.String() != got.String() {
			t.Fatalf("Latest depends on the order of the tags: %s, then %s", got, again)
		}
	})
}
