package mobile

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestRefreshMachine(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := RefreshMachine{Threshold: 60, Delay: 500 * time.Millisecond}
	m.Pull(30)
	if m.Status != RefreshPulling {
		t.Fatal(m.Status)
	}
	if m.Release() || m.Status != RefreshIdle {
		t.Fatal("short pull must not refresh")
	}
	m.Pull(70)
	if m.Status != RefreshCanRelease {
		t.Fatal(m.Status)
	}
	m.Pull(40) // back above the threshold
	if m.Status != RefreshPulling {
		t.Fatal(m.Status)
	}
	m.Pull(80)
	if !m.Release() || m.Status != RefreshRefreshing || !m.Holding() {
		t.Fatal("release past threshold must refresh")
	}
	m.Pull(10) // ignored while refreshing
	if m.Status != RefreshRefreshing {
		t.Fatal(m.Status)
	}
	m.Finish(t0)
	if m.Status != RefreshComplete {
		t.Fatal(m.Status)
	}
	if next := m.Tick(t0.Add(100 * time.Millisecond)); next.IsZero() || m.Status != RefreshComplete {
		t.Fatal("complete should linger")
	}
	m.Tick(t0.Add(600 * time.Millisecond))
	if m.Status != RefreshIdle {
		t.Fatal(m.Status)
	}
}

func TestPullToRefreshGesture(t *testing.T) {
	refreshing := false
	pulled := 0
	var sv *ScrollViewW
	d := phone(func(c *ui.Context) {
		sv = PullToRefresh(c, &refreshing).Layout(layout.H(600)).Body(func() {
			for i := 0; i < 5; i++ {
				ui.Key(c, i, func() { Cell(c, fmt.Sprint("row ", i)) })
			}
		})
		if sv.Refreshed() {
			pulled++
		}
	}, true)
	d.Frame()
	d.Frame()
	f := newFinger(t, d)
	// A short pull springs back without refreshing.
	f.down(geom.Pt(200, 100))
	f.move(geom.Pt(200, 140), 4, 16*time.Millisecond)
	if sv.Status() != RefreshPulling || sv.ScrollOffset() >= 0 {
		t.Fatalf("pulling: %v %v", sv.Status(), sv.ScrollOffset())
	}
	f.hold(200 * time.Millisecond)
	f.up()
	run(d, time.Second)
	if pulled != 0 || sv.ScrollOffset() != 0 || sv.Status() != RefreshIdle {
		t.Fatalf("short pull: %d %v %v", pulled, sv.ScrollOffset(), sv.Status())
	}
	// A long pull arms and refreshes; the header stays open.
	f.down(geom.Pt(200, 100))
	f.move(geom.Pt(200, 400), 10, 16*time.Millisecond)
	if sv.Status() != RefreshCanRelease {
		t.Fatalf("armed: %v", sv.Status())
	}
	f.hold(200 * time.Millisecond)
	f.up()
	run(d, time.Second)
	if pulled != 1 || !refreshing || sv.Status() != RefreshRefreshing {
		t.Fatalf("refresh: %d %v %v", pulled, refreshing, sv.Status())
	}
	if off := sv.ScrollOffset(); off != -sv.headHeight() {
		t.Fatalf("header held at %v", off)
	}
	// Data loaded: complete, then the header closes.
	refreshing = false
	d.Frame()
	if sv.Status() != RefreshComplete {
		t.Fatalf("complete: %v", sv.Status())
	}
	run(d, 1500*time.Millisecond)
	if sv.Status() != RefreshIdle || sv.ScrollOffset() != 0 {
		t.Fatalf("closed: %v %v", sv.Status(), sv.ScrollOffset())
	}
}

