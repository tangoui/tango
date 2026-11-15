package editor

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// zoneRowsBrute returns the view rows of st by brute force: per row, the
// model line and the row (find's terms), from the zones' current lines,
// the hidden lines and the lines' text rows.
func zoneRowsBrute(st *State, owners []string) [][2]int {
	vl := &st.vl
	n := vl.n
	type pz struct{ line, rows int }
	var zs []pz
	sort := slices.Clone(owners)
	slices.Sort(sort)
	for _, o := range sort {
		for _, z := range st.ViewZones(o) {
			l := min(z.AfterLine, n-1)
			if l >= 0 && vl.hidden(l) {
				if !z.ShowInHidden {
					continue
				}
				for l >= 0 && vl.hidden(l) {
					l--
				}
			}
			if z.Rows > 0 {
				zs = append(zs, pz{l, z.Rows})
			}
		}
	}
	after := map[int]int{}
	for _, z := range zs {
		after[z.line] += z.rows
	}
	first := 0
	for first < n-1 && vl.hidden(first) {
		first++
	}
	var out [][2]int
	for k := range after[-1] {
		out = append(out, [2]int{first, k - after[-1]})
	}
	for l := range n {
		if vl.hidden(l) {
			continue
		}
		rows := vl.rowsOf(l) + after[l]
		for r := range rows {
			out = append(out, [2]int{l, r})
		}
	}
	return out
}

// TestViewZonesMapping: random zones (before the first line, after hidden
// lines, with and without ShowInHidden), folds, hidden areas, wrapping and
// edits: find, viewOf and Count agree with a brute force layout, and the
// laid out zone rows are the zones'.
func TestViewZonesMapping(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrap=%v", wrap), func(t *testing.T) {
			r := rand.New(rand.NewPCG(5, 8))
			var b strings.Builder
			b.WriteString(funcsDoc(25))
			for i := range 20 {
				b.WriteString(strings.Repeat("long text ", 3+i%9) + "\n")
			}
			st := NewState(b.String(), "main.go", nil)
			other := NewView(st.Model, nil)
			defer other.Close()
			if wrap {
				st.setWrap(wrapConfig{col: 30, tab: 4}, 8)
			}
			owners := []string{"a", "b"}
			check := func(step string) {
				t.Helper()
				vl := st.lines()
				if wrap {
					st.measure(0, vl.n-1)
				}
				want := zoneRowsBrute(st, owners)
				if vl.Count() != len(want) {
					t.Fatalf("%s: Count %d, want %d (zr %v lead %d hid %v)", step, vl.Count(), len(want), vl.zr, vl.lead, vl.hid)
				}
				for v, lr := range want {
					l, row := vl.find(v)
					if l != lr[0] || row != lr[1] {
						t.Fatalf("%s: find(%d) = %d, %d, want %d, %d", step, v, l, row, lr[0], lr[1])
					}
					if row == vl.minRow(l) || (row == 0 && vl.minRow(l) == 0) {
						if vl.viewOf(l)+row != v {
							t.Fatalf("%s: viewOf(%d) %d + row %d != %d", step, l, vl.viewOf(l), row, v)
						}
					}
					if _, isZone := st.zoneAt(l, row); isZone != (row < 0 || row >= vl.rowsOf(l)) {
						t.Fatalf("%s: zoneAt(%d, %d) = %v", step, l, row, isZone)
					}
				}
			}
			randZones := func() []ViewZone {
				n := st.Snapshot().LineCount()
				var out []ViewZone
				for i := range r.IntN(6) {
					out = append(out, ViewZone{ID: uint64(i), AfterLine: r.IntN(n+1) - 1, Rows: r.IntN(4), ShowInHidden: r.IntN(2) == 0})
				}
				return out
			}
			for step := range 300 {
				snap := st.Snapshot()
				n := snap.LineCount()
				what := ""
				switch k := r.IntN(10); {
				case k < 3:
					o := owners[r.IntN(2)]
					st.SetViewZones(o, randZones())
					what = "zones " + o
				case k == 3:
					a := r.IntN(n)
					st.SetHiddenAreas("h", []LineRange{{a, a + r.IntN(6)}})
					what = "hidden"
				case k == 4:
					l := 2 + 6*r.IntN(25)
					if l < n {
						st.MoveCaret(snap.LineStart(l), false)
						st.Execute("editor.fold", nil)
					}
					what = "fold"
				case k == 5:
					st.Execute("editor.unfoldAll", nil)
					what = "unfold"
				case k < 8:
					v := []*State{st, other}[r.IntN(2)]
					v.MoveCaret(snap.LineStart(r.IntN(n)), false)
					v.Insert([]string{"x\n", "\n", "yy"}[r.IntN(3)])
					what = "insert"
				default:
					v := []*State{st, other}[r.IntN(2)]
					a := snap.LineStart(r.IntN(n))
					v.SetSelections([]Selection{{Anchor: a, Head: min(snap.Len(), a+1+r.IntN(40))}})
					v.Delete(Backward, Char)
					what = "delete"
				}
				check(fmt.Sprintf("step %d (%s)", step, what))
			}
		})
	}
}

