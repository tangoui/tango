//go:build windows && (amd64 || arm64)

package windows

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

const (
	wvAddScriptDialogOpening   = int(unsafe.Offsetof(vtICoreWebView2{}.AddScriptDialogOpening) / slotSize)
	wvAddWindowCloseRequested  = int(unsafe.Offsetof(vtICoreWebView2{}.AddWindowCloseRequested) / slotSize)
	setPutDefaultScriptDialogs = int(unsafe.Offsetof(vtICoreWebView2Settings{}.PutAreDefaultScriptDialogsEnabled) / slotSize)

	scriptDialogGetURI         = 3
	scriptDialogGetKind        = 4
	scriptDialogGetMessage     = 5
	scriptDialogAccept         = 6
	scriptDialogGetDefaultText = 7
	scriptDialogPutResultText  = 9
	scriptDialogGetDeferral    = 10

	newWinPutWindow   = 4
	newWinGetDeferral = 9
)

func (s *surface) interactionHost() platform.WebInteractionHost {
	h, _ := s.b.h.(platform.WebInteractionHost)
	return h
}

func (s *surface) SetWebDialogs(on bool) {
	s.whenReady(func() {
		if settings := comObject(s.webview, wvGetSettings); settings != 0 {
			comCall(settings, setPutDefaultScriptDialogs, boolArg(!on))
			release(settings)
		}
	})
}

func (e *embedded) SetWebDialogs(on bool) {
	e.w.SetWebDialogs(on)
}

func (s *surface) onScriptDialog(_, args uintptr) {
	h := s.interactionHost()
	if s.closed || h == nil {
		return
	}
	kind, _ := comOut[int32](args, scriptDialogGetKind)
	req := platform.WebDialogRequest{
		Kind:        platform.WebDialogKind(kind),
		URL:         comString(args, scriptDialogGetURI),
		Message:     comString(args, scriptDialogGetMessage),
		DefaultText: comString(args, scriptDialogGetDefaultText),
	}
	d := getDeferral(args, scriptDialogGetDeferral)
	if d == 0 {
		return
	}
	addRef(args)
	finish := func(r platform.WebDialogResponse) {
		if r.Accepted {
			if req.Kind == platform.WebDialogPrompt {
				comCall(args, scriptDialogPutResultText, uintptr(unsafe.Pointer(u16(r.Text))))
			}
			comCall(args, scriptDialogAccept)
		}
		completeDeferral(d)
		release(args)
	}
	if !h.OnWebDialog(s.id, req, finish) {
		finish(platform.WebDialogResponse{})
	}
}

func (s *surface) claimPopup(req platform.NewWindowRequest, args uintptr) bool {
	h := s.interactionHost()
	if h == nil {
		return false
	}
	return h.OnWebPopup(s.id, req, func(id, parent platform.SurfaceID, opts platform.SurfaceOptions) (platform.Surface, error) {
		d := getDeferral(args, newWinGetDeferral)
		if d == 0 {
			return nil, platform.ErrUnsupported
		}
		var child *surface
		var result platform.Surface
		if parent != 0 {
			e, err := s.b.newEmbeddedWebView(id, parent, opts, platform.WebOptions{Profile: s.webProfile}, false)
			if err != nil {
				completeDeferral(d)
				return nil, err
			}
			child, result = e.w, e
		} else {
			var err error
			child, err = s.b.newWindow(id, opts, nil)
			if err != nil {
				completeDeferral(d)
				return nil, err
			}
			child.webProfile = s.webProfile
			child.finishSurface()
			result = child
		}
		addRef(args)
		var finished bool
		finish := func(w uintptr) {
			if finished {
				return
			}
			finished = true
			if w != 0 && !s.closed {
				if hr := comCall(args, newWinPutWindow, w); failed(hr) {
					w = 0
				}
			}
			completeDeferral(d)
			release(args)
			if w == 0 && !child.closed {
				result.Close()
			}
		}
		child.webCall(func(error) {
			finish(0)
		}, func(claim func() bool) {
			if claim() {
				if s.closed {
					finish(0)
					return
				}
				addWebViewEvent(child.webview, wvAddWindowCloseRequested, func(_, _ uintptr) { result.Close() })
				finish(child.webview)
			}
		})
		s.b.whenEnvironment(child.createWebView)
		return result, nil
	})
}
