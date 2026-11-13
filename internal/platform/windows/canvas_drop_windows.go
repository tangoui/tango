//go:build windows && (amd64 || arm64)

package windows

// Files dropped on a canvas surface created with SurfaceExtras.AcceptFiles.
// The canvas child window registers an OLE drop target (RegisterDragDrop;
// an IDropTarget implemented in Go with the sysx COM plumbing) rather than
// DragAcceptFiles/WM_DROPFILES, because only OLE tells about the drag
// while it hovers: the sink, when it implements platform.CanvasDropTarget,
// gets CanvasDragOver as the files move over the canvas and
// CanvasDragLeave when they leave, are dropped or the drag is cancelled.
// The drop is reported to the Host as SurfaceFilesDropped with the paths
// of the data object's CF_HDROP and the drop point in logical client
// pixels (the canvas fills the client area), as for WebView surfaces.
// Drags without files are refused (DROPEFFECT_NONE).

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	canvasRegisterDragDrop = ole32.export("RegisterDragDrop")
	canvasRevokeDragDrop   = ole32.export("RevokeDragDrop")
	canvasReleaseStgMedium = ole32.export("ReleaseStgMedium")
)

var canvasIIDIDropTarget = guid("00000122-0000-0000-c000-000000000046")

const (
	canvasDataGetData      = 3 // IDataObject::GetData
	canvasDataQueryGetData = 5 // IDataObject::QueryGetData
)

var (
	canvasDropOnce sync.Once
	canvasDropVtbl []uintptr
)

// canvasDropState is the Go state of a canvas's IDropTarget.
type canvasDropState struct {
	c        *canvas
	onDrop   func(paths []string, x, y int)
	accepted bool // the current drag carries files
}

// enableFileDrop registers the canvas as an OLE drop target. onDrop runs
// on the main thread with the dropped paths and the drop point (logical
// client pixels). UI thread; the registration is revoked in destroy.
func (c *canvas) enableFileDrop(onDrop func(paths []string, x, y int)) {
	if c.closed || c.dropTarget != 0 || !has(canvasRegisterDragDrop) {
		return
	}
	sxOle() // RegisterDragDrop needs OleInitialize, not just COM
	canvasDropOnce.Do(func() {
		canvasDropVtbl = sxVtable(
			// DragEnter(IDataObject*, DWORD grfKeyState, POINTL pt, DWORD* pdwEffect)
			syscall.NewCallback(func(this, data, keys, pt, effect uintptr) uintptr {
				st := canvasDropOf(this)
				if st == nil {
					return canvasDropRefuse(effect)
				}
				st.accepted = canvasHasHDrop(data)
				if !st.accepted {
					return canvasDropRefuse(effect)
				}
				st.c.dragOver()
				return canvasDropCopy(effect)
			}),
			// DragOver(DWORD grfKeyState, POINTL pt, DWORD* pdwEffect)
			syscall.NewCallback(func(this, keys, pt, effect uintptr) uintptr {
				st := canvasDropOf(this)
				if st == nil || !st.accepted {
					return canvasDropRefuse(effect)
				}
				st.c.dragOver()
				return canvasDropCopy(effect)
			}),
			// DragLeave()
			syscall.NewCallback(func(this uintptr) uintptr {
				if st := canvasDropOf(this); st != nil && st.accepted {
					st.accepted = false
					st.c.dragLeave()
				}
				return sOK
			}),
			// Drop(IDataObject*, DWORD grfKeyState, POINTL pt, DWORD* pdwEffect)
			syscall.NewCallback(func(this, data, keys, pt, effect uintptr) uintptr {
				st := canvasDropOf(this)
				if st == nil || !st.accepted {
					return canvasDropRefuse(effect)
				}
				st.accepted = false
				st.c.dragLeave()
				paths := canvasHDropPaths(data)
				if len(paths) == 0 || st.c.closed {
					return canvasDropRefuse(effect)
				}
				x, y := st.c.lastPointer()
				p := st.c.toLogical(x, y)
				st.onDrop(paths, int(p.X+0.5), int(p.Y+0.5))
				return canvasDropCopy(effect)
			}),
		)
	})
	obj := sxNewObject(canvasDropVtbl, []GUID{canvasIIDIDropTarget}, false, &canvasDropState{c: c, onDrop: onDrop})
	if hr, _, _ := canvasRegisterDragDrop.Call(c.hwnd, obj); failed(hr) {
		comCall(obj, 2) // Release: nobody else holds it
		return
	}
	c.dropTarget = obj // our reference, released in disableFileDrop
}

