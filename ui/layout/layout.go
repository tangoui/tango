// Package layout is the flexbox-style layout solver of the Tango self-drawn
// UI: Row/Column with Grow/Shrink/Basis, Gap, Padding, Margin, Align,
// Justify and Wrap, plus Stack (children overlap) and absolute positioning
// for overlays. Sizes are measured on both axes: leaves expose a Measure
// callback that receives the available width and height (text wraps to the
// width it is given).
//
// Deliberate difference from Viva's solver: an item with Grow > 0 and no
// explicit Basis is distributed from a basis of 0 (CSS `flex: 1`) on both
// axes, so equal Grow factors give equal sizes even in a Column.
//
// API tiers: only the symbols listed in api/stable/ui_layout.txt are covered
// by the compatibility promise of docs/release.md ("API 稳定性"); every
// other exported symbol is Experimental and may change in any 0.x
// release. The API reference on the documentation site marks the
// experimental symbols.
package layout

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// Direction is the main axis of a container.
type Direction uint8

const (
	Column Direction = iota // children top to bottom (default)
	Row                     // children left to right
	Stack                   // children overlap, aligned in both axes
)

// Align positions items on the cross axis.
type Align uint8

const (
	AlignAuto    Align = iota // AlignSelf: inherit; Align: same as Stretch
	AlignStretch              // fill the cross axis
	AlignStart
	AlignCenter
	AlignEnd
)

// Justify distributes free space on the main axis.
type Justify uint8

const (
	JustifyStart Justify = iota
	JustifyCenter
	JustifyEnd
	JustifySpaceBetween
	JustifySpaceAround
	JustifySpaceEvenly
)

// Pos holds absolute offsets; NaN means unset.
type Pos struct{ Left, Top, Right, Bottom float32 }

// Unset is the value of an unset Pos edge.
var Unset = float32(math.NaN())

func set(v float32) bool { return !math.IsNaN(float64(v)) }

// Style is the layout input of a node. The zero value is an auto-sized
// Column with stretch alignment.
type Style struct {
	Dir                    Direction
	Width, Height          float32 // fixed border-box size; 0 = auto
	MinW, MinH, MaxW, MaxH float32 // 0 = none
	Grow, Shrink           float32
	Basis                  float32
	HasBasis               bool
	Padding                geom.Insets
	Margin                 geom.Insets
	Gap                    float32 // between items on the main axis
	LineGap                float32 // between wrapped lines
	Align                  Align   // cross alignment of children
	AlignSelf              Align
	Justify                Justify
	Wrap                   bool
	Absolute               bool // out of flow; positioned by Pos in the parent's box
	Pos                    Pos
	ScrollX, ScrollY       bool // content may exceed the box on this axis
	// NoStretch keeps the item's own cross size when the resolved alignment
	// is Stretch (it is then placed at the start). Buttons and tags use it
	// so they don't widen in columns.
	NoStretch bool
	// RTL mirrors the children's x inside this box after the logical (LTR)
	// solve: X' = span - X - W, span being the box width (or the content
	// width when ScrollX). Padding/Margin/Pos Left then act as the start
	// side. Set by the ui package from the scope's direction.
	RTL bool
	// NoMirror keeps this node at its physical position inside an RTL
	// parent (see Physical).
	NoMirror bool
}

// Node is a layout tree node. Callers fill Style/Children/Measure, call
// Solve, then read Rect (relative to the parent's border box) and Content.
type Node struct {
	Style
	Children []*Node
	// Measure returns the content size of a leaf given the available inner
	// width and height (either may be +Inf).
	Measure func(maxW, maxH float32) geom.Size
	Ref     any

	Rect    geom.Rect // output: border box relative to parent
	Content geom.Size // output: size of laid-out content incl. padding

	gen   uint32
	memo  [6]memoEntry
	nmemo int
}

type memoEntry struct {
	cw, ch float32
	fw, fh bool
	out    geom.Size
}

// Option mutates a Style; used by the ui package's container helpers.
type Option func(*Style)

// Apply runs opts on s.
func (s *Style) Apply(opts ...Option) {
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
}

var gen uint32

// Solve lays out root to exactly w×h.
func Solve(root *Node, w, h float32) {
	gen++
	root.Rect = geom.Rect{W: w, H: h}
	arrange(root, w, h)
}

