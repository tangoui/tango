//go:build darwin && !ios && amd64

package main

import (
	"fmt"
	"image"
)

func nativeSmokeStart(*host) error                { return fmt.Errorf("CoreGraphics hosting requires arm64") }
func nativeSmokeFrame(*image.RGBA, float32) error { return fmt.Errorf("unsupported") }
func nativeSmokeFinish(*host) error               { return nil }
