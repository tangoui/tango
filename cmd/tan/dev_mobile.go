package main

// tan dev -platform android | ios-simulator: the mobile dev loop.
//
// The front end is served as on desktop (frontend.dev, -url, or the dist
// directory as is) and the app on the device loads it through
// asset.FromEnv: Android gets the URL as the launch Intent's TANGO_DEV_URL
// extra (debuggable APKs only, internal/platform/android/devurl_android.go)
// and reaches the host's localhost through adb reverse; the iOS Simulator
// shares the host's network and gets the URL as an environment variable
// (SIMCTL_CHILD_TANGO_DEV_URL). Front-end edits show through the dev
// server's HMR on the Simulator. On Android the HMR WebSocket does not reach
// the WebView, so front-end edits restart the app (no rebuild), as they do
// everywhere when the dist directory is served as is.
//
// Go edits are rebuilt (a debug build of the device's architecture only),
// reinstalled and the app restarted. Self-drawn UI is not hot-reloaded on
// mobile (docs/specs/ui-hotreload.md, "移动端").

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// mobileDevFlags are tan dev's mobile flags.
type mobileDevFlags struct {
	platform *string
	device   *string
}

func addMobileDevFlags(fs *flag.FlagSet) mobileDevFlags {
	return mobileDevFlags{
		platform: fs.String("platform", "", "run on a phone instead of the desktop: android (a connected device or a running emulator; android/<arch> to force the ABI) or ios-simulator"),
		device:   fs.String("device", "", "with -platform: the adb serial or the Simulator UDID or name (default: the only/first one running)"),
	}
}

func (f mobileDevFlags) wanted() bool { return *f.platform != "" }

// parseDevPlatform checks a tan dev -platform value. arch is set for
// android/<arch>.
func parseDevPlatform(s string) (goos, arch string, err error) {
	goos, arch, hasArch := strings.Cut(s, "/")
	switch goos {
	case osAndroid:
		if !hasArch {
			return goos, "", nil
		}
		a, ok := parseAndroidArch(arch)
		if !ok {
			return "", "", fmt.Errorf("-platform %s: unsupported Android architecture (want arm64, amd64, arm or 386)", s)
		}
		return goos, a, nil
	case osIOSSimulator:
		if hasArch && arch != "arm64" {
			return "", "", errors.New("-platform ios-simulator: the Simulator build is arm64 only")
		}
		return goos, "", nil
	case osIOS:
		return "", "", errors.New("tan dev does not run on iOS devices yet: use -platform ios-simulator, or tan build -platform ios -debug and install the .ipa with Xcode (Devices and Simulators) or xcrun devicectl device install app")
	}
	return "", "", fmt.Errorf("-platform %s: tan dev runs on android or ios-simulator (leave -platform out for the desktop)", s)
}

// devMobile is tan dev -platform ...
func devMobile(ctx context.Context, cfg *Config, devURL string, appArgs []string, f mobileDevFlags) error {
	goos, arch, err := parseDevPlatform(*f.platform)
	if err != nil {
		return err
	}
	if len(appArgs) > 0 {
		warnf("App arguments (%s) are ignored on %s", strings.Join(appArgs, " "), goos)
	}
	var t mobileTarget
	switch goos {
	case osAndroid:
		if err := checkAndroidBuild(cfg, true); err != nil {
			return err
		}
		tools, err := androidTools()
		if err != nil {
			return err
		}
		dir := mobileDevDir(cfg, osAndroid)
		if _, err := os.Stat(tools.adb()); err != nil {
			return fmt.Errorf("adb not found at %s: install the SDK's platform-tools", tools.adb())
		}
		want := *f.device
		t = &androidDev{cfg: cfg, run: execRunner{}, adb: tools.adb(), arch: arch,
			pick: func(ctx context.Context) (string, error) {
				return androidDevice(ctx, tools, adb{path: tools.adb()}, want, "")
			},
			buildFn: func(ctx context.Context, arches []string) (string, error) {
				a, err := buildAndroid(ctx, cfg, arches, dir, true, true)
				return a.path, err
			}}
	case osIOSSimulator:
		tg := target{osIOSSimulator, "arm64"}
		if err := checkIOSBuild(cfg, tg, true); err != nil {
			return err
		}
		dir := mobileDevDir(cfg, osIOSSimulator)
		want := *f.device
		t = &iosSimDev{cfg: cfg, run: execRunner{},
			pick: func(ctx context.Context) (simDevice, error) {
				ds, err := simDevices(ctx)
				if err != nil {
					return simDevice{}, err
				}
				return pickSimulator(ds, want)
			},
			buildFn: func(ctx context.Context) (string, error) {
				a, err := buildIOS(ctx, cfg, tg, dir, true, true)
				return a.path, err
			}}
	}

	devURL, served, closeFn, err := startDevFrontend(ctx, cfg, devURL)
	defer closeFn()
	if err != nil || ctx.Err() != nil {
		return err
	}
	s := &mobileSession{cfg: cfg, t: t, url: devURL, served: served}
	if goos == osAndroid && served == "" && devURL != "" && cfg.HasFrontend() {
		// The page is https://tango.localhost, served through Go; the
		// WebView's WebSocket to the dev server's HMR endpoint does not get
		// through (wss to a plain server, and Android WebView does not route
		// WebSockets through the asset handler). Restart the app instead.
		logf("Android: front-end edits restart the app (the dev server's HMR WebSocket does not reach the WebView)")
		s.served, s.servedSkip = cfg.FrontendDir(), cfg.DistDir()
	}
	return s.loop(ctx)
}

