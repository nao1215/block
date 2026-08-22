package block

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// states renders a report the way the assertions below want to read it.
func states(s *Status) string {
	var b strings.Builder
	for _, t := range s.Tools {
		fmt.Fprintf(&b, "%s=%s", t.Name, t.State)
		if t.Detail != "" {
			fmt.Fprintf(&b, "(%s)", t.Detail)
		}
		b.WriteString(" ")
	}
	return strings.TrimSpace(b.String())
}

const statusManifest = "[tools.foundry]\nversion = \"1.7\"\n[tools.foundry.source]\ntype = \"github_release\"\nrepo = \"foundry-rs/foundry\"\nasset = \"foundry_v{version}_{os}_{arch}.tar.gz\"\nbin = [\"forge\", \"cast\", \"anvil\", \"chisel\"]\n"

func TestStatusReportsWhatEachToolIsDoing(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, statusManifest)
	ctx := context.Background()

	// Never locked: the pin does not exist yet, and only lock makes one.
	report, err := h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got := states(report); got != "foundry=stale(not in block.lock)" {
		t.Errorf("before lock: %s", got)
	}
	if report.Ready || report.LockExists {
		t.Errorf("before lock: ready=%v lockExists=%v", report.Ready, report.LockExists)
	}
	if !strings.Contains(report.Hint(), "block lock") {
		t.Errorf("hint = %q, want it to name lock", report.Hint())
	}

	// Locked, not installed.
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	report, err = h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got := states(report); got != "foundry=missing" {
		t.Errorf("after lock: %s", got)
	}
	if report.Tools[0].Locked != "1.7.4" || report.Tools[0].Installed != "" || report.Tools[0].Dir != "" {
		t.Errorf("after lock: %+v", report.Tools[0])
	}
	if !strings.Contains(report.Hint(), "block sync") {
		t.Errorf("hint = %q, want it to name sync", report.Hint())
	}

	// Installed.
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	report, err = h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got := states(report); got != "foundry=ok" {
		t.Errorf("after sync: %s", got)
	}
	if !report.Ready || report.Hint() != "" {
		t.Errorf("after sync: ready=%v hint=%q", report.Ready, report.Hint())
	}
	row := report.Tools[0]
	if row.Wanted != "1.7" || row.Locked != "1.7.4" || row.Installed != "1.7.4" {
		t.Errorf("after sync: %+v", row)
	}
	if _, err := os.Stat(filepath.Join(row.Dir, "forge")); err != nil {
		t.Errorf("dir %q does not hold the executables: %v", row.Dir, err)
	}

	// Damaged: the install is there and one executable is not.
	if err := os.Remove(filepath.Join(row.Dir, "forge")); err != nil {
		t.Fatal(err)
	}
	report, err = h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got := states(report); got != `foundry=damaged(executable "forge" is missing)` {
		t.Errorf("after damage: %s", got)
	}
	if report.Tools[0].Installed != "" {
		t.Errorf("a damaged install was reported as installed: %+v", report.Tools[0])
	}
}

// A pin block.toml no longer describes is stale whatever the store holds:
// installing it again would install the wrong thing, so the state names the
// command that moves the pin rather than the one that installs it.
func TestStatusPutsTheLockAheadOfTheStore(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, statusManifest)
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	h.manifest(t, strings.Replace(statusManifest, "1.7", "1.6", 1))
	report, err := h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got := states(report); got != "foundry=stale(constraint changed)" {
		t.Errorf("states = %s", got)
	}
	// The installed version is still reported: the row says what is there as
	// well as what is wrong with it.
	if report.Tools[0].Installed != "1.7.4" {
		t.Errorf("installed = %q, want the version on disk", report.Tools[0].Installed)
	}
	if !strings.Contains(report.Hint(), "block lock") {
		t.Errorf("hint = %q", report.Hint())
	}
}

// A tool in the lockfile that block.toml does not declare has a row of its
// own: it is what the next lock will drop, and a report that left it out
// would not explain why the store holds it.
func TestStatusReportsAPinNoLongerDeclared(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, statusManifest+"[tools.hermes]\nversion = \"1.13\"\n[tools.hermes.source]\ntype = \"github_release\"\nrepo = \"informalsystems/hermes\"\nasset = \"hermes-v{version}-{arch}-{os}.tar.gz\"\nbin = [\"hermes\"]\n[tools.hermes.source.os]\nlinux = \"unknown-linux-gnu\"\ndarwin = \"apple-darwin\"\nwindows = \"pc-windows-msvc\"\n[tools.hermes.source.arch]\namd64 = \"x86_64\"\narm64 = \"aarch64\"\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	h.manifest(t, statusManifest)
	report, err := h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got := states(report); got != "foundry=missing hermes=stale(not in block.toml)" {
		t.Errorf("states = %s", got)
	}
	if report.Tools[1].Wanted != "" {
		t.Errorf("a pin nobody declares has a constraint: %+v", report.Tools[1])
	}
}

