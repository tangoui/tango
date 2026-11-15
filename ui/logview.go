package ui

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// LogView: a virtualized log viewer.

// LogViewLocale holds the LogView toolbar texts.
type LogViewLocale struct {
	_ [0]func()

	Name                 string // accessible name of the log area
	Search               string // search box placeholder
	PrevMatch, NextMatch string
	MatchFmt             string // current match and count, args: i, n ("%d/%d")
	NoMatch              string
	Wrap                 string
	Follow               string // keep showing the newest lines
	Level                string // level filter
	AllLevels            string
	Levels               [5]string // Trace, Debug, Info, Warn, Error
	LinesFmt             string    // line count, arg: n ("%d 行")
}

// Match formats the match position ("3/120").
func (l *LogViewLocale) Match(i, n int) string { return locFmt(l.MatchFmt, i, n) }

// Lines formats a line count.
func (l *LogViewLocale) Lines(n int) string { return locFmt(l.LinesFmt, n) }

// LogViewW shows a LogBuffer: millions of lines scroll smoothly (only the
// visible rows are shaped), ANSI colors and attributes are drawn, new lines
// keep the view at the bottom until the user scrolls up (scrolling back to
// the bottom follows again), and the toolbar searches (Enter / Shift+Enter
// or the arrows move between matches), filters by minimum level, toggles
// soft wrapping and following, and clears the buffer. Text is selectable
// and copies with Cmd/Ctrl+C.
type LogViewW struct {
	_ [0]func()

	c         *Context
	buf       *LogBuffer
	b         *Box
	height    float32
	numbers   bool
	toolbar   bool
	levelCols bool
	wrapP     *bool
	followP   *bool
	queryP    *string
	levelP    *LogLevel
	label     string
	v         *txView
	st        *logViewState
	z         *lazy
}

type logMatch struct {
	abs        int64
	start, end int32 // bytes
}

type logViewState struct {
	init   bool
	base   int64 // snapshot base of the last frame
	bufVer uint64
	layout uint64 // txView ver: bumps when lines change other than by appending

	// Level filter: absolute indexes of the lines that pass.
	minLevel LogLevel
	vis      []int64
	visTo    int64 // lines before this absolute index were considered

	// Search.
	query      string
	matches    []logMatch
	searchedTo int64 // lines before this absolute index were searched
	cur        int   // current match (-1: none)
	jumped     bool  // the first match was revealed

	wrap, follow bool // values when the app binds none
	levelSel     string
}

// logMaxMatches bounds the matches kept for one query.
const logMaxMatches = 1_000_000

