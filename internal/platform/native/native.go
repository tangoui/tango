// Package native picks the platform backend for the GOOS being built and
// holds the per-platform main-thread rules the core follows. Package tango
// reaches the platform backends only through this package.
//
// It imports only internal/platform and the backend packages, never the
// root package.
package native

import "github.com/tangoui/tango/internal/platform"

// newBackend constructs the platform backend NewBackend returns. Each
// backend registers itself from a GOOS-specific file of this package, e.g.
//
//	// backend_darwin.go
//	package native
//	import "github.com/tangoui/tango/internal/platform/darwin"
//	func init() { newBackend = darwin.New }
//
// Nil on platforms without a backend.
var newBackend func() platform.Backend

// NewBackend returns a new backend of the platform being built, or nil on
// platforms without one (App.Run then returns ErrNoBackend).
func NewBackend() platform.Backend {
	if newBackend == nil {
		return nil
	}
	return newBackend()
}
