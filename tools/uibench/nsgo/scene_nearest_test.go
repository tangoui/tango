package main

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// runScene runs a scene headless (nsgotest) at its size for n frames
// of 16.7 ms.
func runScene(t *testing.T, name string, n int) *nsgotest.App {
	t.Helper()
	sc, ok := scenes()[name]
	if !ok {
		t.Fatalf("no scene %q", name)
	}
	root := func(cx *core.Scope) core.Node {
		node := sc.root(cx)
		cx.SetLocale("en")
		return node
	}
	app := nsgotest.Start(t, root, nsgotest.Size(sc.width, sc.height))
	for range n {
		app.Advance(16667 * time.Microsecond) // a display frame
	}
	return app
}

// The nearest-counterpart scenes show what scenes.md says frame f shows
// (nsgotest finds the texts a user sees).
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
	for _, tc := range cases {
		app := runScene(t, tc.scene, tc.frames)
		for _, s := range tc.want {
			if !app.Exists("text:" + s) {
				t.Errorf("%s after %d frames: %q not shown", tc.scene, tc.frames, s)
			}
		}
		for _, s := range tc.absent {
			if app.Exists("text:" + s) {
				t.Errorf("%s after %d frames: %q shown", tc.scene, tc.frames, s)
			}
		}
	}
}

// The table scene's view holds the header and tableShown rows: DataTable
// builds the rows in view only, so the last is there and the next is not;
// the ProTable's 6 rows all show.
func TestTableShowsItsRows(t *testing.T) {
	app := runScene(t, "table", 1)
	if !app.Exists("text:" + tableCell(tableShown-1, 0)) {
		t.Errorf("row %d not shown", tableShown)
	}
	if app.Exists("text:" + tableCell(tableShown, 0)) {
		t.Errorf("row %d shown, want the view to hold %d rows", tableShown+1, tableShown)
	}
	if app := runScene(t, "protable", 1); !app.Exists("text:" + tableCell(proRows-1, 0)) {
		t.Errorf("protable row %d not shown", proRows)
	}
}

// After frame f, the controls the scenes change show the state their
// contract gives, read from the views' accessibility properties. The
// Stepper reports its current step only through StatusText, which the
// scene does not set (it would add work to NSGo's frames), so steps is
// left to the work cadence check.
func TestNearestSceneState(t *testing.T) {
	const f = 23
	var got, want []string
	for _, v := range runScene(t, "rate", f).Query("role:adjustable") {
		got = append(got, strconv.Itoa(int(v.Props.Value.Now)))
	}
	for i := range rateCount {
		want = append(want, strconv.Itoa((rateStart(i)+stepsTaken(i, f, rateCount))%(rateMax+1)))
	}
	if !slices.Equal(got, want) {
		t.Errorf("rate after %d frames: %q, want %q", f, got, want)
	}
	got, want = nil, nil
	for _, v := range runScene(t, "segmented", f).Query("role:tab") {
		if v.Props.Selected {
			got = append(got, v.Props.Label)
		}
	}
	for i := range segCount {
		want = append(want, segLabels[(segStart(i)+stepsTaken(i, f, segCount))%segChoices])
	}
	if !slices.Equal(got, want) {
		t.Errorf("segmented after %d frames: %q, want %q", f, got, want)
	}
	got = nil
	for _, v := range runScene(t, "calendar", f).Query("role:button") {
		if v.Props.Selected {
			got = append(got, v.Props.Label)
		}
	}
	if w := pickDay(f).Format("2006-01-02"); !slices.Equal(got, []string{w}) {
		t.Errorf("calendar after %d frames: selected %q, want %q", f, got, w)
	}
	got, want = runScene(t, "badges", f).Texts(), nil
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

// Every component scene reports once per frame callback, with the work
// sceneWork gives. The tree scene is left out: nsgotest's headless model
// takes about 40 s over its 10,000 rows (a cost of the test host, not of
// the scene), and its work is 1 every frame like the others.
func TestWorkCadence(t *testing.T) {
	const frames = 65
	for name := range scenes() {
		if sceneWork(name, 1) < 0 || name == "tree" {
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
