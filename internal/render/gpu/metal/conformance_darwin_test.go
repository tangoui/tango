//go:build darwin

package metal

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/gpuconf"
)

// TestMetalConformance runs the device conformance suite shared by every
// GPU backend (parity scenes against paint.CPU, upload order, layers,
// images, gradient ramps, partial frames) on a fresh Metal device per
// subtest.
func TestMetalConformance(t *testing.T) {
	gpuconf.Conformance(t, openConformance)
}

func openConformance(t *testing.T) gpu.Device {
	d, err := NewDevice()
	if errors.Is(err, ErrUnsupported) || errors.Is(err, ErrNoDevice) {
		t.Skipf("no Metal device: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// noImages forwards to a Metal device and fails any frame that uses an
// image texture.
type noImages struct {
	*Device
	t      *testing.T
	frames int
}

func (n *noImages) Execute(f *gpu.Frame) error {
	n.frames++
	used := len(f.ImageUploads) > 0
	for _, b := range f.Batches {
		used = used || b.Image != 0
	}
	if used {
		n.t.Errorf("a frame with images ran: %d image uploads", len(f.ImageUploads))
	}
	return n.Device.Execute(f)
}

// TestMetalConformanceSkip: gpuconf.Skip skips single scene subtests (as
// a backend without image support yet skips the image scenes; go test -v
// lists them as SKIP): with every group but Scenes skipped and the scenes
// drawing images skipped too, the other scenes run and no frame uses an
// image texture.
func TestMetalConformanceSkip(t *testing.T) {
	imageScenes := []string{"image", "images", "xform", "xformrot"}
	var dev *noImages
	var skipped []string
	gpuconf.Conformance(t, func(t *testing.T) gpu.Device {
		dev = &noImages{Device: openConformance(t).(*Device), t: t}
		return dev
	}, gpuconf.Skip(func(name string) bool {
		group, scene, _ := strings.Cut(name, "/")
		scene, _, _ = strings.Cut(scene, "@")
		skip := group != "Scenes" || slices.Contains(imageScenes, scene)
		if skip {
			skipped = append(skipped, name)
		}
		return skip
	}))
	if dev == nil {
		t.Skip("no Metal device: every subtest skipped")
	}
	if dev.frames == 0 {
		t.Fatal("no scene ran")
	}
	if n := len(skipped); n != 13+3*len(imageScenes) {
		t.Errorf("%d subtests skipped, want 13 groups and %d image scenes: %v", n, 3*len(imageScenes), skipped)
	}
}
