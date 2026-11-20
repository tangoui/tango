//go:build darwin

package metal

import (
	"image"
	"sync"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

func TestMetalLayerBudgetRestoresLegacyCap(t *testing.T) {
	d := newDevice(t)
	r := gpu.NewRenderer(d)
	l := &paint.List{}
	size := geom.Size{W: 8, H: 8}

	t.Setenv("TANGO_GPU_BUDGET", "on")
	if err := r.Render(l, size, 1); err != nil {
		t.Fatal(err)
	}
	if got := d.layerBudget(); got != 48<<20 {
		t.Fatalf("enabled layer cap = %d bytes, want 48 MiB", got)
	}

	t.Setenv("TANGO_GPU_BUDGET", "off")
	if err := r.Render(l, size, 1); err != nil {
		t.Fatal(err)
	}
	if got := d.layerBudget(); got != layerPoolBytes {
		t.Fatalf("disabled layer cap = %d bytes, want %d", got, layerPoolBytes)
	}
}

func TestMetalLayerBudgetTrimsIdleTextures(t *testing.T) {
	d := newDevice(t)
	const budget = 1 // smaller than any render target
	if err := d.SetLayerBudget(budget); err != nil {
		t.Fatal(err)
	}
	// Use the explicit budget instead of letting the renderer set it per frame.
	r := gpu.NewRenderer(&layerSizes{d: d})
	l := gputest.SceneNamed("opacitynest").Make(t)
	size := gputest.SceneSize
	const scale = 2
	if err := r.Render(l, size, scale); err != nil {
		t.Fatal(err)
	}
	if got := d.layerBytes(); got <= budget {
		t.Fatalf("layer pool bytes = %d, want more than %d for the current frame", got, budget)
	}
	ref := paint.NewCPU()
	ref.Render(l, size, scale)
	gputest.Check(t, "over budget", l, scale, ref.Image(), readPixels(t, d), 4)

	if err := r.Render(&paint.List{}, size, scale); err != nil {
		t.Fatal(err)
	}
	if got := d.layerBytes(); got > budget {
		t.Fatalf("idle layer pool bytes = %d, want at most %d", got, budget)
	}
}

func TestMetalLayerBudgetConcurrentExecute(t *testing.T) {
	d := newDevice(t)
	var setters sync.WaitGroup
	setters.Go(func() {
		for i := range 16 {
			if err := d.SetLayerBudget(int64(i) << 20); err != nil {
				t.Error(err)
			}
		}
	})
	defer setters.Wait()
	for range 16 {
		if err := d.Execute(&gpu.Frame{Size: image.Pt(8, 8)}); err != nil {
			t.Fatal(err)
		}
	}
}
