package terminal

import (
	"math"
	"runtime"
	"slices"
	"strconv"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/terminal/vt"
	"github.com/tangoui/tango/ui/text"
)

const (
	// defaultFontSize is the cell font size when neither ViewW.FontSize
	// nor the terminal token sets one.
	defaultFontSize = 13
	// lineHeightRatio is the cell height relative to the font size.
	lineHeightRatio = 1.2
	// resizeThrottle is the shortest time between two resizes sent to
	// the program while the view is being resized.
	resizeThrottle = 50 * time.Millisecond
	// segCacheSize is the capacity of a view's shaped-segment cache.
	segCacheSize = 512
	// scrollbarWidth is the width of the overlay scrollback scrollbar.
	scrollbarWidth = 8
	// exitedText is drawn below the output once the program has exited.
	// ui.Locale has no terminal section yet, so it is not translated.
	exitedText = "[Process exited]"
)

// ViewW is the terminal view of a [Terminal]: it draws the emulator's
// viewport in a monospace grid, sends keys, typed text and pastes to the
// program, selects and copies with the mouse, and scrolls the scrollback
// with the wheel, Shift+PageUp / Shift+PageDown and an overlay scrollbar.
//
// Keyboard: while focused the view captures Tab (Context.CaptureTab) and
// Esc (Context.ClaimEscape), so both go to the program; leave the
// terminal with Ctrl+Tab or Ctrl+Shift+Tab. "Esc then Tab leaves" does
// not apply inside a terminal. Application shortcuts are Cmd+C (copy the
// selection), Cmd+V (paste), Cmd+A (select all) and Cmd+K (clear the
// screen and the scrollback) on macOS, and Ctrl+Shift+C/V/A/K plus
// Ctrl+Insert / Shift+Insert elsewhere; every other key goes to
// [vt.EncodeKey]. On macOS Option composes characters unless
// [ViewW.OptionAsMeta] is set. Keys pressed during an IME composition go
// to the input method; the composition is drawn at the cursor and sent
// once committed. Input scrolls the view back to the bottom and clears
// the selection.
//
// Show a Terminal in at most one view: the view takes the emulator's
// redraw records (TakeDirty, TakeScroll) each frame.
//
// Input is queued asynchronously; if the program stops reading its input,
// [Terminal.Send] blocks the frame until it reads again (the bounded queue drops whole sends over 8 MB).
type ViewW struct {
	c   *ui.Context
	t   *Terminal
	box *ui.Box
	vs  *viewState
	ia  ui.Interaction

	fontSize     float32
	optionAsMeta bool
	scheme       *Scheme
	done         bool // input applied (applyInput)
}

// pending is the input collected while the view is built, applied once
// per frame when it paints.
type pending struct {
	events   []input.Event // input.KeyEvent and input.TextInput, in order
	press    bool
	pressPos geom.Point
	clicks   int
	shift    bool
	drag     bool
	dragPos  geom.Point
	release  bool
	wheel    float32
}

// metrics are the cell geometry for a font size and scale.
type metrics struct {
	size, scale  float32
	cellW, lineH float32
	st           text.Style
}

// viewState is the view's retained state.
type viewState struct {
	pend pending
	// IME composition shown at the cursor.
	comp    string
	compCur int
	hover   bool

	m       metrics
	started bool
	// cols×rows is the size last sent to the Terminal, at lastResize.
	cols, rows int
	lastResize time.Time

	sel selection
	// evicted is the screen's Evicted count this frame, the base of the
	// stable line numbers of selections (see selPos).
	evicted  int
	wheelAcc float32
	bar      barState

	cache *segCache

	// What the last paint drew, for the redraw hints (paint.go).
	dmg      damageRec
	needFull bool
	dirty    []int // this frame's dirty rows (buffer)

	a11yText string
	openLink func(url string) // ViewW.OnOpenLink (links come in M2)
}

func newViewState() viewState {
	return viewState{cache: newSegCache(segCacheSize), needFull: true}
}

