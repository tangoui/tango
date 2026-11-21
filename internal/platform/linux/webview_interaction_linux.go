//go:build linux && !android

package linux

import (
	"runtime"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var webInteraction struct {
	dialogRef                    func(ptr) ptr
	dialogUnref, dialogClose     func(ptr)
	dialogKind                   func(ptr) int32
	dialogMessage, dialogDefault func(ptr) ptr
	dialogConfirm                func(ptr, bool)
	dialogText                   func(ptr, string)
	fileMultiple                 func(ptr) bool
	fileMIME                     func(ptr) ptr
	fileSelect                   func(ptr, **byte)
	fileCancel                   func(ptr)
	webViewType                  func() uintptr
	newRelated                   func(uintptr, string, ptr, string, ptr, uintptr) ptr
}

func bindWebInteraction() {
	var bd binder
	w := libWebKit
	bd.opt(&webInteraction.dialogRef, "webkit_script_dialog_ref", w)
	bd.opt(&webInteraction.dialogUnref, "webkit_script_dialog_unref", w)
	bd.opt(&webInteraction.dialogClose, "webkit_script_dialog_close", w)
	bd.opt(&webInteraction.dialogKind, "webkit_script_dialog_get_dialog_type", w)
	bd.opt(&webInteraction.dialogMessage, "webkit_script_dialog_get_message", w)
	bd.opt(&webInteraction.dialogDefault, "webkit_script_dialog_prompt_get_default_text", w)
	bd.opt(&webInteraction.dialogConfirm, "webkit_script_dialog_confirm_set_confirmed", w)
	bd.opt(&webInteraction.dialogText, "webkit_script_dialog_prompt_set_text", w)
	bd.opt(&webInteraction.fileMultiple, "webkit_file_chooser_request_get_select_multiple", w)
	bd.opt(&webInteraction.fileMIME, "webkit_file_chooser_request_get_mime_types", w)
	bd.opt(&webInteraction.fileSelect, "webkit_file_chooser_request_select_files", w)
	bd.opt(&webInteraction.fileCancel, "webkit_file_chooser_request_cancel", w)
	bd.opt(&webInteraction.webViewType, "webkit_web_view_get_type", w)
	bd.opt(&webInteraction.newRelated, "g_object_new", libGObject)
}

func (s *surface) interactionHost() platform.WebInteractionHost {
	h, _ := s.b.host.(platform.WebInteractionHost)
	return h
}

func (s *surface) initWebInteraction() {
	x := &webInteraction
	if x.dialogRef != nil && x.dialogUnref != nil && x.dialogClose != nil && x.dialogKind != nil &&
		x.dialogMessage != nil && x.dialogDefault != nil && x.dialogConfirm != nil && x.dialogText != nil {
		connect(s.web, "script-dialog", 1, s.onScriptDialog)
	}
	if x.fileMultiple != nil && x.fileSelect != nil && x.fileCancel != nil {
		connect(s.web, "run-file-chooser", 1, s.onFileChooser)
	}
}

func (s *surface) onScriptDialog(_, dialog ptr) ptr {
	h := s.interactionHost()
	if s.closed || h == nil || !h.HasWebDialogHandler(s.id) {
		return 0
	}
	x := &webInteraction
	kind := platform.WebDialogKind(x.dialogKind(dialog))
	req := platform.WebDialogRequest{Kind: kind, URL: goStr(webkitWebViewGetURI(s.web)), Message: goStr(x.dialogMessage(dialog))}
	if kind == platform.WebDialogPrompt {
		req.DefaultText = goStr(x.dialogDefault(dialog))
	}
	x.dialogRef(dialog)
	if h.OnWebDialog(s.id, req, func(r platform.WebDialogResponse) {
		switch kind {
		case platform.WebDialogConfirm, platform.WebDialogBeforeUnload:
			x.dialogConfirm(dialog, r.Accepted)
		case platform.WebDialogPrompt:
			if r.Accepted {
				x.dialogText(dialog, r.Text)
			}
		}
		x.dialogClose(dialog)
		x.dialogUnref(dialog)
	}) {
		return 1
	}
	x.dialogUnref(dialog)
	return 0
}

func (s *surface) onFileChooser(_, request ptr) ptr {
	h := s.interactionHost()
	if s.closed || h == nil {
		return 0
	}
	x := &webInteraction
	req := platform.WebFileChooserRequest{URL: goStr(webkitWebViewGetURI(s.web)), Multiple: x.fileMultiple(request)}
	if x.fileMIME != nil {
		for p := toPtr(x.fileMIME(request)); p != nil; p = unsafe.Add(p, unsafe.Sizeof(ptr(0))) {
			if v := *(*ptr)(p); v != 0 {
				req.Accept = append(req.Accept, goStr(v))
			} else {
				break
			}
		}
	}
	wv.objectRef(request)
	if h.OnWebFileChooser(s.id, req, func(paths []string) {
		defer gObjectUnref(request)
		if len(paths) == 0 {
			x.fileCancel(request)
			return
		}
		files := make([]*byte, len(paths)+1)
		for i, path := range paths {
			v, err := syscall.BytePtrFromString(path)
			if err != nil {
				x.fileCancel(request)
				return
			}
			files[i] = v
		}
		x.fileSelect(request, &files[0])
		runtime.KeepAlive(files)
	}) {
		return 1
	}
	gObjectUnref(request)
	return 0
}

func (s *surface) claimPopup(act ptr) ptr {
	h := s.interactionHost()
	if h == nil || webInteraction.newRelated == nil || webInteraction.webViewType == nil {
		return 0
	}
	var web ptr
	h.OnWebPopup(s.id, s.windowRequest(act), func(id, parent platform.SurfaceID, o platform.SurfaceOptions) (platform.Surface, error) {
		if parent != 0 {
			if h := embedHosts[s.b.surfaces[parent]]; h != nil {
				for _, opener := range h.list {
					if opener.w == s {
						e, err := s.b.newEmbeddedWebView(id, h.s, h.overlay, opener.attach, o, platform.WebOptions{}, s.web)
						if err == nil {
							web = e.w.web
						}
						return e, err
					}
				}
			}
			return nil, platform.ErrUnsupported
		}
		child, err := s.b.newWebSurface(id, o, nil, s.web)
		if err == nil {
			web = child.(*surface).web
		}
		return child, err
	})
	return web
}
