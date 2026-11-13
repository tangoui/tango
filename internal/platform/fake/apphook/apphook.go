// Package apphook lets test helpers build a tango.App on an injected Backend
// without the root package exporting a constructor for it.
//
// The root tan package sets NewApp in its init. It is a leaf package (it
// imports only internal/platform) so the root package can import it without
// pulling the fake backend into production binaries.
package apphook

import "github.com/tangoui/tango/internal/platform"

// NewApp builds a *tango.App (returned as any) from a tango.Options (passed as
// any) running on b. Set by package tango; nil until it is linked in.
var NewApp func(b platform.Backend, opts any) any
