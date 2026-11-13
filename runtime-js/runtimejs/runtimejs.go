// Package runtimejs embeds the IIFE build of @tangoui/runtime. Inject it into
// every surface with app.AddBridgeScript(runtimejs.Script); pages then use
// window.tango (or import "@tangoui/runtime", which shares the same client).
//
// tango.iife.js is generated: run `npm run build` in runtime-js/ after
// changing runtime-js/src and commit the result.
package runtimejs

import _ "embed"

//go:embed tango.iife.js
var Script string
