package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nao1215/block/internal/diag"
)

type member struct {
	name    string
	content string
	mode    int64
	typ     byte
	link    string
}

func tarGz(t *testing.T, members ...member) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, m := range members {
		typ := m.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{Name: m.name, Mode: m.mode, Typeflag: typ, Linkname: m.link}
		if typ == tar.TypeReg {
			hdr.Size = int64(len(m.content))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write([]byte(m.content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func zipFile(t *testing.T, members ...member) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range members {
		hdr := &zip.FileHeader{Name: m.name, Method: zip.Deflate}
		mode := os.FileMode(m.mode)
		if m.link != "" {
			mode |= os.ModeSymlink
		}
		if strings.HasSuffix(m.name, "/") {
			mode |= os.ModeDir
		}
		hdr.SetMode(mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatal(err)
		}
		content := m.content
		if m.link != "" {
			content = m.link
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	_ = zw.Close()
	path := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractTarGz(t *testing.T) {
	t.Parallel()
	src := tarGz(t,
		member{name: "dir/", typ: tar.TypeDir, mode: 0o755},
		member{name: "dir/forge", content: "#!/bin/sh\necho forge\n", mode: 0o755},
		member{name: "README", content: "hi", mode: 0o600},
		member{name: "pax", typ: tar.TypeXGlobalHeader},
	)
	dst := t.TempDir()
	if err := Extract(src, dst, "x.tar.gz", 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "dir", "forge"))
	if err != nil || !strings.Contains(string(data), "echo forge") {
		t.Fatalf("forge = %q, %v", data, err)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(dst, "dir", "forge"))
		if st.Mode().Perm() != 0o755 {
			t.Errorf("forge mode = %v, want 0755", st.Mode().Perm())
		}
		st, _ = os.Stat(filepath.Join(dst, "README"))
		if st.Mode().Perm() != 0o644 {
			t.Errorf("README mode = %v, want 0644 (archive permissions are normalised)", st.Mode().Perm())
		}
	}
}

func TestExtractZip(t *testing.T) {
	t.Parallel()
	src := zipFile(t,
		member{name: "bin/", mode: 0o755},
		member{name: "bin/tool", content: "#!/bin/sh\n", mode: 0o755},
		member{name: "doc.txt", content: "d", mode: 0o644},
	)
	dst := t.TempDir()
	if err := Extract(src, dst, "x.zip", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "bin", "tool")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "doc.txt")); err != nil {
		t.Error(err)
	}
}

func TestExtractRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  func(*testing.T) string
		ext  string
		want string
	}{
		{"tar traversal", func(t *testing.T) string { return tarGz(t, member{name: "../x", content: "x", mode: 0o644}) }, "a.tar.gz", "path escapes the destination"},
		{"tar nested traversal", func(t *testing.T) string { return tarGz(t, member{name: "a/../../x", content: "x", mode: 0o644}) }, "a.tar.gz", "path escapes the destination"},
		{"tar absolute", func(t *testing.T) string { return tarGz(t, member{name: "/etc/passwd", content: "x", mode: 0o644}) }, "a.tar.gz", "path escapes the destination"},
		{"tar symlink out of the destination", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeSymlink, link: "/etc/passwd"})
		}, "a.tar.gz", "links to the absolute path"},
		{"tar symlink climbing out", func(t *testing.T) string {
			return tarGz(t, member{name: "bin/l", typ: tar.TypeSymlink, link: "../../../etc/passwd"})
		}, "a.tar.gz", "outside the destination"},
		{"tar symlink to a windows path", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeSymlink, link: `C:\Windows\System32`})
		}, "a.tar.gz", "may not name a drive"},
		{"tar symlink to nothing", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeSymlink, link: ""})
		}, "a.tar.gz", "links to nothing"},
		// Each half of the Windows check on its own: a backslash with no
		// drive, and a drive with forward slashes.
		{"tar symlink with a backslash", func(t *testing.T) string {
			return tarGz(t, member{name: "bin/l", typ: tar.TypeSymlink, link: `..\etc\passwd`})
		}, "a.tar.gz", "may not name a drive"},
		{"tar symlink to a drive with forward slashes", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeSymlink, link: "C:/Windows/System32"})
		}, "a.tar.gz", "may not name a drive"},
		// Exactly the parent of the destination, not a path below it.
		{"tar symlink to the parent itself", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeSymlink, link: ".."})
		}, "a.tar.gz", "outside the destination"},
		{"tar symlink to the parent via a subdirectory", func(t *testing.T) string {
			return tarGz(t, member{name: "bin/l", typ: tar.TypeSymlink, link: "../.."})
		}, "a.tar.gz", "outside the destination"},
		// A tar header cannot carry a NUL, but a zip symlink is a file whose
		// contents are the target, and a file can.
		{"zip symlink with a NUL in the target", func(t *testing.T) string {
			return zipFile(t, member{name: "l", mode: 0o777, link: "x\x00y"})
		}, "a.zip", "contains a NUL"},
		{"tar hardlink to a member the archive never wrote", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeLink, link: "etc"})
		}, "a.tar.gz", "the archive did not write"},
		{"tar hardlink out of the destination", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeLink, link: "../../../etc/passwd"})
		}, "a.tar.gz", "outside the destination"},
		{"tar fifo", func(t *testing.T) string { return tarGz(t, member{name: "f", typ: tar.TypeFifo}) }, "a.tar.gz", "unsupported entry type"},
		{"tar empty name", func(t *testing.T) string { return tarGz(t, member{name: "", content: "x", mode: 0o644}) }, "a.tar.gz", "empty name"},
		{"tar character device", func(t *testing.T) string {
			return tarGz(t, member{name: "null", typ: tar.TypeChar})
		}, "a.tar.gz", "unsupported entry type"},
		{"tar block device", func(t *testing.T) string {
			return tarGz(t, member{name: "disk", typ: tar.TypeBlock})
		}, "a.tar.gz", "unsupported entry type"},
		// Refused on every platform, not only on Windows: an archive naming a
		// drive or a share is the same archive wherever it is unpacked.
		{"tar drive letter", func(t *testing.T) string {
			return tarGz(t, member{name: `C:\Windows\x`, content: "x", mode: 0o644})
		}, "a.tar.gz", "may not name a drive"},
		{"tar lower-case drive letter", func(t *testing.T) string {
			return tarGz(t, member{name: "c:/Windows/x", content: "x", mode: 0o644})
		}, "a.tar.gz", "may not name a drive"},
		{"tar unc path", func(t *testing.T) string {
			return tarGz(t, member{name: `\\server\share\x`, content: "x", mode: 0o644})
		}, "a.tar.gz", "may not name a drive"},
		{"tar backslash separator", func(t *testing.T) string {
			return tarGz(t, member{name: `sub\..\..\x`, content: "x", mode: 0o644})
		}, "a.tar.gz", "may not name a drive"},
		{"zip drive letter", func(t *testing.T) string {
			return zipFile(t, member{name: `C:\Windows\x`, content: "x", mode: 0o644})
		}, "a.zip", "may not name a drive"},
		{"zip unc path", func(t *testing.T) string {
			return zipFile(t, member{name: `\\server\share\x`, content: "x", mode: 0o644})
		}, "a.zip", "may not name a drive"},
		{"tar writes one file twice", func(t *testing.T) string {
			return tarGz(t,
				member{name: "tool", content: "first", mode: 0o644},
				member{name: "tool", content: "second", mode: 0o644})
		}, "a.tar.gz", "already wrote that file"},
		{"zip traversal", func(t *testing.T) string { return zipFile(t, member{name: "../x", content: "x", mode: 0o644}) }, "a.zip", "path escapes the destination"},
		{"zip symlink out of the destination", func(t *testing.T) string { return zipFile(t, member{name: "l", link: "/etc/passwd", mode: 0o777}) }, "a.zip", "links to the absolute path"},
		{"unknown format", func(t *testing.T) string { return tarGz(t, member{name: "a", content: "x", mode: 0o644}) }, "a.tar.xz", "unsupported archive format"},
		{"not gzip", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "x")
			_ = os.WriteFile(p, []byte("plain"), 0o600)
			return p
		}, "a.tar.gz", "invalid gzip archive"},
		{"not zip", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "x")
			_ = os.WriteFile(p, []byte("plain"), 0o600)
			return p
		}, "a.zip", "invalid zip archive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dst := t.TempDir()
			err := Extract(tt.src(t), dst, tt.ext, 0)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Extract() error = %v, want containing %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(dst), "x")); err == nil {
				t.Error("a file escaped the destination")
			}
		})
	}
}

