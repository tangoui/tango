//go:build windows && (amd64 || arm64)

package windows

// Files dropped on a WebView2 surface that accepts files (AcceptFiles):
// WebView2 itself only navigates to the first dropped file, which
// navigationStarting turns into a one-path SurfaceFilesDropped. Where the
// runtime has chrome.webview.postMessageWithAdditionalObjects (WebView2
// 1.0.1185+), a document script intercepts the drop and posts the File
// objects; the host reads every path from the message's additional
// objects (ICoreWebView2WebMessageReceivedEventArgs2 →
// ICoreWebView2ObjectCollectionView → ICoreWebView2File::get_Path). The
// page still receives its drop event. On older runtimes the script does
// nothing and the navigation path stays.

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	sxIIDWebMessageArgs2 = guid("06fc7ab7-c90c-4297-9389-33ca01cf6d5e")
	sxIIDObjCollView     = guid("5cfec11c-25bd-4e8d-9e1a-7acdaeeec047")
	sxIIDWebView2File    = guid("f2c19559-6bc1-4583-a757-90021be9afec")
)

const (
	sxMsgArgs2GetAdditionalObjects = 6 // after get_Source, get_WebMessageAsJson, TryGetWebMessageAsString
	sxCollGetCount                 = 3
	sxCollGetValueAtIndex          = 4
	sxFileGetPath                  = 3
)

// sxInstallFileDrop adds the drop script to surfaces that accept files.
func (s *surface) sxInstallFileDrop() {
	if s.ext.acceptFiles {
		s.addDocumentScript(topFrameOnly(sxDropScript))
	}
}

// sxFileDropMessage handles the drop script's message; it reports whether
// text was one (then it is not passed on to the core).
func (s *surface) sxFileDropMessage(text string, args uintptr) bool {
	if text != sxDropMessage || !s.ext.acceptFiles {
		return false
	}
	paths := sxAdditionalFilePaths(args)
	if len(paths) > 0 {
		x, y := s.cursorInClient()
		s.event(platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: paths, X: x, Y: y})
	}
	return true
}

func sxAdditionalFilePaths(args uintptr) []string {
	a2 := queryInterface(args, &sxIIDWebMessageArgs2)
	if a2 == 0 {
		return nil
	}
	defer release(a2)
	var coll uintptr
	if failed(comCall(a2, sxMsgArgs2GetAdditionalObjects, uintptr(unsafe.Pointer(&coll)))) || coll == 0 {
		return nil
	}
	defer release(coll)
	var n uint32
	if failed(comCall(coll, sxCollGetCount, uintptr(unsafe.Pointer(&n)))) {
		return nil
	}
	var paths []string
	for i := uint32(0); i < n; i++ {
		var unk uintptr
		if failed(comCall(coll, sxCollGetValueAtIndex, uintptr(i), uintptr(unsafe.Pointer(&unk)))) || unk == 0 {
			continue
		}
		if f := queryInterface(unk, &sxIIDWebView2File); f != 0 {
			if p := comString(f, sxFileGetPath); p != "" {
				paths = append(paths, p)
			}
			release(f)
		}
		release(unk)
	}
	return paths
}
