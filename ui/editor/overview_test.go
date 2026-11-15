package editor

import (
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"image/color"
	"testing"
)

func TestOverviewDragAndLaneMasks(t *testing.T) {
	goText(t)
	st := NewState(manyLines(200), "a.txt", theme.Base("vs"))
	defer st.Close()
	red := color.NRGBA{255, 0, 0, 255}
	st.SetOverviewMarks("test", []OverviewMark{{Line: 30, Color: red, Lane: 1 | 4}})
	d, vw := viewDriver(st, 640, 300, nil)
	img := d.Frame()
	r := (*vw).vs.overviewRect
	laneW := (r.W - 1) / 3
	lane := func(k int) geom.Rect {
		return geom.Rect{X: r.X + 1 + float32(k)*laneW + .5, Y: r.Y + 40, W: laneW - 1, H: 12}
	}
	if countColor(img, lane(0), red) == 0 || countColor(img, lane(1), red) != 0 || countColor(img, lane(2), red) == 0 {
		t.Fatal("lane mask 1|4 must leave middle empty")
	}
	want := int((float32(st.viewCount()-1)*st.Viewport.LineHeight + st.Viewport.Height) * .9 / st.Viewport.LineHeight)
	d.Drag(geom.Pt(r.X+5, r.Y+r.H*.2), geom.Pt(r.X+5, r.Y+r.H*.9))
	d.Frame()
	if line := st.peekRowOf(st.Primary().Head, st.caretUp()); line != want {
		t.Fatalf("overview drag row %d want %d", line, want)
	}
}

func TestOverviewMarks(t *testing.T) {
	goText(t)
	st := NewState(manyLines(200), "a.txt", theme.Base("vs"))
	defer st.Close()
	other := NewView(st.Model, st.Theme)
	defer other.Close()
	red := color.NRGBA{255, 0, 0, 255}
	blue := color.NRGBA{0, 0, 255, 255}
	marks := []OverviewMark{{Line: 50, Color: red, Lane: 1 | 4}}
	st.SetOverviewMarks("find", marks)
	marks[0].Line = 100
	st.SetOverviewMarks("diagnostics", []OverviewMark{{Line: 100, Color: blue, Lane: 2}})
	if len(other.overview) != 0 || st.overview["find"][0].Line != 50 {
		t.Fatal("marks alias input or another view")
	}
	st.SetOverviewMarks("find", []OverviewMark{{Line: 30, Color: red, Lane: 4}})
	if len(st.overview["find"]) != 1 || st.overview["find"][0].Line != 30 || len(st.overview["diagnostics"]) != 1 {
		t.Fatal("owner replacement")
	}
	d, vw := viewDriver(st, 640, 300, nil)
	img := d.Frame()
	r := (*vw).vs.overviewRect
	if countColor(img, r, red) == 0 || countColor(img, r, blue) == 0 {
		t.Fatal("marks not painted")
	}
	want := int((float32(st.viewCount()-1)*st.Viewport.LineHeight + st.Viewport.Height) * .75 / st.Viewport.LineHeight)
	d.Click(geom.Pt(r.X+r.W/2, r.Y+r.H*.75))
	d.Frame()
	if line := st.peekRowOf(st.Primary().Head, st.caretUp()); line != want {
		t.Fatalf("click jumped to row %d want %d", line, want)
	}
	st.SetOverviewMarks("find", nil)
	d.Frame()
	if len(st.overview["find"]) != 0 || len(st.overview["diagnostics"]) != 1 {
		t.Fatal("owner clear")
	}
	o := DefaultOptions()
	if o.OverviewRulerLanes != 3 || !o.OverviewRulerBorder {
		t.Fatal("overview defaults")
	}
}
