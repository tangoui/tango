// Package jsapi exposes Tango's system capabilities to the page: the
// window the page lives in, app information and control, clipboard,
// dialogs, notifications, shell, context menus and global shortcuts. The
// JS side is the Window, App, Clipboard, ... modules of @tangoui/runtime
// (window.tango.Window, ... in the injected IIFE); docs/runtime-api.md
// lists every function.
//
//	r := ipc.NewRouter()
//	app.AddBridgeScript(runtimejs.Script)
//	app.OnMessage(func(s *tango.Surface, m string) { r.HandleMessage(s, m) })
//	api := jsapi.Install(app, r, jsapi.Options{Allow: jsapi.ClipboardWrite | jsapi.Dialogs})
//
// Install only registers ipc methods on r; feeding page messages to the
// router stays the app's job, as above.
//
// # Capabilities
//
// Methods are grouped into capabilities (Cap). A page may only use the
// enabled ones; calling a method of a disabled capability fails with the
// error code "forbidden" (CodeForbidden). By default only Window (the
// calling window controls itself), Menu (context menus in the calling
// window) and Info (environment, theme, displays and app events, read
// only) are enabled; everything that reaches outside the window — the
// clipboard, dialogs, notifications, opening URLs or files, app-wide
// control, global shortcuts — must be turned on with Options.Allow.
//
// # Wire names
//
// Methods are ipc calls named "tango:<module>.<function>" and events are
// named "tango:<module>.<event>", so they never collide with an app's own
// methods. Errors carry the codes of package ipc plus CodeForbidden,
// CodeUnsupported (the platform cannot do it) and CodeNotRunning (the app
// is quitting).
package jsapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/sys/clipboard"
	"github.com/tangoui/tango/sys/hotkey"
	"github.com/tangoui/tango/sys/shell"
	"github.com/tangoui/tango/sys/single"
)

// Error codes added to the ones of package ipc.
const (
	CodeForbidden   = "forbidden"   // the capability is not enabled in Options
	CodeUnsupported = "unsupported" // the platform cannot do it
	CodeNotRunning  = "not_running" // the app is not running (quitting)
)

// Prefix starts every method and event name of the runtime API.
const Prefix = "tango:"

// Cap is a set of capabilities a page may use.
type Cap uint64

const (
	// Window lets a page control its own window: title, size, position,
	// state, always-on-top, zoom, reload, show/hide/focus/close; and
	// receive its window events. On by default.
	Window Cap = 1 << iota
	// Menu lets a page pop up context menus it describes, in its own
	// window. Only edit roles (copy, paste, ...) are accepted. On by
	// default.
	Menu
	// Info lets a page read the environment, the system theme and the
	// displays, and receive theme, display and app events (OpenURL,
	// OpenFiles, Reopen, second instance). On by default.
	Info
	// AppControl lets a page quit the app, hide/show it, set the dock
	// badge and progress and request attention.
	AppControl
	// ClipboardRead lets a page read the clipboard (text, image, files).
	ClipboardRead
	// ClipboardWrite lets a page write the clipboard (text, image, files).
	ClipboardWrite
	// Dialogs lets a page show message, open and save dialogs.
	Dialogs
	// Notify lets a page ask for notification permission, send
	// notifications and receive their clicks.
	Notify
	// OpenURL lets a page open URLs with the default handler; schemes are
	// still limited by shell.AllowSchemes (http, https, mailto by default).
	OpenURL
	// OpenPath lets a page open files and folders with their default app
	// and reveal them in the file manager.
	OpenPath
	// Hotkeys lets a page register system-wide shortcuts.
	Hotkeys

	// Bits from 32 up are reserved for capabilities of other packages
	// (mobile sys/ packages) that register methods with API.Handle.
)

// Defaults are the capabilities enabled unless Options.Deny removes them.
const Defaults = Window | Menu | Info

// All is every capability defined by this package.
const All = Window | Menu | Info | AppControl | ClipboardRead | ClipboardWrite |
	Dialogs | Notify | OpenURL | OpenPath | Hotkeys | WebAll

