package ui

// QR Code encoder written from ISO/IEC 18004:2015 (model 2): numeric,
// alphanumeric and byte (UTF-8) modes, versions 1–40, error correction
// levels L/M/Q/H, Reed–Solomon over GF(2^8) with the 0x11d field
// polynomial, block interleaving, all eight data masks chosen by the
// penalty rules of §7.8.3, and BCH-protected format/version information.

import (
	"errors"
	"image"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// QRLevel is the error-correction level.
type QRLevel uint8

const (
	QRLevelL QRLevel = iota // ~7% recovery
	QRLevelM                // ~15%
	QRLevelQ                // ~25%
	QRLevelH                // ~30%
)

// ParseQRLevel maps "L","M","Q","H" (any case) to a level; unknown → M.
func ParseQRLevel(s string) QRLevel {
	switch strings.ToUpper(s) {
	case "L":
		return QRLevelL
	case "Q":
		return QRLevelQ
	case "H":
		return QRLevelH
	}
	return QRLevelM
}

// QRMatrix is an encoded symbol without quiet zone. Dark(x, y) reports a
// dark module at column x, row y.
type QRMatrix struct {
	Version int
	Level   QRLevel
	Mask    int
	Size    int
	mods    []bool
	fn      []bool // function pattern (not data)
}

// Dark reports whether module (x, y) is dark. Out of range is light.
func (m *QRMatrix) Dark(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Size || y >= m.Size {
		return false
	}
	return m.mods[y*m.Size+x]
}

func (m *QRMatrix) set(x, y int, dark bool) { m.mods[y*m.Size+x] = dark }
func (m *QRMatrix) setFn(x, y int, dark bool) {
	m.mods[y*m.Size+x] = dark
	m.fn[y*m.Size+x] = true
}

// ErrQRTooLong is returned when the data exceeds version 40 capacity.
var ErrQRTooLong = errors.New("ui: qrcode data too long")

// Error-correction codewords per block and number of blocks, indexed
// [level][version]; level order L, M, Q, H (ISO/IEC 18004 Table 9).
var qrECLen = [4][41]int{
	{0, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28},
	{0, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	{0, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
}

var qrBlocks = [4][41]int{
	{0, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},
	{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},
	{0, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},
	{0, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81},
}

// qrRawModules counts the modules available for codewords and remainder
// bits: the whole symbol minus finder/separator/format areas, timing
// patterns, alignment patterns and version information.
func qrRawModules(v int) int {
	size := 17 + 4*v
	n := size * size
	n -= 3 * 64          // finders + separators
	n -= 2 * 15          // format information (two copies)
	n -= 1               // dark module
	n -= 2 * (size - 16) // timing patterns between finders
	if v >= 2 {
		k := v/7 + 2
		n -= (k*k - 3) * 25
		// Alignment patterns crossing the timing lines were counted twice.
		n += 2 * (k - 2) * 5
	}
	if v >= 7 {
		n -= 2 * 18
	}
	return n
}

// QRDataCapacity returns the number of data codewords of version v at level.
func QRDataCapacity(v int, level QRLevel) int {
	return qrRawModules(v)/8 - qrECLen[level][v]*qrBlocks[level][v]
}

func qrAlignPositions(v int) []int {
	if v == 1 {
		return nil
	}
	k := v/7 + 2
	size := 17 + 4*v
	step := 26
	if v != 32 {
		// (size-13)/(k-1) rounded up to an even number
		d := size - 13
		step = (d + (k-1)*2 - 1) / ((k - 1) * 2) * 2
	}
	out := make([]int, k)
	out[0] = 6
	for i := k - 1; i >= 1; i-- {
		out[i] = size - 7 - (k-1-i)*step
	}
	return out
}

// ---------------------------------------------------------------------------
// Bit stream

type qrBits struct {
	b []byte
	n int
}

func (q *qrBits) put(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		if q.n%8 == 0 {
			q.b = append(q.b, 0)
		}
		if v>>uint(i)&1 == 1 {
			q.b[q.n/8] |= 0x80 >> uint(q.n%8)
		}
		q.n++
	}
}

const qrAlnum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

type qrMode uint8

const (
	qrNumeric qrMode = iota
	qrAlphanumeric
	qrByte
)

func qrPickMode(s string) qrMode {
	num, al := true, true
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			num = false
		}
		if strings.IndexByte(qrAlnum, ch) < 0 {
			al = false
		}
	}
	switch {
	case num:
		return qrNumeric
	case al:
		return qrAlphanumeric
	}
	return qrByte
}

