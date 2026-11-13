package windows

// Pure logic for the round-2 system integration (no build tag, tested on
// every host): global hotkey mapping, DIB <-> PNG conversion for the
// clipboard and the NOTIFYICONDATAW layout used for balloon notifications.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"unicode/utf16"
)

// RegisterHotKey modifiers (winuser.h).
const (
	hkModAlt      = 0x0001
	hkModControl  = 0x0002
	hkModShift    = 0x0004
	hkModWin      = 0x0008
	hkModNoRepeat = 0x4000
)

// hotkeyPlan maps an accelerator to RegisterHotKey's modifiers and virtual
// key. Unlike menu accelerators, global hotkeys may use the Windows key
// (Super/Meta/Win; Cmd means Ctrl, as CmdOrCtrl does).
func hotkeyPlan(acc string) (mods uint32, vk uint16, err error) {
	s := strings.TrimSpace(acc)
	var key string
	switch {
	case s == "+":
		key, s = "Plus", ""
	case strings.HasSuffix(s, "++"):
		key, s = "Plus", strings.TrimSuffix(s, "++")
	default:
		i := strings.LastIndex(s, "+")
		key, s = s[i+1:], s[:max(i, 0)]
	}
	var rest []string
	if s != "" {
		for _, p := range strings.Split(s, "+") {
			switch strings.ToLower(strings.TrimSpace(p)) {
			case "super", "meta", "win", "windows":
				mods |= hkModWin
				continue
			}
			rest = append(rest, p)
		}
	}
	s = strings.Join(append(rest, key), "+")
	a, err := parseAccelerator(s)
	if err != nil {
		return 0, 0, fmt.Errorf("tango: hotkey %q: %w", acc, err)
	}
	if a.Mods&modCtrl != 0 {
		mods |= hkModControl
	}
	if a.Mods&modShift != 0 {
		mods |= hkModShift
	}
	if a.Mods&modAlt != 0 {
		mods |= hkModAlt
	}
	return mods | hkModNoRepeat, a.VK, nil
}

// ---- DIB <-> PNG ----

const (
	biRGB       = 0
	biBitfields = 3
)

var errBadDIB = errors.New("tango: unsupported clipboard bitmap")

