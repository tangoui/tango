package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type checkStatus string

const (
	statusOK      checkStatus = "ok"
	statusMissing checkStatus = "missing"
	statusWarn    checkStatus = "warn"
	statusSkip    checkStatus = "-" // optional and absent
)

// check is one row of tan doctor.
type check struct {
	name     string
	status   checkStatus
	detail   string
	required bool // for this platform's desktop builds
}

func runDoctor(args []string) error {
	fs := newFlags("doctor", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := noArgs(fs); err != nil {
		return err
	}
	checks := doctorChecks()
	printChecks(os.Stdout, checks)
	for _, c := range checks {
		if c.required && c.status == statusMissing {
			return fmt.Errorf("%s is required", c.name)
		}
	}
	// Project problems (statusError) would break or spoil a build: fail
	// so CI catches them; warnings only print.
	var bad []string
	for _, c := range checks {
		if c.status == statusError {
			bad = append(bad, c.name)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("project errors: %s", strings.Join(bad, ", "))
	}
	return nil
}

func printChecks(w io.Writer, checks []check) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, c := range checks {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.name, c.status, c.detail)
	}
	tw.Flush()
}

func doctorChecks() []check {
	cs := []check{
		{name: "OS", status: statusOK, detail: runtime.GOOS + "/" + runtime.GOARCH},
		toolCheck("Go", true, "go", "version"),
		toolCheck("Node.js", false, "node", "--version"),
		toolCheck("npm", false, "npm", "--version"),
	}
	switch runtime.GOOS {
	case "darwin":
		cs = append(cs, xcodeCLTCheck(), toolCheck("hdiutil (dmg)", false, "hdiutil"))
		cs = append(cs, iosChecks()...)
	case "linux":
		cs = append(cs, webkitCheck(), toolCheck("appimagetool", false, "appimagetool", "--version"))
	case "windows":
		cs = append(cs, webview2Check())
	}
	cs = append(cs, androidChecks()...)
	cs = append(cs, toolCheck("NSIS (makensis)", false, "makensis", "-VERSION"))
	cs = append(cs, iconChecks(".")...)    // tan.json's icons, in a project (doctor_icons.go)
	cs = append(cs, appLinksCheck(".")...) // "appLinks" vs the iOS provisioning profile (applinks.go)
	return cs
}

// toolCheck runs name args and reports the first output line; with no
// args it only looks name up.
func toolCheck(label string, required bool, name string, args ...string) check {
	c := check{name: label, required: required}
	path, err := exec.LookPath(name)
	if err != nil {
		c.status, c.detail = statusMissing, name+" not on PATH"
		if !required {
			c.status = statusSkip
		}
		return c
	}
	if len(args) == 0 {
		c.status, c.detail = statusOK, path
		return c
	}
	out, err := commandOutput(path, args...)
	c.status, c.detail = statusOK, firstLine(out)
	if err != nil {
		c.status, c.detail = statusWarn, err.Error()
	}
	if c.detail == "" {
		c.detail = path
	}
	return c
}

