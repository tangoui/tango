package editor

import (
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Indentation guides (Monaco's IndentGuidesOverlay and the model's
// getLinesIndentGuides / getActiveIndentGuide, guidesTextModelPart.js;
// Copyright (c) Microsoft Corporation, MIT License, see
// ui/editor/theme/NOTICE).
//
// A line's guide count is its indentation divided by the indent size
// (rounded up); a blank line takes it from the nearest non-blank lines
// above and below. Guide k (from 1) is a one pixel vertical line at column
// (k-1)*indentSize of every row of the line, in editorIndentGuide.background
// k (cycling through the levels whose colors are not transparent); the
// guide of the primary caret's block (the active guide) is drawn in
// editorIndentGuide.activeBackgroundk. The indent size is the tab size
// (Monaco's indentSize "tabSize").

// indentOf is Monaco's computeIndentLevel: the indentation columns of a
// line, -1 for a blank one.
func indentOf(s string, tab int) int {
	ind := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			ind++
		case '\t':
			ind = ind - ind%tab + tab
		default:
			return ind
		}
	}
	return -1
}

// guideCtx computes guide levels over one snapshot, caching line indents.
type guideCtx struct {
	snap         buffer.Snapshot
	tab          int
	offSide      bool
	ind          map[int]int
	above, below map[int]int // contentAbove / contentBelow results
}

func (g *guideCtx) indent(i int) int {
	if v, ok := g.ind[i]; ok {
		return v
	}
	v := indentOf(g.snap.Line(i), g.tab)
	g.ind[i] = v
	return v
}

func ceilDiv(a, b int) int { return (a + b - 1) / b }

// blankLevel is Monaco's _getIndentLevelForWhitespaceLine.
func (g *guideCtx) blankLevel(above, below int) int {
	switch {
	case above == -1 || below == -1:
		return 0
	case above < below:
		return 1 + above/g.tab
	case above == below:
		return ceilDiv(below, g.tab)
	case g.offSide:
		return ceilDiv(below, g.tab)
	}
	return 1 + below/g.tab
}

// contentAbove and contentBelow return the indent of the nearest
// non-blank line before / after line i, -1 at the document's edge.
// A blank line's search stops at a blank line searched before: a run of
// blank rows scans past its blank lines once.
func (g *guideCtx) contentAbove(i int) int {
	if v, ok := g.above[i]; ok {
		return v
	}
	v := -1
	for j := i - 1; j >= 0; j-- {
		if x := g.indent(j); x >= 0 {
			v = x
			break
		}
		if x, ok := g.above[j]; ok {
			v = x
			break
		}
	}
	g.above[i] = v
	return v
}

func (g *guideCtx) contentBelow(i int) int {
	if v, ok := g.below[i]; ok {
		return v
	}
	v := -1
	for j := i + 1; j < g.snap.LineCount(); j++ {
		if x := g.indent(j); x >= 0 {
			v = x
			break
		}
		if x, ok := g.below[j]; ok {
			v = x
			break
		}
	}
	g.below[i] = v
	return v
}

// level returns line i's guide count.
func (g *guideCtx) level(i int) int {
	if v := g.indent(i); v >= 0 {
		return ceilDiv(v, g.tab)
	}
	return g.blankLevel(g.contentAbove(i), g.contentBelow(i))
}

// active is Monaco's getActiveIndentGuide for the caret's line, searching
// lines lo..hi (the visible ones): the block's first and last lines and
// its level (0: none).
func (g *guideCtx) active(line, lo, hi int) (start, end, level int) {
	n := g.snap.LineCount()
	levelAt := func(i int) int {
		if i < 0 || i >= n {
			return -1
		}
		return g.level(i)
	}
	goUp, goDown := true, true
	initial := 0
	for d := 0; goUp || goDown; d++ {
		up, down := line-d, line+d
		if d > 1 && (up < 0 || up < lo) {
			goUp = false
		}
		if d > 1 && (down >= n || down > hi) {
			goDown = false
		}
		if d > 50000 {
			goUp, goDown = false, false
		}
		upLevel, downLevel := -1, -1
		if goUp && up >= 0 {
			upLevel = levelAt(up)
		}
		if goDown && down < n {
			downLevel = levelAt(down)
		}
		if d == 0 {
			initial = upLevel
			continue
		}
		if d == 1 {
			if down < n && downLevel >= 0 && initial+1 == downLevel {
				// The start of a block: its child level is active.
				goUp = false
				start, end, level = down, down, downLevel
				continue
			}
			if up >= 0 && upLevel >= 0 && upLevel-1 == initial {
				// The end of a block.
				goDown = false
				start, end, level = up, up, upLevel
				continue
			}
			start, end, level = line, line, initial
			if level == 0 {
				return start, end, level
			}
		}
		if goUp {
			if upLevel >= level {
				start = up
			} else {
				goUp = false
			}
		}
		if goDown {
			if downLevel >= level {
				end = down
			} else {
				goDown = false
			}
		}
	}
	return start, end, level
}

