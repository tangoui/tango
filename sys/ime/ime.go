// Package ime hosts a system input method on macOS, Windows, and Linux.
// Experimental.
// Call Serve from OnReady to handle input sessions on the app's main thread:
//
//	a.OnReady(func() {
//		if err := ime.Serve(a, ime.Handler{Key: handleKey}); err != nil {
//			log.Fatal(err)
//		}
//	})
//
// macOS uses InputMethodKit, Windows uses a Text Services Framework DLL,
// and Linux uses IBus or Fcitx5. The [input method example] shows how to build and
// install each host.
//
// Use Handler.End to release per-session state such as a candidate window.
// See Handler.Connection for the name expected by the platform's host.
//
// [input method example]: https://github.com/tangoui/tango/tree/main/examples/inputmethod
package ime

import (
	"errors"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is returned when the app's backend cannot host an input method.
var ErrUnsupported = platform.ErrUnsupported

// ErrServing is returned when the process already serves an input method.
var ErrServing = errors.New("ime: already serving")

// Key is a key-down event. Code follows KeyboardEvent.code naming, such as
// "KeyA", "Digit1", or "ArrowLeft", and is empty for an unknown key. Text
// holds the printable character produced with modifiers applied, if any.
// The desktop backends report CapsLock and Repeat.
type Key struct {
	Code                   string
	Text                   string
	Shift, Ctrl, Alt, Meta bool
	CapsLock, Repeat       bool
}

// ModifierKey reports a change to a modifier, with the side in Code where
// applicable (for example, "ShiftLeft", "ControlRight", or "CapsLock").
// The flags reflect the state after the event. macOS reports CapsLock as a
// lock-state change, so Down on a CapsLock event is the new lock state.
type ModifierKey struct {
	Code                   string
	Down, CapsLock         bool
	Shift, Ctrl, Alt, Meta bool
}

// Mode describes a declared input mode from the application's packaging config.
type Mode struct {
	ID, Label, Short string
}

// MenuItem describes a command in the input method menu. A separator ignores
// all other fields. Command IDs are passed to Handler.MenuClicked.
type MenuItem struct {
	ID, Title                    string
	Checked, Disabled, Separator bool
}

// Candidate is an entry in a native candidate panel.
type Candidate struct {
	Text, Comment string
}

// Handler receives events on the app's main thread. Callback panics are
// recovered and reported to tango.Options.OnPanic when set. The process keeps
// running regardless of its return value. A panicking Key leaves the key
// unhandled.
type Handler struct {
	// Connection overrides the platform default. On macOS the default comes
	// from InputMethodConnectionName in Info.plist, or the bundle identifier
	// followed by _Connection if the plist omits it.
	//
	// On Windows the default comes from the packaged IME configuration.
	// Without a configured name, Windows and Linux use App.ID() followed
	// by _Connection. App.ID() uses Options.Name when Options.ID is empty.
	// The resolved value must match the name configured for the TSF bridge
	// on Windows. IBus uses it without the _Connection suffix as the engine
	// name.
	Connection string
	// Key is required. Return true to consume the key.
	Key func(s *Session, k Key) bool
	// Activate runs when the session becomes active.
	Activate func(s *Session)
	// Deactivate runs when focus leaves or the input method changes.
	// IBus commits the last marked text itself before sending Deactivate.
	Deactivate func(s *Session)
	// CommitComposition runs when the system requests an immediate end to
	// composition. A nil handler leaves composition up to the app.
	CommitComposition func(s *Session)
	// End runs once when an activated session or one seen by a handler ends.
	// The session is already closed. Use End to release associated state.
	// End may run without a preceding Deactivate. On macOS it runs when
	// InputMethodKit releases the controller at the end of the client
	// connection. Focus changes between fields in the same client use
	// Activate and Deactivate.
	End func(s *Session)
	// Modifier receives modifier changes on desktop backends. Return true to consume
	// the event. With no handler the event is passed through.
	Modifier func(s *Session, m ModifierKey) bool
	// ModeChanged receives a system-initiated change of the global input mode
	// on desktop backends. SetMode does not invoke it.
	ModeChanged func(mode string)
	// Menu supplies items for the system input method menu.
	Menu func() []MenuItem
	// MenuClicked receives the ID of a selected input method menu item.
	MenuClicked func(id string)
	// CandidateSelected receives the zero-based index in the session's
	// current candidate list when a user selects an entry from a panel.
	CandidateSelected func(s *Session, index int)
}

var (
	backendOf = func(a *tango.App) (platform.InputMethods, bool) {
		im, ok := a.Backend().(platform.InputMethods)
		return im, ok
	}
	serving atomic.Bool
)

// Serve starts the process's input method server. Call it from OnReady.
// A successful server runs until process exit. Startup errors such as an
// unavailable IBus daemon can be retried. macOS and Linux cache native IME
// loading errors for the life of the process, so retry those after restarting.
func Serve(a *tango.App, h Handler) error {
	im, ok := backendOf(a)
	if !ok {
		return ErrUnsupported
	}
	if h.Key == nil {
		return errors.New("ime: Handler.Key is required")
	}
	type result struct {
		err error
		ran bool
	}
	r := tango.Await(a, func() result {
		if !serving.CompareAndSwap(false, true) {
			return result{ErrServing, true}
		}
		started := false
		defer func() {
			if !started {
				serving.Store(false)
			}
		}()
		s := &server{app: a, h: h, im: im, sessions: make(map[uint64]*Session)}
		err := im.ServeInputMethod(h.Connection, s)
		started = err == nil
		return result{err, true}
	})
	if !r.ran {
		return tango.ErrNotRunning
	}
	return r.err
}

type server struct {
	app      *tango.App
	h        Handler
	im       platform.InputMethods
	sessions map[uint64]*Session // main thread only
}

func (s *server) session(id uint64) *Session {
	session := s.sessions[id]
	if session == nil {
		session = &Session{server: s, id: id}
		s.sessions[id] = session
	}
	return session
}

func (s *server) IMEKey(id uint64, k platform.IMEKey) bool {
	var handled bool
	corehook.GuardNonFatal(s.app, "ime key", func() { handled = s.h.Key(s.session(id), Key(k)) })
	return handled
}

func (s *server) IMEWantsModifiers() bool { return s.h.Modifier != nil }

func (s *server) IMEModifier(id uint64, m platform.IMEModifierKey) bool {
	var handled bool
	if s.h.Modifier != nil {
		corehook.GuardNonFatal(s.app, "ime modifier", func() {
			handled = s.h.Modifier(s.session(id), ModifierKey(m))
		})
	}
	return handled
}

func (s *server) IMEModeChanged(id string) {
	if s.h.ModeChanged != nil {
		corehook.GuardNonFatal(s.app, "ime mode changed", func() { s.h.ModeChanged(id) })
	}
}

func (s *server) IMEMenu() []platform.IMEMenuItem {
	var items []platform.IMEMenuItem
	if s.h.Menu != nil {
		corehook.GuardNonFatal(s.app, "ime menu", func() {
			for _, item := range s.h.Menu() {
				items = append(items, platform.IMEMenuItem(item))
			}
		})
	}
	return items
}

func (s *server) IMEMenuClicked(id string) {
	if s.h.MenuClicked != nil {
		corehook.GuardNonFatal(s.app, "ime menu clicked", func() { s.h.MenuClicked(id) })
	}
}

// SetMode sets the app's process-wide input mode after Serve. The ID must
// be declared in tan.json. Platforms without modes return ErrUnsupported.
func SetMode(a *tango.App, id string) error {
	im, ok := backendOf(a)
	if !ok {
		return ErrUnsupported
	}
	modes, ok := im.(platform.InputMethodModes)
	if !ok {
		return ErrUnsupported
	}
	type result struct {
		err error
		ran bool
	}
	r := tango.Await(a, func() result {
		if id != "" && modes.IMECurrentMode() == id {
			return result{ran: true}
		}
		return result{err: modes.IMESetMode(id), ran: true}
	})
	if !r.ran {
		return tango.ErrNotRunning
	}
	return r.err
}

// CurrentMode returns the app's process-wide mode ID, or an empty string
// when no mode is active or the platform does not support modes.
func CurrentMode(a *tango.App) string {
	im, ok := backendOf(a)
	if !ok {
		return ""
	}
	modes, ok := im.(platform.InputMethodModes)
	if !ok {
		return ""
	}
	return tango.Await(a, modes.IMECurrentMode)
}

// SystemDrawsCandidates reports whether the platform backend provides a
// native candidate panel. Linux uses the IBus or Fcitx5 panel.
func SystemDrawsCandidates() bool { return runtime.GOOS == "linux" }

func (s *server) IMEActivate(id uint64) {
	// An activated session can end before it receives its first key.
	s.session(id).inactive = false
	s.call(id, "ime activate", s.h.Activate)
}

func (s *server) IMEDeactivate(id uint64) {
	if session := s.sessions[id]; session != nil {
		session.inactive = true
		session.candidateCount = 0
	}
	s.call(id, "ime deactivate", s.h.Deactivate)
}

func (s *server) IMECandidateSelected(id uint64, index int) {
	if session := s.sessions[id]; session != nil {
		session.selectCandidate(index)
	}
}

func (s *server) IMECommitComposition(id uint64) {
	s.call(id, "ime commit composition", s.h.CommitComposition)
}

func (s *server) IMEEnd(id uint64) {
	session := s.sessions[id]
	delete(s.sessions, id)
	if session != nil && s.h.End != nil {
		corehook.GuardNonFatal(s.app, "ime end", func() { s.h.End(session) })
	}
}

func (s *server) call(id uint64, where string, fn func(*Session)) {
	if fn != nil {
		corehook.GuardNonFatal(s.app, where, func() { fn(s.session(id)) })
	}
}

// Session is the input method's connection to a client text-entry session.
// Its methods can be called from any goroutine.
type Session struct {
	server         *server
	id             uint64
	candidateCount int
	inactive       bool
}

// ID identifies this session for the life of the process.
func (s *Session) ID() uint64 { return s.id }

// ClientApp returns the macOS bundle ID, Windows executable name, or IBus
// client identification string, when the platform provides one.
func (s *Session) ClientApp() string {
	return tango.Await(s.server.app, func() string {
		if s.live() {
			return s.server.im.IMEClientApp(s.id)
		}
		return ""
	})
}

// SetComposition replaces the marked text, or clears it when text is
// empty. Cursor counts runes and is clamped to the text. Invalid UTF-8 is
// replaced with U+FFFD. Calls from other goroutines are posted to the main
// thread.
func (s *Session) SetComposition(text string, cursor int) {
	text = strings.ToValidUTF8(text, "\uFFFD")
	s.run(func() { s.server.im.IMESetComposition(s.id, text, cursor) })
}

// Commit replaces the marked text with final text. Invalid UTF-8 is
// replaced with U+FFFD. Calls from other goroutines are posted to the main
// thread.
func (s *Session) Commit(text string) {
	text = strings.ToValidUTF8(text, "\uFFFD")
	s.run(func() { s.server.im.IMECommit(s.id, text) })
}

// SetCandidates replaces the session's candidate list. Pass an empty list
// to clear it and use -1 for no highlighted entry. On backends with a native
// panel this also updates the panel. Calls from other goroutines are posted
// to the main thread.
func (s *Session) SetCandidates(list []Candidate, selected int) {
	im, native := s.server.im.(platform.InputMethodCandidates)
	var items []platform.IMECandidate
	if native {
		items = make([]platform.IMECandidate, len(list))
		for i, item := range list {
			items[i] = platform.IMECandidate{
				Text:    strings.ToValidUTF8(item.Text, "\uFFFD"),
				Comment: strings.ToValidUTF8(item.Comment, "\uFFFD"),
			}
		}
	}
	count := len(list)
	if selected < 0 || selected >= count {
		selected = -1
	}
	s.run(func() {
		if !s.inactive {
			s.candidateCount = count
		}
		if native {
			im.IMESetCandidates(s.id, items, selected)
		}
	})
}

// SelectCandidate reports a user selection from a custom panel to
// Handler.CandidateSelected. Index is zero-based in the last list passed to
// SetCandidates. Closed or inactive sessions and out-of-range indexes are
// ignored. Calls from other goroutines are posted to the main thread.
func (s *Session) SelectCandidate(index int) {
	s.run(func() { s.selectCandidate(index) })
}

func (s *Session) selectCandidate(index int) {
	if callback := s.server.h.CandidateSelected; callback != nil &&
		!s.inactive && index >= 0 && index < s.candidateCount {
		corehook.GuardNonFatal(s.server.app, "ime candidate selected", func() { callback(s, index) })
	}
}

// Caret returns the cursor rectangle in logical desktop pixels from the
// top-left of the primary display. It returns the zero rectangle if unknown,
// including on Wayland and when using Fcitx5.
func (s *Session) Caret() tango.Rect {
	return tango.Await(s.server.app, func() tango.Rect {
		if s.live() {
			return s.server.im.IMECaret(s.id)
		}
		return tango.Rect{}
	})
}

func (s *Session) live() bool { return s.server.sessions[s.id] == s }

func (s *Session) run(fn func()) {
	apply := func() {
		if s.live() {
			fn()
		}
	}
	if corehook.IsMain(s.server.app) {
		apply()
		return
	}
	s.server.app.Post(apply)
}
