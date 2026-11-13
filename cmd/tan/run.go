package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func runRun(args []string) error {
	fs := newFlags("run", "-platform ios-simulator|android [flags]")
	dir := projectFlag(fs)
	platform := fs.String("platform", "", "`platform` to run on: ios-simulator or android")
	device := fs.String("device", "", "Simulator name or UDID (ios-simulator), or adb serial (android); default: the booted one")
	avd := fs.String("avd", "", "android: start this emulator (AVD name) when no device is connected (default: the first AVD)")
	out := fs.String("o", "build", "output `dir`")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := LoadConfig(*dir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch *platform {
	case osIOSSimulator:
		return runIOSSimulator(ctx, cfg, *device, *out)
	case osAndroid:
		return runAndroid(ctx, cfg, *device, *avd, *out)
	case osIOS:
		return errors.New("tan run installs on the Simulator: use -platform ios-simulator (for a device, open the project from tan build -platform ios -xcode in Xcode)")
	default:
		return fmt.Errorf("-platform must be ios-simulator or android, not %q (desktop apps: tan dev)", *platform)
	}
}

// simDevice is one entry of simctl list devices -j.
type simDevice struct {
	UDID        string `json:"udid"`
	Name        string `json:"name"`
	State       string `json:"state"`
	IsAvailable bool   `json:"isAvailable"`
	runtime     string
}

func simDevices(ctx context.Context) ([]simDevice, error) {
	out, err := exec.CommandContext(ctx, "xcrun", "simctl", "list", "devices", "available", "-j").Output()
	if err != nil {
		return nil, fmt.Errorf("simctl: %v (is Xcode installed?)", err)
	}
	var list struct {
		Devices map[string][]simDevice `json:"devices"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, err
	}
	var ds []simDevice
	for rt, devs := range list.Devices {
		if !strings.Contains(rt, "SimRuntime.iOS-") {
			continue
		}
		for _, d := range devs {
			d.runtime = rt
			ds = append(ds, d)
		}
	}
	return ds, nil
}

// pickSimulator chooses the device to run on: the one asked for, a booted
// iOS device, or the newest-runtime iPhone. Pure, for tests.
func pickSimulator(ds []simDevice, want string) (simDevice, error) {
	if want != "" {
		for _, d := range ds {
			if d.UDID == want || d.Name == want {
				return d, nil
			}
		}
		return simDevice{}, fmt.Errorf("no available iOS Simulator named %q (list them with: xcrun simctl list devices)", want)
	}
	for _, d := range ds {
		if d.State == "Booted" {
			return d, nil
		}
	}
	var best *simDevice
	for i, d := range ds {
		if !strings.HasPrefix(d.Name, "iPhone") {
			continue
		}
		if best == nil || compareRuntimes(d.runtime, best.runtime) > 0 ||
			d.runtime == best.runtime && strings.Contains(d.Name, "Pro") && !strings.Contains(best.Name, "Pro") {
			best = &ds[i]
		}
	}
	if best == nil {
		return simDevice{}, errors.New("no iPhone Simulator: add one in Xcode (Window › Devices and Simulators) or install an iOS runtime")
	}
	return *best, nil
}

// compareRuntimes orders "com.apple.CoreSimulator.SimRuntime.iOS-26-1".
func compareRuntimes(a, b string) int {
	// Device-specific runtimes ("iOS-27-2-iPhone19-4") rank after the
	// general one of the same version.
	v := func(s string) (string, bool) {
		_, ver, _ := strings.Cut(s, "SimRuntime.iOS-")
		var nums []string
		for _, p := range strings.Split(ver, "-") {
			if _, err := strconv.Atoi(p); err != nil {
				return strings.Join(nums, "."), true
			}
			nums = append(nums, p)
		}
		return strings.Join(nums, "."), false
	}
	va, sa := v(a)
	vb, sb := v(b)
	if c := compareVersions(va, vb); c != 0 || sa == sb {
		return c
	}
	if sa {
		return -1
	}
	return 1
}

// runIOSSimulator builds for the Simulator, boots one if needed, installs
// and launches the app with its stdout/stderr on this terminal.
func runIOSSimulator(ctx context.Context, cfg *Config, want, out string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("the iOS Simulator needs macOS with Xcode")
	}
	ds, err := simDevices(ctx)
	if err != nil {
		return err
	}
	dev, err := pickSimulator(ds, want)
	if err != nil {
		return err
	}
	arts, err := build(ctx, cfg, buildOptions{targets: []target{{osIOSSimulator, "arm64"}}, out: out, debug: true})
	if err != nil {
		return err
	}
	if dev.State != "Booted" {
		logf("Booting %s", dev.Name)
		if err := runIn(ctx, "", nil, "xcrun", "simctl", "boot", dev.UDID); err != nil {
			return err
		}
	}
	// Show the Simulator window (simctl boot alone runs it headless).
	_ = exec.Command("open", "-g", "-a", "Simulator", "--args", "-CurrentDeviceUDID", dev.UDID).Run()
	if err := runIn(ctx, "", nil, "xcrun", "simctl", "bootstatus", dev.UDID, "-b"); err != nil {
		return err
	}
	logf("Installing on %s", dev.Name)
	if err := runIn(ctx, "", nil, "xcrun", "simctl", "install", dev.UDID, arts[0].path); err != nil {
		return err
	}
	logf("Launching %s (Ctrl-C stops it)", cfg.ID)
	// --console-pty connects the app's stdout and stderr (Go's log
	// output) to this terminal; simctl returns when the app exits.
	cmd := exec.Command("xcrun", "simctl", "launch", "--console-pty", "--terminate-running-process", dev.UDID, cfg.ID)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = exec.Command("xcrun", "simctl", "terminate", dev.UDID, cfg.ID).Run()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
		}
		return nil
	}
}

// adbDevice is a line of adb devices.
type adbDevice struct{ serial, state string }

func parseADBDevices(out string) []adbDevice {
	var ds []adbDevice
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] != "List" && !strings.HasPrefix(f[0], "*") {
			ds = append(ds, adbDevice{f[0], f[1]})
		}
	}
	return ds
}

// pickABI returns the first of the device's ABIs (ro.product.cpu.abilist,
// most preferred first) that Tango builds, as a GOARCH.
func pickABI(abilist string) (string, error) {
	for _, abi := range strings.Split(strings.TrimSpace(abilist), ",") {
		if goarch, ok := parseAndroidArch(strings.TrimSpace(abi)); ok {
			return goarch, nil
		}
	}
	return "", fmt.Errorf("device ABIs %q: none supported", abilist)
}

type adb struct {
	path, serial string
}

func (a adb) cmd(ctx context.Context, args ...string) *exec.Cmd {
	if a.serial != "" {
		args = append([]string{"-s", a.serial}, args...)
	}
	return exec.CommandContext(ctx, a.path, args...)
}

func (a adb) output(ctx context.Context, args ...string) (string, error) {
	out, err := a.cmd(ctx, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("adb %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// runAndroid builds a debug APK for the device's ABI, installs it, starts
// the app and shows its log (adb logcat for the app's process) until the
// app exits or Ctrl-C.
func runAndroid(ctx context.Context, cfg *Config, want, avd, out string) error {
	tools, err := androidTools()
	if err != nil {
		return err
	}
	if _, err := os.Stat(tools.adb()); err != nil {
		return fmt.Errorf("adb not found at %s: install the SDK's platform-tools", tools.adb())
	}
	a := adb{path: tools.adb()}
	serial, err := androidDevice(ctx, tools, a, want, avd)
	if err != nil {
		return err
	}
	a.serial = serial
	abis, err := a.output(ctx, "shell", "getprop", "ro.product.cpu.abilist")
	if err != nil {
		return err
	}
	arch, err := pickABI(abis)
	if err != nil {
		return err
	}
	arts, err := build(ctx, cfg, buildOptions{targets: []target{{osAndroid, arch}}, out: out, debug: true})
	if err != nil {
		return err
	}
	apk := arts[0].path
	logf("Installing on %s", serial)
	if msg, err := a.output(ctx, "install", "-r", "-t", apk); err != nil {
		if !strings.Contains(msg+err.Error(), "INSTALL_FAILED_UPDATE_INCOMPATIBLE") {
			return err
		}
		// Signed with another key (e.g. a release build): reinstall.
		warnf("Uninstalling %s, which was signed with a different key", cfg.ID)
		a.output(ctx, "uninstall", cfg.ID)
		if _, err := a.output(ctx, "install", "-t", apk); err != nil {
			return err
		}
	}
	a.output(ctx, "shell", "am", "force-stop", cfg.ID)
	if _, err := a.output(ctx, "shell", "am", "start", "-W", "-n", cfg.ID+"/"+hostActivity); err != nil {
		return err
	}
	pid := ""
	for range 50 {
		if pid, _ = a.output(ctx, "shell", "pidof", cfg.ID); pid != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if pid == "" {
		return fmt.Errorf("%s did not start; see adb -s %s logcat", cfg.ID, serial)
	}
	logf("Running %s (pid %s); showing its log (Ctrl-C stops showing it)", cfg.ID, pid)
	logcat := a.cmd(ctx, "logcat", "-v", "brief", "--pid="+pid)
	logcat.Stdout, logcat.Stderr = os.Stdout, os.Stderr
	if err := logcat.Start(); err != nil {
		return err
	}
	defer logcat.Process.Kill()
	// Stop when the app's process is gone.
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			now, err := a.output(ctx, "shell", "pidof", cfg.ID)
			if ctx.Err() != nil {
				return nil
			}
			if err == nil && now != pid || err != nil && strings.Contains(err.Error(), "exit status 1") {
				time.Sleep(500 * time.Millisecond) // let logcat print the last lines
				logf("%s exited", cfg.ID)
				return nil
			}
		}
	}
}

// androidDevice returns the serial to run on: the one asked for, the only
// connected device, or an emulator it starts.
func androidDevice(ctx context.Context, tools androidToolchain, a adb, want, avd string) (string, error) {
	out, err := a.output(ctx, "devices")
	if err != nil {
		return "", err
	}
	var ready []string
	for _, d := range parseADBDevices(out) {
		if d.state == "device" {
			ready = append(ready, d.serial)
		} else if d.serial == want || want == "" {
			warnf("%s is %s", d.serial, d.state)
		}
	}
	switch {
	case want != "":
		if !slices.Contains(ready, want) {
			return "", fmt.Errorf("device %s is not connected (adb devices)", want)
		}
		return want, nil
	case len(ready) == 1:
		return ready[0], nil
	case len(ready) > 1:
		return "", fmt.Errorf("several devices are connected (%s); pick one with -device", strings.Join(ready, ", "))
	}
	return startEmulator(ctx, tools, a, avd)
}

// startEmulator starts an AVD in the background and waits until Android
// has booted.
func startEmulator(ctx context.Context, tools androidToolchain, a adb, avd string) (string, error) {
	emu := tools.emulator()
	if _, err := os.Stat(emu); err != nil {
		return "", errors.New("no device connected and no emulator installed: connect a phone with USB debugging, or install the emulator and a system image (see tan doctor)")
	}
	if avd == "" {
		out, _ := exec.CommandContext(ctx, emu, "-list-avds").Output()
		for _, line := range strings.Split(string(out), "\n") {
			// Skip informational lines such as "INFO | ...".
			if line = strings.TrimSpace(line); line != "" && !strings.Contains(line, " ") {
				avd = line
				break
			}
		}
		if avd == "" {
			return "", errors.New("no device connected and no emulator (AVD) defined: create one with avdmanager create avd, or connect a phone")
		}
	}
	logf("Starting the emulator %s", avd)
	logPath := filepath.Join(os.TempDir(), "tan-emulator-"+avd+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(emu, "-avd", avd)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	newGroup(cmd) // keeps running after tan exits
	if err := cmd.Start(); err != nil {
		return "", err
	}
	logFile.Close()
	exited := make(chan struct{})
	go func() { cmd.Wait(); close(exited) }()
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		select {
		case <-exited:
			return "", fmt.Errorf("the emulator exited; see %s", logPath)
		default:
		}
		out, _ := a.output(ctx, "devices")
		for _, d := range parseADBDevices(out) {
			if !strings.HasPrefix(d.serial, "emulator-") || d.state != "device" {
				continue
			}
			s := adb{path: a.path, serial: d.serial}
			if booted, _ := s.output(ctx, "shell", "getprop", "sys.boot_completed"); booted == "1" {
				return d.serial, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("the emulator did not finish booting in 5 minutes; see %s", logPath)
}
