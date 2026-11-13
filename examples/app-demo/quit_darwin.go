package main

import (
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango"
)

// userQuit sends [NSApp terminate:nil], exactly what Cmd+Q and the Quit
// menu item do.
func userQuit(app *tango.App) bool {
	app.Post(func() {
		nsApp := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
		nsApp.Send(objc.RegisterName("terminate:"), 0)
	})
	return true
}

// appKitEvents calls the app delegate the way AppKit does for a deep link,
// a Finder "Open With" and a Dock click with no windows open.
func appKitEvents(app *tango.App, file string) bool {
	app.Post(func() {
		sel := objc.RegisterName
		nsApp := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
		del := nsApp.Send(sel("delegate"))
		str := func(s string) objc.ID {
			return objc.ID(objc.GetClass("NSString")).Send(sel("stringWithUTF8String:"), s+"\x00")
		}
		u := objc.ID(objc.GetClass("NSURL")).Send(sel("URLWithString:"), str("demo://from-appkit"))
		f := objc.ID(objc.GetClass("NSURL")).Send(sel("fileURLWithPath:"), str(file))
		arr := objc.ID(objc.GetClass("NSMutableArray")).Send(sel("array"))
		arr.Send(sel("addObject:"), u)
		arr.Send(sel("addObject:"), f)
		del.Send(sel("application:openURLs:"), nsApp, arr)
		del.Send(sel("applicationShouldHandleReopen:hasVisibleWindows:"), nsApp, false)
	})
	return true
}
