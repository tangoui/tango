package ui

import (
	"strings"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// MarkdownEditor: a Markdown source editor with a formatting toolbar and a
// live preview (edit / split / preview modes).

// MarkdownEditorW is a Markdown editor bound to *src.
type MarkdownEditorW struct {
	_ [0]func()

	c           *Context
	b           *Box
	src         *string
	height      float32
	mode        string
	placeholder string
	changed     bool
	clicked     string
	z           *lazy
}

type moreMDEBody struct {
	bar  float32
	tool ID // last focused toolbar item (its Tab stop)
}

type moreMDEState struct {
	mode string
}

// MarkdownEditor adds an editor for *src (default 400px high, split view).
//
//	ui.MarkdownEditor(c, &doc).Height(480)
func MarkdownEditor(c *Context, src *string) *MarkdownEditorW {
	w := &MarkdownEditorW{c: c, src: src, height: 400, mode: "split", placeholder: c.Locale().MarkdownEditor.Placeholder}
	w.configDefaults() // Defaults("MarkdownEditor", ...): mode, placeholder
	w.b = c.box("mdeditor", layout.Column, nil)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

// Height sets the total height (toolbar included).
func (w *MarkdownEditorW) Height(px float32) *MarkdownEditorW { w.height = px; return w }

// Mode sets the initial view: "edit", "split" or "preview". The toolbar
// switches it afterwards.
func (w *MarkdownEditorW) Mode(m string) *MarkdownEditorW { w.mode = m; return w }

// Placeholder sets the empty-editor hint.
func (w *MarkdownEditorW) Placeholder(s string) *MarkdownEditorW { w.placeholder = s; return w }

// Layout applies layout options to the outer frame.
func (w *MarkdownEditorW) Layout(opts ...layout.Option) *MarkdownEditorW {
	flexbox.Apply(&w.b.n.ln.Style, opts...)
	return w
}

// Changed reports an edit this frame (typing, toolbar, task toggles).
func (w *MarkdownEditorW) Changed() bool { w.z.run(); return w.changed }

// ClickedLink returns a link clicked in the preview this frame.
func (w *MarkdownEditorW) ClickedLink() string { w.z.run(); return w.clicked }

// CurrentMode returns the active view.
func (w *MarkdownEditorW) CurrentMode() string {
	w.z.run()
	return State[moreMDEState](w.c, w.b.n.id, nil).mode
}

type moreMDTool struct {
	label string
	apply func(src string, a, b int) (string, int, int)
}

// moreMDShortcut maps a shortcut key to its tool index in moreMDTools.
func moreMDShortcut(k input.Key) int {
	switch k {
	case input.KeyB:
		return 1
	case input.KeyI:
		return 2
	case input.KeyE:
		return 4
	case input.KeyK:
		return 5
	}
	return -1
}

func moreMDTools(loc *MarkdownEditorLocale) []moreMDTool {
	wrap := func(pre, suf, ph string) func(string, int, int) (string, int, int) {
		return func(s string, a, b int) (string, int, int) { return MarkdownWrap(s, a, b, pre, suf, ph) }
	}
	line := func(p string) func(string, int, int) (string, int, int) {
		return func(s string, a, b int) (string, int, int) { return MarkdownLinePrefix(s, a, b, p) }
	}
	block := func(blk string) func(string, int, int) (string, int, int) {
		return func(s string, a, b int) (string, int, int) { return MarkdownInsertBlock(s, b, blk) }
	}
	return []moreMDTool{
		{loc.Heading, line("## ")},
		{loc.Bold, wrap("**", "**", loc.BoldSample)},
		{loc.Italic, wrap("*", "*", loc.ItalicSample)},
		{loc.Strike, wrap("~~", "~~", loc.StrikeSample)},
		{loc.Code, wrap("`", "`", "code")},
		{loc.Link, wrap("[", "](https://)", loc.LinkSample)},
		{loc.Quote, line("> ")},
		{loc.BulletList, line("- ")},
		{loc.OrderedList, line("1. ")},
		{loc.Task, line("- [ ] ")},
		{loc.CodeBlock, block("```\n\n```")},
		{loc.Table, block("| " + locFmt(loc.TableColumnFmt, 1) + " | " + locFmt(loc.TableColumnFmt, 2) + " |\n| --- | --- |\n|  |  |")},
		{loc.Divider, block("---")},
	}
}

func (w *MarkdownEditorW) build() {
	c, th := w.c, w.c.Theme
	st := State[moreMDEState](c, w.b.n.id, func() moreMDEState { return moreMDEState{mode: w.mode} })
	if st.mode != "edit" && st.mode != "preview" {
		st.mode = "split"
	}
	var tool *moreMDTool
	mloc := &c.Locale().MarkdownEditor
	tools := moreMDTools(mloc)
	var items []*node // toolbar items: one Tab stop, arrow keys move
	Row(c, func() {
		Row(c, func() {
			Disabled(c, st.mode == "preview", func() {
				for i := range tools {
					b := Button(c, tools[i].label).Text().Small()
					items = append(items, b.n)
					if b.Clicked() {
						tool = &tools[i]
					}
				}
			})
		}, layout.Grow(1), layout.Basis(0), layout.Gap(2), layout.Middle(), layout.WrapItems())
		Row(c, func() {
			for _, m := range []struct{ id, label string }{{"edit", mloc.Edit}, {"split", mloc.Split}, {"preview", mloc.Preview}} {
				on := st.mode == m.id
				t := CheckableTag(c, m.label, &on)
				items = append(items, t.n)
				if t.Changed() {
					st.mode = m.id
					c.invalid = true
				}
			}
		}, layout.Gap(4), layout.Middle())
	}, layout.MinH(41), layout.PadVH(4, 8), layout.Gap(8), layout.Middle()).Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.BorderSecondary)
	}).Semantics(a11y.RoleToolbar, c.Locale().A11y.Toolbar).A11yState(a11y.Horizontal)
	w.toolbarKeys(items)
	// The toolbar wraps on narrow editors; size the body from its last height.
	st2 := State[moreMDEBody](c, hashID(w.b.n.id, "body"), nil)
	bodyH := max(60, w.height-max(41, st2.bar)-2)
	Row(c, func() {
		if st.mode != "preview" {
			Column(c, func() {
				ta := TextArea(c, w.src).Placeholder(w.placeholder)
				// The editor frame replaces the text area's own border; the
				// text area fills the body and scrolls.
				ta.bare = true
				// Shortcuts (Cmd on Apple, Ctrl elsewhere): B bold, I italic,
				// K link, E inline code. Taken before the textarea reads keys.
				if c.focus == ta.ed.id {
					keep := c.keys[:0]
					for _, k := range c.keys {
						if k.Down && k.Mods.Shortcut() && !k.Mods.Has(input.ModShift) {
							if i := moreMDShortcut(k.Key); i >= 0 {
								tool = &tools[i]
								continue
							}
						}
						if !k.Down && k.Mods.Shortcut() && moreMDShortcut(k.Key) >= 0 {
							continue
						}
						keep = append(keep, k)
					}
					c.keys = keep
				}
				if ta.Changed() {
					w.changed = true
				}
				if tool != nil && !ta.box.disabled {
					e := &ta.st.edit
					a, b := e.sel()
					s, na, nb := tool.apply(*w.src, a, b)
					*w.src = s
					e.anc, e.cur = na, nb
					e.wasFocused = true // keep the selection instead of select-all on refocus
					c.SetFocus(ta.ed.id)
					w.changed = true
					c.invalid = true
				}
			}, layout.Grow(1), layout.Basis(0))
		}
		if st.mode == "split" {
			n := c.add("mde-split", "")
			n.ln.Width = 1
			n.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, th.BorderSecondary) }
		}
		if st.mode != "edit" {
			Scroll(c, func() {
				md := Markdown(c, *w.src)
				if u := md.ClickedLink(); u != "" {
					w.clicked = u
				}
				if i, ok := md.TaskToggled(); ok {
					*w.src = ToggleMarkdownTask(*w.src, i)
					w.changed = true
					c.invalid = true
				}
			}, layout.Grow(1), layout.Basis(0), layout.PadVH(12, 16))
		}
	}, layout.H(bodyH), layout.AlignItems(layout.AlignStretch))
	if len(w.b.n.kids) > 0 {
		if r, ok := c.rects[w.b.n.kids[0].id]; ok {
			if r.H != st2.bar {
				st2.bar = r.H
				c.WakeAt(c.now) // relayout the body with the new toolbar height
			}
		}
	}
	w.b.n.ln.Padding = geom.All(1) // keep children inside the border
	w.b.Border(1, th.Border).Radius(th.Radius).Clip().Bg(th.BgContainer)
	if th.Flavor == theme.Element {
		w.b.Border(1, th.BorderSecondary)
	}
}