// LogView adds a log viewer over buf.
//
//	logs := ui.NewLogBuffer(1_000_000) // keep the newest million lines
//	go cmd.Stdout.CopyTo(logs)        // Write is goroutine-safe
//	ui.LogView(c, logs).Height(400)
func LogView(c *Context, buf *LogBuffer, opts ...layout.Option) *LogViewW {
	w := &LogViewW{c: c, buf: buf, toolbar: true, levelCols: true}
	w.b = c.box("logview", layout.Column, opts)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

// Height fixes the viewer's height (toolbar included). Without it the
// viewer grows in its parent (give the parent a height).
func (w *LogViewW) Height(px float32) *LogViewW { w.height = px; return w }

// LineNumbers shows line numbers (counting dropped lines, so a line keeps
// its number).
func (w *LogViewW) LineNumbers(on bool) *LogViewW { w.numbers = on; return w }

// NoToolbar hides the toolbar (bind Query / MinLevel / Wrap / Follow to
// drive the view from elsewhere).
func (w *LogViewW) NoToolbar() *LogViewW { w.toolbar = false; return w }

// NoLevelColors draws warn / error / debug lines in the plain text color.
func (w *LogViewW) NoLevelColors() *LogViewW { w.levelCols = false; return w }

// Wrap binds soft wrapping to *on.
func (w *LogViewW) Wrap(on *bool) *LogViewW { w.wrapP = on; return w }

// Follow binds following the newest lines to *on.
func (w *LogViewW) Follow(on *bool) *LogViewW { w.followP = on; return w }

// Query binds the search text to *q (case-insensitive).
func (w *LogViewW) Query(q *string) *LogViewW { w.queryP = q; return w }

// MinLevel binds the level filter to *lv: lines below it are hidden
// (LogNone shows everything; lines without a level always show).
func (w *LogViewW) MinLevel(lv *LogLevel) *LogViewW { w.levelP = lv; return w }

// A11yLabel sets the accessible name of the log area.
func (w *LogViewW) A11yLabel(s string) *LogViewW { w.label = s; return w }

// Layout applies layout options to the viewer.
func (w *LogViewW) Layout(opts ...layout.Option) *LogViewW {
	flexbox.Apply(&w.b.n.ln.Style, opts...)
	return w
}

// Selection returns the selected text ("" if none).
func (w *LogViewW) Selection() string {
	w.z.run()
	if w.v == nil {
		return ""
	}
	return w.v.selection()
}

// Matches returns the number of search matches found so far.
func (w *LogViewW) Matches() int { w.z.run(); return len(w.st.matches) }

// ID returns the viewer's ID.
func (w *LogViewW) ID() ID { return w.b.n.id }

func (w *LogViewW) build() {
	c := w.c
	th := c.ThemeFor(theme.CompLogView)
	tk := &th.Components.LogView
	loc := &c.Locale().LogView
	st := State(c, w.b.n.id, func() logViewState { return logViewState{cur: -1, follow: true} })
	w.st = st
	wrap, follow := &st.wrap, &st.follow
	if w.wrapP != nil {
		wrap = w.wrapP
	}
	if w.followP != nil {
		follow = w.followP
	}
	qv := st.query
	if w.queryP != nil {
		qv = *w.queryP
	}
	lv := st.minLevel
	if w.levelP != nil {
		lv = *w.levelP
	}
	v := newTxView(c, hashID(w.b.n.id, "view"))
	w.v = v
	snap := w.buf.snapshot()
	w.sync(snap, lv, qv)

	if w.height > 0 {
		w.b.n.ln.Height = w.height
	} else if w.b.n.ln.Grow == 0 {
		w.b.n.ln.Grow = 1
	}
	w.b.n.ln.MinH = 120
	w.b.Bg(tk.Bg).Radius(tk.Radius).Clip()
	if tk.BorderColor.A > 0 {
		w.b.Border(1, tk.BorderColor)
	}
	if w.toolbar {
		w.toolbarRow(snap, tk, loc, wrap, follow)
	}
	// Re-read: the toolbar may have changed the query or level.
	if w.queryP != nil {
		qv = *w.queryP
	} else {
		qv = st.query
	}
	if w.levelP != nil {
		lv = *w.levelP
	} else {
		lv = st.minLevel
	}
	if qv != st.query || lv != st.minLevel {
		w.sync(snap, lv, qv)
	}
	w.search(snap)

	count := len(snap.lines)
	if lv != LogNone {
		count = len(st.vis)
	}
	abs := func(i int) int64 {
		if lv != LogNone {
			return st.vis[i]
		}
		return snap.base + int64(i)
	}
	lineAt := func(i int) *logLine { return &snap.lines[abs(i)-snap.base] }
	v.count = count
	v.line = func(i int) string { return lineAt(i).text }
	v.cells = func(i int) int { return int(lineAt(i).cells) }
	v.ver = st.layout
	v.wrap = *wrap
	v.size, v.lineH, v.padX, v.padY = tk.LineFontSize, tk.LineHeight, tk.PaddingInline, 4
	v.fg, v.gutter, v.selBg, v.match, v.matchC, v.matchB = tk.Text, tk.GutterColor, tk.SelectionBg, tk.MatchBg, tk.MatchCurrentBg, tk.MatchCurrentBorder
	v.name = w.label
	if v.name == "" {
		v.name = loc.Name
	}
	if w.numbers {
		v.numbers = func(i int) int { return int(abs(i)) + 1 }
		v.maxNo = int(snap.base) + len(snap.lines)
		v.padX = min(v.padX, 8)
	}
	levelCols := w.levelCols
	v.runs = func(i int, dst []txvRun) []txvRun {
		ln := lineAt(i)
		var def theme.Color
		if levelCols && ln.own {
			switch ln.level {
			case LogError:
				def = tk.ErrorColor
			case LogWarn:
				def = tk.WarnColor
			case LogDebug, LogTrace:
				def = tk.DebugColor
			}
		}
		pos := 0
		for _, sp := range ln.spans {
			if sp.Start > pos && def.A > 0 {
				dst = append(dst, txvRun{start: pos, end: sp.Start, fg: def})
			}
			r := txvRun{start: sp.Start, end: sp.End, bold: sp.Style.Bold, italic: sp.Style.Italic,
				underline: sp.Style.Underline, strike: sp.Style.Strike, faint: sp.Style.Faint}
			r.fg, r.bg = ansiColor(tk, sp.Style.Fg), ansiColor(tk, sp.Style.Bg)
			if r.fg.A == 0 {
				r.fg = def
			}
			if sp.Style.Inverse {
				fg, bg := r.fg, r.bg
				if fg.A == 0 {
					fg = tk.Text
				}
				if bg.A == 0 {
					bg = tk.Bg
				}
				r.fg, r.bg = bg, fg
			}
			dst = append(dst, r)
			pos = sp.End
		}
		if def.A > 0 && pos < len(ln.text) {
			dst = append(dst, txvRun{start: pos, end: len(ln.text), fg: def})
		}
		return dst
	}
	if len(st.matches) > 0 {
		v.marks = func(i int, dst []txvMark) []txvMark {
			a := abs(i)
			k := sort.Search(len(st.matches), func(k int) bool { return st.matches[k].abs >= a })
			for ; k < len(st.matches) && st.matches[k].abs == a; k++ {
				m := st.matches[k]
				dst = append(dst, txvMark{start: int(m.start), end: int(m.end), current: k == st.cur})
			}
			return dst
		}
	}
	// Lines dropped from the front move a view that does not follow up by
	// as many rows (txView keeps the visible lines in place); a following
	// view just stays at the bottom.
	shifted := float64(v.st.shift) * float64(v.lineH)
	if *follow {
		v.st.shift, shifted = 0, 0
	}
	// The log reads left to right in RTL layouts too.
	ConfigProvider(c).Direction(LTR).Body(func() {
		v.show("logview-view", []layout.Option{layout.Grow(1), layout.MinH(0)})
	})
	// Following: stop when the user scrolls up, resume at the bottom.
	s := v.sb.s
	if s.view.H > 0 {
		switch {
		case *follow && s.off.Y < s.painted.Y-0.5:
			*follow = false
		case !*follow && s.off.Y > s.painted.Y-shifted+0.5 && s.off.Y >= s.maxY()-float64(v.lineH)/2:
			*follow = true
		}
	}
	if *follow {
		s.off.Y = math.MaxFloat32
	}
	// Reveal the current match when it was just found or moved to.
	if st.cur >= 0 && st.cur < len(st.matches) && !st.jumped {
		st.jumped = true
		m := st.matches[st.cur]
		if i := w.viewIndex(snap, m.abs, lv); i >= 0 {
			*follow = false
			v.revealLine(i)
			v.revealCol(i, int(m.start))
		}
	}
}

// viewIndex returns the row of absolute line a (-1 if hidden).
func (w *LogViewW) viewIndex(snap logSnap, a int64, lv LogLevel) int {
	if lv == LogNone {
		if a < snap.base || a >= snap.base+int64(len(snap.lines)) {
			return -1
		}
		return int(a - snap.base)
	}
	vis := w.st.vis
	k := sort.Search(len(vis), func(k int) bool { return vis[k] >= a })
	if k < len(vis) && vis[k] == a {
		return k
	}
	return -1
}

// sync follows the buffer (cleared, dropped lines, appended lines) and
// applies a new level filter or query.
func (w *LogViewW) sync(snap logSnap, lv LogLevel, q string) {
	st, v := w.st, w.v
	if !st.init || snap.ver != st.bufVer {
		st.init, st.bufVer, st.base = true, snap.ver, snap.base
		st.vis, st.visTo = st.vis[:0], snap.base
		st.matches, st.searchedTo, st.cur, st.jumped = st.matches[:0], snap.base, -1, false
		st.layout++
		v.st.anc, v.st.cur = txvPos{}, txvPos{}
	}
	if d := snap.base - st.base; d > 0 {
		st.base = snap.base
		if st.minLevel != LogNone {
			k := sort.Search(len(st.vis), func(k int) bool { return st.vis[k] >= snap.base })
			st.vis = append(st.vis[:0], st.vis[k:]...)
			v.st.dropFront(k)
		} else {
			v.st.dropFront(int(min(d, math.MaxInt32)))
		}
		k := sort.Search(len(st.matches), func(k int) bool { return st.matches[k].abs >= snap.base })
		if k > 0 {
			st.matches = append(st.matches[:0], st.matches[k:]...)
			st.cur = max(-1, st.cur-k)
		}
		st.visTo = max(st.visTo, snap.base)
		st.searchedTo = max(st.searchedTo, snap.base)
	}
	if lv != st.minLevel {
		st.minLevel = lv
		st.vis, st.visTo = st.vis[:0], snap.base
		st.layout++
		v.st.anc, v.st.cur = txvPos{}, txvPos{}
		st.matches, st.searchedTo, st.cur, st.jumped = st.matches[:0], snap.base, -1, false
	}
	if lv != LogNone {
		end := snap.base + int64(len(snap.lines))
		for a := st.visTo; a < end; a++ {
			if ln := &snap.lines[a-snap.base]; ln.level == LogNone || ln.level >= lv {
				st.vis = append(st.vis, a)
			}
		}
		st.visTo = end
	}
	if q != st.query {
		st.query = q
		st.matches, st.searchedTo, st.cur, st.jumped = st.matches[:0], snap.base, -1, false
	}
}

// search continues the search within a time budget (the rest follows in
// later frames), then picks the first match at or below the top of the
// view.
func (w *LogViewW) search(snap logSnap) {
	c, st := w.c, w.st
	if st.query == "" {
		return
	}
	q := st.query
	end := snap.base + int64(len(snap.lines))
	deadline := time.Now().Add(6 * time.Millisecond)
	n := 0
	for a := st.searchedTo; a < end; a++ {
		ln := &snap.lines[a-snap.base]
		if st.minLevel != LogNone && !(ln.level == LogNone || ln.level >= st.minLevel) {
			st.searchedTo = a + 1
			continue
		}
		for off := 0; off < len(ln.text) && len(st.matches) < logMaxMatches; {
			i := logIndexFold(ln.text[off:], q)
			if i < 0 {
				break
			}
			s := off + i
			e := s + logFoldLen(ln.text[s:], q)
			st.matches = append(st.matches, logMatch{abs: a, start: int32(s), end: int32(e)})
			off = max(e, s+1)
		}
		st.searchedTo = a + 1
		if n++; n&1023 == 0 && time.Now().After(deadline) {
			c.WakeAt(c.now.Add(time.Millisecond))
			break
		}
	}
	if st.cur < 0 && len(st.matches) > 0 {
		st.cur = 0
		st.jumped = false
	}
}

// step moves the current match by d (wrapping around).
func (w *LogViewW) step(d int) {
	st := w.st
	if len(st.matches) == 0 {
		return
	}
	st.cur = ((max(st.cur, 0)+d)%len(st.matches) + len(st.matches)) % len(st.matches)
	st.jumped = false
	w.c.invalid = true
}

func (w *LogViewW) toolbarRow(snap logSnap, tk *theme.LogViewToken, loc *LogViewLocale, wrap, follow *bool) {
	c, st := w.c, w.st
	Row(c, func() {
		q := &st.query
		if w.queryP != nil {
			q = w.queryP
		}
		in := Input(c, q).Placeholder(loc.Search).Prefix(icon.Search).AllowClear().Size(Small).Width(220).A11yLabel(loc.Search)
		in.Changed()
		if in.Submitted() {
			if logShiftEnter(c) {
				w.step(-1)
			} else {
				w.step(1)
			}
		}
		if *q != "" {
			label := loc.NoMatch
			if n := len(st.matches); n > 0 {
				label = loc.Match(st.cur+1, n)
			}
			Text(c, label).Size(c.Theme.FontSizeSM).Color(c.Theme.TextSecondary)
			if IconButton(c, icon.Up, loc.PrevMatch).Size(24, 14).Clicked() {
				w.step(-1)
			}
			if IconButton(c, icon.Down, loc.NextMatch).Size(24, 14).Clicked() {
				w.step(1)
			}
		}
		lv := &st.minLevel
		if w.levelP != nil {
			lv = w.levelP
		}
		opts := []Option{{Label: loc.AllLevels, Value: "0"}}
		for i, name := range loc.Levels {
			opts = append(opts, Option{Label: name, Value: itoaEnt(i + 1)})
		}
		st.levelSel = itoaEnt(int(*lv))
		if Select(c, &st.levelSel, opts).Size(Small).Width(120).A11yLabel(loc.Level).Changed() {
			n := 0
			for _, ch := range st.levelSel {
				n = n*10 + int(ch-'0')
			}
			*lv = LogLevel(n)
			c.invalid = true
		}
		Spacer(c)
		Text(c, loc.Lines(len(snap.lines))).Size(c.Theme.FontSizeSM).Color(c.Theme.TextSecondary)
		Tooltip(c, loc.Wrap, func() {
			Toggle(c, "", wrap).IconOnly(icon.WrapText).Size(Small).A11yLabel(loc.Wrap)
		})
		Tooltip(c, loc.Follow, func() {
			if Toggle(c, "", follow).IconOnly(icon.VerticalAlignBot).Size(Small).A11yLabel(loc.Follow).Changed() && *follow {
				c.invalid = true
			}
		})
		Tooltip(c, c.Locale().Clear, func() {
			if IconButton(c, icon.Delete, c.Locale().Clear).Size(24, 14).Clicked() {
				w.buf.Clear()
				c.invalid = true
			}
		})
	}, layout.Middle(), layout.Gap(tk.ToolbarGap), layout.PadVH(6, 8)).PaintOver(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.BorderColor)
	})
}

