package d3d11

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/gen"
	"github.com/tangoui/tango/ui/gpu/shaders"
)

func TestMaxTextureSizeFor(t *testing.T) {
	for fl, want := range map[uint32]int{featureLevel10_0: 8192, featureLevel10_1: 8192, featureLevel11_0: 16384, featureLevel11_1: 16384} {
		if got := maxTextureSizeFor(fl); got != want {
			t.Errorf("maxTextureSizeFor(%#x) = %d, want %d", fl, got, want)
		}
	}
	if maxTexture2DDimension != 16384 {
		t.Errorf("D3D11_REQ_TEXTURE2D_U_OR_V_DIMENSION = %d", maxTexture2DDimension)
	}
}

func TestShaderProfiles(t *testing.T) {
	if vs, ps := shaderProfiles(featureLevel11_0); vs != "vs_5_0" || ps != "ps_5_0" {
		t.Errorf("11_0: %s %s", vs, ps)
	}
	if vs, ps := shaderProfiles(featureLevel10_1); vs != "vs_4_0" || ps != "ps_4_0" {
		t.Errorf("10_1: %s %s", vs, ps)
	}
}

func TestCheckTextureSizes(t *testing.T) {
	const lim = 16384
	ok := gpu.Frame{Size: image.Pt(lim, lim), MaskSize: image.Pt(lim, 4), ColorSize: image.Pt(4, lim), RampSize: image.Pt(256, 64)}
	if err := checkTextureSizes(&ok, lim); err != nil {
		t.Fatalf("frame at the limit: %v", err)
	}
	for name, f := range map[string]gpu.Frame{
		"target X": {Size: image.Pt(lim+1, 1)},
		"target Y": {Size: image.Pt(1, lim+1)},
		"mask":     {Size: image.Pt(8, 8), MaskSize: image.Pt(4, lim+1)},
		"color":    {Size: image.Pt(8, 8), ColorSize: image.Pt(lim+1, 4)},
		"ramp":     {Size: image.Pt(8, 8), RampSize: image.Pt(256, lim+1)},
		"image":    {Size: image.Pt(8, 8), ImageUploads: []gpu.ImageUpload{{ID: 1, Size: image.Pt(lim+1, 1)}}},
		"layer":    {Size: image.Pt(8, 8), Layers: []gpu.LayerDesc{{Size: image.Pt(1, lim+1)}}},
	} {
		err := checkTextureSizes(&f, lim)
		if !errors.Is(err, gpu.ErrTargetTooLarge) || errors.Is(err, gpu.ErrDeviceLost) {
			t.Errorf("%s: %v, want ErrTargetTooLarge", name, err)
		}
	}
	// A 10_x device's smaller limit.
	if err := checkTextureSizes(&gpu.Frame{Size: image.Pt(8193, 1)}, 8192); !errors.Is(err, gpu.ErrTargetTooLarge) {
		t.Errorf("8193 at limit 8192: %v", err)
	}
}

func TestCheckFrame(t *testing.T) {
	plain := gpu.Frame{Size: image.Pt(8, 8), Batches: []gpu.Batch{{Instances: make([]shaders.Instance, 2)}, {Instances: make([]shaders.Instance, 3)}}}
	if n, err := checkFrame(&plain); err != nil || n != 5 {
		t.Fatalf("plain frame: %d, %v", n, err)
	}
	partial := plain
	partial.Dirty = []image.Rectangle{image.Rect(0, 0, 1, 1), image.Rect(2, 2, 3, 3)}
	if n, err := checkFrame(&partial); err != nil || n != 7 {
		t.Fatalf("partial frame: %d, %v; want 5 instances and 2 clearing quads", n, err)
	}
	_, err := checkFrame(&gpu.Frame{Copies: []gpu.CopyRegion{{Src: image.Rect(0, 0, 1, 1), Dst: image.Rect(1, 1, 2, 2)}}})
	if err == nil || errors.Is(err, gpu.ErrTargetTooLarge) {
		t.Errorf("Copies without Dirty: %v, want an invalid-frame error", err)
	}
	limit := maxInstBuffer / gen.QuadInstanceStride
	huge := gpu.Frame{Batches: []gpu.Batch{{Instances: make([]shaders.Instance, limit)}}}
	if n, err := checkFrame(&huge); err != nil || n != limit {
		t.Errorf("frame at the instance limit: %d, %v", n, err)
	}
	huge.Dirty = []image.Rectangle{image.Rect(0, 0, 1, 1)} // one clearing quad more
	if _, err := checkFrame(&huge); !errors.Is(err, gpu.ErrTargetTooLarge) {
		t.Errorf("beyond the instance limit: %v, want ErrTargetTooLarge", err)
	}
}