// mobileDevDir is where mobile dev builds go, next to the desktop ones
// (devBinary), so Gradle's caches survive across tan dev sessions.
func mobileDevDir(cfg *Config, goos string) string {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "tango", "dev", cfg.ID, goos)
}

// startDevFrontend serves the front end as dev does on desktop: url (from
// -url or frontend.devURL), frontend.dev, or the dist directory as is, in
// which case served is that directory. Without a front end the URL is "".
// A cancelled ctx returns an empty URL and a nil error.
func startDevFrontend(ctx context.Context, cfg *Config, url string) (devURL, served string, closeFn func(), err error) {
	closeFn = func() {}
	if url == "" {
		url = cfg.Frontend.DevURL
	}
	switch {
	case url != "":
	case !cfg.HasFrontend():
		// Self-drawn UI: nothing to serve.
	case cfg.Frontend.Dev != "":
		if err := installFrontend(ctx, cfg); err != nil {
			return "", "", closeFn, err
		}
		logf("Starting the front-end dev server: %s", cfg.Frontend.Dev)
		srv, err := startDevServer(cfg.FrontendDir(), cfg.Frontend.Dev, os.Stdout)
		if err != nil {
			return "", "", closeFn, err
		}
		closeFn = srv.stop
		select {
		case url = <-srv.url:
		case <-srv.exited:
			return "", "", closeFn, errors.New("the front-end dev server exited before printing its URL")
		case <-time.After(90 * time.Second):
			return "", "", closeFn, errors.New(`no URL from the front-end dev server after 90s; set "frontend.devURL" or pass -url`)
		case <-ctx.Done():
			return "", "", closeFn, nil
		}
	default:
		u, c, err := serveDir(cfg.DistDir())
		if err != nil {
			return "", "", closeFn, err
		}
		closeFn, url, served = c, u, cfg.DistDir()
	}
	if url != "" {
		logf("Front end at %s", url)
	}
	return url, served, closeFn, nil
}

// mobileTarget is a device or Simulator tan dev deploys to.
type mobileTarget interface {
	// prepare picks the device and makes url reachable from it.
	prepare(ctx context.Context, url string) error
	describe() string
	// build makes a debug build and returns the APK or .app.
	build(ctx context.Context) (string, error)
	install(ctx context.Context, path string) error
	// launch (re)starts the app with url and streams its log.
	launch(ctx context.Context, url string) error
	// exited is closed when the running app's log stream ends, nil when
	// that is not known.
	exited() <-chan struct{}
	// stop stops the app and its log stream; cleanup undoes prepare.
	stop(ctx context.Context)
	cleanup(ctx context.Context)
}

// mobileSession is the mobile dev loop.
type mobileSession struct {
	cfg    *Config
	t      mobileTarget
	url    string
	served string // the dist directory, when served as is: restart on edits
	// servedSkip is a directory under served not to watch (the build
	// output, when served holds the front-end sources).
	servedSkip string
	tick       time.Duration
}

