package ui

import (
	"fmt"
	"image"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Sender (Ant Design X Sender + Attachments + Suggestion)

// ChatAttachment is a file attached to a message being composed.
type ChatAttachment struct {
	_ [0]func()

	Name    string
	Size    int64       // bytes (0: not shown)
	Image   image.Image // thumbnail of an image (nil: file icon)
	Status  string      // "" ready, "uploading" or "error"
	Percent float32     // upload progress 0..100 while uploading
}

type senderTrigger struct {
	prefix string
	src    func(query string) []Option
}

type senderState struct {
	edit     editState
	pop      entPopup
	active   int
	skip     int // trigger position dismissed with Escape (-1 none)
	init     bool
	composed bool // an IME composition was active last frame
	lastCur  int
	lastLen  int
}

// SenderW is the chat input: an auto-growing multi-line editor where
// Enter sends and Shift+Enter inserts a line break, with an attachment
// strip, @ / command suggestions, a toolbar and a send / stop button.
type SenderW struct {
	_ [0]func()

	c           *Context
	frame, ed   *node
	sb          *ScrollBox
	st          *senderState
	z           *lazy
	loc         *Locale
	val         *string
	placeholder string
	name        string
	minR, maxR  int
	maxLen      int
	loading     bool
	disabled    bool
	files       *[]ChatAttachment
	attach      bool
	triggers    []senderTrigger
	toolbar     func()
	header      func()

	frameIA, edIA Interaction
	optIDs        []ID
	listID        ID

	submitted, stopped, attachClicked, changed bool
	picked                                     Option
	pickedPrefix                               string
	removed                                    int
}

// Sender adds a chat input bound to *val. Read Submitted each frame and
// clear *val after sending:
//
//	s := ui.Sender(c, &draft).Loading(busy).Commands(cmds)
//	if s.Submitted() { send(draft); draft = "" }
//	if s.Stopped() { cancel() }
func Sender(c *Context, val *string) *SenderW {
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	w := &SenderW{c: c, val: val, minR: 1, maxR: 8, loc: c.Locale(), removed: -1}
	w.placeholder = w.loc.Chat.Placeholder
	w.frame = c.add("sender", "")
	w.frame.ln.Dir = layout.Column
	w.frame.ln.Padding = geom.All(tk.SenderPadding)
	w.frame.ln.Gap = 8
	w.st = State[senderState](c, hashID(w.frame.id, "st"), nil)
	if !w.st.init {
		w.st.init, w.st.skip, w.st.lastCur = true, -1, -1
	}
	w.frame.paint = w.paintFrame
	w.frameIA = c.interact(w.frame, false, input.CursorText)
	w.z = c.lazy(w.frame, w.build)
	return w
}

// Placeholder replaces the default placeholder.
func (w *SenderW) Placeholder(s string) *SenderW { w.placeholder = s; return w }

// AutoSize sets the visible rows the editor grows between (default 1..8).
func (w *SenderW) AutoSize(minRows, maxRows int) *SenderW {
	w.minR, w.maxR = max(1, minRows), max(max(1, minRows), maxRows)
	return w
}

// MaxLength limits the number of characters.
func (w *SenderW) MaxLength(n int) *SenderW { w.maxLen = n; return w }

// Loading shows the stop button instead of the send button; Enter does
// not send and Escape (or the button) reports Stopped.
func (w *SenderW) Loading(on bool) *SenderW { w.loading = on; return w }

// Disabled disables the whole input.
func (w *SenderW) Disabled(on bool) *SenderW {
	if on {
		w.disabled = true
		w.frame.disabled = true
		w.frame.cursor = input.CursorNotAllowed
	}
	return w
}

// Attachments shows *files as removable chips above the editor; a click on
// a chip's remove button deletes it from the slice (see Removed).
func (w *SenderW) Attachments(files *[]ChatAttachment) *SenderW { w.files = files; return w }

// AttachButton shows a paperclip button in the toolbar (see
// AttachClicked; the application opens its file dialog).
func (w *SenderW) AttachButton() *SenderW { w.attach = true; return w }

// Mentions suggests opts after '@' (filtered by what follows it).
func (w *SenderW) Mentions(opts []Option) *SenderW { return w.Suggest("@", filterOpts(opts)) }

// Commands suggests opts after '/' (shown as "/value label").
func (w *SenderW) Commands(opts []Option) *SenderW { return w.Suggest("/", filterOpts(opts)) }

// Suggest opens a suggestion list when prefix is typed at the start of the
// text or after a space; src returns the options for the text typed after
// it so far. The chosen option's Value replaces the query.
func (w *SenderW) Suggest(prefix string, src func(query string) []Option) *SenderW {
	w.triggers = append(w.triggers, senderTrigger{prefix, src})
	return w
}

// Toolbar adds controls to the start of the bottom row (model select,
// mode switch …); the send button stays at the end.
func (w *SenderW) Toolbar(fn func()) *SenderW { w.toolbar = fn; return w }

// Header adds content at the top of the input (quoted message, context
// chips).
func (w *SenderW) Header(fn func()) *SenderW { w.header = fn; return w }

// A11yLabel sets the accessible name of the editor (default: placeholder).
func (w *SenderW) A11yLabel(s string) *SenderW { w.name = s; return w }

// Layout applies layout options to the outer frame.
func (w *SenderW) Layout(opts ...layout.Option) *SenderW {
	flexbox.Apply(&w.frame.ln.Style, opts...)
	return w
}

// Submitted reports Enter or a click on the send button with something to
// send (text or attachments).
func (w *SenderW) Submitted() bool { w.z.run(); return w.submitted }

// Stopped reports a click on the stop button (or Escape) while loading.
func (w *SenderW) Stopped() bool { w.z.run(); return w.stopped }

// AttachClicked reports a click on the attach button.
func (w *SenderW) AttachClicked() bool { w.z.run(); return w.attachClicked }

// Removed returns the index of the attachment removed this frame.
func (w *SenderW) Removed() (int, bool) { w.z.run(); return w.removed, w.removed >= 0 }

// Changed reports an edit of the text this frame.
func (w *SenderW) Changed() bool { w.z.run(); return w.changed }

// Picked returns the suggestion inserted this frame and its prefix.
func (w *SenderW) Picked() (prefix string, o Option, ok bool) {
	w.z.run()
	return w.pickedPrefix, w.picked, w.pickedPrefix != ""
}

// ID returns the editor's ID (focus it with c.SetFocus).
func (w *SenderW) ID() ID { return hashID(w.frame.id, "editor") }

func (w *SenderW) semantics(s *a11y.Node) {
	s.Value, s.Placeholder = *w.val, w.placeholder
	if s.Name == "" {
		s.Name = w.placeholder
	}
	if w.st.pop.open && w.listID != 0 {
		s.Controls = a11y.NodeID(w.listID)
		if a := w.st.active; a >= 0 && a < len(w.optIDs) {
			s.ActiveDescendant = a11y.NodeID(w.optIDs[a])
		}
	}
}

func (w *SenderW) font() text.Style { return w.c.style(w.c.Theme.FontSize, text.Regular) }

func (w *SenderW) lineH() float32 { return w.c.Theme.LineHeightFor(w.c.Theme.FontSize) }

func (w *SenderW) measureText(maxW, _ float32) geom.Size {
	if geom.IsInf(maxW) {
		maxW = 200
	}
	h := text.Shape(*w.val, w.font(), text.Options{MaxWidth: maxW}).Height
	return geom.Size{W: maxW, H: max(w.lineH(), h)}
}

func (w *SenderW) rows() int {
	wid := float32(200)
	if r, ok := w.c.rects[w.ID()]; ok && r.W > 0 {
		wid = r.W
	}
	lh := w.lineH()
	h := text.Shape(*w.val, w.font(), text.Options{MaxWidth: wid}).Height
	n := int((max(h, lh) + lh/2) / lh)
	return max(w.minR, min(n, w.maxR))
}

// canSend reports whether there is something to send.
func (w *SenderW) canSend() bool {
	return !w.disabled && !w.loading && (strings.TrimSpace(*w.val) != "" || (w.files != nil && len(*w.files) > 0))
}

func (w *SenderW) build() {
	c, st := w.c, w.st
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	if w.header != nil {
		Column(c, w.header, layout.Gap(6))
	}
	if w.files != nil && len(*w.files) > 0 {
		w.buildFiles(th, tk)
	}
	view := float32(w.rows()) * w.lineH()
	w.sb = Scroll(c, func() {
		// The editor's ID is derived from the frame so that ID() works
		// before the lazy build.
		w.ed = &node{id: w.ID(), parent: c.cur, disabled: c.disabledDepth > 0}
		w.ed.ln.Ref = w.ed
		c.stampEnv(w.ed)
		c.cur.kids = append(c.cur.kids, w.ed)
		w.ed.ln.Measure = w.measureText
	}, layout.H(view))
	if w.name != "" {
		w.ed.setName(w.name)
	}
	w.ed.ln.MinH = view
	w.ed.paint = w.paintText
	w.ed.ownKeys = true
	w.ed.setRole(a11y.RoleTextbox).setState(a11y.Multiline).fill(w)
	if len(w.triggers) > 0 {
		w.ed.setState(a11y.HasPopup)
	}
	if w.disabled {
		w.ed.disabled = true
	}
	w.edIA = c.interact(w.ed, true, input.CursorText)
	w.buildFooter(th, tk)
	if w.disabled {
		st.pop.open = false
		return
	}
	w.edit()
}

// edit handles keys, the suggestion popup and text editing.
func (w *SenderW) edit() {
	c, st := w.c, w.st
	id := w.ed.id
	if w.frameIA.PressedNow && !w.edIA.PressedNow {
		n := utf8.RuneCountInString(*w.val)
		st.edit.cur, st.edit.anc, st.edit.wasFocused = n, n, true
		c.SetFocus(id)
	}
	composing := c.focus == id && c.composing != ""
	recent := st.composed // the Enter that ends a composition is not a send
	st.composed = composing
	pos, tr, q := w.trigger()
	opts := w.options(tr, q)
	if st.pop.open {
		for _, k := range msTakeKeys(c, id, func(k input.KeyEvent) bool {
			return !composing && (k.Key == input.KeyUp || k.Key == input.KeyDown || k.Key == input.KeyEnter || k.Key == input.KeyTab)
		}) {
			switch k.Key {
			case input.KeyUp, input.KeyDown:
				st.active = msStep(opts, st.active, k.Key == input.KeyDown)
			default:
				if st.active >= 0 && st.active < len(opts) {
					w.insert(pos, tr, opts[st.active])
				}
			}
		}
	}
	send := false
	for range msTakeKeys(c, id, func(k input.KeyEvent) bool {
		return k.Key == input.KeyEnter && !k.Mods.Has(input.ModShift) && !k.Mods.Has(input.ModAlt)
	}) {
		if !composing && !recent && w.canSend() {
			send = true
		}
	}
	if w.loading && c.focus == id && !st.pop.open && c.Escape() {
		w.stopped = true
	}
	st.edit.hints.Enter = input.EnterSend
	res := c.processEdit(id, &st.edit, w.val, editConfig{multiline: true, maxLen: w.maxLen}, w.edIA)
	if res.changed {
		w.changed = true
	}
	w.reveal()
	prev := pos
	pos, tr, q = w.trigger()
	if pos != prev {
		st.active = 0
	}
	if st.pop.open && c.Dismissed(st.pop.panel, w.frame.id) {
		st.skip = pos
		c.invalid = true
	}
	if pos < 0 {
		st.skip = -1
	}
	opts = w.options(tr, q)
	st.pop.open = pos >= 0 && pos != st.skip && c.focus == id && len(opts) > 0
	if st.active >= len(opts) {
		st.active = 0
	}
	if st.pop.open {
		w.popup(pos, tr, opts)
		c.CaptureTab(id) // Tab picks the highlighted suggestion
	}
	if send {
		w.submitted = true
		st.pop.open = false
	}
}

// reveal keeps the caret inside the editor's scroll view.
func (w *SenderW) reveal() {
	c, st := w.c, w.st
	e := &st.edit
	n := len(*w.val)
	if c.focus != w.ed.id || (e.cur == st.lastCur && n == st.lastLen) {
		st.lastCur, st.lastLen = e.cur, n
		return
	}
	st.lastCur, st.lastLen = e.cur, n
	wid := float32(200)
	if r, ok := c.rects[w.ed.id]; ok && r.W > 0 {
		wid = r.W
	}
	lay := text.Shape(*w.val, w.font(), text.Options{MaxWidth: wid})
	_, top, h := lay.Caret(clampIdx(e.cur, utf8.RuneCountInString(*w.val)))
	view := float64(w.sb.n.ln.Height)
	s := w.sb.s
	y := s.off.Y
	if float64(top) < y {
		y = float64(top)
	}
	if float64(top+h) > y+view {
		y = float64(top+h) - view
	}
	if y != s.off.Y {
		s.off.Y = max(0, y)
		c.invalid = true
	}
}

// trigger finds a suggestion prefix before the caret: at the start of the
// text or after white space, with no white space between it and the caret.
func (w *SenderW) trigger() (pos int, tr *senderTrigger, query string) {
	if len(w.triggers) == 0 {
		return -1, nil, ""
	}
	rs := []rune(*w.val)
	cur := clampIdx(w.st.edit.cur, len(rs))
	if w.st.edit.hasSel() {
		return -1, nil, ""
	}
	i := cur
	for i > 0 && !unicode.IsSpace(rs[i-1]) {
		i--
	}
	if i >= cur {
		return -1, nil, ""
	}
	for k := range w.triggers {
		p := []rune(w.triggers[k].prefix)
		if len(p) > 0 && cur-i >= len(p) && string(rs[i:i+len(p)]) == string(p) {
			return i, &w.triggers[k], string(rs[i+len(p) : cur])
		}
	}
	return -1, nil, ""
}

func (w *SenderW) options(tr *senderTrigger, q string) []Option {
	if tr == nil || tr.src == nil {
		return nil
	}
	return tr.src(q)
}

func (w *SenderW) insert(pos int, tr *senderTrigger, o Option) {
	if pos < 0 || tr == nil || o.Disabled {
		return
	}
	rs := []rune(*w.val)
	cur := clampIdx(w.st.edit.cur, len(rs))
	ins := []rune(tr.prefix + o.Value + " ")
	nr := append(append(append([]rune(nil), rs[:pos]...), ins...), rs[cur:]...)
	*w.val = string(nr)
	w.st.edit.cur = pos + len(ins)
	w.st.edit.anc = w.st.edit.cur
	w.st.pop.open = false
	w.changed = true
	w.picked, w.pickedPrefix = o, tr.prefix
	w.c.SetFocus(w.ed.id)
	w.c.invalid = true
}

// popup shows the suggestions above the input (chat inputs sit at the
// bottom of the window).
func (w *SenderW) popup(pos int, tr *senderTrigger, opts []Option) {
	c, st := w.c, w.st
	th := c.Theme
	label := func(o Option) string {
		s := tr.prefix + o.Value
		if o.Label != "" && o.Label != o.Value {
			s += "  " + o.Label
		}
		return s
	}
	ov := Overlay(c, OverlayOptions{Anchor: w.frame.id, Place: TopStart, Offset: 6, Flip: true}, func() {
		msOptionList(c, len(opts), func(i int) (string, string, bool) {
			return opts[i].Value, label(opts[i]), opts[i].Disabled
		}, &st.active, nil, &w.optIDs, func(i int, l *paint.List, r geom.Rect) {
			w.paintOption(l, r, tr.prefix, opts[i], st.active == i)
		}, func(i int) { w.insert(pos, tr, opts[i]) })
	}, layout.MinW(240))
	entStylePanel(c, ov)
	ov.Style(layout.Padding(entListPad(c)))
	ov.Opacity(c.AnimFrom(w.frame.id, "pop", 0, 1, th.DurationMid))
	ov.n.setRole(a11y.RoleListbox).setName(w.loc.Chat.Suggestions)
	st.pop.panel = ov.n.id
	w.listID = ov.n.id
}

// paintOption draws "prefix+value" and, when different, the label after it.
func (w *SenderW) paintOption(l *paint.List, r geom.Rect, prefix string, o Option, active bool) {
	c := w.c
	main := prefix + o.Value
	if o.Label == "" || o.Label == o.Value {
		entPaintOption(c, l, r, main, entOptState{active: active, disabled: o.Disabled})
		return
	}
	entPaintOption(c, l, r, main, entOptState{active: active, disabled: o.Disabled})
	th := c.Theme
	fs := th.FontSize
	x := entTextW(c, main, fs, text.Regular) + 12 + 16
	s := c.inline(r)
	avail := r.W - x - 12
	if avail <= 8 {
		return
	}
	lay := text.Shape(o.Label, c.style(fs, text.Regular), c.textOpts(text.Options{NoWrap: true, MaxWidth: avail, Ellipsis: true}))
	l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.TextTertiary)
}

