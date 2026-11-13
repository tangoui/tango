//go:build darwin && !ios

package darwin

// Test hooks for the canvas smoke tests (smokecanvas, smokeattach): a
// simulated file drag (canvas_drop.go), Edit menu validation
// (canvas_edit.go) and mouse moves that also reach the WebView
// (canvas_attach.go's hover clearing). For the internal smoke tests only.

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var (
	smokeDragOnce sync.Once
	smokeDragPB   id      // pasteboard returned by the fake NSDraggingInfo
	smokeDragLoc  nsPoint // its draggingLocation (window coordinates)
	msgMenuItem   func(obj id, s objc.SEL, title id, action objc.SEL, key id) id
)

func loadSmokeDrag() {
	smokeDragOnce.Do(func() {
		purego.RegisterFunc(&msgMenuItem, msgSendAddr)
		classDef("TangoSmokeDraggingInfo", "NSObject", nil, []objc.MethodDef{
			method("draggingPasteboard", func(self id, _ objc.SEL) id { return smokeDragPB }),
			method("draggingLocation", func(self id, _ objc.SEL) nsPoint { return smokeDragLoc }),
			method("draggingSourceOperationMask", func(self id, _ objc.SEL) uint { return nsDragOperationCopy }),
			method("draggingSequenceNumber", func(self id, _ objc.SEL) int { return 1 }),
		})
	})
}

// smokeCanvas returns the darwin canvas behind cv (a canvas surface's or
// an attached one).
func smokeCanvas(cv platform.Canvas) (*canvas, error) {
	switch c := cv.(type) {
	case *canvas:
		if !c.closed {
			return c, nil
		}
	case *overlay:
		if !c.closed {
			return c.canvas, nil
		}
	}
	return nil, errors.New("not an open darwin canvas")
}

// SmokeCanvasDrop simulates a drag of paths (no paths: a drag without
// files) entering cv's view at (x, y) — view coordinates, logical, top-left
// — and being dropped there. It reports the operation draggingEntered:
// returned (1 = copy, 0 = refused) and what performDragOperation:
// returned. Drop events reach the host synchronously. Main thread.
func (b *backend) SmokeCanvasDrop(cv platform.Canvas, paths []string, x, y float64) (op uint, dropped bool, err error) {
	c, err := smokeCanvas(cv)
	if err != nil {
		return 0, false, err
	}
	loadSmokeDrag()
	withPool(func() {
		pb := send(class("NSPasteboard"), "pasteboardWithUniqueName")
		send(pb, "clearContents")
		var urls []id
		for _, p := range paths {
			urls = append(urls, send(class("NSURL"), "fileURLWithPath:", uintptr(nsString(p))))
		}
		if len(urls) > 0 {
			send(pb, "writeObjects:", uintptr(nsArray(urls...)))
		} else {
			send(pb, "setString:forType:", uintptr(nsString("text")), uintptr(nsString(pasteboardTypeString)))
		}
		smokeDragPB = pb
		smokeDragLoc = msgConvertPoint(c.view, sel("convertPoint:toView:"), nsPoint{x, y}, 0)
		info := send(send(class("TangoSmokeDraggingInfo"), "alloc"), "init")
		op = uint(send(c.view, "draggingEntered:", uintptr(info)))
		send(c.view, "draggingUpdated:", uintptr(info))
		if op != nsDragOperationNone && sendBool(c.view, "prepareForDragOperation:", uintptr(info)) {
			dropped = sendBool(c.view, "performDragOperation:", uintptr(info))
			send(c.view, "concludeDragOperation:", uintptr(info))
		} else {
			send(c.view, "draggingExited:", uintptr(info))
		}
		release(info)
		send(pb, "releaseGlobally")
		smokeDragPB = 0
	})
	return op, dropped, nil
}

// SmokeCanvasEditMenu asks cv's view to validate menu items with the Edit
// actions (and one other action, "performClose:") and reports which it
// enables. Main thread.
func (b *backend) SmokeCanvasEditMenu(cv platform.Canvas) (map[string]bool, error) {
	c, err := smokeCanvas(cv)
	if err != nil {
		return nil, err
	}
	loadSmokeDrag()
	out := map[string]bool{}
	withPool(func() {
		for _, act := range []string{"copy:", "cut:", "paste:", "selectAll:", "undo:", "redo:", "performClose:"} {
			item := msgMenuItem(send(class("NSMenuItem"), "alloc"), sel("initWithTitle:action:keyEquivalent:"),
				nsString(act), sel(act), nsString(""))
			out[act] = sendBool(c.view, "validateMenuItem:", uintptr(item))
			release(item)
		}
	})
	out["pasteboard-text"] = pasteboardHasText()
	return out, nil
}

// SmokeAttachPointer sends a synthetic mouse move to content point (x, y)
// of cv's surface the way a real one travels: through the overlay
// routing, and when that does not swallow it, to the WebView and to the
// overlay's tracking area. It reports whether the overlay swallowed it.
// Main thread.
func (b *backend) SmokeAttachPointer(cv platform.Canvas, x, y float64) (bool, error) {
	o, err := smokeOverlay(cv)
	if err != nil {
		return false, err
	}
	loadSmoke()
	var swallowed bool
	withPool(func() {
		s := o.s
		ev := msgMouseEvent(class("NSEvent"), objc.RegisterName("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
			nsEventTypeMouseMoved, s.windowPoint(x, y), 0, msgGetFloat(send(class("NSProcessInfo"), "processInfo"), sel("systemUptime")),
			sendInt(s.win, "windowNumber"), 0, 0, 0, 0)
		if ev == 0 {
			err = errors.New("mouseEventWithType returned nil")
			return
		}
		swallowed = routeOverlayMove(ev)
		if !swallowed {
			send(s.webMoveTarget(), "mouseMoved:", uintptr(ev))
			send(o.view, "mouseMoved:", uintptr(ev))
		}
	})
	return swallowed, err
}