func qrCountBits(m qrMode, v int) int {
	g := 0
	if v >= 27 {
		g = 2
	} else if v >= 10 {
		g = 1
	}
	return [3][3]int{{10, 12, 14}, {9, 11, 13}, {8, 16, 16}}[m][g]
}

func qrPayloadBits(m qrMode, n int) int {
	switch m {
	case qrNumeric:
		b := n / 3 * 10
		switch n % 3 {
		case 1:
			b += 4
		case 2:
			b += 7
		}
		return b
	case qrAlphanumeric:
		return n/2*11 + n%2*6
	}
	return n * 8
}

func qrEncodeData(s string, m qrMode, v int, level QRLevel) []byte {
	var bs qrBits
	bs.put([3]uint32{1, 2, 4}[m], 4)
	bs.put(uint32(len(s)), qrCountBits(m, v))
	switch m {
	case qrNumeric:
		for i := 0; i < len(s); i += 3 {
			j := min(i+3, len(s))
			val := uint32(0)
			for _, ch := range []byte(s[i:j]) {
				val = val*10 + uint32(ch-'0')
			}
			bs.put(val, []int{0, 4, 7, 10}[j-i])
		}
	case qrAlphanumeric:
		for i := 0; i < len(s); i += 2 {
			a := uint32(strings.IndexByte(qrAlnum, s[i]))
			if i+1 < len(s) {
				bs.put(a*45+uint32(strings.IndexByte(qrAlnum, s[i+1])), 11)
			} else {
				bs.put(a, 6)
			}
		}
	default:
		for i := 0; i < len(s); i++ {
			bs.put(uint32(s[i]), 8)
		}
	}
	capBits := QRDataCapacity(v, level) * 8
	bs.put(0, min(4, capBits-bs.n))
	if r := bs.n % 8; r != 0 {
		bs.put(0, 8-r)
	}
	for pad := byte(0xEC); len(bs.b) < capBits/8; pad ^= 0xEC ^ 0x11 {
		bs.b = append(bs.b, pad)
	}
	return bs.b
}

// ---------------------------------------------------------------------------
// Reed–Solomon over GF(256), primitive polynomial x^8+x^4+x^3+x^2+1.

