package main

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/theme"
)

// runScene builds a scene headless in the 800×600 window for n frames
// after the first (so frame f = n has run) and returns the driver.
func runScene(t *testing.T, name string, n int) *ui.Driver {
	t.Helper()
	sc, ok := scenes()[name]
	if !ok {
		t.Fatalf("no scene %q", name)
	}
	useEnglish()
	d := ui.NewDriver(sc.page, ui.DriverOptions{Width: float32(sc.width), Height: float32(sc.height), Scale: 1, Theme: theme.AntDesign()})
	d.Frame()
	for range n {
		d.Advance(16 * time.Millisecond)
		d.Frame()
	}
	return d
}

// The nearest-counterpart scenes show, in the window, what scenes.md says
// frame f shows (and nothing scrolls it out of view).
func TestNearestScenesShowTheirChange(t *testing.T) {
	p := cascadePath(77)
	g, j := treeSelPick(29)
	cases := []struct {
		scene  string
		frames int
		want   []string
		absent []string
	}{
		{"select", 13, []string{selectLabel(0), selectLabel(selectCount - 1)}, nil},
		{"tooltip", tooltipEvery, []string{tooltipAnchor, tooltipText}, nil},
		{"tooltip", 2 * tooltipEvery, []string{tooltipAnchor}, []string{tooltipText}},
		{"table", 9 * tableShown, []string{tableTitles[5], tableCell(7, 1), strconv.Itoa(9 * tableShown)}, nil},
		{"pagination", 12, []string{pageItem(13, 0), pageItem(13, pageSize-1)}, []string{pageItem(12, 0)}},
		{"cascader", 77, []string{cascadeLabel(p[:2], p[2]), cascadeLabel(p[:2], cascadeFan-1), cascadeLabel(nil, cascadeFan-1)}, nil},
		{"transfer", 3, []string{transferLeft, transferRight, transferLabel(transferCount - 1)}, nil},
		{"treeselect", 29, []string{treeSelLeaf(g, j), treeSelLeaf(treeSelGroups-1, treeSelLeaves-1)}, nil},
		{"calendar", 3, []string{"4"}, nil},
		{"steps", 7, []string{stepTitle(stepCount - 1)}, nil},
		{"timeline", 13, []string{timelineText(13%timelineCount, 13), timelineTime(timelineCount - 1)}, nil},
		{"descriptions", 17, []string{descValue(5, 17), descLabel(descCount - 1)}, nil},
		{"form", 3, []string{formLabel(formFields - 1), formSubmit}, nil},
		{"upload", 3, []string{uploadButton, uploadName(uploadCount - 1)}, nil},
		{"carousel", carouselEvery, []string{slideLabel(1)}, nil},
		{"collapse", collapseEvery + 20, []string{collapseLabel(collapseCount - 1), collapseLine(1, collapseLines-1)}, nil},
		{"drawer", overlayEvery, []string{drawerTitle, dialogOK}, nil},
		{"drawer", 2*overlayEvery + 20, []string{drawerBackdrop}, []string{drawerTitle}},
		{"tour", tourEvery, []string{tourTitles[1], tourDesc(1)}, nil},
		{"protable", 15, []string{queryButton, resetButton, tableCell(proShown-1, 1), "15"}, nil},
		{"segmented", 3, []string{segLabels[3]}, nil},
		{"rate", 3, nil, nil},
		{"tags", 31, []string{taggedLabel(31, 31), tagLabel(tagCount - 1)}, nil},
		{"badges", 3, nil, nil},
		{"cards", 14, []string{cardTitle(cardCount - 1), cardUpdated(14)}, nil},
		{"statistics", 5, []string{statTitle(statCount - 1)}, nil},
	}
	win := struct{ w, h float32 }{800, 600}
	for _, tc := range cases {
		d := runScene(t, tc.scene, tc.frames)
		for _, s := range tc.want {
			r, ok := d.FindText(s)
			if !ok {
				t.Errorf("%s after %d frames: %q not shown", tc.scene, tc.frames, s)
				continue
			}
			if r.X < 0 || r.Y < 0 || r.X+r.W > win.w || r.Y+r.H > win.h {
				t.Errorf("%s after %d frames: %q at %v, outside the window", tc.scene, tc.frames, s, r)
			}
		}
		for _, s := range tc.absent {
			if _, ok := d.FindText(s); ok {
				t.Errorf("%s after %d frames: %q shown", tc.scene, tc.frames, s)
			}
		}
	}
}

