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
	// hadFocus: the editor had focus at the last build (TriggerOnFocus).
	hadFocus bool
	filt     msFilterCache // matches of a long list of static options
	textW    msTextW       // label widths of the static options
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
	onFocus   bool // TriggerOnFocus
	forced    bool
	changed   bool
	selected  string
	picked    bool
	z         *lazy
	optIDs    fillIDs // option nodes of the open list
	listID    ID
	place     Placement // dropdown placement (Ant placement, Element placement)
	notFound  string    // text of an empty dropdown ("" = close it, as Ant)
	virt      int       // Virtual: 0 auto (long lists), 1 always, -1 never
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
	v := FieldVariant("")
	selDefaults(c, "AutoComplete", &v, &w.place)
	if v != "" {
		w.in.Variant(string(v))
	}
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
		if id := w.optIDs.at(w.st.active); id != 0 {
			s.ActiveDescendant = a11y.NodeID(id)
		}
	}
}

func (w *AutoCompleteW) Placeholder(s string) *AutoCompleteW { w.in.Placeholder(s); return w }
func (w *AutoCompleteW) AllowClear() *AutoCompleteW          { w.in.AllowClear(); return w }
func (w *AutoCompleteW) Status(s string) *AutoCompleteW      { w.in.Status(s); return w }

// Variant sets the frame look (FieldOutlined by default; the inner
// Input's Variant).
func (w *AutoCompleteW) Variant(v FieldVariant) *AutoCompleteW { w.in.Variant(string(v)); return w }

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

// TriggerOnFocus opens the suggestions as soon as the input gains focus
// (Element trigger-on-focus), before anything is typed: the options (or
// Source's result) for the current text, all of them when it is empty.
// With Remote the callback runs at once for the current text.
func (w *AutoCompleteW) TriggerOnFocus() *AutoCompleteW { w.onFocus = true; return w }

// Highlight emphasises the matched part of each suggestion.
func (w *AutoCompleteW) Highlight() *AutoCompleteW { w.highlight = true; return w }

// Placement sets where the suggestions open: BottomStart (default),
// BottomEnd, TopStart or TopEnd (Ant placement, Element placement); they
// flip when there is no room. ConfigProvider.Defaults("AutoComplete",
// "placement") takes the Ant names.
func (w *AutoCompleteW) Placement(p Placement) *AutoCompleteW { w.place = p; return w }

// NotFoundContent keeps the dropdown open with text s when nothing
// matches (Ant notFoundContent; without it an empty list closes).
func (w *AutoCompleteW) NotFoundContent(s string) *AutoCompleteW { w.notFound = s; return w }

// Virtual forces the virtual suggestion list on or off; by default lists
// longer than 100 suggestions are virtual (only the visible rows and the
// keyboard row are built; the dropdown keeps the width, height and scroll
// range of the full list, but suggestions out of view are not in the
// accessibility tree). Labels, values and Disabled are always read live.
// A virtual list of static options keeps which options match while the
// options slice (identity and length) and the typed text stay the same,
// and measures label widths once per slice and theme: an option renamed
// in place may not join or leave the matches until the text or the slice
// changes, nor change the list's width until the slice (or the theme
// pointer) changes. Virtual(false) builds every row.
func (w *AutoCompleteW) Virtual(on bool) *AutoCompleteW { w.virt = p2bVirtualMode(on); return w }

// Open forces the dropdown open (screenshots, tests).
func (w *AutoCompleteW) Open() *AutoCompleteW { w.st.pop.open, w.forced = true, true; return w }

// IsOpen reports whether the dropdown (suggestions or the loading row) is
// shown.
func (w *AutoCompleteW) IsOpen() bool { w.z.run(); return w.shown }

// Changed reports a typed edit or a picked suggestion this frame.
func (w *AutoCompleteW) Changed() bool { w.z.run(); return w.changed }

// Selected returns the value picked this frame and whether one was picked.
func (w *AutoCompleteW) Selected() (string, bool) { w.z.run(); return w.selected, w.picked }

func (w *AutoCompleteW) options() msView {
	if w.source != nil {
		return msAll(w.source(*w.val))
	}
	if w.remote != nil {
		return msAll(w.opts) // already filtered by the server
	}
	q := strings.ToLower(*w.val)
	return w.st.filt.get(w.opts, *w.val, w.virt, func(o *Option) bool { return strings.Contains(strings.ToLower(o.Label), q) })
}

