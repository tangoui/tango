//go:build windows && (amd64 || arm64)

package windows

// App-owned native views embedded in a canvas surface
// (platform.NativeViewEmbedder). Like an embedded WebView
// (webembed_windows.go), each view gets a clip container: a hidden child
// window of the parent placed above the canvas with embedPlan, holding the
// application's HWND as its child. With PassInput the container and the
// application's window answer WM_NCHITTEST with HTTRANSPARENT, so the
// mouse reaches the canvas below; the application's window procedure is
// replaced for that and restored when the view is given back. A closing
// parent would destroy the application's window with it, so it is moved
// to the message-only parent first and left to the application.

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.NativeViewEmbedder = (*Backend)(nil)

const (
	nativeEmbedClass = "TangoNativeEmbed"
	gwlpWndProc      = ^uintptr(3)  // GWLP_WNDPROC (-4)
	gwlStyle         = ^uintptr(15) // GWL_STYLE (-16)
	hwndMessage      = ^uintptr(2)  // HWND_MESSAGE (-3)
	swShowNA         = 8
)

var (
	procSetParent       = user32.NewProc("SetParent")
	procCallWindowProcW = user32.NewProc("CallWindowProcW")

	nativeClassOnce sync.Once
	nativeClassErr  error
	nativePassProc  uintptr // window procedure of PassInput application windows

	// Main thread.
	nativesByParent = map[*surface][]*embeddedNative{}
	nativeByHwnd    = map[uintptr]*embeddedNative{} // container → view
	nativeOldProc   = map[uintptr]uintptr{}         // application window → its own procedure
)

// embeddedNative is an application's HWND placed in a canvas surface.
type embeddedNative struct {
	parent *surface
	hwnd   uintptr // the clip container
	view   uintptr // the application's window
	pass   bool
	plan   embedPlan
	closed bool
	onGone func() // the parent closed (platform.NativeViewEmbedder)
}

func registerNativeEmbedClass() error {
	nativeClassOnce.Do(func() {
		wc := wndClassEx{
			WndProc:   syscall.NewCallback(nativeEmbedWndProc),
			Instance:  instance(),
			ClassName: u16(nativeEmbedClass),
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			nativeClassErr = fmt.Errorf("tango: RegisterClassExW(%s): %v", nativeEmbedClass, err)
		}
		nativePassProc = syscall.NewCallback(nativePassWndProc)
	})
	return nativeClassErr
}

// nativeEmbedWndProc is the container's window procedure.
func nativeEmbedWndProc(hwnd, m, wp, lp uintptr) uintptr {
	switch uint32(m) {
	case embedWmEraseBkgnd:
		return 1
	case attachWmNCHitTest:
		if n := nativeByHwnd[hwnd]; n != nil && n.pass {
			return attachHTTransparent
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// nativePassWndProc replaces the procedure of a PassInput application
// window: hit tests fall through, everything else goes to its own.
func nativePassWndProc(hwnd, m, wp, lp uintptr) uintptr {
	if uint32(m) == attachWmNCHitTest {
		return attachHTTransparent
	}
	old := nativeOldProc[hwnd]
	if old == 0 {
		r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
		return r
	}
	r, _, _ := procCallWindowProcW.Call(old, hwnd, m, wp, lp)
	return r
}

// NewEmbeddedNativeView implements platform.NativeViewEmbedder. Main thread.
func (b *Backend) NewEmbeddedNativeView(parentID platform.SurfaceID, view uintptr, opts platform.NativeViewOptions, closed func()) (platform.EmbeddedNativeView, error) {
	if b.h == nil {
		return nil, errors.New("tango/windows: NewEmbeddedNativeView before Init")
	}
	parent := b.byID[parentID]
	if parent == nil || parent.closed {
		return nil, fmt.Errorf("tango/windows: NewEmbeddedNativeView: no open surface %d", parentID)
	}
	if view == 0 {
		return nil, errors.New("tango/windows: NewEmbeddedNativeView: no view")
	}
	if err := registerNativeEmbedClass(); err != nil {
		return nil, err
	}
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(nativeEmbedClass))), 0,
		embedWsChild|embedWsClipSiblings|wsClipChildren, // hidden until placed
		0, 0, 0, 0, parent.hwnd, 0, instance(), 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("tango/windows: NewEmbeddedNativeView: CreateWindowExW: %v", err)
	}
	n := &embeddedNative{parent: parent, hwnd: hwnd, view: view, pass: opts.PassInput, onGone: closed}
	nativeByHwnd[hwnd] = n
	style, _, _ := wxGetWindowLongPtrW.Call(view, gwlStyle)
	wxSetWindowLongPtrW.Call(view, gwlStyle, (style|embedWsChild|embedWsClipSiblings)&^wsPopup)
	procSetParent.Call(view, hwnd)
	if n.pass {
		old, _, _ := wxGetWindowLongPtrW.Call(view, gwlpWndProc)
		nativeOldProc[view] = old
		wxSetWindowLongPtrW.Call(view, gwlpWndProc, nativePassProc)
	}
	procShowWindow.Call(view, swShowNA) // shows with its container
	nativesByParent[parent] = append(nativesByParent[parent], n)
	parent.restackEmbedded()
	n.apply() // records the hidden state
	return n, nil
}

