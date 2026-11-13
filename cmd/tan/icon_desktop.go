package main

// Desktop icons. The master icon (tan.json "icon") is full-bleed square
// artwork; desktop systems expect the icon to carry its own shape. tan
// derives it once, on the macOS grid, and Windows and Linux use the same
// shape unless tan.json "icons" names a file of their own:
//
//	1024×1024 transparent canvas
//	824×824 rounded square, centred (100 px margin)
//	corners with continuous curvature, sized like a 185 px radius
//	a soft drop shadow below the square

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	desktopCanvas = 1024  // the derived icon's size
	desktopSquare = 824   // the rounded square inside it
	desktopRadius = 185.4 // the circular corner radius the corners stand in for

	// The corners are superellipse arcs |u|^p + |v|^p = R^p, which meet
	// the straight edges with zero curvature (continuous curvature, the
	// look of Apple's icon shape) as long as p > 2. They start
	// desktopCornerSpan×radius from the corner and are as deep at 45° as a
	// circular arc of desktopRadius: R(1 - 2^(-1/p)) = radius(1 - 1/√2).
	desktopCornerSpan = 1.528

	desktopShadowOffset = 10   // px down
	desktopShadowSigma  = 12   // px of blur
	desktopShadowAlpha  = 0.30 // opacity of the shadow under the opaque square
)

// desktopIcon derives the desktop icon from the master icon: the master
// scaled into the rounded square, anti-aliased against the transparent
// canvas, over a soft shadow.
func desktopIcon(master image.Image) *image.NRGBA {
	const n = desktopCanvas
	lo := float64(n-desktopSquare) / 2
	art := scaleIcon(master, desktopSquare)
	cover := squircleCoverage(n, lo, lo+desktopSquare, desktopRadius)

	shadow := make([]float32, n*n)
	for y := desktopShadowOffset; y < n; y++ {
		copy(shadow[y*n:(y+1)*n], cover[(y-desktopShadowOffset)*n:])
	}
	gaussianBlur(shadow, n, n, desktopShadowSigma)

	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	off := int(lo)
	for y := range n {
		for x := range n {
			i := y*n + x
			sa := float64(shadow[i]) * desktopShadowAlpha
			var r, g, b, ca float64 // the artwork, premultiplied
			if c := float64(cover[i]); c > 0 {
				ax, ay := x-off, y-off
				if ax >= 0 && ay >= 0 && ax < desktopSquare && ay < desktopSquare {
					p := art.Pix[ay*art.Stride+ax*4:]
					ca = c * float64(p[3]) / 255
					r, g, b = float64(p[0])*ca, float64(p[1])*ca, float64(p[2])*ca
				}
			}
			// The artwork over the (black) shadow.
			a := ca + sa*(1-ca)
			if a <= 0 {
				continue
			}
			o := out.Pix[y*out.Stride+x*4:]
			o[0], o[1], o[2] = clampByte(r/a), clampByte(g/a), clampByte(b/a)
			o[3] = clampByte(a * 255)
		}
	}
	return out
}

// squircleCoverage returns, for each pixel of an n×n canvas, how much of
// it the rounded square [lo, hi]² covers (0…1).
func squircleCoverage(n int, lo, hi, radius float64) []float32 {
	span := desktopCornerSpan * radius
	p := -math.Ln2 / math.Log(1-radius*(1-1/math.Sqrt2)/span)
	cover := make([]float32, n*n)
	// The distance a coordinate reaches into the corner zone, toward the
	// outside; 0 along the straight part of an edge.
	into := func(v float64) float64 { return max(lo+span-v, v-(hi-span), 0) }
	for y := range n {
		fy := float64(y) + 0.5
		uy := into(fy)
		for x := range n {
			fx := float64(x) + 0.5
			ux := into(fx)
			// q is the superellipse "radius" of the point; the boundary is
			// q = span. (q - span)/|∇q| is the signed distance to it, to
			// first order, which is what the anti-aliasing needs.
			var d float64
			switch {
			case ux == 0 && uy == 0:
				d = -span
			case uy == 0:
				d = ux - span
			case ux == 0:
				d = uy - span
			default:
				q := math.Pow(math.Pow(ux, p)+math.Pow(uy, p), 1/p)
				gx, gy := math.Pow(ux/q, p-1), math.Pow(uy/q, p-1)
				d = (q - span) / math.Hypot(gx, gy)
			}
			if c := 0.5 - d; c > 0 {
				cover[y*n+x] = float32(min(c, 1))
			}
		}
	}
	return cover
}