func TestCheckUpload(t *testing.T) {
	size := image.Pt(16, 8)
	for _, c := range []struct {
		name string
		u    gpu.Upload
		bpp  int // 0: an error
	}{
		{"mask", gpu.Upload{Tex: gpu.TexMask, Rect: image.Rect(1, 1, 4, 3), Pix: make([]byte, 16+3), Stride: 16}, 1},
		{"color exact", gpu.Upload{Tex: gpu.TexColor, Rect: image.Rect(0, 0, 2, 2), Pix: make([]byte, 8+8), Stride: 8}, 4},
		{"color short", gpu.Upload{Tex: gpu.TexColor, Rect: image.Rect(0, 0, 2, 2), Pix: make([]byte, 8+7), Stride: 8}, 0},
		{"ramp", gpu.Upload{Tex: gpu.TexRamp, Rect: image.Rect(0, 7, 16, 8), Pix: make([]byte, 128), Stride: 0}, 8},
		{"whole", gpu.Upload{Tex: gpu.TexMask, Rect: image.Rectangle{Max: size}, Pix: make([]byte, 128), Stride: 16}, 1},
		{"outside", gpu.Upload{Tex: gpu.TexMask, Rect: image.Rect(15, 0, 17, 1), Pix: make([]byte, 2), Stride: 2}, 0},
		{"negative", gpu.Upload{Tex: gpu.TexMask, Rect: image.Rect(-1, 0, 1, 1), Pix: make([]byte, 2), Stride: 2}, 0},
		{"unknown", gpu.Upload{Tex: 7, Rect: image.Rect(0, 0, 1, 1), Pix: []byte{0}, Stride: 1}, 0},
		{"image slot", gpu.Upload{Tex: gpu.TexImage, Rect: image.Rect(0, 0, 1, 1), Pix: make([]byte, 4), Stride: 4}, 0},
		{"empty", gpu.Upload{Tex: gpu.TexColor, Rect: image.Rect(3, 3, 3, 5)}, 4},
		{"negative stride", gpu.Upload{Tex: gpu.TexMask, Rect: image.Rect(0, 0, 2, 3), Pix: make([]byte, 64), Stride: -4}, 0},
	} {
		bpp, err := checkUpload(c.u, size)
		if c.bpp == 0 {
			if err == nil {
				t.Errorf("%s: no error", c.name)
			}
			continue
		}
		if err != nil || bpp != c.bpp {
			t.Errorf("%s: bpp %d, %v; want %d", c.name, bpp, err, c.bpp)
		}
	}
	// A zero-sized texture (the frame neither uploads nor samples it)
	// accepts only empty uploads.
	if _, err := checkUpload(gpu.Upload{Tex: gpu.TexRamp, Rect: image.Rect(0, 0, 1, 1), Pix: make([]byte, 8), Stride: 8}, image.Point{}); err == nil {
		t.Error("upload into a zero-sized texture: no error")
	}
}

func TestUploadBox(t *testing.T) {
	if got, want := uploadBox(image.Rect(3, 5, 10, 6)), (box{Left: 3, Top: 5, Front: 0, Right: 10, Bottom: 6, Back: 1}); got != want {
		t.Errorf("uploadBox = %+v, want %+v", got, want)
	}
}

