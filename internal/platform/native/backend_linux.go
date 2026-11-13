//go:build linux && !android

package native

import "github.com/tangoui/tango/internal/platform/linux"

func init() { newBackend = linux.New }
