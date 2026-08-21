package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tarGz(t *testing.T, files map[string]string, exec bool) string {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		mode := int64(0o644)
		if exec {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Typeflag: tar.TypeReg, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	p := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpen(t *testing.T) {
	t.Setenv(EnvHome, "/custom")
	s, err := Open()
	if err != nil || s.Root != "/custom" {
		t.Errorf("Open() = %v, %v", s, err)
	}
	t.Setenv(EnvHome, "")
	t.Setenv("XDG_DATA_HOME", "/xdg")
	s, err = Open()
	if err != nil || s.Root != filepath.Join("/xdg", "block") {
		t.Errorf("Open() = %v, %v", s, err)
	}
	t.Setenv("XDG_DATA_HOME", "")
	s, err = Open()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if s.Root != filepath.Join(home, ".local", "share", "block") {
		t.Errorf("Open() = %s", s.Root)
	}
	if s.CacheDir() != filepath.Join(s.Root, "cache") {
		t.Errorf("CacheDir() = %s", s.CacheDir())
	}
}

func TestInstallDir(t *testing.T) {
	t.Parallel()
	s := &Store{Root: "/r"}
	got := s.InstallDir("foundry", "1.7.4", "593c607acd4d8fe57f560298f64779441a0aa7461893223def00eeedc612d0bb")
	if got != filepath.Join("/r", "tools", "foundry", "1.7.4-593c607acd4d") {
		t.Errorf("InstallDir() = %s", got)
	}
	if s.InstallDir("x", "1.0.0", "abc") != filepath.Join("/r", "tools", "x", "1.0.0-abc") {
		t.Error("short digests must not be sliced")
	}
}

func TestInstall(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are not tracked on windows")
	}
	s := &Store{Root: t.TempDir()}
	src := tarGz(t, map[string]string{"forge": "#!/bin/sh\n", "bin/cast": "#!/bin/sh\n"}, true)
	dir := s.InstallDir("foundry", "1.7.4", "abcdef0123456789")
	if s.IsInstalled(dir, []string{"forge", "bin/cast"}) {
		t.Fatal("installed before Install")
	}
	if err := s.Install(src, "foundry.tar.gz", dir, []string{"forge", "bin/cast"}, 0); err != nil {
		t.Fatal(err)
	}
	if !s.IsInstalled(dir, []string{"forge", "bin/cast"}) {
		t.Fatal("not installed after Install")
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Errorf("temp dirs left behind: %v", entries)
	}
	// Idempotent.
	if err := s.Install("/nonexistent", "foundry.tar.gz", dir, nil, 0); err != nil {
		t.Errorf("second Install should be a no-op, got %v", err)
	}
	dirs := BinDirs(dir, []string{"forge", "bin/cast", "anvil"})
	if len(dirs) != 2 || dirs[0] != dir || dirs[1] != filepath.Join(dir, "bin") {
		t.Errorf("BinDirs() = %v", dirs)
	}
}

func TestInstallRejectsBadArchives(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are not tracked on windows")
	}
	s := &Store{Root: t.TempDir()}
	tests := []struct {
		name string
		src  string
		bins []string
		want string
	}{
		{"missing bin", tarGz(t, map[string]string{"forge": "x"}, true), []string{"cast"}, `executable "cast" is missing`},
		{"not executable", tarGz(t, map[string]string{"forge": "x"}, false), []string{"forge"}, `"forge" is not an executable file`},
		{"bad archive", filepath.Join(t.TempDir(), "missing.tar.gz"), []string{"forge"}, "extract"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := s.InstallDir(strings.ReplaceAll(tt.name, " ", "-"), "1.0.0", "abcdef012345")
			err := s.Install(tt.src, "t.tar.gz", dir, tt.bins, 0)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Install() error = %v, want containing %q", err, tt.want)
			}
			if s.IsInstalled(dir, []string{"forge", "bin/cast"}) {
				t.Error("a failed install left the directory in place")
			}
			entries, _ := os.ReadDir(filepath.Dir(dir))
			if len(entries) != 0 {
				t.Errorf("temp dirs left behind: %v", entries)
			}
		})
	}
}

