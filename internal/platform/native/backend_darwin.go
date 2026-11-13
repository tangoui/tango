//go:build darwin && !ios

package native

import "github.com/tangoui/tango/internal/platform/darwin"

func init() { newBackend = darwin.New }
