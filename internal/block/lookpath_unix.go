//go:build !windows

package block

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LookPath resolves name against an explicit PATH value instead of the
// process environment, so that a toolchain's PATH can be used without
// changing the one block itself runs with.
func LookPath(name, path string) (string, error) {
	if strings.Contains(name, string(os.PathSeparator)) {
		return exec.LookPath(name)
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}
