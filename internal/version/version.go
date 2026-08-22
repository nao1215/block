// Package version parses the semantic versions that upstream tools publish and
// the constraints a block.toml declares against them.
//
// block deliberately supports a tiny constraint syntax, in three forms:
//
//   - a dotted prefix: "1" means any 1.x.y, "1.7" means any 1.7.y and "1.7.4"
//     is exact. A pre-release never satisfies one of these, so a project that
//     asks for "1.7" never silently receives 1.7.0-rc1;
//   - an exact pre-release: "1.8.0-rc1" is that release and nothing else. A
//     pre-release is named, never floated onto;
//   - a channel: "nightly" is a release line an upstream publishes under a tag
//     that moves. What a channel resolves to is decided by the resolver, not
//     here, because a moving tag has to be turned into something that does not
//     move before it reaches a lockfile.
//
// Upstreams are not uniformly semver: Bitcoin Core tags "v29.0" and "v29.1rc1".
// Parse accepts two components (patch 0) and a bare alphabetic pre-release
// suffix, and String keeps the original spelling so that URLs and lockfiles
// show the version the way the upstream writes it.
package version

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// components is the number of release components in MAJOR.MINOR.PATCH.
const components = 3

// Version is a parsed version without build metadata.
type Version struct {
	Major int
	Minor int
	Patch int
	// Pre holds the pre-release identifiers, or "" for a release.
	Pre string
	// text is the spelling the version was parsed from ("29.0", "1.7.4").
	text string
}

// maxVersion bounds the whole string. Upstream versions are short; anything
// longer is a lockfile someone is playing with, and a path component has
// limits of its own.
const maxVersion = 128

// Parse parses "MAJOR.MINOR[.PATCH][-PRE]". A bare alphabetic suffix such as
// "rc1" in "29.1rc1" is a pre-release too. A leading "v" is not accepted
// here; stripping the tag prefix is the recipe's job, not the parser's.
//
// The accepted alphabet is closed, and deliberately so: a parsed version
// becomes a directory name under $BLOCK_HOME, and block.lock arrives through
// pull requests and hand edits. A version carrying a separator, a "..", a NUL
// or a control character is refused here rather than defended against
// downstream — see [Version.String], whose result the store joins onto a path.
func Parse(s string) (Version, error) {
	var v Version
	if s == "" {
		return v, errors.New("version is empty")
	}
	if len(s) > maxVersion {
		return v, fmt.Errorf("invalid version: %d characters is longer than the %d a version may be", len(s), maxVersion)
	}
	if err := checkAlphabet(s); err != nil {
		return v, err
	}
	// Build metadata is not part of ordering, but it is part of the spelling
	// String returns, so it is split off first and held to the same shape as
	// the pre-release rather than skipped.
	rest, build, hasBuild := strings.Cut(s, "+")
	core, pre, hasPre := strings.Cut(rest, "-")
	// "1.7.4-" is not a release spelled oddly: a hyphen announces a
	// pre-release, and one that names nothing is refused the same way an
	// empty build field is, or it would parse as 1.7.4 and still carry the
	// stray hyphen into every URL and directory name.
	if hasPre && pre == "" {
		return v, fmt.Errorf("invalid version %q: empty pre-release", s)
	}
	// "29.1rc1": split the bare suffix off the last numeric component.
	if i := strings.IndexFunc(core, func(r rune) bool { return r != '.' && (r < '0' || r > '9') }); i >= 0 {
		if pre != "" {
			return v, fmt.Errorf("invalid version %q: two pre-release suffixes", s)
		}
		core, pre = core[:i], core[i:]
	}
	parts := strings.Split(core, ".")
	if len(parts) < components-1 || len(parts) > components {
		return v, fmt.Errorf("invalid version %q: want MAJOR.MINOR[.PATCH]", s)
	}
	nums := make([]int, components)
	for i, p := range parts {
		n, err := parseNumber(p)
		if err != nil {
			return v, fmt.Errorf("invalid version %q: %w", s, err)
		}
		nums[i] = n
	}
	if pre != "" {
		if err := checkIdentifiers("pre-release", pre); err != nil {
			return v, fmt.Errorf("invalid version %q: %w", s, err)
		}
	}
	if hasBuild {
		if err := checkIdentifiers("build metadata", build); err != nil {
			return v, fmt.Errorf("invalid version %q: %w", s, err)
		}
	}
	v.Major, v.Minor, v.Patch, v.Pre, v.text = nums[0], nums[1], nums[2], pre, s
	return v, nil
}

// checkAlphabet rejects every byte a version may not contain. Only ASCII
// alphanumerics and the four characters semver itself uses are allowed, which
// leaves no way to write a path separator, a NUL, a control character, a
// space, a shell metacharacter or a non-ASCII lookalike.
func checkAlphabet(s string) error {
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= '0' && c <= '9',
			c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c == '.', c == '-', c == '+', c == '_':
		default:
			return fmt.Errorf("invalid version %q: %q is not allowed in a version", s, string(rune(c)))
		}
	}
	return nil
}

