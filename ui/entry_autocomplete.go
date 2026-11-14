package ui

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

type acState struct {
	pop    entPopup
	active int
	remote remoteSearch // debounce of Remote calls
}

// AutoCompleteW is a text input with a suggestion dropdown.
type AutoCompleteW struct {
	_ [0]func()

	c         *Context
	in        *InputW
	val       *string
	opts      []Option
	source    func(q string) []Option
	remote    func(query string)
	debounce  time.Duration
	loading   bool
	shown     bool // the dropdown was built this frame
	st        *acState
	keys      []input.KeyEvent
	highlight bool
	forced    bool
	changed   bool
	selected  string
	picked    bool
	z         *lazy
	optIDs    []ID // option nodes of the open list
	listID    ID
}

// AutoComplete adds an input bound to *val that suggests opts (filtered by
// the typed text). Picking a suggestion writes its Value.
func AutoComplete(c *Context, val *string, opts []Option) *AutoCompleteW {
	w := &AutoCompleteW{c: c, val: val, opts: opts, debounce: remoteDebounce}
	w.in = Input(c, val)
	w.st = State[acState](c, hashID(w.in.box.id, "ac"), nil)
	// Take navigation keys before the input's editor consumes them.
	open := w.st.pop.open
	w.keys = msTakeKeys(c, w.in.ed.id, func(k input.KeyEvent) bool {
		return k.Key == input.KeyUp || k.Key == input.KeyDown || (k.Key == input.KeyEnter && open)
	})
	// The editor is an editable combobox (InputW keeps value, placeholder,
	// status; this adds the popup relations).
	w.in.ed.setRole(a11y.RoleCombobox).setState(a11y.Expandable | a11y.HasPopup | a11y.Editable).fill(w)
	w.configDefaults()
	w.z = c.lazy(w.in.box, w.finish)
	return w
}

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *AutoCompleteW) A11yLabel(s string) *AutoCompleteW { w.in.A11yLabel(s); return w }

func (w *AutoCompleteW) semantics(s *a11y.Node) {
	if s.States.Has(a11y.Focusable) && !s.States.Has(a11y.ReadOnly) {
		s.Actions |= a11y.SetValue // typed text, like a textbox
	}
	if w.shown && w.listID != 0 {
		s.States |= a11y.Expanded
		s.Controls = a11y.NodeID(w.listID)
		if a := w.st.active; a >= 0 && a < len(w.optIDs) {
			s.ActiveDescendant = a11y.NodeID(w.optIDs[a])
		}
	}
}

func (w *AutoCompleteW) Placeholder(s string) *AutoCompleteW { w.in.Placeholder(s); return w }
func (w *AutoCompleteW) AllowClear() *AutoCompleteW          { w.in.AllowClear(); return w }
func (w *AutoCompleteW) Status(s string) *AutoCompleteW      { w.in.Status(s); return w }
func (w *AutoCompleteW) Size(s ControlSize) *AutoCompleteW   { w.in.Size(s); return w }
func (w *AutoCompleteW) Width(px float32) *AutoCompleteW     { w.in.Width(px); return w }
func (w *AutoCompleteW) Prefix(ic *icon.Icon) *AutoCompleteW { w.in.Prefix(ic); return w }
func (w *AutoCompleteW) Layout(o ...layout.Option) *AutoCompleteW {
	w.in.Layout(o...)
	return w
}
func (w *AutoCompleteW) Disabled(on bool) *AutoCompleteW { w.in.Disabled(on); return w }

// Source computes suggestions from the typed text (replaces filtering of
// the static options).
func (w *AutoCompleteW) Source(fn func(q string) []Option) *AutoCompleteW { w.source = fn; return w }

// Remote turns on server-side suggestions: fn receives the typed text
// after Debounce (300ms by default) on the UI thread. Start the request in
// the background, pass Loading(true) while it runs, then replace the
// options given to AutoComplete (they are shown as-is, not filtered by the
// typed text) and wake the window with Invalidate. A Source, when set,
// still computes the options.
//
//	ui.AutoComplete(c, &q, hits).Remote(func(s string) { go fetch(s) }).Loading(busy)
func (w *AutoCompleteW) Remote(fn func(query string)) *AutoCompleteW { w.remote = fn; return w }

