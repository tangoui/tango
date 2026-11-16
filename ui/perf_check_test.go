package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// perfEntries scans the non-test files of the ui package for exported
// functions whose first parameter is a *Context: the component entries
// every perf fixture is checked against (same set as
// grep -E "^func [A-Z][A-Za-z0-9]*(\[[^]]*\])?\(c \*Context").
func perfEntries(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range pkgs {
		if p.Name != "ui" {
			continue
		}
		for _, f := range p.Files {
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !fn.Name.IsExported() {
					continue
				}
				ps := fn.Type.Params.List
				if len(ps) == 0 {
					continue
				}
				if st, ok := ps[0].Type.(*ast.StarExpr); ok {
					if id, ok := st.X.(*ast.Ident); ok && id.Name == "Context" {
						out = append(out, fn.Name.Name)
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestPerfCoverage(t *testing.T) {
	entries := perfEntries(t)
	isEntry := map[string]bool{}
	for _, e := range entries {
		isEntry[e] = true
	}
	covered := map[string]bool{}
	for _, fx := range perfFixtures() {
		if !isEntry[fx.Name] {
			t.Errorf("fixture %q is not a ui entry (exported func taking *Context first)", fx.Name)
		}
		covered[fx.Name] = true
		for _, n := range fx.Covers {
			if !isEntry[n] {
				t.Errorf("fixture %q covers %q, which is not a ui entry", fx.Name, n)
			}
			covered[n] = true
		}
	}
	var missing, stale []string
	for _, e := range entries {
		switch {
		case !covered[e] && !perfCoveragePending[e]:
			missing = append(missing, e)
		case covered[e] && perfCoveragePending[e]:
			stale = append(stale, e)
		}
	}
	for n := range perfCoveragePending {
		if !isEntry[n] {
			stale = append(stale, n)
		}
	}
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("%d ui entries have no perf fixture (register one, Covers it, or add a Skip fixture): %s",
			len(missing), strings.Join(missing, ", "))
	}
	if len(stale) > 0 {
		t.Errorf("remove from perfCoveragePending (covered now, or no longer an entry): %s", strings.Join(stale, ", "))
	}
}

// perfLive returns the fixtures that render something.
func perfLive() []perfFixture {
	var out []perfFixture
	for _, fx := range perfFixtures() {
		if fx.Skip == "" {
			out = append(out, fx)
		}
	}
	return out
}

func TestPerfIdle(t *testing.T) {
	for _, fx := range perfLive() {
		t.Run(fx.Name, func(t *testing.T) {
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			if fx.Animated != "" {
				t.Logf("animates at rest (%s)", fx.Animated)
				return
			}
			if d.NeedsFrame() {
				t.Errorf("NeedsFrame after Settle (invalid=%v animating=%v)", d.w.invalid.Load(), d.r.Animating())
			}
		})
	}
}

func rectArea(r geom.Rect) float32 {
	if r.Empty() {
		return 0
	}
	return r.W * r.H
}

func TestPerfDamage(t *testing.T) {
	for _, fx := range perfLive() {
		if fx.Interact == nil {
			continue
		}
		t.Run(fx.Name, func(t *testing.T) {
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			r := &perfRun{D: d}
			before := perfContentRects(d)
			perfStep(r, fx.Interact, 0)
			// The denominator is the area the fixture shows before and after
			// the step (union of its parts), not their bounding rect: parts
			// far apart (a scroll box top-left, a float button bottom-right)
			// would otherwise span most of the window.
			box := perfContentArea(append(before, perfContentRects(d)...))
			dirty := perfDirtyArea(d)
			ratio := float32(-1) // no box: any damage is too much
			if box > 0 {
				ratio = dirty / box
			}
			over := dirty > 2*box
			debt, inDebt := perfDamageDebt[fx.Name]
			switch {
			case over && !inDebt:
				t.Errorf("interaction frame dirty %.0f px² = %.2f× bbox %.0f px² (budget 2×); fix or add to perfDamageDebt", dirty, ratio, box)
			case !over && inDebt:
				t.Errorf("dirty %.2f× bbox is within budget now (debt says %s): remove %q from perfDamageDebt", ratio, debt.Measured, fx.Name)
			default:
				t.Logf("dirty %.0f px² = %.2f× bbox %.0f px²", dirty, ratio, box)
			}
		})
	}
}

func TestPerfInteractSteady(t *testing.T) {
	for _, fx := range perfLive() {
		t.Run(fx.Name, func(t *testing.T) {
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			h0 := perfListHash(d)
			d.Frame()
			if h := perfListHash(d); h != h0 {
				t.Fatalf("two unchanged frames paint different lists (%x, %x)", h0, h)
			}
			if fx.Interact == nil {
				return
			}
			r := &perfRun{D: d}
			perfStep(r, fx.Interact, 0)
			h1 := perfListHash(d)
			perfStep(r, fx.Interact, 1)
			h2 := perfListHash(d)
			if h1 == h0 {
				t.Errorf("Interact(0) changed nothing on screen")
			}
			if h2 != h0 {
				t.Errorf("Interact(1) does not restore the settled screen")
			}
		})
	}
}

// perfGlyphOutline supplies an actual outline for the hash regression:
// equal source types and glyph IDs need not mean equal glyph shapes.
type perfGlyphOutline struct{ path *paint.Path }

func (s *perfGlyphOutline) GlyphPath(uint32, float32) *paint.Path { return s.path }

func TestPerfListHash(t *testing.T) {
	hash := func(ops ...paint.Op) uint64 {
		return perfListHash(&Driver{r: &Runner{list: paint.List{Ops: ops}}})
	}
	fill := paint.FillRect{Rect: geom.Rect{W: 40, H: 30}, Color: color.NRGBA{R: 200, A: 255}}
	for _, hint := range []paint.Op{
		paint.Damage{Rect: geom.Rect{W: 20, H: 20}},
		paint.ScrollRegion{Rect: geom.Rect{W: 20, H: 20}, Delta: geom.Point{Y: 4}},
	} {
		t.Run(fmt.Sprintf("Hints/%T", hint), func(t *testing.T) {
			if hash(fill, hint) != hash(fill) {
				t.Error("presentation hints changed the visual hash")
			}
		})
	}
	t.Run("ImagePixels", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		op := paint.Image{Img: img, Rect: geom.Rect{W: 20, H: 20}}
		h0 := hash(op)
		img.SetRGBA(1, 1, color.RGBA{R: 255, A: 255})
		if hash(op) == h0 {
			t.Error("different image pixels with the same bounds have the same hash")
		}
		copy := image.NewRGBA(img.Bounds())
		copy.SetRGBA(1, 1, color.RGBA{R: 255, A: 255})
		if hash(paint.Image{Img: copy, Rect: op.Rect}) != hash(op) {
			t.Error("reallocated identical image changed the visual hash")
		}
	})
	t.Run("GlyphOutlines", func(t *testing.T) {
		text := func(width float32) paint.Text {
			p := paint.NewPath().MoveTo(0, 0).LineTo(width, -10).LineTo(0, -10).Close()
			return paint.Text{Color: color.NRGBA{A: 255}, Glyphs: []paint.Glyph{
				{Src: &perfGlyphOutline{path: p}, ID: 1, Size: 14},
			}}
		}
		if hash(text(4)) == hash(text(8)) {
			t.Error("same source type and glyph ID hid different glyph outlines")
		}
		if hash(text(4)) != hash(text(4)) {
			t.Error("reallocated identical glyph outline changed the visual hash")
		}
		// Empty glyphs (spaces) and an absent source must not panic.
		_ = hash(paint.Text{Glyphs: []paint.Glyph{{Src: &perfGlyphOutline{}}, {}}})
	})
}

func TestPerfInteractSteadyWithDamage(t *testing.T) {
	changed, declare := false, false
	box := geom.Rect{X: 24, Y: 24, W: 80, H: 40}
	d := newPerfDriver(perfPage(func(c *Context) {
		col := color.NRGBA{R: 200, A: 255}
		if changed {
			col = color.NRGBA{B: 200, A: 255}
		}
		Div(c, layout.Size(80, 40)).Bg(col)
		if declare {
			c.Damage(box)
		}
	}), false)
	d.Settle()
	h0 := perfListHash(d) // baseline has no presentation hints
	r := &perfRun{D: d}
	act := func(r *perfRun, i int) {
		changed, declare = i%2 == 0, true
		r.D.r.Invalidate()
	}
	perfStep(r, act, 0)
	if perfListHash(d) == h0 {
		t.Error("visual change was not detected with damage declarations")
	}
	perfStep(r, act, 1)
	if perfDirtyArea(d) > 2*rectArea(box) || d.r.lastDirty == nil {
		t.Fatal("regression must exercise a partial redraw after undo")
	}
	if perfListHash(d) != h0 {
		t.Error("undo restored the visuals but different presentation hints changed the hash")
	}
}

func TestPerfThemesRTL(t *testing.T) {
	variants := []struct {
		name string
		th   *theme.Theme
		rtl  bool
	}{
		{"element", theme.ElementPlus(), false},
		{"shadcn", theme.Shadcn(), false},
		{"rtl", nil, true},
	}
	for _, fx := range perfLive() {
		if fx.Interact == nil {
			continue
		}
		for _, v := range variants {
			t.Run(fx.Name+"/"+v.name, func(t *testing.T) {
				build := fx.Build
				if v.rtl {
					build = func(c *Context) { ConfigProvider(c).Direction(RTL).Body(func() { fx.Build(c) }) }
				}
				d := newPerfDriver(perfPage(build), false)
				if v.th != nil {
					d.SetTheme(v.th)
				}
				d.Settle()
				h0 := perfListHash(d)
				r := &perfRun{D: d}
				perfStep(r, fx.Interact, 0)
				perfStep(r, fx.Interact, 1)
				if h2 := perfListHash(d); h2 != h0 {
					t.Errorf("Interact(0)+Interact(1) does not restore the settled screen")
				}
			})
		}
	}
}

// perfBudget is the hard budget of a scene (Global Constraints); 0 = report only.
func perfBudget(fx perfFixture, scene string, large1k time.Duration) time.Duration {
	switch scene {
	case "Mount":
		if fx.Overlay {
			return 2 * time.Millisecond
		}
		return 500 * time.Microsecond
	case "Interact":
		return 2 * time.Millisecond
	case "Large100k":
		b := 4 * time.Millisecond
		if large1k > 0 {
			b = min(b, large1k*3/2)
		}
		return b
	}
	return 0
}

// perfTime times one scene by hand (testing.Benchmark is too slow for
// hundreds of fixtures): warm steps, then a fixed number of timed steps,
// median per-step time of three runs.
func perfTime(sc perfScene) time.Duration {
	warm, n := 20, 200
	if sc.Name == "Mount" {
		n = 50
	}
	var runs [3]time.Duration
	for k := range runs {
		step, _ := sc.Setup()
		for i := 0; i < warm; i++ {
			step(i)
		}
		t0 := time.Now()
		for i := 0; i < n; i++ {
			step(warm + i)
		}
		runs[k] = time.Since(t0) / time.Duration(n)
	}
	slices.Sort(runs[:])
	return runs[1]
}

func TestPerfBudgets(t *testing.T) {
	if os.Getenv("TANGO_PERF_BUDGET") != "1" {
		t.Skip("set TANGO_PERF_BUDGET=1 to check the time budgets")
	}
	for _, fx := range perfLive() {
		var large1k time.Duration
		for _, sc := range perfScenes(fx, false) {
			if sc.Name == "MountCold" || sc.Name == "Unchanged" {
				continue
			}
			ns := perfTime(sc)
			if sc.Name == "Large1k" {
				large1k = ns
			}
			budget := perfBudget(fx, sc.Name, large1k)
			if budget == 0 {
				continue
			}
			key := fx.Name + "/" + sc.Name
			over := ns > budget
			status := "ok"
			if over {
				status = "over"
			}
			fmt.Printf("PERF %s %d %d %s\n", key, ns.Nanoseconds(), budget.Nanoseconds(), status)
			debt, inDebt := perfTimeDebt[key]
			switch {
			case over && !inDebt:
				t.Errorf("%s: %v over budget %v; fix or add to perfTimeDebt", key, ns, budget)
			case !over && inDebt:
				t.Errorf("%s: %v within budget %v now (debt says %s): remove it from perfTimeDebt", key, ns, budget, debt.Measured)
			}
		}
	}
}

// TestPerfDamageUnionArea checks the damage denominator on parts far
// apart: a 40×40 box at the top-left and one at the bottom-right. Their
// bounding rect covers most of the window, so a full-window repaint would
// stay under 2× of it; against the union of the parts it must not.
func TestPerfDamageUnionArea(t *testing.T) {
	d := newPerfDriver(perfPage(func(c *Context) {
		Div(c, layout.Size(40, 40)).Bg(color.NRGBA{R: 200, A: 255})
		Div(c, layout.Size(40, 40), layout.Margin(geom.Insets{Top: 780, Left: 1330})).Bg(color.NRGBA{B: 200, A: 255})
	}), false)
	d.Settle()
	rs := perfContentRects(d)
	area := perfContentArea(rs)
	if want := float32(2 * 40 * 40); area != want {
		t.Errorf("content area %.0f px², want %.0f (two 40×40 boxes)", area, want)
	}
	sz := d.r.win.Size()
	if full := sz.W * sz.H; full <= 2*area {
		t.Errorf("a full-window repaint (%.0f px²) is within 2× of the content area %.0f px²: the gate cannot fail", full, area)
	}
	// Overlaps count once.
	r := geom.Rect{X: 10, Y: 10, W: 20, H: 20}
	if got := perfContentArea([]geom.Rect{r, r, {X: 20, Y: 10, W: 20, H: 20}}); got != 600 {
		t.Errorf("overlapping rects area %.0f, want 600", got)
	}
}

// TestPerfBBoxVisibleContent checks that the damage bbox and the default
// scroll point follow what an overlay shows, not its carrier layer: some
// components build a transparent, input-transparent full-window layer
// (ScreenFill, no hit, no block) and place their content inside it.
func TestPerfBBoxVisibleContent(t *testing.T) {
	sz := geom.Size{W: 1440, H: 900}
	view := sz.W * sz.H
	run := func(name string, steps int) *perfRun {
		fx := perfRegistry[name]
		d := newPerfDriver(perfPage(fx.Build), false)
		d.Settle()
		r := &perfRun{D: d}
		for i := 0; i < steps; i++ {
			perfStep(r, fx.Interact, i)
		}
		r.Target = perfTarget(d)
		return r
	}
	t.Run("FloatButton", func(t *testing.T) {
		r := run("FloatButton", 0)
		if a := rectArea(perfBBox(r.D)); a <= 0 || a >= view/4 {
			t.Fatalf("bbox %v of a lone float button covers %.0f%% of the window", perfBBox(r.D), 100*a/view)
		}
	})
	t.Run("ContextMenuAt", func(t *testing.T) {
		r := run("ContextMenuAt", 1) // menu open
		item, ok := r.D.FindText("复制")
		if !ok {
			t.Fatal("context menu not open")
		}
		box := perfBBox(r.D)
		first, _ := r.D.FindText("撤销")
		last, _ := r.D.FindText("删除行")
		// Target is the full-width row of the area and its status line
		// (top 160px); the menu adds its panel below the pointer.
		if box.H >= sz.H/2 || box.Bottom() > last.Bottom()+16 {
			t.Errorf("bbox %v of an area with its context menu reaches beyond the menu (%.0f%% of the window)", box, 100*rectArea(box)/view)
		}
		if box.Intersect(item) != item {
			t.Errorf("bbox %v does not contain the menu item %v", box, item)
		}
		panel := first.Union(last)
		if p := perfScrollPoint(r); !panel.Contains(p) {
			t.Errorf("scroll point %v is not on the menu panel %v", p, panel)
		}
	})
	t.Run("Modal", func(t *testing.T) {
		r := run("Modal", 0)
		if box := perfBBox(r.D); box != (geom.Rect{W: sz.W, H: sz.H}) {
			t.Fatalf("modal mask bbox %v, want the window", box)
		}
	})
	t.Run("Select", func(t *testing.T) {
		r := run("Select", 1) // dropdown open
		opt, ok := r.D.FindText("定时调度")
		if !ok {
			t.Fatal("select list not open")
		}
		if box := perfBBox(r.D); box.Intersect(opt) != opt {
			t.Fatalf("bbox %v does not contain the open list option %v", box, opt)
		}
	})
}