func TestScrollViewInertia(t *testing.T) {
	var sv *ScrollViewW
	d := phone(func(c *ui.Context) {
		sv = ScrollView(c).Layout(layout.H(600)).Body(func() {
			for i := 0; i < 100; i++ {
				ui.Key(c, i, func() { Cell(c, fmt.Sprint("row ", i)) })
			}
		})
	}, true)
	d.Frame()
	d.Frame()
	f := newFinger(t, d)
	f.down(geom.Pt(200, 500))
	f.move(geom.Pt(200, 300), 5, 16*time.Millisecond) // fast flick up
	f.up()
	atRelease := sv.ScrollOffset()
	if atRelease < 150 {
		t.Fatalf("drag followed finger: %v", atRelease)
	}
	var last float64
	var steps []float64
	for i := 0; i < 200; i++ {
		d.Advance(16 * time.Millisecond)
		d.Frame()
		steps = append(steps, sv.ScrollOffset()-last)
		last = sv.ScrollOffset()
	}
	if last < atRelease+500 {
		t.Fatalf("fling travelled only to %v from %v", last, atRelease)
	}
	if sv.Scrolling() {
		t.Fatal("fling never stopped")
	}
	// Deceleration: later steps are smaller than early ones.
	if steps[5] <= steps[40] {
		t.Fatalf("not decelerating: %v vs %v", steps[5], steps[40])
	}
	// A touch catches the fling.
	f.down(geom.Pt(200, 500))
	f.move(geom.Pt(200, 200), 3, 16*time.Millisecond)
	f.up()
	d.Advance(50 * time.Millisecond)
	d.Frame()
	f.down(geom.Pt(200, 400))
	f.hold(16 * time.Millisecond)
	caught := sv.ScrollOffset()
	f.hold(100 * time.Millisecond)
	if sv.ScrollOffset() != caught {
		t.Fatalf("touch did not stop fling: %v -> %v", caught, sv.ScrollOffset())
	}
	f.up()
}

func TestInfiniteListLoads(t *testing.T) {
	loading := false
	rows := 10
	loads := 0
	d := phone(func(c *ui.Context) {
		l := InfiniteList(c, &loading).Finished(rows >= 30).Layout(layout.H(600)).Body(func() {
			for i := 0; i < rows; i++ {
				ui.Key(c, i, func() { Cell(c, fmt.Sprint("row ", i)) })
			}
		})
		if l.LoadMore() {
			loads++
		}
	}, false)
	d.Frame()
	d.Frame()
	if loads != 0 {
		t.Fatalf("loaded before reaching the end: %d", loads)
	}
	f := newFinger(t, d)
	f.down(geom.Pt(200, 550))
	f.move(geom.Pt(200, 50), 5, 16*time.Millisecond)
	f.up()
	d.Settle()
	if loads != 1 || !loading {
		t.Fatalf("load more: %d %v", loads, loading)
	}
	rows, loading = 30, false
	d.Settle()
	f.down(geom.Pt(200, 550))
	f.move(geom.Pt(200, 50), 5, 16*time.Millisecond)
	f.up()
	d.Settle()
	if loads != 1 {
		t.Fatalf("finished list loaded again: %d", loads)
	}
	if built := rows; built != 30 {
		t.Fatal("rows not built")
	}
}

func TestSwipeCellOpenClose(t *testing.T) {
	var w *SwipeCellW
	action := ""
	clicks := 0
	d := phone(func(c *ui.Context) {
		ScrollView(c).Layout(layout.H(600)).Body(func() {
			w = SwipeCell(c).Right(SwipeAction{Key: "del", Text: "删除", Color: "danger"}).Body(func() {
				if Cell(c, "可左滑的行").Clickable().Clicked() {
					clicks++
				}
			})
			if k := w.ActionKey(); k != "" {
				action = k
			}
			Cell(c, "另一行")
		})
	}, true)
	d.Frame()
	d.Frame()
	f := newFinger(t, d)
	// A vertical drag on the row scrolls, it does not open the cell.
	f.down(geom.Pt(200, 25))
	f.move(geom.Pt(200, 80), 4, 16*time.Millisecond)
	f.up()
	run(d, time.Second)
	if w.IsOpen() {
		t.Fatal("vertical drag opened the cell")
	}
	// Swipe left past half the action width: opens.
	f.down(geom.Pt(300, 25))
	f.move(geom.Pt(240, 25), 6, 30*time.Millisecond)
	f.hold(100 * time.Millisecond)
	f.up()
	run(d, time.Second)
	if !w.IsOpen() || w.st.sc.Pos != w.st.sc.Max {
		t.Fatalf("open: %v %v", w.st.sc.Pos, w.st.sc.Max)
	}
	// Tap the revealed action.
	pos := geom.Pt(390-float32(w.st.sc.Max)/2, 25)
	f.down(pos)
	f.up()
	run(d, time.Second)
	if action != "del" || w.IsOpen() {
		t.Fatalf("action %q open=%v", action, w.IsOpen())
	}
	// Small swipe snaps back closed.
	f.down(geom.Pt(300, 25))
	f.move(geom.Pt(285, 25), 3, 40*time.Millisecond)
	f.hold(100 * time.Millisecond)
	f.up()
	run(d, time.Second)
	if w.IsOpen() {
		t.Fatal("small swipe left it open")
	}
	// Fast flick opens; a tap elsewhere closes without clicking the row.
	f.down(geom.Pt(300, 25))
	f.move(geom.Pt(270, 25), 2, 16*time.Millisecond)
	f.up()
	run(d, time.Second)
	if !w.IsOpen() {
		t.Fatal("flick did not open")
	}
	f.down(geom.Pt(100, 75)) // the other row
	f.up()
	run(d, time.Second)
	if w.IsOpen() {
		t.Fatal("outside tap did not close")
	}
	if clicks != 0 {
		t.Fatalf("row clicked %d times", clicks)
	}
}