// toolbarKeys makes the toolbar one Tab stop (the last focused item) whose
// arrow keys, Home and End move between the items.
func (w *MarkdownEditorW) toolbarKeys(items []*node) {
	c := w.c
	st := State[moreMDEBody](c, hashID(w.b.n.id, "body"), nil)
	ids := make([]ID, len(items))
	cur := -1
	for i, n := range items {
		ids[i] = n.id
		if n.id == st.tool {
			cur = i
		}
		if c.focus == n.id {
			st.tool = n.id
			cur = i
		}
	}
	rovingTabStop(items, cur)
	if to := rovingStep(c, ids, func(i int) bool { return items[i].focus && !items[i].disabled }, rovingHorizontal); to >= 0 {
		st.tool = ids[to]
		navFocus(c, ids[to])
	}
}

// ---------------------------------------------------------------------------
// Source edits used by the toolbar (rune indices; exported for custom
// toolbars and shortcuts). Each returns the new source and the new
// selection [a, b).

// MarkdownWrap surrounds the selection [a,b) with pre/suf, or inserts
// pre+placeholder+suf at a when nothing is selected; the inner text stays
// selected. Wrapping an already wrapped selection unwraps it.
func MarkdownWrap(src string, a, b int, pre, suf, placeholder string) (string, int, int) {
	rs := []rune(src)
	a, b = moreClampSel(len(rs), a, b)
	p, q := []rune(pre), []rune(suf)
	if a >= len(p) && b+len(q) <= len(rs) && string(rs[a-len(p):a]) == pre && string(rs[b:b+len(q)]) == suf && a < b {
		out := string(rs[:a-len(p)]) + string(rs[a:b]) + string(rs[b+len(q):])
		return out, a - len(p), b - len(p)
	}
	inner := rs[a:b]
	if a == b {
		inner = []rune(placeholder)
	}
	out := string(rs[:a]) + pre + string(inner) + suf + string(rs[b:])
	return out, a + len(p), a + len(p) + len(inner)
}

