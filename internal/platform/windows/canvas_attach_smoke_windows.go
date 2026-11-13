//go:build windows && (amd64 || arm64)

package windows

// Test hooks for internal/platform/windows/internal/smokeattach: inspect
// a canvas attached to a WebView surface (canvas_attach_windows.go) and
// drive it with real (SendInput) mouse input. For the internal smoke test
// only.

import (
	"errors"
	"image/color"
	"math"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	attachSmokeGetWindow       = user32.export("GetWindow")
	attachSmokeWindowFromPoint = user32.export("WindowFromPoint")
	attachSmokeGetParent       = user32.export("GetParent")
	attachSmokeSetCursorPos    = user32.export("SetCursorPos")
	attachSmokeSendInput       = user32.export("SendInput")
	attachSmokeGetDC           = user32.export("GetDC")
	attachSmokeReleaseDC       = user32.export("ReleaseDC")
	attachSmokeGetPixel        = attachGDI32.export("GetPixel")
	attachSmokeGetWindowRgnBox = user32.export("GetWindowRgnBox")
)

const (
	attachGwChild    = 5
	attachGwHwndNext = 2
)

// attachMouseInput is INPUT with a MOUSEINPUT (40 bytes on 64-bit).
type attachMouseInput struct {
	Type      uint32
	_         uint32
	Dx, Dy    int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	Extra     uintptr
}

var _ = [1]struct{}{}[unsafe.Sizeof(attachMouseInput{})-40]

// SmokeAttachState describes an attached canvas.
type SmokeAttachState struct {
	Frame          [4]float64 // x, y, w, h in logical pixels relative to the client area
	InParent       bool       // the window exists as a child of the surface window
	AboveWeb       bool       // stacked above the WebView2 window
	WebFound       bool       // the WebView2 window was found among the children
	Layered        bool       // drawn with UpdateLayeredWindow
	Region         bool       // has a window region (pass-through)
	Focused        bool       // has keyboard focus
	WebTransparent bool       // the WebView's default background was made transparent
}

func smokeAttached(cv platform.Canvas) (*attached, error) {
	a, ok := cv.(*attached)
	if !ok {
		return nil, errors.New("not a windows attached canvas")
	}
	return a, nil
}

// SmokeAttachState reports where cv sits. UI thread.
func (b *Backend) SmokeAttachState(cv platform.Canvas) (st SmokeAttachState, err error) {
	a, err := smokeAttached(cv)
	if err != nil {
		return st, err
	}
	s := a.s
	st.WebTransparent = attachBGSet[s]
	if a.closed || s.closed {
		return st, nil
	}
	scale := float64(dpiOf(s.hwnd)) / baseDPI
	idx, web, me := 0, -1, -1
	for h, _, _ := attachSmokeGetWindow.Call(s.hwnd, attachGwChild); h != 0; h, _, _ = attachSmokeGetWindow.Call(h, attachGwHwndNext) {
		switch {
		case h == a.hwnd:
			me = idx
		case canvasByHwnd[h] == nil && web < 0:
			web = idx // the first non-canvas child: WebView2's window
		}
		idx++
	}
	st.InParent = me >= 0
	st.WebFound = web >= 0
	st.AboveWeb = me >= 0 && web >= 0 && me < web // GW_HWNDNEXT walks top to bottom
	var wr rect
	procGetWindowRect.Call(a.hwnd, uintptr(unsafe.Pointer(&wr)))
	p := point{wr.Left, wr.Top}
	canvasScreenToClient.Call(s.hwnd, uintptr(unsafe.Pointer(&p)))
	st.Frame = [4]float64{
		math.Round(float64(p.X) / scale), math.Round(float64(p.Y) / scale),
		math.Round(float64(wr.Right-wr.Left) / scale), math.Round(float64(wr.Bottom-wr.Top) / scale),
	}
	st.Layered = a.layered
	var box rect
	r, _, _ := attachSmokeGetWindowRgnBox.Call(a.hwnd, uintptr(unsafe.Pointer(&box)))
	st.Region = r != 0 // ERROR (0) when there is no region
	f, _, _ := canvasGetFocus.Call()
	st.Focused = f == a.hwnd
	return st, nil
}

// screenPoint converts logical content point (x, y) of s to screen pixels.
func (s *surface) screenPoint(x, y float64) point {
	scale := float64(dpiOf(s.hwnd)) / baseDPI
	p := point{int32(math.Round(x * scale)), int32(math.Round(y * scale))}
	canvasClientToScreen.Call(s.hwnd, uintptr(unsafe.Pointer(&p)))
	return p
}

// SmokeAttachHit reports what WindowFromPoint finds at content point
// (x, y): "canvas" (cv), "web" (another child of the surface, i.e.
// WebView2's windows), "other-canvas", "surface" or "other". The surface
// must be in front. UI thread.
func (b *Backend) SmokeAttachHit(cv platform.Canvas, x, y float64) (string, error) {
	a, err := smokeAttached(cv)
	if err != nil {
		return "", err
	}
	p := a.s.screenPoint(x, y)
	h, _, _ := attachSmokeWindowFromPoint.Call(uintptr(uint32(p.X)) | uintptr(uint32(p.Y))<<32)
	switch {
	case h == 0:
		return "other", nil
	case h == a.hwnd:
		return "canvas", nil
	case h == a.s.hwnd:
		return "surface", nil
	case canvasByHwnd[h] != nil:
		return "other-canvas", nil
	}
	for w := h; w != 0; w, _, _ = attachSmokeGetParent.Call(w) {
		if w == a.s.hwnd {
			return "web", nil
		}
	}
	return "other", nil
}

// SmokeAttachMove moves the real mouse pointer to content point (x, y).
func (b *Backend) SmokeAttachMove(cv platform.Canvas, x, y float64) error {
	a, err := smokeAttached(cv)
	if err != nil {
		return err
	}
	p := a.s.screenPoint(x, y)
	if r, _, err := attachSmokeSetCursorPos.Call(uintptr(p.X), uintptr(p.Y)); r == 0 {
		return err
	}
	return nil
}

// SmokeAttachClick moves the pointer to content point (x, y) and sends a
// left button press and release with SendInput, so the system routes it
// by hit testing as a real click.
func (b *Backend) SmokeAttachClick(cv platform.Canvas, x, y float64) error {
	if err := b.SmokeAttachMove(cv, x, y); err != nil {
		return err
	}
	in := [2]attachMouseInput{{Flags: 0x0002}, {Flags: 0x0004}} // INPUT_MOUSE; LEFTDOWN, LEFTUP
	if n, _, err := attachSmokeSendInput.Call(2, uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0])); n != 2 {
		return err
	}
	return nil
}

// SmokeAttachPixel reads the screen at content point (x, y) of cv's
// surface (what DWM composed, so the surface must be in front).
func (b *Backend) SmokeAttachPixel(cv platform.Canvas, x, y float64) (color.RGBA, error) {
	a, err := smokeAttached(cv)
	if err != nil {
		return color.RGBA{}, err
	}
	p := a.s.screenPoint(x, y)
	dc, _, _ := attachSmokeGetDC.Call(0)
	if dc == 0 {
		return color.RGBA{}, errors.New("GetDC failed")
	}
	defer attachSmokeReleaseDC.Call(0, dc)
	c, _, _ := attachSmokeGetPixel.Call(dc, uintptr(p.X), uintptr(p.Y))
	if c == 0xFFFFFFFF {
		return color.RGBA{}, errors.New("GetPixel: CLR_INVALID")
	}
	return color.RGBA{uint8(c), uint8(c >> 8), uint8(c >> 16), 255}, nil
}
