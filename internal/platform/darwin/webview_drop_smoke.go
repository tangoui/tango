//go:build darwin && !ios

package darwin

// Test hook for smokewebdrop: a simulated file drag over a WebView
// surface, driven phase by phase so the web process can deliver the
// page's drag events in between. WKWebView passes the dragging info to
// WebKit, which asks it more than a canvas does, so the object answers
// the whole NSDraggingInfo protocol (as WebKit's own API tests do). For
// the internal smoke tests only.

import (
	"errors"
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var (
	smokeWebDragOnce sync.Once
	smokeWebDragWin  id      // draggingDestinationWindow
	smokeWebDragN    int     // numberOfValidItemsForDrop
	smokeWebDragInfo id      // the current drag's info, kept across phases
	smokeWebDragPB   id      // its pasteboard
	smokeWebDragLoc  nsPoint // its draggingLocation (window coordinates)
)

func loadSmokeWebDrag() {
	smokeWebDragOnce.Do(func() {
		classDef("TangoSmokeWebDraggingInfo", "NSObject", nil, []objc.MethodDef{
			method("draggingPasteboard", func(self id, _ objc.SEL) id { return smokeWebDragPB }),
			method("draggingLocation", func(self id, _ objc.SEL) nsPoint { return smokeWebDragLoc }),
			method("draggedImageLocation", func(self id, _ objc.SEL) nsPoint { return smokeWebDragLoc }),
			method("draggedImage", func(self id, _ objc.SEL) id { return 0 }),
			method("draggingSource", func(self id, _ objc.SEL) id { return 0 }),
			method("draggingDestinationWindow", func(self id, _ objc.SEL) id { return smokeWebDragWin }),
			method("draggingSourceOperationMask", func(self id, _ objc.SEL) uint { return nsDragOperationCopy }),
			method("draggingSequenceNumber", func(self id, _ objc.SEL) int { return 7 }),
			method("numberOfValidItemsForDrop", func(self id, _ objc.SEL) int { return smokeWebDragN }),
			method("setNumberOfValidItemsForDrop:", func(self id, _ objc.SEL, n int) { smokeWebDragN = n }),
			method("draggingFormation", func(self id, _ objc.SEL) int { return 0 }),
			method("setDraggingFormation:", func(self id, _ objc.SEL, _ int) {}),
			method("animatesToDestination", func(self id, _ objc.SEL) bool { return false }),
			method("setAnimatesToDestination:", func(self id, _ objc.SEL, _ bool) {}),
			method("springLoadingHighlight", func(self id, _ objc.SEL) int { return 0 }),
			method("resetSpringLoading", func(self id, _ objc.SEL) {}),
			method("slideDraggedImageTo:", func(self id, _ objc.SEL, _ nsPoint) {}),
			method("namesOfPromisedFilesDroppedAtDestination:", func(self id, _ objc.SEL, _ id) id { return 0 }),
			method("enumerateDraggingItemsWithOptions:forView:classes:searchOptions:usingBlock:",
				func(self id, _ objc.SEL, _ uint, _, _, _, _ id) {}),
		})
	})
}

// SmokeWebViewDrag drives one phase of a simulated drag of paths over
// surface id's WebView at (x, y), page coordinates (logical, top-left):
// "enter" starts the drag (no paths: a drag without files), "move" moves
// it, "leave" ends it outside the view and "drop" drops it. It reports the
// operation draggingEntered: / draggingUpdated: returned (1 = copy), or
// for "drop" whether performDragOperation: took it (1). Drop events reach
// the host synchronously. Main thread.
func (b *backend) SmokeWebViewDrag(sid platform.SurfaceID, phase string, paths []string, x, y float64) (uint, error) {
	s := b.surfaces[sid]
	if s == nil || s.closed || s.web == 0 {
		return 0, errors.New("no open WebView surface")
	}
	loadSmokeWebDrag()
	var out uint
	var err error
	withPool(func() {
		if phase == "enter" {
			smokeWebDragEnd()
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
			smokeWebDragPB = send(pb, "retain")
			smokeWebDragN = max(len(urls), 1)
			smokeWebDragWin = s.win
			smokeWebDragInfo = send(send(class("TangoSmokeWebDraggingInfo"), "alloc"), "init")
		}
		info := smokeWebDragInfo
		if info == 0 {
			err = errors.New("no drag: start with enter")
			return
		}
		p := nsPoint{x, y}
		if !sendBool(s.web, "isFlipped") {
			p.Y = msgRect(s.web, sel("bounds")).Size.Height - y
		}
		smokeWebDragLoc = msgConvertPoint(s.web, sel("convertPoint:toView:"), p, 0)
		switch phase {
		case "enter":
			out = uint(send(s.web, "draggingEntered:", uintptr(info)))
		case "move":
			out = uint(send(s.web, "draggingUpdated:", uintptr(info)))
		case "leave":
			send(s.web, "draggingExited:", uintptr(info))
			smokeWebDragEnd()
		case "drop":
			if sendBool(s.web, "prepareForDragOperation:", uintptr(info)) && sendBool(s.web, "performDragOperation:", uintptr(info)) {
				out = 1
			}
			send(s.web, "concludeDragOperation:", uintptr(info))
			smokeWebDragEnd()
		default:
			err = errors.New("unknown phase " + phase)
		}
	})
	return out, err
}

func smokeWebDragEnd() {
	if smokeWebDragInfo != 0 {
		release(smokeWebDragInfo)
		smokeWebDragInfo = 0
	}
	if smokeWebDragPB != 0 {
		send(smokeWebDragPB, "releaseGlobally")
		release(smokeWebDragPB)
		smokeWebDragPB = 0
	}
}
