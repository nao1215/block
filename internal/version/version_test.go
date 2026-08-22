package version

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    Version
		wantErr bool
	}{
		{in: "1.7.4", want: Version{Major: 1, Minor: 7, Patch: 4}},
		{in: "0.0.0", want: Version{}},
		{in: "1.8.0-rc1", want: Version{Major: 1, Minor: 8, Pre: "rc1"}},
		{in: "1.8.0-rc.1+build.5", want: Version{Major: 1, Minor: 8, Pre: "rc.1"}},
		{in: "1.8.0+build", want: Version{Major: 1, Minor: 8}},
		// Bitcoin Core style: two components and a bare pre-release suffix.
		{in: "29.0", want: Version{Major: 29}},
		{in: "29.1rc1", want: Version{Major: 29, Minor: 1, Pre: "rc1"}},
		{in: "1.7.4rc2", want: Version{Major: 1, Minor: 7, Patch: 4, Pre: "rc2"}},
		{in: "", wantErr: true},
		{in: "v1.7.4", wantErr: true},
		{in: "1", wantErr: true},
		{in: "1.7.4.1", wantErr: true},
		{in: "1.7rc1-rc1", wantErr: true},
		{in: "1.7.", wantErr: true},
		{in: "1.07.4", wantErr: true},
		{in: "1.x.4", wantErr: true},
		{in: "1..4", wantErr: true},
		{in: "99999999999999999999.0.0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			got.text = ""
			if !tt.wantErr && got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestStringRoundTrip(t *testing.T) {
	t.Parallel()
	// The upstream spelling is preserved, so URLs render the way the upstream
	// names its files ("29.0", not "29.0.0").
	for _, s := range []string{"1.7.4", "0.1.0", "2.0.0-beta.2", "29.0", "29.1rc1"} {
		if got := MustParse(s).String(); got != s {
			t.Errorf("String() = %q, want %q", got, s)
		}
	}
	if got := (Version{Major: 29}).String(); got != "29.0.0" {
		t.Errorf("constructed String() = %q", got)
	}
	if !Equal(MustParse("29.0"), MustParse("29.0.0")) || Equal(MustParse("29.0"), MustParse("29.1")) {
		t.Error("Equal is wrong")
	}
}

func TestMustParsePanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("MustParse did not panic")
		}
	}()
	MustParse("nope")
}

func TestCompare(t *testing.T) {
	t.Parallel()
	ordered := []string{
		"0.9.9", "1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
		"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0",
	}
	for i := range ordered {
		for j := range ordered {
			a, b := MustParse(ordered[i]), MustParse(ordered[j])
			got := Compare(a, b)
			want := 0
			switch {
			case i < j:
				want = -1
			case i > j:
				want = 1
			}
			if got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", a, b, got, want)
			}
		}
	}
}

func TestSort(t *testing.T) {
	t.Parallel()
	vs := []Version{MustParse("1.7.4"), MustParse("1.7.10"), MustParse("1.7.1-rc1"), MustParse("0.2.0")}
	Sort(vs)
	want := []string{"0.2.0", "1.7.1-rc1", "1.7.4", "1.7.10"}
	for i, w := range want {
		if vs[i].String() != w {
			t.Fatalf("Sort() = %v, want %v", vs, want)
		}
	}
}

func TestParseConstraint(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"1", "1.7", "1.7.4", "0"} {
		c, err := ParseConstraint(ok)
		if err != nil {
			t.Errorf("ParseConstraint(%q) error = %v", ok, err)
		}
		if c.String() != ok {
			t.Errorf("String() = %q, want %q", c.String(), ok)
		}
	}
	for _, bad := range []string{"", "^1.7", "~1.7", ">=1", "1.7.x", "1.*", "v1.7", "1.7.4.2", "1.7.4-rc1", "latest", "1.07"} {
		if _, err := ParseConstraint(bad); err == nil {
			t.Errorf("ParseConstraint(%q) accepted", bad)
		}
	}
	if !MustParseConstraint("1.7.4").IsExact() || MustParseConstraint("1.7").IsExact() {
		t.Error("IsExact is wrong")
	}
	if !(Constraint{}).IsZero() || MustParseConstraint("1").IsZero() {
		t.Error("IsZero is wrong")
	}
}

func TestMustParseConstraintPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("MustParseConstraint did not panic")
		}
	}()
	MustParseConstraint("^1")
}

func TestConstraintMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		c, v string
		want bool
	}{
		{"1", "1.0.0", true},
		{"1", "1.99.3", true},
		{"1", "2.0.0", false},
		{"1", "0.9.0", false},
		{"1.7", "1.7.0", true},
		{"1.7", "1.7.12", true},
		{"1.7", "1.8.0", false},
		{"1.7", "1.6.9", false},
		{"1.7.4", "1.7.4", true},
		{"1.7.4", "1.7.5", false},
		{"1.7", "1.7.5-rc1", false},
		{"1", "1.8.0-rc1", false},
		{"1.8.0", "1.8.0-rc1", false},
		{"29", "29.0", true},
		{"29.0", "29.0", true},
		{"29.0.0", "29.0", true},
		{"29", "29.1rc1", false},
	}
	for _, tt := range tests {
		if got := MustParseConstraint(tt.c).Matches(MustParse(tt.v)); got != tt.want {
			t.Errorf("%q.Matches(%s) = %v, want %v", tt.c, tt.v, got, tt.want)
		}
	}
	if (Constraint{}).Matches(MustParse("1.0.0")) {
		t.Error("zero constraint must match nothing")
	}
}

func TestLatest(t *testing.T) {
	t.Parallel()
	vs := []Version{MustParse("1.6.0"), MustParse("1.7.1"), MustParse("1.7.0"), MustParse("1.8.0-rc1"), MustParse("2.0.0")}
	tests := []struct {
		c    string
		want string
		ok   bool
	}{
		{"1", "1.7.1", true},
		{"1.7", "1.7.1", true},
		{"1.7.0", "1.7.0", true},
		{"2", "2.0.0", true},
		{"1.8", "", false},
		{"3", "", false},
	}
	for _, tt := range tests {
		got, ok := Latest(vs, MustParseConstraint(tt.c))
		if ok != tt.ok || (ok && got.String() != tt.want) {
			t.Errorf("Latest(%q) = %s, %v; want %s, %v", tt.c, got, ok, tt.want, tt.ok)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"1.7.4", "1.8.0-rc1", "", "v1", "1..2", "1.7.4+meta"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		v, err := Parse(s)
		if err != nil {
			return
		}
		again, err := Parse(v.String())
		if err != nil {
			t.Fatalf("Parse(%q).String() = %q does not parse: %v", s, v.String(), err)
		}
		if Compare(v, again) != 0 {
			t.Fatalf("round trip changed %q: %v vs %v", s, v, again)
		}
	})
}

// A version becomes a directory name under $BLOCK_HOME, and block.lock
// arrives through pull requests and hand edits. Everything below parsed before
// the alphabet was closed, and "1.7/../../outside" reached filepath.Join.
func TestParseRefusesAnythingThatCouldBeAPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
	}{
		{"unix separator", "1.7/../../outside"},
		{"unix separator in the pre-release", "1.7.0-rc/../../outside"},
		{"windows separator", `1.7\..\..\outside`},
		{"windows separator in the pre-release", `1.7.0-rc\..\outside`},
		{"bare unix separator", "1.7/x"},
		{"bare windows separator", `1.7\x`},
		{"parent directory", "1.7.0-.."},
		{"parent directory alone", ".."},
		{"trailing dot", "1.7.0-rc."},
		{"leading dot in the pre-release", "1.7.0-.rc"},
		{"empty pre-release identifier", "1.7.0-a..b"},
		{"empty build identifier", "1.7.0+a..b"},
		{"empty build metadata", "1.7.0+"},
		{"NUL", "1.7.0-rc\x00"},
		{"newline", "1.7.0-rc\nx"},
		{"carriage return", "1.7.0-rc\rx"},
		{"tab", "1.7.0-rc\tx"},
		{"escape", "1.7.0-rc\x1b[2J"},
		{"space", "1.7.0-rc 1"},
		{"absolute path", "/etc/passwd"},
		{"windows drive", `C:\windows`},
		{"colon", "1.7.0-rc:1"},
		{"tilde", "1.7.0-~"},
		{"non-ascii digit", "1.7.٠"},
		{"non-ascii lookalike", "1.7.0-rcｰ1"},
		{"shell metacharacter", "1.7.0-rc;rm"},
		{"quote", `1.7.0-rc"`},
		{"star", "1.7.0-rc*"},
		{"much too long", "1.7.0-" + strings.Repeat("a", 200)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v, err := Parse(tt.in)
			if err == nil {
				t.Fatalf("Parse(%q) = %q, want an error", tt.in, v)
			}
		})
	}
}

// The formats block already supported have to keep working: the alphabet was
// narrowed to stop paths, not to stop upstreams from spelling versions their
// own way.
func TestParseKeepsTheFormatsUpstreamsUse(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"1.2.3",          // semver
		"29.0",           // two components, Bitcoin Core
		"1.2.3-rc.1",     // dotted pre-release
		"29.1rc1",        // bare suffix, Bitcoin Core
		"1.2.3+build.5",  // build metadata
		"1.2.3-rc.1+b.2", // both
		"0.8.28",         // solc
		"1.0.0-alpha",    // single-identifier pre-release
		"2.0.0-beta.1",   // CometBFT
		"1.7.0-rc_1",     // underscore, which semver allows in an identifier
	} {
		v, err := Parse(s)
		if err != nil {
			t.Errorf("Parse(%q): %v", s, err)
			continue
		}
		if v.String() != s {
			t.Errorf("Parse(%q).String() = %q, want the original spelling", s, v.String())
		}
	}
}

