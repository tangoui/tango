package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tangoui/tango/asset"
)

func runDev(args []string) error {
	fs := newFlags("dev", "[flags] [-- app args]")
	dir := projectFlag(fs)
	url := fs.String("url", "", "use the dev server already running at `url` instead of starting frontend.dev")
	hot := fs.Bool("hot", os.Getenv("TANGO_HOTRELOAD") != "0", "hot-reload self-drawn UI (uihost) windows instead of restarting the app; TANGO_HOTRELOAD=0 turns it off")
	watchFW := fs.Bool("watch-framework", true, "restart the dev host when the Tango sources it links change (local checkouts only)")
	noRestore := fs.Bool("no-restore", false, "do not restore window positions and sizes when the app restarts")
	mobile := addMobileDevFlags(fs) // dev_mobile.go
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := argsAfterDashDash(fs, args); err != nil {
		return err
	}
	cfg, err := LoadConfig(*dir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if mobile.wanted() {
		return devMobile(ctx, cfg, *url, fs.Args(), mobile)
	}
	return dev(ctx, cfg, *url, fs.Args(), devModes{hot: *hot, watchFramework: *watchFW, noRestore: *noRestore})
}

// devModes are dev's switches.
type devModes struct {
	hot            bool // hot reload when the app links uihost
	watchFramework bool // restart the dev host on framework changes
	noRestore      bool // do not keep window placement across restarts
}

const devURLTimeout = 90 * time.Second

var errDevURLTimeout = errors.New(`no URL from the front-end dev server after 90s; set "frontend.devURL" or pass -url`)

// dev serves the front end (its dev server, or the dist directory as is),
// then builds and runs the app, rebuilding when Go sources change, until ctx
// ends or the app exits cleanly.
func dev(ctx context.Context, cfg *Config, url string, appArgs []string, modes devModes) error {
	var startup *frontendStartup
	if url == "" {
		url = cfg.Frontend.DevURL
	}
	switch {
	case url != "":
	case !cfg.HasFrontend():
		// Self-drawn UI: nothing to serve.
	case cfg.Frontend.Dev != "":
		if err := installFrontend(ctx, cfg); err != nil {
			return err
		}
		logf("Starting the front-end dev server: %s", cfg.Frontend.Dev)
		if cfg.Frontend.ParallelDevBuild {
			if ctx.Err() != nil {
				return nil
			}
			srv, stop, err := startParallelDevServer(cfg.FrontendDir(), cfg.Frontend.Dev, os.Stdout)
			if err != nil {
				return err
			}
			defer stop()
			startup = watchFrontend(ctx, srv, time.NewTimer(time.Until(srv.deadline)))
			defer startup.close()
			break
		}
		srv, err := startDevServer(cfg.FrontendDir(), cfg.Frontend.Dev, os.Stdout)
		if err != nil {
			return err
		}
		defer srv.stop()
		select {
		case url = <-srv.url:
		case <-srv.exited:
			return errors.New("the front-end dev server exited before printing its URL")
		case <-time.After(devURLTimeout):
			return errDevURLTimeout
		case <-ctx.Done():
			return nil
		}
	default:
		// No dev server: serve the front-end sources directly, so edits
		// show up on reload without rebuilding the app.
		u, closeFn, err := serveDir(cfg.DistDir())
		if err != nil {
			return err
		}
		defer closeFn()
		url = u
	}
	out, err := devBinary(cfg)
	if err != nil {
		return err
	}
	var env []string
	if !modes.noRestore {
		// The app (and the hot-reload host) restore their windows'
		// placement from this file; see internal/devfeat/windowstate.go.
		env = append(env, devWindowStateEnv+"="+filepath.Join(devDir(out), "windows.json"))
	}
	s := &devSession{cfg: cfg, out: out, env: env, args: appArgs, startup: startup}
	s.setFrontendURL(url)
	if modes.hot {
		err = s.hotLoop(ctx, modes.watchFramework)
	}
	if !modes.hot || errors.Is(err, errNotHot) {
		err = s.loop(ctx)
	}
	if s.startup != nil {
		if cause := s.startup.failure(); cause != nil {
			if cause == ctx.Err() {
				return nil
			}
			return cause
		}
	}
	return err
}

// devBinary is where dev builds go: a fixed path per app id in the user
// cache, not a fresh temp dir like go run. macOS keys privacy permissions
// (camera, files, ...) and Keychain access to the app's location and
// identity, so a stable path (and bundle id) keeps them from being asked for
// again on every run. Ad-hoc signed builds still change identity with each
// build; signing dev builds with an Apple Development certificate would fix
// that and is not done yet.
func devBinary(cfg *Config) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	dir := filepath.Join(cache, "tango", "dev", cfg.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(dir, cfg.Name+".app", "Contents", "MacOS", cfg.Exe()), nil
	case "windows":
		return filepath.Join(dir, cfg.Exe()+".exe"), nil
	}
	return filepath.Join(dir, cfg.Exe()), nil
}

