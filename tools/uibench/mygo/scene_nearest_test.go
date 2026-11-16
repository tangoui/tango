package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// runScene runs a scene in a tester (no window) of its size for n frames
// after the first, so that frame f = n has run.
func runScene(t *testing.T, name string, n int) *ui.Tester {
	t.Helper()
	sc, ok := scenes()[name]
	if !ok {
		t.Fatalf("no scene %q", name)
	}
	tt := ui.NewTester(sc.view, sc.width, sc.height)
	tt.SetLocale("en")
	tt.Frame()
	for range n {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
	return tt
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
			r, ok := d.Find(s)
			if !ok || r.W <= 0 || r.H <= 0 {
				t.Errorf("%s after %d frames: %q not shown", tc.scene, tc.frames, s)
				continue
			}
			if r.X < 0 || r.Y < 0 || r.X+r.W > win.w || r.Y+r.H > win.h {
				t.Errorf("%s after %d frames: %q at %v, outside the window", tc.scene, tc.frames, s, r)
			}
		}
		for _, s := range tc.absent {
			if r, ok := d.Find(s); ok && r.W > 0 && r.H > 0 {
				t.Errorf("%s after %d frames: %q shown", tc.scene, tc.frames, s)
			}
		}
	}
}