func (w *SenderW) buildFiles(th *theme.Theme, tk *theme.ChatToken) {
	c := w.c
	files := *w.files
	ScrollH(c, func() {
		for i := range files {
			f := files[i]
			Key(c, i, func() {
				chip := Row(c, nil, layout.Gap(8), layout.Middle(), layout.PadSE(4, 4, 4))
				chip.Bg(tk.AttachmentBg).Radius(tk.AttachmentRadius)
				switch {
				case f.Status == "error":
					chip.Border(1, th.Error.Border)
				case tk.AttachmentBorder.A > 0:
					chip.Border(1, tk.AttachmentBorder)
				}
				chip.n.setRole(a11y.RoleGroup).setName(f.Name)
				pct := f.Percent
				uploading := f.Status == "uploading"
				chip.n.over = func(l *paint.List, r geom.Rect) {
					if !uploading {
						return
					}
					bar := geom.Rect{X: r.X + 4, Y: r.Bottom() - 4, W: r.W - 8, H: 2}
					l.Fill(bar, geom.Rad(1), th.FillSecondary)
					s := c.inline(bar)
					l.Fill(s.Rect(0, bar.Y, bar.W*max(0, min(pct, 100))/100, 2), geom.Rad(1), th.Primary.Base)
				}
				c.body(chip.n, func() {
					ts := tk.AttachmentThumb
					thumb := c.add("sender-thumb", "")
					thumb.aHidden = true
					thumb.ln.Width, thumb.ln.Height = ts, ts
					img := f.Image
					thumb.paint = func(l *paint.List, r geom.Rect) {
						rad := geom.Rad(max(0, tk.AttachmentRadius-2))
						if img != nil {
							l.DrawImage(img, r, rad)
							return
						}
						l.Fill(r, rad, th.BgContainer)
						col := th.Primary.Base
						if f.Status == "error" {
							col = th.Error.Base
						}
						c.paintIcon(l, icon.File, geom.Rect{X: r.X + ts/4, Y: r.Y + ts/4, W: ts / 2, H: ts / 2}, col, 0)
					}
					Column(c, func() {
						Text(c, f.Name).Ellipsis().Layout(layout.MaxW(160))
						sub, col := "", th.TextTertiary
						switch f.Status {
						case "uploading":
							sub = w.loc.Chat.Uploading
							if pct > 0 {
								sub += fmt.Sprintf(" %d%%", int(pct))
							}
						case "error":
							sub, col = w.loc.Chat.Failed, th.Error.Base
						default:
							if f.Size > 0 {
								sub = chatSize(f.Size)
							}
						}
						if sub != "" {
							Text(c, sub).Size(th.FontSizeSM).Color(col).NoWrap()
						}
					}, layout.Gap(0), layout.MinW(0))
					if !w.disabled && IconButton(c, icon.Close, w.loc.RemoveItem(f.Name)).Size(20, 12).Clicked() {
						w.removed = i
						*w.files = append(append([]ChatAttachment(nil), files[:i]...), files[i+1:]...)
						c.invalid = true
					}
				})
			})
		}
	}, layout.Gap(8), layout.Padding(geom.Insets{Bottom: 2}))
}