func (s *mobileSession) loop(ctx context.Context) error {
	if err := s.t.prepare(ctx, s.url); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	defer func() {
		// ctx is done by now (Ctrl-C): clean up with a fresh one.
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.t.stop(c)
		s.t.cleanup(c)
		logf("Stopped %s on %s", s.cfg.Name, s.t.describe())
	}()

	w := newWatcher(s.cfg.dir, s.cfg.FrontendDir())
	var front *treeStamp
	if s.served != "" {
		front = newTreeStamp(s.served, s.servedSkip)
	}
	installed := false
	deploy := func() {
		start := time.Now()
		logf("Building %s for %s", s.cfg.Name, s.t.describe())
		path, err := s.t.build(ctx)
		if err != nil {
			if ctx.Err() == nil {
				if installed {
					warnf("Build failed: %v; the installed app keeps running", err)
				} else {
					warnf("Build failed: %v; waiting for changes", err)
				}
			}
			return
		}
		logf("Installing")
		if err := s.t.install(ctx, path); err != nil {
			if ctx.Err() == nil {
				warnf("Install failed: %v", err)
			}
			return
		}
		installed = true
		if err := s.t.launch(ctx, s.url); err != nil {
			if ctx.Err() == nil {
				warnf("Launch failed: %v", err)
			}
			return
		}
		logf("%s running on %s (%v); watching Go files", s.cfg.Name, s.t.describe(), time.Since(start).Round(100*time.Millisecond))
	}
	deploy()

	interval := s.tick
	if interval == 0 {
		interval = 400 * time.Millisecond
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.t.exited():
			logf("The app exited; waiting for changes (Ctrl-C to stop)")
			s.t.stop(ctx)
		case <-tick.C:
			switch {
			case w.changed():
				time.Sleep(150 * time.Millisecond) // let editors finish writing
				w.changed()
				deploy()
			case front != nil && front.changed() && installed:
				time.Sleep(150 * time.Millisecond)
				front.changed()
				logf("Front-end files changed; restarting the app")
				if err := s.t.launch(ctx, s.url); err != nil && ctx.Err() == nil {
					warnf("Launch failed: %v", err)
				}
			}
		}
	}
}

// treeStamp fingerprints every file under a directory (names, sizes,
// mtimes), for front ends served as is (or, on Android, front-end
// sources behind a dev server).
type treeStamp struct {
	dir, skip, last string
}

func newTreeStamp(dir, skip string) *treeStamp {
	t := &treeStamp{dir: dir, skip: skip}
	t.last = t.snapshot()
	return t
}

func (t *treeStamp) snapshot() string {
	var b strings.Builder
	_ = filepath.WalkDir(t.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != t.dir && (p == t.skip || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil {
			fmt.Fprintf(&b, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return b.String()
}

func (t *treeStamp) changed() bool {
	s := t.snapshot()
	if s == t.last {
		return false
	}
	t.last = s
	return true
}

// loopbackPort returns the port of a URL on the host's loopback
// (localhost, 127.0.0.1, ::1), which adb reverse can forward, or "".
func loopbackPort(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	switch h := p.Hostname(); h {
	case "localhost", "127.0.0.1", "::1":
	default:
		return ""
	}
	if port := p.Port(); port != "" {
		return port
	}
	if p.Scheme == "https" {
		return "443"
	}
	return "80"
}

// ---- running commands ----

// devRunner runs the external tools (adb, xcrun); tests replace it.
type devRunner interface {
	// run runs a command to completion; its output is part of the error.
	run(ctx context.Context, name string, args ...string) error
	// output runs a command and returns its standard output.
	output(ctx context.Context, name string, args ...string) (string, error)
	// start starts a long-running command (a log stream) writing to w,
	// with env added to its environment.
	start(env []string, w io.Writer, name string, args ...string) (bgProcess, error)
}

// bgProcess is a command started by devRunner.start.
type bgProcess interface {
	done() <-chan struct{}
	stop()
}

type execRunner struct{}

func (execRunner) run(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return fmt.Errorf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, s)
		}
		return fmt.Errorf("%s %s: %v", filepath.Base(name), strings.Join(args, " "), err)
	}
	return nil
}

func (execRunner) output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %v %s", filepath.Base(name), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func (execRunner) start(env []string, w io.Writer, name string, args ...string) (bgProcess, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = w, w
	// Its own process group (startProcess), so Ctrl-C reaches only tan.
	p, err := startProcess(cmd)
	if err != nil {
		return nil, err
	}
	return execProc{p}, nil
}

type execProc struct{ p *process }

func (e execProc) done() <-chan struct{} { return e.p.done }
func (e execProc) stop()                 { e.p.stop(2 * time.Second) }
