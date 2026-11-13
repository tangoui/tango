package uihost

// Window shortcuts of self-drawn windows. Every key event of a window
// opened with Open (locally: host.CanvasInput; under tan dev:
// remoteWin.input) is first matched against the window's shortcut table;
// a hit is consumed and never reaches the ui.Runner.
//
// Rules (see Shortcut):
//
//   - Accelerators use the sys/menu / sys/hotkey syntax and are parsed by
//     hotkey.Parse, so CmdOrCtrl is Command on macOS and Control elsewhere.
//   - Nothing fires while an input method is composing (between a
//     non-empty input.Composition and its commit or cancel).
//   - While a text field has the keyboard focus (the runner turned text
//     input on), a shortcut without ShortcutOptions.InTextInput fires only
//     for F1–F12 or for CmdOrCtrl combinations that are not text editing
//     keys (A, C, V, X, Y, Z, arrows, Home, End, PageUp, PageDown,
//     Backspace, Delete). Plain keys, Shift+key, Alt+key, Enter, Escape,
//     Tab and the edit combinations stay with the text field.
//   - Menu accelerators win: the platform handles them before the canvas
//     sees the key (macOS key equivalents, Windows TranslateAccelerator,
//     GtkWindow accel groups), so a shortcut with the same accelerator as
//     a menu item never fires. Edit menu roles (Copy, Paste, ...) reach the
//     canvas as a single Cmd/Ctrl+key press; with no text field focused, a
//     shortcut on that combination fires once for it.

import (
	"errors"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/sys/hotkey"
	"github.com/tangoui/tango/ui/input"
)

// ErrNotUI is returned by Shortcut for a surface that was not opened with
// Open, or that has closed.
var ErrNotUI = errors.New("uihost: not an open self-drawn window")

// ShortcutOptions refine ShortcutWith.
type ShortcutOptions struct {
	// InTextInput also fires the shortcut while a text field has the
	// keyboard focus, even for keys the field would use (plain keys,
	// Escape, CmdOrCtrl+C ...). Never while an input method is composing.
	InTextInput bool
}

// Shortcut makes accelerator ("CmdOrCtrl+N", "Shift+CmdOrCtrl+Z",
// "Alt+F4", "F5", "Escape") call fn while s, a window opened with Open,
// has the keyboard focus. The key press (with its repeats and release)
// does not reach the UI; holding the key fires fn once. fn runs on the
// main thread; a panic in it goes to the app's panic handling
// (tango.Options.OnPanic, Where "event handler").
//
// While a text field is focused only CmdOrCtrl combinations that are not
// text editing keys and F1–F12 fire (see ShortcutWith for the rest). A
// menu item with the same accelerator takes the key first, so the
// shortcut never fires; do not register accelerators your menus use.
//
// remove unregisters the shortcut; calling it again does nothing.
// Shortcuts end when the window closes. When several shortcuts of a
// window share an accelerator, the first registered fires. The error
// wraps hotkey.ErrBadAccelerator for a malformed accelerator (or a key
// self-drawn windows do not report, such as F13), or is ErrNotUI.
// Shortcut may be called from any goroutine.
func Shortcut(s *tango.Surface, accelerator string, fn func()) (remove func(), err error) {
	return ShortcutWith(s, accelerator, ShortcutOptions{}, fn)
}

// ShortcutWith is Shortcut with options.
func ShortcutWith(s *tango.Surface, accelerator string, opts ShortcutOptions, fn func()) (remove func(), err error) {
	b, err := parseShortcut(accelerator)
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, errors.New("uihost: nil shortcut function")
	}
	b.inText, b.fn = opts.InTextInput, fn
	t := shortcutTable(s)
	if t == nil {
		return nil, ErrNotUI
	}
	t.mu.Lock()
	t.binds = append(t.binds, b)
	t.mu.Unlock()
	return func() { t.remove(b) }, nil
}

// shortcut is one registered binding.
type shortcut struct {
	key    input.Key // as reported for the key on a US layout
	char   rune      // also matches a key producing this character; 0: none
	mods   input.Modifiers
	inText bool
	fn     func()
}

// parseShortcut parses accelerator with hotkey.Parse and maps its key to an
// input.Key.
func parseShortcut(accelerator string) (*shortcut, error) {
	sp, err := hotkey.Parse(accelerator)
	if err != nil {
		return nil, err
	}
	k, ok := shortcutKey(sp.Code)
	if !ok {
		return nil, errors.Join(hotkey.ErrBadAccelerator,
			errors.New("uihost: accelerator "+accelerator+": key not reported in self-drawn windows"))
	}
	b := &shortcut{key: k}
	if r := []rune(sp.Key); len(r) == 1 {
		b.char = r[0]
	}
	if sp.Shift {
		b.mods |= input.ModShift
	}
	if sp.Ctrl {
		b.mods |= input.ModCtrl
	}
	if sp.Alt {
		b.mods |= input.ModAlt
	}
	if sp.Meta {
		b.mods |= input.ModMeta
	}
	return b, nil
}