var qrExp, qrLog = func() ([512]byte, [256]byte) {
	var e [512]byte
	var lg [256]byte
	x := 1
	for i := 0; i < 255; i++ {
		e[i] = byte(x)
		lg[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	for i := 255; i < 512; i++ {
		e[i] = e[i-255]
	}
	return e, lg
}()

func qrMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return qrExp[int(qrLog[a])+int(qrLog[b])]
}

// qrGenerator returns the coefficients (highest degree first, leading 1
// omitted) of Π(x - α^i), i = 0..n-1.
func qrGenerator(n int) []byte {
	g := []byte{1}
	for i := 0; i < n; i++ {
		next := make([]byte, len(g)+1)
		for j, c := range g {
			next[j] ^= c
			next[j+1] ^= qrMul(c, qrExp[i])
		}
		g = next
	}
	return g[1:]
}

// qrRemainder computes the EC codewords of data for generator gen.
func qrRemainder(data, gen []byte) []byte {
	rem := make([]byte, len(gen))
	for _, d := range data {
		f := d ^ rem[0]
		copy(rem, rem[1:])
		rem[len(rem)-1] = 0
		for i, g := range gen {
			rem[i] ^= qrMul(g, f)
		}
	}
	return rem
}

// qrInterleave splits data into blocks, appends RS codewords and
// interleaves them (§7.6).
func qrInterleave(data []byte, v int, level QRLevel) []byte {
	nb := qrBlocks[level][v]
	ecl := qrECLen[level][v]
	raw := qrRawModules(v) / 8
	short := nb - raw%nb
	shortLen := raw/nb - ecl // data codewords in a short block
	gen := qrGenerator(ecl)
	var blocks, ecs [][]byte
	k := 0
	for i := 0; i < nb; i++ {
		n := shortLen
		if i >= short {
			n++
		}
		b := data[k : k+n]
		k += n
		blocks = append(blocks, b)
		ecs = append(ecs, qrRemainder(b, gen))
	}
	out := make([]byte, 0, raw)
	for i := 0; i <= shortLen; i++ {
		for _, b := range blocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	for i := 0; i < ecl; i++ {
		for _, e := range ecs {
			out = append(out, e[i])
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// BCH codes for format and version information.

func qrBCH(data uint32, dataBits int, gen uint32, genBits int) uint32 {
	r := data << uint(genBits-1)
	for i := dataBits + genBits - 2; i >= genBits-1; i-- {
		if r>>uint(i)&1 == 1 {
			r ^= gen << uint(i-genBits+1)
		}
	}
	return data<<uint(genBits-1) | r
}

// QRFormatBits returns the 15-bit masked format information.
func QRFormatBits(level QRLevel, mask int) uint32 {
	lv := [4]uint32{1, 0, 3, 2}[level] // L=01 M=00 Q=11 H=10
	return qrBCH(lv<<3|uint32(mask), 5, 0x537, 11) ^ 0x5412
}

// QRVersionBits returns the 18-bit version information (v >= 7).
func QRVersionBits(v int) uint32 { return qrBCH(uint32(v), 6, 0x1f25, 13) }

// ---------------------------------------------------------------------------
// Matrix construction

func newQRMatrix(v int, level QRLevel) *QRMatrix {
	size := 17 + 4*v
	m := &QRMatrix{Version: v, Level: level, Size: size, mods: make([]bool, size*size), fn: make([]bool, size*size)}
	// Timing patterns.
	for i := 0; i < size; i++ {
		m.setFn(6, i, i%2 == 0)
		m.setFn(i, 6, i%2 == 0)
	}
	// Finders with separators.
	for _, o := range [][2]int{{0, 0}, {size - 7, 0}, {0, size - 7}} {
		for dy := -1; dy <= 7; dy++ {
			for dx := -1; dx <= 7; dx++ {
				x, y := o[0]+dx, o[1]+dy
				if x < 0 || y < 0 || x >= size || y >= size {
					continue
				}
				d := max(qrAbs(dx-3), qrAbs(dy-3))
				m.setFn(x, y, d != 2 && d != 4)
			}
		}
	}
	// Alignment patterns (skipping the three finder corners).
	al := qrAlignPositions(v)
	for i, ay := range al {
		for j, ax := range al {
			if (i == 0 && j == 0) || (i == 0 && j == len(al)-1) || (i == len(al)-1 && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					m.setFn(ax+dx, ay+dy, max(qrAbs(dx), qrAbs(dy)) != 1)
				}
			}
		}
	}
	// Reserve format areas (real bits drawn after masking) and the dark module.
	m.drawFormat(0)
	m.setFn(8, size-8, true)
	if v >= 7 {
		bits := QRVersionBits(v)
		for i := 0; i < 18; i++ {
			d := bits>>uint(i)&1 == 1
			a, b := size-11+i%3, i/3
			m.setFn(a, b, d)
			m.setFn(b, a, d)
		}
	}
	return m
}

func qrAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m *QRMatrix) drawFormat(mask int) {
	bits := QRFormatBits(m.Level, mask)
	bit := func(i int) bool { return bits>>uint(i)&1 == 1 }
	n := m.Size
	// Copy around the top-left finder.
	for i := 0; i <= 5; i++ {
		m.setFn(8, i, bit(i))
	}
	m.setFn(8, 7, bit(6))
	m.setFn(8, 8, bit(7))
	m.setFn(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		m.setFn(14-i, 8, bit(i))
	}
	// Copy split between the top-right and bottom-left finders.
	for i := 0; i < 8; i++ {
		m.setFn(n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		m.setFn(8, n-15+i, bit(i))
	}
	m.setFn(8, n-8, true)
}

func (m *QRMatrix) placeData(cw []byte) {
	n := m.Size
	i := 0
	total := len(cw) * 8
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		up := (right+1)&2 == 0
		for vert := 0; vert < n; vert++ {
			y := vert
			if up {
				y = n - 1 - vert
			}
			for j := 0; j < 2; j++ {
				x := right - j
				if m.fn[y*n+x] {
					continue
				}
				if i < total {
					m.mods[y*n+x] = cw[i/8]>>uint(7-i%8)&1 == 1
				}
				i++
			}
		}
	}
}

func qrMaskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	}
	return ((x+y)%2+x*y%3)%2 == 0
}

func (m *QRMatrix) applyMask(mask int) {
	n := m.Size
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if !m.fn[y*n+x] && qrMaskBit(mask, x, y) {
				m.mods[y*n+x] = !m.mods[y*n+x]
			}
		}
	}
}