func TestExtractRefusesSymlinkedParent(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	dst := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dst, "lib")); err != nil {
		t.Fatal(err)
	}
	src := tarGz(t, member{name: "lib/evil", content: "x", mode: 0o644})
	err := Extract(src, dst, "a.tar.gz", 0)
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("Extract() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "evil")); err == nil {
		t.Error("wrote through a symlinked parent")
	}
}

func TestExtractDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "a"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := tarGz(t, member{name: "a", content: "new", mode: 0o644})
	if err := Extract(src, dst, "a.tar.gz", 0); err == nil {
		t.Fatal("Extract overwrote an existing file")
	}
}

func TestExtractStripComponents(t *testing.T) {
	t.Parallel()
	src := tarGz(t,
		member{name: "geth-linux-amd64-1.17.5-abcdef12/", typ: tar.TypeDir, mode: 0o755},
		member{name: "geth-linux-amd64-1.17.5-abcdef12/geth", content: "#!/bin/sh\n", mode: 0o755},
		member{name: "geth-linux-amd64-1.17.5-abcdef12/COPYING", content: "gpl", mode: 0o644},
		member{name: "toplevel-only", content: "dropped", mode: 0o644},
	)
	dst := t.TempDir()
	if err := Extract(src, dst, "a.tar.gz", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "geth")); err != nil {
		t.Errorf("geth not at the top level: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "toplevel-only")); err == nil {
		t.Error("a member with too few components must be dropped")
	}
	// Stripping cannot be used to escape: "dir/../../x" is still refused.
	evil := tarGz(t, member{name: "dir/../../x", content: "x", mode: 0o644})
	if err := Extract(evil, t.TempDir(), "a.tar.gz", 1); err == nil {
		t.Error("traversal after stripping accepted")
	}
	zsrc := zipFile(t, member{name: "pkg/", mode: 0o755}, member{name: "pkg/tool", content: "x", mode: 0o755})
	zdst := t.TempDir()
	if err := Extract(zsrc, zdst, "a.zip", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(zdst, "tool")); err != nil {
		t.Errorf("zip strip failed: %v", err)
	}
}

