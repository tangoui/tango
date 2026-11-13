package gpu

import (
	"testing"

	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/gpu/internal/racetest"
)

// TestMain pins the pure-Go text engine for every test of the package
// (see gputest.Main): the parity scenes and golden pages compare against
// masks rasterized from outlines in Go. Tests of the system engine switch
// it for themselves with gputest.SystemText.
//
// Under -race (racetest.Enabled) the scenes are compared at scales 1 and 2
// only: every comparison renders through cpuexec, 10–20× slower under the
// race detector, and the fractional scales add no concurrency coverage.
func TestMain(m *testing.M) {
	if racetest.Enabled {
		gputest.Scales = []float32{1, 2}
	}
	gputest.Main(m)
}
