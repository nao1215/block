// Package archive extracts the tar.gz and zip archives upstreams publish.
// Extraction is defensive: every entry must stay inside the destination —
// including where a link points — and only the owner-executable bit is
// preserved from the archive's permissions.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nao1215/block/internal/diag"
)

// maxFileBytes caps a single extracted file, and maxTotalBytes and maxEntries
// cap the archive as a whole, so that a crafted archive cannot fill the disk
// through compression — neither with one enormous member nor with very many
// small ones, which a per-file limit alone does not stop.
//
// The bounds are far above any real toolchain: the largest artifact in the
// registry unpacks to a few hundred megabytes in a few hundred files.
const (
	maxFileBytes  = 2 << 30
	maxTotalBytes = 8 << 30
	maxEntries    = 200_000
)

// budget is what is left of an archive's aggregate allowance.
type budget struct {
	bytes   int64
	entries int
}

func newBudget() *budget { return &budget{bytes: maxTotalBytes, entries: maxEntries} }

// entry accounts for one member before it is written.
func (b *budget) entry(name string) error {
	b.entries--
	if b.entries < 0 {
		return diag.ArchiveTooLarge.Errorf("refusing to extract %q: the archive holds more than %d entries", name, maxEntries)
	}
	return nil
}

// wrote accounts for the bytes one member contributed.
func (b *budget) wrote(name string, n int64) error {
	b.bytes -= n
	if b.bytes < 0 {
		return diag.ArchiveTooLarge.Errorf("refusing to extract %q: the archive unpacks to more than %d bytes", name, int64(maxTotalBytes))
	}
	return nil
}

const (
	dirMode  = 0o755
	fileMode = 0o644
	execMode = 0o755
)

// Extract unpacks src into dst based on name's extension, dropping strip
// leading path components from every member (entries that do not have that
// many components are skipped, as tar --strip-components does). dst must
// already exist and should be empty.
func Extract(src, dst, name string, strip int) error {
	switch {
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		return extractTar(src, dst, strip, func(r io.Reader) (io.Reader, error) {
			gz, err := gzip.NewReader(r)
			if err != nil {
				return nil, diag.ArchiveUnreadable.Errorf("invalid gzip archive: %w", err)
			}
			return gz, nil
		})
	case strings.HasSuffix(name, ".tar.bz2"), strings.HasSuffix(name, ".tbz2"):
		return extractTar(src, dst, strip, func(r io.Reader) (io.Reader, error) { return bzip2.NewReader(r), nil })
	case strings.HasSuffix(name, ".zip"):
		return extractZip(src, dst, strip)
	default:
		return diag.ArchiveUnreadable.Errorf("unsupported archive format: %s", name)
	}
}

// stripName removes n leading components; ok is false when nothing is left.
func stripName(name string, n int) (string, bool) {
	if n == 0 {
		return name, true
	}
	parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
	if len(parts) <= n {
		return "", false
	}
	rest := strings.Join(parts[n:], "/")
	return rest, rest != ""
}

func extractTar(src, dst string, strip int, decompress func(io.Reader) (io.Reader, error)) error {
	f, err := os.Open(src) //nolint:gosec // cache path
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	r, err := decompress(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	left := newBudget()
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return diag.ArchiveUnreadable.Errorf("invalid tar archive: %w", err)
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader || hdr.Typeflag == tar.TypeXHeader {
			continue
		}
		name, ok := stripName(hdr.Name, strip)
		if !ok || isRoot(name) {
			// "./" — the entry tar writes for the directory it was run in —
			// names the destination itself, which already exists.
			continue
		}
		target, err := safePath(dst, name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, dirMode); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := left.entry(hdr.Name); err != nil {
				return err
			}
			n, err := writeFile(target, tr, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			if err := left.wrote(hdr.Name, n); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := symlink(dst, target, hdr.Linkname, hdr.Name); err != nil {
				return err
			}
		case tar.TypeLink:
			// A hard link names another member by the name it has in the
			// archive, so the components dropped from that name have to be
			// dropped from this one too, or the two stop referring to the
			// same file.
			name, ok := stripName(hdr.Linkname, strip)
			if !ok {
				return diag.PathEscape.Errorf("refusing to extract %q: it links to %q, which is not in the archive", hdr.Name, hdr.Linkname)
			}
			if err := hardlink(dst, target, name, hdr.Name); err != nil {
				return err
			}
		default:
			// Character and block devices, FIFOs, sockets: a tool
			// distribution has no use for any of them, and each is a way to
			// make the install directory do something other than hold files.
			return diag.UnsupportedEntry.Errorf("refusing to extract %q: unsupported entry type %q", hdr.Name, hdr.Typeflag)
		}
	}
}

