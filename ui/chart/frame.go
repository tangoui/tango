package chart

import (
	"fmt"
	"hash/fnv"
	"math"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Animation durations (G2 / ECharts use ~1s on entry; UI motion is faster).
const (
	enterDur  = 700 * time.Millisecond
	updateDur = 400 * time.Millisecond
)

// common holds the options every chart shares. Chart types embed it so the
// modifiers return the concrete chart type.
type common[T any] struct {
	self      *T
	c         *ui.Context
	height    float32
	colors    []theme.Color
	legend    LegendPos
	noTooltip bool
	loading   bool
	noAnim    bool
	labels    bool
	format    Formatter
	opts      []layout.Option
	key       any
	tweaks    []func(*Style)
	a11yName  string
	mirror    bool
}

func (b *common[T]) init(c *ui.Context, self *T, h float32) {
	b.c, b.self, b.height = c, self, h
}

// Height sets the chart height in logical pixels (the width follows the
// parent).
func (b *common[T]) Height(h float32) *T { b.height = h; return b.self }

// Colors overrides the palette.
func (b *common[T]) Colors(cs ...theme.Color) *T { b.colors = cs; return b.self }

// Legend places (or with LegendNone hides) the legend.
func (b *common[T]) Legend(p LegendPos) *T { b.legend = p; return b.self }

// NoTooltip disables the hover tooltip and crosshair.
func (b *common[T]) NoTooltip() *T { b.noTooltip = true; return b.self }

// Loading shows a spinner over the chart (or in place of it without data).
func (b *common[T]) Loading(on bool) *T { b.loading = on; return b.self }

// Animate enables (default) or disables entry and update transitions.
func (b *common[T]) Animate(on bool) *T { b.noAnim = !on; return b.self }

// Label shows data labels on the marks.
func (b *common[T]) Label() *T { b.labels = true; return b.self }

// Format sets the value formatter used by the value axis, tooltip and
// labels (default: thousands separators).
func (b *common[T]) Format(f Formatter) *T { b.format = f; return b.self }

// Layout applies layout options to the chart box (Grow, Margin ...).
func (b *common[T]) Layout(o ...layout.Option) *T { b.opts = append(b.opts, o...); return b.self }

// Key gives the chart a stable identity (legend and animation state) when
// charts are added or removed around it.
func (b *common[T]) Key(k any) *T { b.key = k; return b.self }

// Mirror draws the plot right to left: the x axis runs from the right, the
// value axis sits on the right, bars, lines, areas and points are mirrored
// (so categories read in reverse order), and pie, donut, radar and gauge
// start at the mirrored angle and sweep counter-clockwise. Labels stay
// readable; only their positions move. Without it the plot is drawn left
// to right even in RTL scopes, while the legend, tooltip and labels follow
// the scope direction.
func (b *common[T]) Mirror() *T { b.mirror = true; return b.self }

// Style tweaks the resolved chart style for this chart only.
func (b *common[T]) Style(fn func(*Style)) *T { b.tweaks = append(b.tweaks, fn); return b.self }

func (b *common[T]) valueFmt() Formatter {
	if b.format != nil {
		return b.format
	}
	return func(v float64) string { return FormatNumber(v, min(decimalsOf(v), 2)) }
}

// Tip is the content of a tooltip.
type Tip struct {
	Title string
	Items []TipItem
	Index int // x / category / slice index
}

// TipItem is one tooltip row.
type TipItem struct {
	Name, Value string
	Color       theme.Color
	Marker      Marker
}

// hover is a hit-test result: tooltip content plus where to draw the
// crosshair and active marks.
type hover struct {
	tip    Tip
	anchor geom.Point // tooltip anchor (absolute)
	index  int
	series int // hovered series (scatter / pie), -1 = all
}

// View is a shown chart: its box plus the hover / legend state of the frame.
type View struct {
	*ui.Box
	tip    *Tip
	hidden map[string]bool
}

// Tooltip returns the tooltip shown in this frame.
func (v View) Tooltip() (Tip, bool) {
	if v.tip == nil {
		return Tip{}, false
	}
	return *v.tip, true
}

// Hidden reports whether the legend has hidden series (or slice) name.
func (v View) Hidden(name string) bool { return v.hidden[name] }

// chartState is retained per chart.
type chartState struct {
	hidden map[string]bool
	tw     tween
	ds     dsCache
	grid   gridCache
}

// gridCache keeps the parsed grid of a large dataset between frames.
type gridCache struct {
	key uint64
	g   *grid
}

// cacheMin is the row count from which parsed data is cached. Smaller
// datasets are parsed every frame, so in-place edits always show.
const cacheMin = 2000

// gridFor parses rows into a grid, reusing last frame's result for large
// datasets when the slice, the fields and a sample of the rows are
// unchanged. Replace (rather than edit in place) large slices to update.
func (f *frame) gridFor(data []Row, xf, yf, sf string, cat bool) *grid {
	if len(data) < cacheMin {
		if cat {
			return buildCatGrid(data, xf, yf, sf)
		}
		return buildGrid(data, xf, yf, sf)
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%p|%d|%s|%s|%s|%v", &data[0], len(data), xf, yf, sf, cat)
	step := max(1, len(data)/64)
	for i := 0; i < len(data); i += step {
		r := data[i]
		fmt.Fprint(h, r[xf], r[yf], r[sf])
	}
	last := data[len(data)-1]
	fmt.Fprint(h, last[xf], last[yf], last[sf])
	key := h.Sum64()
	gc := &f.cs.grid
	if gc.g != nil && gc.key == key {
		return gc.g
	}
	var g *grid
	if cat {
		g = buildCatGrid(data, xf, yf, sf)
	} else {
		g = buildGrid(data, xf, yf, sf)
	}
	gc.key, gc.g = key, g
	return g
}

type tween struct {
	init     bool
	sig      uint64
	gen      int
	from, to []float64
	cur      []float64
}

type dsCache struct {
	key  uint64
	idx  [][]int
	vals [][]float64
}

// frame is the per-Show context shared by chart implementations.
type frame struct {
	c      *ui.Context
	th     *theme.Theme
	st     *Style
	root   *ui.Box
	id     ui.ID
	cs     *chartState
	noAnim bool
	enter  float32 // entry progress 0..1
	label  string  // A11yLabel
	sem    chartSem
	mirror bool           // plot drawn right to left (Mirror)
	para   text.Direction // paragraph direction of label text (RTL in RTL scopes)
}

func (b *common[T]) begin() *frame {
	c := b.c
	var f *frame
	mk := func() {
		opts := append([]layout.Option{layout.H(b.height)}, b.opts...)
		root := ui.Div(c, opts...)
		cs := ui.State(c, root.ID(), func() chartState { return chartState{hidden: map[string]bool{}} })
		st := StyleOf(c.Theme)
		if len(b.colors) > 0 {
			st.Colors = b.colors
		}
		for _, t := range b.tweaks {
			t(st)
		}
		f = &frame{c: c, th: c.Theme, st: st, root: root, id: root.ID(), cs: cs, noAnim: b.noAnim, label: b.a11yName, mirror: b.mirror}
		if c.IsRTL() {
			f.para = text.DirRTL
		}
		f.enter = 1
		if !b.noAnim {
			f.enter = c.AnimFrom(f.id, "enter", 0, 1, enterDur)
		}
	}
	if b.key != nil {
		ui.Key(c, b.key, mk)
	} else {
		mk()
	}
	return f
}

// tweenVals eases from the previously shown values to vals whenever vals
// change; on the first frame it starts from zero when fromZero is set.
func (f *frame) tweenVals(vals []float64, fromZero bool) []float64 {
	tw := &f.cs.tw
	sig := hashFloats(vals)
	if !tw.init {
		tw.init, tw.sig = true, sig
		tw.to = append(tw.to[:0], vals...)
		tw.from = make([]float64, len(vals))
		if !fromZero {
			copy(tw.from, vals)
		}
		tw.cur = make([]float64, len(vals))
	} else if sig != tw.sig {
		tw.sig = sig
		from := make([]float64, len(vals))
		for i := range from {
			if i < len(tw.cur) && !math.IsNaN(tw.cur[i]) {
				from[i] = tw.cur[i]
			} else {
				from[i] = vals[i]
			}
		}
		tw.from = from
		tw.to = append(tw.to[:0], vals...)
		tw.gen++
		if len(tw.cur) != len(vals) {
			tw.cur = make([]float64, len(vals))
		}
	}
	p := float32(1)
	if !f.noAnim {
		dur := updateDur
		if tw.gen == 0 {
			dur = enterDur
		}
		p = f.c.AnimFrom(f.id, fmt.Sprint("tw", tw.gen), 0, 1, dur)
	}
	for i := range vals {
		a, b := tw.from[i], tw.to[i]
		if math.IsNaN(a) || math.IsNaN(b) {
			tw.cur[i] = b
			continue
		}
		tw.cur[i] = a + (b-a)*float64(p)
	}
	return tw.cur
}

func hashFloats(v []float64) uint64 {
	h := fnv.New64a()
	var b [8]byte
	for _, x := range v {
		u := math.Float64bits(x)
		for i := range b {
			b[i] = byte(u >> (8 * i))
		}
		h.Write(b[:])
	}
	return h.Sum64()
}

// legendItem is one legend entry.
type legendItem struct {
	name   string
	color  theme.Color
	marker Marker
}

// plotSpec describes the chart body to finish.
type plotSpec struct {
	items     []legendItem
	legendPos LegendPos // resolved default for this chart type
	empty     bool
	hit       func(r geom.Rect, p geom.Point) *hover
	paint     func(l *paint.List, r geom.Rect, h *hover)
	tooltip   bool
	loading   bool
}

func (f *frame) legendPos(user LegendPos, def LegendPos, n int) LegendPos {
	if user != LegendAuto {
		return user
	}
	if n < 2 {
		return LegendNone
	}
	if def != LegendAuto {
		return def
	}
	return f.st.LegendPos
}

// finish builds legend, plot, tooltip and the loading / empty states.
func (f *frame) finish(sp plotSpec) View {
	c, st := f.c, f.st
	v := View{Box: f.root, hidden: f.cs.hidden}
	f.semantics(&sp)
	pos := sp.legendPos
	if sp.empty {
		pos = LegendNone
	}
	if pos == LegendLeft || pos == LegendRight {
		f.root.Style(layout.Dir(layout.Row), layout.Gap(16))
	}
	f.root.Body(func() {
		legend := func() {
			if pos != LegendNone && len(sp.items) > 0 {
				f.legend(sp.items, pos)
			}
		}
		if pos == LegendTop || pos == LegendLeft {
			legend()
		}
		if sp.empty {
			ui.Div(c, layout.Grow(1), layout.Center()).Body(func() {
				if !sp.loading {
					ui.Empty(c).Simple()
				}
			})
		} else {
			f.plot(sp, &v)
		}
		if pos == LegendBottom || pos == LegendRight {
			legend()
		}
		if sp.loading {
			bg := theme.Fade(st.Surface, 0.6)
			ui.Div(c, layout.Abs(0, 0, 0, 0), layout.Center()).Bg(bg).Body(func() { ui.Spin(c) })
		}
	})
	return v
}

func (f *frame) plot(sp plotSpec, v *View) {
	c := f.c
	pb := ui.Div(c, layout.Grow(1), layout.Basis(0))
	var h *hover
	ia := pb.Interact(input.CursorDefault)
	ptr, in := c.Pointer()
	if sp.tooltip && ia.Hovered && in && sp.hit != nil {
		if pr := pb.Rect(); !pr.Empty() {
			h = sp.hit(pr, ptr)
		}
	}
	pb.Paint(func(l *paint.List, r geom.Rect) {
		var ph *hover
		if h != nil {
			ph = sp.hit(r, ptr)
		}
		sp.paint(l, r, ph)
	})
	if h == nil || len(h.tip.Items) == 0 {
		return
	}
	v.tip = &h.tip
	pr := pb.Rect()
	pb.Body(func() {
		// The plot is painted left to right in every scope, so the pointer
		// anchor is placed physically (not mirrored in RTL parents).
		anchor := ui.Custom(c, nil, nil, layout.Abs(h.anchor.X-pr.X, h.anchor.Y-pr.Y, layout.Unset, layout.Unset), layout.Size(1, 1), layout.Physical())
		f.tooltip(anchor.ID(), h.tip)
	})
}

// tooltip opens the tooltip panel next to the anchor: on the inline end
// side (right in LTR, left in RTL scopes), with the rows laid out and the
// names shaped in the scope direction. Values keep their own direction so
// numbers with units read left to right.
func (f *frame) tooltip(anchor ui.ID, tip Tip) {
	c, st := f.c, f.st
	ov := ui.Overlay(c, ui.OverlayOptions{Anchor: anchor, Place: ui.InlineEndTop, Offset: 14}, func() {
		ui.Column(c, func() {
			if tip.Title != "" {
				ui.Text(c, tip.Title).Size(st.TooltipFont).LineHeight(st.TooltipLine).Color(st.TooltipTitle)
			}
			for i, it := range tip.Items {
				ui.Key(c, i, func() {
					ui.Row(c, func() {
						col := it.Color
						ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 8, H: 8} }, func(l *paint.List, r geom.Rect) {
							cx, cy := r.X+4, r.Y+4
							if st.Name == "echarts" {
								l.Path(paint.NewPath().Circle(cx, cy, 5), col)
							} else {
								l.Path(paint.NewPath().Circle(cx, cy, 4), col)
							}
						})
						ui.Text(c, it.Name).Size(st.TooltipFont).LineHeight(st.TooltipLine).Color(st.TooltipName)
						ui.Spacer(c)
						val := ui.Text(c, it.Value).TextDir(text.DirAuto).Size(st.TooltipFont).LineHeight(st.TooltipLine).Color(st.TooltipValue)
						if st.TooltipBold {
							val.Strong()
						}
					}, layout.Gap(8), layout.Middle())
				})
			}
		}, layout.Gap(0), layout.MinW(120))
	}, layout.PadVH(st.TooltipPad-4, st.TooltipPad))
	ov.Bg(st.TooltipBg).Radius(st.TooltipRadius).Shadow(st.TooltipShadow)
}

