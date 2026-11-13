// Package e2e holds the macOS update drill: a real update of a running
// .app bundle, from a local HTTP server, through tan release-manifest,
// /usr/bin/open and the updater's helper.
//
//	TANGO_UPDATER_DRILL=1 go test -v -run Drill ./service/updater/internal/e2e
package e2e

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func run(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// repoRoot is the module root, three levels up.
func repoRoot(t *testing.T) string {
	wd, _ := os.Getwd()
	root := filepath.Join(wd, "..", "..", "..", "..")
	if _, err := os.Stat(filepath.Join(root, "go.work")); err != nil {
		t.Skip("not in the tango repository")
	}
	return root
}

// buildApp builds the drill app as <build>/darwin-<arch>/Drill.app, the
// layout tan build leaves, ad hoc signed as tan build signs it.
func buildApp(t *testing.T, root, build, version string, tags, mode string) string {
	app := filepath.Join(build, "darwin-"+runtime.GOARCH, "Drill.app")
	exe := filepath.Join(app, "Contents", "MacOS", "Drill")
	args := []string{"build", "-trimpath", "-ldflags", "-s -w -X main.version=" + version + " -X main.mode=" + mode, "-o", exe}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	run(t, root, []string{"CGO_ENABLED=0"}, "go", append(args, "./service/updater/internal/e2e/drillapp")...)
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>Drill</string>
<key>CFBundleIdentifier</key><string>dev.tango.drill</string>
<key>CFBundleName</key><string>Drill</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>` + version + `</string>
<key>CFBundleVersion</key><string>` + version + `</string>
<key>LSBackgroundOnly</key><true/>
</dict></plist>
`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "", nil, "codesign", "--force", "--sign", "-", app)
	return app
}

// release runs tan release-manifest for a build.
func release(t *testing.T, tan, keyFile, build, out, version string, prev ...string) {
	proj := t.TempDir()
	os.WriteFile(filepath.Join(proj, "tan.json"), []byte(`{"name":"Drill","id":"dev.tango.drill","version":"`+version+`","frontend":{"dir":"frontend","dist":"."}}`), 0o644)
	args := []string{"release-manifest", "-C", proj, "-build", build, "-o", out, "-key", keyFile}
	for _, p := range prev {
		args = append(args, "-prev", p)
	}
	t.Log(strings.TrimSpace(run(t, "", nil, tan, args...)))
}

type logWatcher struct {
	path string
	mu   sync.Mutex
}

func (l *logWatcher) lines() []string {
	f, err := os.Open(l.path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		out = append(out, s.Text())
	}
	return out
}

func (l *logWatcher) wait(t *testing.T, substr string, timeout time.Duration) string {
	t.Helper()
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		for _, line := range l.lines() {
			if strings.Contains(line, substr) {
				return line
			}
			if strings.Contains(line, "error") {
				t.Fatalf("drill app failed: %s", line)
			}
		}
	}
	t.Fatalf("timed out waiting for %q; log:\n%s", substr, strings.Join(l.lines(), "\n"))
	return ""
}

func waitGone(t *testing.T, what string, paths func() []string) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if len(paths()) == 0 {
			return
		}
	}
	t.Fatalf("%s still there: %v", what, paths())
}

func plistVersion(t *testing.T, app string) string {
	return strings.TrimSpace(run(t, "", nil, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleShortVersionString", filepath.Join(app, "Contents", "Info.plist")))
}

func fileSize(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return -1
	}
	return fi.Size()
}

func TestDrillMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("TANGO_UPDATER_DRILL") != "1" {
		t.Skip("set TANGO_UPDATER_DRILL=1 on macOS to run the update drill (it launches apps with open)")
	}
	root := repoRoot(t)
	work := t.TempDir()
	tan := filepath.Join(work, "tan")
	run(t, root, nil, "go", "build", "-o", tan, "./cmd/tan")
	keyBase := filepath.Join(work, "update")
	run(t, work, nil, tan, "keygen", "-o", keyBase)
	pub, _ := os.ReadFile(keyBase + ".pub")

	// Three releases: 0.0.1, 0.0.2 (more code), 0.0.3 (broken: never
	// confirms), each with a patch from the one before.
	builds := map[string]string{}
	rels := map[string]string{}
	var prev []string
	for _, v := range []struct{ version, tags, mode string }{{"0.0.1", "", ""}, {"0.0.2", "drillv2", ""}, {"0.0.3", "drillv2", "noconfirm"}} {
		builds[v.version] = filepath.Join(work, "build-"+v.version)
		buildApp(t, root, builds[v.version], v.version, v.tags, v.mode)
		rels[v.version] = filepath.Join(work, "release-"+v.version)
		release(t, tan, keyBase+".key", builds[v.version], rels[v.version], v.version, prev...)
		prev = []string{rels[v.version]}
	}

	// The server publishes whichever release dir is current.
	var mu sync.Mutex
	current := rels["0.0.2"]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		dir := current
		mu.Unlock()
		http.ServeFile(w, r, filepath.Join(dir, filepath.Base(r.URL.Path)))
	}))
	defer srv.Close()

	// Install 0.0.1 (as a user would: a copy in an Applications folder).
	apps := filepath.Join(work, "Applications")
	os.Mkdir(apps, 0o755)
	app := filepath.Join(apps, "Drill.app")
	run(t, "", nil, "ditto", filepath.Join(builds["0.0.1"], "darwin-"+runtime.GOARCH, "Drill.app"), app)
	state := filepath.Join(work, "state")
	logPath := filepath.Join(work, "drill.log")
	cfg, _ := json.Marshal(map[string]any{"manifest_url": srv.URL + "/latest.json", "public_key": strings.TrimSpace(string(pub)),
		"state_dir": state, "log": logPath, "timeout": 20})
	os.WriteFile(filepath.Join(apps, "drill.json"), cfg, 0o644)
	lw := &logWatcher{path: logPath}

	patch := filepath.Join(rels["0.0.2"], "Drill-0.0.1-to-0.0.2-darwin-"+runtime.GOARCH+".patch")
	full := filepath.Join(rels["0.0.2"], "Drill-0.0.2-darwin-"+runtime.GOARCH+".app.tar.gz")
	exe := filepath.Join(builds["0.0.2"], "darwin-"+runtime.GOARCH, "Drill.app", "Contents", "MacOS", "Drill")
	t.Logf("0.0.1 → 0.0.2: executable %d bytes, full archive %d bytes, patch %d bytes (%.1f%% of the archive)",
		fileSize(exe), fileSize(full), fileSize(patch), 100*float64(fileSize(patch))/float64(fileSize(full)))

	// Drill 1: 0.0.1 updates itself to 0.0.2 by patch, and 0.0.2 starts.
	start := time.Now()
	run(t, "", nil, "open", "-n", app)
	t.Log(lw.wait(t, "0.0.1 pid", 30*time.Second))
	t.Log(lw.wait(t, "downloaded 0.0.2 patched=true", 60*time.Second))
	t.Log(lw.wait(t, "applied", 30*time.Second))
	t.Log(lw.wait(t, "0.0.2 pid", 60*time.Second))
	line := lw.wait(t, "0.0.2 pid", time.Second)
	if !strings.Contains(line, "outcome=updated from=0.0.1") {
		t.Fatalf("0.0.2 reported %s", line)
	}
	t.Logf("update took %v from launch to the new version running", time.Since(start).Round(time.Millisecond))
	waitGone(t, "work dir or pending state", func() []string {
		left, _ := filepath.Glob(filepath.Join(apps, ".tango-*"))
		if _, err := os.Stat(filepath.Join(state, "pending.json")); err == nil {
			left = append(left, "pending.json")
		}
		return left
	})
	if v := plistVersion(t, app); v != "0.0.2" {
		t.Fatalf("installed version %s", v)
	}
	run(t, "", nil, "codesign", "--verify", "--deep", "--strict", app)
	if exec.Command("xattr", "-p", "com.apple.quarantine", app).Run() == nil {
		t.Fatal("installed app is quarantined")
	}

	// Drill 2: 0.0.2 installs the broken 0.0.3, which never confirms; the
	// helper restores 0.0.2 and starts it, and it reports the rollback.
	mu.Lock()
	current = rels["0.0.3"]
	mu.Unlock()
	cfg, _ = json.Marshal(map[string]any{"manifest_url": srv.URL + "/latest.json", "public_key": strings.TrimSpace(string(pub)),
		"state_dir": state, "log": logPath, "timeout": 3})
	os.WriteFile(filepath.Join(apps, "drill.json"), cfg, 0o644)
	os.Remove(logPath)
	run(t, "", nil, "open", "-n", app)
	t.Log(lw.wait(t, "downloaded 0.0.3 patched=true", 60*time.Second))
	t.Log(lw.wait(t, "0.0.3 pid", 60*time.Second))
	line = lw.wait(t, "outcome=rolled_back", 60*time.Second)
	t.Log(line)
	if !strings.HasPrefix(line, "0.0.2 ") || !strings.Contains(line, "did not confirm within 3s") {
		t.Fatalf("rollback reported as %s", line)
	}
	if v := plistVersion(t, app); v != "0.0.2" {
		t.Fatalf("version after rollback %s", v)
	}
	run(t, "", nil, "codesign", "--verify", "--deep", "--strict", app)
	waitGone(t, "work dir", func() []string {
		left, _ := filepath.Glob(filepath.Join(apps, ".tango-*"))
		return left
	})
}