// TestExtractTarBz2 uses a committed bzip2 archive because Go's standard
// library can only read that format, not write it. Agave (the Solana CLI
// suite) ships exactly this shape: tools under a versioned directory.
func TestExtractTarBz2(t *testing.T) {
	t.Parallel()
	dst := t.TempDir()
	if err := Extract(filepath.Join("testdata", "pkg.tar.bz2"), dst, "pkg.tar.bz2", 1); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dst, "bin", "tool"))
	if err != nil {
		t.Fatalf("bin/tool: %v", err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o755 {
		t.Errorf("bin/tool mode = %v", st.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dst, "README")); err != nil {
		t.Errorf("README: %v", err)
	}
	// The same file under its .tbz2 spelling, without stripping.
	dst2 := t.TempDir()
	if err := Extract(filepath.Join("testdata", "pkg.tar.bz2"), dst2, "pkg.tbz2", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst2, "pkg-1.0", "bin", "tool")); err != nil {
		t.Errorf("unstripped layout: %v", err)
	}
	// A corrupt bzip2 stream must fail, not panic.
	broken := filepath.Join(t.TempDir(), "x.tar.bz2")
	if err := os.WriteFile(broken, []byte("BZh9 not really bzip2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Extract(broken, t.TempDir(), "x.tar.bz2", 0); err == nil {
		t.Error("a corrupt bzip2 archive was accepted")
	}
}

// safePath is the one check between a member name in an archive somebody
// else built and a file under $BLOCK_HOME, so any name it accepts must land
// strictly inside the destination, whatever strip did to it first.
func FuzzSafePath(f *testing.F) {
	for _, s := range []string{"forge", "bin/forge", "../x", "/etc/passwd", "a/../../b", "a//b", "./a", "", "a\\b", "C:x", "..", ".", "foundry-1.0/bin/forge", "x/./y", "a/b/../../.."} {
		f.Add(s, 0)
		f.Add(s, 1)
	}
	f.Fuzz(func(t *testing.T, name string, strip int) {
		if strip < 0 || strip > 8 {
			return
		}
		dst := filepath.Join(t.TempDir(), "dst")
		stripped, ok := stripName(name, strip)
		if !ok {
			return
		}
		target, err := safePath(dst, stripped)
		if err != nil {
			return
		}
		if stripped == "" {
			t.Fatalf("safePath accepted the empty name stripName(%q, %d) left", name, strip)
		}
		rel, err := filepath.Rel(dst, target)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			t.Fatalf("safePath(%q -> %q) = %q is not strictly inside %q (rel %q)", name, stripped, target, dst, rel)
		}
		if strings.ContainsAny(target, "\x00") {
			t.Fatalf("safePath(%q) = %q carries a NUL", name, target)
		}
	})
}