// SolveAt lays out root in a box of up to w×h, sizing it to content on
// axes not forced (used for overlays). Returns the chosen size.
func SolveAt(root *Node, w, h float32, fw, fh bool) geom.Size {
	gen++
	sz := measure(root, w, h, fw, fh)
	root.Rect.W, root.Rect.H = sz.W, sz.H
	arrange(root, sz.W, sz.H)
	return sz
}

func clamp(v, lo, hi float32) float32 {
	if hi > 0 && v > hi {
		v = hi
	}
	if v < lo {
		v = lo
	}
	return v
}

// measure returns the border-box size of n. cw/ch are the available sizes;
// fw/fh force the size on that axis to exactly cw/ch.
func measure(n *Node, cw, ch float32, fw, fh bool) geom.Size {
	if n.gen != gen {
		n.gen = gen
		n.nmemo = 0
	}
	for i := 0; i < n.nmemo; i++ {
		m := n.memo[i]
		if m.cw == cw && m.ch == ch && m.fw == fw && m.fh == fh {
			return m.out
		}
	}
	out := measureUncached(n, cw, ch, fw, fh)
	if n.nmemo < len(n.memo) {
		n.memo[n.nmemo] = memoEntry{cw, ch, fw, fh, out}
		n.nmemo++
	} else {
		copy(n.memo[:], n.memo[1:])
		n.memo[len(n.memo)-1] = memoEntry{cw, ch, fw, fh, out}
	}
	return out
}

func measureUncached(n *Node, cw, ch float32, fw, fh bool) geom.Size {
	s := &n.Style
	w, h := float32(0), float32(0)
	knownW, knownH := false, false
	if fw {
		w, knownW = cw, true
	} else if s.Width > 0 {
		w, knownW = s.Width, true
	}
	if fh {
		h, knownH = ch, true
	} else if s.Height > 0 {
		h, knownH = s.Height, true
	}
	if knownW && !fw {
		w = clamp(w, s.MinW, s.MaxW)
	}
	if knownH && !fh {
		h = clamp(h, s.MinH, s.MaxH)
	}
	if knownW && knownH {
		return geom.Size{W: w, H: h}
	}
	availW := cw
	if knownW {
		availW = w
	} else if s.MaxW > 0 && s.MaxW < availW {
		availW = s.MaxW
	}
	availH := ch
	if knownH {
		availH = h
	} else if s.MaxH > 0 && s.MaxH < availH {
		availH = s.MaxH
	}
	innerW := max(0, availW-s.Padding.Horizontal())
	innerH := max(0, availH-s.Padding.Vertical())
	var content geom.Size
	if n.Measure != nil && len(n.Children) == 0 {
		content = n.Measure(innerW, innerH)
	} else {
		if s.ScrollX {
			innerW = geom.Inf
		}
		if s.ScrollY {
			innerH = geom.Inf
		}
		content = flex(n, innerW, innerH, knownW && !s.ScrollX, knownH && !s.ScrollY, false)
	}
	if !knownW {
		w = clamp(content.W+s.Padding.Horizontal(), s.MinW, s.MaxW)
	}
	if !knownH {
		h = clamp(content.H+s.Padding.Vertical(), s.MinH, s.MaxH)
	}
	return geom.Size{W: w, H: h}
}

// arrange positions n's children for a final size w×h.
func arrange(n *Node, w, h float32) {
	s := &n.Style
	innerW := max(0, w-s.Padding.Horizontal())
	innerH := max(0, h-s.Padding.Vertical())
	defW, defH := true, true
	if s.ScrollX {
		innerW, defW = geom.Inf, false
	}
	if s.ScrollY {
		innerH, defH = geom.Inf, false
	}
	var c geom.Size
	if len(n.Children) > 0 {
		c = flex(n, innerW, innerH, defW, defH, true)
	} else if n.Measure != nil && (s.ScrollX || s.ScrollY) {
		c = n.Measure(innerW, innerH)
	}
	n.Content = geom.Size{W: c.W + s.Padding.Horizontal(), H: c.H + s.Padding.Vertical()}
	if !s.ScrollX {
		n.Content.W = max(n.Content.W, w)
	}
	if !s.ScrollY {
		n.Content.H = max(n.Content.H, h)
	}
	// Absolute children relative to the padding box.
	for _, ch := range n.Children {
		if !ch.Absolute {
			continue
		}
		placeAbsolute(ch, w, h)
	}
	if s.RTL {
		mirror(n, w)
	}
}

