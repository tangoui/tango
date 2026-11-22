package ui

import (
	"maps"
	"math/bits"
	"slices"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

type mdWindow struct {
	heights []float32
	sums    []float64 // Fenwick tree of block heights
	built   []*node
	width   float32
	key     mdWindowKey
	jump    int
	hasJump bool
}

type mdWindowKey struct {
	locale            *Locale
	flavor            theme.Flavor
	font              string
	weight            int
	fonts             [8]float32 // body, code, headings
	lineHeights       [9]float32 // font sizes above, plus the code header
	checkWidth        float32
	rtl, copy, images bool
	hideCodeLang      bool
	rules             uint8
	rulesSet          bool
}

func (m *MarkdownW) windowKey() mdWindowKey {
	th := m.c.Theme
	key := mdWindowKey{
		locale: m.c.Locale(), flavor: th.Flavor, font: th.FontFamily, weight: th.FontWeightStrong,
		rtl: m.c.IsRTL(), copy: m.copyable, images: m.images != nil || m.resolveImage != nil,
		hideCodeLang: m.hideCodeLang,
		rules:        m.headingRules, rulesSet: m.rulesSet,
	}
	key.fonts[0], key.fonts[1] = m.fontSize(), m.codeFontSize()
	for i := range 6 {
		key.fonts[i+2] = m.headingSize(i + 1)
	}
	for i, size := range key.fonts {
		key.lineHeights[i] = th.LineHeightFor(size)
	}
	key.lineHeights[8] = th.LineHeightFor(12)
	check := CheckboxW{c: m.c, size: m.c.defaultSize()}
	check.configDefaults()
	key.checkWidth = m.c.boxSizeOf(check.size)
	return key
}

func (w *mdWindow) prefix(n int) float64 {
	var total float64
	for ; n > 0; n -= n & -n {
		total += w.sums[n]
	}
	return total
}

func (w *mdWindow) blockTop(i int, pad geom.Insets) float64 {
	if i == 0 {
		return 0
	}
	return w.prefix(i) + float64(pad.Top)
}

// find returns the block containing y, clamped to the document.
func (w *mdWindow) find(y float64) int {
	n := 0
	for step := 1 << (bits.Len(uint(len(w.heights))) - 1); step > 0; step >>= 1 {
		i := n + step
		if i < len(w.sums) && w.sums[i] <= y {
			n = i
			y -= w.sums[i]
		}
	}
	return min(n, len(w.heights)-1)
}

func (w *mdWindow) measure(i int, h float32) bool {
	if h == w.heights[i] {
		return false
	}
	delta := float64(h) - float64(w.heights[i])
	w.heights[i] = h
	for j := i + 1; j < len(w.sums); j += j & -j {
		w.sums[j] += delta
	}
	return true
}

func (w *mdWindow) reset(m *MarkdownW, blocks []*mdBlock, old []float32, head, tail int) {
	w.key = m.windowKey()
	w.heights = make([]float32, len(blocks))
	w.sums = make([]float64, len(blocks)+1)
	line := m.c.Theme.LineHeightFor(m.fontSize())
	for i, b := range blocks {
		h := line + m.fontSize()
		switch b.kind {
		case mdHeading:
			h = m.headingSize(b.level) * 2
		case mdList:
			h = max(1, float32(len(b.items)))*(line+4) + m.fontSize()
		case mdTable:
			h = float32(len(b.rows)+1)*(line+16) + m.fontSize()
		case mdCode:
			h = float32(strings.Count(b.text, "\n")+1)*line + 45
		case mdImage:
			h = 2*line + m.fontSize()
		}
		switch {
		case i < head:
			h = old[i]
		case i >= len(blocks)-tail:
			h = old[len(old)-(len(blocks)-i)]
		}
		w.heights[i] = h
		j := i + 1
		w.sums[j] += float64(h)
		if parent := j + j&-j; parent < len(w.sums) {
			w.sums[parent] += w.sums[j]
		}
	}
}

func (w *mdWindow) updateDocument(m *MarkdownW, old, next []*mdBlock, prevRefs, nextRefs map[string]string) {
	head, tail := 0, 0
	// First/last position affects margins. Keep geometry only for blocks
	// whose spacing and reference definitions still match.
	if w.key == m.windowKey() && len(w.heights) == len(old) && maps.Equal(prevRefs, nextRefs) {
		limit := min(len(old), len(next))
		for head < limit && (head == len(old)-1) == (head == len(next)-1) && mdSameBlock(old[head], next[head]) {
			head++
		}
		for tail < limit-head {
			i, j := len(old)-1-tail, len(next)-1-tail
			if (i == 0) != (j == 0) || !mdSameBlock(old[i], next[j]) {
				break
			}
			tail++
		}
	}
	w.reset(m, next, w.heights, head, tail)
}

func mdSameBlock(a, b *mdBlock) bool {
	return a.kind == b.kind && a.level == b.level && a.text == b.text && a.src == b.src &&
		a.lang == b.lang && a.ordered == b.ordered && a.start == b.start &&
		slices.Equal(a.header, b.header) && slices.Equal(a.aligns, b.aligns) &&
		slices.EqualFunc(a.rows, b.rows, slices.Equal[[]string]) &&
		slices.EqualFunc(a.kids, b.kids, mdSameBlock) &&
		slices.EqualFunc(a.items, b.items, func(a, b *mdItem) bool {
			return a.task == b.task && slices.EqualFunc(a.blocks, b.blocks, mdSameBlock)
		})
}

func (w *mdWindow) build(m *MarkdownW, blocks []*mdBlock) {
	c := m.c
	m.b.n.ln.ScrollY = true
	sb := c.finishScroll(m.b, nil)
	if len(blocks) == 0 {
		return
	}
	if len(w.heights) != len(blocks) || w.key != m.windowKey() {
		w.reset(m, blocks, nil, 0, 0)
	}
	s, pad := sb.s, sb.n.ln.Padding
	view := fillViewH(sb, c.size.H)
	s.extent.Y = w.prefix(len(blocks)) + float64(pad.Vertical())
	endOffset := max(0, s.extent.Y-view)
	atEnd := s.off.Y > 0 && (s.off.Y >= endOffset || s.off.Y >= s.maxY())
	jump, hasJump := w.jump, w.hasJump
	if hasJump {
		s.off.Y = w.blockTop(jump, pad)
		atEnd = false
	}
	fillClampOff(sb, s.extent.Y, view)
	if atEnd {
		s.off.Y = endOffset
	}
	off := s.off.Y - float64(pad.Top)
	first := max(0, w.find(off)-fillWinOverscan)
	end := min(len(blocks), w.find(off+view)+1+fillWinOverscan)
	s.base.Y = w.prefix(first)
	w.built = w.built[:0]
	for i := first; i < end; i++ {
		Key(c, i, func() {
			bw := Column(c, func() {
				m.block(blocks[i], Span{Color: m.col}, i, i == len(blocks)-1)
			})
			w.built = append(w.built, bw.n)
		})
	}
	over := sb.n.over
	sb.n.over = func(l *paint.List, r geom.Rect) {
		over(l, r)
		if hasJump && w.jump == jump {
			w.hasJump = false
		}
		anchor := w.find(s.off.Y - float64(pad.Top))
		before := w.prefix(anchor)
		width := max(0, r.W-pad.Horizontal())
		changed := w.width != width
		if changed {
			w.width = width
			w.reset(m, blocks, nil, 0, 0)
		}
		for j, n := range w.built {
			changed = w.measure(first+j, max(1, n.ln.Rect.H)) || changed
		}
		if changed || view != float64(r.H) {
			if hasJump || atEnd {
				// The target and the end may both move as estimates settle.
				s.off.Y = max(0, w.prefix(len(blocks))+float64(pad.Vertical())-float64(r.H))
				if hasJump {
					s.off.Y = min(w.blockTop(jump, pad), s.off.Y)
				}
			} else if anchor > 0 {
				s.off.Y += w.prefix(anchor) - before
			}
			c.WakeAt(c.now.Add(time.Millisecond))
		}
	}
}