// devWindowStateEnv names the file in which dev builds keep their windows'
// placement across restarts (internal/devfeat/windowstate.go).
const devWindowStateEnv = "TANGO_DEV_WINDOW_STATE"

// devDir is the per-app dev directory devBinary put out in.
func devDir(out string) string {
	if runtime.GOOS == "darwin" { // <dir>/<Name>.app/Contents/MacOS/<exe>
		return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(out))))
	}
	return filepath.Dir(out)
}

type devSession struct {
	cfg  *Config
	out  string   // dev executable
	env  []string // added to the app's environment
	args []string

	startup     *frontendStartup
	frontendURL string
	bundled     bool // the macOS bundle around out has been written
}

func (s *devSession) setFrontendURL(url string) {
	if url == "" {
		return
	}
	s.frontendURL = url
	s.env = append(s.env, asset.DevURLEnv+"="+url)
	logf("Front end at %s", url)
	if loopbackPort(url) == "" {
		warnf("%s is not a loopback address: asset.FromEnv only proxies to localhost, 127.0.0.0/8 and ::1, so the app will not use it", url)
	}
}

func (s *devSession) startupContext(ctx context.Context) context.Context {
	if s.startup != nil {
		return s.startup.ctx
	}
	return ctx
}

func (s *devSession) prepareLaunch() error {
	if s.startup == nil || s.frontendURL != "" {
		return nil
	}
	url, err := s.startup.waitFrontend(s.startup.ctx)
	if err != nil {
		return err
	}
	s.setFrontendURL(url)
	return nil
}

func (s *devSession) checkLaunch() error {
	if s.startup != nil {
		return s.startup.failure()
	}
	return nil
}

func (s *devSession) finishStartup() {
	if s.startup != nil {
		s.startup.close()
		s.startup = nil
	}
}

func (s *devSession) goBuild(ctx context.Context, path string, stderr io.Writer) error {
	args := []string{"build", "-tags", buildTags(true), "-o", path, "."}
	var cmd *exec.Cmd
	if s.startup == nil {
		cmd = exec.CommandContext(ctx, "go", args...)
	} else {
		cmd = exec.Command("go", args...)
	}
	cmd.Dir = s.cfg.dir
	cmd.Env = append(os.Environ(), s.cfg.cgoEnv())
	cmd.Stdout, cmd.Stderr = os.Stdout, stderr
	if s.startup != nil {
		return runStartupCommand(s.startup.ctx, cmd)
	}
	return cmd.Run()
}

func (s *devSession) build(ctx context.Context) error {
	if err := s.goBuild(ctx, s.out, os.Stderr); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" && !s.bundled {
		s.bundled = true
		if err := writeBundle(s.cfg, s.app()); err != nil {
			return err
		}
	}
	if runtime.GOOS == "darwin" {
		return adhocSign(ctx, s.app())
	}
	return nil
}

// app is the macOS bundle around out.
func (s *devSession) app() string { return filepath.Dir(filepath.Dir(filepath.Dir(s.out))) }

// loop builds and runs the app, and rebuilds and restarts it when the
// project's Go files change. A failed build keeps the running app.
func (s *devSession) loop(ctx context.Context) error {
	w := newWatcher(s.cfg.dir, s.cfg.FrontendDir())
	tty := newTTYGuard() // tty_unix.go
	var app *process
	defer func() {
		if app != nil {
			app.stop(3 * time.Second)
		}
	}()
	start := func() error {
		buildCtx := s.startupContext(ctx)
		logf("Building")
		if err := s.build(buildCtx); err != nil {
			if buildCtx.Err() == nil {
				warnf("Build failed; waiting for changes")
			}
			return nil
		}
		if err := s.prepareLaunch(); err != nil {
			return err
		}
		if app != nil {
			app.stop(3 * time.Second)
			app = nil
		}
		cmd := exec.Command(s.out, s.args...)
		cmd.Dir = s.cfg.dir
		cmd.Env = append(os.Environ(), s.env...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := s.checkLaunch(); err != nil {
			return err
		}
		p, err := startProcess(cmd)
		if err != nil {
			warnf("Starting the app: %v", err)
			return nil
		}
		app = p
		s.finishStartup()
		logf("Running %s (pid %d); watching Go files", s.cfg.Name, cmd.Process.Pid)
		return nil
	}
	if err := start(); err != nil {
		return err
	}
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		var exited <-chan struct{}
		if app != nil {
			exited = app.done
		}
		select {
		case <-s.startupContext(ctx).Done():
			return nil
		case <-exited:
			tty.check()
			if app.err == nil {
				logf("The app quit")
				return nil
			}
			warnf("The app exited: %v; waiting for changes", app.err)
			app = nil
		case <-tick.C:
			tty.check()
			if w.changed() {
				// Let editors finish writing a batch of files.
				time.Sleep(150 * time.Millisecond)
				w.changed()
				if err := start(); err != nil {
					return err
				}
			}
		}
	}
}

