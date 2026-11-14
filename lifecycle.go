package tango

// App lifecycle around the event loop: launch arguments, the last-window
// policy and the mobile app events.

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tangoui/tango/internal/platform"
)

// ---------------------------------------------------------------------------
// Launch arguments

// Launch arguments, deep links and opened files.
//
// On Windows and Linux the OS hands a deep link or a file to open to a new
// process as a command-line argument. The core scans os.Args once the app
// is ready and delivers what it finds as app events, so OnEvent sees the
// same thing on every desktop platform (macOS delivers them through the
// app delegate instead; AppKit's own argument handling is turned off so
// they are not reported twice). The single package feeds the arguments of
// later launches through HandleArgs.

// osArgs returns the launch arguments to scan. Tests replace it.
var osArgs = func() []string {
	if len(os.Args) < 2 {
		return nil
	}
	return os.Args[1:]
}

// dispatchLaunchArgs runs once on the main thread when the app is ready.
func (a *App) dispatchLaunchArgs() {
	switch runtime.GOOS {
	case "ios", "android":
		return
	}
	dir, _ := os.Getwd()
	for _, ev := range a.ParseArgs(osArgs(), dir) {
		a.emitAppEvent(ev)
	}
}

// ParseArgs finds deep links and files in command-line arguments: an
// argument using one of Options.URLSchemes becomes an EventOpenURL; an
// existing file or directory (relative paths resolve against dir), or a
// file: URL, joins a single EventOpenFiles. Arguments starting with "-" are
// skipped until a "--". The program name must not be included.
func (a *App) ParseArgs(args []string, dir string) []AppEvent {
	return parseArgs(args, dir, a.opts.URLSchemes)
}

// HandleArgs parses args like ParseArgs and delivers the events to OnEvent
// listeners on the main thread. Safe from any goroutine.
func (a *App) HandleArgs(args []string, dir string) {
	evs := a.ParseArgs(args, dir)
	if len(evs) == 0 {
		return
	}
	a.run(func() {
		for _, ev := range evs {
			a.emitAppEvent(ev)
		}
	})
}

func parseArgs(args []string, dir string, schemes []string) []AppEvent {
	var evs []AppEvent
	var files []string
	flags := true
	for _, arg := range args {
		switch {
		case arg == "":
			continue
		case flags && arg == "--":
			flags = false
			continue
		case flags && strings.HasPrefix(arg, "-"):
			continue
		}
		if scheme, ok := urlScheme(arg); ok {
			if scheme == "file" {
				if u, err := url.Parse(arg); err == nil && u.Path != "" {
					if p := existingPath(fileURLPath(u), ""); p != "" {
						files = append(files, p)
					}
				}
				continue
			}
			if hasScheme(schemes, scheme) {
				evs = append(evs, AppEvent{Kind: EventOpenURL, URL: arg})
				continue
			}
		}
		if p := existingPath(arg, dir); p != "" {
			files = append(files, p)
		}
	}
	if len(files) > 0 {
		evs = append(evs, AppEvent{Kind: EventOpenFiles, Paths: files})
	}
	return evs
}

// urlScheme returns the lower-cased scheme of s if s starts with one. A
// single letter followed by ':' is a Windows drive, not a scheme.
func urlScheme(s string) (string, bool) {
	i := strings.IndexByte(s, ':')
	if i < 2 {
		return "", false
	}
	for j, c := range s[:i] {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case j > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return "", false
		}
	}
	return strings.ToLower(s[:i]), true
}

// fileURLPath turns file:///C:/x into C:\x on Windows, /x elsewhere.
func fileURLPath(u *url.URL) string {
	p := u.Path
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func hasScheme(schemes []string, scheme string) bool {
	for _, s := range schemes {
		if strings.EqualFold(strings.TrimSuffix(strings.TrimSuffix(s, "://"), ":"), scheme) {
			return true
		}
	}
	return false
}

// existingPath returns the absolute, cleaned form of p if it exists.
func existingPath(p, dir string) string {
	if !filepath.IsAbs(p) {
		if dir == "" {
			return ""
		}
		p = filepath.Join(dir, p)
	}
	p = filepath.Clean(p)
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// OnOpenURL registers fn for EventOpenURL: deep links, from launch
// arguments, later launches (package single) or the OS (macOS).
func (a *App) OnOpenURL(fn func(url string)) {
	a.listen(EventOpenURL, func(ev AppEvent) { fn(ev.URL) })
}

// OnOpenFiles registers fn for EventOpenFiles: files given on the command
// line, dropped on the Dock icon, or opened with the app from the file
// manager.
func (a *App) OnOpenFiles(fn func(paths []string)) {
	a.listen(EventOpenFiles, func(ev AppEvent) { fn(ev.Paths) })
}

// ID returns Options.ID, or Options.Name when ID is empty.
func (a *App) ID() string {
	if a.opts.ID != "" {
		return a.opts.ID
	}
	return a.opts.Name
}

// ---------------------------------------------------------------------------
// Last-window policy

// Last-window policy: Options.QuitOnLastClose.

// quitOnLastClose reports the effective policy.
func (a *App) quitOnLastClose() bool {
	if a.opts.QuitOnLastClose != nil {
		return *a.opts.QuitOnLastClose
	}
	switch runtime.GOOS {
	case "darwin", "ios", "android":
		return false
	}
	return true
}

// afterSurfaceClosed runs on the main thread after a surface closed. When
// it was the last one that is not Auxiliary and the policy says so, the
// app quits (closing the auxiliary ones). Like App.Quit
// this does not ask OnQuitRequest: closing the window already went through
// Surface.OnClose, and a cancelled quit would leave a windowless app with
// no way back on Windows and Linux.
func (a *App) afterSurfaceClosed() {
	if a.state.Load() == stateClosed || !a.quitOnLastClose() {
		return
	}
	switch runtime.GOOS {
	case "ios", "android":
		return
	}
	a.mu.Lock()
	n := 0
	for _, s := range a.surfaces {
		if !s.auxiliary {
			n++
		}
	}
	a.mu.Unlock()
	if n > 0 {
		return
	}
	a.ext.mu.Lock()
	done := a.ext.quitOnClose
	a.ext.quitOnClose = true
	a.ext.mu.Unlock()
	if !done {
		a.b.Quit()
	}
}

// ---------------------------------------------------------------------------
// Mobile app events

// App events of the mobile capabilities (internal/platform/mobile_ext.go).
// Their payloads are typed AppEvent fields: Insets, Orientation, Push
// (Token / Payload) and Surface.
// The sys/... packages wrap most of them (safearea.OnChange,
// orientation.OnChange, push.OnReceive, ...); they are listed here so
// OnEvent listeners can tell them apart.
const (
	EventSafeAreaChanged    = platform.AppSafeAreaChanged
	EventOrientationChanged = platform.AppOrientationChanged
	EventPushToken          = platform.AppPushToken
	EventPushReceived       = platform.AppPushReceived
	EventPushOpened         = platform.AppPushOpened
	EventWebViewRestored    = platform.AppWebViewRestored
)

// OnWebViewRestored registers fn to run on the main thread when the system
// killed the web content process of surface s (memory pressure, a crash)
// and the backend reloaded its page from url. Page state (JS variables,
// form input) is gone: restore what the page needs once it has loaded
// again (Surface.OnLoaded fires as usual). iOS and Android only.
func (a *App) OnWebViewRestored(fn func(s *Surface, url string)) {
	a.listen(EventWebViewRestored, func(ev AppEvent) {
		if s := a.surface(ev.Surface); s != nil {
			fn(s, ev.URL)
		}
	})
}
