package editor

import (
	"math"
	"sync"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
)

// lineHeightRatio is the line height relative to the font size (Monaco's
// GOLDEN_LINE_HEIGHT_RATIO on macOS; one value on every platform keeps the
// layout identical everywhere).
const lineHeightRatio = 1.5

// lineCacheSize is the capacity of a View's LineCache: a few screens of
// lines. Long lines are cached chunk by chunk (see longLine).
const lineCacheSize = 512

// ViewW is the code editor view of a State: it draws the visible lines with
// syntax colors, line numbers, the current-line highlight, the selection
// and the blinking caret, scrolls on the mouse wheel and trackpad, and edits
// the State from the keyboard (keys run commands through a keymap; see
// command.go and keymap.DefaultRules), the mouse and the IME.
//
// The View measures and shapes only the visible lines (plus two above and
// below); lines longer than longLineRunes are cut into chunks and only the
// chunks in the visible horizontal range are tokenized and shaped (see
// longLine). With word wrap (Options.WordWrap, editor.action.toggleWordWrap)
// the lines are view lines: a long model line is shown as several rows,
// each shaped on its own, and only the lines around the viewport are
// wrapped (wrap.go, viewlines.go). Redraws are driven by input and by the application: apart
// from the caret blink (a Context.WakeAt while focused) and one frame after
// input it applied while painting, the View never requests frames on its
// own, not even while highlighting is incomplete. An application that starts the Highlighter's background
// goroutine passes c.Invalidator() (or ui.Invalidate) as its ready
// callback, so newly final tokens are drawn when they arrive.
type ViewW struct {
	c   *ui.Context
	st  *State
	box *ui.Box
	vs  *viewState
	ia  ui.Interaction

	opts    Options // this frame's options: st.Options with the modifiers' overrides
	km      *keymap.Keymap
	ctxFn   func(key string) (any, bool)
	changed bool
	done    bool // input applied (finish)
}

// viewState is the View's retained state.
type viewState struct {
	lines *text.LineCache
	toks  []monarch.Token
	long  map[longKey]*longLine

	// Widest line seen (content width); reset when the buffer or the
	// wrapping column changes.
	buf     *buffer.Buffer
	maxW    float32
	wrapCol int

	// Geometry of the last paint, absolute: the whole view, the gutter and
	// the text area, and the monospace column width.
	rect, gutter, text geom.Rect
	colW               float32

	inputState

	// menu is the context menu (menu.go).
	menu menuState

	// dmg is what the last paint drew, for the redraw hints (damage.go);
	// dmgSpare holds the buffers of the paint before it, for reuse.
	dmg, dmgSpare damageRec
}

func newViewState() viewState {
	return viewState{lines: text.NewLineCache(lineCacheSize), long: map[longKey]*longLine{}}
}

// shapeHook, when set (tests), sees every LineCache call the View makes for
// a document line (a chunk of it for long lines).
var shapeHook func(line int, s string)

// View shows st. The application holds st; the View sets st.Viewport's
// measured sizes and scrolls it.
func View(c *ui.Context, st *State) *ViewW {
	w := &ViewW{c: c, st: st, opts: st.Options}
	st.frameTab = 0
	st.view = w
	w.box = ui.Custom(c, w.measure, w.paint, layout.Grow(1))
	w.box.Clip()
	w.vs = ui.State(c, w.box.ID(), newViewState)
	// While dragging selected text the pointer is an arrow, or the copy
	// arrow while the trigger modifier is held (Monaco's mouseStyle
	// "default" / "copy"; dnd.go).
	cur := input.CursorText
	if d := w.vs.dnd; d != nil && d.moved {
		cur = input.CursorDefault
		if w.dndCopy(d) {
			cur = input.CursorCopy
		}
	}
	w.ia = w.box.Focusable(cur)
	w.collect()
	wheel := false
	if dx, dy, ok := c.WheelFor(w.box.ID()); ok && (dx != 0 || dy != 0) {
		st.ScrollBy(dy, dx)
		wheel = true
	}
	w.contextMenu(wheel)
	return w
}

// The modifiers below override the State's Options for this frame only
// (immediate mode): a frame that does not call them uses st.Options, the
// persistent settings the application sets.