// View shows t. Call it every frame; the application owns t and closes it.
// The first frame starts t's read loop at the view's size.
func View(c *ui.Context, t *Terminal) *ViewW {
	w := &ViewW{c: c, t: t}
	w.box = ui.Custom(c, measure, w.paint, layout.Grow(1))
	w.box.Clip()
	w.box.Semantics(a11y.RoleTextbox, "Terminal").A11yState(a11y.Multiline | a11y.ReadOnly)
	id := w.box.ID()
	w.vs = ui.State(c, id, newViewState)
	if c.A11yEnabled() {
		vs := w.vs
		w.box.A11y(func(n *a11y.Node) { n.Value = vs.a11yText })
	}
	w.ia = w.box.Focusable(input.CursorText)
	if w.ia.Focused {
		c.CaptureTab(id)
		c.ClaimEscape(id)
	}
	w.collect()
	return w
}

// Layout applies layout options to the view's box (it grows by default).
func (w *ViewW) Layout(opts ...layout.Option) *ViewW { w.box.Style(opts...); return w }

// FontSize sets the cell font size in logical pixels (default: the
// terminal token's ContentFontSize, 13); values <= 0 are ignored.
func (w *ViewW) FontSize(px float32) *ViewW {
	if px > 0 {
		w.fontSize = px
	}
	return w
}

// OptionAsMeta makes Option on macOS act as Meta (Alt: an Esc prefix)
// instead of composing characters (default false). Other platforms always
// send Alt as Meta.
func (w *ViewW) OptionAsMeta(on bool) *ViewW { w.optionAsMeta = on; return w }

// OnOpenLink sets the callback that opens links. Milestone 1 does not
// detect links yet; the callback is kept for later milestones.
func (w *ViewW) OnOpenLink(fn func(url string)) *ViewW { w.vs.openLink = fn; return w }

// Scheme sets the color scheme this frame (default: the colors of the
// theme's terminal token).
func (w *ViewW) Scheme(s Scheme) *ViewW { w.scheme = &s; return w }

// Box returns the view's box (ID, rect).
func (w *ViewW) Box() *ui.Box { return w.box }

func measure(maxW, maxH float32) geom.Size {
	sz := geom.Size{W: 400, H: 300}
	if !math.IsInf(float64(maxW), 0) && maxW > 0 {
		sz.W = maxW
	}
	if !math.IsInf(float64(maxH), 0) && maxH > 0 {
		sz.H = maxH
	}
	return sz
}

// collect takes this frame's input for the view from the Context into
// the retained state: only the build pass that sees the events gets them.
func (w *ViewW) collect() {
	c, id, vs := w.c, w.box.ID(), w.vs
	p := &vs.pend
	if !w.ia.Focused {
		vs.comp, vs.compCur = "", 0
	} else {
		// Keys pressed while a composition is in progress belong to the
		// input method (as in the code editor, ui/editor/input.go).
		composing := vs.comp != ""
		for _, e := range c.EventsFor(id) {
			switch e := e.(type) {
			case input.KeyEvent:
				if !composing {
					p.events = append(p.events, e)
				}
			case input.TextInput:
				p.events = append(p.events, e)
				composing = false
			case input.Composition:
				composing = e.Text != ""
			}
		}
		vs.comp, vs.compCur = c.CompositionFor(id)
	}
	pos, _ := c.Pointer()
	if w.ia.PressedNow {
		p.press, p.pressPos, p.clicks = true, pos, (c.PressCount()-1)%3+1
		p.shift = c.PressMods().Has(input.ModShift)
	}
	if c.Dragging(id) {
		p.drag, p.dragPos = true, pos
	}
	if _, ok := c.ReleasePos(id); ok {
		p.release = true
	}
	vs.hover = w.ia.Hovered || c.Dragging(id)
	if _, dy, ok := c.WheelFor(id); ok {
		p.wheel += dy
	}
}

// metricsFor returns the cell geometry for font size px at scale; the
// cell width is the advance of "M" rounded up to a device pixel.
func (vs *viewState) metricsFor(px, scale float32) metrics {
	if vs.m.size == px && vs.m.scale == scale {
		return vs.m
	}
	lh := float32(math.Round(float64(px * lineHeightRatio)))
	cw := text.Width("M", text.Style{Size: px, Mono: true})
	cw = float32(math.Ceil(float64(cw*scale))) / scale
	vs.m = metrics{size: px, scale: scale, cellW: max(cw, 1), lineH: max(lh, 1),
		st: text.Style{Size: px, LineHeight: lh, Mono: true, Weight: text.Regular}}
	vs.cache.reset()
	vs.needFull = true
	return vs.m
}