// chatSize formats a byte count ("820 B", "1.4 KB", "3.2 MB").
func chatSize(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.1f KB", float64(n)/k)
	case n < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
}

func (w *SenderW) buildFooter(th *theme.Theme, tk *theme.ChatToken) {
	c := w.c
	loc := w.loc
	Row(c, func() {
		if w.attach {
			Tooltip(c, loc.Chat.Attach, func() {
				if IconButton(c, icon.Paperclip, loc.Chat.Attach).Size(tk.SendSize, 18).Clicked() && !w.disabled {
					w.attachClicked = true
				}
			})
		}
		if w.toolbar != nil {
			Row(c, w.toolbar, layout.Gap(8), layout.Middle(), layout.MinW(0), layout.Shrink(1))
		}
		Spacer(c)
		if w.loading {
			Tooltip(c, loc.Chat.Stop, func() {
				if Button(c, "").Primary().Circle().Icon(icon.Stop).A11yLabel(loc.Chat.Stop).Disabled(w.disabled).Clicked() {
					w.stopped = true
				}
			})
			return
		}
		b := Button(c, "").Primary().Circle().Icon(icon.Send).A11yLabel(loc.Chat.Send).Disabled(!w.canSend())
		if b.Clicked() && w.canSend() {
			w.submitted = true
		}
	}, layout.Gap(4), layout.Middle())
}