// A hyphen announces a pre-release. One that names nothing used to parse as
// the release before it while keeping the hyphen in its spelling, so "1.7.4-"
// satisfied the constraint "1.7.4" and then became the directory 1.7.4--…
// under $BLOCK_HOME. An empty build field was already refused; the
// pre-release is held to the same rule.
func TestParseRefusesAnEmptyPreRelease(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"1.7.4-", "1.7-", "1.7.4-+b", "1.7.4-+"} {
		if v, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) = %q, want an error", s, v)
		}
	}
}

func FuzzParseConstraint(f *testing.F) {
	for _, s := range []string{"1", "1.7", "1.7.4", "", "01", "1.", ".1", "1.7.4.1", "v1", "1.x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := ParseConstraint(s)
		if err != nil {
			if !c.IsZero() {
				t.Fatalf("ParseConstraint(%q) failed but returned a non-zero constraint %q", s, c)
			}
			return
		}
		if c.IsZero() || c.String() != s {
			t.Fatalf("ParseConstraint(%q) = %q", s, c)
		}
		// A constraint is a prefix of the versions it admits: the version
		// written the same way (padded to three components) must match, and
		// so must the same numbers with a different patch unless the
		// constraint pins it.
		again, err := ParseConstraint(c.String())
		if err != nil {
			t.Fatalf("round trip of %q failed: %v", s, err)
		}
		exact := strings.Split(s, ".")
		for len(exact) < components {
			exact = append(exact, "0")
		}
		v, err := Parse(strings.Join(exact, "."))
		if err != nil {
			t.Fatalf("constraint %q parsed but %q is not a version: %v", s, strings.Join(exact, "."), err)
		}
		if !c.Matches(v) || !again.Matches(v) {
			t.Fatalf("constraint %q does not match %s", s, v)
		}
		pre := v
		pre.Pre = "rc1"
		if c.Matches(pre) {
			t.Fatalf("constraint %q matched a pre-release", s)
		}
		other := v
		other.Major++
		if c.Matches(other) {
			t.Fatalf("constraint %q matched %s, a different major", s, other)
		}
	})
}

// An upstream can publish two tags that are one version to semver: "1.2" and
// "1.2.0", or a release and the same release carrying build metadata. Sorting
// left their order to whatever order the tags arrived in, so which tag lock
// pinned — a different URL and a different lockfile — depended on the order
// GitHub happened to answer in.
func TestSortAndLatestDoNotDependOnTheOrderTagsArriveIn(t *testing.T) {
	t.Parallel()
	tags := []string{"1.2.0", "1.2.3+b1", "1.2.1", "1.2.3", "1.2.2", "1.2.3+b2", "1.2"}
	c := MustParseConstraint("1.2")
	var want string
	for rotate := range tags {
		vs := make([]Version, 0, len(tags))
		for i := range tags {
			vs = append(vs, MustParse(tags[(i+rotate)%len(tags)]))
		}
		// Latest is asked before the list is sorted as well as after it:
		// sorting is what resolution does, but Latest is exported on its own
		// and its answer must not depend on the order either.
		unsorted, ok := Latest(vs, c)
		if !ok {
			t.Fatalf("rotation %d: nothing matched %q", rotate, c)
		}
		Sort(vs)
		got, ok := Latest(vs, c)
		if !ok {
			t.Fatalf("rotation %d: nothing matched %q once sorted", rotate, c)
		}
		if unsorted.String() != got.String() {
			t.Errorf("rotation %d: Latest = %q unsorted and %q sorted", rotate, unsorted, got)
		}
		if rotate == 0 {
			want = got.String()
			continue
		}
		if got.String() != want {
			t.Errorf("rotation %d pinned %q, rotation 0 pinned %q", rotate, got, want)
		}
	}
	// The plain spelling is the one that gets pinned: build metadata is not
	// part of precedence, and a lockfile has to name one tag.
	if want != "1.2.3" {
		t.Errorf("Latest = %q, want the release without build metadata", want)
	}
	// And sorting is a fixed point: sorting an already sorted list, or a
	// reversed one, gives the same sequence.
	first := make([]Version, 0, len(tags))
	for _, s := range tags {
		first = append(first, MustParse(s))
	}
	Sort(first)
	reversed := make([]Version, len(first))
	for i, v := range first {
		reversed[len(first)-1-i] = v
	}
	Sort(reversed)
	for i := range first {
		if first[i].String() != reversed[i].String() {
			t.Fatalf("sorting is not deterministic: %v vs %v", first, reversed)
		}
	}
}