// shortcutKey maps a KeyboardEvent.code name (hotkey.Spec.Code) to the
// input.Key hosts report.
func shortcutKey(code string) (input.Key, bool) {
	name := code
	switch {
	case len(code) == 4 && code[:3] == "Key":
		name = code[3:]
	case len(code) == 6 && code[:5] == "Digit":
		name = code[5:]
	case len(code) > 5 && code[:5] == "Arrow":
		name = code[5:]
	}
	return input.KeyNamed(name)
}

// matches reports whether the key press ev is b's accelerator.
func (b *shortcut) matches(ev input.KeyEvent) bool {
	const mods = input.ModShift | input.ModCtrl | input.ModAlt | input.ModMeta
	if ev.Mods&mods != b.mods {
		return false
	}
	return ev.Key == b.key || ev.Key.Base() == b.key || (b.char != 0 && ev.Key.Rune() == b.char)
}

// allowedInText reports whether b may take the key press ev while a text
// field has the focus.
func (b *shortcut) allowedInText(ev input.KeyEvent) bool {
	if b.inText {
		return true
	}
	if ev.Key >= input.KeyF1 && ev.Key <= input.KeyF12 {
		return true
	}
	if !ev.Mods.Shortcut() {
		return false
	}
	switch ev.Key.Base() {
	case input.KeyA, input.KeyC, input.KeyV, input.KeyX, input.KeyY, input.KeyZ,
		input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown,
		input.KeyHome, input.KeyEnd, input.KeyPageUp, input.KeyPageDown,
		input.KeyBackspace, input.KeyDelete:
		return false
	}
	return true
}

// shortcuts is the table of one window. binds is guarded by mu (Shortcut
// may run anywhere); the rest is main-thread state.
type shortcuts struct {
	app   *tango.App
	mu    sync.Mutex
	binds []*shortcut

	textOn    bool               // the runner turned text input on
	composing bool               // an input method is composing
	held      map[input.Key]bool // keys whose press fired: repeats and release are swallowed
}

var shortcutTables sync.Map // *tango.Surface → *shortcuts

// newShortcuts creates s's table, dropped when s closes. Main thread.
func newShortcuts(app *tango.App, s *tango.Surface) *shortcuts {
	t := &shortcuts{app: app, held: map[input.Key]bool{}}
	if s != nil {
		shortcutTables.Store(s, t)
		s.OnClosed(func() { shortcutTables.Delete(s) })
	}
	return t
}

func shortcutTable(s *tango.Surface) *shortcuts {
	if s == nil {
		return nil
	}
	t, _ := shortcutTables.Load(s)
	sc, _ := t.(*shortcuts)
	return sc
}

func (t *shortcuts) remove(b *shortcut) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, x := range t.binds {
		if x == b {
			t.binds = append(t.binds[:i:i], t.binds[i+1:]...)
			return
		}
	}
}

// setText records the runner's text input state. Main thread; t may be nil.
func (t *shortcuts) setText(on bool) {
	if t != nil {
		t.textOn = on
	}
}

// handle matches ev against the table, runs a hit and reports whether ev
// was consumed (must not reach the runner). Main thread; t may be nil.
func (t *shortcuts) handle(ev input.Event) bool {
	if t == nil {
		return false
	}
	switch ev := ev.(type) {
	case input.Composition:
		t.composing = ev.Text != ""
	case input.TextInput:
		t.composing = false
	case input.WindowFocus:
		clear(t.held) // releases may never come
		t.composing = false
	case input.KeyEvent:
		return t.key(ev)
	}
	return false
}

func (t *shortcuts) key(ev input.KeyEvent) bool {
	if !ev.Down {
		if t.held[ev.Key] {
			delete(t.held, ev.Key)
			return true
		}
		return false
	}
	if t.held[ev.Key] && ev.Repeat {
		return true
	}
	if t.composing || ev.Key.Modifier() != 0 {
		return false
	}
	var fn func()
	t.mu.Lock()
	for _, b := range t.binds {
		if b.matches(ev) && (!t.textOn || b.allowedInText(ev)) {
			fn = b.fn
			break
		}
	}
	t.mu.Unlock()
	if fn == nil {
		return false
	}
	t.held[ev.Key] = true
	if !ev.Repeat {
		corehook.Guard(t.app, "event handler", fn)
	}
	return true
}