// penalty scores the symbol with the four rules of §7.8.3.
func (m *QRMatrix) penalty() int {
	n := m.Size
	score := 0
	line := make([]bool, n)
	finder := func(l []bool, i int) bool { // 1:1:3:1:1 dark-light pattern starting at i
		p := [7]bool{true, false, true, true, true, false, true}
		for k := 0; k < 7; k++ {
			if l[i+k] != p[k] {
				return false
			}
		}
		light := func(a, b int) bool {
			for k := a; k < b; k++ {
				if k >= 0 && k < len(l) && l[k] {
					return false
				}
			}
			return true
		}
		return light(i-4, i) || light(i+7, i+11)
	}
	for pass := 0; pass < 2; pass++ {
		for a := 0; a < n; a++ {
			for b := 0; b < n; b++ {
				if pass == 0 {
					line[b] = m.mods[a*n+b]
				} else {
					line[b] = m.mods[b*n+a]
				}
			}
			run := 1
			for b := 1; b <= n; b++ {
				if b < n && line[b] == line[b-1] {
					run++
					continue
				}
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
			}
			for b := 0; b+7 <= n; b++ {
				if finder(line, b) {
					score += 40
				}
			}
		}
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := m.mods[y*n+x]
			if d {
				dark++
			}
			if x+1 < n && y+1 < n && d == m.mods[y*n+x+1] && d == m.mods[(y+1)*n+x] && d == m.mods[(y+1)*n+x+1] {
				score += 3
			}
		}
	}
	dev := qrAbs(dark*100/(n*n) - 50)
	score += dev / 5 * 10
	return score
}

// EncodeQR encodes text at the smallest version that fits level. Digits use
// numeric mode, [0-9A-Z $%*+-./:] alphanumeric mode, anything else UTF-8
// bytes.
func EncodeQR(s string, level QRLevel) (*QRMatrix, error) {
	return encodeQR(s, level, -1)
}

func encodeQR(s string, level QRLevel, forceMask int) (*QRMatrix, error) {
	mode := qrPickMode(s)
	v := 0
	for ver := 1; ver <= 40; ver++ {
		need := 4 + qrCountBits(mode, ver) + qrPayloadBits(mode, len(s))
		if need <= QRDataCapacity(ver, level)*8 && len(s) < 1<<uint(qrCountBits(mode, ver)) {
			v = ver
			break
		}
	}
	if v == 0 {
		return nil, ErrQRTooLong
	}
	cw := qrInterleave(qrEncodeData(s, mode, v, level), v, level)
	base := newQRMatrix(v, level)
	base.placeData(cw)
	var best *QRMatrix
	bestScore := 0
	for mask := 0; mask < 8; mask++ {
		if forceMask >= 0 && mask != forceMask {
			continue
		}
		m := &QRMatrix{Version: v, Level: level, Mask: mask, Size: base.Size,
			mods: append([]bool(nil), base.mods...), fn: base.fn}
		m.applyMask(mask)
		m.drawFormat(mask)
		if sc := m.penalty(); best == nil || sc < bestScore {
			best, bestScore = m, sc
		}
	}
	return best, nil
}

