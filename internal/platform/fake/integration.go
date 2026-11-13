package fake

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Round-2 desktop integration of the fake backend: platform.Displays,
// platform.SystemTheme, platform.Shell and platform.AppControl. Their state
// lives beside Backend (keyed by it) so this file needs no change to the
// Backend struct.

var (
	_ platform.Displays    = (*Backend)(nil)
	_ platform.SystemTheme = (*Backend)(nil)
	_ platform.Shell       = (*Backend)(nil)
	_ platform.AppControl  = (*Backend)(nil)
	_ platform.WebViewInfo = (*Backend)(nil)
)

// DefaultDisplay is the single display a fake backend starts with.
var DefaultDisplay = platform.Display{
	ID:       "fake-1",
	Name:     "Fake Display",
	Bounds:   platform.Rect{Width: 1440, Height: 900},
	WorkArea: platform.Rect{Y: 25, Width: 1440, Height: 875},
	Scale:    2,
	Primary:  true,
}

// ShellCall records one Shell method call.
type ShellCall struct {
	Op   string // "url", "open", "reveal"
	Arg  string
	Main bool // made on the main thread
}

// AppControlState is what the AppControl calls left behind.
type AppControlState struct {
	Hidden     bool
	Badge      string
	Progress   float64 // negative: hidden
	Attention  int     // RequestAttention calls
	Critical   bool    // last RequestAttention's argument
	DockHidden bool
}

type integState struct {
	mu          sync.Mutex
	displays    []platform.Display
	theme       platform.Theme
	shellErr    error
	shell       []ShellCall
	app         AppControlState
	unsupported platform.AppControlOp
}

var integStates sync.Map // *Backend -> *integState

func (b *Backend) integ() *integState {
	if s, ok := integStates.Load(b); ok {
		return s.(*integState)
	}
	s, _ := integStates.LoadOrStore(b, &integState{
		displays: []platform.Display{DefaultDisplay},
		app:      AppControlState{Progress: -1},
	})
	return s.(*integState)
}

// running reports whether Init ran, so there is a Host to tell.
func (b *Backend) running() bool { return b.mainG.Load() != 0 }

// ---- platform.Displays ----

func (b *Backend) Displays() []platform.Display {
	b.mustMain("Displays")
	s := b.integ()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.displays)
}

// SetDisplays replaces the display list and, once the app runs, emits
// AppDisplaysChanged like a real monitor change would.
func (b *Backend) SetDisplays(ds []platform.Display) bool {
	s := b.integ()
	s.mu.Lock()
	s.displays = slices.Clone(ds)
	s.mu.Unlock()
	if !b.running() {
		return false
	}
	return b.AppEvent(platform.AppEvent{Kind: platform.AppDisplaysChanged})
}

// ---- platform.SystemTheme ----

func (b *Backend) SystemTheme() platform.Theme {
	b.mustMain("SystemTheme")
	s := b.integ()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.theme
}

// SetTheme changes the system theme and, once the app runs, emits
// AppThemeChanged.
func (b *Backend) SetTheme(t platform.Theme) bool {
	s := b.integ()
	s.mu.Lock()
	s.theme = t
	s.mu.Unlock()
	if !b.running() {
		return false
	}
	return b.AppEvent(platform.AppEvent{Kind: platform.AppThemeChanged, Theme: t})
}

// ---- platform.Shell ----

func (b *Backend) shellCall(op, arg string) error {
	s := b.integ()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shell = append(s.shell, ShellCall{Op: op, Arg: arg, Main: b.IsMainThread()})
	return s.shellErr
}

func (b *Backend) OpenURL(url string) error     { return b.shellCall("url", url) }
func (b *Backend) OpenPath(path string) error   { return b.shellCall("open", path) }
func (b *Backend) RevealPath(path string) error { return b.shellCall("reveal", path) }

// SetShellErr makes every Shell call fail with err (nil: succeed).
func (b *Backend) SetShellErr(err error) {
	s := b.integ()
	s.mu.Lock()
	s.shellErr = err
	s.mu.Unlock()
}

// ShellCalls returns the Shell calls made so far.
func (b *Backend) ShellCalls() []ShellCall {
	s := b.integ()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.shell)
}

// ---- platform.AppControl ----

func (b *Backend) appctl(what string, fn func(*AppControlState)) {
	b.mustMain(what)
	s := b.integ()
	s.mu.Lock()
	fn(&s.app)
	s.mu.Unlock()
}

func (b *Backend) HideApp() { b.appctl("HideApp", func(a *AppControlState) { a.Hidden = true }) }
func (b *Backend) ShowApp() { b.appctl("ShowApp", func(a *AppControlState) { a.Hidden = false }) }

func (b *Backend) SetBadge(text string) {
	b.appctl("SetBadge", func(a *AppControlState) { a.Badge = text })
}

func (b *Backend) SetProgress(p float64) {
	b.appctl("SetProgress", func(a *AppControlState) { a.Progress = p })
}

func (b *Backend) RequestAttention(critical bool) {
	b.appctl("RequestAttention", func(a *AppControlState) { a.Attention++; a.Critical = critical })
}

func (b *Backend) SetDockVisible(visible bool) {
	b.appctl("SetDockVisible", func(a *AppControlState) { a.DockHidden = !visible })
}

// AppControlOps reports every operation except those marked with
// SetAppControlUnsupported.
func (b *Backend) AppControlOps() platform.AppControlOp {
	s := b.integ()
	s.mu.Lock()
	defer s.mu.Unlock()
	return platform.CtlAll &^ s.unsupported
}

// SetAppControlUnsupported marks AppControl operations as unsupported,
// like a platform without them.
func (b *Backend) SetAppControlUnsupported(ops platform.AppControlOp) {
	s := b.integ()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unsupported |= ops
}

// AppControl returns a copy of the AppControl state.
func (b *Backend) AppControl() AppControlState {
	s := b.integ()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.app
}

// WebViewVersion reports a fixed engine (platform.WebViewInfo).
func (b *Backend) WebViewVersion() (engine, version string) { return "FakeView", "1.0" }