// gaussianBlur blurs a w×h plane in place, approximating a Gaussian of
// standard deviation sigma with three box blurs in each direction.
func gaussianBlur(v []float32, w, h int, sigma float64) {
	r := int(math.Round((math.Sqrt(4*sigma*sigma+1) - 1) / 2)) // box width 2r+1
	if r < 1 {
		return
	}
	line := make([]float32, max(w, h))
	for range 3 {
		for y := range h {
			boxBlur(v[y*w:], 1, w, r, line)
		}
		for x := range w {
			boxBlur(v[x:], w, h, r, line)
		}
	}
}

// boxBlur averages n samples spaced stride apart over a 2r+1 window,
// treating samples beyond the ends as 0.
func boxBlur(v []float32, stride, n, r int, line []float32) {
	for i := range n {
		line[i] = v[i*stride]
	}
	var sum float32
	for i := 0; i < r && i < n; i++ {
		sum += line[i]
	}
	k := 1 / float32(2*r+1)
	for i := range n {
		if j := i + r; j < n {
			sum += line[j]
		}
		if j := i - r - 1; j >= 0 {
			sum -= line[j]
		}
		v[i*stride] = max(sum*k, 0)
	}
}

// scaleIcon resizes src to n×n, on premultiplied alpha so transparent
// pixels do not darken edges. Shrinking averages the exact area each output pixel
// covers, which keeps fine lines and avoids moiré; enlarging interpolates
// linearly.
func scaleIcon(src image.Image, n int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, n, n))
	if sw == 0 || sh == 0 || n <= 0 {
		return dst
	}
	pm := image.NewRGBA64(image.Rect(0, 0, sw, sh))
	draw.Draw(pm, pm.Bounds(), src, b.Min, draw.Src) // premultiplies
	xs, ys := resampleTaps(sw, n), resampleTaps(sh, n)

	tmp := make([]float32, n*sh*4) // horizontal pass: n columns × sh rows
	for y := range sh {
		row := pm.Pix[y*pm.Stride:]
		for x, taps := range xs {
			var acc [4]float32
			for _, t := range taps {
				p := row[t.i*8:]
				for c := range 4 {
					acc[c] += t.w * float32(uint16(p[2*c])<<8|uint16(p[2*c+1]))
				}
			}
			copy(tmp[(y*n+x)*4:], acc[:])
		}
	}
	for y, taps := range ys {
		for x := range n {
			var acc [4]float32
			for _, t := range taps {
				p := tmp[(t.i*n+x)*4:]
				for c := range 4 {
					acc[c] += t.w * p[c]
				}
			}
			o := dst.Pix[y*dst.Stride+x*4:]
			a := acc[3]
			if a <= 0 {
				continue
			}
			for c := range 3 {
				o[c] = clampByte(float64(acc[c] / a * 255))
			}
			o[3] = clampByte(float64(a / 65535 * 255))
		}
	}
	return dst
}

type resampleTap struct {
	i int
	w float32
}

// resampleTaps returns, for each of n output samples, the source samples
// (of m) that make it and their weights (summing to 1).
func resampleTaps(m, n int) [][]resampleTap {
	out := make([][]resampleTap, n)
	scale := float64(m) / float64(n)
	for d := range n {
		var taps []resampleTap
		if scale >= 1 {
			// Area: the share of [lo, hi) each source sample covers.
			lo, hi := float64(d)*scale, float64(d+1)*scale
			for s := int(lo); s < m && float64(s) < hi; s++ {
				if c := min(hi, float64(s+1)) - max(lo, float64(s)); c > 1e-9 {
					taps = append(taps, resampleTap{s, float32(c / scale)})
				}
			}
		} else {
			// Linear between the two nearest source centres.
			c := (float64(d)+0.5)*scale - 0.5
			s0 := int(math.Floor(c))
			f := c - float64(s0)
			s1 := s0 + 1
			s0, s1 = min(max(s0, 0), m-1), min(max(s1, 0), m-1)
			taps = append(taps, resampleTap{s0, float32(1 - f)}, resampleTap{s1, float32(f)})
		}
		out[d] = taps
	}
	return out
}

