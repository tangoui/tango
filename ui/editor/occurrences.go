package editor

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Occurrences and selection matches (Monaco's WordHighlighter with the
// textual document highlight provider, and its SelectionHighlighter):
//
// Occurrences (Options.OccurrencesHighlight): while the view has focus and
// the primary selection lies on one line inside a word (the language's
// word definition; the word touching the selection's start must cover the
// selection), the whole-word (Monaco's usual word separators),
// case-sensitive matches of that word are drawn with
// editor.wordHighlightTextBackground and a one pixel
// editor.wordHighlightTextBorder (Monaco's .wordHighlightText).
// A cursor move (or focusing the view) runs the highlighter
// occDebounce after the caret came to rest (Monaco's 50 ms Delayer: every
// move restarts it, so holding an arrow key computes nothing until it is
// released). Until it runs the occurrences shown stay; when it runs, they
// stay if the caret is on one of them, else they are replaced in that same
// frame by the new word's, drawn OccurrencesHighlightDelay milliseconds
// later (0 by default, Monaco's occurrencesHighlightDelay: at once). Any
// change of the text (typing, another view's edit, undo) removes them at
// once, and the caret moves of typing do not bring them back: only a move
// that is not an edit does (Monaco runs on CursorChangeReason.Explicit
// only). Losing focus removes them. The waits are Context.WakeAtDamaged
// timers: the frame that changes the occurrences declares the rows they
// change, and no frame is requested once they are shown. At most
// maxFindMatches occurrences are drawn (Monaco's LIMIT_FIND_COUNT).
//
// Selection matches (Options.SelectionHighlight): when the primary
// selection is not empty, lies on one line, is not only blanks and has at
// most selectionHighlightMax UTF-16 code units, and every selection holds
// the same text (ignoring case), the other matches of that text are drawn
// at once with editor.selectionHighlightBackground and a one pixel
// editor.selectionHighlightBorder, or without focus with half that
// background and no border (Monaco's .selectionHighlight rules). They match
// as Monaco's find widget defaults do (any case, not only whole words),
// or as the addSelectionToNextFindMatch session in progress does (whole
// words and matching case when it started from a caret); matches that
// overlap a selection are not drawn, but still consume the document's
// maxFindMatches quota.
//
// Both searches cache the first maxFindMatches matches in document order,
// including matches outside the viewport and selected matches. Only then
// are selected ranges excluded and the visible subset drawn. Scrolling
// never changes the quota or rescans the document. The scan is spread over
// frames, the visible lines matched at once (documentMatches). Large files have neither
// (largefile.go). Long lines are searched, but not decorated.

// selectionHighlightMax is Monaco's selectionHighlightMaxLength default.
const selectionHighlightMax = 200

// occDebounce is Monaco's WordHighlighter runDelayer (a 50 ms Delayer).
const occDebounce = 50 * time.Millisecond

// maxFindMatches is Monaco's LIMIT_FIND_COUNT, the most matches a search
// returns across the entire document, before selections are excluded.
const maxFindMatches = 999

// occState is the occurrences highlight of a view.
type occState struct {
	buf   *buffer.Buffer
	ver   uint64
	sels  uint64    // selsKey of the selections it follows
	run   bool      // a run is pending (the debounce)
	runAt time.Time // when it is due
	word  string    // the word highlighted (or about to be), "" for none
	show  time.Time // when word is drawn
	shown bool
	on    bool // focused, option on: the state is live
}

// occurrenceWord returns the word whose occurrences are drawn this frame,
// "" for none; it waits for the debounce and the delay with damaged
// wake-ups.
func (w *ViewW) occurrenceWord(snap buffer.Snapshot, focused bool) string {
	st, vs := w.st, w.vs
	o := &vs.occ
	if !w.opts.occurrences() || st.LargeFile() || !focused {
		*o = occState{}
		return ""
	}
	buf, ver, sk := st.Model.Buffer(), snap.Version(), selsKey(st.Sels)
	now := w.c.Now()
	switch {
	case !o.on || o.buf != buf:
		// Focus came (or another document): run after the debounce.
		*o = occState{buf: buf, ver: ver, sels: sk, on: true, run: true, runAt: now.Add(occDebounce)}
	case o.ver != ver:
		// The text changed: nothing until the caret moves on its own.
		*o = occState{buf: buf, ver: ver, sels: sk, on: true}
	case o.sels != sk:
		o.sels, o.run, o.runAt = sk, true, now.Add(occDebounce)
	}
	if o.run && !now.Before(o.runAt) {
		o.run = false
		word := w.queryWord(snap)
		switch {
		case word == "":
			o.word, o.shown = "", false
		case o.shown && w.onOccurrence(snap, o.word):
			// Still on one of the occurrences shown: they stay.
		default:
			o.word, o.shown, o.show = word, false, now.Add(w.occDelay())
		}
	}
	if o.run {
		w.c.WakeAtDamaged(o.runAt) // the rows declare what changes (damage.go)
	}
	if o.word != "" && !o.shown {
		if now.Before(o.show) {
			w.c.WakeAtDamaged(o.show)
			return ""
		}
		o.shown = true
	}
	if !o.shown {
		return ""
	}
	return o.word
}

func (w *ViewW) occDelay() time.Duration {
	return time.Duration(max(w.opts.OccurrencesHighlightDelay, 0)) * time.Millisecond
}

// queryWord returns the word the primary selection highlights (see
// above), "" for none.
func (w *ViewW) queryWord(snap buffer.Snapshot) string {
	st := w.st
	p := st.Primary()
	a, b := clampOff(snap, p.Anchor), clampOff(snap, p.Head)
	a, b = min(a, b), max(a, b)
	l := snap.LineOf(a)
	if snap.LineOf(b) != l {
		return ""
	}
	text := snap.Line(l)
	if isLong(text) {
		return ""
	}
	ls := snap.LineStart(l)
	wa, wb, ok := w.wordTouching(text, a-ls)
	if !ok || wa > a-ls || wb < b-ls {
		return ""
	}
	return text[wa:wb]
}

// wordTouching returns the word of text (the language's word definition)
// containing byte col or ending at it (Monaco's getWordAtPosition).
func (w *ViewW) wordTouching(text string, col int) (int, int, bool) {
	for _, m := range w.st.words(text) {
		if m[0] <= col && col <= m[1] {
			return m[0], m[1], true
		}
	}
	return 0, 0, false
}

// onOccurrence reports whether the primary caret is on (or at an end of) a
// whole-word match of word.
func (w *ViewW) onOccurrence(snap buffer.Snapshot, word string) bool {
	head := clampOff(snap, w.st.Primary().Head)
	ms, _ := w.vs.occMatches.find(w.st.Model.Buffer(), snap, &selQuery{text: word, whole: true}, time.Time{}, []int{snap.LineOf(head)})
	i := sort.Search(len(ms), func(i int) bool { return ms[i].b >= head })
	return i < len(ms) && ms[i].a <= head
}

// documentMatches caches bounded document searches, before selected
// matches are excluded. Its key includes every search input; selections
// and scrolling are deliberately not inputs (only their visible subset
// changes). Ranges are absolute UTF-8 offsets, sorted in document order.
//
// A search is resumable (docScan): each frame scans on from the line it
// stopped at for at most docScanBudget, and asks for another frame
// (a damaged wake-up: the rows whose decorations change declare it) until
// the scan is done, so a rare word in a large document never blocks a
// frame. The lines on screen are matched directly in the frame: the
// result of an unfinished scan is the scanned lines' matches followed by
// the visible unscanned lines' matches, the latter provisional and capped
// by the quota left. Monaco's synchronous findMatches has no such state;
// our rule keeps its final result: when the scan is done the result is
// exactly the document's first maxFindMatches matches (a one-shot scan's),
// so a visible match past them, drawn provisionally, goes in the frame
// the scan establishes that the quota ran out before it. The overview
// ruler draws no occurrence or selection-match marks (overview.go), so
// nothing else shows the partial result.
//
// The last docCacheSize searches are kept (least recently used first out),
// so going back to a query (A, B, A) neither rescans nor restarts an
// unfinished scan. A new version of a buffer drops that buffer's scans of
// other versions: versions only grow (buffer.Buffer), they cannot be hit
// again, and an unfinished one is abandoned.
type documentMatches struct {
	ents   [docCacheSize]*docScan
	tick   uint64   // LRU clock
	cur    *docScan // the last search's scan
	ranges []brange // the last search's result (tests)
	scans  uint64   // number of cache misses (tests)
	// now reads the clock for the budget (nil: scanClock); chunk is the
	// work between readings (0: docScanChunk). Tests change both.
	now     func() time.Time
	chunk   int
	out     []brange // scratch: an unfinished scan's result
	scratch []brange
}

// scanClock is the searches' default budget clock. The package's tests
// replace it with a stopped clock, so that whether a scan finishes in the
// frame a test expects does not depend on the wall clock, unless a test
// sets a clock of its own.
var scanClock = time.Now

// docScan is one resumable search.
type docScan struct {
	buf         *buffer.Buffer
	ver         uint64
	text        string
	whole, fold bool
	ranges      []brange // matches of lines [0, next)
	next        int      // next line to scan
	done        bool     // all lines scanned, or the quota is full
	used        uint64   // LRU tick of its last use
}

const (
	// docScanBudget is a frame's scanning time (both searches together).
	docScanBudget = 2 * time.Millisecond
	// docScanChunk is the most scanning work between two clock readings:
	// line bytes, each line counting docScanLineCost more, and at most
	// docScanLines lines (a short line costs a few microseconds to fetch
	// and match: 32 KiB of them would take longer than the budget).
	docScanChunk    = 32 << 10
	docScanLineCost = 32
	docScanLines    = 64
	// docCacheSize is the number of searches kept.
	docCacheSize = 4
)

// find returns q's matches: the scan's result once done, else the scanned
// lines' matches followed by the provisional matches of the lines of vis
// (ascending model lines) the scan has not reached. It first scans on
// until deadline (none with a zero deadline). done reports whether the
// scan is complete; the result is valid until the next call.
func (c *documentMatches) find(buf *buffer.Buffer, snap buffer.Snapshot, q *selQuery, deadline time.Time, vis []int) (ms []brange, done bool) {
	if q == nil || q.text == "" {
		c.cur, c.ranges = nil, nil
		return nil, true
	}
	e := c.entry(buf, snap, q)
	if !deadline.IsZero() {
		c.scan(e, snap, q, deadline)
	}
	c.ranges = e.ranges
	if e.done {
		return e.ranges, true
	}
	i := sort.SearchInts(vis, e.next)
	if i == len(vis) {
		return e.ranges, false
	}
	out := append(c.out[:0], e.ranges...)
	last := -1
	for _, line := range vis[i:] {
		left := maxFindMatches - len(out)
		if left <= 0 || line >= snap.LineCount() {
			break
		}
		if line <= last {
			continue
		}
		last = line
		c.scratch = q.matchesLimit(snap.Line(line), c.scratch[:0], left)
		ls := snap.LineStart(line)
		for _, m := range c.scratch {
			out = append(out, brange{ls + m.a, ls + m.b})
		}
	}
	c.out, c.ranges = out, out
	return out, false
}

// step scans q on until deadline and reports whether its scan is done.
func (c *documentMatches) step(buf *buffer.Buffer, snap buffer.Snapshot, q *selQuery, deadline time.Time) (done bool) {
	if q == nil || q.text == "" {
		return true
	}
	e := c.entry(buf, snap, q)
	c.scan(e, snap, q, deadline)
	return e.done
}

// entry returns the scan of q in snap, starting one on a miss.
func (c *documentMatches) entry(buf *buffer.Buffer, snap buffer.Snapshot, q *selQuery) *docScan {
	c.tick++
	ver, fold := snap.Version(), q.re != nil
	for i, e := range c.ents {
		if e == nil {
			continue
		}
		if e.buf == buf && e.ver == ver && e.text == q.text && e.whole == q.whole && e.fold == fold {
			e.used = c.tick
			c.cur = e
			return e
		}
		if e.buf == buf && e.ver != ver {
			c.ents[i] = nil // another version: never hit again
		}
	}
	slot := 0 // an empty slot, else the least recently used
	for i, e := range c.ents {
		if e == nil {
			slot = i
			break
		}
		if e.used < c.ents[slot].used {
			slot = i
		}
	}
	e := &docScan{buf: buf, ver: ver, text: q.text, whole: q.whole, fold: fold, used: c.tick}
	c.ents[slot] = e
	c.scans++
	c.cur = e
	return e
}

// scan scans e's lines on (at least one chunk), reading the clock after
// each chunk of work (docScanChunk, docScanLines), until it is done or deadline has passed.
func (c *documentMatches) scan(e *docScan, snap buffer.Snapshot, q *selQuery, deadline time.Time) {
	now, chunk := c.now, c.chunk
	if now == nil {
		now = scanClock
	}
	if chunk <= 0 {
		chunk = docScanChunk
	}
	n, work := snap.LineCount(), 0
	for !e.done {
		if e.next >= n || len(e.ranges) >= maxFindMatches {
			e.done = true
			break
		}
		text := snap.Line(e.next)
		c.scratch = q.matchesLimit(text, c.scratch[:0], maxFindMatches-len(e.ranges))
		ls := snap.LineStart(e.next)
		for _, m := range c.scratch {
			e.ranges = append(e.ranges, brange{ls + m.a, ls + m.b})
		}
		e.next++
		if work += len(text) + docScanLineCost; work >= chunk || e.next%docScanLines == 0 {
			work = 0
			if !now().Before(deadline) {
				break
			}
		}
	}
	if !e.done && (e.next >= n || len(e.ranges) >= maxFindMatches) {
		e.done = true
	}
}

// lineMatches copies the cached document matches of line [ls, le) into
// dst as line-relative ranges, without changing the cache.
func lineMatches(ms []brange, ls, le int, dst []brange) []brange {
	i := sort.Search(len(ms), func(i int) bool { return ms[i].a >= ls })
	for ; i < len(ms) && ms[i].a < le; i++ {
		dst = append(dst, brange{ms[i].a - ls, ms[i].b - ls})
	}
	return dst
}

// selQuery is what the selection matches search for.
type selQuery struct {
	text  string
	whole bool
	re    *regexp.Regexp // case-insensitive matcher; nil: match case
}

// selHLState caches the selection matches' query.
type selHLState struct {
	buf  *buffer.Buffer
	ver  uint64
	sels uint64
	q    *selQuery
}

// selectionQuery returns this frame's selection match query, nil for none.
func (w *ViewW) selectionQuery(snap buffer.Snapshot) *selQuery {
	st, vs := w.st, w.vs
	if !w.opts.SelectionHighlight || st.LargeFile() {
		return nil
	}
	c := &vs.selHL
	buf, ver, sk := st.Model.Buffer(), snap.Version(), selsKey(st.Sels)
	if c.buf == buf && c.ver == ver && c.sels == sk {
		return c.q
	}
	*c = selHLState{buf: buf, ver: ver, sels: sk, q: w.newSelQuery(snap)}
	return c.q
}

func (w *ViewW) newSelQuery(snap buffer.Snapshot) *selQuery {
	st := w.st
	sels := st.sels()
	p := sels[0]
	a, b := clampOff(snap, p.Anchor), clampOff(snap, p.Head)
	a, b = min(a, b), max(a, b)
	if a == b || snap.LineOf(a) != snap.LineOf(b) {
		return nil
	}
	q := &selQuery{text: snap.Slice(a, b)}
	matchCase := false
	if f := st.find; f != nil && f.ver == snap.Version() && slices.Equal(f.sels, st.Sels) {
		// The addSelectionToNextFindMatch session's options.
		q.text, q.whole, matchCase = f.text, f.whole, f.re == nil && !f.fold
	} else if len(sels) > 1 {
		for _, s := range sels[1:] {
			x, y := clampOff(snap, s.Anchor), clampOff(snap, s.Head)
			if !strings.EqualFold(snap.Slice(min(x, y), max(x, y)), q.text) {
				return nil
			}
		}
	}
	if strings.Trim(q.text, " \t") == "" || strings.ContainsAny(q.text, "\n") {
		return nil
	}
	if len(utf16.Encode([]rune(q.text))) > selectionHighlightMax {
		return nil
	}
	if !matchCase {
		q.re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(q.text))
	}
	return q
}

