package vt

// Color is a foreground or background color. The high 8 bits hold the
// kind ([ColorKindDefault], [ColorIndexed] or [ColorRGB]); an indexed
// color keeps its palette index in the low 8 bits and an RGB color its
// 0xRRGGBB value in the low 24 bits. The zero Color is the default.
type Color uint32

// Color kinds, as returned by [Color.Kind].
const (
	ColorKindDefault = 0
	ColorIndexed     = 1
	ColorRGB         = 2
)

// ColorDefault is the terminal's default foreground or background.
const ColorDefault Color = 0

// IndexedColor returns palette color n (0–15 the ANSI colors, 16–255 the
// xterm 256-color cube and grays).
func IndexedColor(n uint8) Color { return Color(ColorIndexed<<24 | uint32(n)) }

// RGBColor returns a direct 24-bit color.
func RGBColor(r, g, b uint8) Color {
	return Color(ColorRGB<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b))
}

// Kind returns the color's kind.
func (c Color) Kind() int { return int(c >> 24) }

// Index returns an indexed color's palette index.
func (c Color) Index() uint8 { return uint8(c) }

// RGB returns an RGB color's components.
func (c Color) RGB() (r, g, b uint8) { return uint8(c >> 16), uint8(c >> 8), uint8(c) }

// Attr is a set of character attributes.
type Attr uint16

// Character attributes.
const (
	AttrBold Attr = 1 << iota
	AttrFaint
	AttrItalic
	AttrUnderline
	AttrBlink
	AttrInverse
	AttrInvisible
	AttrStrike
)

// Style is how a cell is drawn. Cells refer to styles by their index in
// the screen's [StyleTable]; the zero Style (index 0) is the default.
type Style struct {
	Fg, Bg Color
	Attr   Attr
}

// Cell is one character cell.
//
// The zero Cell is NOT a blank: Cell{} (Width 0) is the right half of a
// wide character. A blank cell is [BlankCell], Cell{Width: 1}; pad and
// erase with it (or with Cell{Style: s, Width: 1} for a styled blank).
//
// A wide character takes two cells: the left one holds the character with
// Width 2, the right one has Rune 0 and Width 0. For a grapheme with
// combining marks, Rune is the base character.
type Cell struct {
	Rune  rune   // 0 is a blank (drawn as a space)
	Style uint16 // index into the screen's StyleTable
	Width uint8  // 1 or 2; 0 for the right half of a wide character
}

// BlankCell is an empty cell in the default style. Note that the zero
// Cell is not blank; see [Cell].
var BlankCell = Cell{Width: 1}

// Line is one row of cells.
type Line struct {
	Cells   []Cell
	Wrapped bool // the line continues on the next row (soft wrap)
	Dirty   bool // changed since the flag was last cleared
}

// clear blanks l's cells (resizing them to cols, reusing the backing
// array when it is large enough) and resets its flags, marking it dirty.
func (l *Line) clear(cols int) {
	if cap(l.Cells) >= cols {
		l.Cells = l.Cells[:cols]
	} else {
		l.Cells = make([]Cell, cols)
	}
	FillBlank(l.Cells)
	l.Wrapped = false
	l.Dirty = true
}

// FillBlank sets every cell of cells to [BlankCell].
func FillBlank(cells []Cell) {
	// Copy a doubling run instead of storing cell by cell.
	if len(cells) == 0 {
		return
	}
	cells[0] = BlankCell
	for n := 1; n < len(cells); n *= 2 {
		copy(cells[n:], cells[:n])
	}
}