// LineNumbers shows or hides the line numbers this frame (default:
// st.Options.LineNumbers).
func (w *ViewW) LineNumbers(on bool) *ViewW {
	w.opts.LineNumbers = LineNumbersOff
	if on {
		w.opts.LineNumbers = LineNumbersOn
	}
	return w
}

// FontSize sets the font size in logical pixels this frame (default:
// st.Options.FontSize); values <= 0 are ignored.
func (w *ViewW) FontSize(px float32) *ViewW {
	if px > 0 {
		w.opts.FontSize = px
	}
	return w
}

// TabSize sets the tab width in columns this frame (default: the Model's
// TabSize), for drawing and for the caret movement the State does until
// the next frame; values <= 0 are ignored. It does not change the Model's
// TabSize, which Tab and the indentation commands use.
func (w *ViewW) TabSize(n int) *ViewW {
	if n > 0 {
		w.st.frameTab = n
	}
	return w
}

// ReadOnly makes the view refuse edits this frame (typing, deleting, Enter,
// Tab, paste, cut, undo, redo, IME); the caret, selection, navigation and
// copy keep working. A frame that does not call it is read-only only when
// st.Options.ReadOnly is set.
func (w *ViewW) ReadOnly(on bool) *ViewW { w.opts.ReadOnly = on; return w }

// Keymap sets the keymap that turns this frame's keys into commands
// (default: keymap.DefaultRules for the current platform, with a chord
// state of the view's own). Keep the same Keymap across frames: it holds
// the pending chord. Several views may share one; a view that loses focus
// clears its pending chord.
func (w *ViewW) Keymap(km *keymap.Keymap) *ViewW {
	if km != nil {
		w.km = km
	}
	return w
}

// Context adds application context keys for this frame's keybinding when
// clauses and command preconditions: fn is asked before the built-in keys
// (see contextkeys.go) and reports false for keys it does not define.
func (w *ViewW) Context(fn func(key string) (any, bool)) *ViewW {
	w.ctxFn = fn
	return w
}

// defaultKeymap is the compiled default keymap; views clone it.
var defaultKeymap = sync.OnceValue(func() *keymap.Keymap {
	p := keymap.Current()
	return keymap.New(p, keymap.DefaultRules(p))
})

// keymap returns the keymap in effect this frame.
func (w *ViewW) keymap() *keymap.Keymap {
	if w.km != nil {
		return w.km
	}
	vs := w.vs
	if vs.defKeymap == nil {
		vs.defKeymap = defaultKeymap().Clone()
		c := w.c
		vs.defKeymap.SetClock(c.Now)
	}
	return vs.defKeymap
}

// active reports whether the view has keyboard focus in an active window.
func (w *ViewW) active() bool { return w.ia.Focused && w.c.WindowFocused() }

// Layout applies layout options to the view's box (it grows by default).
func (w *ViewW) Layout(opts ...layout.Option) *ViewW { w.box.Style(opts...); return w }

// Box returns the view's box (ID, rect).
func (w *ViewW) Box() *ui.Box { return w.box }

// Changed reports whether the text was edited through the View this
// frame. It applies the frame's input, so call it after the modifiers
// (ReadOnly ...); without a call the input is applied when the view paints.
func (w *ViewW) Changed() bool { w.finish(); return w.changed }

// Focused reports whether the view has keyboard focus.
func (w *ViewW) Focused() bool { return w.ia.Focused }

func (w *ViewW) measure(maxW, maxH float32) geom.Size {
	sz := geom.Size{W: 400, H: 300}
	if !math.IsInf(float64(maxW), 0) && maxW > 0 {
		sz.W = maxW
	}
	if !math.IsInf(float64(maxH), 0) && maxH > 0 {
		sz.H = maxH
	}
	return sz
}

// lineHeight returns the line height for the font size.
func (w *ViewW) lineHeight() float32 {
	return float32(math.Round(float64(w.opts.fontSize()) * lineHeightRatio))
}

// monoStyle returns the text style of the document text.
func (w *ViewW) monoStyle() text.Style {
	return text.Style{Size: w.opts.fontSize(), LineHeight: w.lineHeight(), Mono: true}
}

// tabs returns the tab width in columns.
func (w *ViewW) tabs() int { return w.st.tabSize() }