// ---------------------------------------------------------------------------
// Widget

// QRCodeW renders a QR code (Ant Design QRCode / Element has none; the
// Element flavor keeps the same component with Element tokens).
type QRCodeW struct {
	c        *Context
	n        *node
	value    string
	size     float32
	level    QRLevel
	col, bg  theme.Color
	colSet   bool
	bgSet    bool
	bordered bool
	ic       image.Image
	icSize   float32
	status   string
	refresh  bool
	loc      *Locale // captured at build; paint runs outside ConfigProvider
	ia       Interaction
}

type qrCache struct {
	value string
	level QRLevel
	m     *QRMatrix
	err   error
}

// QRCode adds a QR code for value (default 160px, level M, bordered).
func QRCode(c *Context, value string) *QRCodeW {
	q := &QRCodeW{c: c, value: value, size: 160, level: QRLevelM, bordered: true, icSize: 40, loc: c.Locale()}
	// ConfigProvider.Defaults("QRCode", ...) keys: "size" (px),
	// "errorLevel" ("L" | "M" | "Q" | "H"), "bordered" (bool).
	if v, ok := p2bNum(c, "QRCode", "size"); ok && v > 0 {
		q.size = float32(v)
	}
	if v, ok := p2bStr(c, "QRCode", "errorLevel"); ok {
		q.level = ParseQRLevel(v)
	}
	q.bordered = Prop(c, "QRCode", "bordered", true)
	q.n = c.add("qrcode", value)
	q.n.ln.NoStretch = true
	q.n.ln.AlignSelf = layout.AlignAuto
	q.n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: q.size, H: q.size} }
	q.n.paint = q.paint
	q.n.setRole(a11y.RoleImage).fill(q)
	return q
}

// A11yLabel sets the accessible name (default: the encoded value).
func (q *QRCodeW) A11yLabel(s string) *QRCodeW { q.n.setName(s); return q }

func (q *QRCodeW) semantics(s *a11y.Node) {
	switch q.status {
	case "loading":
		s.States |= a11y.Busy
	case "expired":
		s.Description = q.loc.QRCode.Expired
	case "scanned":
		s.Description = q.loc.QRCode.Scanned
	}
}

// Size sets the outer size in px.
func (q *QRCodeW) Size(px float32) *QRCodeW { q.size = px; return q }

// Level sets the error-correction level: "L", "M", "Q" or "H".
func (q *QRCodeW) Level(l string) *QRCodeW { q.level = ParseQRLevel(l); return q }

// Color sets the module color.
func (q *QRCodeW) Color(col theme.Color) *QRCodeW { q.col, q.colSet = col, true; return q }

// Bg sets the background color.
func (q *QRCodeW) Bg(col theme.Color) *QRCodeW { q.bg, q.bgSet = col, true; return q }

// Bordered toggles the outer border.
func (q *QRCodeW) Bordered(on bool) *QRCodeW { q.bordered = on; return q }

// Icon draws img in the center (use level "H" so the code stays readable).
func (q *QRCodeW) Icon(img image.Image) *QRCodeW { q.ic = img; return q }

// IconSize sets the center icon size (default 40).
func (q *QRCodeW) IconSize(px float32) *QRCodeW { q.icSize = px; return q }

// Status sets "active" (default), "expired", "loading" or "scanned".
func (q *QRCodeW) Status(s string) *QRCodeW {
	q.status = s
	if s == "expired" {
		// The whole code is the "refresh" button of the expired mask.
		q.ia = q.c.interact(q.n, true, input.CursorPointer)
		q.n.setRole(a11y.RoleButton).setName(q.loc.QRCode.Refresh)
	}
	return q
}

// Refreshed reports a click on the "点击刷新" action of an expired code.
func (q *QRCodeW) Refreshed() bool { return q.status == "expired" && q.ia.Clicked }

// Matrix returns the encoded matrix (nil if encoding failed).
func (q *QRCodeW) Matrix() *QRMatrix { return q.matrix() }