// Options configures Install.
type Options struct {
	// Allow adds capabilities to Defaults.
	Allow Cap
	// Deny removes capabilities (from Defaults or Allow).
	Deny Cap
}

// Handler serves one runtime API method. s is the calling surface (nil
// when the caller is not a surface of this app, which Handle-registered
// methods may accept or refuse). args is the raw JSON argument array.
type Handler func(ctx context.Context, s *tango.Surface, args json.RawMessage) (any, error)

// API is the installed runtime API of one app.
type API struct {
	app  *tango.App
	r    *ipc.Router
	caps Cap

	mu       sync.Mutex
	surfaces map[*tango.Surface]*pageState
	order    []*tango.Surface // attach order, for broadcasts
	pending  []pendingEvent   // launch events no page has taken yet
	notes    map[string]*tango.Surface
	noteOnce sync.Once
}

// pageState is what the API keeps per surface.
type pageState struct {
	hotkeys map[int]*hotkey.Hotkey
	nextHK  int
	gen     int // bumped when the page's document is replaced
}

// Install registers the runtime API on r for app's pages and returns it.
// Call it once per app, before Run, so that a deep link or file that
// launches the app is kept until a page subscribes. Install listens to
// App.OnEvent, so launch events no longer wait for OnOpenURL / OnOpenFiles
// listeners registered after it: register those before Install.
func Install(app *tango.App, r *ipc.Router, opts Options) *API {
	api := &API{
		app:      app,
		r:        r,
		caps:     (Defaults | opts.Allow) &^ opts.Deny,
		surfaces: map[*tango.Surface]*pageState{},
		notes:    map[string]*tango.Surface{},
	}
	app.OnMessage(api.observe)
	app.OnEvent(api.onAppEvent)
	// Every surface created from now on gets its window events from the
	// start (focus, resize, ...), before its page first calls the API.
	app.OnSurfaceCreated(api.attach)
	single.OnSecondInstance(app, api.secondInstance)
	api.registerWindow()
	api.registerApp()
	api.registerSystem()
	api.registerWeb() // webview.go
	for _, s := range app.Surfaces() {
		api.attach(s)
	}
	return api
}

// Allowed reports whether every capability in c is enabled.
func (api *API) Allowed(c Cap) bool { return api.caps&c == c }

// Handle registers fn as the method Prefix+name, available when every
// capability in c is enabled (otherwise it fails with CodeForbidden).
// Other packages use it to add modules under the same switch board.
func (api *API) Handle(c Cap, name string, fn Handler) {
	method := Prefix + name
	api.r.Handle(method, func(ctx context.Context, args json.RawMessage) (any, error) {
		if !api.Allowed(c) {
			return nil, ipc.NewError(CodeForbidden, fmt.Sprintf("%s is not enabled (jsapi.Options.Allow)", method))
		}
		s := api.caller(ctx)
		res, err := fn(ctx, s, args)
		if err != nil {
			return nil, wireError(err)
		}
		return res, nil
	})
}

// Emit sends the event Prefix+name to s's page.
func (api *API) Emit(s *tango.Surface, name string, data any) {
	_ = api.r.Emit(s, Prefix+name, data)
}

// Broadcast sends the event Prefix+name to every page that has talked to
// the API (or existed when it was installed).
func (api *API) Broadcast(name string, data any) {
	for _, s := range api.pages() {
		api.Emit(s, name, data)
	}
}

// secondInstance forwards a later launch to the pages as the
// "tango:app.secondInstance" event (App.onSecondInstance in JS). Install
// registers it with single.OnSecondInstance, so it works as soon as the
// app calls single.Claim.
func (api *API) secondInstance(args []string, cwd string) {
	if !api.Allowed(Info) {
		return
	}
	if args == nil {
		args = []string{}
	}
	api.Broadcast("app.secondInstance", map[string]any{"args": args, "cwd": cwd})
}

// caller returns the surface serving ctx, attaching it on first use.
func (api *API) caller(ctx context.Context) *tango.Surface {
	c, ok := ipc.ConnFrom(ctx)
	if !ok {
		return nil
	}
	s, ok := c.(*tango.Surface)
	if !ok || s.App() != api.app {
		return nil
	}
	api.attach(s)
	return s
}

