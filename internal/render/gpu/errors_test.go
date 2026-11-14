package gpu_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/metal"
)

// TestDeviceErrorsShared: the device errors live in package gpu, shared by
// every backend; metal's names are the same values, so callers testing
// either (uihost tests metal.ErrDeviceLost) match errors of any device.
func TestDeviceErrorsShared(t *testing.T) {
	for _, c := range []struct {
		name       string
		metal, gpu error
	}{
		{"ErrDeviceLost", metal.ErrDeviceLost, gpu.ErrDeviceLost},
		{"ErrTargetTooLarge", metal.ErrTargetTooLarge, gpu.ErrTargetTooLarge},
	} {
		if c.metal != c.gpu {
			t.Errorf("metal.%s is not gpu.%s", c.name, c.name)
		}
		if !errors.Is(fmt.Errorf("x: %w", c.metal), c.gpu) {
			t.Errorf("an error wrapping metal.%s is not gpu.%s", c.name, c.name)
		}
		if !errors.Is(fmt.Errorf("x: %w", c.gpu), c.metal) {
			t.Errorf("an error wrapping gpu.%s is not metal.%s", c.name, c.name)
		}
	}
	if errors.Is(gpu.ErrDeviceLost, gpu.ErrTargetTooLarge) || errors.Is(gpu.ErrTargetTooLarge, gpu.ErrDeviceLost) {
		t.Error("ErrDeviceLost and ErrTargetTooLarge match each other")
	}
}
