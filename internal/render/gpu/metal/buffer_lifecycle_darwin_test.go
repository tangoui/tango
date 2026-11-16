//go:build darwin

package metal

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

func uploadBufferFrame() gpu.Frame {
	pix := bytes.Repeat([]byte{255}, 1024*1024)
	inst := shaders.Instance{Rect: sl.V4(0, 0, 16, 16), Color: sl.V4(1, 0, 0, 1), UV: sl.V4(0, 0, 1, 1), Kind: sl.V4(shaders.KindMask, .5/1024, .5/1024, 0)}
	instances := make([]shaders.Instance, 8192)
	instances[0] = inst
	return gpu.Frame{Size: image.Pt(16, 16), MaskSize: image.Pt(1024, 1024), Uploads: []gpu.Upload{{Tex: gpu.TexMask, Rect: image.Rect(0, 0, 1024, 1024), Pix: pix, Stride: 1024}}, Batches: []gpu.Batch{{Instances: instances}}}
}

// Removing buffer retirement leaves native allocations at the transient peak.
// The second large upload must rebuild both buffers and reproduce the pixels.
func TestMetalUploadBuffersRetireAndRebuild(t *testing.T) {
	d := newDevice(t)
	large := uploadBufferFrame()
	execute := func(f *gpu.Frame) {
		t.Helper()
		if err := d.Execute(f); err != nil {
			t.Fatal(err)
		}
	}
	execute(&large)
	want := readPixels(t, d)
	if got := want.RGBAAt(8, 8); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("large upload pixel: %v", got)
	}
	peak := d.staging.Send(sel("allocatedSize")) + d.instBuf.Send(sel("allocatedSize"))
	small := gpu.Frame{Size: large.Size, MaskSize: large.MaskSize}
	for range 121 {
		execute(&small)
	}
	if d.staging != 0 || d.instBuf != 0 || d.stagingCap != 0 || d.instCap != 0 {
		t.Fatalf("unused upload buffers retained: staging=%d instance=%d peak allocated=%d", d.stagingCap, d.instCap, peak)
	}
	execute(&large)
	if got := readPixels(t, d); !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("pixels changed after buffer rebuild")
	}
	t.Logf("transient allocatedSize=%d retired=0 rebuilt=%d", peak, d.staging.Send(sel("allocatedSize"))+d.instBuf.Send(sel("allocatedSize")))
}

func TestMetalUploadBuffersShrinkAfterSustainedLowUse(t *testing.T) {
	d := newDevice(t)
	large := uploadBufferFrame()
	execute := func(f *gpu.Frame) {
		t.Helper()
		if err := d.Execute(f); err != nil {
			t.Fatal(err)
		}
	}
	execute(&large)
	staging, instances := d.staging, d.instBuf
	small := large
	small.Uploads = []gpu.Upload{{Tex: gpu.TexMask, Rect: image.Rect(0, 0, 16, 16), Pix: bytes.Repeat([]byte{127}, 256), Stride: 16}}
	smallInstance := large.Batches[0].Instances[0]
	smallInstance.UV = sl.V4(0, 0, 16.0/1024, 16.0/1024)
	small.Batches = []gpu.Batch{{Instances: []shaders.Instance{smallInstance}}}
	// A recurring spike before the grace period must keep both allocations.
	for range 2 {
		for range 100 {
			execute(&small)
		}
		if d.staging != staging || d.instBuf != instances {
			t.Fatal("buffers shrank during recurring workload")
		}
		execute(&large)
	}
	// Invalid and empty frames cannot age the successful-frame grace period.
	invalid := small
	invalid.Uploads = []gpu.Upload{{Tex: gpu.TexMask, Rect: image.Rect(0, 0, 16, 16)}}
	for range 130 {
		if err := d.Execute(&invalid); err == nil {
			t.Fatal("invalid upload accepted")
		}
		execute(&gpu.Frame{})
	}
	if d.staging != staging || d.instBuf != instances {
		t.Fatal("failed/empty frames aged buffers")
	}
	for range 120 {
		execute(&small)
	}
	// Even an expired grace period must not retire resources on rejection,
	// an empty frame, or a synchronization-only Wait.
	if err := d.Execute(&invalid); err == nil {
		t.Fatal("invalid upload accepted")
	}
	execute(&gpu.Frame{})
	if err := d.Wait(); err != nil {
		t.Fatal(err)
	}
	if d.staging != staging || d.instBuf != instances {
		t.Fatal("rejected frame or Wait retired buffers")
	}
	execute(&small)
	if d.stagingCap > 4096 || d.instCap > 4096 {
		t.Fatalf("low-use buffers retained: staging=%d instance=%d", d.stagingCap, d.instCap)
	}
	if got := readPixels(t, d).RGBAAt(8, 8); got != (color.RGBA{R: 127, A: 127}) {
		t.Fatalf("shrunken buffer pixel: %v", got)
	}
	t.Logf("low-use allocatedSize=%d", d.staging.Send(sel("allocatedSize"))+d.instBuf.Send(sel("allocatedSize")))
}

// Optional replacement failure must retain the already-packed bytes and
// native allocation; the next frame remains able to retry retirement.
func TestMetalUploadBufferShrinkAllocationFailure(t *testing.T) {
	d := newDevice(t)
	f := uploadBufferFrame()
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	if err := d.Wait(); err != nil {
		t.Fatal(err)
	}
	before, capacity := d.staging, d.stagingCap
	usage := uploadBufferUsage{frames: 120, peak: 256}
	usage.trim(0, &d.staging, &d.stagingCap, 256)
	if d.staging != before || d.stagingCap != capacity || usage.frames != 120 {
		t.Fatal("optional replacement failure damaged usable buffer or retry state")
	}
	if got := readPixels(t, d).RGBAAt(8, 8); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("allocation failure damaged pixels: %v", got)
	}
}
