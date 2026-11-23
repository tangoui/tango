package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDevInitialHotFallbackBuildsOnce(t *testing.T) {
	d := newDevTest(t, "")
	// A previously cached executable must be replaced by this run's build.
	writeDevTestFile(t, d.out, "#!/bin/sh\necho old >> \"$TAN_TEST_RUNS\"\n", 0o755)
	if err := runDev([]string{"-C", d.cfg.dir, "-watch-framework=false", "-url", d.url, "--", "app arg"}); err != nil {
		t.Fatal(err)
	}
	d.checkRuns(t, "v1", "app arg")
	builds := devTestLines(t, d.builds)
	if len(builds) != 1 {
		t.Fatalf("go build called %d times, want 1: %q", len(builds), builds)
	}
	if builds[0] == d.out {
		t.Fatalf("initial build bypassed the hot probe: %q", builds)
	}
	if runtime.GOOS == "darwin" {
		appName := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(d.out))))
		if got := strings.Join(devTestLines(t, d.signs), "\n"); !strings.HasPrefix(got, "--force --sign - "+filepath.Join(devDir(d.out), ".publish-")) || !strings.HasSuffix(got, string(filepath.Separator)+appName) {
			t.Errorf("codesign calls = %q", got)
		}
	}
}

func TestDevInitialHotFallbackStagesPrivateBinaryWithoutCopy(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("checks the macOS bundle staging path")
	}
	d := newDevTest(t, "")
	compiler := filepath.Join(d.tools, "go")
	script, err := os.ReadFile(compiler)
	if err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join(filepath.Dir(d.runs), "compiler-output")
	t.Setenv("TAN_TEST_PROBE_REFERENCE", reference)
	writeDevTestFile(t, compiler, string(script)+`/bin/ln "$out" "$TAN_TEST_PROBE_REFERENCE"
`, 0o755)
	writeDevTestFile(t, filepath.Join(d.tools, "codesign"), `#!/bin/sh
set -eu
if [ "$4/Contents/MacOS/DevTest" -ef "$TAN_TEST_PROBE_REFERENCE" ]; then
    echo original-inode >> "$TAN_TEST_SIGNS"
else
    echo copied-inode >> "$TAN_TEST_SIGNS"
fi
`, 0o755)
	d.run(t)
	if got := devTestLines(t, d.signs); len(got) != 1 || got[0] != "original-inode" {
		t.Fatalf("bundle signer saw %q, want the private probe's original inode", got)
	}
	d.checkRuns(t, "v1", "")
	d.checkBuilds(t, "probe")
}

func TestDevInitialHotFallbackAfterEdit(t *testing.T) {
	d := newDevTest(t, `
if [ "$version" = v1 ]; then
    printf '// edited\npackage main\nfunc main() {}\n' > main.go
fi`)
	d.run(t)
	d.checkRuns(t, "edited", "")
	d.checkBuilds(t, "probe", "plain")
}

func TestDevInitialHotFallbackSeedsPrivateBuild(t *testing.T) {
	d := newDevTest(t, `
if [ "$out" != "$TAN_TEST_STABLE" ] && /usr/bin/cmp -s "$out" "$TAN_TEST_STABLE"; then
    printf '%s\n' "$out" > "$TAN_TEST_SEEDED"
fi`)
	seeded := filepath.Join(filepath.Dir(d.runs), "seeded")
	t.Setenv("TAN_TEST_STABLE", d.out)
	t.Setenv("TAN_TEST_SEEDED", seeded)
	tool := filepath.Join(runtime.GOROOT(), "bin", "go")
	data, err := os.ReadFile(tool)
	if err != nil {
		t.Fatal(err)
	}
	writeDevTestFile(t, d.out, string(data), 0o755)
	d.run(t)
	d.checkRuns(t, "v1", "")
	d.checkBuilds(t, "probe")
	build := devTestLines(t, d.builds)[0]
	if got := devTestLines(t, seeded); len(got) != 1 || got[0] != build {
		t.Fatalf("go build saw seed at %q, want private target %q", got, build)
	}
}

