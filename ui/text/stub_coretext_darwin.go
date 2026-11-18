//go:build darwin && !ios && tango_coretext

package text

// Interim stubs of the engine primitives the CoreText build does not
// implement yet. Each later task deletes the stubs it replaces:
//
//   - Task 3: lineEnds, paraLevels
//   - Task 4: shapeItems, fontmapImpl, (*fontmap).rememberFace, resetShaper,
//     coreTextShape (the shaping path makes it unnecessary)
//   - Task 5: simpleNumericLayout, simpleNumericLayoutInto, fastPath,
//     (*LineCache).fastShape

func shapeItems(runes []rune, items []item, fms []*fontmap, adv []float32) []sg {
	panic("ui/text: CoreText shapeItems not implemented yet (Task 3/4)")
}

func lineEnds(runes []rune, a, b int) []int {
	panic("ui/text: CoreText lineEnds not implemented yet (Task 3/4)")
}

func paraLevels(runes []rune, a, b int, dir Direction, lv []level) (base level, mixed bool) {
	panic("ui/text: CoreText paraLevels not implemented yet (Task 3/4)")
}

// fontmapImpl is the engine part of fontmap.
type fontmapImpl struct{}

// rememberFace records f as the face resolve picked.
func (m *fontmap) rememberFace(f *Face) {}

// resetShaper drops the shaping caches (clearPrepared).
func resetShaper() {}

func coreTextShape(s string, st Style, opt Options) *Layout { return nil }

func simpleNumericLayout(s string, st Style, opt Options, version int) *Layout { return nil }

func simpleNumericLayoutInto(dst *Layout, s string, st Style, opt Options, version int) *Layout {
	return nil
}

// fastPath memoizes the monospace fast path of LineCache.
type fastPath struct{}

func (c *LineCache) fastShape(line string, runs []Run, opt Options) *Layout { return nil }