// Debounce sets the delay before a Remote call.
func (w *AutoCompleteW) Debounce(d time.Duration) *AutoCompleteW { w.debounce = d; return w }

// Loading shows a spinner and the loading text in the dropdown (in place
// of the suggestions with Remote, or when there are none).
func (w *AutoCompleteW) Loading(on bool) *AutoCompleteW { w.loading = on; return w }

// Highlight emphasises the matched part of each suggestion.
func (w *AutoCompleteW) Highlight() *AutoCompleteW { w.highlight = true; return w }

// Open forces the dropdown open (screenshots, tests).
func (w *AutoCompleteW) Open() *AutoCompleteW { w.st.pop.open, w.forced = true, true; return w }

// IsOpen reports whether the dropdown (suggestions or the loading row) is
// shown.
func (w *AutoCompleteW) IsOpen() bool { w.z.run(); return w.shown }

// Changed reports a typed edit or a picked suggestion this frame.
func (w *AutoCompleteW) Changed() bool { w.z.run(); return w.changed }

// Selected returns the value picked this frame and whether one was picked.
func (w *AutoCompleteW) Selected() (string, bool) { w.z.run(); return w.selected, w.picked }

func (w *AutoCompleteW) options() []Option {
	if w.source != nil {
		return w.source(*w.val)
	}
	if w.remote != nil {
		return w.opts // already filtered by the server
	}
	return filterOptions(w.opts, *w.val, nil)
}

// busy reports whether the dropdown shows the loading row instead of opts.
func (w *AutoCompleteW) busy(opts []Option) bool {
	return w.loading && (len(opts) == 0 || w.remote != nil)
}

// keepOpen reports whether an empty suggestion list may leave the popup
// open (remote results or a loading row may still arrive).
func (w *AutoCompleteW) keepOpen() bool { return w.remote != nil || w.loading }

func (w *AutoCompleteW) pick(o Option) {
	c := w.c
	*w.val = o.Value
	n := len([]rune(o.Value))
	w.in.e.cur, w.in.e.anc = n, n
	w.changed, w.picked, w.selected = true, true, o.Value
	w.st.pop.open = false
	c.SetFocus(w.in.ed.id)
	c.invalid = true
}

func (w *AutoCompleteW) finish() {
	c, st := w.c, w.st
	if w.in.box.disabled {
		st.pop.open = false
		return
	}
	typed := w.in.Changed()
	opts := w.options()
	if typed {
		w.changed = true
		st.pop.open = len(opts) > 0 || w.keepOpen()
		st.active = 0
		if w.remote != nil {
			st.remote.schedule(c, w.debounce)
		}
	}
	st.remote.tick(c, func() {
		if w.remote != nil {
			w.remote(*w.val)
		}
	})
	if (w.in.edIA.Clicked || w.in.boxIA.Clicked) && !st.pop.open && (len(opts) > 0 || w.loading) {
		st.pop.open = true
		st.active = 0
	}
	if c.focus != w.in.ed.id && c.focus != 0 && !w.forced {
		st.pop.open = false
	}
	if !w.forced {
		st.pop.dismiss(c, w.in.box.id)
	}
	list := opts // pickable suggestions (none behind the loading row)
	if w.busy(opts) {
		list = nil
	}
	for _, k := range w.keys {
		switch k.Key {
		case input.KeyDown, input.KeyUp:
			if !st.pop.open {
				st.pop.open = len(list) > 0
				st.active = 0
				continue
			}
			st.active = msStep(list, st.active, k.Key == input.KeyDown)
		case input.KeyEnter:
			if st.pop.open && st.active >= 0 && st.active < len(list) && !list[st.active].Disabled {
				w.pick(list[st.active])
				opts = w.options()
				list = opts
				if w.busy(opts) {
					list = nil
				}
			}
		}
	}
	if len(opts) == 0 && !w.keepOpen() {
		st.pop.open = false
	}
	if st.active >= len(list) {
		st.active = 0
	}
	busy := w.busy(opts)
	if st.pop.open && (len(list) > 0 || busy) {
		w.shown = true
		w.dropdown(list, busy)
	} else {
		w.listID = 0
	}
}