// SetPlacement implements platform.EmbeddedNativeView.
func (n *embeddedNative) SetPlacement(p platform.WebPlacement) {
	if n.closed {
		return
	}
	n.plan.place(p)
	n.apply()
}

// apply brings the container and the application's window to the
// placement at the parent's DPI; nothing is called when nothing changed.
func (n *embeddedNative) apply() {
	c, k := n.plan.sync(embedScale(dpiOf(n.parent.hwnd)), true)
	if k.Move {
		r := k.Rect
		procSetWindowPos.Call(n.view, 0, uintptr(int32(r.Min.X)), uintptr(int32(r.Min.Y)),
			uintptr(r.Dx()), uintptr(r.Dy()), swpNoZOrder|swpNoActivate)
	}
	if c.Move {
		r := c.Rect
		procSetWindowPos.Call(n.hwnd, 0, uintptr(int32(r.Min.X)), uintptr(int32(r.Min.Y)),
			uintptr(r.Dx()), uintptr(r.Dy()), swpNoZOrder|swpNoActivate)
	}
	if c.Show {
		procShowWindow.Call(n.hwnd, swShowNA)
	}
	if c.Hide {
		procShowWindow.Call(n.hwnd, swHide)
	}
}

// Close implements platform.EmbeddedNativeView.
func (n *embeddedNative) Close() {
	if n.closed {
		return
	}
	n.remove()
	list := nativesByParent[n.parent]
	for i, x := range list {
		if x == n {
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(nativesByParent, n.parent)
	} else {
		nativesByParent[n.parent] = list
	}
}

// remove gives the application's window back, hidden and parked under the
// message-only window, and destroys the container.
func (n *embeddedNative) remove() {
	n.closed = true
	if old, ok := nativeOldProc[n.view]; ok {
		wxSetWindowLongPtrW.Call(n.view, gwlpWndProc, old)
		delete(nativeOldProc, n.view)
	}
	procShowWindow.Call(n.view, swHide)
	procSetParent.Call(n.view, hwndMessage)
	delete(nativeByHwnd, n.hwnd)
	procDestroyWindow.Call(n.hwnd)
}

// destroyNatives gives back the native views of a closing parent before
// its child windows are destroyed and tells their owners (called from
// surface.destroyed, in the parent's WM_DESTROY).
func (s *surface) destroyNatives() {
	list := nativesByParent[s]
	delete(nativesByParent, s)
	for i := len(list) - 1; i >= 0; i-- {
		n := list[i]
		if n.closed {
			continue
		}
		n.remove()
		if n.onGone != nil {
			n.onGone()
		}
	}
}

// restackNatives puts the native views above the canvas and the embedded
// WebViews, in creation order (restackEmbedded).
func (s *surface) restackNatives() {
	for _, n := range nativesByParent[s] {
		if !n.closed {
			procSetWindowPos.Call(n.hwnd, attachHwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		}
	}
}
