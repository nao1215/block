package block

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nao1215/block/internal/lockfile"
	"github.com/nao1215/block/internal/manifest"
	"github.com/nao1215/block/internal/platform"
	"github.com/nao1215/block/internal/store"
)

// ErrNotReady is returned by Status when the toolchain is not the one
// block.toml asks for, or is not installed. It is a result rather than an
// error — the report was produced, and it says something has to be done —
// which is why the command that returns it exits 2 like `lock --check`
// rather than 1.
var ErrNotReady = errors.New("the toolchain is not ready")

// State is what status says about one tool. There are four, because there
// are four answers to "what do I do about it": nothing, `block lock`,
// `block sync`, and `block sync` again.
type State string

// The states one tool can be in.
const (
	// StateOK is a tool the lockfile pins as block.toml asks and the store
	// has installed and usable.
	StateOK State = "ok"
	// StateMissing is a locked tool that is not installed for this platform.
	StateMissing State = "missing"
	// StateDamaged is an install that is there but cannot be used: the
	// completion marker or an executable is gone.
	StateDamaged State = "damaged"
	// StateStale is a pin block.toml no longer describes — or does not
	// describe yet. Only `block lock` moves a pin, so only it can fix one.
	StateStale State = "stale"
)

// ToolStatus is one row of the report: what was asked for, what was resolved,
// what is on disk, and what to do about the difference.
type ToolStatus struct {
	// Name is the tool's name in block.toml and block.lock.
	Name string `json:"name"`
	// Wanted is the constraint block.toml declares, or "" for a pin
	// block.toml no longer declares at all.
	Wanted string `json:"wanted"`
	// Locked is the version block.lock pins, or "" when it pins none.
	Locked string `json:"locked"`
	// Installed is the version installed and usable for this platform, or ""
	// when nothing is.
	Installed string `json:"installed"`
	// State is the one-word answer; Detail says which of the several ways it
	// happened, and is "" when the state says everything.
	State  State  `json:"state"`
	Detail string `json:"detail"`
	// Dir is the install directory, when there is an install to point at.
	Dir string `json:"dir"`
}

// Status is the whole report: one row per tool named by either file, plus
// what the answers depend on.
type Status struct {
	// Platform is the machine the report is about. Every install answer is
	// for this platform and no other.
	Platform string `json:"platform"`
	// Manifest and Lock are the files the report was read from. Lock is
	// reported even when it does not exist, because "which file is missing"
	// is the useful half of that answer.
	Manifest string `json:"manifest"`
	Lock     string `json:"lock"`
	// LockExists says whether block.lock is there at all.
	LockExists bool `json:"lock_exists"`
	// Tools are the rows, sorted by name.
	Tools []ToolStatus `json:"tools"`
	// Notes are the things worth saying that are not about one tool. Never
	// null: a caller reading JSON gets an empty list.
	Notes []string `json:"notes"`
	// Ready is true when every tool is [StateOK], which is the same thing as
	// "block exec would run and block sync would install nothing".
	Ready bool `json:"ready"`
}

