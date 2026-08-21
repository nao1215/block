package version

import (
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
		{in: "", wantErr: true},
		{in: "v1.7.4", wantErr: true},
		{in: "1.7", wantErr: true},
		{in: "1.7.4.1", wantErr: true},
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
			if !tt.wantErr && got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestStringRoundTrip(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"1.7.4", "0.1.0", "2.0.0-beta.2"} {
		if got := MustParse(s).String(); got != s {
			t.Errorf("String() = %q, want %q", got, s)
		}
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
