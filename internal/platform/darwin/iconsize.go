package darwin

import (
	"bytes"
	"image"
	_ "image/png" // PNG headers for iconPoints
)

// iconPoints returns the size in points to show a PNG icon at the given
// height, keeping its aspect ratio. ok is false if png is not a PNG.
// Pure Go (no build tag) so it is tested on every platform.
func iconPoints(png []byte, height float64) (w, h float64, ok bool) {
	if len(png) == 0 || height <= 0 {
		return 0, 0, false
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil || format != "png" || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return height * float64(cfg.Width) / float64(cfg.Height), height, true
}
