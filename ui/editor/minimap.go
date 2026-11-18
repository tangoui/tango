package editor

import (
	"image/color"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/editor/internal/frameclock"
	"unicode/utf8"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// MinimapSide is minimap.side ("right" by default).
type MinimapSide string

const (
	MinimapRight MinimapSide = "right"
	MinimapLeft  MinimapSide = "left"
)

// MinimapSize is minimap.size ("proportional" by default).
type MinimapSize string

const (
	MinimapProportional MinimapSize = "proportional"
	MinimapFit          MinimapSize = "fit"
	MinimapFill         MinimapSize = "fill"
)

// MinimapShowSlider is minimap.showSlider ("mouseover" by default).
type MinimapShowSlider string

const (
	MinimapSliderMouseOver MinimapShowSlider = "mouseover"
	MinimapSliderAlways    MinimapShowSlider = "always"
)

// MinimapOptions configures minimap. RenderCharacters (default true) draws
// Monaco's minimap character shapes (minimap_chars.go); off, each
// character is a token-colored block.
type MinimapOptions struct {
	Enabled          bool              // minimap.enabled: true
	Side             MinimapSide       // minimap.side: "right"
	Size             MinimapSize       // minimap.size: "proportional"
	MaxColumn        int               // minimap.maxColumn: 120
	RenderCharacters bool              // minimap.renderCharacters: true (false: blocks)
	ShowSlider       MinimapShowSlider // minimap.showSlider: "mouseover"
}

type miniBlock struct {
	a, b  int
	color color.NRGBA
}
type miniLine struct {
	text string
	// The source text was made from (miniatureSource, trimmed): its
	// length, its first bytes text depends on (miniatureDep: about text
	// plus a character, not the whole lookahead) and whether the line
	// goes on after it. A source matching all three gives the same text.
	dep    string
	rawLen int
	short  bool
	start  monarch.State
	final  bool
	tokens []monarch.Token
	blocks []miniBlock
	base   int         // byte of the line where text starts, for tokens
	bmp    *miniBitmap // the characters (minimap_chars.go; nil: none)
	// bmpDone: bmp is drawn (characters were on).
	bmpDone bool
	// spanned: tokens are text's own from start (TokenizeSpan; the
	// line's tokens were not cached), so they depend on text and start
	// only.
	spanned bool
	// seen is the snapshot version text was last found current at, for
	// line seenLine (prepareMinimap skips reading the source of a line no
	// edit since touched). The line is part of it: a row moved to another
	// line (takeMoved) is still its old line's row until that is visited.
	seen     uint64
	seenLine int
}
type minimapState struct {
	cache       map[int]*miniLine
	theme       *theme.Theme
	language    string
	lexer       *monarch.Lexer
	buf         *buffer.Buffer
	column, tab int
	rebuilt     uint64 // cache-line builds, also used by incremental regression tests
	sourceReads uint64 // rows whose source text was read (tests)
	touched     []int  // scratch: the lines the last edit touched (miniSourceCarry)
	idle        int    // frames in a row that left rows and built none
	// Query scratch belongs to this view, never to a cached miniature row.
	// Cached tokens below are cloned before the next row reuses this slice.
	tokenScratch []monarch.Token
	keep         map[int]bool
	// byText indexes the previous frame's rows by miniature text, so a row
	// whose line number changed (a line inserted or deleted above it) is
	// found again; it is reused only when nothing else about it changed.
	// It is built lazily on a frame's first miss: a frame where every row
	// is found by its key (typing, the caret blinking) never builds it.
	byText  map[string][]*miniLine
	indexed bool // byText holds this frame's previous rows
	misses  int  // rows this frame the key did not find (takeMoved)
	// The character minimap (minimap_chars.go): the glyph cells in use,
	// the strip images by strip number, whether the strips are numbered
	// by sample (rows thinner than a pixel), the last composition number,
	// and counts of line bitmaps drawn and strips composed (tests).
	glyphs         *miniGlyphs
	strips         map[int]*miniStrip
	stripWants     []miniStripWant // cleared before reuse; at most the visible strips
	stripSampling  bool
	stripGen       uint64
	charRendered   uint64
	stripsComposed uint64
}

// navigationGeometry reserves fixed rails before wrapping is measured.
func (w *ViewW) navigationGeometry(r geom.Rect, gw float32) {
	vs := w.vs
	vs.minimapRect, vs.overviewRect, vs.minimapSlider = geom.Rect{}, geom.Rect{}, geom.Rect{}
	rw := float32(0)
	if w.opts.OverviewRulerLanes > 0 {
		rw = min(12, r.W)
	}
	vs.overviewRect = geom.Rect{X: r.Right() - rw, Y: r.Y, W: rw, H: r.H}
	mw := float32(0)
	if w.opts.Minimap.Enabled && !w.st.LargeFile() {
		cols := w.miniColumns()
		mw = min(float32(cols), max(0, (r.W-rw-gw)/3))
	}
	left := w.opts.Minimap.Side == MinimapLeft
	x := r.X
	if mw > 0 {
		// Fixed and scrolling surfaces must not share a device pixel: dirty
		// miniature rows would otherwise merge with an exposed scroll strip
		// into a full-height redraw at fractional layout widths.
		scale := max(0.01, w.c.Window().Scale())
		snapX := func(v float32) float32 { return float32(math.Round(float64(v*scale))) / scale }
		mx := snapX(r.Right() - rw - mw)
		mw = r.Right() - rw - mx
		if left {
			mx = x
			x = snapX(x + mw)
			mw = x - mx
		}
		vs.minimapRect = geom.Rect{X: mx, Y: r.Y, W: mw, H: r.H}
	}
	vs.gutter = geom.Rect{X: x, Y: r.Y, W: min(gw, max(0, r.W-rw-mw)), H: r.H}
	vs.text = geom.Rect{X: vs.gutter.Right(), Y: r.Y, W: max(0, r.W-rw-mw-vs.gutter.W), H: r.H}
}
func (w *ViewW) miniColumns() int {
	if w.opts.Minimap.MaxColumn <= 0 {
		return 120
	}
	return min(10000, w.opts.Minimap.MaxColumn)
}

// miniatureLine ends at a grapheme boundary on the display-column grid.
func miniatureLine(s string, columns, tab int) string {
	col := 0
	for i := 0; i < len(s); {
		j := nextGrapheme(s, i)
		next := advanceCol(col, s[i:j], tab)
		if next > columns {
			return s[:i]
		}
		col = next
		i = j
		if col >= columns {
			return s[:i]
		}
	}
	return s
}

// boundedMiniature reads a bounded rope slice before inspecting text. A giant
// combining cluster is omitted, not scanned past the bounded lookahead or split.
func boundedMiniature(snap buffer.Snapshot, line, from, columns, tab int) string {
	raw, short := miniatureSource(snap, line, from, columns)
	return miniatureOf(raw, short, columns, tab)
}

// miniatureSource is the bounded slice boundedMiniature reads: the line
// from byte from, at most columns*4+256 bytes; short reports that the
// line goes on after it. The miniature is miniatureOf(raw, short, ...), a
// function of these two only, so a minimap row whose source is unchanged
// keeps its miniature without walking it again.
func miniatureSource(snap buffer.Snapshot, line, from, columns int) (raw string, short bool) {
	start := snap.LineStart(line) + from
	end := lineEnd(snap, line)
	start = min(start, end)
	cut := min(end, start+columns*4+256)
	raw = snap.Slice(start, cut)
	countAdd(&workMiniBytes, len(raw))
	return raw, cut < end
}

// miniatureOf is the miniature of the source raw (miniatureSource): the
// prefix of raw ending at a grapheme boundary on the display-column grid
// (miniatureLine), and when short, before the last cluster of raw, which
// might continue past the lookahead (and before trailing bytes of an
// incomplete character). It walks raw only up to the columns it keeps.
func miniatureOf(raw string, short bool, columns, tab int) string {
	text, _ := miniatureDep(trimMiniSource(raw, short), short, columns, tab)
	return text
}

// trimMiniSource drops, when short, the trailing bytes of an incomplete
// character from raw.
func trimMiniSource(raw string, short bool) string {
	if short {
		for len(raw) > 0 {
			r, n := utf8.DecodeLastRuneInString(raw)
			if r != utf8.RuneError || n != 1 || raw[len(raw)-1] < 0x80 {
				break
			}
			raw = raw[:len(raw)-1]
		}
	}
	return raw
}

// miniatureDep is miniatureOf of a trimmed source; dep is how many leading
// bytes of raw the result depends on (with len(raw) and short): the
// clusters walked and the character after the last one, which
// nextGrapheme reads to end it. Another source with the same length, the
// same short and the same first dep bytes has the same miniature.
func miniatureDep(raw string, short bool, columns, tab int) (text string, dep int) {
	depAt := func(j int) int { return min(len(raw), j+utf8.UTFMax) }
	col := 0
	for i := 0; i < len(raw); {
		j := nextGrapheme(raw, i)
		countAdd(&workMiniScan, j-i)
		if short && j == len(raw) {
			// The last cluster might continue outside our lookahead.
			// Exclude it.
			return strings.Clone(raw[:i]), j
		}
		next := advanceCol(col, raw[i:j], tab)
		if next > columns {
			return strings.Clone(raw[:i]), depAt(j)
		}
		col = next
		i = j
		if col >= columns {
			return strings.Clone(raw[:i]), depAt(j)
		}
	}
	return strings.Clone(raw), len(raw)
}

// A miniature is a scaled view-row canvas. The scrolling proportional window
// follows the ordinary viewport until its bottom reaches the document end;
// beyond that point it remains fixed while the slider shrinks to the last row.
func (w *ViewW) miniCanvas() (unit, offset, extent float32) {
	r := w.vs.minimapRect
	n := max(1, w.st.vl.Count())
	v := w.st.Viewport
	unit = 2
	switch w.opts.Minimap.Size {
	case MinimapFit:
		unit = min(unit, r.H/float32(n))
	case MinimapFill:
		unit = r.H / float32(n)
	}
	unit = max(unit, 0.000001)
	extent = min(r.H, float32(n)*unit)
	offset = w.miniWindow(float32(v.scrollY())/max(1, v.LineHeight), unit)
	return
}
func (w *ViewW) miniWindow(scroll, unit float32) float32 {
	n := float32(w.st.vl.Count())
	shown := w.vs.minimapRect.H / unit
	if n <= shown {
		return 0
	}
	visible := w.st.Viewport.Height / max(1, w.st.Viewport.LineHeight)
	return min(n-shown, max(0, scroll)*(n-shown)/max(1, n-visible))
}

// prepareMinimap builds only the miniature rows that can be drawn. Its own
// prefix tokens never enter the Highlighter's windows. All foreground
// tokenization, including the minimap, shares the frame's deadline.
func (w *ViewW) prepareMinimap(snap buffer.Snapshot, th *theme.Theme, deadline time.Time) {
	vs := w.vs
	r := vs.minimapRect
	if r.Empty() {
		w.dropMiniStrips()
		vs.mini.cache = nil
		return
	}
	m := &vs.mini
	cols, tab := w.miniColumns(), max(1, w.tabs())
	var lexer *monarch.Lexer
	if hl := w.st.Highlighter(); hl != nil {
		lexer = hl.LexerIdentity()
	}
	if m.cache == nil || m.theme != th || m.language != w.st.Model.Language() || m.lexer != lexer || m.buf != w.st.Model.Buffer() || m.column != cols || m.tab != tab {
		m.lexer = lexer
		m.cache = map[int]*miniLine{}
		m.theme = th
		m.language = w.st.Model.Language()
		m.buf = w.st.Model.Buffer()
		m.column = cols
		m.tab = tab
	}
	if w.opts.Minimap.RenderCharacters {
		w.miniGlyphsFor(th)
	} else {
		w.dropMiniStrips()
	}
	unit, offset, extent := w.miniCanvas()
	n := w.st.vl.Count()
	// One source sample per logical pixel when rows are subpixel; its vrow
	// still describes the real view-row position, not a model-line fraction.
	step := max(unit, 1)
	count := max(1, int(math.Ceil(float64(extent/step))))
	if unit >= 1 && float32(n)*unit > r.H {
		count++
	}
	vs.miniRows = vs.miniRows[:0]
	if m.keep == nil {
		m.keep = make(map[int]bool, count)
	} else {
		clear(m.keep)
	}
	keep := m.keep
	m.indexed, m.misses = false, 0 // byText is built on the frame's second miss (takeMoved)
	// Don't retain token strings from an old grammar when the view is idle.
	// A later query can shorten the slice; clear the whole backing array
	// so earlier queries cannot keep an old grammar's token strings alive.
	defer func() { clear(m.tokenScratch[:cap(m.tokenScratch)]) }()
	left := false // rows left for a later frame (the deadline passed)
	built := m.rebuilt
	hl := w.st.Highlighter()
	ver := snap.Version()
	carryVer, touched := miniSourceCarry(w.st.Model.Buffer(), snap, m.touched[:0])
	m.touched = touched
	if carryVer == 0 {
		// Lines added or removed: the rows of the lines the change did not
		// touch move to their new lines, still current.
		carryVer = m.shiftRows(w.st.Model.Buffer(), snap)
	}
	for k := 0; k < count; k++ {
		pos := float32(k) * step
		// Rows of a pixel or more step one view row per sample, exactly:
		// float32 offset+pos/unit skipped rows at units other than 2. A
		// sample past the last row is not a row.
		var vr int
		if unit >= 1 {
			vr = int(math.Floor(float64(offset))) + k
			if vr > n-1 {
				break
			}
		} else {
			vr = min(n-1, int(math.Floor(float64(offset)+float64(pos)/float64(unit))))
		}
		line, part := w.st.vl.find(vr)
		key := line
		if part > 0 {
			key = -vr - 1
		}
		y := r.Y + (float32(vr)-offset)*unit
		h := unit
		if unit < 1 {
			y = r.Y + pos
			h = min(1, extent-pos)
		}
		if part < 0 || part >= w.st.vl.rowsOf(line) {
			// A view zone's row (zones.go): blank, as Monaco's minimap
			// leaves its view zones.
			vs.miniRows = append(vs.miniRows, miniRow{line: line, vrow: vr, key: miniZoneKey, y: y, h: h})
			continue
		}
		from := 0
		if lb := w.st.vl.breaks[line]; lb != nil {
			from, _ = lb.rowRange(part, lineEnd(snap, line)-snap.LineStart(line))
		}
		vs.miniRows = append(vs.miniRows, miniRow{line: line, vrow: vr, key: key, y: y, h: h})
		keep[key] = true
		ln := m.cache[key]
		// The source decides the miniature: a row whose source is the
		// cached one's (a row of a long line the key did not change, or
		// moved without changing) is not walked again.
		var prefix, depText string
		var rawLen int
		var short bool
		if ln != nil && key == line && ln.seenLine == line && (ln.seen == ver || carryVer != 0 && ln.seen == carryVer && !slices.Contains(touched, line)) {
			// The line's text is the one ln was made from: no edit since
			// touched it (a whole line's row: its source does not depend
			// on the wrapping).
			prefix, depText, rawLen, short = ln.text, ln.dep, ln.rawLen, ln.short
		} else {
			m.sourceReads++
			var raw string
			raw, short = miniatureSource(snap, line, from, cols)
			raw = trimMiniSource(raw, short)
			rawLen = len(raw)
			if ln != nil && ln.short == short && ln.rawLen == len(raw) && strings.HasPrefix(raw, ln.dep) {
				prefix, depText = ln.text, ln.dep
			} else {
				var dep int
				prefix, dep = miniatureDep(raw, short, cols, tab)
				depText = strings.Clone(raw[:dep])
			}
		}
		var start monarch.State
		final := true
		var toks []monarch.Token
		cached := false
		if hl != nil {
			start, _ = hl.StartState(snap, line)
			final = hl.FinalBefore(snap, line+1)
			toks, cached = hl.PeekTokenRange(snap, line, from, from+len(prefix), m.tokenScratch[:0])
			m.tokenScratch = toks
		}
		dirty := ln == nil || ln.text != prefix || ln.start != start || ln.final != final || (cached && !slices.Equal(ln.tokens, toks))
		if dirty {
			// A line inserted or deleted above moves rows to other line
			// numbers unchanged: take over the previous frame's row with
			// the same miniature, start state and tokens, if there is one
			// (without cached tokens: a row tokenized on its own from the
			// same start, whose tokens are this row's).
			if moved := m.takeMoved(prefix, start, final, from, toks, cached); moved != nil {
				// A copy: the previous frame's row may still be its old
				// line's (rows with the same text), each keeps its own seen.
				c := *moved
				ln, dirty = &c, false
				m.cache[key] = ln
			}
		}
		if !dirty {
			if ln.text == prefix && ln.dep == depText && ln.rawLen == rawLen && ln.short == short {
				ln.seen, ln.seenLine = ver, line
			}
			w.ensureMiniBitmap(ln, th, tab)
			continue
		}
		base := 0
		if cached {
			base = from
		}
		if !cached && hl != nil {
			if frameclock.Now().After(deadline) {
				left = true
				continue
			}
			toks = hl.TokenizeSpan(snap, line, prefix, m.tokenScratch[:0])
			m.tokenScratch = toks
		}
		ln = &miniLine{spanned: !cached, text: prefix, dep: depText, rawLen: rawLen, short: short, seen: ver, seenLine: line, start: start, final: final, tokens: slices.Clone(toks), blocks: miniBlocksAt(prefix, base, toks, th, tab), base: base}
		m.cache[key] = ln
		m.rebuilt++
		w.ensureMiniBitmap(ln, th, tab)
	}
	for i := range m.cache {
		if !keep[i] {
			delete(m.cache, i)
		}
	}
	// Do not keep the previous frame's rows reachable through the index.
	clear(m.byText)
	// Rows left for later ask for the next frame (ui.Invalidate, not a
	// timer: the caret's blink timer stays; what the rows draw is declared
	// as damage through navPaint, damage.go). The lines in view may all be
	// final, so the Highlighter's ready callback need not come. A frame
	// that built no row counts towards maxMiniFrames, which bounds the
	// frames asked for while the deadline is spent before the minimap
	// every frame.
	switch {
	case !left:
		m.idle = 0
	case m.rebuilt != built:
		m.idle = 0
		ui.Invalidate(w.c.Window())
	case m.idle < maxMiniFrames:
		m.idle++
		ui.Invalidate(w.c.Window())
	}
	v := w.st.Viewport
	rawScroll := float32(v.scrollY()) / max(1, v.LineHeight)
	scroll := max(0, rawScroll)
	sy := max(0, (scroll-offset)*unit)
	bottom := min(float32(n), rawScroll+v.Height/max(1, v.LineHeight))
	height := max(0, min(extent-sy, (bottom-scroll)*unit))
	vs.minimapSlider = geom.Rect{X: r.X, Y: r.Y + sy, W: r.W, H: height}
}

// miniSourceCarry reports the version before b's last change and the
// lines that change touched (appended to dst), when snap is b's current
// snapshot and every edit of the change stayed inside one line, adding
// and removing no line break: a row made at that version from a line not
// touched has the same source now. It returns 0 otherwise.
func miniSourceCarry(b *buffer.Buffer, snap buffer.Snapshot, dst []int) (uint64, []int) {
	if b.Snapshot().Version() != snap.Version() {
		return 0, dst
	}
	old, edits := b.LastChange()
	if len(edits) == 0 || old.Version() == snap.Version() {
		return 0, dst
	}
	for _, e := range edits {
		l := old.LineOf(e.Start)
		if old.LineOf(e.End) != l || lineBreakCount(e.Text) > 0 {
			return 0, dst
		}
		dst = append(dst, l)
	}
	return old.Version(), dst
}

// shiftRows moves the cached rows of the lines b's last change did not
// touch to those lines' numbers in snap (b's current snapshot), when the
// change added or removed line breaks and the rows were found current at
// the version before it; their text is the same, so they stay current
// there (prepareMinimap does not read their source). It returns that
// version, or 0 when it moved nothing. Rows it does not move keep their
// keys: they are checked against their lines' source as before.
func (m *minimapState) shiftRows(b *buffer.Buffer, snap buffer.Snapshot) uint64 {
	if len(m.cache) == 0 || b.Snapshot().Version() != snap.Version() {
		return 0
	}
	old, edits := b.LastChange()
	if len(edits) == 0 || old.Version() == snap.Version() {
		return 0
	}
	type span struct{ sl, el, delta int } // old lines sl..el replaced; lines after move by delta
	spans := make([]span, 0, len(edits))
	moved := false
	for _, e := range edits {
		sl, el := old.LineOf(e.Start), old.LineOf(e.End)
		d := lineBreakCount(e.Text) - (el - sl)
		moved = moved || d != 0
		spans = append(spans, span{sl, el, d})
	}
	if !moved {
		return 0
	}
	newLine := func(o int) int { // -1: touched
		d := 0
		for _, sp := range spans {
			if o < sp.sl {
				break
			}
			if o <= sp.el {
				return -1
			}
			d += sp.delta
		}
		return o + d
	}
	ov := old.Version()
	next := make(map[int]*miniLine, len(m.cache))
	var carried []*miniLine
	for k, ln := range m.cache {
		if k >= 0 && ln.seen == ov && ln.seenLine == k {
			if nk := newLine(k); nk >= 0 {
				ln.seenLine = nk
				carried = append(carried, ln)
				continue
			}
		}
		ln.seen = 0 // not known current: its line may have changed or moved
		next[k] = ln
	}
	if len(carried) == 0 {
		return 0
	}
	for _, ln := range carried {
		next[ln.seenLine] = ln // over a stale row left at that key
	}
	m.cache = next
	return ov
}

// maxMiniFrames bounds the frames in a row the minimap asks for without
// building a row (prepareMinimap).
const maxMiniFrames = 60

// miniZoneKey is the cache key of a view zone's minimap rows: no line
// is cached under it, so they draw nothing.
const miniZoneKey = math.MinInt

type miniRow struct {
	line, vrow, key int
	y, h            float32
}

func miniBlocks(s string, toks []monarch.Token, th *theme.Theme, tab int) []miniBlock {
	return miniBlocksAt(s, 0, toks, th, tab)
}

// indexMoved indexes the cached rows by miniature text (see byText). It
// runs once per frame, on the second row the key does not find; rows are
// taken out of the index as they are reused, so one cached row serves one
// new row at most. Rows the frame already rebuilt under a key are in the
// cache too: they match only rows with their exact contents, which a
// rebuild would give the same miniature.
func (m *minimapState) indexMoved() {
	if m.byText == nil {
		m.byText = make(map[string][]*miniLine, len(m.cache))
	}
	clear(m.byText)
	for _, ln := range m.cache {
		m.byText[ln.text] = append(m.byText[ln.text], ln)
	}
	m.indexed = true
}

// takeMoved returns and removes from the index a previous frame's row
// whose miniature, start state, finality, token base and tokens are the
// ones a rebuild would give the row being built (prefix, start, final,
// from and toks): everything the row and its bitmap are made from, as the
// dirty check compares for a row found by key. Nil when there is none.
//
// Typing changes one row: its key misses once and the row is rebuilt as
// before, with no index. Moved rows miss together (every row below an
// inserted or deleted line), so the index is built on the frame's second
// miss; the first one is rebuilt without it.
func (m *minimapState) takeMoved(prefix string, start monarch.State, final bool, from int, toks []monarch.Token, cached bool) *miniLine {
	m.misses++
	if m.misses < 2 {
		return nil
	}
	if !m.indexed {
		m.indexMoved()
	}
	rows := m.byText[prefix]
	for i, ln := range rows {
		same := ln.start == start && ln.final == final
		if cached {
			same = same && ln.base == from && slices.Equal(ln.tokens, toks)
		} else {
			same = same && ln.spanned && ln.base == 0
		}
		if same {
			m.byText[prefix] = slices.Delete(rows, i, i+1)
			return ln
		}
	}
	return nil
}
func miniBlocksAt(s string, base int, toks []monarch.Token, th *theme.Theme, tab int) []miniBlock {
	var out []miniBlock
	col, k := 0, 0
	fg := th.Style("").Foreground
	for i := 0; i < len(s); {
		j := nextGrapheme(s, i)
		for k < len(toks) && toks[k].ByteStart <= base+i {
			fg = th.Style(toks[k].Type).Foreground
			k++
		}
		next := advanceCol(col, s[i:j], tab)
		if s[i:j] != " " && s[i:j] != "\t" {
			if len(out) > 0 && out[len(out)-1].b == col && out[len(out)-1].color == fg {
				out[len(out)-1].b = next
			} else {
				out = append(out, miniBlock{col, next, fg})
			}
		}
		col = next
		i = j
	}
	return out
}

func (w *ViewW) paintMinimap(l *paint.List, th *theme.Theme) {
	vs := w.vs
	r := vs.minimapRect
	if r.Empty() {
		return
	}
	l.Clip(r, geom.Radii{})
	bg := defaultColor(th, "minimap.background")
	if bg.A == 0 {
		bg = defaultColor(th, "editor.background")
	}
	w.navigationFill(l, r, bg)
	if w.opts.Minimap.RenderCharacters {
		w.paintMinimapChars(l, r)
	} else {
		scale := min(1, r.W/float32(w.miniColumns()))
		for _, row := range vs.miniRows {
			if ln := vs.mini.cache[row.key]; ln != nil {
				for _, b := range ln.blocks {
					w.navigationFill(l, geom.Rect{X: r.X + float32(b.a)*scale, Y: row.y, W: float32(b.b-b.a) * scale, H: min(1, row.h)}, b.color)
				}
			}
		}
	}
	pointer, pointerIn := w.c.Pointer()
	hover := pointerIn && r.Contains(pointer)
	mode := w.opts.Minimap.ShowSlider
	if mode == MinimapSliderAlways || hover || vs.railDrag == 1 {
		c := defaultColor(th, "minimapSlider.background")
		if vs.railDrag == 1 {
			c = defaultColor(th, "minimapSlider.activeBackground")
		} else if hover {
			c = defaultColor(th, "minimapSlider.hoverBackground")
		}
		w.navigationFill(l, vs.minimapSlider, c)
	}
	l.Pop()
}

// Clicks use the exact source sample painted at the pointer. Dragging
// inverts that same canvas/window transform, not a whole-rail model ratio.
func (w *ViewW) navigateMinimapClick(p geom.Point) {
	vs := w.vs
	if len(vs.miniRows) == 0 {
		return
	}
	target := vs.miniRows[len(vs.miniRows)-1].vrow
	for _, row := range vs.miniRows {
		if p.Y < row.y+row.h {
			target = row.vrow
			break
		}
	}
	_, _, extent := w.miniCanvas()
	if p.Y >= vs.minimapRect.Y+extent {
		return // no painted source row in the blank tail of a short preview
	}
	w.st.Viewport.setScrollY(float64(target)*float64(w.st.Viewport.LineHeight), w.st.vl.Count())
}
func (w *ViewW) navigateMinimap(p geom.Point) {
	vs := w.vs
	v := &w.st.Viewport
	n := w.st.vl.Count()
	unit, _, extent := w.miniCanvas()
	if p.Y >= vs.minimapRect.Y+extent {
		v.setScrollY(float64(n-1)*float64(v.LineHeight), n)
		return
	}
	grab := max(0, min(.99999, vs.railGrab/max(.000001, vs.railThumbHeight)))
	thumbPoint := func(row float32) float32 {
		top := (row - w.miniWindow(row, unit)) * unit
		height := max(0, min(extent-top, min(float32(n)-row, v.Height/max(1, v.LineHeight))*unit))
		return top + height*grab
	}
	target := p.Y - vs.minimapRect.Y
	if target <= thumbPoint(0) {
		v.setScrollY(-float64(v.PadTop), n)
		return
	}
	if target >= thumbPoint(float32(n-1)) {
		v.setScrollY(float64(n-1)*float64(v.LineHeight), n)
		return
	}
	lo, hi := float32(0), float32(max(0, n-1))
	// Invert the point held inside the thumb, including its shrinking tail.
	for range 32 {
		mid := (lo + hi) / 2
		if thumbPoint(mid) < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	row := (lo + hi) / 2
	nearest := float32(math.Round(float64(row)))
	if absf(row-nearest) < 0.0001 {
		row = nearest
	}
	v.setScrollY(float64(row)*float64(v.LineHeight), n)
}
