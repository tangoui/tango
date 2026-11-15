package text

// glyphClusters maps each source rune to its glyphs in shaping order. A
// dense, pointer-free linked index replaces one slice header and usually a
// separate allocation per rune. Entries for runes inside a ligature are -1.
type glyphClusters struct{ first, next []int32 }

func indexGlyphClusters(runes int, glyphs []sg) glyphClusters {
	data := make([]int32, runes+1+len(glyphs))
	c := glyphClusters{first: data[: runes+1 : runes+1], next: data[runes+1:]}
	for i := range data {
		data[i] = -1
	}
	// Prepending in reverse retains the shaper's glyph order within each
	// cluster, including right-to-left runs and combining marks.
	for i := len(glyphs) - 1; i >= 0; i-- {
		k := glyphs[i].cluster
		if k >= 0 && k < len(c.first) {
			c.next[i] = c.first[k]
			c.first[k] = int32(i)
		}
	}
	return c
}
