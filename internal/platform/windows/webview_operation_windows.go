//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/json"
	"math"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

func (e *embedded) EvalWebJSON(expression string, done func([]byte, error)) {
	s := e.w
	s.webCall(func(err error) { done(nil, err) }, func(claim func() bool) {
		hr := withHandler(func(hr, result uintptr) {
			if !claim() {
				return
			}
			if failed(hr) {
				done(nil, hresultError("ExecuteScript", hr))
				return
			}
			var value string
			err := json.Unmarshal([]byte(wstr(result)), &value)
			done([]byte(value), err)
		}, func(h uintptr) uintptr {
			return comCall(s.webview, wvExecuteScript, uintptr(unsafe.Pointer(u16(platform.WebJSONExpression(expression)))), h)
		})
		if failed(hr) && claim() {
			done(nil, hresultError("ExecuteScript", hr))
		}
	})
}

func (e *embedded) DispatchWebInput(in platform.WebInput, done func(error)) {
	s := e.w
	if s.closed || !s.ready {
		done(platform.ErrUnsupported)
		return
	}
	zoom, _ := comOut[float64](s.controller, ctlGetZoomFactor)
	if !(zoom > 0) || math.IsInf(zoom, 0) {
		zoom = 1
	}
	mouse := func(typ string) map[string]any {
		return map[string]any{"type": typ, "x": in.X / zoom, "y": in.Y / zoom}
	}
	switch in.Kind {
	case platform.WebType:
		e.FocusWeb()
		s.webInputCDP("Input.insertText", map[string]any{"text": in.Text}, done)
	case platform.WebScroll:
		args := mouse("mouseWheel")
		args["deltaX"], args["deltaY"] = in.DeltaX/zoom, in.DeltaY/zoom
		s.webInputCDP("Input.dispatchMouseEvent", args, done)
	case platform.WebClick:
		e.FocusWeb()
		down, up := mouse("mousePressed"), mouse("mouseReleased")
		down["button"], down["buttons"], down["clickCount"] = "left", 1, 1
		up["button"], up["buttons"], up["clickCount"] = "left", 0, 1
		// Enqueue both events while the same document and placement are current.
		remaining := 2
		var firstErr error
		report := func(err error) {
			if firstErr == nil {
				firstErr = err
			}
			remaining--
			if remaining == 0 {
				done(firstErr)
			}
		}
		s.webInputCDP("Input.dispatchMouseEvent", down, report)
		s.webInputCDP("Input.dispatchMouseEvent", up, report)
	}
}

func (s *surface) webInputCDP(method string, args map[string]any, done func(error)) {
	params, err := json.Marshal(args)
	if err != nil {
		done(err)
		return
	}
	s.webCall(done, func(claim func() bool) {
		hr := withHandler(func(hr, _ uintptr) {
			if claim() {
				if failed(hr) {
					done(hresultError(method, hr))
				} else {
					done(nil)
				}
			}
		}, func(h uintptr) uintptr {
			return comCall(s.webview, wvCallDevToolsProtocolMethod, uintptr(unsafe.Pointer(u16(method))), uintptr(unsafe.Pointer(u16(string(params)))), h)
		})
		if failed(hr) && claim() {
			done(hresultError(method, hr))
		}
	})
}