func TestScissorFor(t *testing.T) {
	full := image.Rect(0, 0, 100, 50)
	for _, c := range []struct {
		s      image.Rectangle
		origin image.Point
		want   rect
		ok     bool
	}{
		{image.Rectangle{}, image.Point{}, rect{0, 0, 100, 50}, true},
		{image.Rect(10, 5, 20, 15), image.Point{}, rect{10, 5, 20, 15}, true},
		{image.Rect(-10, -5, 120, 70), image.Point{}, rect{0, 0, 100, 50}, true},
		{image.Rect(90, 40, 130, 90), image.Point{}, rect{90, 40, 100, 50}, true},
		{image.Rect(100, 0, 120, 10), image.Point{}, rect{}, false},
		{image.Rect(-20, 0, 0, 10), image.Point{}, rect{}, false},
		// A layer at (30, 20): the scissor moves by -origin.
		{image.Rect(35, 25, 45, 30), image.Pt(30, 20), rect{5, 5, 15, 10}, true},
		{image.Rect(0, 0, 32, 22), image.Pt(30, 20), rect{0, 0, 2, 2}, true},
	} {
		got, ok := scissorFor(c.s, c.origin, full)
		if got != c.want || ok != c.ok {
			t.Errorf("scissorFor(%v, %v) = %+v, %v; want %+v, %v", c.s, c.origin, got, ok, c.want, c.ok)
		}
	}
}

func TestClearColor(t *testing.T) {
	if got := clearColor(color.NRGBA{255, 255, 255, 255}); got != [4]float32{1, 1, 1, 1} {
		t.Errorf("white: %v", got)
	}
	// Half-transparent red clears to the premultiplied bytes paint.CPU
	// fills with.
	c := clearColor(color.NRGBA{255, 0, 0, 128})
	p := color.RGBAModel.Convert(color.NRGBA{255, 0, 0, 128}).(color.RGBA)
	for i, b := range [4]uint8{p.R, p.G, p.B, p.A} {
		if got := uint8(c[i]*255 + 0.5); got != b {
			t.Errorf("channel %d: %v → %d, want %d", i, c[i], got, b)
		}
	}
}

func TestInputFormat(t *testing.T) {
	for _, c := range []struct {
		kind string
		n    int
		want uint32
	}{
		{"f32", 4, 2}, {"f32", 3, 6}, {"f32", 2, 16}, {"f32", 1, 41},
		{"u32", 4, 3}, {"u32", 3, 7}, {"u32", 2, 17}, {"u32", 1, 42},
		{"i32", 4, 4}, {"i32", 3, 8}, {"i32", 2, 18}, {"i32", 1, 43},
	} {
		if got, err := inputFormat(c.kind, c.n); err != nil || got != c.want {
			t.Errorf("inputFormat(%s, %d) = %d, %v; want DXGI_FORMAT %d", c.kind, c.n, got, err, c.want)
		}
	}
	for _, c := range []struct {
		kind string
		n    int
	}{{"f64", 1}, {"f32", 0}, {"f32", 5}} {
		if _, err := inputFormat(c.kind, c.n); err == nil {
			t.Errorf("inputFormat(%s, %d): no error", c.kind, c.n)
		}
	}
	// Every generated attribute has a format and fits the instance.
	for _, at := range gen.QuadInstanceAttrs {
		if _, err := inputFormat(at.Kind, at.N); err != nil {
			t.Errorf("attribute %d: %v", at.Location, err)
		}
		if at.Offset+4*at.N > gen.QuadInstanceStride {
			t.Errorf("attribute %d overflows the instance", at.Location)
		}
	}
}

func TestSamplerFilter(t *testing.T) {
	if f, err := samplerFilter("nearest"); err != nil || f != filterMinMagMipPoint {
		t.Errorf("nearest: %#x, %v", f, err)
	}
	if f, err := samplerFilter("linear"); err != nil || f != filterMinMagMipLinear {
		t.Errorf("linear: %#x, %v", f, err)
	}
	if _, err := samplerFilter("cubic"); err == nil {
		t.Error("cubic: no error")
	}
	if len(gen.QuadTextures) != 5 {
		t.Errorf("%d quad textures, want 5 (mask, color, ramp, image, layer)", len(gen.QuadTextures))
	}
	for i, td := range gen.QuadTextures {
		if td.Binding != i {
			t.Errorf("texture %d has binding %d", i, td.Binding)
		}
		if _, err := samplerFilter(td.Filter); err != nil {
			t.Errorf("texture %d: %v", i, err)
		}
	}
	if gen.QuadTextures[gpu.TexRamp].Filter != "linear" {
		t.Error("the ramp texture is not sampled linearly")
	}
}

