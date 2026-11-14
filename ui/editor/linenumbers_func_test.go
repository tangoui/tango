package editor

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// TestLineNumbersFunc: the labels replace the numbers (digits composed,
// other characters shaped), lines without a label show none, maxChars
// sets the numbers' column, and nil restores the numbers.
func TestLineNumbersFunc(t *testing.T) {
	goText(t)
	st := NewState(manyLines(30), "a.txt", theme.Base("vs"))
	labels := func(line int) (string, bool) {
		switch {
		case line%5 == 4:
			return "", false
		case line == 2:
			return "−7", true
		}
		return fmt.Sprint(1000 + line), true
	}
	chars := 5
	var fn LineNumberFunc = labels
	h := newHintHostWith(t, st, geom.Size{W: 480, H: 300}, 1, func(w *ViewW) { w.LineNumbersFunc(fn, chars, 1) })
	h.frame()
	h.frame()
	g := h.vw.vs
	fg := defaultColor(themeOf(st), "editorLineNumber.foreground")
	act := defaultColor(themeOf(st), "editorLineNumber.activeForeground")
	ops := h.r.List().Ops
	nums := append(textsOf(ops, fg), textsOf(ops, act)...)
	at := map[float32]bool{}
	for _, n := range nums {
		if n.Origin.X == 0 {
			at[n.Origin.Y] = true
		}
	}
	lineY := func(l int) float32 { return g.text.Y + st.Viewport.PadTop + float32(l)*st.Viewport.LineHeight }
	for l := range 12 {
		_, want := labels(l)
		if at[lineY(l)] != want {
			t.Fatalf("line %d: label drawn %v, want %v", l, at[lineY(l)], want)
		}
	}
	gw5 := g.gutter.W
	chars = 9
	h.frame()
	if gw := h.vw.vs.gutter.W; gw <= gw5 {
		t.Fatalf("gutter %v with 9 chars, %v with 5", gw, gw5)
	}
	fn = nil
	h.frame()
	if gw := h.vw.vs.gutter.W; gw != gw5 {
		t.Fatalf("gutter %v after restoring the numbers, want %v", gw, gw5)
	}
}

// TestLineNumbersFuncDamage runs labels through the strict verifier: a
// label that changes by itself damages its row; a new generation redraws
// the view.
func TestLineNumbersFuncDamage(t *testing.T) {
	goText(t)
	st := NewState(hintDoc(60), "main.go", theme.Base("vs-dark"))
	shift, gen := 0, uint64(1)
	h := newHintHostWith(t, st, geom.Size{W: 480, H: 360}, 1, func(w *ViewW) {
		w.LineNumbersFunc(func(line int) (string, bool) {
			if line == 5 {
				return fmt.Sprint(500 + shift), true
			}
			return fmt.Sprint(line + 1), true
		}, 4, gen)
	})
	h.focusAt(2)
	shift++
	h.step("one label")
	dam, _ := h.lineHints()
	lh := st.Viewport.LineHeight
	for _, r := range dam {
		if r.H > lh+2*rowPad(lh)+0.5 && r.X < h.vw.vs.overviewRect.X {
			t.Fatalf("one label changed: damage %v", r)
		}
	}
	gen++
	if h.frame() {
		t.Fatal("new generation drawn partially")
	}
	for range 3 {
		h.typeText("x")
		h.step("type")
	}
	h.key(input.KeyDown, 0)
	h.step("down")
	h.verified()
}

// newHintHostWith is newHintHost with the View's modifiers mods.
func newHintHostWith(t *testing.T, st *State, size geom.Size, scale float32, mods func(*ViewW)) *hintHost {
	t.Helper()
	h := &hintHost{t: t, st: st, w: &hintWindow{size: size, scale: scale}, dev: cpuexec.NewDevice(),
		now: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)}
	h.dev.SampledOnly = true
	h.trace = &navTraceDevice{Device: h.dev}
	h.ren = gpu.NewRenderer(h.trace)
	h.ren.SetVerify(true)
	h.ren.SetVerifyStrict(true)
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, st)
		mods(h.vw)
	}, ui.RunnerOptions{Renderer: h.ren, NoAnimations: true, Clock: func() time.Time { return h.now }})
	h.r.Attach(h.w)
	return h
}
