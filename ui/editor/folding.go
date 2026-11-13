package editor

import (
	"cmp"
	"context"
	"math"
	"regexp"
	"slices"
	"strconv"
	"sync"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
)

// Code folding (Monaco's folding contribution: FoldingModel,
// HiddenRangeModel and the fold commands).
//
// The folding ranges (folding_provider.go) are recomputed lazily, when a
// fold command or the gutter's folding controls need them after the text
// changed. A folded (collapsed) region is kept by anchors, so it follows
// every edit, local or from another view of the Model, without
// recomputing the ranges:
//
//   - its start line is that of an anchor at the end of the line (it stays
//     there when a line is inserted after it, as Monaco's decoration, which
//     starts at the line's last column); another anchor at the line's start
//     tells when the start line itself was deleted: the two meet, and the
//     region is dropped;
//   - its end line is that of an anchor at the end of its last line, which
//     grows with text typed there; a region reduced to one line is dropped.
//
// When the ranges are next computed, a folded region whose start line
// starts a range takes that range's end (it is unfolded instead when its
// end changed and a selection starts in the new range, as in Monaco's
// FoldingRegions.sanitizeAndMerge); one that starts none stays folded (a
// "recovered" region).
//
// The lines after a folded region's start line through its end are hidden:
// the view lines give them no rows (viewlines.go). Folding a region that
// holds a selection moves the selection's ends out of it, to the end of
// the start line (HiddenRangeModel.adjustSelections); a selection end that
// gets into a hidden line otherwise (a cursor move, an edit, another
// view's edit) unfolds the regions holding it (FoldingController
// .revealCursor, without its 200 ms delay; Monaco looks at the selection's
// start, the anchor, so a selection extended into a folded region keeps
// it folded). Left and Right step over a folded region as Monaco's cursor
// does in view lines (State.overFolds); moves in model positions (words,
// the document's ends, clicks, edits) may land in it and unfold it. Folding changes happen before
// the View lays out its rows: in commands, in the View's input, or when the
// State syncs to a new text.

// fold is a folded region.
type fold struct {
	start, end int // model lines, as of foldState.ver
	// s0 is at the start line's start (Right), s1 at its end (Left), e at
	// the end line's end (Right).
	s0, s1, e buffer.Anchor
	// empty: the start line was empty when folded (s0 and s1 meet anyway).
	empty bool
}

// foldState is a State's folding.
type foldState struct {
	buf   *buffer.Buffer
	ver   uint64
	folds []fold // by start line, one per line
	gen   uint64 // counts the changes of the folded regions (damage.go)

	// The folding ranges of text version rsKey.ver, of rsLines lines.
	rs      []FoldingRange
	rsKey   foldRangesKey
	rsOK    bool
	rsLines int

	provider FoldingRangeProvider // ViewW.FoldingProvider
	provGen  uint64               // counts provider changes
}

// foldRangesKey is what the folding ranges depend on.
type foldRangesKey struct {
	buf      *buffer.Buffer
	ver      uint64
	tab      int
	conf     *langs.Conf
	strategy FoldingStrategy
	provGen  uint64
}

// foldRegion is a folding range with its state, as the commands see it.
type foldRegion struct {
	start, end int
	kind       string
	collapsed  bool
	parent     int // index of the enclosing region, -1 for none
}

func (r foldRegion) contains(l int) bool { return r.start <= l && l <= r.end }

var markerCache sync.Map // *langs.Conf → *markerMatcher (nil: none)

// foldingOn reports whether folding is on: Options.Folding, and the Model
// is not a large file.
func (s *State) foldingOn() bool { return s.Options.Folding && !s.Model.LargeFile() }

// FoldingProvider sets the provider of the folding ranges (milestone 3:
// the language server), used while Options.FoldingStrategy is "auto"; nil
// restores the built-in strategy. The provider is kept by the State until
// changed: keep passing the same value (a pointer) every frame, as a new
// value makes the ranges be recomputed.
func (w *ViewW) FoldingProvider(p FoldingRangeProvider) *ViewW {
	w.st.setFoldingProvider(p)
	return w
}

