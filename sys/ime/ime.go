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
// and Linux uses IBus. The [input method example] shows how to build and
// install each host.
//
// [input method example]: https://github.com/tangoui/tango/tree/main/examples/inputmethod
package ime

import (
	"errors"
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
type Key struct {
	Code                   string
	Text                   string
	Shift, Ctrl, Alt, Meta bool
}

// Handler receives events on the app's main thread.
type Handler struct {
	// Connection defaults to the app's ID followed by _Connection.
	// It matches InputMethodConnectionName on macOS and the TSF bridge's
	// connection on Windows. IBus uses it without the _Connection suffix
	// as the engine name.
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
}

var (
	backendOf = func(a *tango.App) (platform.InputMethods, bool) {
		im, ok := a.Backend().(platform.InputMethods)
		return im, ok
	}
	serving atomic.Bool
)

// Serve starts the process's input method server. Call it from OnReady.
// A failed start can be retried. A successful server runs until process exit.
func Serve(a *tango.App, h Handler) error {
	im, ok := backendOf(a)
	if !ok {
		return ErrUnsupported
	}
	if h.Key == nil {
		return errors.New("ime: Handler.Key is required")
	}
	if h.Connection == "" {
		h.Connection = a.ID() + "_Connection"
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

func (s *server) IMEActivate(id uint64) {
	s.call(id, "ime activate", s.h.Activate)
}

func (s *server) IMEDeactivate(id uint64) {
	s.call(id, "ime deactivate", s.h.Deactivate)
}

func (s *server) IMECommitComposition(id uint64) {
	s.call(id, "ime commit composition", s.h.CommitComposition)
}

func (s *server) IMEEnd(id uint64) { delete(s.sessions, id) }

func (s *server) call(id uint64, where string, fn func(*Session)) {
	if fn != nil {
		corehook.GuardNonFatal(s.app, where, func() { fn(s.session(id)) })
	}
}

// Session is the input method's connection to a client text-entry session.
// Its methods can be called from any goroutine.
type Session struct {
	server *server
	id     uint64
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
// empty. Cursor counts runes and is clamped to the text. Calls from other
// goroutines are posted to the main thread.
func (s *Session) SetComposition(text string, cursor int) {
	s.run(func() { s.server.im.IMESetComposition(s.id, text, cursor) })
}

// Commit replaces the marked text with final text. Calls from other
// goroutines are posted to the main thread.
func (s *Session) Commit(text string) {
	s.run(func() { s.server.im.IMECommit(s.id, text) })
}

// Caret returns the cursor rectangle in logical desktop pixels from the
// top-left of the primary display. It returns the zero rectangle if unknown.
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