func commandOutput(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

func xcodeCLTCheck() check {
	c := check{name: "Xcode command line tools", required: true}
	out, err := commandOutput("xcode-select", "-p")
	if err != nil {
		c.status, c.detail = statusMissing, "install with: xcode-select --install"
		return c
	}
	c.status, c.detail = statusOK, out
	return c
}

func iosSDKCheck() check {
	c := check{name: "iOS SDK (mobile)"}
	out, err := commandOutput("xcrun", "--sdk", "iphoneos", "--show-sdk-version")
	if err != nil {
		c.status, c.detail = statusSkip, "needs Xcode (not just the command line tools)"
		return c
	}
	c.status, c.detail = statusOK, "iphoneos "+firstLine(out)
	return c
}

// libDirs are where distributions put shared libraries.
var libDirs = []string{
	"/usr/lib", "/usr/lib64", "/usr/local/lib",
	"/usr/lib/x86_64-linux-gnu", "/usr/lib/aarch64-linux-gnu",
	"/lib", "/lib64", "/lib/x86_64-linux-gnu", "/lib/aarch64-linux-gnu",
}

// findLib looks for a shared library by soname prefix in dirs.
func findLib(dirs []string, prefix string) string {
	for _, d := range dirs {
		matches, _ := filepath.Glob(filepath.Join(d, prefix+"*"))
		if len(matches) > 0 {
			slices.Sort(matches)
			return matches[0]
		}
	}
	return ""
}

func webkitCheck() check {
	c := check{name: "WebKitGTK 4.1", required: true}
	if p := findLib(libDirs, "libwebkit2gtk-4.1.so"); p != "" {
		c.status, c.detail = statusOK, p
		return c
	}
	c.status = statusMissing
	c.detail = "install libwebkit2gtk-4.1-0 (Debian/Ubuntu) or webkit2gtk4.1 (Fedora)"
	return c
}

// webview2Check looks up the Evergreen runtime's registration, machine-wide
// (64- and 32-bit views) or per user.
func webview2Check() check {
	const client = `\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	c := check{name: "WebView2 runtime", required: true}
	for _, key := range []string{
		`HKLM\SOFTWARE\WOW6432Node` + client,
		`HKLM\SOFTWARE` + client,
		`HKCU\SOFTWARE` + client,
	} {
		out, err := commandOutput("reg", "query", key, "/v", "pv")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) == 3 && f[0] == "pv" && f[2] != "0.0.0.0" {
				c.status, c.detail = statusOK, f[2]
				return c
			}
		}
	}
	c.status, c.detail = statusMissing, "install from https://developer.microsoft.com/microsoft-edge/webview2/"
	return c
}

func androidChecks() []check {
	sdk := findAndroidSDK()
	sdkCheck := check{name: "Android SDK (mobile)", status: statusSkip, detail: "set ANDROID_HOME"}
	ndkCheck := check{name: "Android NDK (mobile)", status: statusSkip, detail: "set ANDROID_NDK_HOME or install under $ANDROID_HOME/ndk"}
	if sdk == "" {
		return []check{sdkCheck, ndkCheck}
	}
	sdkCheck.status, sdkCheck.detail = statusOK, sdk
	if ndk := findAndroidNDK(sdk); ndk != "" {
		ndkCheck.status, ndkCheck.detail = statusOK, ndk
	}
	return append([]check{sdkCheck, ndkCheck}, androidSDKChecks(sdk)...)
}

// androidSDKChecks looks at the rest of what tan build and tan run need
// for Android: a JDK for Gradle, the compile platform, build tools, adb,
// and an emulator with at least one AVD (or a connected device).
func androidSDKChecks(sdk string) []check {
	cs := []check{javaCheck()}
	platform := check{name: fmt.Sprintf("Android platform %d (mobile)", defaultAndroidSdk), status: statusOK}
	if _, err := os.Stat(filepath.Join(sdk, "platforms", fmt.Sprintf("android-%d", defaultAndroidSdk))); err != nil {
		platform.status, platform.detail = statusWarn, fmt.Sprintf(`install with: sdkmanager "platforms;android-%d"`, defaultAndroidSdk)
	} else {
		platform.detail = filepath.Join(sdk, "platforms", fmt.Sprintf("android-%d", defaultAndroidSdk))
	}
	bt := check{name: "Android build tools (mobile)", status: statusWarn, detail: `install with: sdkmanager "build-tools;36.0.0"`}
	if vs, _ := filepath.Glob(filepath.Join(sdk, "build-tools", "*")); len(vs) > 0 {
		slices.SortFunc(vs, func(a, b string) int { return compareVersions(filepath.Base(a), filepath.Base(b)) })
		bt.status, bt.detail = statusOK, filepath.Base(vs[len(vs)-1])
	}
	tools := androidToolchain{sdk: sdk}
	adbCheck := check{name: "adb (mobile)", status: statusWarn, detail: `install with: sdkmanager platform-tools`}
	if _, err := os.Stat(tools.adb()); err == nil {
		adbCheck.status, adbCheck.detail = statusOK, tools.adb()
		if out, err := commandOutput(tools.adb(), "devices"); err == nil {
			var ready []string
			for _, d := range parseADBDevices(out) {
				if d.state == "device" {
					ready = append(ready, d.serial)
				}
			}
			if len(ready) > 0 {
				adbCheck.detail += " (connected: " + strings.Join(ready, ", ") + ")"
			}
		}
	}
	emu := check{name: "Android emulator (mobile)", status: statusSkip, detail: `optional; install with: sdkmanager emulator "system-images;android-36;google_apis;arm64-v8a"`}
	if _, err := os.Stat(tools.emulator()); err == nil {
		emu.status, emu.detail = statusOK, tools.emulator()
		out, _ := commandOutput(tools.emulator(), "-list-avds")
		var avds []string
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.Contains(line, " ") {
				avds = append(avds, line)
			}
		}
		if len(avds) == 0 {
			emu.status, emu.detail = statusWarn, "no AVD; create one with: avdmanager create avd -n tango -k \"system-images;android-36;google_apis;arm64-v8a\""
		} else {
			emu.detail = "AVDs: " + strings.Join(avds, ", ")
		}
	}
	return append(cs, platform, bt, adbCheck, emu)
}

// javaCheck finds the JDK Gradle will use and checks it is 17 or newer.
func javaCheck() check {
	c := check{name: "JDK 17+ (Android)"}
	java := "java"
	if home := os.Getenv("JAVA_HOME"); home != "" {
		java = filepath.Join(home, "bin", exeName("java"))
	} else if _, err := exec.LookPath("java"); err != nil {
		c.status, c.detail = statusSkip, "set JAVA_HOME to a JDK 17+ (Gradle needs it)"
		return c
	}
	out, err := commandOutput(java, "-version")
	if err != nil {
		c.status, c.detail = statusWarn, firstNonEmpty(firstLine(out), err.Error())
		return c
	}
	line := firstLine(out) // openjdk version "17.0.16" 2025-07-15
	c.status, c.detail = statusOK, line
	if _, v, ok := strings.Cut(line, `"`); ok {
		major, _ := strconv.Atoi(strings.Split(strings.TrimPrefix(v, "1."), ".")[0])
		if major > 0 && major < 17 {
			c.status, c.detail = statusWarn, line+": Android Gradle plugin needs 17 or newer"
		}
	}
	return c
}