func TestPickerWheelSnaps(t *testing.T) {
	var opts []ui.Option
	for i := 0; i < 60; i++ {
		opts = append(opts, ui.Option{Label: fmt.Sprint("item ", i), Value: fmt.Sprint(i)})
	}
	val := []string{"0"}
	changes := 0
	d := phone(func(c *ui.Context) {
		if PickerView(c, [][]ui.Option{opts}, &val).Changed() {
			changes++
		}
	}, true)
	d.Frame()
	d.Frame()
	th := NewTheme(nil, AntMobile)
	ih := th.PickerItemHeight
	cy := ih * float32(th.PickerVisible) / 2
	f := newFinger(t, d)
	// Slow drag up by 2.4 rows, rest, release: snaps to row 2.
	f.down(geom.Pt(200, cy))
	f.move(geom.Pt(200, cy-2.4*ih), 8, 40*time.Millisecond)
	f.hold(200 * time.Millisecond)
	f.up()
	run(d, 2*time.Second)
	if val[0] != "2" || changes != 1 {
		t.Fatalf("slow drag: %v (%d changes)", val, changes)
	}
	// A fast flick travels further and still lands exactly on a row.
	f.down(geom.Pt(200, cy+ih))
	f.move(geom.Pt(200, cy-ih), 3, 16*time.Millisecond)
	f.up()
	run(d, 3*time.Second)
	var k int
	fmt.Sscan(val[0], &k)
	if k <= 4 {
		t.Fatalf("flick only reached %v", val)
	}
	// Tap the row below the centre: it scrolls into the centre.
	f.down(geom.Pt(200, cy+ih))
	f.up()
	run(d, 2*time.Second)
	var k2 int
	fmt.Sscan(val[0], &k2)
	if k2 != k+1 {
		t.Fatalf("tap below: %d -> %d", k, k2)
	}
	// Pull past the top: rubber band, then back to the first row.
	val = []string{"1"}
	d.Frame()
	f.down(geom.Pt(200, cy))
	f.move(geom.Pt(200, cy+5*ih), 8, 30*time.Millisecond)
	f.hold(100 * time.Millisecond)
	f.up()
	run(d, 2*time.Second)
	if val[0] != "0" {
		t.Fatalf("overscroll: %v", val)
	}
}

func TestCascadeAndDatePicker(t *testing.T) {
	tree := []ui.CascaderOption{
		{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{{Label: "杭州", Value: "hz"}, {Label: "宁波", Value: "nb"}}},
		{Label: "江苏", Value: "js", Children: []ui.CascaderOption{{Label: "南京", Value: "nj"}}},
	}
	val := []string{"zj", "nb"}
	date := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	d := phone(func(c *ui.Context) {
		CascadePickerView(c, tree, &val).Show()
		DatePickerView(c, &date, PrecisionDay).Show()
	}, false)
	d.Frame()
	d.Frame()
	th := NewTheme(nil, AntMobile)
	ih := th.PickerItemHeight
	cy := ih * float32(th.PickerVisible) / 2
	// Tap the province below: 江苏; the city column follows.
	d.Click(geom.Pt(50, cy+ih))
	d.Settle()
	if val[0] != "js" || val[1] != "nj" {
		t.Fatalf("cascade: %v", val)
	}
	// Month 1 → 2 clamps the day to the 28th.
	h := ih * float32(th.PickerVisible)
	d.Click(geom.Pt(195, h+cy+ih))
	d.Settle()
	if date.Month() != 2 || date.Day() != 28 {
		t.Fatalf("date: %v", date)
	}
}

