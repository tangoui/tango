package windows

// Conversions between logical pixels (DIPs, 96 per inch, the contract's
// unit) and physical pixels at a given DPI. Both round to nearest.

const baseDPI = 96

func normDPI(dpi int) int {
	if dpi <= 0 {
		return baseDPI
	}
	return dpi
}

// dipToPx converts logical pixels to physical pixels.
func dipToPx(v, dpi int) int {
	dpi = normDPI(dpi)
	if v < 0 {
		return -dipToPx(-v, dpi)
	}
	return (v*dpi + baseDPI/2) / baseDPI
}

// pxToDIP converts physical pixels to logical pixels.
func pxToDIP(v, dpi int) int {
	dpi = normDPI(dpi)
	if v < 0 {
		return -pxToDIP(-v, dpi)
	}
	return (v*baseDPI + dpi/2) / dpi
}

// defaultSurfaceSize applies the contract's 1024x768 default to zero sizes.
func defaultSurfaceSize(w, h int) (int, int) {
	if w <= 0 {
		w = 1024
	}
	if h <= 0 {
		h = 768
	}
	return w, h
}

// centerIn returns the top-left corner that centers a w×h box in the
// rectangle (left, top, right, bottom), clamped so the top-left stays
// inside it when the box is larger.
func centerIn(left, top, right, bottom, w, h int) (x, y int) {
	x = left + (right-left-w)/2
	y = top + (bottom-top-h)/2
	if x < left {
		x = left
	}
	if y < top {
		y = top
	}
	return x, y
}
