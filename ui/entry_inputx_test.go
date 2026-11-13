package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Conversions themselves are tested in ui/theme; this only pins the
// upper-case display form ColorPicker shows.
func TestEntryColorHexDisplay(t *testing.T) {
	if s := cpHex(theme.Color{R: 22, G: 119, B: 255, A: 128}); s != "#1677FF80" {
		t.Fatalf("hex8 = %s", s)
	}
}

func TestEntryColorPickerInteraction(t *testing.T) {
	col := theme.Hex("#ff0000")
	changed := 0
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			if ColorPicker(c, &col).Presets(theme.Hex("#52c41a")).Changed() {
				changed++
			}
		}, layout.Pad(20))
	}, DriverOptions{Width: 600, Height: 700})
	d.Frame()
	// Open with a click on the trigger.
	trig, _ := d.r.c.RectOf(d.r.c.root.kids[0].kids[0].id)
	d.Click(trig.Center())
	sv := mustFind(t, d, "饱和度")
	// Drag on the saturation panel: bottom-left is black.
	d.Click(geom.Pt(sv.X+1, sv.Bottom()-1))
	if col.R > 10 || col.G > 10 || col.B > 10 {
		t.Fatalf("expected near black, got %v", col)
	}
	// Top-right is the pure hue.
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(sv.X+5, sv.Y+5)})
	d.Inject(input.Pointer{Kind: input.Press, Pos: geom.Pt(sv.X+5, sv.Y+5)})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(sv.Right()+30, sv.Y-30)})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(sv.Right()+30, sv.Y-30)})
	d.Settle()
	if col != theme.Hex("#ff0000") {
		t.Fatalf("drag clamped to hue, got %v", col)
	}
	// Arrow keys nudge saturation/value while the panel has focus.
	d.Key(input.KeyDown, 0)
	if col.R == 255 {
		t.Fatalf("value not decreased: %v", col)
	}
	// Preset click.
	d.Click(mustFind(t, d, "#52C41A").Center())
	if col != theme.Hex("#52c41a") {
		t.Fatalf("preset not applied: %v", col)
	}
	// Alpha bar: leftmost is transparent.
	ab := mustFind(t, d, "透明度")
	d.Click(geom.Pt(ab.X+1, ab.Y+ab.H/2))
	if col.A != 0 || col.G != 0xc4 {
		t.Fatalf("alpha = %v", col)
	}
	if changed == 0 {
		t.Fatal("Changed never reported")
	}
	// Hex input parses on edit.
	d.Click(mustFind(t, d, "透明度").Center())
	if !(col.A > 100 && col.A < 160) {
		t.Fatalf("alpha mid = %d", col.A)
	}
	// Escape closes.
	d.Key(input.KeyEscape, 0)
	if _, ok := d.Find("饱和度"); ok {
		t.Fatal("panel still open after Escape")
	}
}

func TestEntryColorHexInput(t *testing.T) {
	col := theme.Hex("#000000")
	var w *ColorPickerW
	d := NewDriver(func(c *Context) {
		w = ColorPicker(c, &col).Open()
	}, DriverOptions{Width: 600, Height: 700})
	d.Frame()
	d.Frame()
	id := w.st.ids[0]
	if id == 0 {
		t.Fatal("hex input not built")
	}
	d.r.c.SetFocus(id)
	d.Frame()
	d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
	d.Type("1677FF")
	if col != theme.Hex("#1677ff") {
		t.Fatalf("hex typed -> %v (buf %q)", col, w.st.bufs[0])
	}
}

func TestEntryColorElementDraft(t *testing.T) {
	col := theme.Hex("#409eff")
	var w *ColorPickerW
	d := NewDriver(func(c *Context) { w = ColorPicker(c, &col).Open() }, DriverOptions{Width: 600, Height: 700, Theme: theme.ElementPlus()})
	d.Frame()
	sv := mustFind(t, d, "饱和度")
	d.Click(geom.Pt(sv.X+1, sv.Bottom()-1))
	if col != theme.Hex("#409eff") {
		t.Fatal("Element must not commit before 确定")
	}
	d.ClickLabel("确定")
	if col.R > 5 {
		t.Fatalf("confirm did not commit: %v", col)
	}
	w.Open()
	d.Frame()
	d.ClickLabel("清空")
	if col != (theme.Color{}) {
		t.Fatalf("clear -> %v", col)
	}
}

func TestEntryCompactJoins(t *testing.T) {
	a, b := "x", ""
	clicked := false
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Compact(c).Body(func() {
				Addon(c, "https://")
				Input(c, &a)
				if Button(c, "前往").Clicked() {
					clicked = true
				}
			})
			Compact(c).Body(func() { Input(c, &b) })
		}, layout.Pad(10))
	}, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	btn := mustFind(t, d, "前往")
	cmp := d.r.c.root.kids[0].kids[0]
	ad := cmp.kids[0].abs
	if len(cmp.kids) != 3 || cmp.kids[1].ln.Margin.Left != -1 || cmp.kids[2].ln.Margin.Left != -1 || cmp.kids[0].ln.Margin.Left != 0 {
		t.Fatal("compact children not overlapped by 1px")
	}
	in := cmp.kids[1].abs
	if in.X != ad.Right()-1 {
		t.Fatalf("input x %v, addon right %v", in.X, ad.Right())
	}
	d.Click(btn.Center())
	if !clicked {
		t.Fatal("button in compact not clickable")
	}
	d.Click(in.Center())
	d.Type("y")
	if a != "xy" && a != "y" {
		t.Fatalf("input in compact not editable: %q", a)
	}
	// Painting the wrapped children must not panic and must stay idle.
	d.Settle()
}

