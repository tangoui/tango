package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"testing"
)

type cacheTrimRenderer struct{ trims int }

func (r *cacheTrimRenderer) Render(*paint.List, geom.Size, float32) error { return nil }
func (r *cacheTrimRenderer) Trim()                                        { r.trims++ }

func TestRunnerTrimCachesCallsOwnedRenderer(t *testing.T) {
	renderer := &cacheTrimRenderer{}
	r := NewRunner(func(*Context) {}, RunnerOptions{Renderer: renderer})
	r.TrimCaches()
	if renderer.trims != 1 {
		t.Fatalf("trims=%d", renderer.trims)
	}
	r.Close()
	r.TrimCaches()
	if renderer.trims != 1 {
		t.Fatal("closed runner trimmed resources")
	}
}
