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
			t.Errorf("README mode = %v, want 0644 (archive permissions are normalized)", st.Mode().Perm())
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
		{"tar symlink", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeSymlink, link: "/etc/passwd"})
		}, "a.tar.gz", "links are not supported"},
		{"tar hardlink", func(t *testing.T) string {
			return tarGz(t, member{name: "l", typ: tar.TypeLink, link: "etc"})
		}, "a.tar.gz", "links are not supported"},
		{"tar fifo", func(t *testing.T) string { return tarGz(t, member{name: "f", typ: tar.TypeFifo}) }, "a.tar.gz", "unsupported entry type"},
		{"tar empty name", func(t *testing.T) string { return tarGz(t, member{name: "", content: "x", mode: 0o644}) }, "a.tar.gz", "empty name"},
		{"zip traversal", func(t *testing.T) string { return zipFile(t, member{name: "../x", content: "x", mode: 0o644}) }, "a.zip", "path escapes the destination"},
		{"zip symlink", func(t *testing.T) string { return zipFile(t, member{name: "l", link: "/etc/passwd", mode: 0o777}) }, "a.zip", "links are not supported"},
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
