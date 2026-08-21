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
	if s.IsInstalled(dir) {
		t.Fatal("installed before Install")
	}
	if err := s.Install(src, "foundry.tar.gz", dir, []string{"forge", "bin/cast"}); err != nil {
		t.Fatal(err)
	}
	if !s.IsInstalled(dir) {
		t.Fatal("not installed after Install")
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Errorf("temp dirs left behind: %v", entries)
	}
	// Idempotent.
	if err := s.Install("/nonexistent", "foundry.tar.gz", dir, nil); err != nil {
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
		{"missing bin", tarGz(t, map[string]string{"forge": "x"}, true), []string{"cast"}, `does not contain executable "cast"`},
		{"not executable", tarGz(t, map[string]string{"forge": "x"}, false), []string{"forge"}, `"forge" is not an executable file`},
		{"bad archive", filepath.Join(t.TempDir(), "missing.tar.gz"), []string{"forge"}, "extract"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := s.InstallDir(strings.ReplaceAll(tt.name, " ", "-"), "1.0.0", "abcdef012345")
			err := s.Install(tt.src, "t.tar.gz", dir, tt.bins)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Install() error = %v, want containing %q", err, tt.want)
			}
			if s.IsInstalled(dir) {
				t.Error("a failed install left the directory in place")
			}
			entries, _ := os.ReadDir(filepath.Dir(dir))
			if len(entries) != 0 {
				t.Errorf("temp dirs left behind: %v", entries)
			}
		})
	}
}
