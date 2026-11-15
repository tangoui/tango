package updateui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/service/updater"
)

func TestStateDirRules(t *testing.T) {
	explicit := t.TempDir()
	if d, err := stateDir(&updater.Updater{StateDir: explicit, AppID: "x"}); err != nil || d != explicit {
		t.Fatalf("explicit StateDir: %q %v", d, err)
	}
	d, err := stateDir(&updater.Updater{AppID: "com.ex/a b"})
	if err != nil || !strings.HasSuffix(d, filepath.Join("tango-updater", "com.ex_a_b")) {
		t.Fatalf("AppID dir: %q %v", d, err)
	}
	exe, _ := os.Executable()
	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	d, err = stateDir(&updater.Updater{})
	if err != nil || !strings.HasSuffix(d, filepath.Join("tango-updater", safeName(name))) {
		t.Fatalf("executable-name dir: %q %v (exe %q)", d, err, name)
	}
}

func TestStateRoundTrip(t *testing.T) {
	u := &updater.Updater{StateDir: t.TempDir()}
	when := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	in := persisted{
		Skipped:   "1.3.0",
		LastCheck: when,
		Pending: &pendingPayload{
			Version: "1.4.0", Path: "/tmp/x.tar.gz", Patched: true,
			Release: &updater.Release{Version: "1.4.0", Notes: "n"},
		},
	}
	if err := saveState(u, in); err != nil {
		t.Fatal(err)
	}
	out := loadState(u)
	if out.Skipped != in.Skipped || !out.LastCheck.Equal(when) || out.Pending == nil ||
		*out.Pending != (pendingPayload{Version: "1.4.0", Path: "/tmp/x.tar.gz", Patched: true, Release: out.Pending.Release}) ||
		out.Pending.Release.Notes != "n" {
		t.Fatalf("round trip: %+v (pending %+v)", out, out.Pending)
	}
	// Windows reports 0666 for any writable file: no owner-only bits to check.
	if fi, err := os.Stat(filepath.Join(u.StateDir, stateFile)); err != nil || runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file mode: %v %v", fi, err)
	}
}

// Apply re-checks a tree release (formats app, dir) against
// Release.TreeSHA256, which Release's JSON leaves out: the kept update
// must carry it, or installing it on the next launch always fails.
func TestPendingKeepsTreeDigest(t *testing.T) {
	u := &updater.Updater{StateDir: t.TempDir()}
	rel := &updater.Release{Version: "1.4.0", Format: updater.FormatApp, TreeSHA256: "abc123"}
	pay := &updater.Payload{Release: rel, Path: "/tmp/new.app", Patched: true}
	if err := saveState(u, persisted{Pending: newPending(pay)}); err != nil {
		t.Fatal(err)
	}
	got := loadState(u).Pending.payload()
	if got.Release.TreeSHA256 != "abc123" || got.Release.Version != "1.4.0" || got.Path != "/tmp/new.app" || !got.Patched {
		t.Fatalf("payload after reload: %+v release %+v", got, got.Release)
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	u := &updater.Updater{StateDir: t.TempDir()}
	os.WriteFile(filepath.Join(u.StateDir, stateFile), []byte("{bad"), 0o600)
	if got := loadState(u); got.Skipped != "" || !got.LastCheck.IsZero() || got.Pending != nil {
		t.Fatalf("corrupt file: %+v", got)
	}
}

func TestPendingDiscardedWhenMissingOrStale(t *testing.T) {
	file := filepath.Join(t.TempDir(), "update.tar.gz")
	os.WriteFile(file, []byte("x"), 0o600)
	p := persisted{Pending: &pendingPayload{Version: "1.3.0", Path: filepath.Join(t.TempDir(), "gone")}}
	if got := p.usablePending("1.2.0"); got != nil || p.Pending != nil {
		t.Fatalf("missing file kept: %+v", got)
	}
	p = persisted{Pending: &pendingPayload{Version: "1.2.0", Path: file}}
	if got := p.usablePending("1.2.0"); got != nil || p.Pending != nil {
		t.Fatalf("same version kept: %+v", got)
	}
	p = persisted{Pending: &pendingPayload{Version: "1.3.0", Path: file}}
	if got := p.usablePending("1.2.0"); got == nil || p.Pending == nil {
		t.Fatal("usable pending update dropped")
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.10.0", "1.9.0", true},
		{"v2.0.0", "1.9.9", true},
		{"1.2.0-beta", "1.2.0", false},
		{"1.2.0", "1.2.0-beta", true},
		{"1.2.0", "1.2.0", false},
		{"1.2", "1.2.0", false},
		{"1.2.0+b2", "1.2.0+b1", false},
		{"garbage", "1.0.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