// legend builds a clickable legend.
func (f *frame) legend(items []legendItem, pos LegendPos) {
	c, st := f.c, f.st
	vertical := pos == LegendLeft || pos == LegendRight
	body := func() {
		for _, it := range items {
			it := it
			ui.Key(c, "lg:"+it.name, func() {
				off := f.cs.hidden[it.name]
				col, tc := it.color, st.LegendText
				if off {
					col, tc = st.LegendOff, st.LegendOff
				}
				ms := markerSize(it.marker)
				row := ui.Row(c, func() {
					ui.Custom(c, func(_, _ float32) geom.Size { return ms }, func(l *paint.List, r geom.Rect) {
						drawMarker(l, it.marker, r.X+r.W/2, r.Y+r.H/2, col, st.Surface)
					})
					ui.Text(c, it.name).Size(st.Font).LineHeight(20).Color(tc)
				}, layout.Gap(6), layout.Middle(), layout.Self(layout.AlignStart))
				// A toggle button per series: Tab / Enter / Space work.
				row.Semantics(a11y.RoleButton, it.name).A11yState(a11y.Toggleable)
				if !off {
					row.A11yState(a11y.Pressed)
				}
				ia := row.Focusable(input.CursorPointer)
				if ia.FocusVisible {
					row.PaintOver(func(l *paint.List, r geom.Rect) {
						l.Stroke(r.Expand(2), geom.Rad(4), 2, f.th.Primary.Base)
					})
				}
				if ia.Clicked {
					f.cs.hidden[it.name] = !off
					// Never hide everything: keep at least one entry.
					all := true
					for _, o := range items {
						if !f.cs.hidden[o.name] {
							all = false
						}
					}
					if all {
						f.cs.hidden[it.name] = false
					}
					c.Invalidate()
				}
			})
		}
	}
	if vertical {
		ui.Column(c, body, layout.Gap(8), layout.JustifyContent(layout.JustifyCenter))
		return
	}
	opts := []layout.Option{layout.GapXY(st.LegendGap, 4), layout.WrapItems(), layout.Middle()}
	if st.LegendCenter || pos == LegendBottom {
		opts = append(opts, layout.JustifyContent(layout.JustifyCenter))
	}
	m := geom.Insets{Bottom: 8}
	if pos == LegendBottom {
		m = geom.Insets{Top: 8}
	}
	opts = append(opts, layout.Margin(m))
	ui.Row(c, body, opts...)
}

// hiddenMask returns the hidden flag of each name.
func (f *frame) hiddenMask(names []string) []bool {
	out := make([]bool, len(names))
	for i, n := range names {
		out[i] = f.cs.hidden[n]
	}
	return out
}