// sizeTo starts the Terminal at cols×rows on the first frame and later
// resizes it, at most once per resizeThrottle; a size that arrives
// within the window is applied when it ends (Context.WakeAt).
func (w *ViewW) sizeTo(cols, rows int, m metrics) {
	vs, c := w.vs, w.c
	now := c.Now()
	wpx := int(math.Round(float64(float32(cols) * m.cellW * m.scale)))
	hpx := int(math.Round(float64(float32(rows) * m.lineH * m.scale)))
	if !vs.started {
		vs.started = true
		w.t.Start(cols, rows, c.Invalidator())
		// Start tells the program 0×0 pixels; send the real size.
		w.t.Resize(cols, rows, wpx, hpx)
		vs.cols, vs.rows, vs.lastResize = cols, rows, now
		return
	}
	if cols == vs.cols && rows == vs.rows {
		return
	}
	if due := vs.lastResize.Add(resizeThrottle); now.Before(due) {
		c.WakeAt(due)
		return
	}
	w.t.Resize(cols, rows, wpx, hpx)
	vs.cols, vs.rows, vs.lastResize = cols, rows, now
	vs.sel.clear() // lines may move between the screen and the scrollback
}

// isMac reports whether application shortcuts use Cmd.
var isMac = runtime.GOOS == "darwin"

// applyInput applies the collected input once per frame: pointer and
// wheel, then keys and text in order. It returns the bytes for the
// program; the caller sends them after unlocking the emulator, so a
// program that is slow to read its input never holds up the read loop.
// v is locked.
func (w *ViewW) applyInput(v *vt.Terminal, r geom.Rect) []effect {
	if w.done {
		return nil
	}
	w.done = true
	vs := w.vs
	p := vs.pend
	vs.pend = pending{}
	w.pointerInput(v, p, r)
	if p.wheel != 0 {
		vs.wheelAcc += p.wheel
		n := int(vs.wheelAcc / vs.m.lineH)
		vs.wheelAcc -= float32(n) * vs.m.lineH
		s := v.Screen()
		s.SetViewOffset(s.ViewOffset() - n)
	}
	out := w.keyInput(v, p.events)
	if exited, _ := w.t.Exited(); exited {
		// Nobody reads the input any more: keep only the copies.
		out = slices.DeleteFunc(out, func(e effect) bool { return !e.isCopy })
		return out
	}
	for _, e := range out {
		if !e.isCopy {
			v.Screen().SetViewOffset(0)
			vs.sel.clear()
			break
		}
	}
	return out
}

// effect is one result of the frame's input, carried out in order after
// the emulator is unlocked (runEffects): bytes for the program, a paste
// of the clipboard, or a copy to the clipboard.
type effect struct {
	send   []byte
	paste  bool
	modes  vt.Modes // the modes the paste is encoded for
	isCopy bool
	copy   string
}

// runEffects carries out the effects; the emulator is not locked, so
// neither the clipboard nor a slow program holds up the read loop.
func (w *ViewW) runEffects(out []effect) {
	win := w.c.Window()
	for _, e := range out {
		switch {
		case e.isCopy:
			if win != nil {
				win.WriteClipboard(e.copy)
			}
		case e.paste:
			if win != nil {
				if txt := win.ReadClipboard(); txt != "" {
					w.t.Send(vt.EncodePaste(txt, e.modes))
				}
			}
		default:
			w.t.Send(e.send)
		}
	}
}