// matchesLimit appends at most limit matches of q in line to dst. Whole
// word filtering happens before counting the quota; the caller filters
// selections only after the document's first maxFindMatches were found.
func (q *selQuery) matchesLimit(line string, dst []brange, limit int) []brange {
	if limit <= 0 || q.text == "" {
		return dst
	}
	added := 0
	if q.re == nil {
		for from := 0; from <= len(line)-len(q.text); {
			i := strings.Index(line[from:], q.text)
			if i < 0 {
				break
			}
			a, b := from+i, from+i+len(q.text)
			if !q.whole || wholeWordMatch(line, a, b) {
				dst = append(dst, brange{a, b})
				added++
				if added == limit {
					break
				}
			}
			from = b
		}
		return dst
	}
	// FindAll with a finite limit cannot be used for whole-word searches:
	// partial-word matches do not consume the quota. Walk the literal regex
	// incrementally, keeping boundary checks in the original line.
	for from := 0; from < len(line); {
		m := q.re.FindStringIndex(line[from:])
		if m == nil {
			break
		}
		a, b := from+m[0], from+m[1]
		if !q.whole || wholeWordMatch(line, a, b) {
			dst = append(dst, brange{a, b})
			added++
			if added == limit {
				break
			}
		}
		from = b
	}
	return dst
}

// dropSelected removes from ms (bytes of the line starting at ls) the
// matches that overlap a non-empty selection, or equal any selection.
func dropSelected(ms []brange, ls int, vis []visSel) []brange {
	out := ms[:0]
	for _, m := range ms {
		a, b := ls+m.a, ls+m.b
		keep := true
		for _, s := range vis {
			if s.a == a && s.b == b || s.a != s.b && s.a < b && a < s.b {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, m)
		}
	}
	return out
}
