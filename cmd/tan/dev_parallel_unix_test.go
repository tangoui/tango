//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/devhost"
)

const fixtureDevURL = "http://localhost:4321/"

func TestDevParallelHotStartup(t *testing.T) {
	cfg := parallelDevFixture(t, true)
	configureHotDevFixture(t, cfg)
	_, done := launchDevFixture(t, cfg, "", devModes{hot: true})
	if want, got := fixtureDevURL, waitDevMarker(t, cfg, "host-url"); got != want {
		t.Errorf("host URL = %q, want %q", got, want)
	}
	if want, got := fixtureDevURL, waitDevMarker(t, cfg, "worker-url"); got != want {
		t.Errorf("worker URL = %q, want %q", got, want)
	}
	writeDevFixtureFile(t, cfg, "release-host", "exit", 0o644)
	if err := waitDevResult(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestDevParallelHotCancelFrameworkLookup(t *testing.T) {
	cfg := parallelDevFixture(t, true)
	configureHotDevFixture(t, cfg)
	t.Setenv("GOMODCACHE", "")
	t.Setenv("TAN_TEST_FIXTURE_DIR", cfg.dir)
	bin := filepath.Join(cfg.dir, "bin")
	if err := os.Rename(filepath.Join(bin, "go"), filepath.Join(bin, "fake-build")); err != nil {
		t.Fatal(err)
	}
	writeDevFixtureFile(t, cfg, "bin/go", `#!/bin/sh
case "$1" in
    build) exec "$PWD/bin/fake-build" "$@" ;;
    list)  printf 'github.com/tangoui/tango\t%s\n' "$PWD" ;;
    env)   echo $$ > "$TAN_TEST_FIXTURE_DIR/lookup"; exec sleep 60 ;;
    *)     exit 2 ;;
esac
`, 0o755)
	cancel, done := launchDevFixture(t, cfg, "", devModes{hot: true, watchFramework: true})
	pid := waitDevMarker(t, cfg, "lookup")
	defer killDevFixturePID(pid)
	cancel()
	if err := waitDevResult(t, done); err != nil {
		t.Fatal(err)
	}
	assertGone(t, pid)
	if _, err := os.Stat(filepath.Join(cfg.dir, "worker-url")); !os.IsNotExist(err) {
		t.Fatalf("worker started after cancelling framework lookup: %v", err)
	}
}

func configureHotDevFixture(t *testing.T, cfg *Config) {
	t.Helper()
	cfg.Frontend.Dev = `while [ ! -e building ]; do sleep .02; done
echo http://localhost:4321/
exec sleep 60`
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TAN_TEST_HOT_APP", exe)
	writeDevGo(t, cfg, "", "# "+devhost.Marker+`
exec "$TAN_TEST_HOT_APP" -test.run '^TestParallelDevHotApp$'
`)
}

func TestParallelDevHotApp(t *testing.T) {
	if os.Getenv("TAN_TEST_HOT_APP") == "" {
		t.Skip("used as a helper process")
	}
	url := []byte(os.Getenv("TANGO_DEV_URL"))
	switch os.Getenv(devhost.EnvRole) {
	case devhost.RoleHost:
		if err := os.WriteFile("host-url", url, 0o644); err != nil {
			t.Fatal(err)
		}
		ln, err := devhost.Listen(os.Getenv(devhost.EnvAddr))
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			t.Fatal(err)
		}
		ctl := devhost.NewConn(conn)
		defer ctl.Close()
		msg, err := ctl.Recv()
		hello, ok := msg.(devhost.Hello)
		if err != nil || !ok || hello.Role != devhost.RoleCtl || hello.Token != os.Getenv(devhost.EnvToken) {
			t.Fatalf("host handshake: %v, %v", msg, err)
		}
		if err := ctl.Send(devhost.Welcome{}); err != nil {
			t.Fatal(err)
		}
		waitDevMarker(t, &Config{dir: "."}, "release-host")
	case devhost.RoleWorker:
		if err := os.WriteFile("worker-url", url, 0o644); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("app launched without hot reload")
	}
}

func TestDevParallelBuildStartsBeforeFrontendURL(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(fmt.Sprintf("parallel=%t", parallel), func(t *testing.T) {
			cfg := parallelDevFixture(t, parallel)
			server := `touch url-printed
echo http://localhost:4321/
exec sleep 60
`
			if parallel {
				server = `while [ ! -e building ]; do sleep .02; done
` + server
			}
			cfg.Frontend.Dev = server
			if !parallel {
				writeDevGo(t, cfg, `test -f url-printed || exit 42
`, "")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := dev(ctx, cfg, "", nil, devModes{}); err != nil {
				t.Fatal(err)
			}
			assertDevLaunch(t, cfg, fixtureDevURL)
		})
	}
}

func TestDevParallelFrontendExitBeforeLaunch(t *testing.T) {
	cfg := parallelDevFixture(t, true)
	cfg.Frontend.Dev = `echo http://localhost:4321/
while [ ! -e exit-frontend ]; do sleep .02; done
exit 23`
	writeDevGo(t, cfg, blockingDevBuild, "")
	cancel, done := launchDevFixture(t, cfg, "", devModes{})
	defer cancel()
	child := waitDevMarker(t, cfg, "build-child")
	writeDevFixtureFile(t, cfg, "exit-frontend", "", 0o644)
	if err := waitDevResult(t, done); err == nil || !strings.Contains(err.Error(), "23") {
		t.Fatalf("dev = %v, want the frontend exit status", err)
	}
	assertGone(t, child)
	assertNoDevLaunch(t, cfg)
}

func TestDevParallelStartupCancellation(t *testing.T) {
	t.Run("install failure", func(t *testing.T) {
		cfg := parallelDevFixture(t, true)
		writeDevFixtureFile(t, cfg, "package.json", "{}", 0o644)
		cfg.Frontend.Install = "exit 29"
		cfg.Frontend.Dev = "touch started-frontend"
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := dev(ctx, cfg, "", nil, devModes{}); err == nil || !strings.Contains(err.Error(), "install") {
			t.Fatalf("dev = %v, want the install error", err)
		}
		for _, name := range []string{"building", "started-frontend"} {
			if _, err := os.Stat(filepath.Join(cfg.dir, name)); !os.IsNotExist(err) {
				t.Errorf("%s exists or could not be checked: %v", name, err)
			}
		}
		assertNoDevLaunch(t, cfg)
	})

	t.Run("missing URL times out", func(t *testing.T) {
		cfg := parallelDevFixture(t, true)
		writeDevGo(t, cfg, blockingDevBuild, "")
		srv, stop, err := startParallelDevServer(cfg.FrontendDir(), "exec sleep 60", io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		defer stop()
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		startup := watchFrontend(t.Context(), srv, timer)
		defer startup.close()
		out, err := devBinary(cfg)
		if err != nil {
			t.Fatal(err)
		}
		s := &devSession{cfg: cfg, out: out, startup: startup}
		done := make(chan error, 1)
		go func() {
			defer close(done)
			done <- s.loop(t.Context())
		}()
		defer func() {
			startup.close()
			// A failed assertion can return before the loop stops.
			for range done {
			}
		}()
		buildChild := waitDevMarker(t, cfg, "build-child")
		timer.Reset(0)
		if err := waitDevResult(t, done); err != nil && !errors.Is(err, errDevURLTimeout) {
			t.Fatalf("dev loop = %v", err)
		}
		if err := startup.failure(); !errors.Is(err, errDevURLTimeout) {
			t.Fatalf("startup failure = %v, want missing frontend URL timeout", err)
		}
		assertGone(t, buildChild)
		assertNoDevLaunch(t, cfg)
	})

	t.Run("user cancellation", func(t *testing.T) {
		cfg := parallelDevFixture(t, true)
		cfg.Frontend.Dev = `sleep 60 & echo $! > frontend-child; wait`
		writeDevGo(t, cfg, blockingDevBuild, "")
		cancel, done := launchDevFixture(t, cfg, "", devModes{})
		defer cancel()
		frontChild := waitDevMarker(t, cfg, "frontend-child")
		buildChild := waitDevMarker(t, cfg, "build-child")
		cancel()
		if err := waitDevResult(t, done); err != nil {
			t.Fatal(err)
		}
		assertGone(t, frontChild)
		assertGone(t, buildChild)
		assertNoDevLaunch(t, cfg)
	})
}

func TestDevParallelServerStopWithBlockedOutput(t *testing.T) {
	out := &blockedDevOutput{writing: make(chan struct{}), unblock: make(chan struct{})}
	defer close(out.unblock)
	srv, stop, err := startParallelDevServer(t.TempDir(), "echo http://localhost:4321/; exec sleep 60", out)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	startup := watchFrontend(ctx, srv, time.NewTimer(time.Minute))
	defer startup.close()
	stopped := make(chan struct{})
	defer func() {
		go func() {
			stop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Error("front-end cleanup waited for the blocked log writer")
		}
	}()
	select {
	case <-out.writing:
	case <-ctx.Done():
		t.Fatal("frontend did not reach the blocked log writer")
	}
	if url, err := startup.waitFrontend(ctx); err != nil || url != fixtureDevURL {
		t.Fatalf("waitFrontend while log output is blocked = %q, %v", url, err)
	}
}

type blockedDevOutput struct {
	once    sync.Once
	writing chan struct{}
	unblock chan struct{}
}

func (w *blockedDevOutput) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.writing) })
	<-w.unblock
	return len(b), nil
}

