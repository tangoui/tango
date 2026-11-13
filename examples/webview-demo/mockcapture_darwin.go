//go:build darwin && !ios

package main

// Smoke-test helper, not an example of the API: this machine may have no
// camera or microphone, and WebKit only asks the embedder for permission
// when a capture device exists. WebKit's private test switches (mock
// capture devices, mediaDevices without an app bundle) make getUserMedia
// reach the permission callback anywhere.

import (
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango"
)

func enableMockCapture(s *tango.Surface) {
	tango.Await(s.App(), func() struct{} {
		win := objc.ID(s.Native())
		if win == 0 {
			return struct{}{}
		}
		web := findWebView(win.Send(objc.RegisterName("contentView")))
		if web == 0 {
			return struct{}{}
		}
		prefs := web.Send(objc.RegisterName("configuration")).Send(objc.RegisterName("preferences"))
		for _, name := range []string{"_setMockCaptureDevicesEnabled:", "_setMediaDevicesEnabled:"} {
			sel := objc.RegisterName(name)
			if prefs.Send(objc.RegisterName("respondsToSelector:"), sel)&0xff != 0 {
				prefs.Send(sel, 1)
			}
		}
		return struct{}{}
	})
}

func findWebView(v objc.ID) objc.ID {
	if v == 0 {
		return 0
	}
	if v.Send(objc.RegisterName("isKindOfClass:"), objc.ID(objc.GetClass("WKWebView")))&0xff != 0 {
		return v
	}
	subs := v.Send(objc.RegisterName("subviews"))
	n := int(subs.Send(objc.RegisterName("count")))
	for i := range n {
		if w := findWebView(subs.Send(objc.RegisterName("objectAtIndex:"), i)); w != 0 {
			return w
		}
	}
	return 0
}