// mirror flips the children's x inside an RTL box after the logical solve.
// Each child has already arranged its own subtree in its own coordinates,
// so only its Rect.X moves. Scrolling boxes flip inside the content width
// so the logical start ends up at the right edge of the scroll range.
func mirror(n *Node, w float32) {
	span := w
	if n.ScrollX {
		span = max(w, n.Content.W)
	}
	for _, ch := range n.Children {
		if !ch.NoMirror {
			ch.Rect.X = span - ch.Rect.X - ch.Rect.W
		}
	}
}

func placeAbsolute(ch *Node, w, h float32) {
	p := ch.Pos
	cw, fw := w, false
	if set(p.Left) && set(p.Right) {
		cw, fw = max(0, w-p.Left-p.Right), true
	}
	chh, fh := h, false
	if set(p.Top) && set(p.Bottom) {
		chh, fh = max(0, h-p.Top-p.Bottom), true
	}
	sz := measure(ch, cw, chh, fw, fh)
	x := float32(0)
	switch {
	case set(p.Left):
		x = p.Left
	case set(p.Right):
		x = w - p.Right - sz.W
	}
	y := float32(0)
	switch {
	case set(p.Top):
		y = p.Top
	case set(p.Bottom):
		y = h - p.Bottom - sz.H
	}
	ch.Rect = geom.Rect{X: x, Y: y, W: sz.W, H: sz.H}
	arrange(ch, sz.W, sz.H)
}

type item struct {
	n            *Node
	basis, main  float32
	cross        float32
	frozen       bool
	mMain, mCrs  float32 // margins along each axis (sum)
	mMainStart   float32
	mCrossStart  float32
	align        Align
	fixedCross   bool
	fixedCrossSz float32
}

