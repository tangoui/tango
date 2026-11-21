package terminal

import (
	"regexp"
	"sort"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// LinkMatch identifies a link in a logical line by its UTF-8 byte offsets
// [From, To). An empty Target uses the matched text.
type LinkMatch struct {
	From, To int
	Target   string
}

// LinkHit is a matched target and its range in the terminal grid.
type LinkHit struct {
	Target string
	Range  CellRange
}

// LinkMatcher replaces the standard URL and path matcher for this view.
// The function receives the whole logical line. Return matches in
// priority order. A nil matcher restores standard matching.
func (w *ViewW) LinkMatcher(match func(string) []LinkMatch) *ViewW {
	w.matchLink = match
	return w
}

// OnOpenLink sets the callback for a primary click on a link. A
// drag or a modified click selects text instead. The application opens
// the target, which may be a URL or a filesystem path.
func (w *ViewW) OnOpenLink(fn func(target string)) *ViewW {
	w.openLink = fn
	return w
}

// LinkAt returns the link under p in window coordinates, independent of
// whether OnOpenLink is set.
func (w *ViewW) LinkAt(p geom.Point) (LinkHit, bool) {
	hit := w.linkAt(w.box.Rect(), p)
	return hit, hit.Target != ""
}

func (w *ViewW) linkAt(r geom.Rect, p geom.Point) LinkHit {
	v := w.t.Lock()
	s := v.Screen()
	cell, ok := w.gridCell(s, r, p)
	if !ok || s.ScrollbackLen() != 0 && p.X >= r.Right()-scrollbarWidth {
		w.t.Unlock()
		return LinkHit{}
	}
	line := logicalLine(s, cell.Row)
	w.t.Unlock()
	i := sort.Search(len(line.cells), func(i int) bool {
		return !positionLess(line.cells[i].span.Start, cell)
	})
	if i == len(line.cells) || line.cells[i].span.Start != cell {
		return LinkHit{}
	}
	under := line.cells[i]
	match := w.matchLink
	if match == nil {
		match = defaultLinkMatches
	}
	for _, candidate := range match(line.Text) {
		span, ok := line.Range(candidate.From, candidate.To)
		if !ok || candidate.From >= under.to || candidate.To <= under.from {
			continue
		}
		target := candidate.Target
		if target == "" {
			target = line.Text[candidate.From:candidate.To]
		}
		return LinkHit{target, span}
	}
	return LinkHit{}
}

func positionLess(a, b CellPosition) bool {
	return a.Row < b.Row || a.Row == b.Row && a.Column < b.Column
}

var linkURL = regexp.MustCompile(`(?i)\b(?:https?|ftp|file)://[^\s<>"']+`)
var linkPath = regexp.MustCompile(`(?:^|[\s(<"'=])((?:[A-Za-z]:[/\\]|\\\\|/|\.{1,2}[/\\]|~/)[^\s<>"'|]+|[\pL\pN_.-]+(?:[/\\][\pL\pN_.:@+~-]+)+)`)

func defaultLinkMatches(line string) []LinkMatch {
	var matches []LinkMatch
	add := func(from, to int) {
		value := strings.TrimRight(line[from:to], ".,;!?")
		for value != "" {
			last := value[len(value)-1]
			i := strings.IndexByte(")]}", last)
			if i < 0 || strings.Count(value, string(last)) <= strings.Count(value, string("([{"[i])) {
				break
			}
			value = value[:len(value)-1]
		}
		if value != "" {
			matches = append(matches, LinkMatch{From: from, To: from + len(value)})
		}
	}
	for _, m := range linkURL.FindAllStringIndex(line, -1) {
		add(m[0], m[1])
	}
	for _, m := range linkPath.FindAllStringSubmatchIndex(line, -1) {
		add(m[2], m[3])
	}
	return matches
}

func (w *ViewW) linkInput(p pending, r geom.Rect) []effect {
	vs := w.vs
	if w.openLink == nil {
		vs.linkPress = LinkHit{}
		return nil
	}
	if p.press {
		vs.linkPress = LinkHit{}
		if p.clicks == 1 && !p.modified {
			vs.linkPress = w.linkAt(r, p.pressPos)
			vs.linkPressPos = p.pressPos
		}
	}
	if p.drag && p.dragPos != vs.linkPressPos {
		vs.linkPress = LinkHit{}
	}
	if p.release {
		pressed := vs.linkPress
		vs.linkPress = LinkHit{}
		if p.clicked && pressed.Target != "" && w.linkAt(r, p.releasePos) == pressed {
			a, b := p.releasePos, vs.linkPressPos
			if dx, dy := a.X-b.X, a.Y-b.Y; dx*dx+dy*dy <= 9 {
				return []effect{{isLink: true, copy: pressed.Target}}
			}
		}
	}
	return nil
}

func (w *ViewW) paintLink(f *frame, first, rows, cols int, hit LinkHit) {
	start, end := hit.Range.Start, hit.Range.End
	h := 1 / f.m.scale
	for row := max(start.Row, first); row <= min(end.Row, first+rows-1); row++ {
		lo, hi := 0, cols
		if row == start.Row {
			lo = start.Column
		}
		if row == end.Row {
			hi = end.Column
		}
		rect := f.cellRect(lo, hi, f.r.Y+float32(row-first)*f.m.lineH)
		rect.Y, rect.H = rect.Bottom()-h, h
		f.l.Fill(rect, geom.Radii{}, f.sch.Foreground)
	}
}

func (w *ViewW) hoverLink(f *frame, first, rows, cols int) LinkHit {
	if !w.ia.Hovered || w.openLink == nil || w.vs.sel.dragging {
		return LinkHit{}
	}
	p, over := w.c.Pointer()
	if !over {
		return LinkHit{}
	}
	hit := w.linkAt(f.r, p)
	if hit.Target != "" {
		w.box.Cursor(input.CursorPointer)
		f.l.Clip(f.r, geom.Radii{})
		w.paintLink(f, first, rows, cols, hit)
		f.l.Pop()
	}
	return hit
}
