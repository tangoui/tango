package gpu

import (
	"image"
	"testing"
)

func TestBudgetPolicyScalesLargeWindow(t *testing.T) {
	t.Setenv("TANGO_GPU_BUDGET", "on")
	small := budgetPolicy(image.Pt(360, 260), 2)
	large := budgetPolicy(image.Pt(800, 600), 2)
	if small.AtlasMaxPages <= large.AtlasMaxPages || large.AtlasMaxPages <= 0 || large.ImageMaxBytes <= 0 {
		t.Fatalf("small=%+v large=%+v", small, large)
	}
	if large.AtlasIdleFrames == 0 || large.ImageIdleFrames == 0 {
		t.Fatal("idle budgets missing")
	}
}
func TestBudgetPolicyOffRestoresUnbounded(t *testing.T) {
	t.Setenv("TANGO_GPU_BUDGET", "off")
	if got := budgetPolicy(image.Pt(800, 600), 2); got != (BudgetPolicy{}) {
		t.Fatalf("off policy=%+v", got)
	}
	for _, v := range []string{"0", "false", " OFF "} {
		t.Setenv("TANGO_GPU_BUDGET", v)
		if got := budgetPolicy(image.Pt(800, 600), 2); got != (BudgetPolicy{}) {
			t.Fatalf("%q policy=%+v", v, got)
		}
	}
}
func TestBudgetPolicyDoesNotSelectRenderer(t *testing.T) {
	t.Setenv("TANGO_GPU_BUDGET", "on")
	if got := budgetPolicy(image.Pt(800, 600), 2); got.AtlasMaxPages <= 0 {
		t.Fatal("policy is not available to renderer")
	}
}
