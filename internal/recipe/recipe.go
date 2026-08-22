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
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/nao1215/block/internal/diag"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/version"
)

// Source types, in the order block prefers them. A recipe states one; block
// executes it deterministically and never falls back to another.
const (
	// TypeGitHubRelease discovers versions from git tags and downloads a
	// GitHub Release asset (an archive or a single raw executable).
	TypeGitHubRelease = "github_release"
	// TypeHTTP discovers versions from git tags and downloads a prebuilt
	// artifact from the upstream's own HTTPS download server.
	TypeHTTP = "http"
)

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
	// Asset is the release asset file name template (github_release).
	// {version}, {os} and {arch} are substituted; {os}/{arch} go through the
	// OS/Arch maps first. Upstreams that stamp the build commit into the
	// asset name (vyper, Nethermind, Nimbus) may also use {commit}. A name
	// without an archive extension is a single raw executable installed
	// under the one name in Bin.
	Asset string `toml:"asset,omitempty"`
	// URL is the HTTPS download URL template (http). It accepts the same
	// placeholders as Asset, including {commit}.
	URL string `toml:"url,omitempty"`
	// StripComponents drops this many leading path components when
	// extracting, for archives that wrap everything in a versioned directory.
	StripComponents int `toml:"strip_components,omitempty"`
	// OS renames Go's GOOS into the upstream's spelling (darwin -> apple-darwin).
	OS map[string]string `toml:"os,omitempty"`
	// Arch renames Go's GOARCH into the upstream's spelling (amd64 -> x86_64).
	Arch map[string]string `toml:"arch,omitempty"`
	// Target maps a whole "os/arch" pair to the upstream's platform string,
	// for upstreams whose naming is not a product of the two (Bitcoin Core
	// writes aarch64-linux-gnu but arm64-apple-darwin). It expands {target}
	// and, when Platforms is empty, its keys are the supported platforms.
	Target map[string]string `toml:"target,omitempty"`
	// Platforms lists the "os/arch" pairs the upstream ships. Empty means every
	// platform block supports.
	Platforms []string `toml:"platforms,omitempty"`
	// Bin lists the executables inside the extracted archive, relative to its
	// root. They become available to `block exec`.
	Bin []string `toml:"bin"`
	// Channels are the release lines an upstream publishes under a tag that
	// moves — Foundry's "nightly" — keyed by the name block.toml asks for.
	// They are declared because their assets are named differently from the
	// versioned ones: what block does with a channel is the same for all of
	// them, and is described at [Channel].
	Channels map[string]Channel `toml:"channels,omitempty"`
}

// Channel is one moving release line of a source.
//
// A channel is a tag that points somewhere new every day, which is the one
// thing a lockfile may not record. block resolves it the way the upstream
// makes possible: the moving tag is dereferenced to the commit it points at,
// and the release tagged "<channel>-<commit>" — the tag that will never move
// again — is what gets pinned. That model is Foundry's, and it is the only one
// in the registry today; an upstream that publishes a moving tag without a
// tag for the commit under it cannot be pinned, and block says so rather than
// recording something that will change.
type Channel struct {
	// Asset is the release asset file name template for this channel. It
	// exists because a channel names its assets after the channel rather than
	// after a version — "foundry_nightly_linux_amd64.tar.gz" — so the source's
	// own template cannot render them. {os}, {arch}, {target} and {commit}
	// mean what they mean everywhere else; {version} has no meaning here,
	// because a channel release has no version.
	Asset string `toml:"asset"`
}

// Recipe is a named Source plus the metadata that describes the tool to a
// human. block attaches no behaviour to the metadata: it exists so that the
// registry can answer "what is this tool?" wherever that is asked — a
// listing, the block-registry catalogue site, a documentation build.
type Recipe struct {
	Name string `toml:"name"`
	// Ecosystems are the blockchain systems the tool serves (bitcoin,
	// ethereum, solana, cosmos, ibc, ...). A tool can belong to more than
	// one: an IBC relayer is used from both cosmos and ibc work. The names
	// are canonical registry data, not a closed set block knows about.
	Ecosystems []string `toml:"ecosystems"`
	// Description is one plain sentence saying what the tool is, phrased so
	// it reads on its own next to the tool's name.
	Description string `toml:"description"`
	Source      Source `toml:"source"`
}

