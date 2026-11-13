package ui

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// moreBoxPage renders a trigger button that runs open once per click.
func moreBoxPage(open func(c *Context)) func(c *Context) {
	return func(c *Context) {
		Column(c, func() {
			if Button(c, "打开").Clicked() {
				open(c)
			}
		}, layout.Pad(20))
	}
}

func TestMoreConfirmOkCancelEscape(t *testing.T) {
	oks, cancels := 0, 0
	var ref *ConfirmRef
	d := driver(moreBoxPage(func(c *Context) {
		ref = Confirm(c, ConfirmOptions{Title: "删除确认", Content: "确定删除该文件吗？",
			OnOk: func(*ConfirmRef) { oks++ }, OnCancel: func() { cancels++ }})
	}))
	d.Frame()
	d.ClickLabel("打开")
	if _, ok := d.Find("确定"); !ok || !ref.IsOpen() {
		t.Fatal("confirm not shown")
	}
	// The page underneath is blocked by the mask.
	d.ClickLabel("打开")
	if OpenDialogs(d.r.c) != 1 {
		t.Fatalf("mask did not block: %d dialogs", OpenDialogs(d.r.c))
	}
	d.ClickLabel("确定")
	if oks != 1 || cancels != 0 || ref.IsOpen() {
		t.Fatalf("ok: oks=%d cancels=%d open=%v", oks, cancels, ref.IsOpen())
	}
	if _, ok := d.Find("确定"); ok {
		t.Fatal("dialog still drawn")
	}
	d.ClickLabel("打开")
	d.ClickLabel("取消")
	if cancels != 1 || ref.IsOpen() {
		t.Fatalf("cancel: %d", cancels)
	}
	d.ClickLabel("打开")
	d.Key(input.KeyEscape, 0)
	if cancels != 2 || ref.IsOpen() {
		t.Fatalf("escape: %d", cancels)
	}
	// Mask clicks are ignored unless MaskClosable.
	d.ClickLabel("打开")
	d.Click(geom.Pt(5, 395))
	if !ref.IsOpen() {
		t.Fatal("mask closed a non-mask-closable dialog")
	}
	ref.Close()
	d.Settle()
	if OpenDialogs(d.r.c) != 0 || oks != 1 || cancels != 2 {
		t.Fatalf("Close ran callbacks or left dialog: %d %d %d", OpenDialogs(d.r.c), oks, cancels)
	}
}

func TestMoreConfirmMaskAndKeyboard(t *testing.T) {
	oks, cancels := 0, 0
	d := driver(moreBoxPage(func(c *Context) {
		Confirm(c, ConfirmOptions{Title: "提示", MaskClosable: true, Danger: true,
			OnOk: func(*ConfirmRef) { oks++ }, OnCancel: func() { cancels++ }})
	}))
	d.Frame()
	d.ClickLabel("打开")
	d.Click(geom.Pt(5, 395))
	if cancels != 1 || OpenDialogs(d.r.c) != 0 {
		t.Fatalf("mask close: %d", cancels)
	}
	// OK has focus when the dialog opens: Enter confirms.
	d.ClickLabel("打开")
	d.Key(input.KeyEnter, 0)
	if oks != 1 || OpenDialogs(d.r.c) != 0 {
		t.Fatalf("enter: oks=%d", oks)
	}
}

func TestMoreConfirmLoading(t *testing.T) {
	calls := 0
	var ref *ConfirmRef
	d := driver(moreBoxPage(func(c *Context) {
		ref = Confirm(c, ConfirmOptions{Title: "提交", OnOk: func(r *ConfirmRef) { calls++; r.Loading(true) }})
	}))
	d.Frame()
	d.ClickLabel("打开")
	d.ClickLabel("确定")
	if calls != 1 || !ref.IsOpen() {
		t.Fatalf("loading should keep dialog: calls=%d open=%v", calls, ref.IsOpen())
	}
	d.ClickLabel("确定")
	if calls != 1 {
		t.Fatal("OK ran while loading")
	}
	ref.Close()
	d.Settle()
	if OpenDialogs(d.r.c) != 0 {
		t.Fatal("not closed")
	}
}

