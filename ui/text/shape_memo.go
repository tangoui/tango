package text

import (
	"encoding/binary"
	"unsafe"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// Shape's layout cache misses for every new (string, Style, Options) key:
// a label measured at another width, more distinct labels on screen than
// the layout ring holds, or a label inside a different paragraph. The runs
// handed to HarfBuzz still repeat, so compactShaper memoizes short runs.
//
// The key is everything HarfBuzz reads: the font face (pointer, plus its
// mutable ppem and variation coordinates), the size, direction, script,
// language and features, the run's runes, and the up to five runes of
// context on each side that Buffer.AddRunes installs (Arabic joining and
// other contextual lookups read them). The value is the compact glyph
// output with clusters relative to the run start, so it is exactly what
// shaping the run again would produce. Faces are keyed by pointer and the
// memo keeps them alive, so a pointer cannot be reused by another face
// while an entry refers to it; the memo is also dropped with the other
// shaping caches (clearPrepared) whenever the registry version changes,
// the system engine is switched, fonts are registered or caches trimmed.
//
// Access is under mu, as for the rest of compactShaper.
const (
	shapeMemoRunes   = 64      // longer runs are shaped every time
	shapeMemoEntries = 4096    // FIFO entry bound
	shapeMemoBudget  = 2 << 20 // FIFO byte bound (see shapeMemoCost)
	// HarfBuzz's buffer context length (harfbuzz.contextLength).
	shapeMemoContext = 5
)

// shapeMemoOff disables the memo (tests compare against the uncached path).
var shapeMemoOff bool

type shapeMemoKey struct {
	gt           *font.Face
	face         *Face
	size         fixed.Int26_6
	xPpem, yPpem uint16
	dir          di.Direction
	script       language.Script
	lang         language.Language
	// Uvarint-encoded: pre-context, run and post-context runes (each with
	// its length), variation coordinates and features.
	text string
}

type shapeMemo struct {
	m       map[shapeMemoKey][]sg
	order   []shapeMemoKey // insertion order; live keys are order[head:]
	head    int
	bytes   int
	buf     []byte
	hits    int
	misses  int
	pending bool // the last lookup missed; buf holds its key text
}

// shapeMemoCost bounds what one entry retains: the key in the map and in
// the FIFO, the key text, the glyphs, and map slot overhead.
func shapeMemoCost(k shapeMemoKey, gl []sg) int {
	return 2*int(unsafe.Sizeof(k)) + len(k.text) + cap(gl)*int(unsafe.Sizeof(sg{})) + 32
}

// encode writes the variable part of the key for in into m.buf; false when
// the run is not memoized.
func (m *shapeMemo) encode(in shaping.Input, coords []tables.Coord) bool {
	n := in.RunEnd - in.RunStart
	if shapeMemoOff || n <= 0 || n > shapeMemoRunes {
		return false
	}
	pre := max(0, in.RunStart-shapeMemoContext)
	post := min(len(in.Text), in.RunEnd+shapeMemoContext)
	b := m.buf[:0]
	for _, part := range [3][]rune{in.Text[pre:in.RunStart], in.Text[in.RunStart:in.RunEnd], in.Text[in.RunEnd:post]} {
		b = binary.AppendUvarint(b, uint64(len(part)))
		for _, r := range part {
			b = binary.AppendUvarint(b, uint64(uint32(r)))
		}
	}
	b = binary.AppendUvarint(b, uint64(len(coords)))
	for _, c := range coords {
		b = binary.AppendUvarint(b, uint64(uint16(c)))
	}
	b = binary.AppendUvarint(b, uint64(len(in.FontFeatures)))
	for _, f := range in.FontFeatures {
		b = binary.AppendUvarint(b, uint64(f.Tag))
		b = binary.AppendUvarint(b, uint64(f.Value))
	}
	m.buf = b
	return true
}

// lookup appends the memoized glyphs of in to dst, or reports a miss
// (leaving the encoded key for store).
func (m *shapeMemo) lookup(dst []sg, in shaping.Input, face *Face, run int32, xPpem, yPpem uint16, coords []tables.Coord) ([]sg, bool) {
	m.pending = false
	if !m.encode(in, coords) {
		return dst, false
	}
	// The string(m.buf) conversion inside the index expression does not
	// allocate.
	gl, ok := m.m[shapeMemoKey{gt: in.Face, face: face, size: in.Size, xPpem: xPpem, yPpem: yPpem, dir: in.Direction, script: in.Script, lang: in.Language, text: string(m.buf)}]
	if !ok {
		m.misses++
		m.pending = true
		return dst, false
	}
	m.hits++
	for _, g := range gl {
		g.face, g.cluster, g.run = face, g.cluster+in.RunStart, run
		dst = append(dst, g)
	}
	return dst, true
}

// store remembers out, the glyphs just shaped for in after a missed lookup.
func (m *shapeMemo) store(out []sg, in shaping.Input, face *Face, xPpem, yPpem uint16) {
	if !m.pending {
		return
	}
	m.pending = false
	gl := make([]sg, len(out))
	for i, g := range out {
		g.face, g.cluster, g.run = nil, g.cluster-in.RunStart, 0
		gl[i] = g
	}
	k := shapeMemoKey{gt: in.Face, face: face, size: in.Size, xPpem: xPpem, yPpem: yPpem, dir: in.Direction, script: in.Script, lang: in.Language, text: string(m.buf)}
	cost := shapeMemoCost(k, gl)
	if cost > shapeMemoBudget {
		return
	}
	if m.m == nil {
		m.m = make(map[shapeMemoKey][]sg)
	}
	for len(m.m) > 0 && (len(m.m) >= shapeMemoEntries || m.bytes+cost > shapeMemoBudget) {
		old := m.order[m.head]
		m.bytes -= shapeMemoCost(old, m.m[old])
		delete(m.m, old)
		m.order[m.head] = shapeMemoKey{}
		m.head++
	}
	if m.head > len(m.order)/2 {
		n := copy(m.order, m.order[m.head:])
		clear(m.order[n:])
		m.order = m.order[:n]
		m.head = 0
	}
	m.order = append(m.order, k)
	m.m[k] = gl
	m.bytes += cost
}