// Status reports what block.toml, block.lock and the store say, and changes
// none of them.
//
// It resolves nothing, downloads nothing, installs nothing and writes
// nothing — not even the shims — so it answers the same way with the network
// unplugged as with it. Everything it reports is read through the same code
// the other commands act on: [Disagreements] for what the two files disagree
// about, and the store for what is on disk.
//
// A missing block.toml is an error, because there is no project to report on.
// A missing block.lock is not: that a project has never been locked is a
// state, and reporting it is the job.
func (a *App) Status() (*Status, error) {
	m, err := a.loadManifest()
	if err != nil {
		return nil, err
	}
	l, err := a.loadLock()
	if err != nil {
		return nil, err
	}
	s := &Status{
		Platform:   a.Platform.String(),
		Manifest:   a.ManifestPath(),
		Lock:       a.LockPath(),
		LockExists: l != nil,
		Notes:      []string{},
		Ready:      true,
	}
	if l == nil {
		// Everything below reads the lockfile. An empty one answers every
		// question the same way — "not in block.lock" — which is the truth
		// and keeps one code path instead of two.
		l = &lockfile.Lock{Version: lockfile.FormatVersion}
	}
	// The one thing that is not about a single tool: a manifest that does not
	// name this machine cannot be fixed by locking again, and every row below
	// would otherwise point that way.
	if err := platformNotDeclared(m, a.Platform); err != nil {
		s.Notes = append(s.Notes, err.Error())
	}
	stale := map[string][]string{}
	for _, d := range Disagreements(m, l, []platform.Platform{a.Platform}) {
		stale[d.Tool] = append(stale[d.Tool], d.Short)
	}
	for _, t := range m.Tools {
		s.Tools = append(s.Tools, a.toolStatus(t.Name, t.Constraint.String(), l, stale[t.Name]))
	}
	for _, e := range l.Tools {
		if _, ok := m.Tool(e.Name); !ok {
			s.Tools = append(s.Tools, a.toolStatus(e.Name, "", l, stale[e.Name]))
		}
	}
	sort.Slice(s.Tools, func(i, j int) bool { return s.Tools[i].Name < s.Tools[j].Name })
	for _, t := range s.Tools {
		if t.State != StateOK {
			s.Ready = false
			break
		}
	}
	return s, nil
}

// toolStatus fills in one row. stale is what the two files disagree about for
// this tool, which decides the state before anything on disk does: a pin
// block.toml no longer describes is not made right by having been installed.
func (a *App) toolStatus(name, wanted string, l *lockfile.Lock, stale []string) ToolStatus {
	row := ToolStatus{Name: name, Wanted: wanted, State: StateOK}
	e, locked := l.Tool(name)
	if locked {
		row.Locked = e.Version
	}
	installed, dir, damaged := a.installState(e)
	switch {
	case len(stale) > 0:
		row.State, row.Detail = StateStale, strings.Join(stale, ", ")
	case damaged != nil:
		// The store phrases its answer for the sentence sync and exec print
		// ("hermes 1.13.0 is damaged: …"), and the column beside this one has
		// already said which tool and that it is damaged. Anything phrased
		// another way is carried whole rather than guessed at.
		row.State, row.Detail = StateDamaged, strings.TrimPrefix(damaged.Error(), "is damaged: ")
	case !installed:
		row.State = StateMissing
	}
	if installed {
		row.Installed, row.Dir = e.Version, dir
	}
	return row
}

// installState reports what the store has for a locked tool on this platform:
// whether it is installed and usable, where it is, and why it is not when it
// is not. A tool the lockfile does not pin — a nil entry — has nothing to
// look for.
func (a *App) installState(e *lockfile.Tool) (installed bool, dir string, damaged error) {
	if e == nil {
		return false, "", nil
	}
	art, ok := e.Artifact(a.Platform)
	if !ok {
		return false, "", nil
	}
	dir, err := a.Store.InstallDir(e.Name, e.Version, art.SHA256)
	if err != nil {
		return false, "", err
	}
	switch err := a.Store.Verify(dir, e.Bin); {
	case err == nil:
		return true, dir, nil
	case errors.Is(err, store.ErrNotInstalled):
		return false, "", nil
	default:
		return false, "", err
	}
}

// Hint is the sentence a report ends on: the command that would move it
// forward, or nothing when there is nothing to do. Which command it names is
// decided by the states present, not by the first row: a stale pin has to be
// locked before installing it means anything.
func (s *Status) Hint() string {
	var stale, install bool
	for _, t := range s.Tools {
		switch t.State {
		case StateStale:
			stale = true
		case StateMissing, StateDamaged:
			install = true
		case StateOK:
		}
	}
	switch {
	case stale:
		return fmt.Sprintf("run \"block lock\" to bring %s up to date with %s", lockfile.FileName, manifest.FileName)
	case install:
		return "run \"block sync\" to install the locked toolchain"
	}
	return ""
}