func TestMoreMessageBoxAndStacking(t *testing.T) {
	oks := 0
	d := driver(moreBoxPage(func(c *Context) {
		MessageBox(c, ConfirmOptions{Title: "成功", Content: "已保存", Status: "success", OnOk: func(*ConfirmRef) { oks++ }})
	}))
	d.Frame()
	d.ClickLabel("打开")
	if _, ok := d.Find("取消"); ok {
		t.Fatal("message box has a cancel button")
	}
	d.ClickLabel("知道了")
	if oks != 1 || OpenDialogs(d.r.c) != 0 {
		t.Fatalf("ok: %d", oks)
	}
	// Element: single "确定" plus a close button; a nested Confirm opened
	// from OnOk stacks on top and Escape only closes the top one.
	inner := 0
	d2 := NewDriver(moreBoxPage(func(c *Context) {
		MessageBox(c, ConfirmOptions{Title: "外层", OnOk: func(*ConfirmRef) {}})
		Confirm(c, ConfirmOptions{Title: "内层", OnCancel: func() { inner++ }})
	}), DriverOptions{Width: 600, Height: 400, Theme: theme.ElementPlus()})
	d2.Frame()
	d2.ClickLabel("打开")
	if OpenDialogs(d2.r.c) != 2 {
		t.Fatalf("stack: %d", OpenDialogs(d2.r.c))
	}
	if _, ok := d2.Find("关闭"); !ok {
		t.Fatal("element close button missing")
	}
	d2.Key(input.KeyEscape, 0)
	if inner != 1 || OpenDialogs(d2.r.c) != 1 {
		t.Fatalf("escape top: inner=%d open=%d", inner, OpenDialogs(d2.r.c))
	}
	d2.ClickLabel("关闭")
	if OpenDialogs(d2.r.c) != 0 {
		t.Fatal("close button")
	}
}

func TestMorePromptValidate(t *testing.T) {
	got := ""
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		got = ""
		d := NewDriver(moreBoxPage(func(c *Context) {
			Prompt(c, ConfirmOptions{Title: "请输入邮箱", InputPlaceholder: "name@example.com",
				Validate: func(v string) string {
					if !strings.Contains(v, "@") {
						return "邮箱格式不正确"
					}
					return ""
				},
				OnOk: func(r *ConfirmRef) { got = r.Value() }})
		}), DriverOptions{Width: 600, Height: 400, Theme: th})
		d.Frame()
		d.ClickLabel("打开")
		// The input has focus: typing goes straight in.
		d.Type("abc")
		d.ClickLabel("确定")
		if got != "" || OpenDialogs(d.r.c) != 1 {
			t.Fatalf("%s: invalid value accepted %q", th.Name, got)
		}
		if _, ok := d.Find("邮箱格式不正确"); !ok {
			// error text is not interactive; check the state instead
			bs := State[moreBoxes](d.r.c, moreBoxesKey, nil)
			if bs.list[0].errMsg == "" {
				t.Fatalf("%s: no error", th.Name)
			}
		}
		// Focus returns to the input with its text selected; retype.
		d.Type("abc@x.cn")
		if bs := State[moreBoxes](d.r.c, moreBoxesKey, nil); bs.list[0].errMsg != "" {
			t.Fatalf("%s: error not revalidated", th.Name)
		}
		d.Key(input.KeyEnter, 0)
		if got != "abc@x.cn" || OpenDialogs(d.r.c) != 0 {
			t.Fatalf("%s: submit got %q", th.Name, got)
		}
	}
}

func TestMoreConfirmCloseFromGoroutineWakes(t *testing.T) {
	var ref *ConfirmRef
	done := make(chan struct{})
	// release holds the goroutine until ClickLabel has returned: Close must
	// land while the driver is idle, or the frames ClickLabel settles would
	// consume the invalidation before the test looks at it.
	release := make(chan struct{})
	d := driver(moreBoxPage(func(c *Context) {
		ref = Confirm(c, ConfirmOptions{Title: "上传", OnOk: func(r *ConfirmRef) {
			r.Loading(true)
			go func() { <-release; r.Close(); close(done) }()
		}})
	}))
	d.Frame()
	d.ClickLabel("打开")
	d.ClickLabel("确定")
	d.w.invalid.Store(false)
	close(release)
	<-done
	if !d.w.invalid.Load() {
		t.Fatal("Close from goroutine did not request a frame")
	}
	d.Frame()
	if ref.IsOpen() || OpenDialogs(d.r.c) != 0 {
		t.Fatal("dialog still open")
	}
}
