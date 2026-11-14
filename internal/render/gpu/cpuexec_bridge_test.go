package gpu_test

import (
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
)

// The package's internal tests draw on cpuexec.Device, which they cannot
// import (cpuexec imports gpu): this external test file hands it to them
// (gpu.NewCPUExecDevice, see cpuDevice in renderer_test.go). Package
// initialization ends before any test runs, so they always find it set.
func init() {
	gpu.NewCPUExecDevice = func() gpu.CPUExecDevice { return bridgeDevice{cpuexec.NewDevice()} }
	gpu.CPUExecRun = cpuexec.Run
}

type bridgeDevice struct{ *cpuexec.Device }

func (b bridgeDevice) State() gpu.CPUExecState {
	s := b.Stats()
	m, c, r := b.Textures()
	return gpu.CPUExecState{
		Calls: s.Calls, Executed: s.Executed, BatchUploads: s.BatchUploads, LiveLayers: s.LiveLayers,
		Img: b.Target(), Mask: m, Color: c, Ramp: r, Images: b.Images(), Last: b.Last(),
	}
}
