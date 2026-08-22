package diag

import (
	"strings"
	"testing"
)

// register is the only way to obtain a [Code], so its refusals are what keep
// the published reference honest: a code that is documented, unique, and
// inside a family. They are panics because every call is a package-level
// initialiser, and a test is the only place they can be observed.
func TestRegisterRefusesACodeTheReferenceCouldNotPublish(t *testing.T) {
	t.Parallel()
	const (
		summary = "a summary"
		detail  = "a detail"
		fix     = "a fix"
		since   = "v0.1.0"
	)
	tests := []struct {
		name string
		call func()
		want string
	}{
		{"a code already registered", func() { register(int(LockStale), summary, detail, fix, since) }, "registered twice"},
		{"a code below the families", func() { register(999, summary, detail, fix, since) }, "outside the code families"},
		{"a code above the families", func() { register(10001, summary, detail, fix, since) }, "outside the code families"},
		{"no summary", func() { register(1900, " ", detail, fix, since) }, "has no summary"},
		{"no detail", func() { register(1901, summary, "", fix, since) }, "has no detail"},
		{"no fix", func() { register(1902, summary, detail, "\t", since) }, "has no fix"},
		{"no since", func() { register(1903, summary, detail, fix, "") }, "has no since"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				got, ok := recover().(string)
				switch {
				case !ok:
					t.Fatalf("register returned instead of panicking")
				case !strings.Contains(got, tt.want):
					t.Errorf("panic %q, want it to mention %q", got, tt.want)
				}
			}()
			tt.call()
		})
	}
}