// disableFileDrop revokes the drop target. Called from destroy, while
// the window still exists.
func (c *canvas) disableFileDrop() {
	if c.dropTarget == 0 {
		return
	}
	c.dropHover = false // no sink calls after destroy
	canvasRevokeDragDrop.Call(c.hwnd)
	comCall(c.dropTarget, 2) // Release
	c.dropTarget = 0
}

func canvasDropOf(this uintptr) *canvasDropState {
	st, ok := sxState[*canvasDropState](this)
	if !ok || st == nil || st.c.closed {
		return nil
	}
	return st
}

// canvasDropCopy answers with DROPEFFECT_COPY when the source allows it.
func canvasDropCopy(effect uintptr) uintptr {
	if effect != 0 {
		e := (*uint32)(native(effect))
		*e = canvasDropEffect(*e)
	}
	return sOK
}

func canvasDropRefuse(effect uintptr) uintptr {
	if effect != 0 {
		*(*uint32)(native(effect)) = 0 // DROPEFFECT_NONE
	}
	return sOK
}

// dragOver reports the hover position to a sink that wants it.
func (c *canvas) dragOver() {
	t, ok := c.sink.(platform.CanvasDropTarget)
	if !ok || !c.ready || c.closed {
		return
	}
	x, y := c.lastPointer()
	p := c.toLogical(x, y)
	c.dropHover = true
	c.sinkDepth++
	t.CanvasDragOver(p.X, p.Y)
	c.sinkDepth--
}

// dragLeave ends the hover feedback.
func (c *canvas) dragLeave() {
	if !c.dropHover {
		return
	}
	c.dropHover = false
	if t, ok := c.sink.(platform.CanvasDropTarget); ok && c.ready && !c.closed {
		c.sinkDepth++
		t.CanvasDragLeave()
		c.sinkDepth--
	}
}

// canvasHDropFormat is FORMATETC for CF_HDROP in an HGLOBAL.
func canvasHDropFormat() sxFormatEtc {
	return sxFormatEtc{CfFormat: cfHDROP, Aspect: sxDVAspectContent, Lindex: -1, Tymed: sxTymedHGlobal}
}

// canvasHasHDrop reports whether a data object offers CF_HDROP.
func canvasHasHDrop(data uintptr) bool {
	if data == 0 {
		return false
	}
	f := canvasHDropFormat()
	return comCall(data, canvasDataQueryGetData, uintptr(unsafe.Pointer(&f))) == sOK
}

// canvasHDropPaths reads the file paths of a data object's CF_HDROP.
func canvasHDropPaths(data uintptr) []string {
	if data == 0 {
		return nil
	}
	f := canvasHDropFormat()
	var m sxStgMedium
	if failed(comCall(data, canvasDataGetData, uintptr(unsafe.Pointer(&f)), uintptr(unsafe.Pointer(&m)))) {
		return nil
	}
	defer canvasReleaseStgMedium.Call(uintptr(unsafe.Pointer(&m)))
	if m.Tymed != sxTymedHGlobal || m.Data == 0 {
		return nil
	}
	hdrop := m.Data
	n, _, _ := procDragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	var paths []string
	for i := uintptr(0); i < n; i++ {
		l, _, _ := procDragQueryFileW.Call(hdrop, i, 0, 0)
		buf := make([]uint16, l+1)
		procDragQueryFileW.Call(hdrop, i, uintptr(unsafe.Pointer(&buf[0])), l+1)
		if p := utf16Units(buf); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}
