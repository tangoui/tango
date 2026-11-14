package d3d11

import (
	"os"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
)

// hardwareTestEnv is the environment variable that runs the package's
// tests on the hardware adapter when set to "1" (see TestMain).
const hardwareTestEnv = "TANGO_D3D_HARDWARE"

// TestMain pins the pure-Go text engine for every test of the package
// (see gputest.Main): the parity scenes compare against masks rasterized
// from outlines in Go. Tests of the system engine switch it for
// themselves with gputest.SystemText.
//
// The tests run on WARP (TANGO_D3D_WARP=1) unless TANGO_D3D_HARDWARE=1 or
// TANGO_D3D_WARP is already set: the pixel-exact parity scenes then give
// the same result on every machine, as on CI runners, which have no GPU.
// Some hardware drivers do not (an Intel Iris Xe driver of 2023,
// 31.0.101.4146, samples stale texels after a binding change in some
// frames, failing the layer and image scenes depending on its shader
// cache).
func TestMain(m *testing.M) {
	if os.Getenv(hardwareTestEnv) != "1" && os.Getenv("TANGO_D3D_WARP") == "" {
		os.Setenv("TANGO_D3D_WARP", "1")
	}
	gputest.Main(m)
}
