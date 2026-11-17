//go:build !tango_guiprobe

package guiprobe

import "github.com/tangoui/tango/ui/input"

const Enabled = false

func SetObserver(func(input.Event)) {}
func Observe(input.Event)           {}