func extractZip(src, dst string, strip int) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return diag.ArchiveUnreadable.Errorf("invalid zip archive: %w", err)
	}
	defer zr.Close() //nolint:errcheck // read-only
	left := newBudget()
	for _, zf := range zr.File {
		name, ok := stripName(zf.Name, strip)
		if !ok || isRoot(name) {
			continue
		}
		target, err := safePath(dst, name)
		if err != nil {
			return err
		}
		mode := zf.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, dirMode); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			// A zip symlink is a file whose contents are the target path.
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			const maxLinkBytes = 4096
			link, err := io.ReadAll(io.LimitReader(rc, maxLinkBytes))
			_ = rc.Close()
			if err != nil {
				return diag.ArchiveUnreadable.Errorf("invalid zip archive: %w", err)
			}
			if err := symlink(dst, target, string(link), zf.Name); err != nil {
				return err
			}
		case mode.IsRegular():
			if err := left.entry(zf.Name); err != nil {
				return err
			}
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			n, err := writeFile(target, rc, mode)
			_ = rc.Close()
			if err != nil {
				return err
			}
			if err := left.wrote(zf.Name, n); err != nil {
				return err
			}
		default:
			// Zip carries device nodes, FIFOs and sockets in the same mode
			// bits Go decodes here, so the one branch refuses them all.
			return diag.UnsupportedEntry.Errorf("refusing to extract %q: unsupported entry type", zf.Name)
		}
	}
	return nil
}

// symlink creates the link an archive member describes, once its target is
// known to stay inside dst.
//
// A link is where an archive gets to name a path block never checked: the
// entry is inside the destination and the thing it points at need not be.
// Real distributions rely on them — Nethermind ships Nethermind.Runner
// pointing at nethermind beside it, and versioned shared libraries are almost
// always a name and a link — so they are extracted, and the target is held to
// the same rule every other path is: it resolves inside dst or it is refused.
//
// A dangling link is allowed. Archives list members in their own order, and a
// link to a file that comes later is not an error; what the link may not do
// is leave the directory.
func symlink(dst, target, linkname, name string) error {
	resolved, err := linkTarget(dst, target, linkname, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return err
	}
	if err := os.Symlink(filepath.FromSlash(linkname), target); err != nil {
		// Windows needs a privilege block will not ask for. The target is
		// inside the install directory, so a copy of it is the same thing to
		// everything that follows — when it has been unpacked already.
		if st, lerr := os.Lstat(resolved); lerr != nil || !st.Mode().IsRegular() {
			return diag.UnsupportedEntry.Errorf("refusing to extract %q: this filesystem cannot link to %q (%w)", name, linkname, err)
		}
		return copyLinked(resolved, target)
	}
	return nil
}

// hardlink reproduces a hard link between two members of one archive. The
// member it points at has to be there already, which is what an archive that
// carries one guarantees.
func hardlink(dst, target, linkname, name string) error {
	resolved, err := linkTarget(dst, target, linkname, name)
	if err != nil {
		return err
	}
	// A hard link is a second name for a member the archive already wrote.
	// One pointing at something that is not there is a broken archive, not a
	// link, and saying which name is missing is the only useful thing to say.
	if st, err := os.Lstat(resolved); err != nil || !st.Mode().IsRegular() {
		return diag.UnsupportedEntry.Errorf("refusing to extract %q: it links to %q, which the archive did not write", name, linkname)
	}
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return err
	}
	if err := os.Link(resolved, target); err != nil {
		return copyLinked(resolved, target)
	}
	return nil
}

// linkTarget resolves what a link points at and refuses anything that leaves
// dst: an absolute path, a Windows path, or a relative one with enough ".."
// in it. A symlink's target is relative to the directory the link is in; a
// hard link's has already been made relative to dst by the caller.
func linkTarget(dst, target, linkname, name string) (string, error) {
	switch {
	case linkname == "":
		return "", diag.PathEscape.Errorf("refusing to extract %q: it links to nothing", name)
	case strings.ContainsRune(linkname, 0):
		return "", diag.PathEscape.Errorf("refusing to extract %q: its link target contains a NUL", name)
	case strings.ContainsRune(linkname, '\\'), driveLetter(linkname):
		return "", diag.PathEscape.Errorf("refusing to extract %q: a link may not name a drive, a share or a Windows path", name)
	case path.IsAbs(linkname), filepath.IsAbs(filepath.FromSlash(linkname)):
		return "", diag.PathEscape.Errorf("refusing to extract %q: it links to the absolute path %q", name, linkname)
	}
	resolved := filepath.Join(filepath.Dir(target), filepath.FromSlash(linkname))
	rel, err := filepath.Rel(dst, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", diag.PathEscape.Errorf("refusing to extract %q: it links to %q, which is outside the destination", name, linkname)
	}
	return resolved, nil
}

