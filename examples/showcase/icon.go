package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// trayIcon draws the tray icon at run time (44x44, for 2x menu bars) so the
// example needs no binary file: a ring with a dot, black on transparent,
// which macOS treats as a template image and tints for the menu bar.
func trayIcon() []byte {
	const n = 44
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	c := float64(n-1) / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := math.Hypot(float64(x)-c, float64(y)-c)
			// Coverage of the ring (radius 15..19) and the dot (radius 7),
			// with one pixel of anti-aliasing.
			a := math.Max(band(d, 15, 19), band(d, -1, 7))
			if a > 0 {
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(a * 255)})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// band is 1 inside [lo, hi] and fades out over one pixel on both sides.
func band(d, lo, hi float64) float64 {
	return math.Max(0, math.Min(1, math.Min(d-lo+0.5, hi-d+0.5)))
}