func pngBytes(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func clampByte(v float64) uint8 { return uint8(min(max(math.Round(v), 0), 255)) }

// decodeDesktopPNG reads an icon file named in tan.json.
func decodeDesktopPNG(cfg *Config, key, rel string) (image.Image, error) {
	f, err := os.Open(cfg.Path(rel))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", key, rel, err)
	}
	return img, nil
}

// desktopIconImage returns the icon a desktop platform shows: override, a
// file already in the desktop shape, used as is; otherwise the shape
// derived from the master icon. Nil when the project has neither.
func desktopIconImage(cfg *Config, key, override string) (image.Image, error) {
	if override != "" {
		return decodeDesktopPNG(cfg, `"icons.`+key+`"`, override)
	}
	master, err := loadIcon(cfg)
	if err != nil || master == nil {
		return nil, err
	}
	return desktopIcon(master), nil
}

// macOSIconFile returns the 1024 PNG the app's .icns is made from:
// "icons.macos" as is, or the derived shape written to a temporary file
// that cleanup removes. "" when the project has no icon.
func macOSIconFile(cfg *Config) (path string, cleanup func(), err error) {
	cleanup = func() {}
	if cfg.Icons.MacOS != "" {
		return cfg.Path(cfg.Icons.MacOS), cleanup, nil
	}
	img, err := desktopIconImage(cfg, "macos", "")
	if err != nil || img == nil {
		return "", cleanup, err
	}
	tmp, err := os.MkdirTemp("", "tango-macicon-*")
	if err != nil {
		return "", cleanup, err
	}
	path = filepath.Join(tmp, "icon.png")
	if err := writePNG(path, img); err != nil {
		os.RemoveAll(tmp)
		return "", cleanup, err
	}
	return path, func() { os.RemoveAll(tmp) }, nil
}

// linuxIconSizes are the hicolor theme sizes tan installs.
var linuxIconSizes = []int{16, 22, 24, 32, 48, 64, 128, 256, 512}

// linuxIcons renders the app icon ("icons.linux.app" or the derived shape)
// at every hicolor size, keyed by size. Nil when the project has no icon.
func linuxIcons(cfg *Config) (map[int][]byte, error) {
	img, err := desktopIconImage(cfg, "linux.app", cfg.Icons.Linux.App)
	if err != nil || img == nil {
		return nil, err
	}
	out := make(map[int][]byte, len(linuxIconSizes))
	for _, n := range linuxIconSizes {
		b, err := pngBytes(scaleIcon(img, n))
		if err != nil {
			return nil, err
		}
		out[n] = b
	}
	return out, nil
}

// hicolorPath is where the icon of the given size goes under share/.
func hicolorPath(exe string, size int) string {
	return fmt.Sprintf("icons/hicolor/%dx%d/apps/%s.png", size, size, exe)
}

// hicolorSymbolicPath is where "icons.linux.symbolic" goes under share/.
func hicolorSymbolicPath(exe string) string {
	return "icons/hicolor/scalable/apps/" + exe + "-symbolic.svg"
}

// linuxIconTree returns the freedesktop icons keyed by their path under
// share/: the app icon in the hicolor theme at every linuxIconSizes size,
// and "icons.linux.symbolic" as scalable/apps/<exe>-symbolic.svg. Empty
// when the project has no icon.
func linuxIconTree(cfg *Config) (map[string][]byte, error) {
	exe := cfg.Exe()
	sized, err := linuxIcons(cfg)
	if err != nil {
		return nil, err
	}
	tree := make(map[string][]byte)
	for n, b := range sized {
		tree[hicolorPath(exe, n)] = b
	}
	if p := cfg.Icons.Linux.Symbolic; p != "" {
		b, err := os.ReadFile(cfg.Path(p))
		if err != nil {
			return nil, fmt.Errorf(`"icons.linux.symbolic": %w`, err)
		}
		tree[hicolorSymbolicPath(exe)] = b
	}
	return tree, nil
}