func TestPopupActionSheetDialog(t *testing.T) {
	open, dlg := true, true
	sel := -1
	ok := false
	d := phone(func(c *ui.Context) {
		Provider(c, Options{Style: Vant}, func() {
			if i, picked := ActionSheet(c, &open, []Action{{Text: "复制"}, {Text: "删除", Danger: true}}).Cancel("").Selected(); picked {
				sel = i
			}
		})
	}, false)
	d.Settle()
	d.Click(geom.Pt(195, 844-25)) // 取消 (bottom item)
	if sel != -1 || open {
		t.Fatalf("cancel: selected %d open=%v", sel, open)
	}
	open = true
	d.Settle()
	d.Click(geom.Pt(195, 844-50-8-25))
	if sel != 1 || open {
		t.Fatalf("select: %d open=%v", sel, open)
	}
	var dialogPanel ui.ID
	d2 := phone(func(c *ui.Context) {
		w := Dialog(c, &dlg, "提示").Content("确定删除？").Confirm("", "")
		if w.Confirmed() {
			ok = true
		}
		dialogPanel = w.PanelID()
	}, false)
	d2.Settle()
	// Confirm is the right half of the button row at the dialog bottom.
	dr, _ := d2.Runner().Context().RectOf(dialogPanel)
	d2.Click(geom.Pt(dr.X+dr.W*3/4, dr.Bottom()-20))
	if !ok || dlg {
		t.Fatalf("dialog confirm: %v %v", ok, dlg)
	}
	// Mask tap closes a popup.
	pop := true
	d3 := phone(func(c *ui.Context) {
		Popup(c, &pop).Body(func() { ui.Div(c, layout.H(200)) })
	}, false)
	d3.Settle()
	d3.Click(geom.Pt(195, 100))
	if pop {
		t.Fatal("mask tap did not close the popup")
	}
}

func TestStepperTabsTabBar(t *testing.T) {
	v := 1.0
	active := 0
	tab := "home"
	var bar ui.ID
	d := phone(func(c *ui.Context) {
		Stepper(c, &v).Min(0).Max(3)
		Tabs(c, []string{"一", "二", "三"}, &active).Swipeable().Body(func(i int) {
			ui.Div(c, layout.H(300)).Body(func() { ui.Text(c, fmt.Sprint("pane ", i)) })
		})
		bar = TabBar(c, []TabItem{{Key: "home", Title: "首页"}, {Key: "me", Title: "我的"}}, &tab).ID()
	}, true)
	d.Settle()
	th := NewTheme(nil, AntMobile)
	plus := geom.Pt(th.StepperHeight*1.5+th.StepperInputW, 22)
	d.Click(plus)
	d.Click(plus)
	d.Click(plus)
	if v != 3 {
		t.Fatalf("stepper max: %v", v)
	}
	d.Click(geom.Pt(10, 22))
	if v != 2 {
		t.Fatalf("stepper minus: %v", v)
	}
	// Swipe the content left: next tab.
	f := newFinger(t, d)
	f.down(geom.Pt(300, 44+th.TabsHeight+100))
	f.move(geom.Pt(150, 44+th.TabsHeight+100), 4, 16*time.Millisecond)
	f.up()
	run(d, time.Second)
	if active != 1 {
		t.Fatalf("tabs swipe: %d", active)
	}
	// Tap the third title.
	d.Click(geom.Pt(390*5/6, 44+th.TabsHeight/2))
	d.Settle()
	if active != 2 {
		t.Fatalf("tabs tap: %d", active)
	}
	br, _ := d.Runner().Context().RectOf(bar)
	d.Click(geom.Pt(300, br.Y+20))
	if tab != "me" {
		t.Fatalf("tabbar: %q", tab)
	}
}