// copyLinked stands in for a link the filesystem would not make, by copying
// what it points at. The caller has checked that the target is a regular file
// inside the destination.
func copyLinked(resolved, target string) error {
	st, err := os.Lstat(resolved)
	if err != nil {
		return err
	}
	in, err := os.Open(resolved) //nolint:gosec // inside the destination, already checked
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only
	perm := os.FileMode(fileMode)
	if st.Mode()&0o100 != 0 {
		perm = execMode
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm) //nolint:gosec // inside the destination
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// isRoot reports whether a member name, once cleaned, is the destination
// itself rather than something inside it.
func isRoot(name string) bool {
	return name != "" && filepath.Clean(filepath.FromSlash(name)) == "."
}

// safePath resolves an archive member name inside dst, rejecting absolute
// names and any traversal outside dst.
func safePath(dst, name string) (string, error) {
	if name == "" {
		return "", diag.PathEscape.Errorf("archive entry has an empty name")
	}
	// A NUL cannot be part of a file name on any platform block runs on. It
	// is refused by name rather than left to the operating system, whose
	// "invalid argument" would not say which member of the archive was
	// wrong — and the archive is somebody else's, so it has to be said.
	if strings.ContainsRune(name, 0) {
		return "", diag.PathEscape.Errorf("refusing to extract %q: an archive entry may not contain a NUL", name)
	}
	// tar and zip member names are slash-separated by specification, so a
	// backslash, a drive letter or a UNC prefix is never a legitimate one.
	// They are refused on every platform rather than only on Windows: an
	// archive that carries "C:\Windows\..." is the same archive wherever it
	// is unpacked, and finding out only on the platform where it would have
	// worked is finding out too late. The check also keeps extraction — and
	// the test that pins it — identical everywhere.
	if strings.ContainsRune(name, '\\') || driveLetter(name) {
		return "", diag.PathEscape.Errorf("refusing to extract %q: an archive entry may not name a drive, a share or a Windows path", name)
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	// A leading separator is rejected explicitly: on Windows "\etc" is not
	// absolute in filepath's terms, but it is never a legitimate member name.
	// The destination itself is not a member either: "." names nothing
	// inside dst, and the callers skip the directory entry that spells it.
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, string(filepath.Separator)) ||
		strings.HasPrefix(clean, "..") || filepath.VolumeName(clean) != "" {
		return "", diag.PathEscape.Errorf("refusing to extract %q: path escapes the destination", name)
	}
	target := filepath.Join(dst, clean)
	rel, err := filepath.Rel(dst, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", diag.PathEscape.Errorf("refusing to extract %q: path escapes the destination", name)
	}
	// Refuse to follow a symlink that an earlier entry could not have created
	// but that might pre-exist in dst.
	for dir := filepath.Dir(target); strings.HasPrefix(dir, dst) && dir != dst; dir = filepath.Dir(dir) {
		if st, err := os.Lstat(dir); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return "", diag.PathEscape.Errorf("refusing to extract %q: parent %q is a symlink", name, dir)
		}
	}
	return target, nil
}

// driveLetter reports whether name starts with a Windows drive specification
// ("C:", "c:/"), whatever platform this is.
func driveLetter(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	c := name[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// writeFile writes one archive member and reports how many bytes it held.
func writeFile(target string, r io.Reader, mode os.FileMode) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return 0, err
	}
	perm := os.FileMode(fileMode)
	if mode&0o100 != 0 {
		perm = execMode
	}
	// O_EXCL, so a second member can never rewrite what a first one wrote.
	// That matters most where the two names are not equal: on a
	// case-insensitive filesystem "tool" and "TOOL" are one file, and an
	// archive that relies on which of them lands last means different things
	// on macOS and on Linux.
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm) //nolint:gosec // inside the install dir
	if errors.Is(err, os.ErrExist) {
		return 0, diag.DuplicateEntry.Errorf("refusing to extract %q: the archive already wrote that file (names differing only in case are one file on some filesystems)", filepath.Base(target))
	}
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxFileBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	if n > maxFileBytes {
		return 0, diag.ArchiveTooLarge.Errorf("refusing to extract %q: file exceeds %d bytes", filepath.Base(target), maxFileBytes)
	}
	return n, nil
}
