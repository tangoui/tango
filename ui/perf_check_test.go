package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
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
			if d.NeedsFrame() {
				if fx.Animated != "" {
					t.Logf("animates at rest (%s)", fx.Animated)
					return
				}
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
			before := perfBBox(d)
			perfStep(r, fx.Interact, 0)
			box := rectArea(before.Union(perfBBox(d)))
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
