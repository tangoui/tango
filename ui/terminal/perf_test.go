package terminal

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/terminal/vt"
)

func TestPerfStatsRecordFeedLockWait(t *testing.T) {
	term := New(Options{Conn: newPipeConn()})
	t.Cleanup(func() { term.Close() })
	term.feed([]byte(strings.Repeat("ansi ", 1024)))
	st := term.perfStats()
	if st.FeedBytes == 0 || st.FeedBatches == 0 {
		t.Fatalf("feed stats = %+v, want bytes and batches", st)
	}
	if st.FeedLockWait == 0 {
		t.Logf("feed lock wait = %d ns", st.FeedLockWait)
	}
}

func TestViewPerfStatsCountsPaintRows(t *testing.T) {
	g := newViewRig(t, 600, 300, nil)
	g.output("one\r\ntwo\r\n", func(v *vt.Terminal) bool { return rowText(v, 1) == "two" })
	g.d.Frame()
	st := g.vw.perfStats()
	if st.PaintRows == 0 {
		t.Fatalf("paint stats = %+v, want painted rows", st)
	}
}

func BenchmarkTerminalASCII(b *testing.B) {
	data := []byte(strings.Repeat("plain terminal output\r\n", 4096))
	b.ReportMetric(float64(len(data)), "bytes/input")
	b.ReportAllocs()
	for range b.N {
		v := vt.New(vt.Options{Cols: 120, Rows: 40})
		b.StartTimer()
		v.Write(data)
		b.StopTimer()
	}
}

func BenchmarkTerminalANSI(b *testing.B) {
	data := []byte(strings.Repeat("\x1b[38;5;208mcolored output\x1b[0m\r\n", 4096))
	b.ReportMetric(float64(len(data)), "bytes/input")
	b.ReportAllocs()
	for range b.N {
		v := vt.New(vt.Options{Cols: 120, Rows: 40})
		b.StartTimer()
		v.Write(data)
		b.StopTimer()
	}
}

func BenchmarkTerminalPTYReplay(b *testing.B) {
	data := []byte(strings.Repeat("\x1b[32mfile output\x1b[0m\r\n", 4096))
	b.ReportMetric(float64(len(data)), "bytes/input")
	b.ReportAllocs()
	for range b.N {
		v := vt.New(vt.Options{Cols: 120, Rows: 40})
		b.StartTimer()
		v.Write(data)
		b.StopTimer()
	}
}

func BenchmarkViewFrameIdle(b *testing.B) {
	g := newViewRig(b, 900, 600, nil)
	g.output("stable output\r\n", func(v *vt.Terminal) bool { return rowText(v, 0) == "stable output" })
	g.d.Frame()
	b.ResetTimer()
	for range b.N {
		g.d.Frame()
	}
}

func BenchmarkViewFrameOneDirtyRow(b *testing.B) {
	g := newViewRig(b, 900, 600, nil)
	g.output(strings.Repeat("line\r\n", 100), func(v *vt.Terminal) bool { return true })
	g.d.Frame()
	b.ResetTimer()
	for i := range b.N {
		g.output("dirty "+itoaTest(i)+"\r\n", func(v *vt.Terminal) bool { return true })
	}
}

func BenchmarkViewFrameScroll(b *testing.B) {
	g := newViewRig(b, 900, 600, nil)
	g.fill(100)
	g.d.Frame()
	b.ResetTimer()
	for i := range b.N {
		g.output("scroll "+itoaTest(i)+"\r\n", func(v *vt.Terminal) bool { return true })
	}
}