func TestInstallRawExecutable(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are not tracked on windows")
	}
	s := &Store{Root: t.TempDir()}
	src := filepath.Join(t.TempDir(), "solc-static-linux")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho solc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := s.InstallDir("solc", "0.8.30", "abcdef012345")
	if err := s.Install(src, "solc-static-linux", dir, []string{"solc"}, 0); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "solc"))
	if err != nil || st.Mode()&0o100 == 0 {
		t.Errorf("solc = %v, %v", st, err)
	}
	bad := s.InstallDir("solc", "0.8.31", "abcdef012345")
	if err := s.Install(src, "solc-static-linux", bad, []string{"a", "b"}, 0); err == nil || !strings.Contains(err.Error(), "exactly one bin name") {
		t.Errorf("Install(two bins) error = %v", err)
	}
}

func TestIsInstalledRejectsDamagedInstalls(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are not tracked on windows")
	}
	bins := []string{"forge", "bin/cast"}
	src := tarGz(t, map[string]string{"forge": "#!/bin/sh\n", "bin/cast": "#!/bin/sh\n"}, true)

	tests := []struct {
		name   string
		damage func(t *testing.T, dir string)
	}{
		{"marker removed", func(t *testing.T, dir string) {
			// What a half-restored CI cache looks like: the files are there
			// but the install never completed.
			if err := os.Remove(filepath.Join(dir, markerName)); err != nil {
				t.Fatal(err)
			}
		}},
		{"executable removed", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "bin", "cast")); err != nil {
				t.Fatal(err)
			}
		}},
		{"executable not executable", func(t *testing.T, dir string) {
			if err := os.Chmod(filepath.Join(dir, "forge"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"executable replaced by a directory", func(t *testing.T, dir string) {
			p := filepath.Join(dir, "forge")
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(p, 0o750); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Store{Root: t.TempDir()}
			dir := s.InstallDir("foundry", "1.7.4", "abcdef012345")
			if err := s.Install(src, "foundry.tar.gz", dir, bins, 0); err != nil {
				t.Fatal(err)
			}
			if !s.IsInstalled(dir, bins) || s.Verify(dir, bins) != nil {
				t.Fatal("a fresh install did not verify")
			}
			tt.damage(t, dir)
			if s.IsInstalled(dir, bins) {
				t.Error("a damaged install was reported as installed")
			}
			if err := s.Verify(dir, bins); err == nil {
				t.Error("Verify accepted a damaged install")
			}
			// Installing again repairs it instead of leaving the wreck.
			if err := s.Install(src, "foundry.tar.gz", dir, bins, 0); err != nil {
				t.Fatal(err)
			}
			if !s.IsInstalled(dir, bins) {
				t.Error("the install was not repaired")
			}
			entries, _ := os.ReadDir(filepath.Dir(dir))
			if len(entries) != 1 {
				t.Errorf("temp dirs left behind: %v", entries)
			}
		})
	}
}

func TestInstallRejectsUnsafeBinEntries(t *testing.T) {
	t.Parallel()
	raw := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(raw, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	archived := tarGz(t, map[string]string{"tool": "#!/bin/sh\n"}, true)
	outside := t.TempDir()
	escape := filepath.Join(outside, "pwned")

	for _, tt := range []struct {
		name string
		src  string
		bins []string
	}{
		{"raw traversal", raw, []string{"../pwned"}},
		{"raw absolute", raw, []string{escape}},
		{"raw nested traversal", raw, []string{"a/../../pwned"}},
		{"archive traversal", archived, []string{"../pwned"}},
		{"empty", raw, []string{""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Store{Root: t.TempDir()}
			dir := s.InstallDir("tool", "1.0.0", "abcdef012345")
			if err := s.Install(tt.src, "tool", dir, tt.bins, 0); err == nil {
				t.Fatal("an unsafe bin entry was accepted")
			}
			if _, err := os.Stat(escape); err == nil {
				t.Fatal("a file was written outside the install directory")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(s.Root), "pwned")); err == nil {
				t.Fatal("a file was written outside the store")
			}
		})
	}
}
