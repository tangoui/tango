//go:build linux || darwin

package gl

import (
	"math"
	"strings"
)

// OpenGL ES 3.0 (Android). The Device runs on an ES 3.0 context too; it
// tells from GL_VERSION ("OpenGL ES 3.2 ...") and differs from desktop GL
// in three places only, each guarded by Device.es / pipeline.es:
//
//   - Shaders. The quad shader is compiled from esSource of the committed
//     GLSL 330 sources: internal/render/shader/glsl's ES300 dialect is the same text
//     with "#version 300 es" and highp default precisions
//     (TestESSourceMatchesGenerator pins that against the generator).
//   - The ramp texture. GL_RGBA16 (16-bit normalized) needs
//     GL_EXT_texture_norm16 on ES. With it the ramp is GL_RGBA16 as on
//     desktop; without it it is GL_RGBA16F, filterable in core ES 3.0, and
//     every ramp upload is converted from 16-bit unorm to half floats
//     (unormToHalf): 11 significant bits, so a ramp step is at most 2^-12
//     off near 1 and finer below, against 1/255 for RGBA8, which would
//     band visibly in long gradients. Two RGBA8 textures (high and low
//     bytes) would need a second sampler and shader changes; RGBA16F
//     needs neither.
//   - State. GL_FRAMEBUFFER_SRGB does not exist on ES (glDisable would be
//     GL_INVALID_ENUM, a failed frame): ES framebuffers without an sRGB
//     format never convert. glGetGraphicsResetStatus is the KHR or EXT
//     form (glGetGraphicsResetStatusKHR / EXT).

// isES reports whether a GL_VERSION string is an OpenGL ES context's.
func isES(version string) bool { return strings.HasPrefix(version, "OpenGL ES") }

// esPrelude replaces the "#version 330 core" line of a GLSL 330 source.
const esPrelude = "#version 300 es\nprecision highp float;\nprecision highp int;\nprecision highp sampler2D;"

// esSource returns the GLSL ES 3.00 form of a GLSL 330 core source as
// internal/render/shader/glsl generates them: the version line is replaced by the ES
// one followed by the default precisions; nothing else differs.
func esSource(src string) string {
	_, rest, _ := strings.Cut(src, "\n")
	return esPrelude + "\n" + rest
}

// rgba16fFormat is the ramp texture's format on ES without
// GL_EXT_texture_norm16: half floats converted from the 16-bit unorm
// upload (bpp is the upload's, 8; a half texel is 8 bytes too).
var rgba16fFormat = texFormat{internal: glRGBA16F, format: glRGBA, typ: glHALF_FLOAT, bpp: 8, half: true}

const glRGBA16F = 0x881A

// unormToHalf converts a 16-bit unsigned normalized value (u/65535) to
// the nearest IEEE 754 half float (round to nearest, ties to even).
func unormToHalf(u uint16) uint16 {
	if u == 0 {
		return 0
	}
	if u == 0xFFFF {
		return 0x3C00 // 1.0
	}
	// u/65535 in float64 is exact enough: 53 bits for a 16-bit ratio.
	v := float64(u) / 65535
	frac, exp := math.Frexp(v) // v = frac·2^exp, frac in [0.5, 1)
	e := exp - 1 + 15          // biased half exponent of 1.m·2^(exp-1)
	if e <= 0 {
		// Subnormal: v / 2^-24 units, rounded to nearest even.
		return uint16(math.RoundToEven(v * (1 << 24)))
	}
	m := math.RoundToEven((frac*2 - 1) * 1024) // 10 mantissa bits
	if m == 1024 {                             // rounded up to the next power of two
		m = 0
		e++
	}
	return uint16(e)<<10 | uint16(m)
}

// halfRows converts an upload's rows (w texels of 4 little-endian uint16
// each, stride bytes apart) into dst as tightly packed half floats and
// returns it (grown when needed).
func halfRows(dst, pix []byte, w, h, stride int) []byte {
	n := w * 8 * h
	if cap(dst) < n {
		dst = make([]byte, n)
	}
	dst = dst[:n]
	o := 0
	for r := range h {
		row := pix[r*stride : r*stride+w*8]
		for i := 0; i < len(row); i += 2 {
			v := unormToHalf(uint16(row[i]) | uint16(row[i+1])<<8)
			dst[o], dst[o+1] = byte(v), byte(v>>8)
			o += 2
		}
	}
	return dst
}
