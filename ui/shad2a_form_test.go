package ui

import (
	"context"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestShad2aFormValidateFieldsClear(t *testing.T) {
	a, b := "", ""
	var f *FormW
	var act func()
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		f = Form(c)
		f.Body(func() {
			f.Item("甲").Rules(&a, Required("甲必填")).Body(func() { Input(c, &a) })
			f.Item("乙").Name("b").Rules(&b, Required("乙必填")).Body(func() { Input(c, &b) })
		})
		if act != nil {
			act()
			act = nil
		}
	})
	ok := true
	act = func() { ok = f.ValidateFields("b") }
	d.Settle()
	if ok || f.Errors()["b"] != "乙必填" || f.Errors()["甲"] != "" {
		t.Fatalf("ValidateFields(b): ok %v errs %v", ok, f.Errors())
	}
	act = func() { f.Validate() }
	d.Settle()
	if f.Errors()["甲"] == "" {
		t.Fatal("Validate did not check 甲")
	}
	act = func() { f.ClearValidate("b") }
	d.Settle()
	if f.Errors()["b"] != "" || f.Errors()["甲"] == "" {
		t.Fatalf("ClearValidate(b): %v", f.Errors())
	}
	if _, ok := d.FindText("乙必填"); ok {
		t.Fatal("cleared message still shown")
	}
	act = func() { f.ClearValidate() }
	d.Settle()
	if len(f.Errors()) != 0 && (f.Errors()["甲"] != "" || f.Errors()["b"] != "") {
		t.Fatalf("ClearValidate(): %v", f.Errors())
	}
}

func TestShad2aFormDisabledAndOnReset(t *testing.T) {
	v := "x"
	n := 5
	dis := true
	var f *FormW
	var in *InputW
	reset := false
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		if reset {
			reset = false
			f.ResetFields()
		}
		f = Form(c).Disabled(dis).RequiredMark(false)
		f.Body(func() {
			f.Item("名称").Rules(&v, Required("必填")).Body(func() { in = Input(c, &v) })
			f.Item("数字").RulesFunc(func() string { return itoaEnt(n) }, Required("必填")).
				OnReset(func() { n = 5 }).Body(func() { Text(c, itoaEnt(n)) })
		})
	})
	if !in.box.disabled {
		t.Fatal("Form.Disabled did not reach the input")
	}
	if f.items[0].star() {
		t.Fatal("RequiredMark(false) still draws the asterisk")
	}
	dis = false
	d.Settle()
	if in.box.disabled {
		t.Fatal("input still disabled")
	}
	n, v = 9, "y"
	d.Settle()
	reset = true
	d.Settle()
	if n != 5 || v != "x" {
		t.Fatalf("ResetFields: n %v v %q", n, v)
	}
	// Reset only one field.
	n, v = 7, "z"
	d.Settle()
	f.ResetFields("数字")
	d.Settle()
	if n != 5 || v != "z" {
		t.Fatalf("ResetFields(数字): n %v v %q", n, v)
	}
}

func TestShad2aFormDefaults(t *testing.T) {
	v := ""
	var f *FormW
	var in *InputW
	p2dDriver(theme.AntDesign(), func(c *Context) {
		ConfigProvider(c).Defaults("Form", Props{"labelAlign": "left", "disabled": true, "requiredMark": false, "size": "small"}).Body(func() {
			f = Form(c)
			f.Body(func() {
				f.Item("字段").Rules(&v, Required("必填")).Body(func() { in = Input(c, &v) })
			})
		})
	})
	if f.labelAlign != AlignLeft || !f.disabled || !f.noStar || !f.hasSize || f.size != Small {
		t.Fatalf("defaults: %+v", f)
	}
	if !in.box.disabled {
		t.Fatal("disabled default not applied")
	}
}

// shadcn: the item keeps its gap (space-y-6) under a message, the message
// sits gap-2 under the control, and the label turns destructive.
func TestShad2aFormShadcnSpacing(t *testing.T) {
	a, b := "", ""
	var f *FormW
	var ia, ib *InputW
	first := true
	d := p2dDriver(theme.Shadcn(), func(c *Context) {
		f = Form(c).Vertical()
		f.Body(func() {
			f.Item("甲").Rules(&a, Required("甲必填")).Body(func() { ia = Input(c, &a) })
			f.Item("乙").Body(func() { ib = Input(c, &b) })
			if first {
				first = false
				f.Validate()
			}
		})
	})
	d.Settle()
	ra, rb := d.r.c.rects[ia.box.id], d.r.c.rects[ib.box.id]
	msg, ok := d.FindText("甲必填")
	if !ok {
		t.Fatal("message missing")
	}
	if g := msg.Y - ra.Bottom(); g < shadFormGap-0.5 || g > shadFormGap+0.5 {
		t.Fatalf("message gap %v, want %v", g, shadFormGap)
	}
	lab, _ := d.FindText("乙")
	if g := lab.Y - msg.Bottom(); g < 23 {
		t.Fatalf("item gap under a message %v, want >= 24", g)
	}
	// Vertical label: leading-none line + gap-2 above the control.
	if g := rb.Y - lab.Y; g > shadFormLabelH+shadFormGap+4 {
		t.Fatalf("label to control %v", g)
	}
}

