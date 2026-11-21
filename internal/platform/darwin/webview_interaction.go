//go:build darwin && !ios

package darwin

import (
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

func (s *surface) interactionHost() platform.WebInteractionHost {
	h, _ := s.b.h.(platform.WebInteractionHost)
	return h
}

func (s *surface) scriptDialog(kind platform.WebDialogKind, message, text, frame, handler id) {
	held := holdBlock(handler)
	finish := func(r platform.WebDialogResponse) {
		withPool(func() {
			switch kind {
			case platform.WebDialogAlert:
				held.call()
			case platform.WebDialogPrompt:
				if r.Accepted {
					held.call(uintptr(nsString(r.Text)))
				} else {
					held.call(0)
				}
			default:
				held.call(boolArg(r.Accepted))
			}
		})
	}
	req := platform.WebDialogRequest{
		Kind: kind, URL: goString(send(send(send(frame, "request"), "URL"), "absoluteString")),
		Message: goString(message), DefaultText: goString(text),
	}
	if h := s.interactionHost(); h != nil && h.OnWebDialog(s.id, req, finish) {
		return
	}
	finish(platform.WebDialogResponse{})
}

func (s *surface) fileChooser(params, frame, handler id) {
	held := holdBlock(handler)
	req := platform.WebFileChooserRequest{
		URL:         goString(send(send(send(frame, "request"), "URL"), "absoluteString")),
		Multiple:    sendBool(params, "allowsMultipleSelection"),
		Directories: respondsTo(params, "allowsDirectories") && sendBool(params, "allowsDirectories"),
	}
	finish := func(paths []string) {
		withPool(func() {
			if len(paths) == 0 || s.closed {
				held.call(0)
				return
			}
			urls := make([]id, len(paths))
			for i, p := range paths {
				urls[i] = fileURL(p)
			}
			held.call(uintptr(nsArray(urls...)))
		})
	}
	if h := s.interactionHost(); h != nil && h.OnWebFileChooser(s.id, req, finish) {
		return
	}
	parent := s.id
	if e := embedByWeb[s.web]; e != nil {
		parent = e.parent.id
	}
	s.b.OpenFileDialog(parent, platform.FileOptions{Multiple: req.Multiple, PickFolders: req.Directories}, func(paths []string, _ error) {
		finish(paths)
	})
}

func webInteractionMethods(of func(id) *surface) []objc.MethodDef {
	dialog := func(kind platform.WebDialogKind) func(id, objc.SEL, id, id, id, id) {
		return func(self id, _ objc.SEL, web, message, frame, handler id) {
			if s := of(self); s != nil {
				s.scriptDialog(kind, message, 0, frame, handler)
			} else if kind == platform.WebDialogAlert {
				invokeBlock(uintptr(handler))
			} else {
				invokeBlock(uintptr(handler), 0)
			}
		}
	}
	return []objc.MethodDef{
		method("webView:runJavaScriptAlertPanelWithMessage:initiatedByFrame:completionHandler:", dialog(platform.WebDialogAlert)),
		method("webView:runJavaScriptConfirmPanelWithMessage:initiatedByFrame:completionHandler:", dialog(platform.WebDialogConfirm)),
		method("_webView:runBeforeUnloadConfirmPanelWithMessage:initiatedByFrame:completionHandler:", dialog(platform.WebDialogBeforeUnload)),
		method("webView:runJavaScriptTextInputPanelWithPrompt:defaultText:initiatedByFrame:completionHandler:", func(self id, _ objc.SEL, web, message, text, frame, handler id) {
			if s := of(self); s != nil {
				s.scriptDialog(platform.WebDialogPrompt, message, text, frame, handler)
			} else {
				invokeBlock(uintptr(handler), 0)
			}
		}),
		method("webView:runOpenPanelWithParameters:initiatedByFrame:completionHandler:", func(self id, _ objc.SEL, web, params, frame, handler id) {
			if s := of(self); s != nil {
				s.fileChooser(params, frame, handler)
			} else {
				invokeBlock(uintptr(handler), 0)
			}
		}),
		method("webViewDidClose:", func(self id, _ objc.SEL, web id) {
			if s := of(self); s != nil && s.popup {
				if e := embedByWeb[s.web]; e != nil {
					e.Close()
				} else {
					s.Close()
				}
			}
		}),
	}
}