// MarkdownLinePrefix toggles prefix on every line touched by [a,b).
func MarkdownLinePrefix(src string, a, b int, prefix string) (string, int, int) {
	rs := []rune(src)
	a, b = moreClampSel(len(rs), a, b)
	start := a
	for start > 0 && rs[start-1] != '\n' {
		start--
	}
	end := b
	for end < len(rs) && rs[end] != '\n' {
		end++
	}
	lines := strings.Split(string(rs[start:end]), "\n")
	all := true
	for _, l := range lines {
		if !strings.HasPrefix(l, prefix) {
			all = false
		}
	}
	delta := 0
	for i, l := range lines {
		if all {
			lines[i] = strings.TrimPrefix(l, prefix)
			delta -= len([]rune(prefix))
		} else {
			lines[i] = prefix + l
			delta += len([]rune(prefix))
		}
	}
	mid := strings.Join(lines, "\n")
	out := string(rs[:start]) + mid + string(rs[end:])
	first := len([]rune(prefix))
	if all {
		first = -first
	}
	na := max(start, a+first)
	nb := max(na, b+delta)
	return out, na, nb
}

// MarkdownInsertBlock inserts block on its own lines at position at; the
// caret lands on the block's first empty line (or after it).
func MarkdownInsertBlock(src string, at int, block string) (string, int, int) {
	rs := []rune(src)
	at = max(0, min(at, len(rs)))
	pre := ""
	if at > 0 && rs[at-1] != '\n' {
		pre = "\n\n"
	} else if at > 1 && rs[at-2] != '\n' {
		pre = "\n"
	}
	post := "\n"
	if at < len(rs) && rs[at] != '\n' {
		post = "\n\n"
	}
	ins := pre + block + post
	out := string(rs[:at]) + ins + string(rs[at:])
	caret := at + len([]rune(pre))
	if k := strings.Index(block, "\n\n"); k >= 0 {
		caret += len([]rune(block[:k])) + 1
	} else {
		caret += len([]rune(block))
	}
	return out, caret, caret
}

func moreClampSel(n, a, b int) (int, int) {
	if a > b {
		a, b = b, a
	}
	return max(0, min(a, n)), max(0, min(b, n))
}
