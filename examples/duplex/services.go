package main

import (
	"github.com/tangoui/tango/examples/duplex/services"
	"github.com/tangoui/tango/ipc"
)

// registerServices registers every service of the example on r. It is the
// single place they are registered, shared by the WebView page and any Go
// panel using the same Router: the platform-independent services of
// package services ("Echo", "Upload", "Sink", "Source", "Bench.Report")
// plus "Term" (duplex), a local shell over byte frames (term.go).
func registerServices(r *ipc.Router) {
	services.Register(r)
	r.HandleDuplex("Term", term)
}
