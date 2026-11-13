package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
)

func driver(page func(*Context)) *Driver {
	return NewDriver(page, DriverOptions{Width: 600, Height: 400})
}

func mustFind(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	r, ok := d.Find(label)
	if !ok {
		t.Fatalf("widget %q not found", label)
	}
	return r
}

func TestClickTriggersCallbackAndRedraw(t *testing.T) {
	clicks := 0
	d := driver(func(c *Context) {
		Column(c, func() {
			if Button(c, "保存").Primary().Clicked() {
				clicks++
			}
			Text(c, map[bool]string{true: "已保存", false: "未保存"}[clicks > 0])
		}, layout.Pad(20))
	})
	d.Frame()
	before := d.Presented()
	d.Click(mustFind(t, d, "保存").Center())
	if clicks != 1 {
		t.Fatalf("clicks = %d", clicks)
	}
	if d.Presented() <= before {
		t.Fatal("no redraw after click")
	}
	if _, ok := d.Find("保存"); !ok {
		t.Fatal("button vanished")
	}
	// Disabled buttons never report clicks.
	n := 0
	d2 := driver(func(c *Context) {
		if Button(c, "禁用").Disabled(true).Clicked() {
			n++
		}
	})
	d2.Frame()
	d2.Click(mustFind(t, d2, "禁用").Center())
	if n != 0 {
		t.Fatal("disabled button clicked")
	}
}

func TestHoverCursorAndIdle(t *testing.T) {
	var hovered bool
	d := driver(func(c *Context) {
		hovered = Button(c, "悬停").Hovered()
	})
	d.Frame()
	d.Move(mustFind(t, d, "悬停").Center())
	d.Frame()
	if !hovered {
		t.Fatal("hover not reported")
	}
	if d.Cursor() != input.CursorPointer {
		t.Fatalf("cursor = %v", d.Cursor())
	}
	d.Settle()
	if d.NeedsFrame() {
		t.Fatal("idle UI keeps requesting frames")
	}
}

func TestAnimationRequestsFramesUntilDone(t *testing.T) {
	d := NewDriver(func(c *Context) { Button(c, "动画") }, DriverOptions{Width: 300, Height: 100, Animations: true})
	d.Frame()
	d.Move(mustFind(t, d, "动画").Center())
	if !d.Runner().Animating() {
		t.Fatal("hover transition should animate")
	}
	d.Settle()
	if d.Runner().Animating() {
		t.Fatal("animation did not settle")
	}
}

func TestCheckboxSwitchRadio(t *testing.T) {
	var ck, sw bool
	radio := "a"
	d := driver(func(c *Context) {
		Column(c, func() {
			Checkbox(c, "同意", &ck)
			Switch(c, &sw)
			RadioGroup(c, &radio, Opts("a", "b"))
		})
	})
	d.Frame()
	d.Click(mustFind(t, d, "同意").Center())
	if !ck {
		t.Fatal("checkbox not toggled")
	}
	d.Click(mustFind(t, d, "同意").Center())
	if ck {
		t.Fatal("checkbox not toggled back")
	}
	sr := d.r.c.hits
	_ = sr
	d.Click(mustFind(t, d, "b").Center())
	if radio != "b" {
		t.Fatalf("radio = %q", radio)
	}
}

