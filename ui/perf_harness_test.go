package ui

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"image"
	"io"
	"math"
	"sort"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Component performance harness (first layer): every root ui entry has a
// fixture with a typical instance, its most common interaction and, for
// collections, a large data set. perf_check_test.go checks coverage, idle
// frames, damage and steady interaction; perf_bench_test.go times them.

type perfFixture struct {
	Name          string                       // root ui exported entry, e.g. "Button"
	Covers        []string                     // other entries this fixture also covers, e.g. "CardMeta", "CardAction"
	Skip          string                       // non-empty = not applicable, with the reason (helpers that render nothing…)
	Overlay       bool                         // overlay kinds: first build budget 2 ms
	Animated      string                       // non-empty = animates legitimately at rest (Spin, Skeleton shimmer…), with the reason
	Build         func(c *Context)             // typical instance; overlays build open
	Interact      func(r *perfRun, i int)      // step i: even steps act, odd steps undo
	Large         func(n int) func(c *Context) // collections only: a page with n items
	LargeInteract func(r *perfRun, i int)      // nil = default scroll (perfScroll)
	// LargeVirtual is the opt-in virtual form of Large (a component whose
	// default form is not windowed and that offers Height + Virtual or
	// the like): scenes LargeVirtual1k / 10k / 100k, interacted with
	// LargeInteract, budgeted like Large (perfBudget).
	LargeVirtual func(n int) func(c *Context)
	// NA marks single scenes 不适用 (not applicable), scene name → reason
	// (e.g. "Large100k": perfNANonVirtual). Such a scene still runs in
	// BenchmarkComponents for reference, but TestPerfBudgets does not time
	// it: it prints "PERF <Name/Scene> 0 0 n/a <reason>", which uiperf
	// bench shows in status.md. Rules (perfNAProblems, run by
	// TestPerfCoverage): only Large10k / Large100k (Large1k cannot be NA
	// and still runs in BenchmarkComponents, though TestPerfBudgets skips
	// it when its 100k is NA, having nothing to be the reference of;
	// other scenes over budget go to perfTimeDebt); the scene
	// exists, the reason is set, the scene is not in perfTimeDebt; and the
	// fixture either has LargeVirtual scenes (the default form is O(n) by
	// design, its virtual form is budgeted) or the reason starts with
	// "unrealistic:" (n items is not a real size for the widget),
	// "closure:" (arbitrary content, use VirtualList / Listy) or
	// "covered:" (the virtual form is measured by another fixture: name
	// it).
	NA map[string]string
}

// perfNANonVirtual is the NA reason of a Large100k scene that measures a
// component's default, non-virtual form, which is O(n) by design; its
// opt-in virtual form is timed by the LargeVirtual scenes.
const perfNANonVirtual = "非虚拟形态按设计是 O(n)；虚拟形态由 LargeVirtual 场景计时"

// Batch 3 V4 NA reasons (virtualization proposal §5) shared by several
// fixtures.
const (
	perfNAClosure     = "closure: 通用闭包容器，看不到“项”，无法窗口化；大列表用 VirtualList / Listy（已计时）"
	perfNAOptionGroup = "unrealistic: 表单内联选项组，几十项已是上限；大集合用 MultiSelect / Transfer（已虚拟）"
)

// perfNA100k marks fx's Large100k 不适用 with why (see perfFixture.NA).
func perfNA100k(fx perfFixture, why string) perfFixture {
	fx.NA = map[string]string{"Large100k": why}
	return fx
}

type perfRun struct {
	D      *Driver
	Target geom.Rect // the component's bounding box (perfTarget)
}

// Center is the center of Target.
func (r *perfRun) Center() geom.Point { return r.Target.Center() }

// At is the point at fractions fx, fy (0..1) of Target.
func (r *perfRun) At(fx, fy float32) geom.Point {
	return geom.Point{X: r.Target.X + fx*r.Target.W, Y: r.Target.Y + fy*r.Target.H}
}

var perfRegistry = map[string]perfFixture{}

// registerPerf adds fixtures; a duplicate name panics.
func registerPerf(fx ...perfFixture) {
	for _, f := range fx {
		if _, dup := perfRegistry[f.Name]; dup {
			panic("ui: duplicate perf fixture " + f.Name)
		}
		perfRegistry[f.Name] = f
	}
}

