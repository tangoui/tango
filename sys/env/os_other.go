//go:build !darwin && !linux && !windows

package env

import "runtime"

func osVersion() (name, version string) { return runtime.GOOS, "" }
