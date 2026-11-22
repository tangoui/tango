package ui

import (
	"strings"
	"testing"
)

func BenchmarkCodeBlockWrappedTail(b *testing.B) {
	code := strings.Repeat("abcd1234\t界", 50_000)
	var w *CodeBlockW
	d := NewDriver(func(c *Context) {
		w = CodeBlock(c, code).Wrap(true).Highlighter(nil).NoCopy().Height(480)
	}, DriverOptions{Width: 800, Height: 500})
	d.Frame()
	w.v.sb.s.off.Y = float64(w.v.rows()) * float64(w.v.lineH) * 0.95
	d.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.v.sb.s.off.Y += float64(w.v.lineH)
		d.Frame()
	}
}