func TestShad2aSliderCrossEnd(t *testing.T) {
	lo, hi := 20.0, 60.0
	var w *SliderW
	d := fillDriver(func() *theme.Theme { return theme.Shadcn() }, func(c *Context) {
		Column(c, func() { w = SliderRange(c, &lo, &hi, 0, 100).Layout(layout.W(412)) }, layout.Pad(20))
	})
	tr := p2bRect(t, d, w.track)
	h1 := p2bRect(t, d, w.thumbs[1])
	x10 := tr.X + sliderPad + (tr.W-2*sliderPad)*0.1
	d.Drag(h1.Center(), geom.Pt(x10, h1.Center().Y))
	if lo != 10 || hi != 20 {
		t.Fatalf("end crossing start: lo %v hi %v", lo, hi)
	}
	if d.r.c.focus != w.thumbs[0].id {
		t.Fatal("focus did not follow")
	}
	if c := shadSliderRing(theme.Shadcn()); c.A == 0 {
		t.Fatal("no ring color")
	}
}

func TestShad2aUploadCustomRequestAndCardRetry(t *testing.T) {
	dir := t.TempDir()
	p := p2dFile(t, dir, "pic.png", 10)
	var files []UploadFile
	fail := true
	calls := 0
	d := p2dDriver(theme.Shadcn(), func(c *Context) {
		Upload(c, &files).PictureCard().
			CustomRequest(func(ctx context.Context, f UploadFile, progress func(float64)) ([]byte, error) {
				calls++
				progress(50)
				if fail {
					fail = false
					return nil, context.Canceled
				}
				return []byte("ok"), nil
			}).
			Picker(func(string, bool, bool) ([]string, error) { return []string{p}, nil })
	})
	d.EnableAccessibility()
	if err := d.ClickLabel(ZhCN.Upload.Trigger); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "failed custom upload", func() bool { return len(files) == 1 && files[0].Status == "error" })
	if _, ok := d.Find(ZhCN.Retry); !ok {
		t.Fatal("picture card has no retry button")
	}
	if err := d.ClickLabel(ZhCN.Retry); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "retried custom upload", func() bool { return files[0].Status == "done" })
	if calls != 2 || string(files[0].Response) != "ok" {
		t.Fatalf("calls %d resp %q", calls, files[0].Response)
	}
}

func TestShad2aUploadDefaults(t *testing.T) {
	var files []UploadFile
	var w *UploadW
	p2dDriver(theme.AntDesign(), func(c *Context) {
		ConfigProvider(c).Defaults("Upload", Props{"listType": "picture-card", "directory": true, "disabled": true}).Body(func() {
			w = Upload(c, &files)
		})
	})
	if w.kind != upCard || !w.dir || !w.disabled {
		t.Fatalf("defaults: kind %v dir %v disabled %v", w.kind, w.dir, w.disabled)
	}
}

func TestShad2aTransferPagination(t *testing.T) {
	items := make([]TransferItem, 0, 13)
	for i := 0; i < 13; i++ {
		items = append(items, TransferItem{Key: itoaEnt(i), Title: "项" + itoaEnt(i)})
	}
	var target []string
	d := p2dDriver(theme.Shadcn(), func(c *Context) {
		ConfigProvider(c).Defaults("Transfer", Props{"titles": []string{"左", "右"}}).Body(func() {
			Transfer(c, items, &target).Pagination(5)
		})
	})
	if _, ok := d.FindText("左"); !ok {
		t.Fatal("titles default not applied")
	}
	if _, ok := d.FindText("项0"); !ok {
		t.Fatal("first page missing")
	}
	// The pager is the simple Pagination: next page button.
	if err := d.ClickLabel(ZhCN.NextPage); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if _, ok := d.FindText("项5"); !ok {
		t.Fatal("next page not shown")
	}
	if _, ok := d.FindText("项0"); ok {
		t.Fatal("first page still shown")
	}
}
