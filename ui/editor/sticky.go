package editor

import (
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/editor/internal/frameclock"
	"unicode/utf8"
)

// StickyScrollOptions configures stickyScroll (enabled: true, maxLineCount: 5).
// Scopes come from the editor's folding ranges, including its provider.
type StickyScrollOptions struct {
	Enabled      bool
	MaxLineCount int
}

func (w *ViewW) stickyOn() bool { return w.opts.StickyScroll.Enabled && w.st.foldingOn() }

// prepareSticky runs before row layout. Reading the already prepared folding
// ranges never runs a provider or measures a view line after layout. It
// decides the sticky lines and their places; layoutStickyRows lays them
// out once the frame's bracket colors are known.
func (w *ViewW) prepareSticky(snap buffer.Snapshot) {
	vs, st := w.vs, w.st
	vs.stickyLines = vs.stickyLines[:0]
	vs.stickyY = vs.stickyY[:0]
	vs.stickyClips = vs.stickyClips[:0]
	vs.stickyRect = geom.Rect{}
	fs := &st.fold
	if !w.stickyOn() || !fs.rsOK || fs.rsKey.buf != st.Model.Buffer() || (fs.rsKey.ver != snap.Version() && fs.rsLines != snap.LineCount()) {
		return
	}
	limit := w.opts.StickyScroll.MaxLineCount
	if limit <= 0 {
		limit = 5
	}
	limit = min(limit, 20, max(0, int(vs.text.H/w.lineHeight())-1))
	if limit == 0 {
		return
	}
	lh := w.lineHeight()
	scroll := float32(st.Viewport.scrollY())
	var parents []int
	for _, scope := range fs.rs {
		for len(parents) > 0 && parents[len(parents)-1] < scope.Start {
			parents = parents[:len(parents)-1]
		}
		slot := float32(len(parents)) * lh
		parents = append(parents, scope.End)
		if st.vl.hidden(scope.Start) || fs.foldAt(scope.Start) >= 0 {
			continue
		}
		startY := float32(st.vl.viewOf(scope.Start))*lh - scroll
		if startY >= slot {
			break
		}
		last := w.navigationRow(scope.End)
		endY := float32(last+st.vl.rowsOf(scope.End))*lh - scroll
		if st.vl.hidden(scope.End) {
			endY = float32(last+1)*lh - scroll
		}
		if endY <= slot {
			continue
		}
		y := min(slot, endY-lh)
		clip := geom.Rect{X: vs.gutter.X, Y: vs.text.Y + slot, W: vs.gutter.W + vs.text.W, H: min(lh, endY-slot)}
		vs.stickyLines = append(vs.stickyLines, scope.Start)
		vs.stickyY = append(vs.stickyY, vs.text.Y+y)
		vs.stickyClips = append(vs.stickyClips, clip)
		if len(vs.stickyLines) >= limit {
			break
		}
	}
	if len(vs.stickyLines) == 0 {
		return
	}
	bottom := vs.stickyClips[len(vs.stickyClips)-1].Bottom()
	vs.stickyRect = geom.Rect{X: vs.gutter.X, Y: vs.text.Y, W: vs.gutter.W + vs.text.W, H: bottom - vs.text.Y}
}

// layoutStickyRows lays out the sticky lines prepareSticky chose.
func (w *ViewW) layoutStickyRows(snap buffer.Snapshot, th *theme.Theme) []lineView {
	vs := w.vs
	if len(vs.stickyLines) == 0 {
		return nil
	}
	rows := make([]lineView, 0, len(vs.stickyLines))
	for _, i := range vs.stickyLines {
		lv := w.layoutSticky(snap, i, th)
		if lv.long != nil {
			w.layoutChunks(&lv, th, vs.colW)
		}
		lv.vi = len(rows)
		rows = append(rows, lv)
	}
	for i := range vs.stickyTokens {
		keep := false
		for _, j := range vs.stickyLines {
			keep = keep || i == j
		}
		if !keep {
			delete(vs.stickyTokens, i)
		}
	}
	return rows
}