// maxDescription keeps a description to something that still fits a terminal
// column next to the other metadata.
const maxDescription = 100

// Validate checks that the source is complete and internally consistent.
func (s Source) Validate() error {
	owner, name, ok := strings.Cut(s.Repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("invalid repo %q: want owner/name", s.Repo)
	}
	// The two halves are spliced into API URLs verbatim. GitHub names are
	// ASCII letters, digits, '-', '_' and '.', so anything else — a space, a
	// '?', a '#', a '%' — is a typo that would otherwise surface as a
	// puzzling "not found" from the wrong endpoint.
	if !validRepoPart(owner) || !validRepoPart(name) {
		return fmt.Errorf("invalid repo %q: owner and name may only use letters, digits, '-', '_' and '.'", s.Repo)
	}
	switch s.Type {
	case TypeGitHubRelease:
		if s.URL != "" {
			return errors.New("url is only valid for type \"http\"")
		}
		if err := s.validateAsset(); err != nil {
			return err
		}
	case TypeHTTP:
		if s.Asset != "" {
			return errors.New("asset is only valid for type \"github_release\"")
		}
		if err := s.validateURL(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported source type %q: want %q or %q", s.Type, TypeGitHubRelease, TypeHTTP)
	}
	if s.StripComponents < 0 {
		return errors.New("strip_components must not be negative")
	}
	if len(s.Bin) == 0 {
		return errors.New("bin must list at least one executable")
	}
	if !s.IsArchive() {
		if len(s.Bin) != 1 || strings.Contains(s.Bin[0], "/") {
			return fmt.Errorf("a raw executable %q needs exactly one bare bin name", s.ArtifactTemplate())
		}
		if s.StripComponents != 0 {
			return errors.New("strip_components is only valid for archives")
		}
	}
	if err := ValidateBins(s.Bin); err != nil {
		return err
	}
	for _, p := range s.Platforms {
		if _, err := platform.Parse(p); err != nil {
			return err
		}
	}
	for p := range s.Target {
		if _, err := platform.Parse(p); err != nil {
			return err
		}
	}
	if err := s.validateChannels(); err != nil {
		return err
	}
	if strings.Contains(s.ArtifactTemplate(), "{target}") {
		if len(s.Target) == 0 {
			return fmt.Errorf("template %q uses {target} but no [source.target] table is defined", s.ArtifactTemplate())
		}
		// Every platform the recipe claims must have something to expand
		// {target} to. Without this, a recipe can say it ships for a platform
		// and then refuse it at resolution time with "unsupported platform
		// linux/arm64 (available: ..., linux/arm64, ...)" — an error that
		// contradicts itself and that only the recipe's author can fix.
		for _, p := range s.SupportedPlatforms() {
			if _, ok := s.Target[p.String()]; !ok {
				return fmt.Errorf("platform %s is listed but [source.target] has no name for it", p)
			}
		}
	}
	return nil
}

// validRepoPart reports whether a repository owner or name uses only the
// characters GitHub allows in one.
func validRepoPart(part string) bool {
	if part == "." || part == ".." {
		return false
	}
	for i := range len(part) {
		c := part[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// validateChannels checks the moving release lines a source declares.
func (s Source) validateChannels() error {
	for name, ch := range s.Channels {
		if err := ValidateChannelName(name); err != nil {
			return err
		}
		if s.Type != TypeGitHubRelease {
			return fmt.Errorf("channel %q: channels need type %q, because pinning one needs the release the moving tag points at", name, TypeGitHubRelease)
		}
		switch {
		case ch.Asset == "":
			return fmt.Errorf("channel %q: asset template is required", name)
		case strings.ContainsAny(ch.Asset, "/\\"):
			return fmt.Errorf("channel %q: asset template %q must be a bare file name", name, ch.Asset)
		case strings.Contains(ch.Asset, "{version}"):
			return fmt.Errorf("channel %q: asset template %q uses {version}, and a channel release has no version", name, ch.Asset)
		case strings.Contains(ch.Asset, "{target}") && len(s.Target) == 0:
			return fmt.Errorf("channel %q: asset template %q uses {target} but no [source.target] table is defined", name, ch.Asset)
		case IsArchiveName(ch.Asset) != s.IsArchive():
			// One tool, one shape: bin and strip_components describe what is
			// inside the artifact, and there is one of each.
			return fmt.Errorf("channel %q: asset %q is not the same kind of artifact as %q", name, ch.Asset, s.ArtifactTemplate())
		}
	}
	return nil
}

// ValidateChannelName accepts the names an upstream gives a release line:
// lower-case letters, digits and '-', starting with a letter. It is the same
// alphabet a constraint accepts, so a channel a recipe declares is one
// block.toml can ask for.
func ValidateChannelName(name string) error {
	if name == "" {
		return errors.New("channel name is empty")
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '-'):
		default:
			return fmt.Errorf("invalid channel name %q: use lower-case letters, digits and '-'", name)
		}
	}
	return nil
}

// Channel returns the declared channel by name.
func (s Source) Channel(name string) (Channel, bool) {
	ch, ok := s.Channels[name]
	return ch, ok
}

// ChannelNames lists the channels the source declares, sorted.
func (s Source) ChannelNames() []string {
	return slices.Sorted(maps.Keys(s.Channels))
}

func (s Source) validateAsset() error {
	if s.Asset == "" {
		return errors.New("asset template is required")
	}
	// {version} is optional here: the release is already version-specific
	// and some upstreams (solc) name their assets without it.
	if strings.ContainsAny(s.Asset, "/\\") {
		return fmt.Errorf("asset template %q must be a bare file name", s.Asset)
	}
	return nil
}

func (s Source) validateURL() error {
	if s.URL == "" {
		return errors.New("url template is required")
	}
	if !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://") {
		return fmt.Errorf("url template %q must start with https://", s.URL)
	}
	if !strings.Contains(s.URL, "{version}") {
		return fmt.Errorf("url template %q must contain {version}", s.URL)
	}
	return nil
}

// ArtifactTemplate is the asset or url template, whichever the type uses.
func (s Source) ArtifactTemplate() string {
	if s.Type == TypeHTTP {
		return s.URL
	}
	return s.Asset
}

// IsArchive reports whether the artifact is an archive (as opposed to a
// single raw executable), judged by the template's extension.
func (s Source) IsArchive() bool {
	return IsArchiveName(s.ArtifactTemplate())
}

// IsArchiveName reports whether a file name has an archive extension block
// can extract.
func IsArchiveName(name string) bool {
	for _, ext := range []string{".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".zip"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// NeedsCommit reports whether resolving an artifact requires the commit the
// version tag points at.
func (s Source) NeedsCommit() bool {
	return strings.Contains(s.ArtifactTemplate(), "{commit}")
}

// ValidateBin checks one executable path from a recipe or a lockfile. A
// lockfile is untrusted input — it arrives through pull requests and hand
// edits — so both go through this same check, and an entry that could point
// outside the install directory is refused.
func ValidateBin(b string) error {
	if b == "" {
		return errors.New("bin entry is empty")
	}
	if strings.ContainsRune(b, 0) {
		return errors.New("bin entry contains a NUL byte")
	}
	// Backslashes, drive letters and colons are rejected wherever block
	// runs: an entry that means one thing on Linux and another on Windows is
	// not a path a recipe or a lockfile may carry.
	if strings.ContainsAny(b, "\\:") || filepath.VolumeName(b) != "" {
		return fmt.Errorf("invalid bin entry %q: want a relative slash-separated path inside the archive", b)
	}
	clean := path.Clean(b)
	if clean != b || path.IsAbs(b) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("invalid bin entry %q: want a relative path inside the archive", b)
	}
	return nil
}

// ValidateBins checks one tool's list of executables: every entry a relative
// path inside the artifact, no entry twice, and no two entries ending in the
// same command name.
//
// A recipe and a lockfile carry the same list and are checked by this same
// function, because they have to agree: a list a recipe accepted and a
// lockfile then refused would be a toolchain that locks and will not sync.
// Each caller wraps the result with whichever of the two it is reading.
func ValidateBins(bins []string) error {
	seen := map[string]bool{}
	commands := map[string]string{}
	for _, b := range bins {
		if err := ValidateBin(b); err != nil {
			return err
		}
		if seen[b] {
			return fmt.Errorf("bin %q is listed twice", b)
		}
		seen[b] = true
		// Two different paths ending in the same command name are worse than
		// a duplicate: only one of them can be the command, and which one
		// depends on whether the caller went through a shim or through PATH.
		if first, ok := commands[CommandKey(b)]; ok {
			return fmt.Errorf("bin %q and %q are both the command %q", first, b, CommandName(b))
		}
		commands[CommandKey(b)] = b
	}
	return nil
}

// CommandName is the command a user types for an executable path.
func CommandName(bin string) string { return path.Base(bin) }

// CommandKey is the name two commands must not share. Windows resolves a
// command on PATH without regard to case, so "foo" and "FOO" are one command
// there and two everywhere else. A lockfile is committed and read on every
// platform, so block takes the stricter reading everywhere: a toolchain that
// installs on Linux and collides on Windows is the failure block exists to
// prevent, and it should be found by whoever runs lock, not by whoever runs
// Windows.
func CommandKey(bin string) string { return strings.ToLower(CommandName(bin)) }

// Validate checks the recipe name, its metadata and its source.
func (r Recipe) Validate() error {
	if err := ValidateName(r.Name); err != nil {
		return err
	}
	if err := r.validateEcosystems(); err != nil {
		return fmt.Errorf("tool %q: %w", r.Name, err)
	}
	if err := r.validateDescription(); err != nil {
		return fmt.Errorf("tool %q: %w", r.Name, err)
	}
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("tool %q: %w", r.Name, err)
	}
	return nil
}

func (r Recipe) validateEcosystems() error {
	if len(r.Ecosystems) == 0 {
		return errors.New("ecosystems is required: list the blockchain systems the tool serves")
	}
	seen := map[string]bool{}
	for _, e := range r.Ecosystems {
		if err := ValidateName(e); err != nil {
			return fmt.Errorf("ecosystem: %w", err)
		}
		if seen[e] {
			return fmt.Errorf("ecosystem %q is listed twice", e)
		}
		seen[e] = true
	}
	return nil
}

func (r Recipe) validateDescription() error {
	switch {
	case strings.TrimSpace(r.Description) == "":
		return errors.New("description is required: one sentence saying what the tool is")
	case r.Description != strings.TrimSpace(r.Description):
		return fmt.Errorf("description %q has leading or trailing whitespace", r.Description)
	case strings.ContainsAny(r.Description, "\n\r\t"):
		return errors.New("description must be a single line")
	case len(r.Description) > maxDescription:
		return fmt.Errorf("description is %d characters long, keep it under %d", len(r.Description), maxDescription)
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

// SupportedPlatforms returns the platforms the source ships for: the declared
// list, else the target table's keys, else every platform block supports.
//
// A declared list and a target table come back sorted; the default comes back
// in [platform.Supported]'s own reading order. The difference is deliberate
// and pinned by a test: [Source.Hash] joins this list, that hash is recorded
// in block.lock for a project-local source, and reordering it would make
// every such lockfile stale for no reason a reader could see.
func (s Source) SupportedPlatforms() []platform.Platform {
	names := s.Platforms
	if len(names) == 0 {
		if len(s.Target) == 0 {
			return platform.Supported()
		}
		names = make([]string, 0, len(s.Target))
		for p := range s.Target {
			names = append(names, p)
		}
	}
	out := make([]platform.Platform, 0, len(names))
	for _, p := range names {
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

// commitLen is how many hex digits of a commit {commit} expands to.
const commitLen = 8

// Render expands the asset or url template for a version, platform and
// (when the template uses it) commit.
func (s Source) Render(v version.Version, p platform.Platform, commit string) (string, error) {
	if !s.Supports(p) {
		return "", &UnsupportedPlatformError{Platform: p, Supported: s.SupportedPlatforms()}
	}
	if s.NeedsCommit() && len(commit) < commitLen {
		return "", fmt.Errorf("template %q needs a commit but none was resolved", s.ArtifactTemplate())
	}
	return s.expand(s.ArtifactTemplate(), v.String(), p, commit)
}

// expand substitutes the placeholders of one template. It is shared by the
// versioned artifact and by a channel's, so the two cannot come to spell
// {os} or {target} differently.
func (s Source) expand(tmpl, version string, p platform.Platform, commit string) (string, error) {
	os, arch := p.OS, p.Arch
	if m, ok := s.OS[os]; ok {
		os = m
	}
	if m, ok := s.Arch[arch]; ok {
		arch = m
	}
	short := commit
	if len(short) > commitLen {
		short = short[:commitLen]
	}
	target, ok := s.Target[p.String()]
	if !ok && strings.Contains(tmpl, "{target}") {
		return "", &UnsupportedPlatformError{Platform: p, Supported: s.SupportedPlatforms()}
	}
	r := strings.NewReplacer("{version}", version, "{os}", os, "{arch}", arch, "{commit}", short, "{target}", target)
	return r.Replace(tmpl), nil
}

// RenderChannel expands a channel's asset template for a platform and the
// commit its moving tag pointed at.
func (s Source) RenderChannel(name string, p platform.Platform, commit string) (string, error) {
	ch, ok := s.Channel(name)
	if !ok {
		return "", fmt.Errorf("no channel %q", name)
	}
	if !s.Supports(p) {
		return "", &UnsupportedPlatformError{Platform: p, Supported: s.SupportedPlatforms()}
	}
	return s.expand(ch.Asset, "", p, commit)
}

// AssetName renders the release asset file name for a version and platform.
func (s Source) AssetName(v version.Version, p platform.Platform) (string, error) {
	return s.Render(v, p, "")
}

// Equal reports whether two sources resolve artifacts identically.
//
// Platforms is compared as a set and Bin as a sequence, because that is what
// each one means: the order platforms are listed in changes nothing, while
// the order of Bin is the order a lockfile records them in.
func (s Source) Equal(o Source) bool {
	return s.Type == o.Type && s.Repo == o.Repo && s.EffectiveTagPrefix() == o.EffectiveTagPrefix() &&
		s.Asset == o.Asset && s.URL == o.URL && s.StripComponents == o.StripComponents &&
		maps.Equal(s.OS, o.OS) && maps.Equal(s.Arch, o.Arch) && maps.Equal(s.Target, o.Target) &&
		sameSet(s.Platforms, o.Platforms) && slices.Equal(s.Bin, o.Bin) &&
		maps.Equal(s.Channels, o.Channels)
}

// sameSet compares two lists without regard to order, leaving the originals
// untouched.
func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// UnsupportedPlatformError reports that a source ships nothing for a platform.
type UnsupportedPlatformError struct {
	Platform  platform.Platform
	Supported []platform.Platform
}

// Code names the diagnostic this refusal is published under.
func (e *UnsupportedPlatformError) Code() diag.Code { return diag.PlatformUnsupported }

func (e *UnsupportedPlatformError) Error() string {
	return fmt.Sprintf("unsupported platform %s (available: %s)", e.Platform, strings.Join(platform.Strings(e.Supported), ", "))
}

// Hash returns a stable fingerprint of everything that influences how the
// source resolves artifacts. block.lock records it so that a changed recipe
// (renamed asset, different executables, other repository) is detected as
// stale instead of silently installing something the lock never resolved.
func (s Source) Hash() string {
	var b strings.Builder
	b.WriteString(s.Type + "\n" + s.Repo + "\n" + s.EffectiveTagPrefix() + "\n" + s.Asset + "\n" + s.URL + "\n")
	b.WriteString(strconv.Itoa(s.StripComponents) + "\n")
	writeMap := func(m map[string]string) {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			b.WriteString(k + "=" + m[k] + "\n")
		}
		b.WriteString("--\n")
	}
	writeMap(s.OS)
	writeMap(s.Arch)
	writeMap(s.Target)
	b.WriteString(strings.Join(platform.Strings(s.SupportedPlatforms()), ",") + "\n")
	b.WriteString(strings.Join(s.Bin, ",") + "\n")
	for _, name := range s.ChannelNames() {
		b.WriteString(name + "=" + s.Channels[name].Asset + "\n")
	}
	b.WriteString("--\n")
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}
