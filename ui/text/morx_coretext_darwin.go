//go:build darwin && !ios && !tango_gotext

package text

import (
	"encoding/binary"
	"math/bits"
	"runtime"
	"slices"
	"unsafe"
)

// morxASCII proves that a face's AAT ligature subtables cannot change a
// printable ASCII item. It considers every subtable, including optional
// features, and every substring in case CoreText splits the item into runs.
// Only version 2 ligature subtables with format 6 class lookups are supported.
type morxASCII struct {
	machines []morxLigatures
}

type morxLigatures struct {
	reverse bool
	next    [][128]uint8 // dense state indices, 64 means unsafe
}

func (m *morxASCII) allows(rs []rune) bool {
	for _, table := range m.machines {
		active := uint64(0)
		for i := range rs {
			j := i
			if table.reverse {
				j = len(rs) - 1 - i
			}
			r := rs[j]
			// Start-of-text and start-of-line can both begin here.
			var next uint64
			for states := active | 3; states != 0; states &= states - 1 {
				s := table.next[bits.TrailingZeros64(states)][r]
				if s == 64 {
					return false
				}
				next |= 1 << s
			}
			active = next
		}
	}
	return true
}

func readMorxASCII(f *Face) *morxASCII {
	if f.noMorx || !f.noKern || f.substTrak || f.own == nil {
		return nil
	}
	defer runtime.KeepAlive(f)
	base := f.own.base
	if d := ctFontCopyTable(base, tag4("mort"), 0); d != 0 {
		cfRelease(d)
		return nil
	}
	d := ctFontCopyTable(base, tag4("morx"), 0)
	if d == 0 {
		return nil
	}
	defer cfRelease(d)
	n := cfDataGetLength(d)
	if n < 8 || n > 1<<20 {
		return nil
	}
	var glyphs [128]uint32
	for r := rune(' '); r <= '~'; r++ {
		g, ok := f.glyphFor(r)
		if !ok || g == 0 || g >= 0xffff {
			return nil
		}
		glyphs[r] = g
	}
	return parseMorxASCII(unsafe.Slice((*byte)(cfDataGetBytePtr(d)), n), glyphs)
}

func parseMorxASCII(b []byte, glyphs [128]uint32) *morxASCII {
	u32 := func(b []byte) int { return int(binary.BigEndian.Uint32(b)) }
	if len(b) < 8 || u32(b) != 0x00020000 {
		return nil
	}
	m := &morxASCII{}
	chains, b := u32(b[4:]), b[8:]
	remaining := 16 // bound the work even for fonts with aliased data
	for range chains {
		if len(b) < 16 {
			return nil
		}
		n, features, subtables := u32(b[4:]), u32(b[8:]), u32(b[12:])
		if n < 16 || n > len(b) || features > (n-16)/12 || subtables > remaining {
			return nil
		}
		remaining -= subtables
		body := b[16+12*features : n]
		for range subtables {
			if len(body) < 12 {
				return nil
			}
			size, coverage := u32(body), u32(body[4:])
			if size < 12 || size > len(body) || coverage&0x0fffff00 != 0 || coverage&0xff != 2 {
				return nil
			}
			t, ok := compileMorxLigatures(body[12:size], glyphs)
			if !ok {
				return nil
			}
			if t.next != nil {
				t.reverse = coverage&0x40000000 != 0
				m.machines = append(m.machines, t)
			}
			body = body[size:]
		}
		// Some fonts pad version 2 chains after the declared subtables.
		if slices.ContainsFunc(body, func(b byte) bool { return b != 0 }) {
			return nil
		}
		b = b[n:]
	}
	if len(b) != 0 {
		return nil
	}
	return m
}

func compileMorxLigatures(b []byte, glyphs [128]uint32) (morxLigatures, bool) {
	var t morxLigatures
	u16 := func(b []byte) int { return int(binary.BigEndian.Uint16(b)) }
	u32 := func(b []byte) int { return int(binary.BigEndian.Uint32(b)) }
	if len(b) < 28 {
		return t, false
	}
	nc, cl, st, en, act := u32(b), u32(b[4:]), u32(b[8:]), u32(b[12:]), u32(b[16:])
	if nc < 4 || nc > 1<<16 || cl < 28 || st-cl < 12 || en < st || act < en || act > len(b) {
		return t, false
	}
	lookup, states, entries := b[cl:st], b[st:en], b[en:act]
	if u16(lookup) != 6 || u16(lookup[2:]) != 4 || (len(states)%(2*nc)) != 0 {
		return t, false
	}
	n := u16(lookup[4:])
	if n > (len(lookup)-12)/4 || len(states)/(2*nc) < 2 {
		return t, false
	}
	if n > 0 {
		power := bits.Len(uint(n)) - 1
		searchRange := 4 << power
		if u16(lookup[6:]) != searchRange || u16(lookup[8:]) != power || u16(lookup[10:]) != 4*n-searchRange {
			return t, false
		}
	}
	classes := make(map[uint32]int, n)
	last := -1
	for i := range n {
		g, class := u16(lookup[12+4*i:]), u16(lookup[14+4*i:])
		if g <= last || class < 4 || class >= nc {
			return t, false
		}
		classes[uint32(g)] = class
		last = g
	}
	// Follow non-advancing entries until the glyph is consumed. A component
	// stack push is harmless until a ligature action consumes it.
	step := func(state, class int) int {
		for range 16 {
			i := 2 * (state*nc + class)
			if i >= len(states) {
				break
			}
			e := 6 * u16(states[i:])
			if e+6 > len(entries) {
				break
			}
			state = u16(entries[e:])
			flags := u16(entries[e+2:])
			if flags&^0xc000 != 0 || state >= len(states)/(2*nc) {
				break
			}
			if flags&0x4000 == 0 {
				return state
			}
		}
		return -1
	}
	if step(0, 0) < 0 || step(0, 3) < 0 || step(1, 0) < 0 || step(1, 3) < 0 {
		return t, false
	}
	// Compile only states reachable with printable ASCII, and discard the
	// machine entirely when no ASCII sequence can trigger an action.
	queue := []int{0, 1}
	ids := map[int]uint8{0: 0, 1: 1}
	hasUnsafe := false
	for q := 0; q < len(queue); q++ {
		var row [128]uint8
		for r := ' '; r <= '~'; r++ {
			class := classes[glyphs[r]]
			if class == 0 {
				class = 1 // out of bounds
			}
			s := step(queue[q], class)
			if s < 0 || step(s, 0) < 0 || step(s, 3) < 0 {
				row[r] = 64
				hasUnsafe = true
				continue
			}
			id, known := ids[s]
			if !known {
				if len(queue) == 64 {
					return morxLigatures{}, false
				}
				id = uint8(len(queue))
				ids[s] = id
				queue = append(queue, s)
			}
			row[r] = id
		}
		t.next = append(t.next, row)
	}
	if !hasUnsafe {
		t.next = nil
	}
	return t, true
}
