package frameclock

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32  = windows.NewLazySystemDLL("kernel32.dll")
	frequency = kernel32.NewProc("QueryPerformanceFrequency")
	clock     = newClock()
)

// QueryPerformanceCounter is already linked by the Go runtime. Reusing that
// leaf avoids a syscall wrapper on every line-budget check.
//
//go:linkname runtimeQueryPerformanceCounter internal/syscall/windows.QueryPerformanceCounter
func runtimeQueryPerformanceCounter() int64

type performanceClock struct {
	frequency int64
	origin    int64
	base      time.Time
}

func newClock() performanceClock {
	var hz int64
	ok, _, err := frequency.Call(uintptr(unsafe.Pointer(&hz)))
	if ok == 0 || hz <= 0 {
		panic(fmt.Sprintf("frameclock: QueryPerformanceFrequency: %v (frequency %d)", err, hz))
	}
	origin := readCounter()
	return performanceClock{frequency: hz, origin: origin, base: time.Now()}
}

func readCounter() int64 { return runtimeQueryPerformanceCounter() }

// Now advances one shared wall-clock anchor by QPC elapsed time. Round(0)
// intentionally removes Go's monotonic component: callers may pass deadlines
// made with time.Now, whose Windows interrupt-time monotonic domain is not the
// QPC domain. Comparing wall values keeps both callers in one time.Time domain.
// QPC is a performance timer with an OS-specified fixed frequency, NOT the
// processor-cycle count returned by QueryThreadCycleTime. It includes waiting.
func Now() time.Time {
	return clock.base.Add(counterDuration(readCounter()-clock.origin, clock.frequency)).Round(0)
}
