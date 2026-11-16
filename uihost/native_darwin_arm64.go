//go:build darwin && !ios && arm64

package uihost

import "github.com/tangoui/tango/internal/render/quartz"

var platformNativePresenter presenterFunc = quartz.NewPresenter