func (q *QRCodeW) matrix() *QRMatrix {
	st := State[qrCache](q.c, q.n.id, nil)
	if st.m == nil && st.err == nil || st.value != q.value || st.level != q.level {
		st.value, st.level = q.value, q.level
		st.m, st.err = EncodeQR(q.value, q.level)
	}
	return st.m
}

func (q *QRCodeW) paint(l *paint.List, r geom.Rect) {
	th := q.c.Theme
	m := q.matrix()
	bg := theme.Color{}
	if q.bgSet {
		bg = q.bg
	}
	col := th.Text
	if th.Flavor == theme.Element {
		col = th.TextHeading
	}
	if th.Dark && !q.bgSet {
		// Intentionally not a theme token: inverted (light-on-dark) symbols
		// are not read by many scanners, so dark themes keep black modules
		// on a white plate for maximum contrast.
		bg, col = theme.Hex("#ffffff"), theme.Hex("#000000")
	}
	if q.colSet {
		col = q.col
	}
	rad := geom.Rad(th.RadiusLG)
	if th.Flavor == theme.Element {
		rad = geom.Rad(th.Radius)
	}
	l.Fill(r, rad, bg)
	if q.bordered {
		l.Stroke(r, rad, 1, th.BorderSecondary)
	}
	pad := float32(12)
	inner := geom.Rect{X: r.X + pad, Y: r.Y + pad, W: r.W - 2*pad, H: r.H - 2*pad}
	if m != nil {
		cell := inner.W / float32(m.Size)
		// Merge horizontal runs into one rect per run so edges stay crisp
		// and the display list stays small.
		p := paint.NewPath()
		for y := 0; y < m.Size; y++ {
			for x := 0; x < m.Size; {
				if !m.Dark(x, y) {
					x++
					continue
				}
				x0 := x
				for x < m.Size && m.Dark(x, y) {
					x++
				}
				p.Rect(geom.Rect{X: inner.X + float32(x0)*cell, Y: inner.Y + float32(y)*cell, W: float32(x-x0) * cell, H: cell + 0.02})
			}
		}
		l.Path(p, col)
		if q.ic != nil {
			s := min(q.icSize, inner.W*0.3)
			ir := geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}
			l.Fill(ir.Expand(3), geom.Rad(th.RadiusSM+2), th.BgContainer)
			l.DrawImage(q.ic, ir, geom.Rad(th.RadiusSM))
		}
	}
	switch q.status {
	case "expired", "loading", "scanned":
		mask := theme.Fade(th.BgContainer, 0.96)
		l.Fill(r, rad, mask)
		if q.bordered {
			l.Stroke(r, rad, 1, th.BorderSecondary)
		}
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		switch q.status {
		case "loading":
			drawSpinner(q.c, l, geom.Rect{X: cx - 10, Y: cy - 10, W: 20, H: 20})
		case "expired":
			t1 := q.c.dataShape(q.loc.QRCode.Expired, th.FontSize, text.Regular)
			t2 := q.c.dataShape(q.loc.QRCode.Refresh, th.FontSize, text.Regular)
			y := cy - (t1.Height+t2.Height)/2
			dataText(l, t1, cx-t1.Width/2, y, th.Text)
			x := cx - (t2.Width+18)/2
			icon.Draw(l, icon.Reload, geom.Rect{X: x, Y: y + t1.Height + (t2.Height-14)/2, W: 14, H: 14}, th.TextLink, 0)
			dataText(l, t2, x+18, y+t1.Height, th.TextLink)
			if q.ia.FocusVisible {
				drawFocusRing(l, th, r, rad)
			}
		case "scanned":
			t := q.c.dataShape(q.loc.QRCode.Scanned, th.FontSize, text.Regular)
			drawStatusIcon(l, "success", geom.Rect{X: cx - 12, Y: cy - 12 - t.Height/2 - 4, W: 24, H: 24}, th.Success.Base, th.BgContainer)
			dataText(l, t, cx-t.Width/2, cy+12-t.Height/2+4, th.Text)
		}
	}
}