// The per-file cap alone does not stop an archive from filling the disk: many
// small members do it just as well as one enormous one. The aggregate budget
// is what makes the package's stated invariant true, so it is checked at its
// own boundary rather than by unpacking two hundred thousand files.
func TestBudgetRefusesAnArchiveThatIsTooLargeAsAWhole(t *testing.T) {
	t.Parallel()
	left := newBudget()
	for range maxEntries {
		if err := left.entry("f"); err != nil {
			t.Fatalf("entry within the allowance = %v", err)
		}
	}
	err := left.entry("one-too-many")
	if err == nil || !strings.Contains(err.Error(), "more than") || diag.Of(err) != diag.ArchiveTooLarge {
		t.Errorf("entry past the allowance = %v (%v)", err, diag.Of(err))
	}

	left = newBudget()
	if err := left.wrote("big", maxTotalBytes); err != nil {
		t.Fatalf("wrote the whole allowance = %v", err)
	}
	err = left.wrote("one-byte-more", 1)
	if err == nil || !strings.Contains(err.Error(), "more than") || diag.Of(err) != diag.ArchiveTooLarge {
		t.Errorf("wrote past the allowance = %v (%v)", err, diag.Of(err))
	}

	// Many members that are individually small still spend the same budget:
	// a per-file limit would let every one of these through.
	left = newBudget()
	var err2 error
	for i := 0; err2 == nil && i < 16; i++ {
		err2 = left.wrote("chunk", maxTotalBytes/8)
	}
	if err2 == nil {
		t.Error("sixteen half-gigabyte members fit in the whole-archive allowance")
	}
}

