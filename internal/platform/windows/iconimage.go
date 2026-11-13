package windows

import (
	"bytes"
	"errors"
	"image"
	"image/png"
)

// Icon images for menus (hbmpItem) and the notification area (HICON).
// Win32 wants 32-bit BGRA DIB pixels: menu bitmaps are drawn with
// AlphaBlend and must be premultiplied, icon color bitmaps use straight
// alpha. Rows are top-down (the DIB is created with a negative height).

// iconPixels decodes a PNG and scales it to fit a w×h box, keeping the
// aspect ratio and centering it on transparent pixels. Downscaling
// averages the source pixels each destination pixel covers (box filter);
// upscaling picks the nearest source pixel. The result is w*h*4 bytes of
// BGRA, premultiplied by alpha when premul is set.
func iconPixels(data []byte, w, h int, premul bool) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, errors.New("tango: bad icon size")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return scaleIcon(img, w, h, premul), nil
}

func scaleIcon(img image.Image, w, h int, premul bool) []byte {
	out := make([]byte, w*h*4)
	sb := img.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return out
	}
	// Fit: the scaled image is dw×dh at (ox, oy) inside the box.
	dw, dh := w, h
	if sw*h > sh*w {
		dh = max(1, (sh*w+sw/2)/sw)
	} else {
		dw = max(1, (sw*h+sh/2)/sh)
	}
	ox, oy := (w-dw)/2, (h-dh)/2
	for y := 0; y < dh; y++ {
		y0, y1 := span(y, dh, sh)
		for x := 0; x < dw; x++ {
			x0, x1 := span(x, dw, sw)
			var r, g, b, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					// RGBA returns 16-bit premultiplied components.
					cr, cg, cb, ca := img.At(sb.Min.X+sx, sb.Min.Y+sy).RGBA()
					r, g, b, a = r+uint64(cr), g+uint64(cg), b+uint64(cb), a+uint64(ca)
					n++
				}
			}
			r, g, b, a = r/n>>8, g/n>>8, b/n>>8, a/n>>8
			if !premul && a != 0 {
				r, g, b = min(255, r*255/a), min(255, g*255/a), min(255, b*255/a)
			}
			i := ((oy+y)*w + ox + x) * 4
			out[i], out[i+1], out[i+2], out[i+3] = byte(b), byte(g), byte(r), byte(a)
		}
	}
	return out
}

// span returns the source range [lo, hi) that destination index i of n
// covers in a source of length m; never empty.
func span(i, n, m int) (lo, hi int) {
	lo, hi = i*m/n, (i+1)*m/n
	if hi <= lo {
		hi = lo + 1
	}
	if hi > m {
		lo, hi = m-1, m
	}
	return lo, hi
}

// iconMask builds the monochrome AND mask of an icon from BGRA pixels: a
// set bit (transparent) where alpha is 0. Rows are padded to 16 bits, as
// CreateBitmap requires. Windows uses the color bitmap's alpha for 32-bit
// icons; the mask matters for code that draws without alpha.
func iconMask(pix []byte, w, h int) []byte {
	stride := (w + 15) / 16 * 2
	mask := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if pix[(y*w+x)*4+3] == 0 {
				mask[y*stride+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	return mask
}