// perfFixtures returns the fixtures sorted by Name.
func perfFixtures() []perfFixture {
	out := make([]perfFixture, 0, len(perfRegistry))
	for _, f := range perfRegistry {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// perfNopRenderer renders nothing: timings cover build, layout, the
// display list and damage fingerprints, not rasterization.
type perfNopRenderer struct{}

func (perfNopRenderer) Render(*paint.List, geom.Size, float32) error { return nil }
func (perfNopRenderer) BackdropBlur() bool                           { return true }

// newPerfDriver is a 1440×900 driver at scale 2 without animations, theme
// antd. raster=false renders through perfNopRenderer.
func newPerfDriver(page func(*Context), raster bool) *Driver {
	win := &driverWindow{size: geom.Size{W: 1440, H: 900}, scale: 2, now: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)}
	d := &Driver{w: win}
	var rd paint.Renderer
	if !raster {
		rd = perfNopRenderer{}
	}
	d.r = NewRunner(page, RunnerOptions{Theme: theme.AntDesign(), Clock: func() time.Time { return win.now }, NoAnimations: true, Renderer: rd})
	d.r.Attach(win)
	return d
}

// perfPage wraps a fixture's build in the page every fixture shares.
func perfPage(build func(c *Context)) func(*Context) {
	return func(c *Context) { Column(c, func() { build(c) }, layout.Pad(24)) }
}

// perfTarget is the bounding box of what the fixture built in the page:
// the union of the rects of the page column's children (the root's first
// child is perfPage's full-width column, so its own rect is not the
// component's).
func perfTarget(d *Driver) geom.Rect {
	root := d.r.c.root
	if root == nil || len(root.kids) == 0 {
		return geom.Rect{}
	}
	var r geom.Rect
	for _, k := range root.kids[0].kids {
		r = r.Union(k.abs)
	}
	return r
}

// perfBBox is Target united with the visible content of all open
// overlays (perfOverlayContent), clipped to the window (dirty rects never
// reach beyond it).
func perfBBox(d *Driver) geom.Rect {
	r := perfTarget(d)
	for _, o := range d.r.c.overlays {
		r = r.Union(perfOverlayContent(o))
	}
	sz := d.r.win.Size()
	return r.Intersect(geom.Rect{W: sz.W, H: sz.H})
}

// perfOverlayContent is what an overlay shows. A layer that takes input
// (hit) or blocks it (a modal mask) counts whole. Otherwise it is a
// transparent carrier (Affix, float buttons, context menus, notifications
// build a ScreenFill layer and place their content inside, often only at
// paint time): its rect is the union of its descendants' rects, recursive
// because pinned Affix content sits in an inner node under a clip node
// (declared 1×1, resized to the viewport at paint time); a carrier without
// descendants (Affix's watcher) contributes nothing. abs is set while
// painting, so after a frame it holds the final positions.
func perfOverlayContent(o *overlay) geom.Rect {
	if o.root.hit || o.root.blocks {
		return o.root.abs
	}
	var r geom.Rect
	var walk func(n *node)
	walk = func(n *node) {
		for _, k := range n.kids {
			r = r.Union(k.abs)
			walk(k)
		}
	}
	walk(o.root)
	return r
}

// perfContentRects are the rects of what the fixture shows: the page
// column's children (the parts perfTarget unions) and the visible content
// of every open overlay (perfOverlayContent), clipped to the window.
func perfContentRects(d *Driver) []geom.Rect {
	sz := d.r.win.Size()
	win := geom.Rect{W: sz.W, H: sz.H}
	var out []geom.Rect
	add := func(r geom.Rect) {
		if r = r.Intersect(win); !r.Empty() {
			out = append(out, r)
		}
	}
	if root := d.r.c.root; root != nil && len(root.kids) > 0 {
		for _, k := range root.kids[0].kids {
			add(k.abs)
		}
	}
	for _, o := range d.r.c.overlays {
		add(perfOverlayContent(o))
	}
	return out
}

// perfContentArea is the area (logical px², the unit perfDirtyArea
// returns) covered by rs, overlaps counted once. rectsUnionArea works on
// integer rects, so coordinates go on a 1/64 logical-px grid (rounded):
// layout positions are fractional, and snapping them to the device-pixel
// grid would inflate small parts' areas by up to a pixel per edge.
func perfContentArea(rs []geom.Rect) float32 {
	const grid = 64
	q := func(v float32) int { return int(math.Round(float64(v * grid))) }
	px := make([]image.Rectangle, 0, len(rs))
	for _, r := range rs {
		px = append(px, image.Rect(q(r.X), q(r.Y), q(r.Right()), q(r.Bottom())))
	}
	return float32(float64(rectsUnionArea(px)) / (grid * grid))
}

// perfDirtyArea is the area (logical px²) of the union of the last frame's
// dirty rects; a frame presented whole (nil) counts as the viewport.
func perfDirtyArea(d *Driver) float32 {
	s := d.r.win.Scale()
	if d.r.lastDirty == nil {
		sz := d.r.win.Size()
		return sz.W * sz.H
	}
	return float32(rectsUnionArea(d.r.lastDirty)) / (s * s)
}

// rectsUnionArea is the exact area covered by rs (overlaps counted once).
func rectsUnionArea(rs []image.Rectangle) int {
	var xs []int
	for _, r := range rs {
		xs = append(xs, r.Min.X, r.Max.X)
	}
	sort.Ints(xs)
	area := 0
	for i := 0; i+1 < len(xs); i++ {
		x0, x1 := xs[i], xs[i+1]
		if x0 == x1 {
			continue
		}
		type span struct{ a, b int }
		var sp []span
		for _, r := range rs {
			if r.Min.X <= x0 && r.Max.X >= x1 {
				sp = append(sp, span{r.Min.Y, r.Max.Y})
			}
		}
		sort.Slice(sp, func(i, j int) bool { return sp[i].a < sp[j].a })
		h, end := 0, -1<<62
		for _, s := range sp {
			if s.a > end {
				h += s.b - s.a
				end = s.b
			} else if s.b > end {
				h += s.b - end
				end = s.b
			}
		}
		area += h * (x1 - x0)
	}
	return area
}

// perfListHash is an FNV-64a hash of the last frame's visual display list.
// Presentation hints (Damage/ScrollRegion) are excluded. Paths, glyph
// outlines and image pixels are hashed by content, never pointer identity.
// This correctness check runs outside benchmark timing; resources are not
// cached here, so replacing or editing one cannot leave a stale hash.
func perfListHash(d *Driver) uint64 {
	h := fnv.New64a()
	for _, op := range d.r.List().Ops {
		perfHashOp(h, op)
	}
	return h.Sum64()
}

func perfHashOp(w io.Writer, op paint.Op) {
	switch o := op.(type) {
	case paint.Damage, paint.ScrollRegion:
		return // presentation strategy, not screen content
	case paint.FillPath:
		fmt.Fprintf(w, "FillPath{%#v %#v}", perfPathSegs(o.Path), o.Color)
	case paint.Gradient:
		segs := perfPathSegs(o.Path)
		o.Path = nil
		fmt.Fprintf(w, "%#v %#v", o, segs)
	case paint.Text:
		fmt.Fprintf(w, "Text{%#v %#v", o.Origin, o.Color)
		for _, g := range o.Glyphs {
			var p *paint.Path
			if g.Src != nil {
				p = g.Src.GlyphPath(g.ID, g.Size)
			}
			fmt.Fprintf(w, " {%d %g %g %g %g %#v}", g.ID, g.Size, g.X, g.Y, g.Skew, perfPathSegs(p))
		}
		io.WriteString(w, "}")
	case paint.Image:
		fmt.Fprintf(w, "Image{%#v %#v", o.Rect, o.Radii)
		if o.Img == nil {
			io.WriteString(w, " nil}")
			return
		}
		b := o.Img.Bounds()
		fmt.Fprintf(w, " %v ", b)
		// RGBA() provides canonical 16-bit premultiplied channels across
		// image/color implementations. Include every pixel, not just bounds.
		var pixel [8]byte
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, b, a := o.Img.At(x, y).RGBA()
				binary.LittleEndian.PutUint16(pixel[0:2], uint16(r))
				binary.LittleEndian.PutUint16(pixel[2:4], uint16(g))
				binary.LittleEndian.PutUint16(pixel[4:6], uint16(b))
				binary.LittleEndian.PutUint16(pixel[6:8], uint16(a))
				w.Write(pixel[:])
			}
		}
		io.WriteString(w, "}")
	default:
		fmt.Fprintf(w, "%#v", op)
	}
}

