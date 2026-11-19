package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// After verification the runner launches only private copies: a build.sh
// run (or any change to bin/) mid-run does not reach later rounds.
func TestStagedCopiesIsolateBin(t *testing.T) {
	b, f := builtTree(t)
	m, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect)
	if err != nil {
		t.Fatal(err)
	}
	var reg stagedDir
	dir, err := stageBinaries(b.bin, m, &reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reg.cleanup)
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("staged dir %v %v, want mode 0700", st, err)
	}
	want := m.entry("mygo").SHA256
	// Someone rebuilds mygo in bin/ after verification.
	if err := os.WriteFile(filepath.Join(b.bin, "mygo"), []byte("rebuilt mid-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config{bin: dir, manifest: m}
	line, err := launchBuild(cfg, "mygo")
	if err != nil {
		t.Fatalf("launchBuild after bin/ changed: %v", err)
	}
	if !strings.Contains(line, "sha256="+want[:12]) {
		t.Fatalf("build line %q, want the verified sha %s", line, want[:12])
	}
	cmd := sceneCommand(cfg, "mygo", "list")
	if filepath.Dir(cmd.Path) != dir || strings.HasPrefix(cmd.Path, b.bin) {
		t.Fatalf("launch path %s, want the private copy in %s", cmd.Path, dir)
	}
	if sum, _ := fileSHA256(cmd.Path); sum != want {
		t.Fatalf("launched binary sha %s, want %s", sum, want)
	}
	// A private copy that changed anyway is refused for the round.
	if err := os.WriteFile(filepath.Join(dir, "nsgo"), []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := launchBuild(cfg, "nsgo"); err == nil {
		t.Fatal("launchBuild accepted a changed private copy")
	}
	// Cleanup removes the copies; staging after cleanup refuses.
	reg.cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("staged dir left after cleanup: %v", err)
	}
	if _, err := stageBinaries(b.bin, m, &reg); err == nil {
		t.Fatal("staged after cleanup")
	}
}

// A binary that changed between verification and staging is refused.
func TestStageRefusesChangedBinary(t *testing.T) {
	b, f := builtTree(t)
	m, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(b.bin, "tango"), []byte("new"), 0o755)
	var reg stagedDir
	if _, err := stageBinaries(b.bin, m, &reg); err == nil || !strings.Contains(err.Error(), "tango") {
		t.Fatalf("stage err %v, want tango refused", err)
	}
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.dir != "" {
		t.Fatalf("failed staging left %s registered", reg.dir)
	}
}

func TestVerifyRefusesOtherFileName(t *testing.T) {
	for _, file := range []string{"../mygo", "sub/mygo", "nsgo"} {
		b, f := builtTree(t)
		editManifest(t, b.bin, func(m *Manifest) { m.entry("mygo").File = file })
		if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err == nil || !strings.Contains(err.Error(), "file") {
			t.Fatalf("file %q: err %v, want refusal", file, err)
		}
	}
}

// The MyGo version in the manifest must be the one linked into the binary.
func TestVerifyRefusesMyGoDepMismatch(t *testing.T) {
	b, f := builtTree(t)
	f.deps = map[string]string{"github.com/egoist/mygo": "v0.8.0 h1:old="}
	if _, err := verifyManifest(b.bin, filepath.Join(b.bin, "runner"), f.inspect); err == nil || !strings.Contains(err.Error(), "v0.8.0") {
		t.Fatalf("err %v, want mygo dep mismatch", err)
	}
	f2 := &fakeBuild{deps: map[string]string{"github.com/egoist/mygo": "v0.9.1 h1:other="}}
	b2, _ := newFakeBuilder(t, f2)
	if _, err := b2.build(); err == nil || !strings.Contains(err.Error(), "v0.9.1") {
		t.Fatalf("build err %v, want mygo dep mismatch", err)
	}
}

func TestNSGoModuleFilesHashed(t *testing.T) {
	b, _ := newFakeBuilder(t, &fakeBuild{})
	m, err := b.build()
	if err != nil {
		t.Fatal(err)
	}
	s := m.entry("nsgo").Source
	if len(s.GoModSHA256) != 64 || s.GoSumSHA256 != "absent" {
		t.Fatalf("nsgo source %+v, want go.mod hashed and go.sum absent", s)
	}
}

func TestDirtyBuildMarked(t *testing.T) {
	f := &fakeBuild{dirty: "?? x.go\n"}
	b, _ := newFakeBuilder(t, f)
	m, err := b.build()
	if err != nil {
		t.Fatal(err)
	}
	if !m.dirty() {
		t.Fatal("dirty source not reported")
	}
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, nil)
	_, why := judgeWhy(tg, o, 5)
	if got := annotateWhy(why, m, true); !strings.Contains(got, "(dirty build)") || !strings.HasPrefix(got, "UNVERIFIED RUNNER") {
		t.Fatalf("why %q, want dirty suffix and unverified prefix", got)
	}
}

// A renderer time before the process started, or more than 1 s after it
// was read, is a clock step: the round says so.
func TestMonitorClockStep(t *testing.T) {
	for _, tc := range []struct {
		line string
		step bool
	}{
		{"bench renderer gpu 1500000000", false},
		{"bench renderer gpu 900000000", true},  // before the process start (1 s)
		{"bench renderer gpu 3100000000", true}, // 1.1 s after receipt (2 s)
		{"bench renderer gpu", false},           // untimed: the runner's clock
	} {
		m := newMonitor()
		m.startNs = time.Unix(1, 0).UnixNano()
		m.now = func() time.Time { return time.Unix(2, 0) }
		m.feed(tc.line)
		if m.clockStep != tc.step {
			t.Errorf("%q: clockStep %t, want %t", tc.line, m.clockStep, tc.step)
		}
	}
}