func TestInstRing(t *testing.T) {
	const s = gen.QuadInstanceStride
	var r instRing
	steps := []struct {
		n    int
		want ringAlloc
	}{
		{10 * s, ringAlloc{off: 0, discard: true, grow: minInstBuffer}}, // first frame: create, discard
		{10 * s, ringAlloc{off: 10 * s}},                                // after it, no overwrite
		{200 * s, ringAlloc{off: 20 * s}},
		{300 * s, ringAlloc{off: 0, discard: true}}, // 220+300 > 455 instances: wrap
		{455 * s, ringAlloc{off: 0, discard: true}}, // exactly the buffer, from 0
		{456 * s, ringAlloc{off: 0, discard: true, grow: 2 * minInstBuffer}},
		{s, ringAlloc{off: 456 * s}},
		{2*minInstBuffer - 457*s, ringAlloc{off: 457 * s}}, // fills it exactly
		{1, ringAlloc{off: 0, discard: true}},
	}
	for i, st := range steps {
		got, err := r.alloc(st.n)
		if err != nil || got != st.want {
			t.Fatalf("step %d alloc(%d) = %+v, %v; want %+v", i, st.n, got, err, st.want)
		}
		if r.pos > r.cap {
			t.Fatalf("step %d: pos %d beyond cap %d", i, r.pos, r.cap)
		}
	}
	// A frame larger than twice the buffer gets a buffer of its size.
	big := 5*minInstBuffer + 7
	if got, _ := r.alloc(big); got != (ringAlloc{off: 0, discard: true, grow: big}) {
		t.Errorf("alloc(%d) = %+v", big, got)
	}
	// Growth stops at the limit; beyond it is an error.
	if got, _ := r.alloc(maxInstBuffer); got.grow != maxInstBuffer || !got.discard {
		t.Errorf("alloc(max) = %+v", got)
	}
	if _, err := r.alloc(maxInstBuffer + 1); err == nil {
		t.Error("alloc beyond the limit: no error")
	}
	if _, err := r.alloc(0); err == nil {
		t.Error("alloc(0): no error")
	}
	// After reset (buffer released or a failed map) the next frame
	// creates a buffer again.
	r.reset()
	if got, _ := r.alloc(s); got != (ringAlloc{off: 0, discard: true, grow: minInstBuffer}) {
		t.Errorf("after reset: %+v", got)
	}
}

func TestHealth(t *testing.T) {
	plain := errors.New("CreateTexture2D: out of memory")
	var h health
	for i := range maxFailures - 1 {
		if err := h.fail(plain); err != plain || h.lost != nil {
			t.Fatalf("failure %d: %v, lost %v", i+1, err, h.lost)
		}
	}
	h.succeeded()
	if h.failures != 0 {
		t.Fatalf("a successful frame left %d failures", h.failures)
	}
	for i := range maxFailures - 1 {
		h.fail(plain)
		if h.lost != nil {
			t.Fatalf("lost after %d failures following a success", i+1)
		}
	}
	err := h.fail(plain)
	if !errors.Is(err, gpu.ErrDeviceLost) || !errors.Is(err, plain) || !errors.Is(err, ErrDeviceLost) {
		t.Fatalf("failure %d: %v, want ErrDeviceLost wrapping the cause", maxFailures, err)
	}
	h.succeeded() // lost is permanent
	if h.lost == nil || !errors.Is(h.fail(plain), gpu.ErrDeviceLost) {
		t.Fatal("the device came back after being lost")
	}

	// One device-lost HRESULT is enough.
	for _, hr := range []uint32{dxgiErrorDeviceRemoved, dxgiErrorDeviceReset, dxgiErrorDeviceHung, dxgiErrorDriverInternalError} {
		var h health
		cause := fmt.Errorf("frame: %w", &hresultError{Op: "Map", HR: hr})
		if err := h.fail(cause); !errors.Is(err, gpu.ErrDeviceLost) || !errors.Is(err, cause) {
			t.Errorf("%s: %v, want ErrDeviceLost", hrName(hr), err)
		}
	}
	// Other HRESULTs count like any failure.
	var h2 health
	if err := h2.fail(&hresultError{Op: "Map", HR: eOutOfMemory}); errors.Is(err, gpu.ErrDeviceLost) {
		t.Errorf("E_OUTOFMEMORY lost the device: %v", err)
	}
	// lose loses the device whatever the cause (a removed reason).
	var h3 health
	if err := h3.lose(&hresultError{Op: "GetDeviceRemovedReason", HR: dxgiErrorInvalidCall}); !errors.Is(err, gpu.ErrDeviceLost) {
		t.Errorf("lose: %v", err)
	}
}