// dibToPNG converts a packed DIB (CF_DIB / CF_DIBV5 contents: a
// BITMAPINFOHEADER, V4 or V5 header, optional masks / palette, then the
// pixels) to PNG. 1/4/8-bit palette, 16, 24 and 32-bit images are
// supported.
func dibToPNG(dib []byte) ([]byte, error) {
	img, err := decodeDIB(dib)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeDIB(dib []byte) (image.Image, error) {
	le := binary.LittleEndian
	if len(dib) < 40 {
		return nil, errBadDIB
	}
	hsize := int(le.Uint32(dib[0:]))
	if hsize < 40 || hsize > len(dib) {
		return nil, errBadDIB
	}
	w := int(int32(le.Uint32(dib[4:])))
	h := int(int32(le.Uint32(dib[8:])))
	bpp := int(le.Uint16(dib[14:]))
	comp := le.Uint32(dib[16:])
	clrUsed := int(le.Uint32(dib[32:]))
	topDown := h < 0
	if topDown {
		h = -h
	}
	if w <= 0 || h <= 0 || w > 1<<15 || h > 1<<15 {
		return nil, errBadDIB
	}
	off := hsize
	var rm, gm, bm, am uint32
	switch {
	case comp == biBitfields && (bpp == 16 || bpp == 32):
		if hsize >= 52 {
			rm, gm, bm = le.Uint32(dib[40:]), le.Uint32(dib[44:]), le.Uint32(dib[48:])
			if hsize >= 56 {
				am = le.Uint32(dib[52:])
			}
		} else {
			if len(dib) < off+12 {
				return nil, errBadDIB
			}
			rm, gm, bm = le.Uint32(dib[off:]), le.Uint32(dib[off+4:]), le.Uint32(dib[off+8:])
			off += 12
		}
	case comp == biRGB && bpp == 16:
		rm, gm, bm = 0x7C00, 0x03E0, 0x001F
	case comp == biRGB && bpp == 32:
		rm, gm, bm, am = 0x00FF0000, 0x0000FF00, 0x000000FF, 0xFF000000
	case comp == biRGB && (bpp == 24 || bpp == 8 || bpp == 4 || bpp == 1):
	default:
		return nil, fmt.Errorf("%w (compression %d, %d bpp)", errBadDIB, comp, bpp)
	}
	var palette []color.NRGBA
	if bpp <= 8 {
		n := clrUsed
		if n == 0 {
			n = 1 << bpp
		}
		if len(dib) < off+4*n {
			return nil, errBadDIB
		}
		for i := 0; i < n; i++ {
			p := dib[off+4*i:]
			palette = append(palette, color.NRGBA{p[2], p[1], p[0], 0xFF})
		}
		off += 4 * n
	}
	stride := ((w*bpp + 31) / 32) * 4
	if len(dib) < off+stride*h {
		return nil, errBadDIB
	}
	px := dib[off:]
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for y := 0; y < h; y++ {
		row := px[stride*y:]
		dy := h - 1 - y
		if topDown {
			dy = y
		}
		for x := 0; x < w; x++ {
			var c color.NRGBA
			switch bpp {
			case 1, 4, 8:
				bit := x * bpp
				idx := int(row[bit/8]>>(8-bpp-bit%8)) & (1<<bpp - 1)
				if idx < len(palette) {
					c = palette[idx]
				}
			case 24:
				p := row[3*x:]
				c = color.NRGBA{p[2], p[1], p[0], 0xFF}
			case 16:
				v := uint32(le.Uint16(row[2*x:]))
				c = color.NRGBA{maskByte(v, rm), maskByte(v, gm), maskByte(v, bm), 0xFF}
			case 32:
				v := le.Uint32(row[4*x:])
				c = color.NRGBA{maskByte(v, rm), maskByte(v, gm), maskByte(v, bm), 0xFF}
				if am != 0 {
					c.A = maskByte(v, am)
					if c.A != 0 {
						anyAlpha = true
					}
				}
			}
			img.SetNRGBA(x, dy, c)
		}
	}
	// 32-bit DIBs from most apps leave the alpha byte 0: treat an all-zero
	// alpha channel as opaque rather than as a fully transparent image.
	if bpp == 32 && am != 0 && !anyAlpha {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 0xFF
		}
	}
	return img, nil
}

// maskByte extracts the channel selected by mask from v, scaled to 8 bits.
func maskByte(v, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := 0
	for mask&1 == 0 {
		mask >>= 1
		shift++
	}
	bits := 0
	for m := mask; m&1 == 1; m >>= 1 {
		bits++
	}
	x := (v >> shift) & mask
	if bits >= 8 {
		return uint8(x >> (bits - 8))
	}
	max := uint32(1)<<bits - 1
	return uint8(x * 255 / max)
}

// pngToDIBV5 decodes a PNG and returns it as a packed CF_DIBV5: a
// 124-byte BITMAPV5HEADER (32 bpp, BI_BITFIELDS with an alpha mask, sRGB)
// followed by bottom-up BGRA rows with straight alpha. Windows synthesizes
// CF_DIB and CF_BITMAP from it.
func pngToDIBV5(data []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	img := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(img, img.Bounds(), src, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	const hsize = 124
	out := make([]byte, hsize+4*w*h)
	le := binary.LittleEndian
	le.PutUint32(out[0:], hsize)
	le.PutUint32(out[4:], uint32(int32(w)))
	le.PutUint32(out[8:], uint32(int32(h))) // positive: bottom-up
	le.PutUint16(out[12:], 1)               // planes
	le.PutUint16(out[14:], 32)              // bit count
	le.PutUint32(out[16:], biBitfields)
	le.PutUint32(out[20:], uint32(4*w*h))
	le.PutUint32(out[40:], 0x00FF0000) // red mask
	le.PutUint32(out[44:], 0x0000FF00) // green
	le.PutUint32(out[48:], 0x000000FF) // blue
	le.PutUint32(out[52:], 0xFF000000) // alpha
	le.PutUint32(out[56:], 0x73524742) // LCS_sRGB 'sRGB'
	le.PutUint32(out[108:], 4)         // LCS_GM_IMAGES
	px := out[hsize:]
	for y := 0; y < h; y++ {
		row := px[4*w*(h-1-y):]
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, y)
			row[4*x], row[4*x+1], row[4*x+2], row[4*x+3] = c.B, c.G, c.R, c.A
		}
	}
	return out, nil
}