func (w *AutoCompleteW) dropdown(opts []Option, busy bool) {
	c, st := w.c, w.st
	q := *w.val
	loc := c.Locale()
	ov := entPanel(c, w.in.box.id, &st.pop, true, func() {
		if busy {
			entLoadingRow(c, loc.Loading, entOptionH(c))
			return
		}
		msOptionList(c, len(opts), func(i int) (string, string, bool) {
			return opts[i].Value, opts[i].Label, opts[i].Disabled
		}, &st.active, func(i int) bool { return opts[i].Value == *w.val }, &w.optIDs, func(i int, l *paint.List, r geom.Rect) {
			o := opts[i]
			s := entOptState{active: st.active == i, disabled: o.Disabled}
			if !w.highlight || q == "" {
				entPaintOption(c, l, r, o.Label, s)
				return
			}
			entPaintOption(c, l, r, "", s)
			acPaintMatch(c, l, r, o.Label, q, o.Disabled)
		}, func(i int) { w.pick(opts[i]) })
	})
	ov.Style(layout.Padding(entListPad(c)))
	w.listID = 0
	if !busy { // the loading row is a status, not a list
		ov.n.setRole(a11y.RoleListbox).setName(popupName(c, w.in.ed, w.in.placeholder))
		w.listID = ov.n.id
	}
}

// acPaintMatch draws label in an option row with the first case-insensitive
// occurrence of q emphasised.
func acPaintMatch(c *Context, l *paint.List, r geom.Rect, label, q string, disabled bool) {
	th := c.Theme
	padL := float32(12)
	col := th.Text
	if th.Flavor == theme.Element {
		padL, col = 20, th.TextSecondary
	}
	if disabled {
		col = th.TextDisabled
	}
	hi := th.Primary.Base
	if th.Flavor == theme.Ant {
		hi = th.Error.Base
	}
	hw := text.Semibold
	if th.Flavor == theme.StyleShadcn {
		// Command item: px-2, the match only one weight step up.
		padL, hw = 8, text.Medium
	}
	i := strings.Index(strings.ToLower(label), strings.ToLower(q))
	x := r.X + padL
	if i < 0 || disabled || i+len(q) > len(label) {
		entTextIn(c, l, label, padL, r, th.FontSize, text.Regular, col, r.W-padL-12)
		return
	}
	if c.IsRTL() {
		acPaintMatchRTL(c, l, r, padL, label, i, i+len(q), col, hi, hw)
		return
	}
	x += entText(c, l, label[:i], x, r, th.FontSize, text.Regular, col, 0)
	x += entText(c, l, label[i:i+len(q)], x, r, th.FontSize, hw, hi, 0)
	entText(c, l, label[i+len(q):], x, r, th.FontSize, text.Regular, col, 0)
}

// acPaintMatchRTL draws the highlighted label of acPaintMatch in an RTL
// scope: shaped as one paragraph (so its pieces keep their bidi order)
// and right-aligned at padL from the inline start; the match is bytes
// [a, b) of label.
func acPaintMatchRTL(c *Context, l *paint.List, r geom.Rect, padL float32, label string, a, b int, col, hi theme.Color, hw text.Weight) {
	th := c.Theme
	ra, rb := utf8.RuneCountInString(label[:a]), utf8.RuneCountInString(label[:b])
	reg, em := c.style(th.FontSize, text.Regular), c.style(th.FontSize, hw)
	var runs []text.Run
	if ra > 0 {
		runs = append(runs, text.Run{Start: 0, End: ra, Style: reg})
	}
	match := int32(len(runs))
	runs = append(runs, text.Run{Start: ra, End: rb, Style: em})
	if n := utf8.RuneCountInString(label); rb < n {
		runs = append(runs, text.Run{Start: rb, End: n, Style: reg})
	}
	lay := text.ShapeRuns(label, runs, text.Options{NoWrap: true})
	pt := geom.Pt(c.inline(r).X(padL, lay.Width), r.Y+(r.H-lay.Height)/2)
	// Paint consecutive glyphs of the same run with one color.
	for g := 0; g < len(lay.Glyphs); {
		run := lay.GlyphRun[g]
		e := g + 1
		for e < len(lay.Glyphs) && lay.GlyphRun[e] == run {
			e++
		}
		gc := col
		if run == match {
			gc = hi
		}
		l.Glyphs(pt, lay.Glyphs[g:e], gc)
		g = e
	}
}
