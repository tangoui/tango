package gpu

import (
	"image"
	"os"
	"strings"
)

// BudgetPolicy controls rebuildable GPU residency. It is internal and has no
// effect on renderer selection.
type BudgetPolicy struct {
	AtlasMaxPages   int
	AtlasIdleFrames uint32
	ImageMaxBytes   int64
	ImageIdleFrames uint64
	LayerMaxBytes   int64
}

func gpuBudgetEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TANGO_GPU_BUDGET")))
	return v != "0" && v != "off" && v != "false"
}

// budgetPolicy derives bounded budgets from device-pixel area. TANGO_GPU_BUDGET=off
// restores the pre-budget residency policy.
func budgetPolicy(size image.Point, scale float32) BudgetPolicy {
	if !gpuBudgetEnabled() {
		return BudgetPolicy{}
	}
	area := int64(size.X) * int64(size.Y)
	pages := 16
	if area >= 400000 {
		pages = 12
	}
	if area >= 2000000 {
		pages = 8
	}
	return BudgetPolicy{AtlasMaxPages: pages, AtlasIdleFrames: 120, ImageMaxBytes: 2 << 20, ImageIdleFrames: 120, LayerMaxBytes: 48 << 20}
}
