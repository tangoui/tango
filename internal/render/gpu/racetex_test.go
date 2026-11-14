package gpu

import (
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/racetest"
)

// raceSkip skips t under -race (racetest.Enabled) unless keep: the heavy
// pixel tests (every case renders full frames through cpuexec, 10–20×
// slower under the race detector) run a representative subset of their
// cases there, at least one per test, and every case without -race.
func raceSkip(t *testing.T, keep bool) {
	t.Helper()
	if racetest.Enabled && !keep {
		t.Skip("slow under -race: a representative subset of the cases runs")
	}
}