func TestDevParallelBuildRetry(t *testing.T) {
	for _, exit := range []bool{false, true} {
		t.Run(fmt.Sprintf("frontend exits=%t", exit), func(t *testing.T) {
			cfg := parallelDevFixture(t, true)
			cfg.Frontend.Dev = `echo http://localhost:4321/
while [ ! -e exit-frontend ]; do sleep .02; done
exit 24`
			writeDevGo(t, cfg, `if [ ! -e edited.go ]; then
    printf failed > first-build-failed
    exit 41
fi
`, "")
			cancel, done := launchDevFixture(t, cfg, "", devModes{})
			defer cancel()
			waitDevMarker(t, cfg, "first-build-failed")
			assertNoDevLaunch(t, cfg)
			if exit {
				writeDevFixtureFile(t, cfg, "exit-frontend", "", 0o644)
				if err := waitDevResult(t, done); err == nil || !strings.Contains(err.Error(), "24") {
					t.Fatalf("dev = %v, want frontend exit while waiting for a Go edit", err)
				}
				assertNoDevLaunch(t, cfg)
				return
			}
			writeDevFixtureFile(t, cfg, "edited.go", "package main", 0o644)
			if err := waitDevResult(t, done); err != nil {
				t.Fatal(err)
			}
			assertDevLaunch(t, cfg, fixtureDevURL)
		})
	}
}