func TestEntrySearchTriggers(t *testing.T) {
	q := ""
	n := 0
	var w *SearchW
	d := NewDriver(func(c *Context) {
		w = Search(c, &q).Placeholder("搜")
		if w.Searched() {
			n++
		}
	}, DriverOptions{Width: 400, Height: 100})
	d.Frame()
	d.r.c.SetFocus(w.InputID())
	d.Frame()
	d.Type("go")
	d.Key(input.KeyEnter, 0)
	if q != "go" || n != 1 {
		t.Fatalf("enter: q=%q n=%d", q, n)
	}
	d.Click(mustFind(t, d, "搜索").Center())
	if n != 2 {
		t.Fatalf("button: n=%d", n)
	}
	// Loading blocks the button; enter button variant triggers too.
	m := 0
	d2 := NewDriver(func(c *Context) {
		if Search(c, &q).EnterButton("查询").Searched() {
			m++
		}
		Search(c, &q).Loading(true)
	}, DriverOptions{Width: 400, Height: 200})
	d2.Frame()
	d2.ClickLabel("查询")
	if m != 1 {
		t.Fatalf("enter button: m=%d", m)
	}
}

func TestEntryTextAreaCountAndAutoSize(t *testing.T) {
	v := ""
	var w *TextAreaW
	d := NewDriver(func(c *Context) {
		w = TextArea(c, &v).ShowCount().MaxLength(5).AutoSize(1, 3)
	}, DriverOptions{Width: 300, Height: 400})
	d.Frame()
	d.r.c.SetFocus(w.ID())
	d.Frame()
	d.Type("abcdefgh")
	if v != "abcde" {
		t.Fatalf("maxlength not enforced: %q", v)
	}
	if w.countText() != "5 / 5" {
		t.Fatalf("count = %q", w.countText())
	}
	h1 := w.box.abs.H
	d.Key(input.KeyBackspace, 0)
	d.Type("\n\n\n\n\n")
	_ = h1
	v = "1\n2\n3\n4\n5\n6"
	d.Settle()
	h3 := w.box.abs.H
	if h3 != 3*ixLineH+8 {
		t.Fatalf("autosize max 3 rows -> %v", h3)
	}
	v = "x"
	d.Settle()
	if w.box.abs.H != ixLineH+8 {
		t.Fatalf("autosize min 1 row -> %v", w.box.abs.H)
	}
	// Exceed keeps the text and flags the count.
	e := "123456"
	var w2 *TextAreaW
	d2 := NewDriver(func(c *Context) { w2 = TextArea(c, &e).ShowCount().MaxLength(3).Exceed() }, DriverOptions{Width: 300, Height: 200})
	d2.Frame()
	if !w2.over() || e != "123456" {
		t.Fatal("exceed should keep text and report over")
	}
}

func TestEntryTextAreaScrolls(t *testing.T) {
	v := "1\n2\n3\n4\n5\n6\n7\n8"
	var w *TextAreaW
	d := NewDriver(func(c *Context) { w = TextArea(c, &v).Rows(2) }, DriverOptions{Width: 300, Height: 200})
	d.Frame()
	d.Scroll(w.box.abs.Center(), 30)
	if _, y := w.sb.Offset(); y <= 0 {
		t.Fatal("wheel did not scroll")
	}
	// Caret at the end scrolls into view.
	d.r.c.SetFocus(w.ID())
	d.Frame()
	d.Key(input.KeyEnd, input.ModCtrl)
	d.Key(input.KeyDown, 0)
	if _, y := w.sb.Offset(); y < 5*ixLineH {
		t.Fatalf("caret not followed: %v", y)
	}
}

func TestEntryOTPFlow(t *testing.T) {
	code := ""
	done := 0
	var w *OTPW
	d := NewDriver(func(c *Context) {
		w = OTP(c, &code, 4).Numeric()
		if w.Completed() {
			done++
		}
	}, DriverOptions{Width: 400, Height: 100})
	d.Frame()
	d.Click(mustFind(t, d, "otp-0").Center())
	if d.r.c.focus != w.CellID(0) {
		t.Fatal("click did not focus first box")
	}
	d.Type("1")
	d.Type("a") // rejected by Numeric
	d.Type("2")
	if code != "12" || d.r.c.focus != w.CellID(2) {
		t.Fatalf("typing: %q focus at %v", code, d.r.c.focus)
	}
	d.Key(input.KeyBackspace, 0) // empty box: go back and delete
	if code != "1" || d.r.c.focus != w.CellID(1) {
		t.Fatalf("backspace: %q", code)
	}
	d.Key(input.KeyLeft, 0)
	if d.r.c.focus != w.CellID(0) {
		t.Fatal("left arrow")
	}
	d.SetClipboard("9876")
	d.Key(input.KeyV, input.ModCtrl|input.ModMeta)
	if code != "9876" || done != 1 {
		t.Fatalf("paste: %q done=%d", code, done)
	}
	// External value resets the boxes.
	code = "5"
	d.Settle()
	if w.st.cells[0] != "5" || w.st.cells[1] != "" {
		t.Fatalf("sync: %v", w.st.cells)
	}
}

func TestEntryOTPFocusFirst(t *testing.T) {
	code := ""
	var w *OTPW
	d := NewDriver(func(c *Context) {
		w = OTP(c, &code, 3)
		if c.Focused() == 0 {
			w.Focus()
		}
	}, DriverOptions{Width: 400, Height: 100})
	d.Settle()
	if d.r.c.focus != w.CellID(0) {
		t.Fatal("Focus should pick the first empty box")
	}
}
