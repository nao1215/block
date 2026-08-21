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
	if runtime.GOOS != "windows" {
		// The XDG base directory specification, where Unix keeps user-local
		// application data.
		t.Setenv("XDG_DATA_HOME", "/xdg")
		s, err = Open()
		if err != nil || s.Root != filepath.Join("/xdg", "block") {
			t.Errorf("Open() = %v, %v", s, err)
		}
		t.Setenv("XDG_DATA_HOME", "")
	}
	s, err = Open()
	if err != nil {
		t.Fatal(err)
	}
	want, err := defaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if s.Root != want {
		t.Errorf("Open() = %s, want %s", s.Root, want)
	}
	if runtime.GOOS != "windows" {
		home, _ := os.UserHomeDir()
		if want != filepath.Join(home, ".local", "share", "block") {
			t.Errorf("the Unix default moved: %s", want)
		}
	}
	if s.CacheDir() != filepath.Join(s.Root, "cache") {
		t.Errorf("CacheDir() = %s", s.CacheDir())
	}
}

func TestInstallDir(t *testing.T) {
	t.Parallel()
	s := &Store{Root: "/r"}
	got, err := s.InstallDir("foundry", "1.7.4", "593c607acd4d8fe57f560298f64779441a0aa7461893223def00eeedc612d0bb")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("/r", "tools", "foundry", "1.7.4-593c607acd4d") {
		t.Errorf("InstallDir() = %s", got)
	}
	short, err := s.InstallDir("x", "1.0.0", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if short != filepath.Join("/r", "tools", "x", "1.0.0-abc") {
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
	dir, err := s.InstallDir("foundry", "1.7.4", "abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
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

// Some upstreams package their binary without the executable bit (Lotus
// ships lotus as 0644). block declared that path an executable, so it makes
// it one — and touches nothing else in the archive.
func TestInstallSetsTheExecutableBitOnDeclaredBins(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are not tracked on windows")
	}
	s := &Store{Root: t.TempDir()}
	src := tarGz(t, map[string]string{"lotus": "x", "README": "docs"}, false)
	dir, err := s.InstallDir("lotus", "1.36.2", "abcdef012345")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(src, "lotus.tar.gz", dir, []string{"lotus"}, 0); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	st, err := os.Stat(filepath.Join(dir, "lotus"))
	if err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Errorf("lotus mode = %v, %v", st.Mode(), err)
	}
	// The bit is given to what the recipe named, not to the whole archive.
	if st, err := os.Stat(filepath.Join(dir, "README")); err != nil || st.Mode().Perm()&0o111 != 0 {
		t.Errorf("README mode = %v, %v", st.Mode(), err)
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
		// A directory where an executable was promised cannot be repaired
		// by setting a bit, so it is still refused.
		{"not a file", tarGz(t, map[string]string{"forge/inner": "x"}, true), []string{"forge"}, `"forge" is not an executable file`},
		{"bad archive", filepath.Join(t.TempDir(), "missing.tar.gz"), []string{"forge"}, "extract"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, err := s.InstallDir(strings.ReplaceAll(tt.name, " ", "-"), "1.0.0", "abcdef012345")
			if err != nil {
				t.Fatal(err)
			}
			err = s.Install(tt.src, "t.tar.gz", dir, tt.bins, 0)
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
	dir, err := s.InstallDir("solc", "0.8.30", "abcdef012345")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(src, "solc-static-linux", dir, []string{"solc"}, 0); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "solc"))
	if err != nil || st.Mode()&0o100 == 0 {
		t.Errorf("solc = %v, %v", st, err)
	}
	bad, err := s.InstallDir("solc", "0.8.31", "abcdef012345")
	if err != nil {
		t.Fatal(err)
	}
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
			dir, err := s.InstallDir("foundry", "1.7.4", "abcdef012345")
			if err != nil {
				t.Fatal(err)
			}
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
			dir, err := s.InstallDir("tool", "1.0.0", "abcdef012345")
			if err != nil {
				t.Fatal(err)
			}
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

// InstallDir builds a path this package creates, populates and removes, from
// a name and a version that both arrive through block.lock. The version parser
// closed the hole that let "1.7/../../outside" through; this is the second
// wall, and it has to hold on its own.
func TestInstallDirRefusesToEscapeTheStore(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "keep-me")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Root: filepath.Join(root, "home")}

	tests := []struct {
		name string
		tool string
		ver  string
	}{
		{"version climbs out with unix separators", "foundry", "1.7/../../../outside"},
		{"version climbs out with the OS separator", "foundry", filepath.Join("1.7", "..", "..", "..", "outside")},
		{"version is a parent reference", "foundry", ".."},
		{"version is an absolute path", "foundry", filepath.Join(root, "outside")},
		{"tool name climbs out", filepath.Join("..", "..", "outside"), "1.7.0"},
		{"tool name is absolute", filepath.Join(root, "outside"), "1.7.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, err := s.InstallDir(tt.tool, tt.ver, "abcdef012345")
			if err == nil {
				t.Fatalf("InstallDir(%q, %q) = %q, want an error", tt.tool, tt.ver, dir)
			}
			if dir != "" {
				t.Errorf("InstallDir returned %q alongside its error", dir)
			}
		})
	}

	// Nothing was created and nothing was removed: refusing has to happen
	// before any filesystem call, not after a directory has been made.
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "untouched" {
		t.Errorf("the sentinel outside the store changed: %q, %v", got, err)
	}
	if _, err := os.Stat(s.Root); !os.IsNotExist(err) {
		t.Errorf("the store root was created by a refused install: %v", err)
	}
}