// Extraction of an ordinary archive spends the budget without tripping it.
func TestExtractStaysWithinTheBudget(t *testing.T) {
	t.Parallel()
	src := tarGz(t,
		member{name: "forge", content: "#!/bin/sh\n", mode: 0o755},
		member{name: "bin/cast", content: "#!/bin/sh\n", mode: 0o755},
	)
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Extract(src, dst, "a.tar.gz", 0); err != nil {
		t.Fatalf("Extract() = %v", err)
	}
	for _, name := range []string{"forge", filepath.Join("bin", "cast")} {
		if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Real distributions link inside their own directory: Nethermind ships
// Nethermind.Runner pointing at the executable beside it, and a versioned
// shared library is almost always a name and a link. Refusing every link
// meant refusing those archives outright; what has to be refused is a link
// that leaves the destination, which the table above covers.
func TestExtractKeepsLinksThatStayInside(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		archive func(t *testing.T) string
		file    string
	}{
		{
			"tar symlink beside its target",
			func(t *testing.T) string {
				return tarGz(t,
					member{name: "nethermind", content: "#!/bin/sh\nexit 0\n", mode: 0o755},
					member{name: "Nethermind.Runner", typ: tar.TypeSymlink, link: "nethermind"},
				)
			},
			"a.tar.gz",
		},
		{
			"tar symlink into a subdirectory",
			func(t *testing.T) string {
				return tarGz(t,
					member{name: "lib/libfoo.so.1", content: "binary\n", mode: 0o644},
					member{name: "bin/libfoo.so", typ: tar.TypeSymlink, link: "../lib/libfoo.so.1"},
					member{name: "nethermind", content: "#!/bin/sh\nexit 0\n", mode: 0o755},
					member{name: "Nethermind.Runner", typ: tar.TypeSymlink, link: "nethermind"},
				)
			},
			"a.tar.gz",
		},
		{
			"tar hard link to a member already written",
			func(t *testing.T) string {
				return tarGz(t,
					member{name: "nethermind", content: "#!/bin/sh\nexit 0\n", mode: 0o755},
					member{name: "Nethermind.Runner", typ: tar.TypeLink, link: "nethermind"},
				)
			},
			"a.tar.gz",
		},
		{
			"zip symlink beside its target",
			func(t *testing.T) string {
				return zipFile(t,
					member{name: "nethermind", content: "#!/bin/sh\nexit 0\n", mode: 0o755},
					member{name: "Nethermind.Runner", link: "nethermind", mode: 0o777},
				)
			},
			"a.zip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dst := filepath.Join(t.TempDir(), "dst")
			if err := os.MkdirAll(dst, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := Extract(tt.archive(t), dst, tt.file, 0); err != nil {
				t.Fatalf("Extract() = %v", err)
			}
			// However the platform made it — link or copy — the name resolves
			// to the same bytes as the file it points at.
			body, err := os.ReadFile(filepath.Join(dst, "Nethermind.Runner"))
			if err != nil {
				t.Fatalf("reading the link: %v", err)
			}
			if string(body) != "#!/bin/sh\nexit 0\n" {
				t.Errorf("the link resolves to %q", body)
			}
			// And it stays inside: nothing followed it out. Both sides are
			// resolved before they are compared, because one directory has
			// more than one spelling — /var is /private/var on macOS, and a
			// Windows temporary path has an 8.3 short name as well as a long
			// one — and the test would otherwise fail on the spelling.
			resolved, err := filepath.EvalSymlinks(filepath.Join(dst, "Nethermind.Runner"))
			if err != nil {
				t.Fatal(err)
			}
			root, err := filepath.EvalSymlinks(dst)
			if err != nil {
				t.Fatal(err)
			}
			if rel, err := filepath.Rel(root, resolved); err != nil || strings.HasPrefix(rel, "..") {
				t.Errorf("the link resolves to %q, outside %q", resolved, root)
			}
		})
	}
}

// A link is where an archive gets to name a path that was never checked as a
// member, so the whole point is that the check happens anyway. A dangling
// symlink inside the destination is fine — archives list members in their own
// order — as long as it could not point out of it.
func TestExtractAllowsADanglingLinkInsideTheDestination(t *testing.T) {
	t.Parallel()
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	src := tarGz(t,
		member{name: "later", typ: tar.TypeSymlink, link: "written-afterwards"},
		member{name: "written-afterwards", content: "here\n", mode: 0o644},
	)
	if err := Extract(src, dst, "a.tar.gz", 0); err != nil {
		t.Fatalf("Extract() = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dst, "later"))
	if err != nil || string(body) != "here\n" {
		t.Errorf("the link resolves to %q (%v)", body, err)
	}
}

// A member that arrives after a link cannot be written through it: the link
// is inside the destination, but what it points at is a directory the
// archive never created.
func TestExtractRefusesWritingThroughALink(t *testing.T) {
	t.Parallel()
	dst := filepath.Join(t.TempDir(), "dst")
	outside := filepath.Join(filepath.Dir(dst), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	// The link itself is refused, so the member behind it never gets a
	// chance; both halves are asserted.
	src := tarGz(t,
		member{name: "escape", typ: tar.TypeSymlink, link: "../outside"},
		member{name: "escape/planted", content: "owned\n", mode: 0o644},
	)
	err := Extract(src, dst, "a.tar.gz", 0)
	if err == nil || !strings.Contains(err.Error(), "outside the destination") {
		t.Fatalf("Extract() = %v, want the link refused", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "planted")); err == nil {
		t.Error("a file was written outside the destination through a link")
	}
}

// A hard link names its target by the member's archive name, so with
// strip_components the link is refused when that name is not under the
// components being dropped — and the message says that, rather than "links
// to nothing".
func TestExtractRefusesAHardLinkOutsideTheStrippedPrefix(t *testing.T) {
	t.Parallel()
	src := tarGz(t,
		member{name: "pkg/bin/tool", content: "#!/bin/sh\nexit 0\n", mode: 0o755},
		member{name: "pkg/bin/alias", typ: tar.TypeLink, link: "tool"},
	)
	err := Extract(src, t.TempDir(), "a.tar.gz", 1)
	if diag.Of(err) != diag.PathEscape || !strings.Contains(err.Error(), `links to "tool", which is not in the archive`) {
		t.Fatalf("err = %v", err)
	}
}

// copyLinked stands in for a link the filesystem refused, and what it
// copies has to stay runnable: the executable bit follows the target.
func TestCopyLinkedKeepsTheExecutableBit(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no executable bit")
	}
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		mode os.FileMode
		exec bool
	}{
		{"tool", 0o755, true},
		{"data", 0o644, false},
	} {
		src := filepath.Join(dir, tc.name)
		if err := os.WriteFile(src, []byte("x"), tc.mode); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, tc.name+".copy")
		if err := copyLinked(src, dst); err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if got := st.Mode()&0o100 != 0; got != tc.exec {
			t.Errorf("%s: executable = %v, want %v (mode %v)", tc.name, got, tc.exec, st.Mode())
		}
	}
}

// The entry allowance counts every member, not only the files: an archive of
// two hundred thousand directories, symlinks or hard links costs the same
// inodes as one of two hundred thousand files, and counting files alone let
// all of them through.
func TestExtractCountsEveryKindOfEntry(t *testing.T) {
	t.Parallel()
	// All the directory members share one name, so the archive is cheap to
	// unpack right up to the refusal; the two links at the end are what an
	// archive that counted files alone would not have counted.
	crowd := func(n int) []member {
		out := []member{{name: "tool", content: "#!/bin/sh\n", mode: 0o755}}
		for range n {
			out = append(out, member{name: "pad/", mode: 0o755, typ: tar.TypeDir})
		}
		return out
	}
	tests := []struct {
		name string
		src  string
		last string
	}{
		{
			name: "tar: directories, a symlink and a hard link",
			src: tarGz(t, append(crowd(maxEntries-2),
				member{name: "Runner", typ: tar.TypeSymlink, link: "tool"},
				member{name: "Alias", typ: tar.TypeLink, link: "tool"})...),
			last: "Alias",
		},
		{
			name: "zip: directories and a symlink",
			src:  zipFile(t, append(crowd(maxEntries-1), member{name: "Runner", link: "tool"})...),
			last: "Runner",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dst := filepath.Join(t.TempDir(), "dst")
			if err := os.MkdirAll(dst, 0o755); err != nil {
				t.Fatal(err)
			}
			name := "a.tar.gz"
			if strings.HasSuffix(tt.src, ".zip") {
				name = "a.zip"
			}
			err := Extract(tt.src, dst, name, 0)
			if err == nil || diag.Of(err) != diag.ArchiveTooLarge || !strings.Contains(err.Error(), "more than") {
				t.Fatalf("Extract() = %v (%v), want the entry allowance refused", err, diag.Of(err))
			}
			// The refusal lands on the last member, which is the one that
			// would not have been counted before.
			if !strings.Contains(err.Error(), `"`+tt.last+`"`) {
				t.Errorf("Extract() = %v, want it to name %q", err, tt.last)
			}
			if _, err := os.Lstat(filepath.Join(dst, tt.last)); err == nil {
				t.Errorf("%s was created past the allowance", tt.last)
			}
		})
	}
}

