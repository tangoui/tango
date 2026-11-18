//go:build !darwin

package main

import "time"

// hostNS is wall-clock nanoseconds where the probe (macOS only) is absent.
func hostNS() int64 { return time.Now().UnixNano() }