// flex lays out in-flow children inside an inner box and returns the content
// size. When commit is set, child Rects are written and children arranged.
func flex(n *Node, innerW, innerH float32, defW, defH, commit bool) geom.Size {
	s := &n.Style
	if s.Dir == Stack {
		return stack(n, innerW, innerH, defW, defH, commit)
	}
	row := s.Dir == Row
	mainAvail, crossAvail := innerH, innerW
	defMain, defCross := defH, defW
	if row {
		mainAvail, crossAvail = innerW, innerH
		defMain, defCross = defW, defH
	}
	if geom.IsInf(mainAvail) {
		defMain = false
	}
	if geom.IsInf(crossAvail) {
		defCross = false
	}
	items := make([]item, 0, len(n.Children))
	for _, c := range n.Children {
		if c.Absolute {
			continue
		}
		it := item{n: c}
		cs := &c.Style
		if row {
			it.mMain, it.mCrs = cs.Margin.Horizontal(), cs.Margin.Vertical()
			it.mMainStart, it.mCrossStart = cs.Margin.Left, cs.Margin.Top
		} else {
			it.mMain, it.mCrs = cs.Margin.Vertical(), cs.Margin.Horizontal()
			it.mMainStart, it.mCrossStart = cs.Margin.Top, cs.Margin.Left
		}
		it.align = cs.AlignSelf
		if it.align == AlignAuto {
			it.align = s.Align
		}
		if it.align == AlignAuto {
			it.align = AlignStretch
		}
		if it.align == AlignStretch && cs.NoStretch {
			it.align = AlignStart
		}
		fixedMain := cs.Height
		fixedCross := cs.Width
		if row {
			fixedMain, fixedCross = cs.Width, cs.Height
		}
		it.fixedCross = fixedCross > 0
		it.fixedCrossSz = fixedCross
		crossForSizing := max(0, crossAvail-it.mCrs)
		stretchCross := it.align == AlignStretch && !it.fixedCross && defCross
		switch {
		case cs.HasBasis:
			it.basis = cs.Basis
		case cs.Grow > 0 && defMain:
			it.basis = 0
		case fixedMain > 0:
			it.basis = fixedMain
		default:
			sz := measureAxis(c, row, geom.Inf, crossForSizing, false, stretchCross)
			if row {
				it.basis = sz.W
			} else {
				it.basis = sz.H
			}
			// In a definite box, don't let auto basis overflow the main axis
			// beyond what's there: CSS would still overflow; we keep it.
		}
		items = append(items, it)
	}
	gap := s.Gap
	// Break into lines.
	var lines [][]int
	if s.Wrap && !geom.IsInf(mainAvail) {
		var cur []int
		used := float32(0)
		for i, it := range items {
			sz := it.basis + it.mMain
			if len(cur) > 0 && used+gap+sz > mainAvail+0.01 {
				lines = append(lines, cur)
				cur, used = nil, 0
			}
			if len(cur) > 0 {
				used += gap
			}
			used += sz
			cur = append(cur, i)
		}
		if len(cur) > 0 {
			lines = append(lines, cur)
		}
	} else {
		idx := make([]int, len(items))
		for i := range idx {
			idx[i] = i
		}
		lines = [][]int{idx}
	}
	// Resolve main sizes per line.
	var contentMain float32
	lineCross := make([]float32, len(lines))
	for li, ln := range lines {
		sum := float32(0)
		for k, i := range ln {
			if k > 0 {
				sum += gap
			}
			sum += items[i].basis + items[i].mMain
			items[i].main = items[i].basis
		}
		if defMain {
			free := mainAvail - sum
			resolveFlexible(items, ln, free, row)
		}
		for _, i := range ln {
			it := &items[i]
			cs := &it.n.Style
			if row {
				it.main = clamp(it.main, cs.MinW, cs.MaxW)
			} else {
				it.main = clamp(it.main, cs.MinH, cs.MaxH)
			}
		}
		total := float32(0)
		for k, i := range ln {
			if k > 0 {
				total += gap
			}
			total += items[i].main + items[i].mMain
		}
		contentMain = max(contentMain, total)
		// Cross sizes.
		var lc float32
		for _, i := range ln {
			it := &items[i]
			crossFor := max(0, crossAvail-it.mCrs)
			stretch := it.align == AlignStretch && !it.fixedCross && defCross && len(lines) == 1
			sz := measureAxis(it.n, row, it.main, crossFor, true, stretch)
			if row {
				it.cross = sz.H
			} else {
				it.cross = sz.W
			}
			lc = max(lc, it.cross+it.mCrs)
		}
		if len(lines) == 1 && defCross {
			lc = max(lc, crossAvail)
		}
		lineCross[li] = lc
	}
	contentCross := float32(0)
	for li, lc := range lineCross {
		if li > 0 {
			contentCross += s.LineGap
		}
		contentCross += lc
	}
	if !commit {
		if row {
			return geom.Size{W: contentMain, H: contentCross}
		}
		return geom.Size{W: contentCross, H: contentMain}
	}
	// Positions.
	padL, padT := s.Padding.Left, s.Padding.Top
	crossPos := float32(0)
	for li, ln := range lines {
		lc := lineCross[li]
		total := float32(0)
		for k, i := range ln {
			if k > 0 {
				total += gap
			}
			total += items[i].main + items[i].mMain
		}
		free := float32(0)
		if !geom.IsInf(mainAvail) {
			free = mainAvail - total
		}
		pos, between := justify(s.Justify, free, len(ln))
		for k, i := range ln {
			it := &items[i]
			if k > 0 {
				pos += gap + between
			}
			cross := it.cross
			if it.align == AlignStretch && !it.fixedCross {
				cross = max(0, lc-it.mCrs)
				if cs := &it.n.Style; row {
					cross = clamp(cross, cs.MinH, cs.MaxH)
				} else {
					cross = clamp(cross, cs.MinW, cs.MaxW)
				}
			}
			off := float32(0)
			switch it.align {
			case AlignCenter:
				off = (lc - it.mCrs - cross) / 2
			case AlignEnd:
				off = lc - it.mCrs - cross
			}
			mp := pos + it.mMainStart
			cp := crossPos + off + it.mCrossStart
			if row {
				it.n.Rect = geom.Rect{X: padL + mp, Y: padT + cp, W: it.main, H: cross}
			} else {
				it.n.Rect = geom.Rect{X: padL + cp, Y: padT + mp, W: cross, H: it.main}
			}
			arrange(it.n, it.n.Rect.W, it.n.Rect.H)
			pos += it.main + it.mMain
		}
		crossPos += lc + s.LineGap
	}
	if row {
		return geom.Size{W: contentMain, H: contentCross}
	}
	return geom.Size{W: contentCross, H: contentMain}
}