func TestDevParallelFailedBuildReapsChildrenBeforeRetry(t *testing.T) {
	for _, hot := range []bool{false, true} {
		t.Run(fmt.Sprintf("hot=%t", hot), func(t *testing.T) {
			cfg := parallelDevFixture(t, true)
			cfg.Frontend.Dev = "echo http://localhost:4321/; exec sleep 60"
			writeDevGo(t, cfg, `if [ ! -e edited.go ]; then
    sleep 60 &
    printf '%s\n' "$!" > orphan
    exit 41
fi
`, "")
			_, done := launchDevFixture(t, cfg, "", devModes{hot: hot})
			pid := waitDevMarker(t, cfg, "orphan")
			defer killDevFixturePID(pid)
			assertGone(t, pid)
			assertNoDevLaunch(t, cfg)
			writeDevFixtureFile(t, cfg, "edited.go", "package main", 0o644)
			if err := waitDevResult(t, done); err != nil {
				t.Fatal(err)
			}
			assertDevLaunch(t, cfg, fixtureDevURL)
		})
	}
}

func killDevFixturePID(pid string) {
	if n, err := strconv.Atoi(pid); err == nil {
		_ = syscall.Kill(n, syscall.SIGKILL)
	}
}

func TestDevParallelFallbackAndExternalURL(t *testing.T) {
	t.Run("non-hot fallback", func(t *testing.T) {
		cfg := parallelDevFixture(t, true)
		cfg.Frontend.Dev = `printf "server\n" >> server-starts
while [ ! -e building ]; do sleep .02; done
echo http://localhost:4321/
exec sleep 60`
		writeDevGo(t, cfg, `printf "build\n" >> build-count
`, "")
		_, done := launchDevFixture(t, cfg, "", devModes{hot: true})
		if err := waitDevResult(t, done); err != nil {
			t.Fatal(err)
		}
		assertDevLaunch(t, cfg, fixtureDevURL)
		for file, want := range map[string]string{"server-starts": "server", "build-count": "build\nbuild"} {
			if got := waitDevMarker(t, cfg, file); got != want {
				t.Errorf("%s = %q, want %q", file, got, want)
			}
		}
	})
	for _, flag := range []bool{false, true} {
		t.Run(fmt.Sprintf("external flag=%t", flag), func(t *testing.T) {
			cfg := parallelDevFixture(t, true)
			cfg.Frontend.Dev = "touch unexpected-frontend; exit 23"
			const external = "http://localhost:4399/"
			cfg.Frontend.DevURL = external
			var url string
			if flag {
				cfg.Frontend.DevURL = fixtureDevURL
				url = external
			}
			_, done := launchDevFixture(t, cfg, url, devModes{})
			if err := waitDevResult(t, done); err != nil {
				t.Fatal(err)
			}
			assertDevLaunch(t, cfg, external)
			if _, err := os.Stat(filepath.Join(cfg.dir, "unexpected-frontend")); !os.IsNotExist(err) {
				t.Fatalf("frontend ran with an external URL: %v", err)
			}
		})
	}
}