// iosChecks reports Xcode, the iOS SDKs and Simulator runtimes.
func iosChecks() []check {
	xcode := check{name: "Xcode (iOS)"}
	out, err := commandOutput("xcodebuild", "-version")
	if err != nil {
		xcode.status, xcode.detail = statusSkip, "install Xcode from the App Store, then: sudo xcode-select -s /Applications/Xcode.app"
		return []check{xcode}
	}
	xcode.status, xcode.detail = statusOK, strings.ReplaceAll(out, "\n", ", ")
	sim := check{name: "iOS Simulator runtimes", status: statusWarn, detail: "none; install with: xcodebuild -downloadPlatform iOS"}
	if out, err := commandOutput("xcrun", "simctl", "list", "runtimes"); err == nil {
		var rts []string
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "iOS ") {
				rts = append(rts, strings.Fields(line)[1])
			}
		}
		if len(rts) > 0 {
			sim.status, sim.detail = statusOK, "iOS "+strings.Join(rts, ", ")
		}
	}
	ident := check{name: "iOS signing identities", status: statusSkip, detail: "none (needed for device builds only)"}
	if out, err := commandOutput("security", "find-identity", "-v", "-p", "codesigning"); err == nil {
		n := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "Apple Development") || strings.Contains(line, "Apple Distribution") || strings.Contains(line, "iPhone") {
				n++
			}
		}
		if n > 0 {
			ident.status, ident.detail = statusOK, fmt.Sprintf("%d iOS identities (security find-identity -v -p codesigning)", n)
		}
	}
	return []check{xcode, iosSDKCheck(), sim, ident}
}
