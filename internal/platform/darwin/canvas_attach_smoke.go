//go:build darwin && !ios

package darwin

// Test hooks for internal/platform/darwin/internal/smokeattach: inspect
// and poke a canvas attached to a WebView surface (canvas_attach.go)
// without a real mouse. For the internal smoke test only.

import (
	"errors"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// SmokeAttachState describes an attached canvas.
type SmokeAttachState struct {
	Frame           [4]float64 // x, y, w, h relative to the content area's top-left
	InContainer     bool       // the view's superview is the WebView's container
	AboveWeb        bool       // stacked above the WebView
	FirstResponder  bool       // the view is its window's first responder
	DrawsBackground bool       // the WebView draws its own background
}

func smokeOverlay(cv platform.Canvas) (*overlay, error) {
	o, ok := cv.(*overlay)
	if !ok {
		return nil, errors.New("not a darwin attached canvas")
	}
	return o, nil
}

// SmokeAttachState reports where cv sits. Main thread.
func (b *backend) SmokeAttachState(cv platform.Canvas) (st SmokeAttachState, err error) {
	o, err := smokeOverlay(cv)
	if err != nil {
		return st, err
	}
	withPool(func() {
		s := o.s
		if !s.closed {
			st.DrawsBackground = sendBool(send(s.web, "valueForKey:", uintptr(nsString("drawsBackground"))), "boolValue")
		}
		if o.closed {
			return
		}
		container := send(s.web, "superview")
		sup := send(o.view, "superview")
		st.InContainer = sup != 0 && sup == container
		st.FirstResponder = send(s.win, "firstResponder") == o.view
		if !st.InContainer {
			return
		}
		cb := msgRect(container, sel("bounds"))
		f := msgRect(o.view, sel("frame"))
		st.Frame = [4]float64{f.Origin.X, cb.Size.Height - f.Origin.Y - f.Size.Height, f.Size.Width, f.Size.Height}
		subs := send(container, "subviews")
		n := sendInt(subs, "count")
		iv, iw := -1, -1
		for i := range n {
			v := send(subs, "objectAtIndex:", uintptr(i))
			switch v {
			case o.view:
				iv = i
			case s.web:
				iw = i
			}
		}
		st.AboveWeb = iv > iw
	})
	return st, nil
}

// windowPoint converts a content-area point (top-left origin) of s to
// window coordinates.
func (s *surface) windowPoint(x, y float64) nsPoint {
	container := send(s.web, "superview")
	cb := msgRect(container, sel("bounds"))
	return msgConvertPoint(container, sel("convertPoint:toView:"), nsPoint{x, cb.Size.Height - y}, 0)
}

// SmokeAttachHit reports what AppKit's hit testing finds at content point
// (x, y) of cv's surface: "canvas" (cv), "web" (the WKWebView or one of
// its subviews) or "other". Main thread.
func (b *backend) SmokeAttachHit(cv platform.Canvas, x, y float64) (string, error) {
	o, err := smokeOverlay(cv)
	if err != nil {
		return "", err
	}
	loadOverlay()
	res := "other"
	withPool(func() {
		s := o.s
		container := send(s.web, "superview")
		p := msgConvertPoint(send(container, "superview"), sel("convertPoint:fromView:"), s.windowPoint(x, y), 0)
		hit := msgHitTest(container, sel("hitTest:"), p)
		switch {
		case hit == 0:
		case hit == o.view:
			res = "canvas"
		case hit == s.web || sendBool(hit, "isDescendantOf:", uintptr(s.web)):
			res = "web"
		}
	})
	return res, nil
}

// SmokeAttachClick sends a synthetic left click at content point (x, y)
// through the window (NSWindow sendEvent:), so AppKit routes it by hit
// testing as for a real click. Main thread.
func (b *backend) SmokeAttachClick(cv platform.Canvas, x, y float64) error {
	o, err := smokeOverlay(cv)
	if err != nil {
		return err
	}
	loadSmoke()
	withPool(func() {
		s := o.s
		p := s.windowPoint(x, y)
		wn := sendInt(s.win, "windowNumber")
		for _, typ := range []uint{1, 2} { // left mouse down, up
			ev := msgMouseEvent(class("NSEvent"), objc.RegisterName("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
				typ, p, 0, 0, wn, 0, 0, 1, 1)
			if ev == 0 {
				err = errors.New("mouseEventWithType returned nil")
				return
			}
			send(s.win, "sendEvent:", uintptr(ev))
		}
	})
	return err
}

// SmokeAttachMove runs the mouse-moved routing for a synthetic move to
// content point (x, y) and reports whether the event went to the canvas
// only (true) or would continue to the window (false). Main thread.
func (b *backend) SmokeAttachMove(cv platform.Canvas, x, y float64) (bool, error) {
	o, err := smokeOverlay(cv)
	if err != nil {
		return false, err
	}
	loadSmoke()
	var swallowed bool
	withPool(func() {
		s := o.s
		ev := msgMouseEvent(class("NSEvent"), objc.RegisterName("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
			5, s.windowPoint(x, y), 0, 0, sendInt(s.win, "windowNumber"), 0, 0, 0, 0)
		if ev == 0 {
			err = errors.New("mouseEventWithType returned nil")
			return
		}
		swallowed = routeOverlayMove(ev)
		if !swallowed {
			// What the overlay's tracking area would get.
			send(o.view, "mouseMoved:", uintptr(ev))
		}
	})
	return swallowed, err
}