func TestHRESULTNames(t *testing.T) {
	if hrName(eFail) != "E_FAIL" || eFail != 0x80004005 {
		t.Errorf("E_FAIL: %#x %q", uint32(eFail), hrName(eFail))
	}
	e := hrError("Map", eFail)
	if e == nil || !strings.Contains(e.Error(), "E_FAIL") || isDeviceLost(e) {
		t.Errorf("hrError(E_FAIL) = %v", e)
	}
	if hrError("Map", sOK) != nil || hrError("Present", dxgiStatusOccluded) != nil {
		t.Error("hrError of a success code")
	}
	if !isDeviceLost(fmt.Errorf("x: %w", hrError("Map", dxgiErrorDeviceRemoved))) {
		t.Error("isDeviceLost misses a wrapped DXGI_ERROR_DEVICE_REMOVED")
	}
}

func TestCompilerLog(t *testing.T) {
	for in, want := range map[string]string{
		"":     "",
		"\x00": "",
		"tango_quad.hlsl(3,1): warning X4000: use of potentially uninitialized variable\n\x00": "tango_quad.hlsl(3,1): warning X4000: use of potentially uninitialized variable",
		"  a\r\n\x00\x00": "a",
	} {
		if got := compilerLog([]byte(in)); got != want {
			t.Errorf("compilerLog(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestHealthPresentFailures: present failures have their own counter,
// which successful frames do not reset (the platform presents only after
// a successful Render, so a shared counter would never reach the limit);
// only a successful present resets it.
func TestHealthPresentFailures(t *testing.T) {
	cause := &hresultError{Op: "ResizeBuffers", HR: dxgiErrorInvalidCall}
	var h health
	for i := range maxFailures - 1 {
		h.succeeded() // the frame before each present executed fine
		if err := h.presentFailed(cause); errors.Is(err, gpu.ErrDeviceLost) {
			t.Fatalf("present failure %d lost the device", i+1)
		}
	}
	h.succeeded()
	err := h.presentFailed(cause)
	if !errors.Is(err, gpu.ErrDeviceLost) || !errors.Is(err, cause) {
		t.Fatalf("present failure %d with frames in between: %v, want ErrDeviceLost", maxFailures, err)
	}

	var h2 health
	h2.presentFailed(cause)
	h2.presentFailed(cause)
	h2.presentSucceeded()
	if err := h2.presentFailed(cause); errors.Is(err, gpu.ErrDeviceLost) {
		t.Fatal("a successful present did not reset the count")
	}
	// Frame and present failures count apart.
	var h3 health
	h3.fail(errors.New("frame"))
	h3.fail(errors.New("frame"))
	if err := h3.presentFailed(cause); errors.Is(err, gpu.ErrDeviceLost) {
		t.Fatal("frame failures counted toward the present limit")
	}
	// A device-lost HRESULT loses at once.
	var h4 health
	if err := h4.presentFailed(&hresultError{Op: "Present1", HR: dxgiErrorDeviceRemoved}); !errors.Is(err, gpu.ErrDeviceLost) {
		t.Fatalf("device removed: %v", err)
	}
}