// process is a started command in its own process group.
type process struct {
	cmd  *exec.Cmd
	done chan struct{} // closed when it has exited
	err  error         // its Wait error, valid after done
}

func startProcess(cmd *exec.Cmd) (*process, error) {
	newGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return waitProcess(cmd), nil
}

func waitProcess(cmd *exec.Cmd) *process {
	p := &process{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p
}

// stop asks the group to terminate and kills it after grace.
func (p *process) stop(grace time.Duration) {
	select {
	case <-p.done:
		if groupOutlivesLeader {
			signalGroup(p.cmd, true) // children may outlive the leader
		}
		return
	default:
	}
	signalGroup(p.cmd, false)
	select {
	case <-p.done:
		if groupOutlivesLeader {
			signalGroup(p.cmd, true)
		}
	case <-time.After(grace):
		signalGroup(p.cmd, true)
	}
	<-p.done
}

// devServer is the running front-end dev command.
type devServer struct {
	*process
	url    chan string   // receives the first URL it prints
	exited chan struct{} // closed when it exits
}

func startDevServer(dir, line string, out io.Writer) (*devServer, error) {
	cmd := shellCommand(line)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1")
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	p, err := startProcess(cmd)
	if err != nil {
		return nil, err
	}
	s := &devServer{process: p, url: make(chan string, 1), exited: p.done}
	go func() {
		<-p.done
		pw.Close()
	}()
	go scanURL(pr, out, s.url)
	return s, nil
}

func (s *devServer) stop() { s.process.stop(3 * time.Second) }

var (
	ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	urlPattern  = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\]|[A-Za-z0-9.-]+\.local)(?::\d+)?(?:/[^\s'"]*)?`)
)

// findURL returns the first local http(s) URL in a line of dev server
// output, without colour codes; 0.0.0.0 becomes localhost.
func findURL(line string) string {
	u := urlPattern.FindString(ansiPattern.ReplaceAllString(line, ""))
	return strings.Replace(u, "://0.0.0.0", "://localhost", 1)
}

// scanURL copies r to w line by line and sends the first URL found to
// found. It keeps copying until r ends.
func scanURL(r io.Reader, w io.Writer, found chan<- string) {
	scanDevOutput(r, w, func(url string) { found <- url }, false)
}

func scanDevOutput(r io.Reader, w io.Writer, report func(string), reportBeforeLog bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	sent := false
	for sc.Scan() {
		line := sc.Text()
		if !reportBeforeLog {
			fmt.Fprintln(w, line)
		}
		if !sent {
			if u := findURL(line); u != "" {
				report(u)
				sent = true
			}
		}
		if reportBeforeLog {
			fmt.Fprintln(w, line)
		}
	}
	_, _ = io.Copy(w, r)
}

// serveDir serves dir over HTTP on a free loopback port.
func serveDir(dir string) (url string, closeFn func(), err error) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", nil, fmt.Errorf("front-end directory %s is missing", dir)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	h := http.FileServer(http.Dir(dir))
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})}
	go srv.Serve(ln)
	return "http://" + ln.Addr().String(), func() { srv.Close() }, nil
}

// watcher polls a project for changes to Go sources and go.mod/go.sum.
// Polling avoids platform file-event APIs and copes with editors that
// replace files.
type watcher struct {
	root, frontend string
	last           string
}

func newWatcher(root, frontend string) *watcher {
	w := &watcher{root: root, frontend: frontend}
	w.last = w.snapshot()
	return w
}

// skipDir reports whether the watcher ignores a directory.
func (w *watcher) skipDir(path, name string) bool {
	if path == w.root {
		return false
	}
	switch name {
	case "node_modules", "build", "dist", "vendor", "testdata":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || path == w.frontend
}

// snapshot fingerprints the watched files: names, sizes and mtimes.
func (w *watcher) snapshot() string {
	var b strings.Builder
	_ = filepath.WalkDir(w.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if w.skipDir(p, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		n := d.Name()
		if !strings.HasSuffix(n, ".go") && n != "go.mod" && n != "go.sum" && n != "go.work" {
			return nil
		}
		if strings.HasSuffix(n, "_test.go") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(&b, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return b.String()
}

// changed reports whether anything changed since the last call.
func (w *watcher) changed() bool {
	s := w.snapshot()
	if s == w.last {
		return false
	}
	w.last = s
	return true
}