// zoneDoc is a view with numbered lines and a strict verify host.
func zoneHost(t *testing.T, lines int, size geom.Size) (*State, *hintHost) {
	t.Helper()
	goText(t)
	st := NewState(hintDoc(lines), "main.go", theme.Base("vs-dark"))
	h := newHintHost(t, st, size, 1)
	h.frame()
	h.frame()
	return st, h
}

// TestViewZonesLayoutAndInput: a zone takes its rows after its line (and
// before the first line), its Paint sees its rectangle, Up and Down step
// over it, a press calls OnPointer and puts the caret above or below by
// the half pressed (not with SuppressMouseDown), MinWidth widens the
// content, a zone follows Enter at its line's end (it stays after the
// line), and the view state keeps a top inside a zone.
func TestViewZonesLayoutAndInput(t *testing.T) {
	st, h := zoneHost(t, 40, geom.Size{W: 480, H: 400})
	var got []ZonePaint
	var events []ZonePointer
	paintFn := func(l *paint.List, z ZonePaint) {
		got = append(got, z)
		l.Fill(z.Rect, geom.Radii{}, decoRed)
	}
	st.SetViewZones("t", []ViewZone{
		{ID: 1, AfterLine: 2, Rows: 3, Paint: paintFn, Version: 1, MinWidth: 3000,
			OnPointer: func(e ZonePointer) { events = append(events, e) }},
		{ID: 2, AfterLine: -1, Rows: 1, Paint: paintFn, Version: 1, SuppressMouseDown: true},
	})
	h.frame()
	lh := st.Viewport.LineHeight
	g := h.vw.vs
	if c := st.viewCount(); c != st.Snapshot().LineCount()+4 {
		t.Fatalf("view lines %d", c)
	}
	if len(got) != 2 || got[0].Rect.Y != g.text.Y+st.Viewport.PadTop || got[1].Rect.Y != g.text.Y+st.Viewport.PadTop+4*lh || got[1].Rect.H != 3*lh {
		t.Fatalf("zones painted at %+v", got)
	}
	if st.Viewport.ContentWidth < 3000 {
		t.Fatalf("ContentWidth %v, want the zone's MinWidth", st.Viewport.ContentWidth)
	}
	// Line 2 is at view line 3 (after the lead zone), line 3 at 7.
	if st.vl.viewOf(2) != 3 || st.vl.viewOf(3) != 7 {
		t.Fatalf("viewOf(2) %d, viewOf(3) %d", st.vl.viewOf(2), st.vl.viewOf(3))
	}
	snap := st.Snapshot()
	st.MoveCaret(snap.LineStart(2)+3, false)
	st.Move(Forward, Line, false)
	if l := snap.LineOf(st.Primary().Head); l != 3 {
		t.Fatalf("Down from line 2: line %d, want 3", l)
	}
	st.Move(Backward, Line, false)
	st.Move(Backward, Line, false)
	st.Move(Backward, Line, false)
	if l := snap.LineOf(st.Primary().Head); l != 0 {
		t.Fatalf("Up to the top: line %d", l)
	}
	// Presses in the zone's upper and lower halves.
	y := func(view int, frac float32) float32 { return g.text.Y + st.Viewport.PadTop + (float32(view)+frac)*lh }
	h.click(geom.Pt(g.text.X+30, y(4, 0.2)), 0)
	h.frame()
	if hd := st.Primary().Head; hd != lineEnd(snap, 2) {
		t.Fatalf("press in the upper half: caret %d, want line 2's end %d", hd, lineEnd(snap, 2))
	}
	h.now = h.now.Add(time.Second) // not a double click
	h.click(geom.Pt(g.text.X+30, y(6, 0.8)), 0)
	h.frame()
	if hd := st.Primary().Head; hd != snap.LineStart(3) {
		t.Fatalf("press in the lower half: caret %d, want line 3's start %d", hd, snap.LineStart(3))
	}
	if len(events) < 2 || events[0].Kind != input.Press || events[0].Area != ZoneText || events[1].Kind != input.Release {
		t.Fatalf("zone events %+v", events)
	}
	if p := events[0].Pos; absf(p.Y-0.2*lh) > 0.5 || absf(p.X-30) > 0.5 {
		t.Fatalf("press position %v, relative to the zone", p)
	}
	// SuppressMouseDown: the caret stays.
	before := st.Primary().Head
	h.now = h.now.Add(time.Second)
	h.click(geom.Pt(g.text.X+30, y(0, 0.5)), 0)
	h.frame()
	if st.Primary().Head != before {
		t.Fatal("a press on a SuppressMouseDown zone moved the caret")
	}
	// Enter at line 2's end: the zone stays after line 2.
	st.MoveCaret(lineEnd(snap, 2), false)
	st.Insert("\n")
	if z := st.ViewZones("t"); z[0].AfterLine != 2 {
		t.Fatalf("after Enter at the line's end: AfterLine %d, want 2", z[0].AfterLine)
	}
	// Enter at line 2's start: the zone moves down with its line.
	st.MoveCaret(st.Snapshot().LineStart(2), false)
	st.Insert("\n")
	if z := st.ViewZones("t"); z[0].AfterLine != 3 {
		t.Fatalf("after Enter at the line's start: AfterLine %d, want 3", z[0].AfterLine)
	}
	// The top inside the zone survives SaveViewState / RestoreViewState.
	h.frame()
	st.Viewport.TopLine = st.vl.viewOf(3) + 2 // the zone's second row
	st.Viewport.OffsetY = 5
	vs := st.SaveViewState()
	st.Viewport.TopLine, st.Viewport.OffsetY = 0, 0
	st.RestoreViewState(vs)
	if st.Viewport.TopLine != st.vl.viewOf(3)+2 || st.Viewport.OffsetY != 5 {
		t.Fatalf("restored top %d+%v, want %d+5", st.Viewport.TopLine, st.Viewport.OffsetY, st.vl.viewOf(3)+2)
	}
	// Removing the zones restores the identity mapping.
	st.SetViewZones("t", nil)
	if st.vl.mapped() || st.tracks() {
		t.Fatal("mapping not the identity after removing the zones")
	}
}

