package shaders

import "image"

// grayImage returns a w×h gray image with the given levels in row order.
func grayImage(w, h int, levels ...uint8) *image.Gray {
	m := image.NewGray(image.Rect(0, 0, w, h))
	copy(m.Pix, levels)
	return m
}
