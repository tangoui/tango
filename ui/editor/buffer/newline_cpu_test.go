//go:build darwin || linux

package buffer

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// BenchmarkLeafNewlineRankCPU also reports the calling thread's CPU time:
// wall time counts other sessions preempting this process, and is not a
// trustworthy measure of this tiny operation on a busy development host.
func BenchmarkLeafNewlineRankCPU(b *testing.B) {
	for _, width := range []int{1, 2, 10, 100, 1024} {
		b.Run(fmt.Sprintf("width%d", width), func(b *testing.B) {
			n := newLeaf(strings.Repeat(strings.Repeat("x", width-1)+"\n", leafTarget/width))
			for _, arm := range []struct {
				name string
				read func(*node, int) int
			}{{"linear", linearAfterNewline}, {"candidate", afterNewline}} {
				b.Run(arm.name, func(b *testing.B) {
					runtime.LockOSThread()
					defer runtime.UnlockOSThread()
					var start, end unix.Timespec
					k := 0
					b.ReportAllocs()
					b.ResetTimer()
					if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &start); err != nil {
						b.Fatal(err)
					}
					for range b.N {
						k = (k + 37) % n.lines
						newlineQuery = arm.read(n, k+1)
					}
					if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &end); err != nil {
						b.Fatal(err)
					}
					b.StopTimer()
					b.ReportMetric(float64(end.Nano()-start.Nano())/float64(b.N), "cpu-ns/op")
				})
			}
		})
	}
}

// Snapshot-level counterpart, with the same actual Line queries as
// BenchmarkShortLineRead rather than an isolated leaf.
func BenchmarkRopeLineReadCPU(b *testing.B) {
	for _, width := range []int{10, 100, 3000} {
		b.Run(fmt.Sprintf("width%d", width), func(b *testing.B) {
			s := NewSnapshot(strings.Repeat(strings.Repeat("x", width-1)+"\n", 10000))
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			var start, end unix.Timespec
			i := 0
			b.ReportAllocs()
			b.ResetTimer()
			if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &start); err != nil {
				b.Fatal(err)
			}
			for range b.N {
				i = (i + 7919) % 10000
				queriedLine = s.Line(i)
			}
			if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &end); err != nil {
				b.Fatal(err)
			}
			b.StopTimer()
			b.ReportMetric(float64(end.Nano()-start.Nano())/float64(b.N), "cpu-ns/op")
		})
	}
}

// BenchmarkPackedLeafNewlineCPU: the shape where the dense-leaf cutoff can
// cost more than the byte search: 40 newlines packed in the first bytes of
// a ~2 KB leaf, queried at a low rank just past the cutoff.
func BenchmarkPackedLeafNewlineCPU(b *testing.B) {
	n := newLeaf(strings.Repeat("\n", 40) + strings.Repeat("z", leafMax-40))
	for _, arm := range []struct {
		name string
		read func(*node, int) int
	}{{"linear", linearAfterNewline}, {"candidate", afterNewline}} {
		b.Run(arm.name, func(b *testing.B) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			var start, end unix.Timespec
			b.ReportAllocs()
			b.ResetTimer()
			if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &start); err != nil {
				b.Fatal(err)
			}
			for range b.N {
				newlineQuery = arm.read(n, 9)
			}
			if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &end); err != nil {
				b.Fatal(err)
			}
			b.StopTimer()
			b.ReportMetric(float64(end.Nano()-start.Nano())/float64(b.N), "cpu-ns/op")
		})
	}
}