func (w *SenderW) paintFrame(l *paint.List, r geom.Rect) {
	c := w.c
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	rad := geom.Rad(tk.SenderRadius)
	focus := c.Anim(w.frame.id, "focus", b2f(c.focus == w.ID()), th.DurationMid)
	if len(tk.SenderShadow) > 0 && !w.disabled {
		l.Shadows(r, rad, tk.SenderShadow)
	}
	bg := tk.SenderBg
	if w.disabled {
		bg = th.BgContainerDisabled
	}
	l.Fill(r, rad, bg)
	border := theme.Lerp(tk.SenderBorder, tk.SenderFocusBorder, focus)
	l.Stroke(r, rad, 1, border)
	if focus > 0 && th.ControlOutline.A > 0 && th.ControlOutlineWidth > 0 {
		g := th.ControlOutlineOffset + th.ControlOutlineWidth/2
		l.Stroke(r.Expand(g), rad.Grow(g), th.ControlOutlineWidth, theme.Fade(th.ControlOutline, float64(focus)))
	}
}

func (w *SenderW) paintText(l *paint.List, r geom.Rect) {
	c := w.c
	th := c.Theme
	col := th.Text
	if w.disabled {
		col = th.TextDisabled
	}
	c.paintEditor(l, r, w.ed.id, &w.st.edit, *w.val, w.font(), col, w.placeholder, false, true)
}

// filterOpts returns a case-insensitive substring filter over opts' values
// and labels.
func filterOpts(opts []Option) func(q string) []Option {
	return func(q string) []Option {
		if q == "" {
			return opts
		}
		q = strings.ToLower(q)
		var out []Option
		for _, o := range opts {
			if strings.Contains(strings.ToLower(o.Value), q) || strings.Contains(strings.ToLower(o.Label), q) {
				out = append(out, o)
			}
		}
		return out
	}
}
