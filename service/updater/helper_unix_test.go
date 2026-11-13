//go:build unix

package updater

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shJob lays out an AppImage-style replacement for the shell helper; the
// launch command appends the target's content to a log.
func shJob(t *testing.T, timeout int) (*helperJob, string) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Notes.AppImage")
	os.WriteFile(target, []byte("old"), 0o755)
	work := filepath.Join(dir, ".tango-update-x")
	os.MkdirAll(filepath.Join(work, "new"), 0o755)
	repl := filepath.Join(work, "new", "Notes.AppImage")
	os.WriteFile(repl, []byte("new"), 0o755)
	log := filepath.Join(dir, "launches")
	j := &helperJob{PID: exitedPid(t), Format: FormatAppImage, Target: target, Repl: repl, Work: work,
		State: t.TempDir(), Timeout: timeout, Launch: []string{"/bin/sh", "-c", `cat "$0" >> "$1"; echo >> "$1"`, target, log}}
	return j, log
}

func runScript(t *testing.T, j *helperJob) {
	t.Helper()
	out, err := exec.Command("/bin/sh", helperArgs(j)...).CombinedOutput()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func launches(log string) string {
	b, _ := os.ReadFile(log)
	return strings.Join(strings.Fields(string(b)), ",")
}

func TestHelperScriptConfirmed(t *testing.T) {
	j, log := shJob(t, 10)
	os.WriteFile(filepath.Join(j.State, statePending), []byte("{}"), 0o600)
	// The "new version" confirms as soon as it has started.
	go func() {
		waitFor(t, "launch", func() bool { return launches(log) == "new" })
		os.WriteFile(filepath.Join(j.State, stateConfirmed), nil, 0o600)
	}()
	runScript(t, j)
	if b, _ := os.ReadFile(j.Target); string(b) != "new" {
		t.Fatalf("target %q", b)
	}
	for _, p := range []string{j.Work, filepath.Join(j.State, statePending), filepath.Join(j.State, stateConfirmed)} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s left", p)
		}
	}
}

func TestHelperScriptRollsBack(t *testing.T) {
	j, log := shJob(t, 1)
	runScript(t, j) // nobody confirms
	if b, _ := os.ReadFile(j.Target); string(b) != "old" {
		t.Fatalf("not rolled back: %q", b)
	}
	waitFor(t, "relaunches", func() bool { return launches(log) == "new,old" })
	if b, _ := os.ReadFile(filepath.Join(j.State, stateRolledBack)); !strings.Contains(string(b), "did not confirm within 1s") {
		t.Fatalf("reason %q", b)
	}
	if _, err := os.Stat(j.Work); err == nil {
		t.Error("work dir left")
	}

	// Rollback requested by the new version (pid already gone).
	j, _ = shJob(t, 30)
	go func() {
		waitFor(t, "swap", func() bool { b, _ := os.ReadFile(j.Target); return string(b) == "new" })
		os.WriteFile(filepath.Join(j.State, stateRollback), []byte("999999"), 0o600)
	}()
	runScript(t, j)
	if b, _ := os.ReadFile(filepath.Join(j.State, stateRolledBack)); !strings.Contains(string(b), "asked for a rollback") {
		t.Fatalf("reason %q", b)
	}

	// The replacement is missing: the old version stays and restarts.
	j, log = shJob(t, 30)
	os.Remove(j.Repl)
	runScript(t, j)
	waitFor(t, "relaunch", func() bool { return launches(log) == "old" })
}

func TestHelperScriptWaitsForExit(t *testing.T) {
	sleeper := exec.Command("/bin/sleep", "0.5")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	j, _ := shJob(t, 0)
	j.PID, j.State = sleeper.Process.Pid, ""
	helper := exec.Command("/bin/sh", helperArgs(j)...)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if b, _ := os.ReadFile(j.Target); string(b) != "old" {
		t.Fatal("helper swapped before the process exited")
	}
	sleeper.Wait()
	if err := helper.Wait(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(j.Target); string(b) != "new" {
		t.Fatal("helper did not swap after exit")
	}
}

// TestApplyDirConfirm runs Apply for a Linux-style install directory end
// to end: Check, Download (tree patch), Apply (the real helper waiting
// for an exited pid), then Confirm as the new version.
func TestApplyDirConfirm(t *testing.T) {
	old := filepath.Join(t.TempDir(), "notes")
	writeTree(t, old, map[string]string{"notes": "#!/bin/sh\necho 1\n", "data/a.txt": strings.Repeat("a", 5000)})
	os.Chmod(filepath.Join(old, "notes"), 0o755)
	newSrc := filepath.Join(t.TempDir(), "notes-2.0.0")
	writeTree(t, newSrc, map[string]string{"notes": "#!/bin/sh\necho 2\n", "data/a.txt": strings.Repeat("a", 5000) + "b"})
	os.Chmod(filepath.Join(newSrc, "notes"), 0o755)
	archive := filepath.Join(t.TempDir(), "notes-2.0.0.tar.gz")
	if err := PackTree(newSrc, archive); err != nil {
		t.Fatal(err)
	}
	full, _ := os.ReadFile(archive)
	var patch strings.Builder
	if err := DiffTree(old, newSrc, &patch); err != nil {
		t.Fatal(err)
	}
	digest, _ := TreeDigest(newSrc)

	s := newServer(t)
	m := manifest("2.0.0", FormatDir, s.put("notes-2.0.0.tar.gz", full))
	a := m.Platforms["testos-testarch"]
	a.TreeSHA256 = digest
	a.Patches = []PatchAsset{{From: "1.0.0", File: s.put("notes-1.0.0-to-2.0.0.patch", []byte(patch.String()))}}
	m.Platforms["testos-testarch"] = a
	s.publish(m)

	pid := exitedPid(t)
	defer func(f func() int) { helperPID = f }(helperPID)
	helperPID = func() int { return pid }

	u := s.updater("1.0.0")
	u.Target = old
	u.ConfirmTimeout = 20 * time.Second
	var events []EventKind
	u.OnEvent = func(e Event) {
		if len(events) == 0 || events[len(events)-1] != e.Kind {
			events = append(events, e.Kind)
		}
	}
	if o, _ := u.Confirm(); o.Status != StatusNone {
		t.Fatalf("before: %+v", o)
	}
	rel, err := u.Update(context.Background())
	if err != nil || rel == nil {
		t.Fatalf("Update = %v, %v", rel, err)
	}
	if events[len(events)-1] != EventReady {
		t.Fatalf("events %v", events)
	}
	waitFor(t, "swap", func() bool {
		b, _ := os.ReadFile(filepath.Join(old, "notes"))
		return strings.Contains(string(b), "echo 2")
	})

	nu := s.updater("2.0.0")
	nu.StateDir = u.StateDir
	if o, err := nu.Confirm(); err != nil || o.Status != StatusUpdated || o.From != "1.0.0" {
		t.Fatalf("Confirm = %+v, %v", o, err)
	}
	waitFor(t, "cleanup", func() bool {
		_, err := os.Stat(filepath.Join(u.StateDir, statePending))
		left, _ := filepath.Glob(filepath.Join(filepath.Dir(old), ".tango-*"))
		return os.IsNotExist(err) && len(left) == 0
	})
	if got, _ := TreeDigest(old); got != digest {
		t.Fatal("installed tree differs")
	}
}