func perfPathSegs(p *paint.Path) []paint.Seg {
	if p == nil {
		return nil
	}
	return p.Segs
}

// perfStep runs interaction step i: Target is refreshed, the fixture acts,
// and the frame a host would run next (only if one is needed: queued
// input, an invalidation or an animation) is run. Interactions that run
// their own frames (Driver.Move, Click, Key, Type) leave the frame that
// processed the input last; a forced frame with nothing to do would
// measure an idle frame's damage instead.
func perfStep(r *perfRun, interact func(r *perfRun, i int), i int) {
	r.Target = perfTarget(r.D)
	interact(r, i)
	if r.D.NeedsFrame() {
		r.D.Frame()
	}
}

// perfScrollPoint is where perfScroll wheels: the center of the top-most
// open overlay that shows something (perfOverlayContent), else of Target.
func perfScrollPoint(r *perfRun) geom.Point {
	ov := r.D.r.c.overlays
	for i := len(ov) - 1; i >= 0; i-- {
		if c := perfOverlayContent(ov[i]); !c.Empty() {
			return c.Center()
		}
	}
	return r.Center()
}

// perfScroll is the default large-data interaction: wheel scrolling at
// perfScrollPoint (the top-most open overlay's content, such as a
// dropdown list, or Target when none is open); 20 steps down, then 20 up.
func perfScroll(r *perfRun, i int) {
	p := perfScrollPoint(r)
	dy := float32(120)
	if i%40 >= 20 {
		dy = -120
	}
	r.D.Inject(input.Scroll{Pos: p, DY: dy})
}
