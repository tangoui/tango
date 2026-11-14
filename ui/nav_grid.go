package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Responsive breakpoints

// Breakpoint names a window width range.
//
// Append-only: never reorder or insert; the zero value stays XS.
type Breakpoint uint8

const (
	XS Breakpoint = iota
	SM
	MD
	LG
	XL
	XXL
)

// String returns "xs" ... "xxl".
func (b Breakpoint) String() string {
	return [...]string{"xs", "sm", "md", "lg", "xl", "xxl"}[b]
}

// BreakpointMins returns the minimum widths of SM..XXL for the theme: Ant
// 576/768/992/1200/1600, Element 768/992/1200/1920 (no xxl), shadcn
// Tailwind's sm..2xl 640/768/1024/1280/1536.
func BreakpointMins(th *theme.Theme) [5]float32 {
	switch th.Flavor {
	case theme.Element:
		return [5]float32{768, 992, 1200, 1920, 1e9}
	case theme.StyleShadcn:
		return [5]float32{640, 768, 1024, 1280, 1536}
	}
	return [5]float32{576, 768, 992, 1200, 1600}
}

// ScreenBreakpoint returns the breakpoint of the current window width.
func ScreenBreakpoint(c *Context) Breakpoint {
	mins := BreakpointMins(c.Theme)
	w := c.size.W
	bp := XS
	for i, m := range mins {
		if w >= m {
			bp = Breakpoint(i + 1)
		}
	}
	return bp
}

// ---------------------------------------------------------------------------
// Grid

type gridSpan struct {
	span, offset int
	set          bool
}

// GridColW is a column of a GridRow.
type GridColW struct {
	_ [0]func()

	c      *Context
	base   gridSpan
	resp   [6]gridSpan
	fn     func()
	flex   float32
	opts   []layout.Option
	bg     theme.Color
	radius float32
	push   int
	pull   int
	order  int
}

type gridRowCtx struct {
	cols []*GridColW
}

var gridRowKey = hashID("grid-row-ctx")

// GridRowW is a 24-column grid row (Ant Row / Element el-row).
type GridRowW struct {
	_ [0]func()

	c       *Context
	gh, gv  float32
	respG   map[Breakpoint]float32
	justify layout.Justify
	align   layout.Align
	noWrap  bool
}

// GridRow starts a grid row; chain options, then Body with Col calls.
func GridRow(c *Context) *GridRowW { return &GridRowW{c: c} }

// Gutter sets the horizontal and vertical spacing between columns.
func (g *GridRowW) Gutter(h, v float32) *GridRowW { g.gh, g.gv = h, v; return g }

// GutterAt sets a responsive horizontal gutter per breakpoint.
func (g *GridRowW) GutterAt(bp Breakpoint, px float32) *GridRowW {
	if g.respG == nil {
		g.respG = map[Breakpoint]float32{}
	}
	g.respG[bp] = px
	return g
}

// Justify distributes free columns: JustifyStart, JustifyEnd, JustifyCenter,
// JustifySpaceBetween, JustifySpaceAround, JustifySpaceEvenly.
func (g *GridRowW) Justify(j layout.Justify) *GridRowW { g.justify = j; return g }

// Align sets vertical alignment of columns (AlignStart, AlignCenter, AlignEnd).
func (g *GridRowW) Align(a layout.Align) *GridRowW { g.align = a; return g }

// NoWrap keeps all columns on one line.
func (g *GridRowW) NoWrap() *GridRowW { g.noWrap = true; return g }

// Col adds a column spanning span/24 of the row; call it inside GridRow.Body.
func Col(c *Context, span int) *GridColW {
	col := &GridColW{c: c, base: gridSpan{span: span, set: true}}
	if st := State[[]*gridRowCtx](c, gridRowKey, nil); len(*st) > 0 {
		r := (*st)[len(*st)-1]
		r.cols = append(r.cols, col)
	}
	return col
}

// Offset shifts the column towards the end (right; left in RTL) by n
// columns.
func (col *GridColW) Offset(n int) *GridColW { col.base.offset = n; return col }

// Push moves the column towards the end by n columns without moving its
// neighbours (Ant push; for reordering columns visually).
func (col *GridColW) Push(n int) *GridColW { col.push = n; return col }

// Pull moves the column towards the start by n columns (Ant pull).
func (col *GridColW) Pull(n int) *GridColW { col.pull = n; return col }

// Order sets the flex order of the column within the row (lower first;
// equal orders keep the call order).
func (col *GridColW) Order(n int) *GridColW { col.order = n; return col }

// Flex makes the column take free space (flex: n) instead of a span.
func (col *GridColW) Flex(n float32) *GridColW { col.flex = n; return col }

func (col *GridColW) at(bp Breakpoint, span int, offset ...int) *GridColW {
	g := gridSpan{span: span, set: true}
	if len(offset) > 0 {
		g.offset = offset[0]
	}
	col.resp[bp] = g
	return col
}

// XS..XXL set the span (and optional offset) from that breakpoint up.
func (col *GridColW) XS(span int, offset ...int) *GridColW  { return col.at(XS, span, offset...) }
func (col *GridColW) SM(span int, offset ...int) *GridColW  { return col.at(SM, span, offset...) }
func (col *GridColW) MD(span int, offset ...int) *GridColW  { return col.at(MD, span, offset...) }
func (col *GridColW) LG(span int, offset ...int) *GridColW  { return col.at(LG, span, offset...) }
func (col *GridColW) XL(span int, offset ...int) *GridColW  { return col.at(XL, span, offset...) }
func (col *GridColW) XXL(span int, offset ...int) *GridColW { return col.at(XXL, span, offset...) }

