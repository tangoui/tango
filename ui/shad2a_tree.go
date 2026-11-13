package ui

// shadcn/ui look of the tree-shaped selects (TreeSelect, Cascader) and the
// helpers they share: the bound-value form of the checked strategy, path
// lookups and the "size" default.
//
// shadcn has no tree select or cascader; both follow its Combobox /
// Command language: popover panel with p-1, items rounded-sm px-2 py-1.5
// (32px) whose accent background follows the pointer and keyboard, the
// selection marked by a 16px check on the right, 16px chevrons in the
// muted foreground and thin border-colored column separators.

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shadTreeItemH is the height of a shadcn tree / cascader item
// (py-1.5 + 20px line): the shadcn TreeToken.DropdownNodeHeight and
// CascaderToken.OptionHeight.
const shadTreeItemH = 32

// shadTreeIcon is the lucide icon size (size-4) of chevrons and checks.
const shadTreeIcon = 16

// shad2aTreeSize applies a "size" default given as a ControlSize or as an
// Ant size name ("small", "middle"/"default", "large").
func shad2aTreeSize(c *Context, comp string, dst *ControlSize) {
	v, ok := c.ConfigProp(comp, "size")
	if !ok {
		return
	}
	switch x := v.(type) {
	case ControlSize:
		*dst = x
	case string:
		switch strings.ToLower(x) {
		case "small", "sm":
			*dst = Small
		case "middle", "default", "medium":
			*dst = Middle
		case "large", "lg":
			*dst = Large
		}
	}
}

// shadTreeLook is what a shadcn tree / cascader item paints. Horizontal
// positions (x, chevronAt) are given as in a left-to-right row; in RTL the
// whole item is drawn mirrored.
type shadTreeLook struct {
	label     string
	query     string // highlighted substring ("" none)
	x         float32
	highlight bool // accent background (pointer, keyboard, expanded path)
	disabled  bool
	chevron   bool // has children: chevron at x (TreeSelect) or at the end edge (Cascader)
	chevronAt float32
	chevronR  bool // the chevron sits at the end edge (Cascader): no check mark beside it
	expanded  bool // TreeSelect: chevron turned down
	loading   bool
	checkbox  bool
	checked   bool
	half      bool
	selected  bool // check mark at the end
	padR      float32
	hiBg      theme.Color // accent background of a highlighted item
	hiCol     theme.Color // label of a highlighted item
	muted     theme.Color // chevron / spinner
}

// shadPaintTreeItem draws one shadcn item row in r.
func shadPaintTreeItem(c *Context, th *theme.Theme, l *paint.List, r geom.Rect, s shadTreeLook) {
	rtl := c.IsRTL()
	mx := func(b geom.Rect) geom.Rect { return treeMirror(b, r, rtl) }
	col := th.Text
	if s.highlight && !s.disabled {
		l.Fill(r, geom.Rad(th.RadiusSM), s.hiBg)
		col = s.hiCol
	}
	if s.disabled {
		col = th.TextDisabled
	}
	ic := float32(shadTreeIcon)
	iy := r.Y + (r.H-ic)/2
	muted := s.muted
	if s.disabled {
		muted = th.TextDisabled
	}
	switch {
	case s.loading:
		sz := float32(14)
		p2bSpin(c, l, mx(geom.Rect{X: s.chevronAt + (ic-sz)/2, Y: r.Y + (r.H-sz)/2, W: sz, H: sz}), muted)
	case s.chevron:
		ang := 0.0
		if s.expanded {
			ang = math.Pi / 2
		}
		// chevron-right mirrors to chevron-left in RTL; turned down it
		// stays down.
		c.paintIcon(l, icon.Right, mx(geom.Rect{X: s.chevronAt, Y: iy, W: ic, H: ic}), muted, ang)
	}
	x := s.x
	if s.checkbox {
		bs := c.boxSize()
		drawCheckbox(c, l, mx(geom.Rect{X: x, Y: r.Y + (r.H-bs)/2, W: bs, H: bs}), checkLook{checked: s.checked, indeterminate: s.half, disabled: s.disabled, t: 1})
		x += bs + 8
	}
	maxW := r.Right() - s.padR - x
	if q := strings.TrimSpace(s.query); q != "" {
		low := strings.ToLower(s.label)
		if k := strings.Index(low, strings.ToLower(q)); k >= 0 && len(low) == len(s.label) {
			// The match is set in the foreground at medium weight.
			if rtl {
				treeMarkedText(l, r, x, s.label, k, k+len(q), c.style(th.FontSize, text.Regular), c.style(th.FontSize, text.Semibold), 0, treeMid, col, col, nil)
			} else {
				x += entText(c, l, s.label[:k], x, r, th.FontSize, text.Regular, col, 0)
				x += entText(c, l, s.label[k:k+len(q)], x, r, th.FontSize, text.Semibold, col, 0)
				entText(c, l, s.label[k+len(q):], x, r, th.FontSize, text.Regular, col, 0)
			}
			maxW = -1
		}
	}
	if maxW >= 0 {
		treeText(c, l, r, x, s.label, c.style(th.FontSize, text.Regular), max(1, maxW), treeMid, col)
	}
	if s.selected && !(s.chevronR && (s.chevron || s.loading)) {
		b := mx(geom.Rect{X: r.Right() - 8 - ic, Y: iy, W: ic, H: ic})
		drawIcon(l, icon.Check, b.X, iy, ic, col)
	}
}

// shadTree reports the shadcn flavor.
func shadTree(th *theme.Theme) bool { return th.Flavor == theme.StyleShadcn }

// ---------------------------------------------------------------------------
// Checked strategy as the bound value (StrategyValue)