// busy reports whether the dropdown shows the loading row instead of opts.
func (w *AutoCompleteW) busy(opts msView) bool {
	return w.loading && (opts.len() == 0 || w.remote != nil)
}

// keepOpen reports whether an empty suggestion list may leave the popup
// open (remote results or a loading row may still arrive).
func (w *AutoCompleteW) keepOpen() bool { return w.remote != nil || w.loading || w.notFound != "" }

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
		st.pop.open = opts.len() > 0 || w.keepOpen()
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
	if (w.in.edIA.Clicked || w.in.boxIA.Clicked) && !st.pop.open && (opts.len() > 0 || w.loading || w.notFound != "") {
		st.pop.open = true
		st.active = 0
	}
	focused := c.focus == w.in.ed.id
	if w.onFocus && focused && !st.hadFocus && !typed {
		if w.remote != nil && !st.remote.pending {
			w.remote(*w.val)
			c.invalid = true
		}
		if opts.len() > 0 || w.keepOpen() {
			st.pop.open = true
			st.active = 0
		}
	}
	st.hadFocus = focused
	if c.focus != w.in.ed.id && c.focus != 0 && !w.forced {
		st.pop.open = false
	}
	if !w.forced {
		st.pop.dismiss(c, w.in.box.id)
	}
	list := opts // pickable suggestions (none behind the loading row)
	if w.busy(opts) {
		list = msView{}
	}
	for _, k := range w.keys {
		switch k.Key {
		case input.KeyDown, input.KeyUp:
			if !st.pop.open {
				st.pop.open = list.len() > 0
				st.active = 0
				continue
			}
			st.active = msStepBy(list.len(), st.active, k.Key == input.KeyDown, func(i int) bool { return list.at(i).Disabled })
		case input.KeyEnter:
			if st.pop.open && st.active >= 0 && st.active < list.len() && !list.at(st.active).Disabled {
				w.pick(list.at(st.active))
				opts = w.options()
				list = opts
				if w.busy(opts) {
					list = msView{}
				}
			}
		}
	}
	if opts.len() == 0 && !w.keepOpen() {
		st.pop.open = false
	}
	if st.active >= list.len() {
		st.active = 0
	}
	busy := w.busy(opts)
	if st.pop.open && (list.len() > 0 || busy || w.notFound != "") {
		w.shown = true
		w.dropdown(list, busy)
	} else {
		w.listID = 0
	}
}

func (w *AutoCompleteW) dropdown(opts msView, busy bool) {
	c, st := w.c, w.st
	q := *w.val
	loc := c.Locale()
	ov := selPanel(c, w.in.box.id, &st.pop, true, w.place, func() {
		if busy {
			entLoadingRow(c, loc.Loading, entOptionH(c))
			return
		}
		if opts.len() == 0 {
			Empty(c).Simple().Description(w.notFound)
			return
		}
		msOptionList(c, opts.len(), func(i int) (string, string, bool) {
			o := opts.at(i)
			return o.Value, o.Label, o.Disabled
		}, &st.active, func(i int) bool { return opts.at(i).Value == *w.val }, &w.optIDs, func(i int, l *paint.List, r geom.Rect) {
			o := opts.at(i)
			s := entOptState{active: st.active == i, disabled: o.Disabled}
			if !w.highlight || q == "" {
				entPaintOption(c, l, r, o.Label, s)
				return
			}
			entPaintOption(c, l, r, "", s)
			acPaintMatch(c, l, r, o.Label, q, o.Disabled)
		}, func(i int) { w.pick(opts.at(i)) }, w.virtual(opts))
	})
	ov.Style(layout.Padding(entListPad(c)))
	w.listID = 0
	if !busy && opts.len() > 0 { // the loading row is a status, not a list
		ov.n.setRole(a11y.RoleListbox).setName(popupName(c, w.in.ed, w.in.placeholder))
		w.listID = ov.n.id
	}
}

// virtual is the virtual mode of the suggestion list: the static
// options' matches keep their widest row per query, from label widths
// measured once per options slice; what Source or Remote return is
// measured every frame (their slices may be reused for new results).
func (w *AutoCompleteW) virtual(opts msView) msVirt {
	v := msVirt{mode: w.virt}
	if w.source == nil && w.remote == nil {
		v.key = opts.key(*w.val)
		v.textW = func(i int) float32 { return w.st.textW.of(w.c, w.opts)[opts.srcIndex(i)] }
	}
	return v
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
