//go:build windows

package block

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultPathExt is what Windows uses when PATHEXT is unset.
const defaultPathExt = ".COM;.EXE;.BAT;.CMD"

// LookPath resolves name against an explicit PATH value instead of the
// process environment. Windows decides what is executable by extension, so
// each PATHEXT entry is tried in turn — and a name that already carries one
// is taken as it is.
func LookPath(name, path string) (string, error) {
	if strings.ContainsAny(name, `\/:`) {
		return exec.LookPath(name)
	}
	exts := []string{}
	if filepath.Ext(name) != "" {
		exts = append(exts, "")
	}
	pathExt := os.Getenv("PATHEXT")
	if pathExt == "" {
		pathExt = defaultPathExt
	}
	for _, ext := range strings.Split(pathExt, string(os.PathListSeparator)) {
		if ext = strings.TrimSpace(ext); ext != "" {
			exts = append(exts, ext)
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			candidate := filepath.Join(dir, name+ext)
			if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}