// logShiftEnter reports Shift+Enter this frame (search backwards).
func logShiftEnter(c *Context) bool {
	for _, k := range c.keys {
		if k.Down && k.Key == input.KeyEnter && k.Mods&input.ModShift != 0 {
			return true
		}
	}
	return false
}

// logIndexFold returns the byte index of the first case-insensitive match
// of q in s (-1 if none). ASCII queries are matched byte by byte.
func logIndexFold(s, q string) int {
	if q == "" {
		return -1
	}
	for i := 0; i < len(q); i++ {
		if q[i] >= utf8.RuneSelf {
			for i := range s {
				if _, ok := logFoldPrefix(s[i:], q); ok {
					return i
				}
			}
			return -1
		}
	}
	f0 := lowerASCII(q[0])
	for i := 0; i+len(q) <= len(s); i++ {
		if lowerASCII(s[i]) != f0 {
			continue
		}
		j := 1
		for j < len(q) && lowerASCII(s[i+j]) == lowerASCII(q[j]) {
			j++
		}
		if j == len(q) {
			return i
		}
	}
	return -1
}

// logFoldPrefix reports whether s starts with q under Unicode case
// folding, and how many bytes of s the match takes.
func logFoldPrefix(s, q string) (int, bool) {
	b := 0
	for _, qr := range q {
		if b >= len(s) {
			return 0, false
		}
		sr, sz := utf8.DecodeRuneInString(s[b:])
		if sr != qr && !strings.EqualFold(string(sr), string(qr)) {
			return 0, false
		}
		b += sz
	}
	return b, true
}

// logFoldLen is the byte length of the match of q at the start of s.
func logFoldLen(s, q string) int {
	if n, ok := logFoldPrefix(s, q); ok {
		return n
	}
	return min(len(q), len(s))
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}
