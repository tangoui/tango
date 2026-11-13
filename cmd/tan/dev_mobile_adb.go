package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// androidDev deploys to an Android device or emulator through adb.
type androidDev struct {
	cfg     *Config
	run     devRunner
	adb     string
	arch    string                                    // GOARCH to build; from the device unless -platform android/<arch>
	pick    func(ctx context.Context) (string, error) // the serial: run.go's androidDevice (may start an emulator)
	buildFn func(ctx context.Context, arches []string) (string, error)

	serial    string
	port      string // adb reverse tcp:port, or ""
	component string // the launcher Activity
	logs      bgProcess
}

func (a *androidDev) describe() string { return "android " + a.serial }

func (a *androidDev) adbArgs(args ...string) []string {
	return append([]string{"-s", a.serial}, args...)
}

func (a *androidDev) prepare(ctx context.Context, url string) error {
	serial, err := a.pick(ctx)
	if err != nil {
		return err
	}
	a.serial = serial
	if a.arch == "" {
		// Only the device's ABI is built: half the work of a full APK.
		out, err := a.run.output(ctx, a.adb, a.adbArgs("shell", "getprop", "ro.product.cpu.abilist")...)
		if err != nil {
			return err
		}
		if a.arch, err = pickABI(out); err != nil {
			return err
		}
	}
	if port := loopbackPort(url); port != "" {
		// The app reaches the dev server at the same localhost URL.
		if err := a.run.run(ctx, a.adb, a.adbArgs("reverse", "tcp:"+port, "tcp:"+port)...); err != nil {
			return err
		}
		a.port = port
	} else if url != "" {
		warnf("%s is not on this machine's localhost: asset.FromEnv only proxies to loopback addresses, so the app will not use it (call asset.DevProxy with AllowRemote yourself if you must)", url)
	}
	return nil
}

func (a *androidDev) build(ctx context.Context) (string, error) {
	return a.buildFn(ctx, []string{a.arch})
}

func (a *androidDev) install(ctx context.Context, apk string) error {
	// -r replaces the app and keeps its data; -t allows debug builds.
	err := a.run.run(ctx, a.adb, a.adbArgs("install", "-r", "-t", apk)...)
	if err != nil && strings.Contains(err.Error(), "INSTALL_FAILED_UPDATE_INCOMPATIBLE") {
		return fmt.Errorf("%w\n(the installed %s is signed with another key, a release build probably; tan dev keeps its data and does not uninstall it: %s -s %s uninstall %s)", err, a.cfg.ID, a.adb, a.serial, a.cfg.ID)
	}
	return err
}

func (a *androidDev) launch(ctx context.Context, url string) error {
	a.stopLogs()
	if a.component == "" {
		a.component = a.cfg.ID + "/" + hostActivity
		// A project may declare its own launcher Activity.
		if out, err := a.run.output(ctx, a.adb, a.adbArgs("shell", "cmd", "package", "resolve-activity", "--brief", "-c", "android.intent.category.LAUNCHER", a.cfg.ID)...); err == nil {
			if c := lastLine(out); strings.HasPrefix(c, a.cfg.ID+"/") {
				a.component = c
			}
		}
	}
	// adb shell runs one command line on the device: quote it. -S stops a
	// running instance first, -W waits until the Activity is up.
	line := "am start -S -W -n " + shQuote(a.component)
	if url != "" {
		line += " -e TANGO_DEV_URL " + shQuote(url)
	}
	if err := a.run.run(ctx, a.adb, a.adbArgs("shell", line)...); err != nil {
		return err
	}
	pid, err := a.pid(ctx)
	if err != nil {
		return err
	}
	logs, err := a.run.start(nil, os.Stdout, a.adb, a.adbArgs("logcat", "-v", "brief", "--pid="+pid)...)
	if err != nil {
		return err
	}
	a.logs = logs
	return nil
}

// pid waits for the app's process.
func (a *androidDev) pid(ctx context.Context) (string, error) {
	for i := 0; ; i++ {
		out, err := a.run.output(ctx, a.adb, a.adbArgs("shell", "pidof", "-s", a.cfg.ID)...)
		if pid := strings.TrimSpace(out); err == nil && pid != "" {
			return pid, nil
		}
		if i == 20 || ctx.Err() != nil {
			return "", fmt.Errorf("%s did not start on %s (see %s -s %s logcat)", a.cfg.ID, a.serial, a.adb, a.serial)
		}
		select {
		case <-ctx.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// exited: logcat --pid does not end with the app, so unknown.
func (a *androidDev) exited() <-chan struct{} { return nil }

func (a *androidDev) stopLogs() {
	if a.logs != nil {
		a.logs.stop()
		a.logs = nil
	}
}

func (a *androidDev) stop(ctx context.Context) {
	a.stopLogs()
	if a.serial != "" {
		_ = a.run.run(ctx, a.adb, a.adbArgs("shell", "am", "force-stop", a.cfg.ID)...)
	}
}

func (a *androidDev) cleanup(ctx context.Context) {
	if a.port != "" {
		_ = a.run.run(ctx, a.adb, a.adbArgs("reverse", "--remove", "tcp:"+a.port)...)
		a.port = ""
	}
}

// shQuote quotes s for the device's sh.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