// checkIdentifiers holds a pre-release or a build-metadata field to semver's
// own shape: dot-separated identifiers, each non-empty. It is what stops "..",
// a leading "." and a trailing "." from reaching a directory name, since a dot
// is a legal separator but an empty identifier is not.
func checkIdentifiers(what, field string) error {
	if field == "" {
		return fmt.Errorf("empty %s", what)
	}
	for _, id := range strings.Split(field, ".") {
		if id == "" {
			return fmt.Errorf("empty %s identifier", what)
		}
	}
	return nil
}

// MustParse is Parse for literals in tests and tables; it panics on error.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

func parseNumber(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty component")
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("component %q has a leading zero", s)
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("component %q is not a number", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("component %q is out of range", s)
	}
	return n, nil
}

// String renders the version as it was written upstream, or canonically as
// "MAJOR.MINOR.PATCH[-PRE]" for a constructed value.
func (v Version) String() string {
	if v.text != "" {
		return v.text
	}
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Equal reports whether two versions denote the same release regardless of
// spelling.
func Equal(a, b Version) bool { return Compare(a, b) == 0 }

// IsPrerelease reports whether the version carries pre-release identifiers.
func (v Version) IsPrerelease() bool { return v.Pre != "" }

// Compare orders two versions per semver: release parts numerically, then a
// release sorts after any pre-release of the same core, then pre-release
// identifiers dot-by-dot (numeric before alphanumeric).
func Compare(a, b Version) int {
	switch {
	case a.Major != b.Major:
		return cmpInt(a.Major, b.Major)
	case a.Minor != b.Minor:
		return cmpInt(a.Minor, b.Minor)
	case a.Patch != b.Patch:
		return cmpInt(a.Patch, b.Patch)
	case a.Pre == b.Pre:
		return 0
	case a.Pre == "":
		return 1
	case b.Pre == "":
		return -1
	}
	return comparePre(a.Pre, b.Pre)
}

func comparePre(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				return cmpInt(an, bn)
			}
		case aerr == nil:
			return -1
		case berr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(as), len(bs))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Sort orders versions ascending in place.
//
// The order is total, which [Compare] on its own is not: semver says build
// metadata does not affect precedence, and block additionally accepts "1.2"
// for "1.2.0", so an upstream can publish two tags that compare equal. Sorting
// those with an unstable sort would leave their order to whatever order the
// tags arrived in, and [Latest] would then pin a different tag — a different
// URL, a different lockfile — for the same repository on two runs. Ties are
// therefore broken by the spelling; see [compareTotal].
func Sort(vs []Version) {
	sort.Slice(vs, func(i, j int) bool { return compareTotal(vs[i], vs[j]) < 0 })
}

// compareTotal is [Compare] with its ties broken, so that no two distinct
// spellings are interchangeable. A release and the same release carrying build
// metadata are one release to semver, but they are two tags upstream and only
// one of them can be pinned: the plain spelling wins, and two metadata
// spellings are ordered against each other. Everything else falls back to the
// spelling, which is what separates "1.2" from "1.2.0".
func compareTotal(a, b Version) int {
	if c := Compare(a, b); c != 0 {
		return c
	}
	ab, bb := a.buildMetadata(), b.buildMetadata()
	switch {
	case ab == "" && bb != "":
		return 1
	case ab != "" && bb == "":
		return -1
	}
	return strings.Compare(a.String(), b.String())
}

// buildMetadata is the "+..." part of the spelling this version was parsed
// from, or "" for one written without any.
func (v Version) buildMetadata() string {
	_, build, _ := strings.Cut(v.text, "+")
	return build
}

// Constraint is what block.toml declares for a tool: a dotted version prefix
// such as "1", "1.7" or "1.7.4", an exact pre-release such as "1.8.0-rc1", or
// a channel such as "nightly".
type Constraint struct {
	raw   string
	parts []int
	// pre is the pre-release a constraint names exactly, or "".
	pre string
	// channel is the release line a constraint floats on, or "".
	channel string
}

