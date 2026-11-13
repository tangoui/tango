package windows

// Platform-independent pieces of the desktop integration (displays, theme,
// taskbar badge), tested on every OS.

// accentFromDWM converts HKCU\Software\Microsoft\Windows\DWM\AccentColor
// (0xAABBGGRR) to the contract's 0xRRGGBBAA. A zero alpha is taken as
// opaque: the value is a color, not a blend.
func accentFromDWM(v uint32) uint32 {
	r, g, b, a := v&0xff, v>>8&0xff, v>>16&0xff, v>>24
	if a == 0 {
		a = 0xff
	}
	return r<<24 | g<<16 | b<<8 | a
}

// accentFromColorization converts DwmGetColorizationColor's 0xAARRGGBB.
func accentFromColorization(v uint32) uint32 {
	return v<<8 | 0xff
}

// badgeText is what fits on a taskbar overlay: up to two characters of a
// count; larger counts show as "9+" (one digit and the plus sign).
func badgeText(s string) string {
	digits := s
	plus := false
	if n := len(digits); n > 0 && digits[n-1] == '+' {
		digits, plus = digits[:n-1], true
	}
	if len(digits) > 2 || (plus && len(digits) > 1) {
		return "9+"
	}
	if plus {
		return digits + "+"
	}
	return digits
}

// badgeGlyphs is a 3×5 pixel font for digits and '+'; each row's low three
// bits are the pixels, most significant bit leftmost.
var badgeGlyphs = map[byte][5]uint8{
	'0': {7, 5, 5, 5, 7},
	'1': {2, 6, 2, 2, 7},
	'2': {7, 1, 7, 4, 7},
	'3': {7, 1, 7, 1, 7},
	'4': {5, 5, 7, 1, 1},
	'5': {7, 4, 7, 1, 7},
	'6': {7, 4, 7, 5, 7},
	'7': {7, 1, 2, 2, 2},
	'8': {7, 5, 7, 5, 7},
	'9': {7, 5, 7, 1, 7},
	'+': {0, 2, 7, 2, 0},
}

// badgePixels draws text (digits and '+') in white on a red disc, size×size
// BGRA pixels with straight alpha, for an overlay icon. Unknown characters
// are skipped.
func badgePixels(text string, size int) []byte {
	pix := make([]byte, size*size*4)
	// Disc with 4×4 supersampled edges.
	c := float64(size) / 2
	r2 := c * c
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			in := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					dx := float64(x) + (float64(sx)+0.5)/4 - c
					dy := float64(y) + (float64(sy)+0.5)/4 - c
					if dx*dx+dy*dy <= r2 {
						in++
					}
				}
			}
			if in == 0 {
				continue
			}
			i := (y*size + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = 0x23, 0x11, 0xe8, byte(in*255/16) // #e81123
		}
	}
	var glyphs [][5]uint8
	for i := 0; i < len(text); i++ {
		if g, ok := badgeGlyphs[text[i]]; ok {
			glyphs = append(glyphs, g)
		}
	}
	if len(glyphs) == 0 {
		return pix
	}
	// Scale the 3×5 glyphs (1 px apart) to fit the disc's inner square.
	inner := size * 7 / 10
	units := len(glyphs)*4 - 1
	scale := min(inner/units, inner/5)
	if scale < 1 {
		scale = 1
	}
	w, h := units*scale, 5*scale
	ox, oy := (size-w)/2, (size-h)/2
	for gi, g := range glyphs {
		for row := 0; row < 5; row++ {
			for col := 0; col < 3; col++ {
				if g[row]&(4>>col) == 0 {
					continue
				}
				for py := 0; py < scale; py++ {
					for px := 0; px < scale; px++ {
						x := ox + (gi*4+col)*scale + px
						y := oy + row*scale + py
						if x < 0 || y < 0 || x >= size || y >= size {
							continue
						}
						i := (y*size + x) * 4
						pix[i], pix[i+1], pix[i+2], pix[i+3] = 0xff, 0xff, 0xff, 0xff
					}
				}
			}
		}
	}
	return pix
}
