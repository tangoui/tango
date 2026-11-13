// Package devfeat holds the core's development features: window placement
// across `tan dev` restarts (windowstate.go) and the whole-app automation
// channel (automation.go, docs/automation.md). They are only built with
// -tags tango_dev; production builds, the default, get the empty
// implementations of release.go.
//
// Package tango calls AttachWindowState and AttachAutomation for every new
// App. This package cannot import package tango (tango imports it): it
// uses the App's exported methods through the App interface and reaches
// core internals through the hooks of package internal/corehook, which
// package tango sets.
package devfeat

import "github.com/tangoui/tango/internal/platform"

// App is the part of *tango.App's exported API the development features
// use. The value passed to the Attach functions must be a *tango.App: the
// corehook hooks type-assert it.
type App interface {
	OnReady(fn func())
	OnLifecycle(fn func(platform.LifecycleEvent))
	Backend() platform.Backend
	Quit()
}
