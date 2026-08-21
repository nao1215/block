// Package recipe defines how block discovers versions and artifacts of one
// tool from its upstream. A recipe is not a list of versions: it is the rule
// that turns "upstream published tag X" into "download this asset, run this
// binary". The same model backs both the built-in registry and a
// project-local [tools.<name>.source] table in block.toml.
package recipe

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/version"
)

// TypeGitHubRelease names the only source type v0.1 implements: versions come
// from git tags and artifacts from GitHub Release assets.
const TypeGitHubRelease = "github_release"

// DefaultTagPrefix is stripped from tags when no tag_prefix is configured.
const DefaultTagPrefix = "v"

// Source describes where a tool's versions and artifacts come from.
type Source struct {
	// Type selects the discovery strategy. Only "github_release" exists.
	Type string `toml:"type"`
	// Repo is the "owner/name" GitHub repository.
	Repo string `toml:"repo"`
	// TagPrefix is the text before the semantic version in a git tag ("v"
	// for "v1.7.4"). Set it to "" explicitly for bare "1.7.4" tags.
	TagPrefix *string `toml:"tag_prefix,omitempty"`
	// Asset is the release asset file name template. {version}, {os} and
	// {arch} are substituted; {os}/{arch} go through the OS/Arch maps first.
	Asset string `toml:"asset"`
	// OS renames Go's GOOS into the upstream's spelling (darwin -> apple-darwin).
	OS map[string]string `toml:"os,omitempty"`
	// Arch renames Go's GOARCH into the upstream's spelling (amd64 -> x86_64).
	Arch map[string]string `toml:"arch,omitempty"`
	// Platforms lists the "os/arch" pairs the upstream ships. Empty means every
	// platform block supports.
	Platforms []string `toml:"platforms,omitempty"`
	// Bin lists the executables inside the extracted archive, relative to its
	// root. They become available to `block exec`.
	Bin []string `toml:"bin"`
}

// Recipe is a named Source.
type Recipe struct {
	Name   string `toml:"name"`
	Source Source `toml:"source"`
}

// Validate checks that the source is complete and internally consistent.
func (s Source) Validate() error {
	if s.Type != TypeGitHubRelease {
		return fmt.Errorf("unsupported source type %q: only %q is supported", s.Type, TypeGitHubRelease)
	}
	owner, name, ok := strings.Cut(s.Repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("invalid repo %q: want owner/name", s.Repo)
	}
	if s.Asset == "" {
		return errors.New("asset template is required")
	}
	if !strings.Contains(s.Asset, "{version}") {
		return fmt.Errorf("asset template %q must contain {version}", s.Asset)
	}
	if strings.ContainsAny(s.Asset, "/\\") {
		return fmt.Errorf("asset template %q must be a bare file name", s.Asset)
	}
	if !supportedArchive(s.Asset) {
		return fmt.Errorf("asset template %q must end in .tar.gz, .tgz or .zip", s.Asset)
	}
	if len(s.Bin) == 0 {
		return errors.New("bin must list at least one executable")
	}
	for _, b := range s.Bin {
		if err := validateBin(b); err != nil {
			return err
		}
	}
	for _, p := range s.Platforms {
		if _, err := platform.Parse(p); err != nil {
			return err
		}
	}
	return nil
}

func supportedArchive(name string) bool {
	for _, ext := range []string{".tar.gz", ".tgz", ".zip"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func validateBin(b string) error {
	if b == "" {
		return errors.New("bin entry is empty")
	}
	clean := path.Clean(b)
	if clean != b || path.IsAbs(b) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("invalid bin entry %q: want a relative path inside the archive", b)
	}
	return nil
}

// Validate checks the recipe name and its source.
func (r Recipe) Validate() error {
	if err := ValidateName(r.Name); err != nil {
		return err
	}
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("tool %q: %w", r.Name, err)
	}
	return nil
}