// measureAxis measures a child with main size either forced or unbounded.
func measureAxis(c *Node, row bool, main, cross float32, forceMain, forceCross bool) geom.Size {
	if row {
		return measure(c, main, cross, forceMain, forceCross)
	}
	return measure(c, cross, main, forceCross, forceMain)
}

func resolveFlexible(items []item, ln []int, free float32, row bool) {
	if free > 0 {
		var g float32
		for _, i := range ln {
			g += items[i].n.Grow
		}
		if g == 0 {
			return
		}
		// Iterate to honour max constraints.
		for iter := 0; iter < 4 && free > 0.01; iter++ {
			g = 0
			for _, i := range ln {
				if !items[i].frozen {
					g += items[i].n.Grow
				}
			}
			if g == 0 {
				break
			}
			spent := float32(0)
			for _, i := range ln {
				it := &items[i]
				if it.frozen || it.n.Grow == 0 {
					continue
				}
				add := free * it.n.Grow / g
				mx := it.n.MaxH
				if row {
					mx = it.n.MaxW
				}
				if mx > 0 && it.main+add > mx {
					add = mx - it.main
					it.frozen = true
				}
				it.main += add
				spent += add
			}
			free -= spent
		}
		return
	}
	if free < 0 {
		var w float32
		for _, i := range ln {
			w += items[i].n.Shrink * items[i].basis
		}
		if w == 0 {
			return
		}
		for _, i := range ln {
			it := &items[i]
			it.main = max(0, it.main+free*it.n.Shrink*it.basis/w)
		}
	}
}

func justify(j Justify, free float32, n int) (start, between float32) {
	if free < 0 {
		free = 0
	}
	switch j {
	case JustifyCenter:
		return free / 2, 0
	case JustifyEnd:
		return free, 0
	case JustifySpaceBetween:
		if n > 1 {
			return 0, free / float32(n-1)
		}
	case JustifySpaceAround:
		if n > 0 {
			b := free / float32(n)
			return b / 2, b
		}
	case JustifySpaceEvenly:
		b := free / float32(n+1)
		return b, b
	}
	return 0, 0
}

func stack(n *Node, innerW, innerH float32, defW, defH, commit bool) geom.Size {
	s := &n.Style
	var cw, chh float32
	for _, c := range n.Children {
		if c.Absolute {
			continue
		}
		al := c.AlignSelf
		if al == AlignAuto {
			al = s.Align
		}
		stretch := (al == AlignAuto || al == AlignStretch) && !c.NoStretch
		fw := stretch && c.Width == 0 && defW
		fh := stretch && c.Height == 0 && defH
		sz := measure(c, max(0, innerW-c.Margin.Horizontal()), max(0, innerH-c.Margin.Vertical()), fw, fh)
		cw = max(cw, sz.W+c.Margin.Horizontal())
		chh = max(chh, sz.H+c.Margin.Vertical())
		if commit {
			bw, bh := innerW, innerH
			if !defW {
				bw = sz.W + c.Margin.Horizontal()
			}
			if !defH {
				bh = sz.H + c.Margin.Vertical()
			}
			x, y := c.Margin.Left, c.Margin.Top
			switch al {
			case AlignCenter:
				x += (bw - c.Margin.Horizontal() - sz.W) / 2
				y += (bh - c.Margin.Vertical() - sz.H) / 2
			case AlignEnd:
				x += bw - c.Margin.Horizontal() - sz.W
				y += bh - c.Margin.Vertical() - sz.H
			}
			c.Rect = geom.Rect{X: s.Padding.Left + x, Y: s.Padding.Top + y, W: sz.W, H: sz.H}
			arrange(c, sz.W, sz.H)
		}
	}
	return geom.Size{W: cw, H: chh}
}
