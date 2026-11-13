//go:build windows && (amd64 || arm64)

package windows

// Test hook for internal/platform/windows/internal/smokecanvas: drives the
// canvas's OLE drop target (canvas_drop_windows.go) through its vtable
// with a CF_HDROP data object, as OLE does during a real drag, so the
// smoke test covers the drop path without a mouse. For the internal smoke
// test only.

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// IDropTarget vtable slots after IUnknown.
const (
	canvasDTDragEnter = 3
	canvasDTDragOver  = 4
	canvasDTDragLeave = 5
	canvasDTDrop      = 6
)

// SmokeCanvasDrop drags paths over cv and drops them (cancel false), or
// drags them over and away (cancel true). The drop is reported to the
// Host as SurfaceFilesDropped and, when the sink implements
// platform.CanvasDropTarget, the hover through it. It returns the
// effect the canvas answered on DragEnter. Main thread.
func (b *Backend) SmokeCanvasDrop(cv platform.Canvas, paths []string, cancel bool) (effect uint32, err error) {
	c, ok := cv.(*canvas)
	if !ok || c.closed {
		return 0, errors.New("not an open windows canvas")
	}
	if c.dropTarget == 0 {
		return 0, errors.New("canvas has no drop target (SurfaceExtras.AcceptFiles off?)")
	}
	hdrop, err := dropFiles(paths)
	if err != nil {
		return 0, err
	}
	data := sxNewHDropObject(hdrop)
	defer comCall(data, 2) // Release
	t := c.dropTarget
	allowed := uint32(sxDropEffectCopy | sxDropEffectLink)
	effect = allowed
	if hr := comCall(t, canvasDTDragEnter, data, 0, 0, uintptr(unsafe.Pointer(&effect))); failed(hr) {
		return 0, fmt.Errorf("DragEnter: %#x", hr)
	}
	e := allowed
	comCall(t, canvasDTDragOver, 0, 0, uintptr(unsafe.Pointer(&e)))
	if cancel {
		comCall(t, canvasDTDragLeave)
		return effect, nil
	}
	e = allowed
	if hr := comCall(t, canvasDTDrop, data, 0, 0, uintptr(unsafe.Pointer(&e))); failed(hr) {
		return effect, fmt.Errorf("Drop: %#x", hr)
	}
	return effect, nil
}