// ValidateName accepts lower-case names made of letters, digits, '-' and '_'.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("tool name is empty")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return fmt.Errorf("invalid tool name %q: use lower-case letters, digits, '-' and '_'", name)
		}
	}
	return nil
}

// EffectiveTagPrefix returns the configured tag prefix or the default "v".
func (s Source) EffectiveTagPrefix() string {
	if s.TagPrefix == nil {
		return DefaultTagPrefix
	}
	return *s.TagPrefix
}

// Tag renders the git tag for a version.
func (s Source) Tag(v version.Version) string {
	return s.EffectiveTagPrefix() + v.String()
}

// ParseTag turns a git tag into a version, reporting false for tags that do
// not carry the prefix or are not semantic versions (nightly builds, "stable").
func (s Source) ParseTag(tag string) (version.Version, bool) {
	rest, ok := strings.CutPrefix(tag, s.EffectiveTagPrefix())
	if !ok {
		return version.Version{}, false
	}
	v, err := version.Parse(rest)
	if err != nil {
		return version.Version{}, false
	}
	return v, true
}

// Platforms returns the platforms the source ships for, sorted.
func (s Source) SupportedPlatforms() []platform.Platform {
	if len(s.Platforms) == 0 {
		return platform.Supported()
	}
	out := make([]platform.Platform, 0, len(s.Platforms))
	for _, p := range s.Platforms {
		if pp, err := platform.Parse(p); err == nil {
			out = append(out, pp)
		}
	}
	platform.Sort(out)
	return out
}

// Supports reports whether the source ships an artifact for p.
func (s Source) Supports(p platform.Platform) bool {
	for _, sp := range s.SupportedPlatforms() {
		if sp == p {
			return true
		}
	}
	return false
}

// AssetName renders the release asset file name for a version and platform.
func (s Source) AssetName(v version.Version, p platform.Platform) (string, error) {
	if !s.Supports(p) {
		return "", &UnsupportedPlatformError{Platform: p, Supported: s.SupportedPlatforms()}
	}
	os, arch := p.OS, p.Arch
	if m, ok := s.OS[os]; ok {
		os = m
	}
	if m, ok := s.Arch[arch]; ok {
		arch = m
	}
	r := strings.NewReplacer("{version}", v.String(), "{os}", os, "{arch}", arch)
	return r.Replace(s.Asset), nil
}

// Equal reports whether two sources resolve artifacts identically.
func (s Source) Equal(o Source) bool {
	return s.Type == o.Type && s.Repo == o.Repo && s.EffectiveTagPrefix() == o.EffectiveTagPrefix() &&
		s.Asset == o.Asset && mapsEqual(s.OS, o.OS) && mapsEqual(s.Arch, o.Arch) &&
		sliceSetEqual(s.Platforms, o.Platforms) && sliceEqual(s.Bin, o.Bin)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sliceSetEqual(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return sliceEqual(a, b)
}

// UnsupportedPlatformError reports that a source ships nothing for a platform.
type UnsupportedPlatformError struct {
	Platform  platform.Platform
	Supported []platform.Platform
}

func (e *UnsupportedPlatformError) Error() string {
	return fmt.Sprintf("unsupported platform %s (available: %s)", e.Platform, strings.Join(platform.Strings(e.Supported), ", "))
}

// Hash returns a stable fingerprint of everything that influences how the
// source resolves artifacts. block.lock records it so that a changed recipe
// (renamed asset, different executables, other repository) is detected as
// stale instead of silently installing something the lock never resolved.
func (s Source) Hash() string {
	var b strings.Builder
	b.WriteString(s.Type + "\n" + s.Repo + "\n" + s.EffectiveTagPrefix() + "\n" + s.Asset + "\n")
	writeMap := func(m map[string]string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(k + "=" + m[k] + "\n")
		}
		b.WriteString("--\n")
	}
	writeMap(s.OS)
	writeMap(s.Arch)
	b.WriteString(strings.Join(platform.Strings(s.SupportedPlatforms()), ",") + "\n")
	b.WriteString(strings.Join(s.Bin, ",") + "\n")
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}