// The report answers for the machine it runs on, and a manifest that does not
// name that machine is the one case re-locking cannot fix. It is said once,
// beside the rows rather than in every one of them.
func TestStatusNotesAManifestThatExcludesThisMachine(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "platforms = [\"darwin/arm64\"]\n"+statusManifest)
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	report, err := h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Notes) != 1 || !strings.Contains(report.Notes[0], "this machine is linux/amd64") {
		t.Fatalf("notes = %v", report.Notes)
	}
	if got := states(report); got != "foundry=stale(no artifact for linux/amd64)" {
		t.Errorf("states = %s", got)
	}
}

// status is a report, not a command that acts: it must leave the project and
// the store exactly as it found them, and it must not need the network to say
// so. The harness's upstream is unreachable here, which is what proves the
// second half.
func TestStatusChangesNothingAndAsksNobody(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fake tools are shell scripts")
	}
	h := newHarness(t, "/t1")
	h.manifest(t, statusManifest)
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := h.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, h.Dir, h.Store.Root)
	h.offline()
	h.reset()
	report, err := h.Status()
	if err != nil {
		t.Fatalf("status needed the network: %v", err)
	}
	if !report.Ready {
		t.Errorf("states = %s", states(report))
	}
	if after := snapshotTree(t, h.Dir, h.Store.Root); after != before {
		t.Errorf("status changed something:\nbefore\n%s\nafter\n%s", before, after)
	}
	if h.stdout.Len() != 0 || h.stderr.Len() != 0 {
		t.Errorf("Status wrote to the streams: %q %q", h.stdout, h.stderr)
	}
}

// snapshotTree renders every path under the given roots with its size, so a
// test can say "nothing changed" about a whole tree.
func snapshotTree(t *testing.T, roots ...string) string {
	t.Helper()
	var b strings.Builder
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "%s %s", rel, info.Mode())
			if !info.IsDir() {
				fmt.Fprintf(&b, " %d", info.Size())
			}
			b.WriteString("\n")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return b.String()
}

// Rows are sorted by name across both files: a pin block.toml no longer
// declares sorts among the declared ones rather than trailing them.
func TestStatusSortsRowsAcrossBothFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, statusManifest+"[tools.bare]\nversion = \"2.5\"\n[tools.bare.source]\ntype = \"github_release\"\nrepo = \"example/bare\"\ntag_prefix = \"\"\nasset = \"bare_{version}_{os}_{arch}.tar.gz\"\nbin = [\"bare\"]\n")
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	h.manifest(t, statusManifest)
	report, err := h.Status()
	if err != nil {
		t.Fatal(err)
	}
	if got := states(report); got != "bare=stale(not in block.toml) foundry=missing" {
		t.Errorf("states = %s", got)
	}
}

// The hint names the command that moves the report forward, and a stale pin
// has to be locked before installing anything means something — whichever
// row comes first.
func TestHintPutsLockAheadOfSync(t *testing.T) {
	t.Parallel()
	lock := `run "block lock" to bring block.lock up to date with block.toml`
	sync := `run "block sync" to install the locked toolchain`
	for _, tc := range []struct {
		states []State
		want   string
	}{
		{nil, ""},
		{[]State{StateOK, StateOK}, ""},
		{[]State{StateOK, StateMissing}, sync},
		{[]State{StateDamaged, StateOK}, sync},
		{[]State{StateMissing, StateStale}, lock},
		{[]State{StateStale, StateDamaged}, lock},
		{[]State{StateDamaged, StateMissing, StateStale, StateOK}, lock},
	} {
		s := &Status{}
		for _, st := range tc.states {
			s.Tools = append(s.Tools, ToolStatus{State: st})
		}
		if got := s.Hint(); got != tc.want {
			t.Errorf("Hint(%v) = %q, want %q", tc.states, got, tc.want)
		}
	}
}

// A lockfile with no artifact for this machine is a disagreement, and the
// store is not consulted about an install that cannot exist.
func TestStatusReportsAPinWithNoArtifactForThisMachine(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "/t1")
	h.manifest(t, "platforms = [\"darwin/arm64\", \"linux/arm64\"]\n"+statusManifest)
	ctx := context.Background()
	if err := h.Lock(ctx, nil, false); err != nil {
		t.Fatal(err)
	}
	h.manifest(t, statusManifest)
	report, err := h.Status()
	if err != nil {
		t.Fatal(err)
	}
	want := "foundry=stale(no artifact for " + h.Platform.String() + ")"
	if got := states(report); got != want {
		t.Errorf("states = %s, want %s", got, want)
	}
	if report.Tools[0].Installed != "" || report.Tools[0].Dir != "" {
		t.Errorf("an install was reported for a platform the lock has nothing for: %+v", report.Tools[0])
	}
}