// guideColors are the inactive and active guide colors of the levels
// whose colors are both opaque enough to draw (Monaco skips transparent
// ones).
type guideColors struct{ bg, active []color.NRGBA }

func guideColorsOf(th *theme.Theme) guideColors {
	var gc guideColors
	for _, k := range [...]string{"1", "2", "3", "4", "5", "6"} {
		bg, ok1 := lookupColor(th, "editorIndentGuide.background"+k)
		ac, ok2 := lookupColor(th, "editorIndentGuide.activeBackground"+k)
		if ok1 && ok2 && bg.A != 0 && ac.A != 0 {
			gc.bg = append(gc.bg, bg)
			gc.active = append(gc.active, ac)
		}
	}
	return gc
}

// rowGuides is the guides of a laid out row: its line's level, and the
// active level on it (0: none).
type rowGuides struct{ level, active int }

// guidesFor returns the guides of the laid out rows (nil when indentation
// guides are off). It reads only the text: paint calls it after layout.
func (w *ViewW) guidesFor(snap buffer.Snapshot, rows []lineView, caretLine int) []rowGuides {
	if !w.opts.Guides.Indentation || len(rows) == 0 {
		return nil
	}
	g := &guideCtx{snap: snap, tab: max(w.tabs(), 1), ind: map[int]int{}, above: map[int]int{}, below: map[int]int{}}
	if c := w.st.Model.Conf(); c != nil {
		g.offSide = c.Folding.OffSide
	}
	out := make([]rowGuides, len(rows))
	// The searches below blank lines bottom-up, so a run of blank rows
	// shares one (contentBelow's memo).
	for k := len(rows) - 1; k >= 0; k-- {
		if i := rows[k].i; g.indent(i) < 0 {
			g.contentBelow(i)
		}
	}
	levels := map[int]int{}
	for k := range rows {
		i := rows[k].i
		v, ok := levels[i]
		if !ok {
			v = g.level(i)
			levels[i] = v
		}
		out[k].level = v
	}
	if w.opts.Guides.HighlightActiveIndentation {
		lo, hi := rows[0].i, rows[len(rows)-1].i
		s, e, lv := g.active(caretLine, lo, hi)
		if lv > 0 {
			for k := range rows {
				if i := rows[k].i; s <= i && i <= e && out[k].level >= lv {
					out[k].active = lv
				}
			}
		}
	}
	return out
}

// paintGuides draws the indentation guides of the laid out rows; maxX is
// the wrapping column's x while wrapping (0: none).
func paintGuides(l *paint.List, rows []lineView, guides []rowGuides, gc guideColors, tx, colW, lh float32, tab int, maxX float32, lineY func(int) float32) {
	if len(gc.bg) == 0 {
		return
	}
	for k := range rows {
		gd := guides[k]
		y := lineY(rows[k].vi)
		for lvl := 1; lvl <= gd.level; lvl++ {
			x := float32((lvl-1)*tab) * colW
			if maxX > 0 && x > maxX {
				break
			}
			c := gc.bg[(lvl-1)%len(gc.bg)]
			if lvl == gd.active {
				c = gc.active[(lvl-1)%len(gc.active)]
			}
			// On whole pixels, as a browser lays out Monaco's guide boxes.
			gx := float32(math.Round(float64(tx + x)))
			l.Fill(geom.Rect{X: gx, Y: y, W: 1, H: lh}, geom.Radii{}, c)
		}
	}
}
