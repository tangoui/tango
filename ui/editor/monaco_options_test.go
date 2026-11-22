package editor

import (
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestMonacoGutterLanesAndFoldingFade(t *testing.T) {
	st := NewState("func main() {\n\tprintln(0)\n}\n", "main.go", theme.Base("vs"))
	defer st.Close()
	st.Options.LineNumbersMinChars, st.Options.LineDecorationsWidth = 3, 4
	st.Options.GlyphMargin, st.Options.AnimateFoldingControls = true, true
	st.Options.LineHeight = 21
	st.Options.FontFamily = "Menlo"
	var w *ViewW
	d := ui.NewDriver(func(c *ui.Context) { w = View(c, st) }, ui.DriverOptions{Width: 640, Height: 300, Animations: true})
	d.Frame()
	g := w.vs.gutter
	numW := float32(math.Round(float64(3 * digitsOf(w.monoStyle()).adv)))
	if want := float32(21+16+4) + numW; g.W != want || w.vs.foldX != g.X+21+numW {
		t.Fatalf("gutter width = %v, folding slot = %v, want width %v", g.W, w.vs.foldX, want)
	}
	if line, ok := w.GlyphMarginLineAtPoint(geom.Pt(g.X+10, g.Y+10)); !ok || line != 0 {
		t.Fatalf("glyph slot returned line %d, %t", line, ok)
	}
	if _, ok := w.GlyphMarginLineAtPoint(geom.Pt(g.X+21, g.Y+10)); ok {
		t.Fatal("line number hit the glyph slot")
	}
	if w.foldOpacity != 0 {
		t.Fatal("expanded controls should start hidden")
	}
	d.Move(geom.Pt(w.vs.foldX+8, g.Y+10))
	d.Frame()
	d.Advance(250 * time.Millisecond)
	d.Frame()
	if w.foldOpacity <= 0 || w.foldOpacity >= 1 {
		t.Fatalf("fold opacity at half duration = %v", w.foldOpacity)
	}
	d.Advance(250 * time.Millisecond)
	d.Frame()
	if w.foldOpacity != 1 {
		t.Fatalf("fold opacity at end = %v", w.foldOpacity)
	}
}

func TestMountFindFollowsEditorAndRestoresFocus(t *testing.T) {
	st := NewState("hello", "hello.txt", theme.Base("vs"))
	defer st.Close()
	st.Options.Minimap.Enabled = false
	var w *ViewW
	var panel *ui.Box
	var query string
	open := true
	d := ui.NewDriver(func(c *ui.Context) {
		ui.Row(c, func() {
			ui.Div(c, layout.W(100))
			w = View(c, st)
			if open {
				panel = w.MountFind(func() { ui.Input(c, &query).Placeholder("Find") }, layout.H(34))
			}
		}, layout.Grow(1))
	}, ui.DriverOptions{Width: 640, Height: 240, Accessibility: true})
	d.Frame()
	d.Frame()
	if r, vr := panel.Rect(), w.Box().Rect(); r.W != 419 || r.Y != vr.Y+4 || r.Right() > vr.Right()-28 {
		t.Fatalf("panel %v for editor %v", r, vr)
	}
	d.Click(panel.Rect().Center())
	d.Type("test")
	if query != "test" {
		t.Fatalf("query = %q", query)
	}
	open = false
	d.Frame()
	if f := d.A11yFocused(); f == nil || f.ID != a11y.NodeID(w.Box().ID()) {
		t.Fatalf("focus after close = %+v", f)
	}
}
