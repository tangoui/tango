package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestDataFormatDuration(t *testing.T) {
	d := 2*24*time.Hour + 4*time.Hour + 30*time.Minute + 12*time.Second + 34*time.Millisecond
	cases := map[string]string{
		"HH:mm:ss":           "52:30:12",
		"D 天 HH 时 mm 分 ss 秒": "2 天 04 时 30 分 12 秒",
		"HH:mm:ss:SSS":       "52:30:12:034",
		"mm:ss":              "3150:12",
		"[剩余] D[天] HH:mm":    "剩余 2天 04:30",
	}
	for f, want := range cases {
		if got := FormatDuration(d, f); got != want {
			t.Errorf("FormatDuration(%q) = %q, want %q", f, got, want)
		}
	}
	if got := FormatDuration(-time.Second, "HH:mm:ss"); got != "00:00:00" {
		t.Errorf("negative: %q", got)
	}
}

func TestDataCountdownFinishesOnce(t *testing.T) {
	var start time.Time
	finished := 0
	d := NewDriver(func(c *Context) {
		if start.IsZero() {
			start = c.Now()
		}
		if Countdown(c, "cd", start.Add(2*time.Second)).Finished() {
			finished++
		}
	}, DriverOptions{})
	d.Frame()
	if w := d.Runner().NextWake(); w.IsZero() || w.Sub(d.Now()) > time.Second {
		t.Fatalf("countdown should wake within a second, got %v", w)
	}
	d.Advance(3 * time.Second)
	d.Frame()
	d.Frame()
	if finished != 1 {
		t.Fatalf("finished reported %d times, want 1", finished)
	}
}

func TestDataCarouselArrowsAndAutoplay(t *testing.T) {
	cur := 0
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Carousel(c, 3, func(i int) { Text(c, "slide") }).Current(&cur).Arrows("always").Autoplay(time.Second).Show()
		}, layout.W(400))
	}, DriverOptions{})
	d.Settle()
	d.Settle()
	r, ok := d.Find("下一张")
	if !ok {
		t.Fatal("next arrow missing")
	}
	d.Click(r.Center())
	if cur != 1 {
		t.Fatalf("after next: cur=%d", cur)
	}
	r, _ = d.Find("上一张")
	d.Click(r.Center())
	d.Click(r.Center())
	if cur != 2 {
		t.Fatalf("prev should wrap to 2, got %d", cur)
	}
	// Autoplay: move the pointer away and let time pass.
	d.Move(geom.Pt(900, 700))
	d.Advance(1100 * time.Millisecond)
	d.Frame()
	if cur != 0 {
		t.Fatalf("autoplay should wrap to 0, got %d", cur)
	}
}

func TestDataPopoverClickOpenClose(t *testing.T) {
	var open bool
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			p := Popover(c).Title("标题").Text("内容").Trigger("click").Placement("right")
			p.Body(func() { Button(c, "trigger") })
			open = p.IsOpen()
		}, layout.Pad(100))
	}, DriverOptions{})
	d.Settle()
	d.ClickLabel("trigger")
	d.Settle()
	if !open {
		t.Fatal("popover should open on click")
	}
	d.Click(geom.Pt(1000, 750))
	d.Settle()
	if open {
		t.Fatal("popover should close on outside click")
	}
}

func TestDataTourSteps(t *testing.T) {
	open, cur := true, 0
	var ids [2]ID
	finished, closed := false, false
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			ids[0] = Button(c, "A").ID()
			ids[1] = Button(c, "B").ID()
		}, layout.Pad(40), layout.Gap(8))
		tw := Tour(c, &open, &cur, []TourStep{{Target: ids[0], Title: "一"}, {Target: ids[1], Title: "二"}}).Show()
		finished = finished || tw.Finished()
		closed = closed || tw.Closed()
	}, DriverOptions{})
	d.Settle()
	d.Settle()
	// The mask blocks the page: clicking the button does nothing.
	if err := d.ClickLabel("下一步"); err != nil {
		t.Fatal(err)
	}
	if cur != 1 {
		t.Fatalf("cur=%d after next", cur)
	}
	if err := d.ClickLabel("上一步"); err != nil {
		t.Fatal(err)
	}
	if cur != 0 {
		t.Fatalf("cur=%d after prev", cur)
	}
	d.ClickLabel("下一步")
	d.ClickLabel("结束导览")
	if open || !finished {
		t.Fatalf("tour should finish: open=%v finished=%v", open, finished)
	}
	open, cur = true, 0
	d.Settle()
	d.Key(input.KeyEscape, 0)
	if open || !closed {
		t.Fatalf("escape should close: open=%v closed=%v", open, closed)
	}
}

