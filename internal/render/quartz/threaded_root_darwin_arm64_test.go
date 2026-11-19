//go:build darwin && !ios && arm64

package quartz

import (
	"image/color"
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// On a window UI thread the hosted root's geometry belongs to the main
// thread (AppKit sizes it); frames resize only Quartz's own child layer.
func TestThreadedRendererLeavesRootGeometryToMain(t *testing.T) {
	if err := loadNative(); err != nil {
		t.Skipf("CoreGraphics unavailable: %v", err)
	}
	th := uithread.Start("quartz root")
	defer th.Stop()
	root := objc.ID(objc.GetClass("CALayer")).Send(sel("new"))
	defer root.Send(sel("release"))
	setNativeRect(root, sel("setBounds:"), nativeRect{Size: nativeSize{300, 200}})
	setNativeFloat(root, sel("setContentsScale:"), 1)
	th.Do(func() {
		pr, present, err := NewPresenter(uintptr(root))
		if err != nil {
			t.Fatal(err)
		}
		r := pr.(*renderer)
		defer r.Close()
		var l paint.List
		l.Background = color.NRGBA{R: 200, A: 255}
		if err := r.Render(&l, geom.Size{W: 150, H: 100}, 2); err != nil {
			t.Fatal(err)
		}
		if err := present(r.scene.bounds.Size(), 2); err != nil {
			t.Fatal(err)
		}
		if b := nativeBounds(root); b != (nativeSize{300, 200}) {
			t.Fatalf("UI frame resized the main-owned root to %v", b)
		}
		if r.layer == root || nativeBounds(r.layer) != (nativeSize{150, 100}) {
			t.Fatalf("frame geometry not on Quartz's child layer")
		}
		// Pinned to the root's top-left corner, not centred on its origin.
		if f := layerFrame(r.layer); f != (nativeRect{Size: nativeSize{150, 100}}) {
			t.Fatalf("child frame %v, want origin (0,0) size 150x100", f)
		}
	})
}

var getNativeRect func(objc.ID, objc.SEL) nativeRect

func layerFrame(layer objc.ID) nativeRect {
	nativeBounds(layer) // binds getNativeRect
	return getNativeRect(layer, sel("frame"))
}

func nativeBounds(layer objc.ID) nativeSize {
	if getNativeRect == nil {
		ob, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		send, err := purego.Dlsym(ob, "objc_msgSend")
		if err != nil {
			panic(err)
		}
		purego.RegisterFunc(&getNativeRect, send)
	}
	return getNativeRect(layer, sel("bounds")).Size
}