// TestViewZonesDamageVerified runs view zones through the strict verifier
// (spec 6.8): adding and removing zones (whole redraws), a zone whose
// Version changes, a zone without Version whose drawing changes (the
// editor's fingerprint), a Fixed zone while scrolling horizontally, the
// top stopping inside a zone while scrolling, typing above and inside
// the zones' lines, and a zone in the gutter.
func TestViewZonesDamageVerified(t *testing.T) {
	for _, scale := range []float32{1, 1.5} {
		t.Run(fmt.Sprintf("scale%v", scale), func(t *testing.T) {
			goText(t)
			st := NewState(hintDoc(80), "main.go", theme.Base("vs-dark"))
			h := newHintHost(t, st, geom.Size{W: 480, H: 360}, scale)
			h.focusAt(1)
			stripe := color.NRGBA{0x50, 0x50, 0x80, 0xff}
			label := 0 // drawn by the zone without Version
			ver := 1   // the versioned zone's Version and its drawing
			mark := "" // the fixed zone's text
			stripes := func(l *paint.List, z ZonePaint) {
				for k := range int(z.Rect.H / z.RowH) {
					if k%2 == 0 {
						l.Fill(geom.Rect{X: z.Rect.X, Y: z.Rect.Y + float32(k)*z.RowH, W: z.Rect.W, H: z.RowH}, geom.Radii{}, stripe)
					}
				}
				l.Line(geom.Pt(z.OriginX, z.Rect.Y+2), geom.Pt(z.OriginX+200, z.Rect.Y+z.Rect.H-2), 1, decoOrange)
			}
			zones := func() []ViewZone {
				return []ViewZone{
					{ID: 1, AfterLine: 4, Rows: 3, Version: 1, Paint: stripes, PaintGutter: func(l *paint.List, z ZonePaint) {
						l.Fill(z.Rect, geom.Radii{}, decoTeal)
					}},
					{ID: 2, AfterLine: 7, Rows: 2, Paint: func(l *paint.List, z ZonePaint) {
						l.Fill(geom.Rect{X: z.OriginX + float32(label*7), Y: z.Rect.Y, W: 40, H: z.RowH}, geom.Radii{}, decoRed)
					}},
					{ID: 3, AfterLine: 9, Rows: 1, Version: uint64(ver), Paint: func(l *paint.List, z ZonePaint) {
						l.Fill(geom.Rect{X: z.OriginX + float32(ver*5), Y: z.Rect.Y + 3, W: 30, H: z.RowH - 6}, geom.Radii{}, decoGreen)
					}},
					{ID: 4, AfterLine: 1, Rows: 1, Version: uint64(len(mark) + 1), Fixed: true, Paint: func(l *paint.List, z ZonePaint) {
						l.Fill(geom.Rect{X: z.OriginX + 10, Y: z.Rect.Y + 2, W: float32(20 + 10*len(mark)), H: z.RowH - 4}, geom.Radii{}, decoBlue)
					}},
					{ID: 5, AfterLine: -1, Rows: 1, Version: 1, Paint: stripes},
				}
			}
			st.SetViewZones("diff", zones())
			h.step("add zones")
			for i := range 4 {
				h.typeText(string(rune('a' + i)))
				h.step("type above the zones")
			}
			// A zone without Version draws something else: its rows only.
			label++
			st.SetViewZones("diff", zones())
			h.step("unversioned zone changes")
			label++
			h.step("unversioned zone changes without a SetViewZones")
			ver++
			st.SetViewZones("diff", zones())
			h.step("Version changes")
			p := geom.Pt(240, 180)
			for _, dy := range []float32{84, 8, 16, 12, -40, 4, 24, -8, 32} {
				h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
				h.step(fmt.Sprintf("scroll %v", dy))
			}
			// The top in the middle of a zone (its second row), then
			// scrolling from there.
			st.Viewport.TopLine = st.vl.viewOf(4) + 2
			st.Viewport.OffsetY = 4
			h.step("top inside a zone")
			if l, row := st.topAnchor(); l != 4 || row != 2 {
				t.Fatalf("top at %d, %d, want line 4's second zone row", l, row)
			}
			for _, dy := range []float32{4, 8, 20} {
				h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
				h.step("scroll from inside a zone")
			}
			st.Viewport.TopLine = 0
			st.Viewport.OffsetY = 0
			h.step("top")
			for _, dx := range []float32{16, 24, -8} {
				h.r.HandleEvent(input.Scroll{Pos: p, DX: dx})
				h.step(fmt.Sprintf("scroll x %v (fixed zone)", dx))
			}
			mark = "xx"
			st.SetViewZones("diff", zones())
			h.step("fixed zone text")
			other := NewView(st.Model, theme.Base("vs-dark"))
			defer other.Close()
			other.MoveCaret(other.Snapshot().LineStart(6), false)
			other.Insert("z\n")
			h.step("another view inserts a line before a zone")
			st.SetViewZones("diff", zones()[:2])
			h.step("remove zones")
			st.SetViewZones("diff", nil)
			h.step("remove all")
			h.verified()
		})
	}
}