// The path it does build has to be the one the rest of the store expects.
func TestInstallDirStaysUnderTools(t *testing.T) {
	t.Parallel()
	s := &Store{Root: t.TempDir()}
	dir, err := s.InstallDir("foundry", "1.7.1", "abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(s.Root, "tools", "foundry", "1.7.1-abcdef012345")
	if dir != want {
		t.Errorf("InstallDir() = %q, want %q", dir, want)
	}
}

// Windows has a 260-character limit on paths that are not spelled with the
// extended-length prefix, and $BLOCK_HOME can sit deep — inside a runner's
// workspace, inside a user profile, inside a project. The store has to keep
// working there, so this installs into a root long enough that a naive path
// would be refused, and then reads the executable back out of it.
//
// It runs everywhere: the limit is Windows's, but the test is the same
// question on every platform, and a Unix run is what would catch a change
// that broke the layout for reasons other than length.
func TestInstallIntoADeeplyNestedStore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// Each component is well under any per-name limit; it is the total that
	// crosses 260.
	for range 12 {
		root = filepath.Join(root, strings.Repeat("d", 24))
	}
	if len(root) < 260 {
		t.Fatalf("the test root is only %d characters; it no longer tests what it says", len(root))
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Skipf("this filesystem will not create a %d-character path: %v", len(root), err)
	}

	s := &Store{Root: root}
	src := tarGz(t, map[string]string{"bin/deep": "#!/bin/sh\n"}, true)
	dir, err := s.InstallDir("deep", "1.0.0", "abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(src, "deep.tar.gz", dir, []string{"bin/deep"}, 0); err != nil {
		t.Fatalf("installing into a %d-character store: %v", len(dir), err)
	}
	if err := s.Verify(dir, []string{"bin/deep"}); err != nil {
		t.Fatalf("verifying a deeply nested install: %v", err)
	}
	bin, err := BinPath(dir, "bin/deep")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Errorf("the executable is not readable at %d characters: %v", len(bin), err)
	}
}

// Two syncs of the same tool can race — two projects in one CI job, two
// terminals — and on Windows a rename cannot replace a directory that already
// exists. Whichever loses must not report a failure for a tool that is, by
// then, installed and complete.
func TestInstallLosingTheRaceIsNotAnError(t *testing.T) {
	t.Parallel()

	s := &Store{Root: t.TempDir()}
	src := tarGz(t, map[string]string{"bin/deep": "#!/bin/sh\n"}, true)
	dir, err := s.InstallDir("racer", "1.0.0", "abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	bins := []string{"bin/deep"}
	if err := s.Install(src, "racer.tar.gz", dir, bins, 0); err != nil {
		t.Fatal(err)
	}
	// The second install finds a complete directory already there. It has to
	// leave it alone and say nothing went wrong.
	before, err := os.ReadFile(filepath.Join(dir, markerName))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(src, "racer.tar.gz", dir, bins, 0); err != nil {
		t.Errorf("a second install of a complete directory: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, markerName))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the second install rewrote the completion marker of the first")
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Errorf("temporary directories were left behind: %v", entries)
	}
}
