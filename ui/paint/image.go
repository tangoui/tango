package paint

import (
	"bufio"
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
)

// ErrImageFormat is returned by DecodeImage for data that is neither PNG
// nor JPEG.
var ErrImageFormat = errors.New("paint: unsupported image format (want PNG or JPEG)")

// DecodeImage decodes a PNG or JPEG stream (sniffed from its signature)
// with the standard library decoders.
func DecodeImage(r io.Reader) (image.Image, error) {
	br := bufio.NewReader(r)
	head, _ := br.Peek(8)
	switch {
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return png.Decode(br)
	case bytes.HasPrefix(head, []byte{0xff, 0xd8, 0xff}):
		return jpeg.Decode(br)
	}
	return nil, ErrImageFormat
}

// ToRGBA returns src as a premultiplied *image.RGBA anchored at 0,0. An
// *image.RGBA that already starts at the origin is returned as is (not
// copied); interpolating premultiplied values avoids dark alpha fringes.
func ToRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	if m, ok := src.(*image.RGBA); ok && b.Min == (image.Point{}) {
		return m
	}
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// ResizeBilinear scales src to w×h with bilinear interpolation. Large
// reductions first halve the source with a 2×2 box filter so that every
// source pixel contributes (no aliasing).
func ResizeBilinear(src image.Image, w, h int) *image.RGBA {
	s := ToRGBA(src)
	b := s.Bounds()
	return Resample(s, 0, 0, float64(b.Dx()), float64(b.Dy()), w, h)
}

// Resample maps the source rectangle (sx, sy, sw, sh), in pixels of src's
// bounds relative to its top-left corner, onto a new w×h image with
// bilinear filtering (pixel centres aligned, edges clamped, large
// reductions prefiltered like ResizeBilinear). It crops and scales in one
// pass, e.g. for "cover" image fitting.
func Resample(src image.Image, sx, sy, sw, sh float64, w, h int) *image.RGBA {
	s := ToRGBA(src)
	w, h = max(1, w), max(1, h)
	for sw/float64(w) > 2 && sh/float64(h) > 2 && s.Bounds().Dx() > 1 && s.Bounds().Dy() > 1 {
		s = halve(s)
		sx, sy, sw, sh = sx/2, sy/2, sw/2, sh/2
	}
	srcW, srcH := s.Bounds().Dx(), s.Bounds().Dy()
	d := image.NewRGBA(image.Rect(0, 0, w, h))
	kx, ky := sw/float64(w), sh/float64(h)
	xi := make([][2]int, w)
	xf := make([]uint32, w)
	for x := 0; x < w; x++ {
		fx := sx + (float64(x)+0.5)*kx - 0.5
		fx = math.Max(0, math.Min(fx, float64(srcW-1)))
		x0 := int(fx)
		xi[x] = [2]int{x0 * 4, min(x0+1, srcW-1) * 4}
		xf[x] = uint32((fx - float64(x0)) * 256)
	}
	for y := 0; y < h; y++ {
		fy := sy + (float64(y)+0.5)*ky - 0.5
		fy = math.Max(0, math.Min(fy, float64(srcH-1)))
		y0 := int(fy)
		y1 := min(y0+1, srcH-1)
		wy := uint32((fy - float64(y0)) * 256)
		r0 := s.Pix[y0*s.Stride:]
		r1 := s.Pix[y1*s.Stride:]
		out := d.Pix[y*d.Stride:]
		for x := 0; x < w; x++ {
			a, b := xi[x][0], xi[x][1]
			wx := xf[x]
			for c := 0; c < 4; c++ {
				top := uint32(r0[a+c])*(256-wx) + uint32(r0[b+c])*wx
				bot := uint32(r1[a+c])*(256-wx) + uint32(r1[b+c])*wx
				out[x*4+c] = uint8((top*(256-wy) + bot*wy + 32768) >> 16)
			}
		}
	}
	return d
}

// halve downsamples by two with a box filter.
func halve(s *image.RGBA) *image.RGBA {
	sw, sh := s.Bounds().Dx(), s.Bounds().Dy()
	w, h := max(1, sw/2), max(1, sh/2)
	d := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := min(2*y, sh-1), min(2*y+1, sh-1)
		for x := 0; x < w; x++ {
			x0, x1 := min(2*x, sw-1), min(2*x+1, sw-1)
			for c := 0; c < 4; c++ {
				v := int(s.Pix[y0*s.Stride+x0*4+c]) + int(s.Pix[y0*s.Stride+x1*4+c]) +
					int(s.Pix[y1*s.Stride+x0*4+c]) + int(s.Pix[y1*s.Stride+x1*4+c])
				d.Pix[y*d.Stride+x*4+c] = uint8((v + 2) / 4)
			}
		}
	}
	return d
}

// Orient mirrors src (flipX horizontally, flipY vertically) and then
// rotates it by quarter turns clockwise (negative = counter-clockwise),
// losslessly. With nothing to do it returns ToRGBA(src).
func Orient(src image.Image, quarter int, flipX, flipY bool) *image.RGBA {
	s := ToRGBA(src)
	quarter = ((quarter % 4) + 4) % 4
	if quarter == 0 && !flipX && !flipY {
		return s
	}
	sw, sh := s.Bounds().Dx(), s.Bounds().Dy()
	dw, dh := sw, sh
	if quarter%2 == 1 {
		dw, dh = sh, sw
	}
	d := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < sh; y++ {
		for x := 0; x < sw; x++ {
			fx, fy := x, y
			if flipX {
				fx = sw - 1 - x
			}
			if flipY {
				fy = sh - 1 - y
			}
			var tx, ty int
			switch quarter {
			case 0:
				tx, ty = fx, fy
			case 1:
				tx, ty = sh-1-fy, fx
			case 2:
				tx, ty = sw-1-fx, sh-1-fy
			case 3:
				tx, ty = fy, sw-1-fx
			}
			copy(d.Pix[ty*d.Stride+tx*4:ty*d.Stride+tx*4+4], s.Pix[y*s.Stride+x*4:y*s.Stride+x*4+4])
		}
	}
	return d
}