// tsExpandValues maps strategy values (parents standing for their
// subtrees) to the checked leaves; unknown values are kept.
func tsExpandValues(nodes []TreeSelectNode, vals []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range vals {
		nd := tsFind(nodes, v)
		if nd == nil {
			add(v)
			continue
		}
		ls := tsLeaves(nd, true, nil)
		if len(ls) == 0 {
			ls = tsLeaves(nd, false, nil)
		}
		for _, l := range ls {
			add(l)
		}
	}
	return out
}

// casFindPath returns the option at a value path (loaded lazy children
// included).
func (w *CascaderW) casFindPath(path []string) (CascaderOption, bool) {
	opts := w.opts
	var o CascaderOption
	for i, v := range path {
		var k int
		o, k = casFind(opts, v)
		if k < 0 {
			return CascaderOption{}, false
		}
		if i < len(path)-1 {
			opts = w.kids(o, path[:i+1])
		}
	}
	return o, len(path) > 0
}

// casExpandPaths maps strategy paths (parents standing for their subtrees)
// to the checked leaf paths; unknown paths are kept.
func (w *CascaderW) casExpandPaths(paths [][]string) [][]string {
	seen := map[string]bool{}
	var out [][]string
	add := func(p []string) {
		if k := casKey(p); !seen[k] {
			seen[k] = true
			out = append(out, append([]string(nil), p...))
		}
	}
	for _, p := range paths {
		o, ok := w.casFindPath(p)
		if !ok {
			add(p)
			continue
		}
		ls := w.leaves(o, p, nil)
		if len(ls) == 0 {
			add(p)
			continue
		}
		for _, l := range ls {
			add(l)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Right-to-left helpers shared by Tree, VirtualTree, TreeSelect, Cascader
// and Transfer rows.

// treeMirrorX runs fn with the paint list mirrored around the vertical
// line x = cx when rtl is set, so a hand-drawn shape pointing along the
// reading direction (switcher carets) points the other way. With rtl unset
// fn draws untouched.
func treeMirrorX(l *paint.List, cx float32, rtl bool, fn func()) {
	if !rtl {
		fn()
		return
	}
	l.Transform(geom.Affine{A: -1, D: 1, E: 2 * cx})
	fn()
	l.Pop()
}

// treeMid as the y of treeText / treeMarkedText centers the line in r.
const treeMid = float32(-1 << 30)

// treeMirror reflects b inside the row box r when rtl is set: rows lay
// out their parts as in a left-to-right row and mirror them in RTL, which
// keeps the LTR arithmetic (and pixels) unchanged.
func treeMirror(b, r geom.Rect, rtl bool) geom.Rect {
	if rtl {
		return mirrorRect(b, r)
	}
	return b
}

// treeText shapes s on one line (ellipsized to maxW when maxW > 0) and
// draws it starting at x as in a left-to-right row of r (in RTL the text
// ends at the mirror of x, so it is start-aligned from the right), its
// glyph origin at y or vertically centered in r when y is treeMid. It returns
// the width. In LTR it draws exactly like entText.
func treeText(c *Context, l *paint.List, r geom.Rect, x float32, s string, st text.Style, maxW, y float32, col theme.Color) float32 {
	if s == "" {
		return 0
	}
	opt := text.Options{NoWrap: true}
	if maxW > 0 {
		opt.Ellipsis, opt.MaxWidth = true, maxW
	}
	lay := text.Shape(s, st, opt)
	if y == treeMid {
		y = r.Y + (r.H-lay.Height)/2
	}
	if c.IsRTL() {
		x = 2*r.X + r.W - x - lay.Width
	}
	l.Glyphs(geom.Pt(x, y), lay.Glyphs, col)
	return lay.Width
}

// treeMarkedText draws s on one line of an RTL row with the byte range
// [a, b) set in mark style and color, starting (at the right) at the
// mirror of the left-to-right x; y and r as for treeText. The line is
// shaped as a whole so the mark keeps its place in bidirectional text (the
// piecewise drawing of the LTR paths would reorder an English title in an
// RTL row). bg, when set, receives the boxes behind the mark (x and width
// set; y and height are the caller's).
func treeMarkedText(l *paint.List, r geom.Rect, x float32, s string, a, b int, base, mark text.Style, maxW, y float32, col, mcol theme.Color, bg func(geom.Rect)) float32 {
	ra := utf8.RuneCountInString(s[:a])
	rb := ra + utf8.RuneCountInString(s[a:b])
	n := rb + utf8.RuneCountInString(s[b:])
	var runs []text.Run
	for _, rn := range []text.Run{{Start: 0, End: ra, Style: base}, {Start: ra, End: rb, Style: mark}, {Start: rb, End: n, Style: base}} {
		if rn.End > rn.Start {
			runs = append(runs, rn)
		}
	}
	opt := text.Options{NoWrap: true}
	if maxW > 0 {
		opt.Ellipsis, opt.MaxWidth = true, maxW
	}
	lay := text.ShapeRuns(s, runs, opt)
	if y == treeMid {
		y = r.Y + (r.H-lay.Height)/2
	}
	org := geom.Pt(2*r.X+r.W-x-lay.Width, y)
	if bg != nil {
		for _, sr := range lay.SelectionRects(ra, rb) {
			bg(geom.Rect{X: org.X + sr.X, W: sr.W})
		}
	}
	in := func(g int) bool {
		k := int(lay.GlyphCluster[g])
		return k >= ra && k < rb
	}
	for i := 0; i < len(lay.Glyphs); {
		j, m := i+1, in(i)
		for j < len(lay.Glyphs) && in(j) == m {
			j++
		}
		cc := col
		if m {
			cc = mcol
		}
		l.Glyphs(org, lay.Glyphs[i:j], cc)
		i = j
	}
	return lay.Width
}
