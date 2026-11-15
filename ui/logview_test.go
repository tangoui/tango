package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestDetectLogLevel(t *testing.T) {
	for in, want := range map[string]LogLevel{
		"2026-11-14 INFO server up":        LogInfo,
		"[warn] disk":                      LogWarn,
		"level=error msg=x":                LogError,
		"E: FATAL boom":                    LogError,
		"time=1 DBG tick":                  LogDebug,
		"information only":                 LogNone, // part of a longer word
		"the user's error count is 0":      LogError,
		"err := f() // lower-case err ok?": LogNone, // abbreviations are upper-case only
		"":                                 LogNone,
	} {
		if got := DetectLogLevel(in); got != want {
			t.Errorf("DetectLogLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLogBufferWriteAndLimit(t *testing.T) {
	b := NewLogBuffer(3)
	calls := 0
	b.OnChange(func() { calls++ })
	fmt.Fprint(b, "INFO one\nERROR two\n  at x\npart")
	if b.Len() != 3 || b.Line(2) != "  at x" {
		t.Fatalf("lines: %d %q", b.Len(), b.Line(2))
	}
	if b.Level(2) != LogError {
		t.Errorf("a continuation line takes the level above, got %v", b.Level(2))
	}
	b.Flush()
	if b.Len() != 3 || b.Line(0) != "ERROR two" || b.Line(2) != "part" {
		t.Errorf("limit: %d %q %q", b.Len(), b.Line(0), b.Line(2))
	}
	if calls != 2 {
		t.Errorf("OnChange calls = %d", calls)
	}
	b.Append("\x1b[31mred\x1b[0m")
	if b.Line(2) != "red" {
		t.Errorf("escape sequences must be stripped: %q", b.Line(2))
	}
	b.Clear()
	if b.Len() != 0 {
		t.Error("Clear")
	}
}

func logViewDriver(buf *LogBuffer, opts ...func(*LogViewW)) (*Driver, **LogViewW) {
	var cur *LogViewW
	d := NewDriver(func(c *Context) {
		cur = LogView(c, buf).Height(300)
		for _, o := range opts {
			o(cur)
		}
	}, DriverOptions{Width: 800, Height: 400})
	d.Frame()
	d.Frame()
	return d, &cur
}

func TestLogViewSearchAndNavigate(t *testing.T) {
	buf := NewLogBuffer(0)
	buf.Append("INFO a conn here", "ERROR conn reset", "plain", "conn conn")
	q := "conn"
	d, w := logViewDriver(buf, func(w *LogViewW) { w.Query(&q) })
	if n := (*w).Matches(); n != 4 {
		t.Fatalf("matches = %d, want 4", n)
	}
	if _, ok := d.FindText("1/4"); !ok {
		t.Error("match counter 1/4 not shown")
	}
	if err := d.ClickLabel("下一个匹配"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.FindText("2/4"); !ok {
		t.Error("next match: 2/4 not shown")
	}
	if err := d.ClickLabel("上一个匹配"); err != nil {
		t.Fatal(err)
	}
	d.ClickLabel("上一个匹配")
	if _, ok := d.FindText("4/4"); !ok {
		t.Error("previous wraps around to 4/4")
	}
	q = "nothing"
	d.Frame()
	if _, ok := d.FindText("无匹配"); !ok {
		t.Error("no match text")
	}
}

func TestLogViewLevelFilter(t *testing.T) {
	buf := NewLogBuffer(0)
	buf.Append("DEBUG a", "INFO b", "WARN c", "  continuation of c", "ERROR d", "untagged")
	lv := LogWarn
	var v *txView
	d := NewDriver(func(c *Context) {
		w := LogView(c, buf).Height(300).MinLevel(&lv)
		w.z.run()
		v = w.v
	}, DriverOptions{Width: 800, Height: 400})
	d.Frame()
	got := []string{}
	for i := 0; i < v.count; i++ {
		got = append(got, v.line(i))
	}
	if strings.Join(got, "|") != "WARN c|  continuation of c|ERROR d|untagged" {
		t.Errorf("filtered: %q", got)
	}
	lv = LogNone
	d.Frame()
	if v.count != 6 {
		t.Errorf("unfiltered: %d", v.count)
	}
}

func TestLogViewFollowsUntilScrolledUp(t *testing.T) {
	buf := NewLogBuffer(0)
	for i := 0; i < 200; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
	}
	follow := true
	var w *LogViewW
	d := NewDriver(func(c *Context) {
		w = LogView(c, buf).Height(300).Follow(&follow).NoToolbar()
		w.z.run()
	}, DriverOptions{Width: 800, Height: 400})
	d.Frame()
	d.Frame()
	if !w.v.atBottom() {
		t.Fatal("a following view starts at the bottom")
	}
	buf.Append("new line")
	d.Frame()
	d.Frame()
	if !w.v.atBottom() {
		t.Error("appended lines keep a following view at the bottom")
	}
	d.Scroll(geom.Pt(300, 150), -200)
	d.Frame()
	if follow {
		t.Fatal("scrolling up stops following")
	}
	off := w.v.sb.s.off.Y
	buf.Append("another")
	d.Frame()
	if w.v.sb.s.off.Y != off {
		t.Error("a view that does not follow stays put")
	}
	d.Key(input.KeyEnd, 0) // not focused: no effect
	d.Scroll(geom.Pt(300, 150), 100000)
	d.Frame()
	if !follow {
		t.Error("scrolling back to the bottom follows again")
	}
}

func TestLogViewSelectCopy(t *testing.T) {
	buf := NewLogBuffer(0)
	buf.Append("alpha beta", "gamma delta")
	var w *LogViewW
	d := NewDriver(func(c *Context) {
		w = LogView(c, buf).Height(300).NoToolbar()
		w.z.run()
	}, DriverOptions{Width: 800, Height: 400})
	d.Frame()
	d.Frame()
	st := w.v.st
	y0 := st.originY + w.v.padY + w.v.lineH/2
	x0 := st.textX + 1
	d.Drag(geom.Pt(x0, y0), geom.Pt(st.textX+w.v.cellW()*5, y0+w.v.lineH))
	if got := w.Selection(); got != "alpha beta\ngamma" {
		t.Fatalf("selection = %q", got)
	}
	d.Key(input.KeyC, input.ModCtrl|input.ModMeta)
	if d.Clipboard() != "alpha beta\ngamma" {
		t.Errorf("clipboard = %q", d.Clipboard())
	}
	d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
	if w.Selection() != "alpha beta\ngamma delta" {
		t.Errorf("select all = %q", w.Selection())
	}
}

func TestLogViewWrapRows(t *testing.T) {
	buf := NewLogBuffer(0)
	buf.Append(strings.Repeat("x", 500), "short")
	wrap := true
	var w *LogViewW
	d := NewDriver(func(c *Context) {
		w = LogView(c, buf).Height(300).Wrap(&wrap).NoToolbar()
		w.z.run()
	}, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	d.Frame()
	cols := w.v.st.wrapCols
	if rows := w.v.rows(); rows != (500+cols-1)/cols+1 {
		t.Errorf("rows = %d with %d columns", rows, cols)
	}
	wrap = false
	d.Frame()
	if w.v.rows() != 2 {
		t.Errorf("unwrapped rows = %d", w.v.rows())
	}
}

func BenchmarkLogViewFrameMillion(b *testing.B) {
	buf := NewLogBuffer(0)
	lines := make([]string, 1_000_000)
	for i := range lines {
		lines[i] = fmt.Sprintf("2026-11-14 10:00:00.%03d \x1b[32mINFO\x1b[0m worker-%d: job %d done", i%1000, i%16, i)
	}
	buf.Append(lines...)
	d := NewDriver(func(c *Context) { LogView(c, buf).Height(600).LineNumbers(true) }, DriverOptions{Width: 1000, Height: 700})
	d.Frame()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Append("INFO appended line")
		d.Frame()
	}
}

func BenchmarkLogBufferAppend(b *testing.B) {
	buf := NewLogBuffer(100_000)
	line := "2026-11-14 10:00:00.000 \x1b[33mWARN\x1b[0m cache: miss ratio \x1b[1m42%\x1b[22m"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Append(line)
	}
}

func TestLogViewLimitKeepsPosition(t *testing.T) {
	buf := NewLogBuffer(100)
	for i := 0; i < 100; i++ {
		buf.Append(fmt.Sprintf("line %d", i))
	}
	follow := false
	var w *LogViewW
	d := NewDriver(func(c *Context) {
		w = LogView(c, buf).Height(300).Follow(&follow).NoToolbar()
		w.z.run()
	}, DriverOptions{Width: 800, Height: 400})
	d.Frame()
	w.v.sb.s.off.Y = 40 * float64(w.v.lineH)
	d.Frame()
	d.Frame()
	top := func() string { line, _ := w.v.rowLine(int(w.v.sb.s.off.Y / float64(w.v.lineH))); return w.v.line(line) }
	before := top()
	for i := 0; i < 10; i++ {
		buf.Append(fmt.Sprintf("new %d", i))
	}
	d.Frame()
	d.Frame()
	if got := top(); got != before {
		t.Errorf("top line moved from %q to %q", before, got)
	}
	if follow {
		t.Error("dropping lines must not turn following on")
	}
	follow = true
	d.Frame()
	for i := 0; i < 10; i++ {
		buf.Append(fmt.Sprintf("more %d", i))
	}
	d.Frame()
	d.Frame()
	if !follow || !w.v.atBottom() {
		t.Error("a following view stays at the bottom while old lines drop")
	}
}