type stickyTokenLine struct {
	text   string
	lexer  *monarch.Lexer
	start  monarch.State
	final  bool
	tokens []monarch.Token
}

// Sticky headers have a small independent token cache, not a Highlighter
// window: jumping between a far-away scope start and the viewport must not
// evict and re-tokenize all the visible lines on every frame.
func (w *ViewW) layoutSticky(snap buffer.Snapshot, i int, th *theme.Theme) lineView {
	s := w.st.Model.line(snap, i) // a long line: the frame's one copy (lineMemo)
	if isLong(s) {
		return w.layoutLine(snap, i, nil, th, w.vs.colW)
	}
	var toks []monarch.Token
	if hl := w.st.Highlighter(); hl != nil {
		start, _ := hl.StartState(snap, i)
		final := hl.FinalBefore(snap, i+1)
		lexer := hl.LexerIdentity()
		cached := w.vs.stickyTokens[i]
		if got, ok := hl.PeekTokens(snap, i, nil); ok {
			toks = got
		} else if cached != nil && cached.text == s && cached.lexer == lexer && cached.start == start && cached.final == final {
			toks = cached.tokens
		} else if frameclock.Now().Before(w.vs.highlightDeadline) {
			toks = hl.TokenizeSpan(snap, i, s, nil)
		} // Otherwise use plain text until the host's next highlight-ready frame.
		if toks != nil {
			if w.vs.stickyTokens == nil {
				w.vs.stickyTokens = map[int]*stickyTokenLine{}
			}
			w.vs.stickyTokens[i] = &stickyTokenLine{s, lexer, start, final, toks}
		}
	}
	nr := utf8.RuneCountInString(s)
	sg := w.shapeSeg(i, s, 0, nr, 0, toks, th)
	return lineView{i: i, vi: i, rows: 1, text: s, runes: nr, b1: len(s), r1: nr, segs: []seg{sg}, endX: sg.lay.Width}
}

func (w *ViewW) paintSticky(l *paint.List, rows []lineView, th *theme.Theme) {
	vs := w.vs
	if len(rows) == 0 {
		return
	}
	r := vs.stickyRect
	pf := paintFrame{g: damageGlobal{opts: w.opts}, snap: w.st.Snapshot(), lines: rows, caretLine: w.st.Snapshot().LineOf(w.st.Primary().Head)}
	keys := w.rowHashes(&pf, nil)
	bg := defaultColor(th, "editorStickyScroll.background")
	if bg.A == 0 {
		bg = defaultColor(th, "editor.background")
	}
	l.Clip(r, geom.Radii{})
	l.Fill(r, geom.Radii{}, bg)
	for k, lv := range rows {
		clip := vs.stickyClips[k]
		vs.navPaint = append(vs.navPaint, fixedPaint{clip, keys[k] ^ uint64(math.Float32bits(w.st.Viewport.ScrollX)) ^ uint64(math.Float32bits(vs.stickyY[k]))<<32})
		l.Clip(clip, geom.Radii{})
		l.Clip(vs.text.Intersect(clip), geom.Radii{})
		for _, sg := range lv.segs {
			drawSeg(l, sg, vs.text.X-w.st.Viewport.ScrollX, vs.stickyY[k])
		}
		l.Pop()
		l.Clip(vs.gutter.Intersect(clip), geom.Radii{})
		w.paintGutter(l, vs.gutter.X, vs.gutter.W, []lineView{lv}, nil, func(int) float32 { return vs.stickyY[k] }, w.st.Snapshot().LineCount(), pf.caretLine, digitsOf(w.monoStyle()), bg, nil, color.NRGBA{}, false, color.NRGBA{})
		l.Pop()
		l.Pop()
	}
	l.Pop()
}
