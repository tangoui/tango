//go:build darwin && !ios

package darwin

// platform.FileDragger on macOS: an NSDraggingSession started on the
// surface's content view with one NSDraggingItem per file (NSURL is an
// NSPasteboardWriting) and a TangoDragSource as the NSDraggingSource.
//
// AppKit ties a session to a mouse event. A local event monitor
// (installSysx, called from Init) remembers the last left-button press; if
// the button is still down when StartFileDrag runs — the page relayed its
// mousedown — that press starts the session (a drag event at the mouse
// location is synthesized if the press was elsewhere). With no button held
// no session starts and done reports DragNone, as the contract says: a
// session without a press would drop wherever the pointer happens to be.

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.FileDragger = (*surface)(nil)

// NSDragOperation bits and event constants.
const (
	nsDragOpCopy    = 1
	nsDragOpLink    = 2
	nsDragOpGeneric = 4
	nsDragOpMove    = 16

	nsEventTypeLeftMouseDown    = 1
	nsEventTypeLeftMouseDragged = 6

	dragIconSize = 64.0
)

var dragState struct {
	once     sync.Once
	monitor  id // the local event monitor, retained
	lastDown id // last left mouse-down event, retained
	pending  map[id]func(platform.DragOperation)

	msgMouseEvent func(cls id, s objc.SEL, typ uint, loc nsPoint, flags uint, ts float64, wn int, ctx id, evNum, clicks int, pressure float32) id
	msgSetFrame   func(obj id, s objc.SEL, r nsRect, contents id)
	msgBegin      func(obj id, s objc.SEL, items, event, source id) id
	msgLocation   func(obj id, s objc.SEL) nsPoint // +mouseLocation, -locationInWindow
	msgConvert    func(obj id, s objc.SEL, p nsPoint, view id) nsPoint
}

// dragOperationFrom maps an NSDragOperation to the contract's.
func dragOperationFrom(op uint) platform.DragOperation {
	switch {
	case op&nsDragOpMove != 0:
		return platform.DragMove
	case op&nsDragOpCopy != 0, op&nsDragOpGeneric != 0:
		return platform.DragCopy
	case op&nsDragOpLink != 0:
		return platform.DragLink
	}
	return platform.DragNone
}

func initDrag() {
	dragState.once.Do(func() {
		purego.RegisterFunc(&dragState.msgMouseEvent, msgSendAddr)
		purego.RegisterFunc(&dragState.msgSetFrame, msgSendAddr)
		purego.RegisterFunc(&dragState.msgBegin, msgSendAddr)
		purego.RegisterFunc(&dragState.msgLocation, msgSendAddr)
		purego.RegisterFunc(&dragState.msgConvert, msgSendAddr)
		dragState.pending = map[id]func(platform.DragOperation){}
		classDef("TangoDragSource", "NSObject", []string{"NSDraggingSource"}, []objc.MethodDef{
			// Outside the app the target may copy or link; moving is not
			// offered, so Finder never takes the app's own file away.
			method("draggingSession:sourceOperationMaskForDraggingContext:", func(self id, _ objc.SEL, session id, ctx int) uint {
				if ctx == 0 { // NSDraggingContextOutsideApplication
					return nsDragOpCopy | nsDragOpLink | nsDragOpGeneric
				}
				return nsDragOpCopy | nsDragOpGeneric
			}),
			// The NSPoint argument travels in two floating-point registers
			// on arm64 and amd64, the same as two float64 parameters.
			method("draggingSession:endedAtPoint:operation:", func(self id, _ objc.SEL, session id, x, y float64, op uint) {
				done := dragState.pending[self]
				delete(dragState.pending, self)
				release(self)
				if done != nil {
					done(dragOperationFrom(op))
				}
			}),
		})
	})
}

// installSysx sets up what the sysx capabilities need from launch on: the
// mouse-down monitor for StartFileDrag.
func installSysx() {
	initDrag()
	withPool(func() {
		handler := objc.NewBlock(func(_ objc.Block, ev id) id {
			if old := dragState.lastDown; old != ev {
				dragState.lastDown = retain(ev)
				release(old)
			}
			return ev // pass it on
		})
		dragState.monitor = retain(send(class("NSEvent"), "addLocalMonitorForEventsMatchingMask:handler:",
			nsEventMaskLeftMouseDown, uintptr(handler)))
		handler.Release()
	})
}

var errNoView = errors.New("tango/darwin: the surface has no view to drag from")

func (s *surface) StartFileDrag(paths []string, icon []byte, done func(platform.DragOperation)) error {
	initDrag()
	if s.closed || s.view == 0 || s.win == 0 {
		return errNoView
	}
	if sendInt(class("NSEvent"), "pressedMouseButtons")&1 == 0 {
		hopMain(func() { done(platform.DragNone) })
		return nil
	}
	var err error
	withPool(func() {
		ev := s.dragEvent()
		if ev == 0 {
			err = errors.New("tango/darwin: cannot create a drag event")
			return
		}
		// Event location in window coordinates, then in the view's.
		p := dragState.msgConvert(s.view, sel("convertPoint:fromView:"), dragState.msgLocation(ev, sel("locationInWindow")), 0)
		var custom id
		if len(icon) > 0 {
			custom = nsImagePNG(icon, dragIconSize)
		}
		ws := send(class("NSWorkspace"), "sharedWorkspace")
		items := send(class("NSMutableArray"), "arrayWithCapacity:", uintptr(len(paths)))
		for i, path := range paths {
			item := autorelease(send(send(class("NSDraggingItem"), "alloc"), "initWithPasteboardWriter:", uintptr(fileURL(path))))
			img := custom
			if img == 0 {
				img = send(ws, "iconForFile:", uintptr(nsString(path)))
			}
			off := float64(i) * 8 // fan the icons out a little
			frame := nsRect{Origin: nsPoint{X: p.X - dragIconSize/2 + off, Y: p.Y - dragIconSize/2 - off}, Size: nsSize{dragIconSize, dragIconSize}}
			dragState.msgSetFrame(item, sel("setDraggingFrame:contents:"), frame, img)
			send(items, "addObject:", uintptr(item))
		}
		source := alloc("TangoDragSource") // released when the session ends
		dragState.pending[source] = done
		session := dragState.msgBegin(s.view, sel("beginDraggingSessionWithItems:event:source:"), items, ev, source)
		if session == 0 {
			delete(dragState.pending, source)
			release(source)
			err = errors.New("tango/darwin: beginDraggingSessionWithItems failed")
			return
		}
		send(session, "setAnimatesToStartingPositionsOnCancelOrFail:", 1)
	})
	return err
}

// dragEvent is the press that starts the drag (the left button is held):
// the remembered mouse-down when it was in this window, else a
// synthesized drag event at the mouse location.
func (s *surface) dragEvent() id {
	if ev := dragState.lastDown; ev != 0 && send(ev, "window") == s.win {
		return ev
	}
	if ev := send(s.b.app, "currentEvent"); ev != 0 {
		switch sendInt(ev, "type") {
		case nsEventTypeLeftMouseDown, nsEventTypeLeftMouseDragged:
			if send(ev, "window") == s.win {
				return ev
			}
		}
	}
	screen := dragState.msgLocation(class("NSEvent"), sel("mouseLocation"))
	frame := msgRect(s.win, sel("frame"))
	inWin := nsPoint{X: screen.X - frame.Origin.X, Y: screen.Y - frame.Origin.Y}
	return dragState.msgMouseEvent(class("NSEvent"),
		sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
		nsEventTypeLeftMouseDragged, inWin, 0, 0, sendInt(s.win, "windowNumber"), 0, 0, 1, 1)
}
