package windows

// Locating the WebView2 Runtime the way WebView2Loader.dll documents it:
// either a fixed-version runtime folder given by the app, or the Evergreen
// runtime that EdgeUpdate registered for one of its release channels.

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const webview2Download = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// webview2Channels are the EdgeUpdate client IDs of the runtime's release
// channels, searched in this order (most stable first) like the loader.
var webview2Channels = []string{
	"{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}", // WebView2 Runtime (Stable / Evergreen)
	"{2CD8A007-E189-409D-A2C8-9AF4EF3C72AA}", // Edge Beta
	"{0D50BFEC-CD6A-4F9A-964C-C7416E3ACB10}", // Edge Dev
	"{65C35B14-6C1D-4122-AC46-7148CC9D6497}", // Edge Canary
}

const (
	webview2BrowserExe = "msedgewebview2.exe"
	webview2ClientDir  = "EBWebView"
	webview2ClientDLL  = "EmbeddedBrowserWebView.dll"

	// edgeUpdateClients is where each channel records its install folder,
	// in a value named like webview2ClientDir.
	edgeUpdateClients = `SOFTWARE\Microsoft\EdgeUpdate\ClientState\`
)

var errWebView2NotInstalled = errors.New("tango: the Microsoft Edge WebView2 Runtime is not installed; get it from " + webview2Download)

// registryReader returns a string value from HKLM (machine) or HKCU, or
// "" when it is absent.
type registryReader func(machine bool, path, name string) string

// findWebView2 picks the runtime folder for goarch. A non-empty fixedDir
// (WEBVIEW2_BROWSER_EXECUTABLE_FOLDER) is taken as a fixed-version runtime
// and must hold one; otherwise the channels are searched, and the first
// one with a usable registration wins. The bool reports the fixed case.
func findWebView2(fixedDir, goarch string, read func(machine bool, path, name string) string) (string, bool, error) {
	if fixedDir != "" {
		if webview2Client(fixedDir, goarch) != "" {
			return fixedDir, true, nil
		}
		return "", false, fmt.Errorf("tango: WEBVIEW2_BROWSER_EXECUTABLE_FOLDER=%s holds no WebView2 Runtime", fixedDir)
	}
	firstBroken := ""
	for _, channel := range webview2Channels {
		usable, broken := channelRuntime(channel, goarch, read)
		if usable != "" {
			return usable, false, nil
		}
		if firstBroken == "" {
			firstBroken = broken
		}
	}
	if firstBroken == "" {
		return "", false, errWebView2NotInstalled
	}
	return "", false, fmt.Errorf("tango: the WebView2 Runtime registered at %s is missing or incomplete; reinstall it from %s", firstBroken, webview2Download)
}

// channelRuntime looks at a channel's per-machine and per-user
// registrations (in that order). usable is the newest folder that holds a
// runtime for goarch; broken is the first registered folder that does not.
func channelRuntime(channel, goarch string, read registryReader) (usable, broken string) {
	var usableVersion []int
	for _, machine := range [2]bool{true, false} {
		dir := read(machine, edgeUpdateClients+channel, webview2ClientDir)
		switch {
		case dir == "":
			continue
		case webview2Client(dir, goarch) == "":
			if broken == "" {
				broken = dir
			}
			continue
		}
		// Runtime folders are named after their version.
		version := parseVersion(filepath.Base(dir))
		if usable == "" || compareVersions(version, usableVersion) > 0 {
			usable, usableVersion = dir, version
		}
	}
	return usable, broken
}

// webview2Client returns the path of the client DLL for goarch inside a
// runtime folder, or "" if dir is not a WebView2 Runtime. Edge installs
// also carry the DLL; only a runtime has msedgewebview2.exe as well.
func webview2Client(dir, goarch string) string {
	dll := filepath.Join(dir, webview2ClientDir, webview2ArchDir(goarch), webview2ClientDLL)
	if !fileExists(filepath.Join(dir, webview2BrowserExe)) || !fileExists(dll) {
		return ""
	}
	return dll
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// webview2ArchDir names the per-architecture folder under EBWebView.
func webview2ArchDir(goarch string) string {
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if dir, ok := webview2ArchDirs[goarch]; ok {
		return dir
	}
	return "x64" // amd64, and anything unexpected
}

var webview2ArchDirs = map[string]string{"arm64": "arm64", "386": "x86"}

// parseVersion splits a dotted version into its numbers; nil if any part
// is not a number.
func parseVersion(s string) []int {
	var nums []int
	for field := range strings.SplitSeq(s, ".") {
		n, err := strconv.Atoi(field)
		if err != nil {
			return nil
		}
		nums = append(nums, n)
	}
	return nums
}

// compareVersions orders two parsed versions (-1, 0, +1), a missing
// trailing part counting as 0.
func compareVersions(a, b []int) int {
	part := func(v []int, i int) int {
		if i < len(v) {
			return v[i]
		}
		return 0
	}
	for i := range max(len(a), len(b)) {
		if c := cmp.Compare(part(a, i), part(b, i)); c != 0 {
			return c
		}
	}
	return 0
}

// userDataDir is %LOCALAPPDATA%\<app>\WebView2, where app is the app ID,
// else its name, else "Tango", made safe for a path. WEBVIEW2_USER_DATA_FOLDER
// overrides it, relative to exeDir when not absolute.
func userDataDir(getenv func(string) string, exeDir, id, name string) string {
	if d := getenv("WEBVIEW2_USER_DATA_FOLDER"); d != "" {
		if !filepath.IsAbs(d) && exeDir != "" {
			d = filepath.Join(exeDir, d)
		}
		return d
	}
	base := getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	app := id
	if app == "" {
		app = name
	}
	return filepath.Join(base, sanitizeFileName(app), "WebView2")
}

func sanitizeFileName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < ' ' || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = '_'
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), " .")
	if out == "" {
		return "Tango"
	}
	return out
}