// ---- NOTIFYICONDATAW (shellapi.h), 64-bit layout ----

const (
	nidSize          = 976
	nidOffHwnd       = 8
	nidOffID         = 16
	nidOffFlags      = 20
	nidOffCallback   = 24
	nidOffIcon       = 32
	nidOffTip        = 40  // WCHAR[128]
	nidOffInfo       = 304 // WCHAR[256]
	nidOffVersion    = 816 // union { uTimeout; uVersion }
	nidOffInfoTitle  = 820 // WCHAR[64]
	nidOffInfoFlags  = 948
	nidOffBalloonIco = 968

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifInfo    = 0x10
	nifShowTip = 0x80

	niifInfo    = 0x01
	niifUser    = 0x04
	niifNoSound = 0x10
	niifLarge   = 0x20
)

// notifyIconData builds a NOTIFYICONDATAW. Strings are truncated to their
// fields (tip 127, info 255, title 63 UTF-16 units).
type notifyIconData [nidSize]byte

func newNotifyIconData(hwnd uintptr, id, flags, callback uint32, icon uintptr) *notifyIconData {
	var d notifyIconData
	le := binary.LittleEndian
	le.PutUint32(d[0:], nidSize)
	le.PutUint64(d[nidOffHwnd:], uint64(hwnd))
	le.PutUint32(d[nidOffID:], id)
	le.PutUint32(d[nidOffFlags:], flags)
	le.PutUint32(d[nidOffCallback:], callback)
	le.PutUint64(d[nidOffIcon:], uint64(icon))
	return &d
}

func (d *notifyIconData) putString(off, units int, s string) {
	u := utf16.Encode([]rune(strings.ReplaceAll(s, "\x00", "")))
	if len(u) > units-1 {
		u = u[:units-1]
	}
	for i := 0; i < units; i++ {
		var c uint16
		if i < len(u) {
			c = u[i]
		}
		binary.LittleEndian.PutUint16(d[off+2*i:], c)
	}
}

func (d *notifyIconData) setTip(s string) { d.putString(nidOffTip, 128, s) }

// setBalloon fills the balloon fields (and NIF_INFO).
func (d *notifyIconData) setBalloon(title, text string, flags uint32) {
	le := binary.LittleEndian
	le.PutUint32(d[nidOffFlags:], le.Uint32(d[nidOffFlags:])|nifInfo)
	d.putString(nidOffInfoTitle, 64, title)
	d.putString(nidOffInfo, 256, text)
	le.PutUint32(d[nidOffInfoFlags:], flags)
}

func (d *notifyIconData) setVersion(v uint32) { binary.LittleEndian.PutUint32(d[nidOffVersion:], v) }

// balloonText merges a subtitle into the body: balloons have only a title
// and a text. An empty text would remove the balloon, so it is at least a
// space.
func balloonText(subtitle, body string) string {
	switch {
	case subtitle != "" && body != "":
		return subtitle + "\n" + body
	case subtitle != "":
		return subtitle
	case body != "":
		return body
	}
	return " "
}

// dropFilesSize is sizeof(DROPFILES): pFiles, pt (POINT), fNC, fWide.
const dropFilesSize = 20

// dropFiles builds CF_HDROP contents for paths: a DROPFILES header with
// fWide set, then each path as NUL-terminated UTF-16, then one more NUL.
func dropFiles(paths []string) ([]byte, error) {
	var units []uint16
	for _, p := range paths {
		if p == "" || strings.ContainsRune(p, 0) {
			return nil, fmt.Errorf("tango: invalid path %q", p)
		}
		units = append(units, utf16.Encode([]rune(p))...)
		units = append(units, 0)
	}
	units = append(units, 0)
	out := make([]byte, dropFilesSize+2*len(units))
	binary.LittleEndian.PutUint32(out[0:], dropFilesSize) // pFiles
	binary.LittleEndian.PutUint32(out[16:], 1)            // fWide
	for i, u := range units {
		binary.LittleEndian.PutUint16(out[dropFilesSize+2*i:], u)
	}
	return out, nil
}