// The table scene's view holds the header and tableShown rows: the row
// after them is cut off (its band of pixels is blank), the last one is
// drawn. Tango's virtual table builds a row or two beyond its view, so
// the check is on the pixels rather than on the texts.
func TestTableShowsItsRows(t *testing.T) {
	d := runScene(t, "table", 1)
	img := d.Image()
	first, _ := d.FindText(tableCell(0, 0))
	blank := func(y0 float32) bool {
		bg := img.RGBAAt(4, int(y0)+2)
		for y := int(y0) + 2; y < int(y0+tangoRowH)-2; y++ {
			for x := int(first.X); x < int(first.X)+120; x++ {
				if img.RGBAAt(x, y) != bg {
					return false
				}
			}
		}
		return true
	}
	last := first.Y + (tableShown-1)*tangoRowH
	if blank(last) {
		t.Errorf("row %d (y %v) is blank, want it drawn", tableShown, last)
	}
	if !blank(last + tangoRowH) {
		t.Errorf("row %d (y %v) is drawn, want it cut off: the view holds %d rows", tableShown+1, last+tangoRowH, tableShown)
	}
	// The ProTable's rows all fit under its query form.
	d = runScene(t, "protable", 1)
	if r, ok := d.FindText(tableCell(proRows-1, 1)); !ok || r.Y+r.H > 600 {
		t.Errorf("protable: last row %v (shown %v), want it inside the window", r, ok)
	}
	if _, ok := d.FindText(tableCell(proRows, 1)); ok {
		t.Errorf("protable shows more than %d rows", proRows)
	}
}

// After frame f, the controls the scenes change show the state their
// contract gives, read from Tango's accessibility tree.
func TestNearestSceneState(t *testing.T) {
	const f = 23
	tree := func(scene string) *a11y.Tree {
		sc := scenes()[scene]
		useEnglish()
		d := ui.NewDriver(sc.page, ui.DriverOptions{Width: float32(sc.width), Height: float32(sc.height), Scale: 1, Theme: theme.AntDesign()})
		d.EnableAccessibility()
		d.Frame()
		for range f {
			d.Advance(16 * time.Millisecond)
			d.Frame()
		}
		return d.AccessibilityTree()
	}
	// rate: rating i at rateStart(i) plus the steps it took, mod 6.
	var got, want []string
	for _, n := range tree("rate").FindAll(a11y.RoleSlider, "") {
		got = append(got, n.Value)
	}
	for i := range rateCount {
		want = append(want, strconv.Itoa((rateStart(i)+stepsTaken(i, f, rateCount))%(rateMax+1))+" of 5 stars")
	}
	if !slices.Equal(got, want) {
		t.Errorf("rate after %d frames: %q, want %q", f, got, want)
	}
	// segmented: the checked choice of each control.
	got, want = nil, nil
	for _, n := range tree("segmented").FindAll(a11y.RoleRadio, "") {
		if n.States&a11y.Checked != 0 {
			got = append(got, n.Name)
		}
	}
	for i := range segCount {
		want = append(want, segLabels[(segStart(i)+stepsTaken(i, f, segCount))%segChoices])
	}
	if !slices.Equal(got, want) {
		t.Errorf("segmented after %d frames: %q, want %q", f, got, want)
	}
	// calendar: the selected cell is pickDay(f).
	got = nil
	for _, n := range tree("calendar").FindAll(a11y.RoleCell, "") {
		if n.States&a11y.Selected != 0 {
			got = append(got, n.Name)
		}
	}
	if w := pickDay(f).Format("Jan 2, 2006"); !slices.Equal(got, []string{w}) {
		t.Errorf("calendar after %d frames: selected %q, want %q", f, got, w)
	}
	// steps: the current step is f%5.
	got = nil
	for _, n := range tree("steps").FindAll(a11y.RoleListItem, "") {
		if n.States&a11y.Current != 0 {
			got = append(got, n.Name)
		}
	}
	if w := stepTitle(f % stepCount); !slices.Equal(got, []string{w}) {
		t.Errorf("steps after %d frames: current %q, want %q", f, got, w)
	}
	// badges: each badge's count (its description).
	got, want = nil, nil
	for _, n := range tree("badges").FindAll(a11y.RoleGroup, "") {
		if n.Description != "" {
			got = append(got, n.Description)
		}
	}
	for i := range badgeCount {
		c := badgeStart(i)
		for range stepsTaken(i, f, badgeCount) {
			c = nextCount(c)
		}
		want = append(want, strconv.Itoa(c))
	}
	if !slices.Equal(got, want) {
		t.Errorf("badges after %d frames: %q, want %q", f, got, want)
	}
}

// Every component scene reports one frame per frame after the first, with
// the work sceneWork gives: 1 every frame, or 1 on every period-th frame
// and 0 between (the cadence the judge's work check relies on).
func TestWorkCadence(t *testing.T) {
	const frames = 65
	for name := range scenes() {
		if sceneWork(name, 1) < 0 {
			continue
		}
		var works []int
		reportHook = func(w int) { works = append(works, w) }
		runScene(t, name, frames)
		reportHook = nil
		if len(works) != frames {
			t.Errorf("%s: %d reports over %d frames", name, len(works), frames)
			continue
		}
		for i, w := range works {
			if want := sceneWork(name, i+1); w != want {
				t.Errorf("%s: frame %d reported work %d, want %d", name, i+1, w, want)
				break
			}
		}
	}
}