// A zip link target longer than block reads used to be read up to the limit
// and cut there, and a link made to whatever path the cut left — a path
// nobody wrote. It is refused whole instead.
func TestExtractRefusesAZipLinkTargetLongerThanItReads(t *testing.T) {
	t.Parallel()
	tooLong := strings.Repeat("a", maxLinkBytes+1)
	src := zipFile(t,
		member{name: "tool", content: "#!/bin/sh\n", mode: 0o755},
		member{name: "Runner", link: tooLong},
	)
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Extract(src, dst, "a.zip", 0)
	if err == nil || diag.Of(err) != diag.ArchiveTooLarge || !strings.Contains(err.Error(), "link target is longer than") {
		t.Fatalf("Extract() = %v (%v), want the link target refused", err, diag.Of(err))
	}
	if _, err := os.Lstat(filepath.Join(dst, "Runner")); err == nil {
		t.Error("a link was made from a cut-short target")
	}

	// A long target within the limit is still a target, and the link is
	// made. (Exactly at the limit cannot be tried: the operating systems
	// block runs on refuse a target that long before block sees it.)
	atLimit := strings.Repeat("b", 200)
	src = zipFile(t,
		member{name: "tool", content: "#!/bin/sh\n", mode: 0o755},
		member{name: "Runner", link: atLimit},
	)
	dst = filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	err = Extract(src, dst, "a.zip", 0)
	if err != nil && runtime.GOOS == "windows" && !strings.Contains(err.Error(), "longer than") {
		// Without the symlink privilege the link cannot be made, and the
		// target is not a file to copy in its place: refused for that
		// reason, never for its length.
		return
	}
	if err != nil {
		t.Fatalf("Extract() with a long target within the limit = %v", err)
	}
	if got, err := os.Readlink(filepath.Join(dst, "Runner")); err != nil || got != atLimit {
		t.Errorf("Readlink() = %q, %v", got, err)
	}
}
