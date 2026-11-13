package native

import "github.com/tangoui/tango/internal/platform/windows"

func init() { newBackend = windows.New }
