//go:build darwin && !ios

package darwin

// Edit menu validation for canvas views. TangoCanvasView implements
// copy:, cut:, paste:, selectAll:, undo: and redo: (reported as the
// matching Cmd+key, canvas_view.go), so AppKit would enable those menu
// items whenever a canvas is first responder. validateMenuItem: and
// validateUserInterfaceItem: enable them only while text input is on
// (SetTextInput(true, ...)). Paste accepts text and, for opted-in fields,
// PNG or TIFF. Other actions keep AppKit's default (enabled).

import (
	"github.com/ebitengine/purego/objc"
)

// editActions are the Edit menu actions a canvas view implements.
var editActions = map[string]bool{
	"copy:": true, "cut:": true, "paste:": true, "selectAll:": true, "undo:": true, "redo:": true,
}

func canvasValidateMethods() []objc.MethodDef {
	validate := func(self id, _ objc.SEL, item id) bool {
		return canvasValidates(self, item)
	}
	return []objc.MethodDef{
		method("validateMenuItem:", validate),
		method("validateUserInterfaceItem:", validate),
	}
}

// canvasValidates reports whether the canvas view self enables item.
func canvasValidates(self, item id) bool {
	if item == 0 {
		return true
	}
	act := objc.SEL(send(item, "action"))
	if act == 0 {
		return true
	}
	name := selGetName(act)
	if !editActions[name] {
		return true
	}
	c := canvasOf(self)
	if c == nil || !c.textOn {
		return false
	}
	if name == "paste:" {
		if c.imagePaste {
			return pasteboardHasTypes(pasteboardTypeString, pasteboardTypePNG, pasteboardTypeTIFF)
		}
		return pasteboardHasText()
	}
	return true
}

// pasteboardHasText reports whether the general pasteboard holds text.
func pasteboardHasText() bool {
	return pasteboardHasTypes(pasteboardTypeString)
}

func (c *canvas) SetImagePasteEnabled(on bool) { c.imagePaste = on }

func pasteboardHasTypes(types ...string) bool {
	has := false
	withPool(func() {
		pb := send(class("NSPasteboard"), "generalPasteboard")
		if pb == 0 {
			return
		}
		ids := make([]id, len(types))
		for i, typ := range types {
			ids[i] = nsString(typ)
		}
		has = send(pb, "availableTypeFromArray:", uintptr(nsArray(ids...))) != 0
	})
	return has
}
