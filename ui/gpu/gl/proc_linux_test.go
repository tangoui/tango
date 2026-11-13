//go:build linux

package gl

import (
	"os"
	"testing"

	"github.com/ebitengine/purego"
)

// TestEpoxyStub checks what glProc relies on: the value of epoxy_glFoo is
// a stub before the first call, which the first call rewrites to the real
// function, and the stub keeps forwarding afterwards.
func TestEpoxyStub(t *testing.T) {
	ctx := newTestContext(t)
	proc, err := glProc()
	if err != nil {
		t.Fatal(err)
	}
	const name = "glGetStringi" // not called while creating the context
	var (
		before, after uintptr
		nExt          int32
		first         string
		viaStub       [3]string
		viaResolved   [3]string
		glErr         uint32
	)
	// Only GL calls inside do; the assertions follow outside it.
	ctx.thread.do(func() {
		f := ctx.funcs
		before = proc(name)
		f.GetIntegerv(glNUM_EXTENSIONS, &nExt)
		if nExt == 0 {
			return
		}
		first = f.GetStringi(glEXTENSIONS, 0) // bound to the value read at load: the stub
		after = proc(name)
		var direct func(name, index uint32) string
		purego.RegisterFunc(&direct, after)
		for i := range viaStub {
			viaStub[i] = f.GetStringi(glEXTENSIONS, 0)
			viaResolved[i] = direct(glEXTENSIONS, 0)
		}
		glErr = f.GetError()
	})
	if nExt == 0 {
		t.Fatal("no extensions")
	}
	t.Logf("epoxy_%s: %#x before the first call, %#x after", name, before, after)
	if before == after {
		t.Log("libepoxy resolved the function before its first call")
	}
	for i := range viaStub {
		if viaStub[i] != first || first == "" {
			t.Fatalf("call %d through the stub: %q, want %q", i+2, viaStub[i], first)
		}
		if viaResolved[i] != first {
			t.Fatalf("call through the resolved pointer: %q, want %q", viaResolved[i], first)
		}
	}
	if glErr != glNO_ERROR {
		t.Fatalf("glGetError %#x", glErr)
	}
}

// TestEGLDisplays creates a context on each EGL path: Mesa's surfaceless
// platform (the default here) and the default display with a 1×1 pbuffer
// (which needs a window system: run under xvfb-run).
func TestEGLDisplays(t *testing.T) {
	ctx := newTestContext(t)
	if ctx.surface != 0 {
		t.Log("the surfaceless platform is unavailable; the context uses a pbuffer")
	}
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no DISPLAY or WAYLAND_DISPLAY for the default EGL display")
	}
	skipSurfaceless = true
	pb, err := newHeadlessContext()
	skipSurfaceless = false
	if err != nil {
		t.Fatalf("default display with a pbuffer: %v", err)
	}
	defer pb.close()
	if pb.surface == 0 {
		t.Fatal("default display: no pbuffer")
	}
	var maxTex int32
	pb.thread.do(func() { pb.funcs.GetIntegerv(glMAX_TEXTURE_SIZE, &maxTex) })
	if maxTex < 4096 {
		t.Fatalf("pbuffer context: GL_MAX_TEXTURE_SIZE %d", maxTex)
	}
}