func TestInputEditingAndCaret(t *testing.T) {
	val := ""
	changed := 0
	var id ID
	d := driver(func(c *Context) {
		in := Input(c, &val).Placeholder("名字")
		id = in.ID()
		if in.Changed() {
			changed++
		}
	})
	d.Frame()
	r := d.r.c.rects[id]
	d.Click(r.Center())
	if d.r.c.focus != id {
		t.Fatal("input not focused on click")
	}
	if on, _ := d.TextInputActive(); !on {
		t.Fatal("IME not enabled for focused input")
	}
	d.Type("hello")
	if val != "hello" || changed == 0 {
		t.Fatalf("val=%q changed=%d", val, changed)
	}
	d.Key(input.KeyLeft, 0)
	d.Key(input.KeyLeft, 0)
	d.Type("X")
	if val != "helXlo" {
		t.Fatalf("insert at caret: %q", val)
	}
	d.Key(input.KeyBackspace, 0)
	if val != "hello" {
		t.Fatalf("backspace: %q", val)
	}
	d.Key(input.KeyHome, 0)
	d.Key(input.KeyDelete, 0)
	if val != "ello" {
		t.Fatalf("delete: %q", val)
	}
	short := input.ModCtrl
	if input.Modifiers(input.ModMeta).Shortcut() {
		short = input.ModMeta
	}
	d.Key(input.KeyA, short)
	d.Key(input.KeyX, short)
	if val != "" || d.Clipboard() != "ello" {
		t.Fatalf("cut: val=%q clip=%q", val, d.Clipboard())
	}
	d.Key(input.KeyV, short)
	d.Key(input.KeyV, short)
	if val != "elloello" {
		t.Fatalf("paste: %q", val)
	}
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyLeft, input.ModShift)
	d.Key(input.KeyLeft, input.ModShift)
	d.Type("O")
	if val != "elloelO" {
		t.Fatalf("replace selection: %q", val)
	}
}

func TestIMECompositionCommit(t *testing.T) {
	val := "ab"
	var id ID
	d := driver(func(c *Context) { id = Input(c, &val).ID() })
	d.Frame()
	d.Click(d.r.c.rects[id].Center())
	d.Key(input.KeyEnd, 0)
	d.Compose("ni", 2)
	if val != "ab" {
		t.Fatalf("composition must not edit the value: %q", val)
	}
	if s, cur := d.r.c.CompositionFor(id); s != "ni" || cur != 2 {
		t.Fatalf("composition = %q %d", s, cur)
	}
	_, caret := d.TextInputActive()
	if caret.H == 0 {
		t.Fatal("no caret rect for IME window")
	}
	d.Type("你")
	if val != "ab你" {
		t.Fatalf("commit: %q", val)
	}
	if s, _ := d.r.c.CompositionFor(id); s != "" {
		t.Fatal("composition not cleared on commit")
	}
}

