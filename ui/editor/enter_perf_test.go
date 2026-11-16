package editor

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// BenchmarkEnterManyCursorsStale measures just Type for the same document
// and stale-highlighter case as TestEnterManyCursorsStale. The assist time
// budget can change how much highlighter work it does under load; use the
// Warm companion when comparing a fixed amount of work.
func BenchmarkEnterManyCursorsStale(b *testing.B) { benchmarkEnterManyCursors(b, false) }

// BenchmarkEnterManyCursorsWarm starts with every line final, so a faster
// query or a scheduler pause cannot change how much synchronous catch-up
// fits in the typing assistance budget. Setup and Close are not timed.
func BenchmarkEnterManyCursorsWarm(b *testing.B) { benchmarkEnterManyCursors(b, true) }

func benchmarkEnterManyCursors(b *testing.B, warm bool) {
	b.StopTimer() // exclude even the shared fixture and thread setup
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var cpu time.Duration
	var doc strings.Builder
	for i := range 10000 {
		if i%10 == 0 {
			doc.WriteString("\tf(a) {\n")
		} else {
			doc.WriteString("\tx := f(a)\n")
		}
	}
	text := doc.String()
	b.ResetTimer() // discard the entry-to-StopTimer interval as well
	b.ReportAllocs()
	for i := range b.N {
		b.StopTimer()
		if i == 0 && b.Elapsed() != 0 {
			b.Fatal("fixture construction was included in the benchmark timer")
		}
		st := NewState(text, "a.go", nil)
		snap := st.Snapshot()
		sels := make([]Selection, 0, 1000)
		for i := 0; i < 10000; i += 10 {
			end := lineEnd(snap, i)
			sels = append(sels, Selection{Anchor: end, Head: end})
		}
		st.SetSelections(sels)
		if warm {
			hl := st.Highlighter()
			hl.TokenWindow = snap.LineCount()
			if !hl.HighlightFor(st.hlKey, snap, snap.LineCount()-1, time.Hour) {
				st.Close()
				b.Fatal("highlighter fixture did not finish")
			}
		}
		b.StartTimer()
		start := workClock()
		st.Type("\n")
		cpu += workClock() - start
		b.StopTimer()
		if st.Snapshot().LineCount() != 11001 || len(st.Sels) != 1000 {
			b.Fatal("Enter lost lines or cursors")
		}
		st.Close()
	}
	if threadCPUClock {
		b.ReportMetric(float64(cpu.Nanoseconds())/float64(b.N), "cpu-ns/op")
	}
}