// setFoldingProvider sets the folding range provider.
func (s *State) setFoldingProvider(p FoldingRangeProvider) {
	fs := &s.fold
	if !sameProvider(fs.provider, p) {
		fs.provider = p
		fs.provGen++
	}
}

// sameProvider reports whether a and b are the same provider (false for
// values that cannot be compared).
func sameProvider(a, b FoldingRangeProvider) (same bool) {
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// FoldingRanges returns the folding ranges of the current text (nil while
// folding is off), by start line.
func (s *State) FoldingRanges() []FoldingRange {
	s.lines()
	return slices.Clone(s.foldRanges())
}

// Folded returns the folded regions, by start line.
func (s *State) Folded() []FoldingRange {
	s.lines()
	out := make([]FoldingRange, 0, len(s.fold.folds))
	for _, f := range s.fold.folds {
		out = append(out, FoldingRange{Start: f.start, End: f.end})
	}
	return out
}

// foldRanges returns the folding ranges of the current text, computing
// them when the text, the language, the tab size, the strategy or the
// provider changed since. Computing them reconciles the folded regions
// (see the top of the file), which may change the hidden lines: call it
// before the View lays out its rows.
func (s *State) foldRanges() []FoldingRange {
	if !s.foldingOn() {
		return nil
	}
	s.lines() // the folded regions follow the text first
	fs := &s.fold
	snap := s.Snapshot()
	key := s.foldRangesKey()
	if fs.rsOK && fs.rsKey == key {
		return fs.rs
	}
	var rs []FoldingRange
	if key.strategy == FoldingAuto && fs.provider != nil {
		if got, err := fs.provider.FoldingRanges(context.Background(), snap); err == nil && got != nil {
			rs = sanitizeRanges(got, snap.LineCount(), foldingMaximumRegions)
		}
	}
	if rs == nil {
		conf := s.Model.Conf()
		off := conf != nil && conf.Folding.OffSide
		rs = indentRanges(snap, s.indentSize(), off, markersOf(conf), foldingMaximumRegions)
	}
	fs.rs, fs.rsKey, fs.rsOK, fs.rsLines = rs, key, true, snap.LineCount()
	s.mergeFolds()
	return fs.rs
}

// foldRangesKey returns what the folding ranges of the current text
// depend on.
func (s *State) foldRangesKey() foldRangesKey {
	return foldRangesKey{buf: s.Model.Buffer(), ver: s.Snapshot().Version(), tab: s.indentSize(), conf: s.Model.Conf(),
		strategy: s.Options.foldingStrategy(), provGen: s.fold.provGen}
}

// foldRangesFresh reports whether the folding ranges of the current text
// are computed.
func (s *State) foldRangesFresh() bool { return s.fold.rsOK && s.fold.rsKey == s.foldRangesKey() }

// syncFolds follows the folded regions to the current text (through their
// anchors) and hides their lines in the view lines, which must be synced
// to the same text.
func (s *State) syncFolds() {
	s.resolveFolds()
	s.vl.setHidden(s.fold.hidden(s.vl.n))
}

// resolveFolds moves the folded regions to the current text.
func (s *State) resolveFolds() {
	fs := &s.fold
	b, snap := s.Model.Buffer(), s.Snapshot()
	if !s.foldingOn() || fs.buf != b {
		if len(fs.folds) > 0 {
			fs.folds = nil
			fs.gen++
		}
		fs.buf, fs.ver = b, snap.Version()
		return
	}
	if fs.ver == snap.Version() {
		return
	}
	fs.ver = snap.Version()
	out := fs.folds[:0]
	changed := false
	for _, f := range fs.folds {
		o0, o1, oe := b.Resolve(f.s0), b.Resolve(f.s1), b.Resolve(f.e)
		start, end := snap.LineOf(o1), snap.LineOf(oe)
		if (o0 == o1 && !f.empty) || end <= start {
			changed = true // the start line was deleted, or one line is left
			continue
		}
		if k := len(out); k > 0 && out[k-1].start >= start {
			changed = true
			continue
		}
		// Re-anchored here, so resolving stays cheap.
		f.start, f.end = start, end
		f.s0, f.s1, f.e = b.Anchor(o0, buffer.Right), b.Anchor(o1, buffer.Left), b.Anchor(oe, buffer.Right)
		out = append(out, f)
	}
	clear(fs.folds[len(out):])
	fs.folds = out
	if changed {
		fs.gen++
	}
}

// setFolds sets the folded regions, of the current text.
func (fs *foldState) setFolds(s *State, folds []fold) {
	fs.folds = folds
	fs.buf, fs.ver = s.Model.Buffer(), s.Snapshot().Version()
}

// newFold returns the folded region of lines start..end of the current
// text.
func (s *State) newFold(start, end int) fold {
	b, snap := s.Model.Buffer(), s.Snapshot()
	ls, le := snap.LineStart(start), lineEnd(snap, start)
	return fold{start: start, end: end, empty: ls == le,
		s0: b.Anchor(ls, buffer.Right), s1: b.Anchor(le, buffer.Left), e: b.Anchor(lineEnd(snap, end), buffer.Right)}
}

// hidden returns the hidden lines of the folded regions in a document of
// n lines: the lines after each start line through its end, regions
// inside a folded one included in it.
func (fs *foldState) hidden(n int) []lineSpan {
	var out []lineSpan
	for _, f := range fs.folds {
		a, b := f.start+1, min(f.end, n-1)
		if a > b {
			continue
		}
		if k := len(out); k > 0 && out[k-1].b >= a-1 {
			if out[k-1].b < b {
				out[k-1].b = b
			}
			continue
		}
		out = append(out, lineSpan{a, b})
	}
	return out
}

// foldAt returns the index of the folded region starting at line l, -1
// for none.
func (fs *foldState) foldAt(l int) int {
	i, ok := slices.BinarySearchFunc(fs.folds, l, func(f fold, l int) int { return cmp.Compare(f.start, l) })
	if !ok {
		return -1
	}
	return i
}

// rangeAt reports whether a folding range (of the last computation)
// starts at line l.
func (fs *foldState) rangeAt(l int) bool {
	_, ok := slices.BinarySearchFunc(fs.rs, l, func(r FoldingRange, l int) int { return cmp.Compare(r.Start, l) })
	return ok
}

// mergeFolds reconciles the folded regions with freshly computed ranges
// (see the top of the file).
func (s *State) mergeFolds() {
	fs := &s.fold
	if len(fs.folds) == 0 {
		return
	}
	snap := s.Snapshot()
	changed := false
	out := fs.folds[:0]
	for _, f := range fs.folds {
		i, ok := slices.BinarySearchFunc(fs.rs, f.start, func(r FoldingRange, l int) int { return cmp.Compare(r.Start, l) })
		if ok && fs.rs[i].End != f.end {
			r := fs.rs[i]
			changed = true
			if s.selStartsIn(snap, r.Start+1, r.End+1) {
				continue // unfolded: a selection starts in it
			}
			f = s.newFold(r.Start, r.End)
		}
		out = append(out, f)
	}
	clear(fs.folds[len(out):])
	fs.folds = out
	if changed {
		s.applyFolds()
	}
}

// selStartsIn reports whether a selection starts on lines [a, b].
func (s *State) selStartsIn(snap buffer.Snapshot, a, b int) bool {
	for _, sel := range s.sels() {
		lo, _ := ordered(sel)
		if l := snap.LineOf(clampOff(snap, lo)); l >= a && l <= b {
			return true
		}
	}
	return false
}

// applyFolds hides the folded regions' lines after a change of the folded
// regions, keeping the top model line at the top (the start line when the
// top line got hidden).
func (s *State) applyFolds() {
	vl := s.lines()
	line, row := vl.find(s.Viewport.TopLine)
	vl.setHidden(s.fold.hidden(vl.n))
	s.anchorTop(line, row)
	s.clampViewport()
	s.fold.gen++
}

// regions returns the folding ranges merged with the folded regions, as
// the commands see them: by start line, properly nested, with their
// parents.
func (s *State) regions() []foldRegion {
	rs := s.foldRanges()
	s.lines()
	fs := &s.fold
	out := make([]foldRegion, 0, len(rs)+len(fs.folds))
	i, j := 0, 0
	for i < len(rs) || j < len(fs.folds) {
		switch {
		case j >= len(fs.folds) || (i < len(rs) && rs[i].Start < fs.folds[j].start):
			out = append(out, foldRegion{start: rs[i].Start, end: rs[i].End, kind: rs[i].Kind})
			i++
		case i >= len(rs) || fs.folds[j].start < rs[i].Start:
			f := fs.folds[j]
			out = append(out, foldRegion{start: f.start, end: f.end, collapsed: true})
			j++
		default:
			out = append(out, foldRegion{start: rs[i].Start, end: fs.folds[j].end, kind: rs[i].Kind, collapsed: true})
			i++
			j++
		}
	}
	// Nesting: drop the regions crossing their parent.
	kept := out[:0]
	var stack []int // indexes in kept
	for _, r := range out {
		for len(stack) > 0 && kept[stack[len(stack)-1]].end < r.start {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 && kept[stack[len(stack)-1]].end < r.end {
			continue
		}
		r.parent = -1
		if len(stack) > 0 {
			r.parent = stack[len(stack)-1]
		}
		stack = append(stack, len(kept))
		kept = append(kept, r)
	}
	return kept
}

// regionAt returns the index of the innermost region holding line l, -1
// for none (Monaco's FoldingRegions.findRange).
func regionAt(regs []foldRegion, l int) int {
	i, _ := slices.BinarySearchFunc(regs, l+1, func(r foldRegion, l int) int { return cmp.Compare(r.start, l) })
	i--
	for i >= 0 && regs[i].end < l {
		i = regs[i].parent
	}
	return i
}

// level returns the nesting level of region i (1: top level).
func level(regs []foldRegion, i int) int {
	n := 0
	for ; i >= 0; i = regs[i].parent {
		n++
	}
	return n
}

// inside returns the indexes of the regions inside region i and their
// level below it (1: its children).
func inside(regs []foldRegion, i int) (idx, lv []int) {
	base := level(regs, i)
	for k := i + 1; k < len(regs) && regs[k].start <= regs[i].end; k++ {
		idx = append(idx, k)
		lv = append(lv, level(regs, k)-base)
	}
	return idx, lv
}

// setRegions makes the collapsed regions of regs the folded regions
// (keeping the anchors of those already folded), moves the selections out
// of the hidden lines and reveals the primary selection's start (Monaco's
// FoldingAction). It reports whether anything changed.
func (s *State) setRegions(regs []foldRegion) bool {
	fs := &s.fold
	var folds []fold
	for _, r := range regs {
		if !r.collapsed {
			continue
		}
		if k := fs.foldAt(r.start); k >= 0 && fs.folds[k].end == r.end {
			folds = append(folds, fs.folds[k])
		} else {
			folds = append(folds, s.newFold(r.start, r.end))
		}
	}
	same := len(folds) == len(fs.folds)
	for k := 0; same && k < len(folds); k++ {
		same = folds[k].start == fs.folds[k].start && folds[k].end == fs.folds[k].end
	}
	if same {
		return false
	}
	fs.setFolds(s, folds)
	s.applyFolds()
	s.adjustSels()
	lo, _ := ordered(s.Primary())
	s.revealOffset(lo)
	return true
}

// adjustSels moves the selection ends on hidden lines to the end of the
// visible line before them (Monaco's HiddenRangeModel.adjustSelections).
func (s *State) adjustSels() {
	vl := s.lines()
	if len(vl.hid) == 0 {
		return
	}
	snap := s.Snapshot()
	move := func(off int) (int, bool) {
		l := snap.LineOf(clampOff(snap, off))
		if vis, hidden := vl.visibleLine(l); hidden {
			return lineEnd(snap, vis), true
		}
		return off, false
	}
	out := slices.Clone(s.sels())
	any := false
	for k, sel := range out {
		a, ma := move(sel.Anchor)
		h, mh := move(sel.Head)
		if ma || mh {
			out[k], any = Selection{Anchor: a, Head: h}, true
		}
	}
	if any {
		s.setSels(out, nil)
	}
}

// revealFolds unfolds the folded regions holding a selection's anchor on
// a hidden line (see the top of the file).
func (s *State) revealFolds() {
	if len(s.fold.folds) == 0 {
		return
	}
	vl := s.lines()
	if len(vl.hid) == 0 {
		return
	}
	snap := s.Snapshot()
	// Monaco checks the selections' selectionStartLineNumber: the
	// anchor. A selection extended into a folded region (Select All,
	// Shift+Ctrl+End) leaves it folded.
	var lines []int
	for _, sel := range s.sels() {
		if l := snap.LineOf(clampOff(snap, sel.Anchor)); vl.hidden(l) {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return
	}
	fs := &s.fold
	out := fs.folds[:0]
	for _, f := range fs.folds {
		keep := true
		for _, l := range lines {
			if f.start < l && l <= f.end {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, f)
		}
	}
	clear(fs.folds[len(out):])
	fs.folds = out
	s.applyFolds()
	s.RevealCaret()
}

// restoreFolds folds the regions starting at lines (ViewState.Folded) that
// are folding ranges of the current text.
func (s *State) restoreFolds(lines []int) {
	s.lines()
	fs := &s.fold
	fs.setFolds(s, nil)
	if !s.foldingOn() {
		s.applyFolds()
		return
	}
	rs := s.foldRanges()
	for _, l := range lines {
		if i, ok := slices.BinarySearchFunc(rs, l, func(r FoldingRange, l int) int { return cmp.Compare(r.Start, l) }); ok {
			if fs.foldAt(l) < 0 {
				fs.folds = append(fs.folds, s.newFold(rs[i].Start, rs[i].End))
				slices.SortFunc(fs.folds, func(a, b fold) int { return cmp.Compare(a.start, b.start) })
			}
		}
	}
	s.applyFolds()
}

// foldedStarts returns the start lines of the folded regions.
func (s *State) foldedStarts() []int {
	s.lines()
	out := make([]int, 0, len(s.fold.folds))
	for _, f := range s.fold.folds {
		out = append(out, f.start)
	}
	return out
}

// --- Commands ---

// selectedLines returns the start lines of the selections (Monaco's
// FoldingAction.getSelectedLines).
func (s *State) selectedLines() []int {
	snap := s.Snapshot()
	var out []int
	for _, sel := range s.sels() {
		lo, _ := ordered(sel)
		out = append(out, snap.LineOf(clampOff(snap, lo)))
	}
	return out
}

// foldLines returns the lines a fold command applies to: the args'
// "selectionLines" (0-based, as Monaco's) when given, else the selections'
// start lines.
func (s *State) foldLines(args any) []int {
	if m, ok := args.(map[string]any); ok {
		if v, ok := m["selectionLines"].([]any); ok {
			var out []int
			for _, x := range v {
				if f, ok := x.(float64); ok {
					out = append(out, int(f))
				} else if i, ok := x.(int); ok {
					out = append(out, i)
				}
			}
			return out
		}
	}
	return s.selectedLines()
}

// foldArgs returns the "levels" and "direction" of a fold command's args
// (0 and "" when not given).
func foldArgs(args any) (levels int, dir string) {
	if m, ok := args.(map[string]any); ok {
		switch v := m["levels"].(type) {
		case float64:
			levels = int(v)
		case int:
			levels = v
		}
		dir, _ = m["direction"].(string)
	}
	return levels, dir
}

// collapseLevelsDown sets the regions at lines (all regions when lines is
// nil), and those inside them less than levels deep, to collapse (Monaco's
// setCollapseStateLevelsDown).
func collapseLevelsDown(regs []foldRegion, collapse bool, levels int, lines []int, all bool) {
	if all {
		for k := range regs {
			if level(regs, k) <= levels {
				regs[k].collapsed = collapse
			}
		}
		return
	}
	for _, l := range lines {
		i := regionAt(regs, l)
		if i < 0 {
			continue
		}
		regs[i].collapsed = collapse
		if levels > 1 {
			idx, lv := inside(regs, i)
			for k, j := range idx {
				if lv[k] < levels {
					regs[j].collapsed = collapse
				}
			}
		}
	}
}

// collapseLevelsUp sets the regions at lines and their parents up to
// levels to collapse (Monaco's setCollapseStateLevelsUp).
func collapseLevelsUp(regs []foldRegion, collapse bool, levels int, lines []int) {
	for _, l := range lines {
		for i, n := regionAt(regs, l), 1; i >= 0 && n <= levels; i, n = regs[i].parent, n+1 {
			regs[i].collapsed = collapse
		}
	}
}

// foldUp folds the innermost unfolded region at each line (Monaco's
// setCollapseStateUp).
func foldUp(regs []foldRegion, lines []int) {
	// Every line's region is chosen before any is folded: several
	// cursors in one region fold it once (Monaco's toggleCollapseState
	// processes each region once).
	var fold []int
	for _, l := range lines {
		for i := regionAt(regs, l); i >= 0; i = regs[i].parent {
			if !regs[i].collapsed {
				fold = append(fold, i)
				break
			}
		}
	}
	for _, i := range fold {
		regs[i].collapsed = true
	}
}

// foldCommand registers a fold command: run changes the regions.
func foldCommand(id, label string, run func(st *State, regs []foldRegion, args any)) Command {
	return Command{ID: id, Label: label, Precondition: CtxFoldingEnabled, Run: func(st *State, args any) bool {
		if !st.foldingOn() {
			return false
		}
		regs := st.regions()
		run(st, regs, args)
		st.setRegions(regs)
		return true
	}}
}

func foldCommands() []Command {
	cs := []Command{
		foldCommand("editor.fold", "Fold", func(st *State, regs []foldRegion, args any) {
			lines := st.foldLines(args)
			levels, dir := foldArgs(args)
			switch {
			case levels == 0 && dir == "":
				foldUp(regs, lines)
			case dir == "up":
				collapseLevelsUp(regs, true, max(levels, 1), lines)
			default:
				collapseLevelsDown(regs, true, max(levels, 1), lines, false)
			}
		}),
		foldCommand("editor.unfold", "Unfold", func(st *State, regs []foldRegion, args any) {
			lines := st.foldLines(args)
			levels, dir := foldArgs(args)
			if dir == "up" {
				collapseLevelsUp(regs, false, max(levels, 1), lines)
			} else {
				collapseLevelsDown(regs, false, max(levels, 1), lines, false)
			}
		}),
		foldCommand("editor.toggleFold", "Toggle Fold", func(st *State, regs []foldRegion, _ any) {
			// Each region once, however many cursors are in it.
			done := map[int]bool{}
			for _, l := range st.selectedLines() {
				if i := regionAt(regs, l); i >= 0 && !done[i] {
					done[i] = true
					regs[i].collapsed = !regs[i].collapsed
				}
			}
		}),
		foldCommand("editor.foldRecursively", "Fold Recursively", func(st *State, regs []foldRegion, _ any) {
			collapseLevelsDown(regs, true, math.MaxInt, st.selectedLines(), false)
		}),
		foldCommand("editor.foldAll", "Fold All", func(_ *State, regs []foldRegion, _ any) {
			collapseLevelsDown(regs, true, math.MaxInt, nil, true)
		}),
		foldCommand("editor.unfoldAll", "Unfold All", func(_ *State, regs []foldRegion, _ any) {
			collapseLevelsDown(regs, false, math.MaxInt, nil, true)
		}),
		foldCommand("editor.foldAllBlockComments", "Fold All Block Comments", func(st *State, regs []foldRegion, _ any) {
			if hasKinds(regs) {
				foldKind(regs, FoldingKindComment)
				return
			}
			if conf := st.Model.Conf(); conf != nil && conf.BlockComment[0] != "" {
				foldMatching(st, regs, regexp.MustCompile(`^\s*`+regexp.QuoteMeta(conf.BlockComment[0])).MatchString)
			}
		}),
		foldCommand("editor.foldAllMarkerRegions", "Fold All Regions", func(st *State, regs []foldRegion, _ any) {
			if hasKinds(regs) {
				foldKind(regs, FoldingKindRegion)
				return
			}
			if conf := st.Model.Conf(); conf != nil {
				if re := compileTest(conf.Folding.Start); re != nil {
					foldMatching(st, regs, func(s string) bool { return test(re, s) })
				}
			}
		}),
	}
	for n := 1; n <= 7; n++ {
		cs = append(cs, foldCommand("editor.foldLevel"+strconv.Itoa(n), "Fold Level "+strconv.Itoa(n), func(st *State, regs []foldRegion, _ any) {
			// Monaco's setCollapseStateAtLevel: the regions at level n
			// holding no selection's start line.
			blocked := st.selectedLines()
			for k := range regs {
				if regs[k].collapsed || level(regs, k) != n {
					continue
				}
				if !slices.ContainsFunc(blocked, regs[k].contains) {
					regs[k].collapsed = true
				}
			}
		}))
	}
	return cs
}

// hasKinds reports whether some region has a kind (Monaco's
// FoldingRegions.hasTypes).
func hasKinds(regs []foldRegion) bool {
	return slices.ContainsFunc(regs, func(r foldRegion) bool { return r.kind != "" })
}

// foldKind folds the regions of kind (Monaco's setCollapseStateForType).
func foldKind(regs []foldRegion, kind string) {
	for k := range regs {
		if regs[k].kind == kind {
			regs[k].collapsed = true
		}
	}
}

// foldMatching folds the regions whose start line matches (Monaco's
// setCollapseStateForMatchingLines).
func foldMatching(st *State, regs []foldRegion, match func(string) bool) {
	snap := st.Snapshot()
	for k := range regs {
		if !regs[k].collapsed && match(snap.Line(regs[k].start)) {
			regs[k].collapsed = true
		}
	}
}

func init() {
	for _, c := range foldCommands() {
		RegisterCommand(c)
	}
}

// toggleFoldAt folds or unfolds the region starting at line l, as a click
// on its folding control (recursive: Shift, Monaco's onEditorMouseUp). It
// reports whether a region starts there.
func (s *State) toggleFoldAt(l int, recursive bool) bool {
	if !s.foldingOn() {
		return false
	}
	regs := s.regions()
	i := regionAt(regs, l)
	if i < 0 || regs[i].start != l {
		return false
	}
	collapsed := regs[i].collapsed
	toggle := []int{}
	if recursive {
		idx, _ := inside(regs, i)
		for _, j := range idx {
			if regs[j].collapsed == collapsed {
				toggle = append(toggle, j)
			}
		}
	}
	if collapsed || !recursive || len(toggle) == 0 {
		toggle = append(toggle, i)
	}
	for _, j := range toggle {
		regs[j].collapsed = !regs[j].collapsed
	}
	s.setRegions(regs)
	return true
}

// foldPrepare is the folding work of the View's paint before it lays out
// its rows: it drops the folded regions when folding went off and, when
// the gutter shows every folding control (all), computes the folding
// ranges (which reconciles the folded regions).
func (s *State) foldPrepare(all bool) {
	s.lines()
	if !s.foldingOn() && len(s.fold.folds) > 0 {
		s.fold.folds = nil
		s.applyFolds()
	}
	if all {
		s.foldRanges()
	}
}