// Style applies layout options to the column content box.
func (col *GridColW) Style(opts ...layout.Option) *GridColW { col.opts = opts; return col }

// Body sets the column content (built when the row lays out its lines).
func (col *GridColW) Body(fn func()) *GridColW { col.fn = fn; return col }

func (col *GridColW) resolve(bp Breakpoint) gridSpan {
	for b := int(bp); b >= 0; b-- {
		if col.resp[b].set {
			return col.resp[b]
		}
	}
	return col.base
}

// Body builds the row.
func (g *GridRowW) Body(fn func()) *Box {
	c := g.c
	st := State[[]*gridRowCtx](c, gridRowKey, nil)
	ctx := &gridRowCtx{}
	*st = append(*st, ctx)
	fn()
	st = State[[]*gridRowCtx](c, gridRowKey, nil)
	*st = (*st)[:len(*st)-1]

	// Order: a stable sort of the columns by their order value.
	cols := ctx.cols
	for i := 1; i < len(cols); i++ {
		for j := i; j > 0 && cols[j].order < cols[j-1].order; j-- {
			cols[j], cols[j-1] = cols[j-1], cols[j]
		}
	}
	bp := ScreenBreakpoint(c)
	gh := g.gh
	// ConfigProvider.Defaults("GridRow", {"gutter": float32}) sets the
	// horizontal gutter of rows that set none.
	if gh == 0 {
		gh = Prop(c, "GridRow", "gutter", float32(0))
	}
	for b := int(bp); b >= 0; b-- {
		if v, ok := g.respG[Breakpoint(b)]; ok {
			gh = v
			break
		}
	}
	type cell struct {
		col  *GridColW
		span gridSpan
	}
	var lines [][]cell
	var cur []cell
	used := 0
	for _, col := range cols {
		s := col.resolve(bp)
		if s.span <= 0 && col.flex == 0 {
			continue // span 0 hides the column
		}
		w := s.span + s.offset
		if col.flex > 0 {
			w = s.offset
		}
		if !g.noWrap && used+w > 24 && len(cur) > 0 {
			lines = append(lines, cur)
			cur, used = nil, 0
		}
		cur = append(cur, cell{col, s})
		used += w
	}
	if len(cur) > 0 {
		lines = append(lines, cur)
	}
	outer := Column(c, func() {
		for li, line := range lines {
			Key(c, li, func() {
				total, flexSum := 0, float32(0)
				for _, cl := range line {
					total += cl.span.offset
					if cl.col.flex > 0 {
						flexSum += cl.col.flex
					} else {
						total += cl.span.span
					}
				}
				rem := float32(max(0, 24-total))
				if flexSum > 0 {
					rem = 0
				}
				n := float32(len(line))
				var lead, between, trail float32
				switch g.justify {
				case layout.JustifyEnd:
					lead = rem
				case layout.JustifyCenter:
					lead, trail = rem/2, rem/2
				case layout.JustifySpaceBetween:
					if n > 1 {
						between = rem / (n - 1)
					} else {
						trail = rem
					}
				case layout.JustifySpaceAround:
					lead, between, trail = rem/(2*n), rem/n, rem/(2*n)
				case layout.JustifySpaceEvenly:
					lead, between, trail = rem/(n+1), rem/(n+1), rem/(n+1)
				default:
					trail = rem
				}
				spacer := func(grow float32) {
					if grow <= 0 {
						return
					}
					s := c.add("grid-gap", "")
					s.ln.Grow, s.ln.HasBasis = grow, true
				}
				row := Row(c, func() {
					spacer(lead)
					for i, cl := range line {
						if i > 0 {
							spacer(between)
						}
						spacer(float32(cl.span.offset))
						cell := c.add("grid-col", "")
						cell.ln.Dir = layout.Column
						cell.ln.HasBasis = true
						if cl.col.flex > 0 {
							cell.ln.Grow = cl.col.flex / flexSum * float32(max(1, 24-total))
						} else {
							cell.ln.Grow = float32(cl.span.span)
						}
						cell.ln.MinW = 0
						if shift := cl.col.push - cl.col.pull; shift != 0 && cl.col.flex == 0 && cl.span.span > 0 {
							// Relative offset by whole columns: one column is the
							// cell width divided by its span.
							// Push / pull are logical: in RTL push moves left.
							if c.IsRTL() {
								shift = -shift
							}
							span := float32(cl.span.span)
							cell.xf = func(r geom.Rect) geom.Affine { return geom.Translate(float32(shift)*r.W/span, 0) }
						}
						cl := cl
						c.body(cell, func() {
							inner := Column(c, cl.col.fn, append([]layout.Option{layout.Grow(1), layout.Margin(geom.Insets{Left: gh / 2, Right: gh / 2})}, cl.col.opts...)...)
							inner.n.ln.MinW = 0
						})
					}
					spacer(trail)
				})
				row.n.ln.Margin = geom.Insets{Left: -gh / 2, Right: -gh / 2}
				if g.align != layout.AlignAuto {
					row.n.ln.Align = g.align
				}
			})
		}
	}, layout.Gap(g.gv))
	return outer
}