func TestTabFocusOrder(t *testing.T) {
	a, b := "", ""
	var ia, ib ID
	d := driver(func(c *Context) {
		Column(c, func() {
			ia = Input(c, &a).ID()
			Button(c, "中间")
			ib = Input(c, &b).ID()
		})
	})
	d.Frame()
	d.Key(input.KeyTab, 0)
	if d.r.c.focus != ia {
		t.Fatal("first tab should focus first input")
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	if d.r.c.focus != ib {
		t.Fatal("third tab should focus second input")
	}
	d.Key(input.KeyTab, input.ModShift)
	if d.r.c.focus == ib || d.r.c.focus == 0 {
		t.Fatal("shift+tab")
	}
	if !d.r.c.focusVisible {
		t.Fatal("keyboard focus should be visible")
	}
}

func TestEnterActivatesFocusedButton(t *testing.T) {
	n := 0
	d := driver(func(c *Context) {
		if Button(c, "确定").Clicked() {
			n++
		}
	})
	d.Frame()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	if n != 1 {
		t.Fatalf("enter activation = %d", n)
	}
}

func TestSelectOpenPickAndKeyboard(t *testing.T) {
	val := ""
	var sel *SelectW
	d := driver(func(c *Context) {
		sel = Select(c, &val, Opts("北京", "上海", "杭州")).Placeholder("城市").Width(160)
	})
	d.Frame()
	d.Click(d.r.c.rects[sel.n.id].Center())
	if !sel.IsOpen() {
		t.Fatal("select did not open")
	}
	d.Click(mustFind(t, d, "上海").Center())
	if val != "上海" || sel.IsOpen() {
		t.Fatalf("pick: val=%q open=%v", val, sel.IsOpen())
	}
	// Keyboard: open with Down, move, Enter.
	d.Move(geom.Pt(590, 390))
	d.Key(input.KeyDown, 0)
	if !sel.IsOpen() {
		t.Fatal("down should open")
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if val != "杭州" {
		t.Fatalf("keyboard pick: %q", val)
	}
	// Outside click closes.
	d.Key(input.KeyDown, 0)
	d.Click(geom.Pt(590, 390))
	if sel.IsOpen() {
		t.Fatal("outside click should close")
	}
}

func TestModalEscapeAndMask(t *testing.T) {
	open := true
	behind := 0
	d := driver(func(c *Context) {
		if Button(c, "后面").Clicked() {
			behind++
		}
		Modal(c, &open, "标题").Body(func() { Text(c, "内容") })
	})
	d.Settle()
	// The mask swallows clicks for widgets underneath.
	d.Click(geom.Pt(20, 15))
	if behind != 0 {
		t.Fatal("click went through the modal mask")
	}
	if open {
		t.Fatal("mask click should close (maskClosable)")
	}
	open = true
	d.Settle()
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("escape should close")
	}
}

func TestScrollContainer(t *testing.T) {
	var sb *ScrollBox
	d := driver(func(c *Context) {
		sb = Scroll(c, func() {
			for i := 0; i < 50; i++ {
				Text(c, "行")
			}
		}, layout.H(200))
	})
	d.Frame()
	d.Scroll(geom.Pt(50, 100), 120)
	if _, y := sb.Offset(); y != 120 {
		t.Fatalf("offset = %v", y)
	}
	d.Scroll(geom.Pt(50, 100), 100000)
	if _, got := sb.Offset(); got != 50*22-200 {
		t.Fatalf("clamped offset = %v", got)
	}
}

func TestFormValidation(t *testing.T) {
	name := ""
	var f *FormW
	ok := true
	d := driver(func(c *Context) {
		f = Form(c)
		f.Body(func() {
			f.Item("用户名").Rules(&name, Required("必填"), MinLen(3, "太短")).Body(func() { Input(c, &name) })
			if Button(c, "提交").Clicked() {
				ok = f.Validate()
			}
		})
	})
	d.Frame()
	d.ClickLabel("提交")
	if ok || f.Errors()["用户名"] != "必填" {
		t.Fatalf("expected required error, got %v", f.Errors())
	}
	if _, found := d.Find("必填"); found {
		// error text is not interactive; check via layout instead
	}
	name = "ab"
	d.Settle()
	if f.Errors()["用户名"] != "太短" {
		t.Fatalf("live validation: %v", f.Errors())
	}
	name = "abc"
	d.Settle()
	d.ClickLabel("提交")
	if !ok {
		t.Fatal("valid form rejected")
	}
}

func TestKeyScopeKeepsState(t *testing.T) {
	items := []string{"a", "b"}
	vals := map[string]*string{"a": new(string), "b": new(string)}
	var ids map[string]ID
	d := driver(func(c *Context) {
		ids = map[string]ID{}
		for _, it := range items {
			Key(c, it, func() { ids[it] = Input(c, vals[it]).ID() })
		}
	})
	d.Frame()
	before := ids["b"]
	items = []string{"new", "a", "b"}
	vals["new"] = new(string)
	d.Frame()
	if ids["b"] != before {
		t.Fatal("keyed ID changed when a sibling was inserted")
	}
}

func TestTextWidgetWrapsToWidth(t *testing.T) {
	text.UseSystemFonts(false)
	defer text.UseSystemFonts(true)
	var lbl *Label
	d := driver(func(c *Context) {
		Column(c, func() { lbl = Text(c, "一二三四五六七八九十一二三四五六七八九十") }, layout.W(100))
	})
	d.Frame()
	r := d.r.c.rects[lbl.n.id]
	if r.W != 100 || r.H < 44 {
		t.Fatalf("wrapped text rect %+v", r)
	}
}

func TestMessageExpires(t *testing.T) {
	shown := false
	d := NewDriver(func(c *Context) {
		if !shown {
			shown = true
			Message(c, "success", "完成")
		}
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if len(d.r.c.toasts().msgs) != 1 {
		t.Fatal("message not queued")
	}
	if d.Runner().NextWake().IsZero() {
		t.Fatal("message should request a wake-up")
	}
	d.Advance(MessageDuration + time.Second)
	d.Settle()
	if len(d.r.c.toasts().msgs) != 0 {
		t.Fatal("message did not expire")
	}
}
