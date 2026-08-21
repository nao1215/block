// Package archive extracts the tar.gz and zip archives upstreams publish.
// Extraction is defensive: every entry must stay inside the destination,
// symlinks and hard links are refused, and only the owner-executable bit is
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
	"path/filepath"
	"strings"

	"github.com/nao1215/block/internal/diag"
)

// maxFileBytes caps a single extracted file so a crafted archive cannot fill
// the disk through compression.
const maxFileBytes = 2 << 30

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
		if !ok {
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
			if err := writeFile(target, tr, hdr.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			return diag.UnsupportedEntry.Errorf("refusing to extract link %q: links are not supported", hdr.Name)
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
	for _, zf := range zr.File {
		name, ok := stripName(zf.Name, strip)
		if !ok {
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
			return diag.UnsupportedEntry.Errorf("refusing to extract link %q: links are not supported", zf.Name)
		case mode.IsRegular():
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			err = writeFile(target, rc, mode)
			_ = rc.Close()
			if err != nil {
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

// safePath resolves an archive member name inside dst, rejecting absolute
// names and any traversal outside dst.
func safePath(dst, name string) (string, error) {
	if name == "" {
		return "", diag.PathEscape.Errorf("archive entry has an empty name")
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
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, string(filepath.Separator)) ||
		strings.HasPrefix(clean, "..") || filepath.VolumeName(clean) != "" {
		return "", diag.PathEscape.Errorf("refusing to extract %q: path escapes the destination", name)
	}
	target := filepath.Join(dst, clean)
	rel, err := filepath.Rel(dst, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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

func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return err
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
		return diag.DuplicateEntry.Errorf("refusing to extract %q: the archive already wrote that file (names differing only in case are one file on some filesystems)", filepath.Base(target))
	}
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxFileBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > maxFileBytes {
		return diag.ArchiveTooLarge.Errorf("refusing to extract %q: file exceeds %d bytes", filepath.Base(target), maxFileBytes)
	}
	return nil
}