// observe sees every page message (main thread): it attaches the surface
// and resets per-page state when a new document's runtime says hello.
func (api *API) observe(s *tango.Surface, msg string) {
	api.attach(s)
	if isHello(msg) {
		api.resetPage(s)
	}
}

func isHello(msg string) bool {
	m := strings.TrimSpace(msg)
	if !strings.HasPrefix(m, "{") || !strings.Contains(m, `"hello"`) {
		return false
	}
	var w struct {
		Tango string `json:"tango"`
	}
	return json.Unmarshal([]byte(m), &w) == nil && w.Tango == "hello"
}

// attach starts forwarding s's window events once.
func (api *API) attach(s *tango.Surface) {
	api.mu.Lock()
	if _, ok := api.surfaces[s]; ok {
		api.mu.Unlock()
		return
	}
	api.surfaces[s] = &pageState{hotkeys: map[int]*hotkey.Hotkey{}}
	api.order = append(api.order, s)
	api.mu.Unlock()
	s.OnClosed(func() { api.detach(s) })
	api.hookWindow(s)
}

func (api *API) detach(s *tango.Surface) {
	api.resetPage(s)
	api.mu.Lock()
	delete(api.surfaces, s)
	for i, x := range api.order {
		if x == s {
			api.order = append(api.order[:i:i], api.order[i+1:]...)
			break
		}
	}
	for id, ns := range api.notes {
		if ns == s {
			delete(api.notes, id)
		}
	}
	api.mu.Unlock()
}

// resetPage drops what the previous document of s registered.
func (api *API) resetPage(s *tango.Surface) {
	api.mu.Lock()
	ps := api.surfaces[s]
	var hks []*hotkey.Hotkey
	if ps != nil {
		ps.gen++
		for id, h := range ps.hotkeys {
			hks = append(hks, h)
			delete(ps.hotkeys, id)
		}
	}
	api.mu.Unlock()
	for _, h := range hks {
		_ = h.Unregister()
	}
}

func (api *API) pages() []*tango.Surface {
	api.mu.Lock()
	defer api.mu.Unlock()
	return append([]*tango.Surface(nil), api.order...)
}

func (api *API) page(s *tango.Surface) *pageState {
	api.mu.Lock()
	defer api.mu.Unlock()
	return api.surfaces[s]
}

// ---- arguments and errors ----

// decode unmarshals the JSON argument array into dst, one value per
// argument. Missing trailing arguments leave their zero value.
func decode(args json.RawMessage, dst ...any) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return ipc.NewError(ipc.CodeBadArgs, "arguments must be an array")
	}
	if len(raw) > len(dst) {
		return ipc.NewError(ipc.CodeBadArgs, fmt.Sprintf("want at most %d arguments, got %d", len(dst), len(raw)))
	}
	for i, r := range raw {
		if string(r) == "null" {
			continue
		}
		if err := json.Unmarshal(r, dst[i]); err != nil {
			return ipc.NewError(ipc.CodeBadArgs, fmt.Sprintf("argument %d: %v", i+1, err))
		}
	}
	return nil
}

var errNoSurface = ipc.NewError(ipc.CodeNotFound, "the caller is not a window of this app")

// wireError maps capability-package errors to wire codes.
func wireError(err error) error {
	var ie *ipc.Error
	switch {
	case errors.As(err, &ie):
		return err
	case errors.Is(err, tango.ErrUnsupported):
		return ipc.NewError(CodeUnsupported, err.Error())
	case errors.Is(err, errCanceled):
		return ipc.NewError(ipc.CodeCanceled, "canceled")
	case errors.Is(err, shell.ErrSchemeNotAllowed):
		return ipc.NewError(CodeForbidden, err.Error())
	case errors.Is(err, clipboard.ErrNotPNG), errors.Is(err, clipboard.ErrNotAbsolute),
		errors.Is(err, hotkey.ErrBadAccelerator):
		return ipc.NewError(ipc.CodeBadArgs, err.Error())
	case errors.Is(err, tango.ErrNotRunning):
		return ipc.NewError(CodeNotRunning, err.Error())
	}
	return err
}
