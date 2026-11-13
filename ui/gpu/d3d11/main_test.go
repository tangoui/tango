package d3d11

import (
	"testing"

	"github.com/tangoui/tango/ui/gpu/internal/gputest"
)

// TestMain pins the pure-Go text engine for every test of the package
// (see gputest.Main): the parity scenes compare against masks rasterized
// from outlines in Go. Tests of the system engine switch it for
// themselves with gputest.SystemText.
func TestMain(m *testing.M) { gputest.Main(m) }