func TestDataCalendarPickAndYear(t *testing.T) {
	val := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	mode := "month"
	d := NewDriver(func(c *Context) {
		Calendar(c, &val).Mode(&mode).DisabledDate(func(d time.Time) bool { return d.Day() == 20 })
	}, DriverOptions{Width: 1000, Height: 900})
	d.Settle()
	d.Settle()
	if err := d.ClickLabel("2026-01-21"); err != nil {
		t.Fatal(err)
	}
	if val.Day() != 21 {
		t.Fatalf("picked %v", val)
	}
	d.ClickLabel("2026-01-20")
	if val.Day() != 21 {
		t.Fatalf("disabled date was picked: %v", val)
	}
	// Clicking a day of the next month moves the view.
	d.ClickLabel("2026-02-01")
	d.Settle()
	if _, ok := d.Find("2026-02-28"); !ok {
		t.Fatal("view should move to February")
	}
	mode = "year"
	d.Settle()
	if err := d.ClickLabel("2026-07"); err != nil {
		t.Fatal(err)
	}
	if mode != "month" {
		t.Fatalf("clicking a month should return to month view, mode=%q", mode)
	}
	if _, ok := d.Find("2026-07-15"); !ok {
		t.Fatal("July should be shown")
	}
}

func TestDataTypographyCopyEditExpand(t *testing.T) {
	s := "hello"
	open := false
	long := "一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十"
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Row(c, func() { Text(c, "copy me").Copyable() })
			Row(c, func() { Text(c, s).Editable(&s) })
			Paragraph(c, long).Lines(1).Expandable(&open)
		}, layout.W(300), layout.Pad(20))
	}, DriverOptions{})
	d.Settle()
	r, _ := d.Find("copy me")
	d.Click(geom.Pt(r.Right()-7, r.Y+r.H/2))
	if d.Clipboard() != "copy me" {
		t.Fatalf("clipboard = %q", d.Clipboard())
	}
	r, _ = d.Find("hello")
	d.Click(geom.Pt(r.Right()-7, r.Y+r.H/2))
	d.Settle()
	if ok, _ := d.TextInputActive(); !ok {
		t.Fatal("editing should focus an input")
	}
	d.Type(" world")
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if s != "hello world" {
		t.Fatalf("edited value %q", s)
	}
	// Expand: the link sits at the end of the single visible line.
	pr, _ := d.Find(long)
	if pr.H > 30 {
		t.Fatalf("collapsed paragraph too tall: %v", pr)
	}
	d.Click(geom.Pt(pr.Right()-10, pr.Y+pr.H/2))
	d.Settle()
	if !open {
		t.Fatal("expand link should open the paragraph")
	}
	pr, _ = d.Find(long)
	if pr.H < 40 {
		t.Fatalf("expanded paragraph should wrap: %v", pr)
	}
}

func TestDataBadgeContent(t *testing.T) {
	b := &BadgeW{overflow: 99}
	if _, ok := b.content(); ok {
		t.Error("zero count should hide")
	}
	b.showZero = true
	if s, ok := b.content(); !ok || s != "0" {
		t.Errorf("showZero: %q %v", s, ok)
	}
	b.count = 120
	if s, _ := b.content(); s != "99+" {
		t.Errorf("overflow: %q", s)
	}
}

func TestDataAvatarGroupMax(t *testing.T) {
	var kids int
	d := NewDriver(func(c *Context) {
		g := AvatarGroup(c).Max(2).Body(func() {
			for i := 0; i < 5; i++ {
				Avatar(c, "A")
			}
		})
		kids = len(g.n.kids)
	}, DriverOptions{})
	d.Frame()
	if kids != 3 {
		t.Fatalf("group should show 2 avatars + overflow, got %d", kids)
	}
	if _, ok := d.Find("+3"); ok {
		return
	}
}

func TestDataDescriptionsRows(t *testing.T) {
	items := []DescItem{{Label: "a"}, {Label: "b", Span: 2}, {Label: "c", Span: 2}, {Label: "d"}}
	rows := (&DescriptionsW{items: items, cols: 3}).rows()
	if len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Fatalf("rows = %v", rows)
	}
	if rows[1][1].Span != 1 {
		t.Fatalf("last item should fill the row: span=%d", rows[1][1].Span)
	}
}

func TestDataSkeletonLoadingSwitch(t *testing.T) {
	loading := true
	d := NewDriver(func(c *Context) {
		Skeleton(c).Loading(loading).Body(func() { Button(c, "real") })
	}, DriverOptions{})
	d.Frame()
	if _, ok := d.Find("real"); ok {
		t.Fatal("content should be hidden while loading")
	}
	loading = false
	d.Frame()
	if _, ok := d.Find("real"); !ok {
		t.Fatal("content should show when loaded")
	}
}

func TestDataProgress(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Progress(c, 150).Steps(5)
			Progress(c, 40).Dashboard(120)
			Progress(c, 60).Striped(true).Indeterminate()
		}, layout.W(400))
	}, DriverOptions{})
	d.Frame()
	p := &ProgressW{c: d.r.c, percent: clampPct(150), showInfo: true}
	if p.effStatus() != "success" || p.label() != "100%" {
		t.Fatalf("status=%q label=%q", p.effStatus(), p.label())
	}
}

func TestDataResultAndEmpty(t *testing.T) {
	d := NewDriver(func(c *Context) {
		for _, s := range []string{"success", "error", "info", "warning", "403", "404", "500"} {
			Result(c, s, s).SubTitle("sub").Extra(func() { Button(c, "go-"+s) }).Show()
		}
		Empty(c).Body(func() { Button(c, "create") })
	}, DriverOptions{Height: 3000})
	d.Frame()
	for _, l := range []string{"go-success", "go-404", "create"} {
		if _, ok := d.Find(l); !ok {
			t.Errorf("%s missing", l)
		}
	}
}
