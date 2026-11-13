//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/tangoui/tango/ui/text"
	"golang.org/x/image/font/gofont/gomono"
)

// Code editor iframes register an embedded monospace face before the other
// app fonts. Other component previews retain their existing typography.
func init() {
	if js.Global().Get("tangoUseMonoFont").Truthy() {
		if err := text.RegisterFont("Go Mono Preview", gomono.TTF); err != nil {
			panic(err)
		}
	}
}