// keyInput turns the frame's keys and committed text into bytes for the
// program, running application shortcuts on the way. v is locked.
func (w *ViewW) keyInput(v *vt.Terminal, evs []input.Event) []effect {
	var out []effect
	modes := v.Modes()
	// swallow drops the text the host generates for a key press that was
	// encoded with Ctrl or Alt (or taken as a shortcut), so it is not sent
	// twice.
	swallow := false
	for i, e := range evs {
		switch e := e.(type) {
		case input.KeyEvent:
			if !e.Down {
				continue
			}
			swallow = false
			if w.shortcut(v, e, &out) {
				swallow = true
				continue
			}
			if e.Key == input.KeyTab && e.Mods.Has(input.ModCtrl) {
				continue // the toolkit's "leave the terminal" key
			}
			ev := e
			if isMac && !w.optionAsMeta {
				ev.Mods &^= input.ModAlt // Option composes characters
			}
			b, ok := vt.EncodeKey(ev, modes)
			if !ok {
				continue
			}
			// AltGr arrives as Ctrl+Alt on Windows: when the press also
			// produced text (European layouts type @, € ... this way),
			// the text wins.
			if e.Mods&(input.ModCtrl|input.ModAlt|input.ModShift|input.ModMeta) == input.ModCtrl|input.ModAlt &&
				i+1 < len(evs) && isText(evs[i+1]) {
				continue
			}
			out = append(out, effect{send: b})
			swallow = ev.Mods&(input.ModCtrl|input.ModAlt) != 0
		case input.TextInput:
			if swallow {
				swallow = false
				continue
			}
			if !isText(e) {
				continue // control characters arrive as keys
			}
			out = append(out, effect{send: vt.EncodeText(e.Text)})
		}
	}
	return out
}

// isText reports whether e is committed text other than a lone control
// character.
func isText(e input.Event) bool {
	t, ok := e.(input.TextInput)
	if !ok || t.Text == "" {
		return false
	}
	return !(len(t.Text) == 1 && (t.Text[0] < 0x20 || t.Text[0] == 0x7f))
}

// shortcut runs the application shortcut of key press e, if it is one,
// appending its effects to out. v is locked.
func (w *ViewW) shortcut(v *vt.Terminal, e input.KeyEvent, out *[]effect) bool {
	mods := e.Mods & (input.ModShift | input.ModCtrl | input.ModAlt | input.ModMeta)
	appMods := input.ModCtrl | input.ModShift
	if isMac {
		appMods = input.ModMeta
	}
	s := v.Screen()
	_, rows := s.Size()
	action := ""
	switch {
	case mods == appMods && e.Key == input.KeyC, !isMac && mods == input.ModCtrl && e.Key == input.KeyInsert:
		action = "copy"
	case mods == appMods && e.Key == input.KeyV, !isMac && mods == input.ModShift && e.Key == input.KeyInsert:
		action = "paste"
	case mods == appMods && e.Key == input.KeyA:
		action = "selectAll"
	case mods == appMods && e.Key == input.KeyK:
		action = "clear"
	case mods == input.ModShift && e.Key == input.KeyPageUp:
		s.SetViewOffset(s.ViewOffset() + max(rows-1, 1))
		return true
	case mods == input.ModShift && e.Key == input.KeyPageDown:
		s.SetViewOffset(s.ViewOffset() - max(rows-1, 1))
		return true
	default:
		return false
	}
	switch action {
	case "copy":
		if txt := w.selectedText(s); txt != "" {
			*out = append(*out, effect{isCopy: true, copy: txt})
		}
	case "paste":
		*out = append(*out, effect{paste: true, modes: v.Modes()})
	case "selectAll":
		w.selectAll(v)
	case "clear":
		clearScreen(v)
		w.vs.sel.clear()
		w.vs.trackEviction(s) // ClearScrollback dropped lines: renumber now
		w.vs.needFull = true
	}
	return true
}

// clearScreen clears the scrollback and the screen except the cursor's
// line, which moves to the top (xterm.js's Terminal.clear). v is locked.
func clearScreen(v *vt.Terminal) {
	cx, cy, _ := v.Cursor()
	_, rows := v.Screen().Size()
	var b []byte
	if cy > 0 {
		b = append(b, "\x1b["+strconv.Itoa(cy)+"S"...) // scroll the cursor's line to the top
	}
	if rows > 1 {
		b = append(b, "\x1b[2;1H\x1b[J"...) // erase below it
	}
	b = append(b, "\x1b[1;"+strconv.Itoa(cx+1)+"H"...)
	v.Write(b)
	v.Screen().ClearScrollback()
}
