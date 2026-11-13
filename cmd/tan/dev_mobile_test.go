package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseDevPlatform(t *testing.T) {
	for in, want := range map[string]string{
		"android":           "android ",
		"android/arm64":     "android arm64",
		"android/x86_64":    "android amd64",
		"ios-simulator":     "ios-simulator ",
		"ios":               "error",
		"android/mips":      "error",
		"ios-simulator/x86": "error",
		"darwin":            "error",
	} {
		goos, arch, err := parseDevPlatform(in)
		got := goos + " " + arch
		if err != nil {
			got = "error"
		}
		if got != want {
			t.Errorf("parseDevPlatform(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, _, err := parseDevPlatform("ios"); err == nil || !strings.Contains(err.Error(), "ios-simulator") {
		t.Errorf("ios: %v, want a hint at ios-simulator", err)
	}
}

func TestMobileDevFlags(t *testing.T) {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	m := addMobileDevFlags(fs)
	if err := fs.Parse([]string{"-platform", "android", "-device", "emulator-5554", "--", "x"}); err != nil {
		t.Fatal(err)
	}
	if !m.wanted() || *m.platform != "android" || *m.device != "emulator-5554" || fs.Arg(0) != "x" {
		t.Errorf("flags: %q %q %v", *m.platform, *m.device, fs.Args())
	}
	fs = flag.NewFlagSet("dev", flag.ContinueOnError)
	if m := addMobileDevFlags(fs); fs.Parse(nil) != nil || m.wanted() {
		t.Error("no -platform should mean the desktop")
	}
}

func TestLoopbackPort(t *testing.T) {
	for in, want := range map[string]string{
		"http://localhost:5173/":    "5173",
		"http://127.0.0.1:61234":    "61234",
		"http://[::1]:4000/x":       "4000",
		"http://localhost/":         "80",
		"https://localhost/":        "443",
		"http://192.168.1.2:5173/":  "",
		"http://my-mac.local:5173/": "",
		"":                          "",
	} {
		if got := loopbackPort(in); got != want {
			t.Errorf("loopbackPort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShQuote(t *testing.T) {
	if got := shQuote("http://localhost:1/?a=1&b='x'"); got != `'http://localhost:1/?a=1&b='\''x'\'''` {
		t.Errorf("shQuote = %s", got)
	}
}

// fakeRunner records the commands it is given.
type fakeRunner struct {
	mu       sync.Mutex
	calls    []string
	outputs  map[string]string // command substring → output
	fail     map[string]error  // command substring → error
	launched chan string       // receives start's command line
	procs    []*fakeProc
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{outputs: map[string]string{}, fail: map[string]error{}, launched: make(chan string, 16)}
}

func (f *fakeRunner) record(prefix, name string, args []string) string {
	line := strings.Join(append([]string{filepath.Base(name)}, args...), " ")
	f.mu.Lock()
	f.calls = append(f.calls, prefix+line)
	f.mu.Unlock()
	return line
}

func (f *fakeRunner) add(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeRunner) errFor(line string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, err := range f.fail {
		if strings.Contains(line, k) {
			return err
		}
	}
	return nil
}

func (f *fakeRunner) run(ctx context.Context, name string, args ...string) error {
	return f.errFor(f.record("", name, args))
}

func (f *fakeRunner) output(ctx context.Context, name string, args ...string) (string, error) {
	line := f.record("", name, args)
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range f.outputs {
		if strings.Contains(line, k) {
			return v, nil
		}
	}
	return "", nil
}

func (f *fakeRunner) start(env []string, w io.Writer, name string, args ...string) (bgProcess, error) {
	line := f.record("start: "+strings.Join(env, " ")+" ", name, args)
	p := &fakeProc{f: f, line: line, ch: make(chan struct{})}
	f.mu.Lock()
	f.procs = append(f.procs, p)
	f.mu.Unlock()
	f.launched <- line
	return p, nil
}

func (f *fakeRunner) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

type fakeProc struct {
	f    *fakeRunner
	line string
	once sync.Once
	ch   chan struct{}
}

func (p *fakeProc) done() <-chan struct{} { return p.ch }
func (p *fakeProc) stop() {
	p.f.add("stop: " + p.line)
	p.exit()
}

// exit ends the process as if it had quit by itself.
func (p *fakeProc) exit() { p.once.Do(func() { close(p.ch) }) }

// mobileProject is a project directory with one Go file.
func mobileProject(t *testing.T) *Config {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Config{Name: "Demo", ID: "dev.tango.demo", Version: "0.1.0", dir: dir}
}

func waitLaunch(t *testing.T, f *fakeRunner) string {
	t.Helper()
	select {
	case l := <-f.launched:
		return l
	case <-time.After(10 * time.Second):
		t.Fatalf("no launch; calls:\n%s", strings.Join(f.log(), "\n"))
		return ""
	}
}

// touch rewrites a file so the watcher sees it change.
func touch(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(path, later, later)
}

// runSession runs s.loop until the returned stop is called.
func runSession(s *mobileSession) (stop func() error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.loop(ctx) }()
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(10 * time.Second):
			return errors.New("loop did not return")
		}
	}
}

func checkCalls(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("commands:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAndroidDevSequence(t *testing.T) {
	cfg := mobileProject(t)
	f := newFakeRunner()
	f.outputs["getprop ro.product.cpu.abilist"] = "x86_64,arm64-v8a\n"
	f.outputs["resolve-activity"] = "priority=0 preferredOrder=0\ndev.tango.demo/org.tango.host.TangoActivity\n"
	f.outputs["pidof"] = "4242\n"
	builds := 0
	a := &androidDev{cfg: cfg, run: f, adb: "/sdk/platform-tools/adb",
		pick: func(context.Context) (string, error) { return "emulator-5554", nil },
		buildFn: func(_ context.Context, arches []string) (string, error) {
			builds++
			f.add("BUILD " + strings.Join(arches, ","))
			return "/cache/demo-0.1.0-debug.apk", nil
		}}
	s := &mobileSession{cfg: cfg, t: a, url: "http://localhost:5173/", tick: 20 * time.Millisecond}
	stop := runSession(s)
	waitLaunch(t, f)
	first := len(f.log())
	touch(t, filepath.Join(cfg.dir, "main.go"), "package main // changed\n")
	waitLaunch(t, f)
	afterChange := len(f.log())
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	calls := f.log()
	checkCalls(t, calls[:first], []string{
		"adb -s emulator-5554 shell getprop ro.product.cpu.abilist",
		"adb -s emulator-5554 reverse tcp:5173 tcp:5173",
		"BUILD amd64",
		"adb -s emulator-5554 install -r -t /cache/demo-0.1.0-debug.apk",
		"adb -s emulator-5554 shell cmd package resolve-activity --brief -c android.intent.category.LAUNCHER dev.tango.demo",
		"adb -s emulator-5554 shell am start -S -W -n 'dev.tango.demo/org.tango.host.TangoActivity' -e TANGO_DEV_URL 'http://localhost:5173/'",
		"adb -s emulator-5554 shell pidof -s dev.tango.demo",
		"start:  adb -s emulator-5554 logcat -v brief --pid=4242",
	})
	// A Go change: rebuild, reinstall, restart (the log stream follows the
	// new process).
	checkCalls(t, calls[first:afterChange], []string{
		"BUILD amd64",
		"adb -s emulator-5554 install -r -t /cache/demo-0.1.0-debug.apk",
		"stop: adb -s emulator-5554 logcat -v brief --pid=4242",
		"adb -s emulator-5554 shell am start -S -W -n 'dev.tango.demo/org.tango.host.TangoActivity' -e TANGO_DEV_URL 'http://localhost:5173/'",
		"adb -s emulator-5554 shell pidof -s dev.tango.demo",
		"start:  adb -s emulator-5554 logcat -v brief --pid=4242",
	})
	// Ctrl-C: stop the log stream and the app, remove the reverse forward.
	checkCalls(t, calls[afterChange:], []string{
		"stop: adb -s emulator-5554 logcat -v brief --pid=4242",
		"adb -s emulator-5554 shell am force-stop dev.tango.demo",
		"adb -s emulator-5554 reverse --remove tcp:5173",
	})
	if builds != 2 {
		t.Errorf("%d builds, want 2", builds)
	}
}

func TestAndroidDevBuildFailureKeepsApp(t *testing.T) {
	cfg := mobileProject(t)
	f := newFakeRunner()
	f.outputs["pidof"] = "7\n"
	fail := false
	var mu sync.Mutex
	a := &androidDev{cfg: cfg, run: f, adb: "adb", arch: "arm64",
		pick: func(context.Context) (string, error) { return "R5C", nil },
		buildFn: func(context.Context, []string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			f.add("BUILD")
			if fail {
				return "", errors.New("compile error")
			}
			return "app.apk", nil
		}}
	// No front end: no reverse, no TANGO_DEV_URL; -platform android/arm64:
	// no ABI query.
	s := &mobileSession{cfg: cfg, t: a, tick: 20 * time.Millisecond}
	stop := runSession(s)
	waitLaunch(t, f)
	mu.Lock()
	fail = true
	mu.Unlock()
	touch(t, filepath.Join(cfg.dir, "main.go"), "package main // broken\n")
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(strings.Join(f.log(), "\n"), "BUILD") < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	checkCalls(t, f.log(), []string{
		"BUILD",
		"adb -s R5C install -r -t app.apk",
		"adb -s R5C shell cmd package resolve-activity --brief -c android.intent.category.LAUNCHER dev.tango.demo",
		"adb -s R5C shell am start -S -W -n 'dev.tango.demo/org.tango.host.TangoActivity'",
		"adb -s R5C shell pidof -s dev.tango.demo",
		"start:  adb -s R5C logcat -v brief --pid=7",
		"BUILD",
		"stop: adb -s R5C logcat -v brief --pid=7",
		"adb -s R5C shell am force-stop dev.tango.demo",
	})
}

func TestIOSSimulatorDevSequence(t *testing.T) {
	cfg := mobileProject(t)
	f := newFakeRunner()
	sim := &iosSimDev{cfg: cfg, run: f,
		pick: func(context.Context) (simDevice, error) {
			return simDevice{UDID: "UDID-1", Name: "iPhone 18 Pro", State: "Shutdown"}, nil
		},
		buildFn: func(context.Context) (string, error) {
			f.add("BUILD")
			return "/cache/Demo.app", nil
		}}
	s := &mobileSession{cfg: cfg, t: sim, url: "http://localhost:5173/", tick: 20 * time.Millisecond}
	stop := runSession(s)
	waitLaunch(t, f)
	first := len(f.log())
	touch(t, filepath.Join(cfg.dir, "main.go"), "package main // changed\n")
	waitLaunch(t, f)
	afterChange := len(f.log())
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	calls := f.log()
	launch := "start: SIMCTL_CHILD_TANGO_DEV_URL=http://localhost:5173/ xcrun simctl launch --console-pty UDID-1 dev.tango.demo"
	stopLaunch := "stop: xcrun simctl launch --console-pty UDID-1 dev.tango.demo"
	checkCalls(t, calls[:first], []string{
		"xcrun simctl bootstatus UDID-1 -b",
		"open -a Simulator --args -CurrentDeviceUDID UDID-1",
		"BUILD",
		"xcrun simctl install UDID-1 /cache/Demo.app",
		"xcrun simctl terminate UDID-1 dev.tango.demo",
		launch,
	})
	checkCalls(t, calls[first:afterChange], []string{
		"BUILD",
		"xcrun simctl install UDID-1 /cache/Demo.app",
		"xcrun simctl terminate UDID-1 dev.tango.demo",
		stopLaunch,
		launch,
	})
	checkCalls(t, calls[afterChange:], []string{
		"xcrun simctl terminate UDID-1 dev.tango.demo",
		stopLaunch,
	})
}

// A front end served as is: edits restart the app without a rebuild.
func TestMobileDevServedFrontendRestarts(t *testing.T) {
	cfg := mobileProject(t)
	web := filepath.Join(cfg.dir, "web")
	os.MkdirAll(web, 0o755)
	touch(t, filepath.Join(web, "index.html"), "<p>one</p>")
	cfg.Frontend = Frontend{Dir: "web", Dist: "."}
	f := newFakeRunner()
	builds := 0
	sim := &iosSimDev{cfg: cfg, run: f,
		pick: func(context.Context) (simDevice, error) {
			return simDevice{UDID: "U", Name: "iPhone", State: "Booted"}, nil
		},
		buildFn: func(context.Context) (string, error) { builds++; return "Demo.app", nil }}
	s := &mobileSession{cfg: cfg, t: sim, url: "http://127.0.0.1:1234", served: cfg.DistDir(), tick: 20 * time.Millisecond}
	stop := runSession(s)
	waitLaunch(t, f)
	time.Sleep(50 * time.Millisecond)
	touch(t, filepath.Join(web, "index.html"), "<p>two!</p>")
	waitLaunch(t, f)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if builds != 1 {
		t.Errorf("%d builds, want 1 (front-end edits must not rebuild)", builds)
	}
	if n := strings.Count(strings.Join(f.log(), "\n"), "simctl install"); n != 1 {
		t.Errorf("%d installs, want 1", n)
	}
}

// The app exiting on its own (simctl launch returns) is reported once and
// the loop keeps watching.
func TestMobileDevAppExit(t *testing.T) {
	cfg := mobileProject(t)
	f := newFakeRunner()
	sim := &iosSimDev{cfg: cfg, run: f,
		pick: func(context.Context) (simDevice, error) {
			return simDevice{UDID: "U", Name: "iPhone", State: "Booted"}, nil
		},
		buildFn: func(context.Context) (string, error) { return "Demo.app", nil }}
	s := &mobileSession{cfg: cfg, t: sim, tick: 20 * time.Millisecond}
	stop := runSession(s)
	waitLaunch(t, f)
	f.mu.Lock()
	p := f.procs[0]
	f.mu.Unlock()
	p.exit() // the app quit
	// The loop notices and forgets the app: simctl terminate, once more.
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(strings.Join(f.log(), "\n"), "simctl terminate") < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := strings.Count(strings.Join(f.log(), "\n"), "simctl terminate"); n != 2 {
		t.Errorf("%d terminates after the app exited, want 2", n)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestMobileDevPrepareError(t *testing.T) {
	cfg := mobileProject(t)
	f := newFakeRunner()
	a := &androidDev{cfg: cfg, run: f, adb: "adb",
		pick:    func(context.Context) (string, error) { return "", errors.New("no device") },
		buildFn: func(context.Context, []string) (string, error) { t.Error("built without a device"); return "", nil }}
	s := &mobileSession{cfg: cfg, t: a, url: "http://localhost:1/"}
	if err := s.loop(context.Background()); err == nil || err.Error() != "no device" {
		t.Errorf("loop = %v", err)
	}
	if len(f.log()) != 0 {
		t.Errorf("commands without a device: %v", f.log())
	}
}

// On Android behind a dev server the front-end sources are watched, but
// not the build output or node_modules.
func TestTreeStampSkip(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"src", "dist", "node_modules/x"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	touch(t, filepath.Join(dir, "src", "main.ts"), "a")
	ts := newTreeStamp(dir, filepath.Join(dir, "dist"))
	touch(t, filepath.Join(dir, "dist", "index.js"), "built")
	touch(t, filepath.Join(dir, "node_modules", "x", "i.js"), "dep")
	if ts.changed() {
		t.Error("dist or node_modules changes were seen")
	}
	touch(t, filepath.Join(dir, "src", "main.ts"), "ab")
	if !ts.changed() {
		t.Error("a source change was missed")
	}
}