// ParseConstraint parses a constraint. Operators, ranges and wildcards are
// rejected so that block.toml stays trivially readable; what is accepted is a
// dotted release prefix, one of those with an exact pre-release after it, or a
// channel name.
func ParseConstraint(s string) (Constraint, error) {
	if s == "" {
		return Constraint{}, errors.New("version constraint is empty")
	}
	if len(s) > maxVersion {
		return Constraint{}, fmt.Errorf("invalid version constraint: %d characters is longer than the %d a constraint may be", len(s), maxVersion)
	}
	// A constraint that does not start with a digit is a channel: an upstream
	// spells its release lines in words ("nightly"), and its versions in
	// numbers, so the first character tells the two apart with nothing to
	// configure and no ambiguity to resolve.
	if s[0] < '0' || s[0] > '9' {
		if err := validateChannel(s); err != nil {
			return Constraint{}, err
		}
		return Constraint{raw: s, channel: s}, nil
	}
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) > components {
		return Constraint{}, fmt.Errorf("invalid version constraint %q: want at most MAJOR.MINOR.PATCH", s)
	}
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := parseNumber(p)
		if err != nil {
			return Constraint{}, fmt.Errorf("invalid version constraint %q: %w", s, err)
		}
		nums = append(nums, n)
	}
	if !hasPre {
		return Constraint{raw: s, parts: nums}, nil
	}
	// A pre-release is named, never floated onto: "1.8.0-rc1" is that release
	// and nothing else, so there is no way to write "the newest rc" and be
	// given one nobody chose.
	if len(nums) != components {
		return Constraint{}, fmt.Errorf("invalid version constraint %q: a pre-release is pinned exactly, so it needs MAJOR.MINOR.PATCH before it", s)
	}
	if err := checkIdentifiers("pre-release", pre); err != nil {
		return Constraint{}, fmt.Errorf("invalid version constraint %q: %w", s, err)
	}
	if err := checkAlphabet(s); err != nil {
		return Constraint{}, err
	}
	return Constraint{raw: s, parts: nums, pre: pre}, nil
}

// maxChannel bounds a channel name, which becomes part of a directory name
// under $BLOCK_HOME by way of the tag it resolves to.
const maxChannel = 32

// validateChannel holds a channel name to lower-case letters, digits and
// hyphens, starting with a letter. That is what upstreams call their release
// lines, and it leaves no way to write a path, a separator or something that
// could be read as a version.
func validateChannel(s string) error {
	// "v1" is a version somebody wrote the tag prefix into, not a channel.
	// Accepting it as one would trade a parse error for a resolution error
	// about a channel the upstream has never heard of.
	if len(s) > 1 && (s[0] == 'v' || s[0] == 'V') && s[1] >= '0' && s[1] <= '9' {
		return fmt.Errorf("invalid version constraint %q: write the version without the tag prefix, as %q", s, s[1:])
	}
	if len(s) > maxChannel {
		return fmt.Errorf("invalid channel %q: keep it under %d characters", s, maxChannel)
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case i > 0 && (c >= '0' && c <= '9' || c == '-'):
		default:
			return fmt.Errorf("invalid version constraint %q: want a version like \"1.7\" or a channel like \"nightly\" (lower-case letters, digits and '-')", s)
		}
	}
	return nil
}

// MustParseConstraint is ParseConstraint for literals; it panics on error.
func MustParseConstraint(s string) Constraint {
	c, err := ParseConstraint(s)
	if err != nil {
		panic(err)
	}
	return c
}

// String returns the constraint exactly as written in the manifest.
func (c Constraint) String() string { return c.raw }

// IsZero reports whether the constraint was never parsed.
func (c Constraint) IsZero() bool { return c.raw == "" }

// IsExact reports whether the constraint pins all three components.
func (c Constraint) IsExact() bool { return len(c.parts) == components }

// IsChannel reports whether the constraint names a release line rather than a
// version.
func (c Constraint) IsChannel() bool { return c.channel != "" }

// Channel is the release line the constraint floats on, or "".
func (c Constraint) Channel() string { return c.channel }

// Matches reports whether v satisfies the constraint. A pre-release matches
// only a constraint that names it exactly, and a channel matches no version at
// all: what a channel resolves to is a release the resolver pins, not a
// version this package can recognise.
func (c Constraint) Matches(v Version) bool {
	if c.IsZero() || c.IsChannel() {
		return false
	}
	if v.Pre != c.pre {
		return false
	}
	got := []int{v.Major, v.Minor, v.Patch}
	for i, want := range c.parts {
		if got[i] != want {
			return false
		}
	}
	return true
}

// MatchesRelease reports whether an identity a lockfile records — a version
// for a version constraint, the tag that never moves for a channel — is one
// this constraint could have resolved to. It is what tells a hand-edited pin
// from one block wrote.
func (c Constraint) MatchesRelease(id string) bool {
	if c.IsZero() {
		return false
	}
	if c.IsChannel() {
		return strings.HasPrefix(id, c.channel+"-")
	}
	v, err := Parse(id)
	if err != nil {
		return false
	}
	return c.Matches(v)
}

// ValidateReleaseID checks that an identity is something block can put in a
// path and read back: the same closed alphabet [Parse] accepts, and the same
// bound. A version goes through Parse itself; a channel release is a tag, and
// a tag is not a version.
func ValidateReleaseID(id string) error {
	if id == "" {
		return errors.New("release is empty")
	}
	if len(id) > maxVersion {
		return fmt.Errorf("invalid release: %d characters is longer than the %d a release may be", len(id), maxVersion)
	}
	return checkAlphabet(id)
}

// Latest returns the highest version in vs that satisfies c. Two versions
// semver calls equal are separated by [compareTotal], so the answer does not
// depend on the order vs is in.
func Latest(vs []Version, c Constraint) (Version, bool) {
	var best Version
	found := false
	for _, v := range vs {
		if !c.Matches(v) {
			continue
		}
		if !found || compareTotal(v, best) > 0 {
			best, found = v, true
		}
	}
	return best, found
}