func TestDevInitialHotFallbackKeepsEachWorktreesBuild(t *testing.T) {
	a := newDevTest(t, "")
	bDir := filepath.Join(filepath.Dir(a.cfg.dir), "other-project")
	config, err := os.ReadFile(a.cfg.Path(ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	writeDevTestFile(t, filepath.Join(bDir, ConfigFile), string(config), 0o644)
	writeDevTestFile(t, filepath.Join(bDir, "main.go"), "// v2\npackage main\nfunc main() {}\n", 0o644)
	bConfig, err := LoadConfig(bDir)
	if err != nil {
		t.Fatal(err)
	}
	bOut, err := devBinary(bConfig)
	if err != nil || bOut != a.out {
		t.Fatalf("second worktree's stable path = %q, %v; want %q", bOut, err, a.out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	firstBuild := func(cfg *Config) (*devSession, *initialHotFallback) {
		s := &devSession{cfg: cfg, out: a.out}
		err := s.hotLoop(ctx, false)
		var initial *initialHotFallback
		if !errors.As(err, &initial) {
			t.Fatalf("%s: expected initial fallback, got %v", cfg.dir, err)
		}
		return s, initial
	}

	// Both sessions finish probing before either publishes to the shared
	// stable path. A publishes first, after B has built in its own worktree.
	s1, initial1 := firstBuild(a.cfg)
	s2, initial2 := firstBuild(bConfig)
	for _, pair := range []struct {
		s       *devSession
		initial *initialHotFallback
	}{{s1, initial1}, {s2, initial2}} {
		if err := pair.s.loop(ctx, pair.initial); err != nil {
			t.Fatal(err)
		}
	}
	runs := devTestLines(t, a.runs)
	t.Logf("build outputs: %q", devTestLines(t, a.builds))
	t.Logf("executions (version, path, worktree, environment, args): %q", runs)
	if len(runs) != 2 || !strings.HasPrefix(runs[0], "v1|"+a.out+"|"+a.cfg.dir+"|") ||
		!strings.HasPrefix(runs[1], "v2|"+a.out+"|"+bDir+"|") {
		t.Fatalf("wrong worktree artifact executed: %q", runs)
	}
	a.checkBuilds(t, initial1.path, initial2.path)
	if initial1.path == initial2.path {
		t.Errorf("sessions shared probe path %s", initial1.path)
	}
	for _, initial := range []*initialHotFallback{initial1, initial2} {
		if _, err := os.Stat(filepath.Dir(initial.path)); !os.IsNotExist(err) {
			t.Errorf("session probe directory still present after handoff: %s (%v)", filepath.Dir(initial.path), err)
		}
	}
}

func TestDevSameIDLaunchesFromItsOwnWorktree(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("uses the macOS signer to pause A")
	}
	d := newDevTest(t, "")
	bDir := filepath.Join(filepath.Dir(d.cfg.dir), "project-b")
	config, err := os.ReadFile(d.cfg.Path(ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	writeDevTestFile(t, filepath.Join(bDir, ConfigFile), string(config), 0o644)
	writeDevTestFile(t, filepath.Join(bDir, "main.go"), "// v2\npackage main\nfunc main() {}\n", 0o644)
	paused := filepath.Join(filepath.Dir(d.runs), "signing-a")
	resume := filepath.Join(filepath.Dir(d.runs), "resume-a")
	t.Setenv("TAN_TEST_SIGN_PAUSED", paused)
	t.Setenv("TAN_TEST_SIGN_RESUME", resume)
	writeDevTestFile(t, filepath.Join(d.tools, "codesign"), `#!/bin/sh
set -eu
if [ "$TAN_TEST_SESSION" = A ]; then
    echo signing > "$TAN_TEST_SIGN_PAUSED"
    while [ ! -f "$TAN_TEST_SIGN_RESUME" ]; do /bin/sleep 0.01; done
fi
`, 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := func(session, dir string) <-chan error {
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, self, "dev", "-C", dir, "-watch-framework=false", "-url", d.url)
		cmd.Env = append(os.Environ(), "TAN_TEST_MAIN=1", "TAN_TEST_SESSION="+session)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { defer close(done); done <- cmd.Wait() }()
		return done
	}
	aDone := start("A", d.cfg.dir)
	defer func() {
		_ = os.WriteFile(resume, nil, 0o644)
		cancel()
		<-aDone
	}()
	waitDevTestLines(t, ctx, paused, 1)
	bDone := start("B", bDir)
	if err := <-bDone; err != nil {
		t.Fatal(err)
	}
	writeDevTestFile(t, resume, "resume", 0o644)
	if err := <-aDone; err != nil {
		t.Fatal(err)
	}
	runs := devTestLines(t, d.runs)
	if len(runs) != 2 || !strings.HasPrefix(runs[0], "v2|"+d.out+"|"+bDir+"|") ||
		!strings.HasPrefix(runs[1], "v1|"+d.out+"|"+d.cfg.dir+"|") {
		t.Fatalf("wrong worktree launched while A was paused: %q", runs)
	}
}

func TestDevCanceledSigningPreservesPreviousBundle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS codesign")
	}
	d := newDevTest(t, "")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	writeDevTestFile(t, d.out, string(old), 0o755)
	app := filepath.Dir(filepath.Dir(filepath.Dir(d.out)))
	if err := writeBundle(d.cfg, app); err != nil {
		t.Fatal(err)
	}
	sign := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("/usr/bin/codesign", append(args, app)...).CombinedOutput(); err != nil {
			t.Errorf("real codesign %v: %v: %s", args, err, out)
		}
	}
	sign("--force", "--sign", "-")
	sign("--verify", "--strict")
	old, err = os.ReadFile(d.out)
	if err != nil {
		t.Fatal(err)
	}
	writeDevTestFile(t, filepath.Join(d.tools, "codesign"), `#!/bin/sh
if [ -f "$TAN_TEST_SIGNS" ]; then exit 37; fi
echo signing >> "$TAN_TEST_SIGNS"
exec /bin/sleep 30
`, 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := d.start(ctx)
	defer func() { cancel(); <-done }()
	waitDevTestLines(t, ctx, d.signs, 1)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(d.out)
	if err != nil || !bytes.Equal(got, old) {
		t.Errorf("canceled signing replaced the previous executable: size=%d, want %d (%v)", len(got), len(old), err)
	}
	sign("--verify", "--strict")
	if runs := devTestLines(t, d.runs); len(runs) != 0 {
		t.Errorf("the canceled app ran: %q", runs)
	}
}

func TestDevInitialHotFallbackFailureKeepsOldAppStopped(t *testing.T) {
	for _, phase := range []string{"initial build", "publication lock error", "signing", "canceled build"} {
		t.Run(phase, func(t *testing.T) {
			var hook string
			switch phase {
			case "initial build":
				hook = "exit 42"
			case "canceled build":
				hook = "exec /bin/sleep 30"
			case "signing":
				if runtime.GOOS != "darwin" {
					t.Skip("only macOS signs the bundle")
				}
			}
			d := newDevTest(t, hook)
			writeDevTestFile(t, d.out, "#!/bin/sh\necho old >> \"$TAN_TEST_RUNS\"\n", 0o755)
			if phase == "publication lock error" {
				// Keep the old output out of the seed path so the injected
				// error occurs during publication, after a successful build.
				if err := os.Chmod(d.out, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(devDir(d.out), ".publish.lock"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "signing" {
				writeDevTestFile(t, filepath.Join(d.tools, "codesign"), "#!/bin/sh\necho signing >> \"$TAN_TEST_SIGNS\"\nexit 37\n", 0o755)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := d.start(ctx)
			defer func() {
				cancel()
				if err := <-done; err != nil {
					t.Error(err)
				}
			}()
			log := d.builds
			if phase == "signing" {
				log = d.signs
			}
			waitDevTestLines(t, ctx, log, 1)
			if !strings.HasPrefix(phase, "canceled") {
				// Give the loop time to handle the failing command before cancellation.
				time.Sleep(250 * time.Millisecond)
			}
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if got := devTestLines(t, d.runs); len(got) != 0 {
				t.Fatalf("unexpected app executions: %q", got)
			}
			if got := devTestLines(t, d.out); len(got) != 2 || !strings.Contains(got[1], "echo old") {
				t.Errorf("previous executable was replaced: %q", got)
			}
			d.checkBuilds(t, "probe")
			if probeDir := filepath.Dir(devTestLines(t, d.builds)[0]); probeDir != filepath.Dir(d.out) {
				if _, err := os.Stat(probeDir); !os.IsNotExist(err) {
					t.Errorf("probe directory still present after %s: %s (%v)", phase, probeDir, err)
				}
			}
		})
	}
}

func TestDevInitialHotFallbackLaterRebuild(t *testing.T) {
	d := newDevTest(t, "")
	t.Setenv("TAN_TEST_HOLD_V1", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := d.start(ctx)
	defer func() { cancel(); <-done }()
	waitDevTestLines(t, ctx, d.runs, 1)
	writeDevTestFile(t, d.cfg.Path("main.go"), "// edited\npackage main\nfunc main() {}\n", 0o644)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := ctx.Err(); err != nil {
		t.Fatal(err)
	}
	d.checkBuilds(t, "probe", "plain")
	if got := devTestLines(t, d.runs); len(got) != 2 || !strings.HasPrefix(got[0], "v1|"+d.out+"|") || !strings.HasPrefix(got[1], "edited|"+d.out+"|") {
		t.Errorf("app executions: %q", got)
	}
}

func TestDevInitialHotFallbackCancellationStopsProcessGroup(t *testing.T) {
	d := newDevTest(t, "")
	shutdown := filepath.Join(filepath.Dir(d.runs), "shutdown")
	t.Setenv("TAN_TEST_SHUTDOWN", shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := d.start(ctx)
	defer func() { cancel(); <-done }()
	waitDevTestLines(t, ctx, d.runs, 1)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := devTestLines(t, shutdown); len(got) != 1 || got[0] != "TERM" {
		t.Errorf("app did not handle graceful group shutdown: %q", got)
	}
}

func TestDevHotDisabledBuildsDirectly(t *testing.T) {
	d := newDevTest(t, "")
	if err := runDev([]string{"-C", d.cfg.dir, "-hot=false", "-url", d.url}); err != nil {
		t.Fatal(err)
	}
	d.checkBuilds(t, "plain")
	d.checkRuns(t, "v1", "")
}

type devTest struct {
	cfg                                  *Config
	url, out, builds, runs, signs, tools string
}

// newDevTest sets up a shell-based compiler and app with their own cache
// and logs. buildHook runs before the compiler writes its output.
func newDevTest(t *testing.T, buildHook string) devTest {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell process fixtures")
	}
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "project")
	writeDevTestFile(t, filepath.Join(project, ConfigFile), `{"name":"Dev Test","id":"org.tango.dev-test","version":"1.0.0"}`, 0o644)
	writeDevTestFile(t, filepath.Join(project, "main.go"), "// v1\npackage main\nfunc main() {}\n", 0o644)
	// os.UserCacheDir consults HOME on macOS and XDG_CACHE_HOME on Linux.
	if runtime.GOOS == "darwin" {
		t.Setenv("HOME", root)
	} else {
		t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	}
	cfg, err := LoadConfig(project)
	if err != nil {
		t.Fatal(err)
	}
	out, err := devBinary(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := devTest{cfg: cfg, out: out, url: "http://127.0.0.1:4321/", tools: filepath.Join(root, "bin"),
		builds: filepath.Join(root, "builds"), runs: filepath.Join(root, "runs"), signs: filepath.Join(root, "signs")}
	t.Setenv("PATH", d.tools)
	t.Setenv("TANGO_HOTRELOAD", "1")
	t.Setenv("TAN_TEST_BUILDS", d.builds)
	t.Setenv("TAN_TEST_RUNS", d.runs)
	t.Setenv("TAN_TEST_SIGNS", d.signs)
	writeDevTestFile(t, filepath.Join(d.tools, "go"), `#!/bin/sh
set -eu
test "$1" = build
test "$2" = -tags && test "$3" = "`+buildTags(true)+`"
test "$4" = -o
out=$5
case "$out" in
    */workers/initial-*/*) test "$6" = -n=false && test "$7" = . ;;
    *) test "$6" = . ;;
esac
printf '%s\n' "$out" >> "$TAN_TEST_BUILDS"
read -r comment version < main.go
`+buildHook+`
/bin/mkdir -p "${out%/*}"
{
    printf '#!/bin/sh\nversion=%s\n' "$version"
    /bin/cat <<'APP'
if [ -n "${TAN_TEST_SHUTDOWN:-}" ]; then
    trap 'echo TERM >> "$TAN_TEST_SHUTDOWN"; exit 0' TERM
    /bin/sleep 30 &
fi
printf '%s|%s|%s|%s|%s|%s\n' "$version" "$0" "$PWD" "$TANGO_DEV_URL" "$TANGO_DEV_WINDOW_STATE" "$*" >> "$TAN_TEST_RUNS"
if [ -n "${TAN_TEST_SHUTDOWN:-}" ]; then wait; fi
if [ "${TAN_TEST_HOLD_V1:-}" = 1 ] && [ "$version" = v1 ]; then exec /bin/sleep 30; fi
APP
} > "$out"
/bin/chmod 755 "$out"
`, 0o755)
	writeDevTestFile(t, filepath.Join(d.tools, "codesign"), `#!/bin/sh
set -eu
test -f "$4/Contents/Info.plist"
test -f "$4/Contents/PkgInfo"
printf '%s\n' "$*" >> "$TAN_TEST_SIGNS"
`, 0o755)
	return d
}

func writeDevTestFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func devTestLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	// A writer may have opened the log or appended only part of a line.
	end := bytes.LastIndexByte(b, '\n')
	if end < 0 {
		return nil
	}
	return strings.Split(string(b[:end]), "\n")
}

func TestDevTestLinesRequiresCompleteEntries(t *testing.T) {
	log := filepath.Join(t.TempDir(), "builds")
	for _, entry := range []string{"", "/workers/initial-123"} {
		writeDevTestFile(t, log, entry, 0o644)
		if got := devTestLines(t, log); len(got) != 0 {
			t.Errorf("incomplete log %q returned entries %q", entry, got)
		}
	}
	writeDevTestFile(t, log, "/workers/initial-123/app\n/partial", 0o644)
	if got := devTestLines(t, log); len(got) != 1 || got[0] != "/workers/initial-123/app" {
		t.Errorf("log entries = %q, want one complete entry", got)
	}
}

func (d devTest) checkRuns(t *testing.T, version, args string) {
	t.Helper()
	want := strings.Join([]string{version, d.out, d.cfg.dir, d.url, filepath.Join(devDir(d.out), "windows.json"), args}, "|")
	if got := devTestLines(t, d.runs); len(got) != 1 || got[0] != want {
		t.Fatalf("app runs = %q, want [%q]", got, want)
	}
}

func (d devTest) run(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := dev(ctx, d.cfg, d.url, nil, devModes{hot: true}); err != nil {
		t.Fatal(err)
	}
}

func (d devTest) start(ctx context.Context) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- dev(ctx, d.cfg, d.url, nil, devModes{hot: true})
	}()
	return done
}

func (d devTest) checkBuilds(t *testing.T, want ...string) {
	t.Helper()
	got := devTestLines(t, d.builds)
	if len(got) != len(want) {
		t.Fatalf("build destinations = %q, want %q", got, want)
	}
	for i, path := range want {
		if path == "probe" && strings.HasPrefix(got[i], filepath.Join(devDir(d.out), "workers")+string(filepath.Separator)) {
			continue
		}
		if path == "plain" && strings.HasPrefix(got[i], filepath.Join(devDir(d.out), ".publish-")) {
			continue
		}
		if got[i] != path {
			t.Errorf("build %d destination = %q, want %q", i+1, got[i], path)
		}
	}
}

func waitDevTestLines(t *testing.T, ctx context.Context, path string, n int) {
	t.Helper()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for len(devTestLines(t, path)) < n {
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %d entries in %s: %v", n, path, ctx.Err())
		case <-tick.C:
		}
	}
}

func TestFindURL(t *testing.T) {
	for line, want := range map[string]string{
		"  \x1b[32m➜\x1b[39m  \x1b[1mLocal\x1b[22m:   \x1b[36mhttp://localhost:\x1b[1m5173\x1b[22m/\x1b[39m": "http://localhost:5173/",
		"  ➜  Network: use --host to expose":                               "",
		"ready - started server on 0.0.0.0:3000, url: http://0.0.0.0:3000": "http://localhost:3000",
		"Listening on http://127.0.0.1:8080/app/":                          "http://127.0.0.1:8080/app/",
		"Local: https://[::1]:4443/":                                       "https://[::1]:4443/",
		"see https://vitejs.dev/config for help":                           "",
	} {
		if got := findURL(line); got != want {
			t.Errorf("findURL(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestScanURL(t *testing.T) {
	in := "VITE v7.1.0  ready in 120 ms\n\n  ➜  Local:   http://localhost:5173/\n  ➜  Local:   http://localhost:9999/\nhmr update\n"
	var out bytes.Buffer
	found := make(chan string, 2)
	scanURL(strings.NewReader(in), &out, found)
	if u := <-found; u != "http://localhost:5173/" {
		t.Errorf("url = %q", u)
	}
	if len(found) != 0 {
		t.Error("more than one URL sent")
	}
	if out.String() != in {
		t.Errorf("output not passed through:\n%q", out.String())
	}
}

func TestDevServerURLAndStop(t *testing.T) {
	if testing.Short() {
		t.Skip("starts processes")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	srv, err := startDevServer(dir, fakeDevServer, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case u := <-srv.url:
		if u != "http://localhost:4321/" {
			t.Fatalf("url = %q", u)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no URL")
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	srv.stop()
	if time.Since(start) > 2*time.Second {
		t.Errorf("stop took %v; SIGTERM should end it", time.Since(start))
	}
	select {
	case <-srv.exited:
	default:
		t.Fatal("exited not closed after stop")
	}
	assertGone(t, strings.TrimSpace(string(b)))
}

func TestServeDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0o644)
	u, closeFn, err := serveDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	res, err := http.Get(u + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "<h1>hi</h1>" || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("got %q %v", body, res.Header)
	}
	if _, _, err := serveDir(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing dir served")
	}
}

func TestWatcher(t *testing.T) {
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main")
	write("frontend/node_modules/x/x.go", "package x")
	w := newWatcher(root, filepath.Join(root, "frontend"))
	if w.changed() {
		t.Fatal("changed with no edits")
	}
	for _, ignored := range []string{"frontend/a.go", "node_modules/y.go", "build/z.go", ".git/h.go", "main_test.go", "README.md"} {
		write(ignored, "x")
		if w.changed() {
			t.Errorf("edit to %s noticed", ignored)
		}
	}
	write("internal/svc/svc.go", "package svc")
	if !w.changed() {
		t.Error("new Go file missed")
	}
	future := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(root, "main.go"), future, future)
	if !w.changed() {
		t.Error("mtime change missed")
	}
	write("go.mod", "module x")
	if !w.changed() {
		t.Error("go.mod missed")
	}
	os.Remove(filepath.Join(root, "internal/svc/svc.go"))
	if !w.changed() {
		t.Error("removal missed")
	}
}

func TestProcessExitStatus(t *testing.T) {
	p, err := startProcess(shellCommand("exit 3"))
	if err != nil {
		t.Fatal(err)
	}
	<-p.done
	if p.err == nil {
		t.Error("exit 3 reported as success")
	}
	p.stop(time.Second) // after exit: must not block
}

func TestDevDir(t *testing.T) {
	cfg := &Config{ID: "com.example.devdir", Name: "Dev Dir"}
	out, err := devBinary(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	want := filepath.Join(cache, "tango", "dev", cfg.ID)
	if got := devDir(out); got != want {
		t.Fatalf("devDir(%s) = %s, want %s", out, got, want)
	}
	os.RemoveAll(want)
}