// The fake compiler can block or fail without compiling code or opening a
// window. Its executable records the URL supplied by tan dev.
func writeDevGo(t *testing.T, cfg *Config, before, app string) {
	t.Helper()
	if app == "" {
		app = `printf '%s\n' "$TANGO_DEV_URL" > launched`
	}
	writeDevFixtureFile(t, cfg, "bin/go", `#!/bin/sh
set -eu
while [ "$1" != "-o" ]; do shift; done
out=$2
touch building
`+before+`
mkdir -p "$(dirname "$out")"
cat > "$out" <<'APP'
#!/bin/sh
`+app+`
APP
chmod +x "$out"
`, 0o755)
}

const blockingDevBuild = `sleep 60 &
printf '%s\n' "$!" > build-child
wait
`

func parallelDevFixture(t *testing.T, parallel bool) *Config {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	// os.UserCacheDir on Darwin derives its path from this variable.
	t.Setenv("HOME", dir)
	cfg := &Config{Name: "Parallel Dev Fixture", ID: "com.example.dev", Version: "1.0.0", dir: dir,
		Frontend: Frontend{Dir: ".", Dist: ".", ParallelDevBuild: parallel}}
	writeDevFixtureFile(t, cfg, "bin/codesign", "#!/bin/sh\nexit 0\n", 0o755)
	writeDevFixtureFile(t, cfg, "main.go", "package main", 0o644)
	writeDevGo(t, cfg, "", "")
	// A previous executable is already present. Failed startup must never
	// run it even though devBinary always returns the same path.
	out, err := devBinary(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeDevFixtureFile(t, cfg, strings.TrimPrefix(out, dir+string(os.PathSeparator)),
		"#!/bin/sh\necho stale > launched\n", 0o755)
	return cfg
}

func writeDevFixtureFile(t *testing.T, cfg *Config, name, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(cfg.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func launchDevFixture(t *testing.T, cfg *Config, url string, modes devModes) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	done := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		defer close(done)
		result <- dev(ctx, cfg, url, nil, modes)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("dev fixture did not stop")
		}
	})
	return cancel, result
}

func waitDevMarker(t *testing.T, cfg *Config, name string) string {
	t.Helper()
	var marker string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		data, err := os.ReadFile(filepath.Join(cfg.dir, name))
		marker = strings.TrimSpace(string(data))
		if err == nil && marker != "" {
			return marker
		}
		select {
		case <-ctx.Done():
			t.Fatalf("no signal in %s: %v", name, err)
		case <-tick.C:
		}
	}
}

func waitDevResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("dev did not return")
		return nil
	}
}

func assertDevLaunch(t *testing.T, cfg *Config, url string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(cfg.dir, "launched"))
	if err != nil || strings.TrimSpace(string(got)) != url {
		t.Fatalf("app launch recorded %q (%v), want %q", got, err, url)
	}
}

func assertNoDevLaunch(t *testing.T, cfg *Config) {
	t.Helper()
	if got, err := os.ReadFile(filepath.Join(cfg.dir, "launched")); !os.IsNotExist(err) {
		t.Fatalf("app unexpectedly launched (%q) or could not be checked: %v", got, err)
	}
}
