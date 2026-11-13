// Package highlight runs Monarch tokenization incrementally for the code
// editor view: the UI thread tokenizes what is about to be drawn within a
// time budget (Highlight), and a background goroutine (Start) catches up on
// the rest of the document from immutable buffer snapshots.
//
// Storage. Every line keeps its end State (a uint32), so any line can be
// re-tokenized from the line above. Token slices are kept only in token
// windows of ±TokenWindow lines around the line each view most recently
// requested; outside them they are dropped and recomputed from the stored
// start state on demand. Each view of the document (a ViewKey; the methods
// without a key use DefaultView) has a window of its own, up to four, the
// least recently used evicted beyond that, so two views far apart in one
// document do not move one shared window back and forth every frame.
//
// Invalidation. Edit maps each edit to lines of the old snapshot and the
// number of line breaks it inserts, shifts the per-line arrays accordingly and
// marks the edited lines dirty. Re-tokenizing always starts at the first dirty
// line, so its start state is final. After tokenizing a line, the next line is
// marked dirty unless the new end state equals the stored one. A Degraded
// line (monarch.LineInfo) never counts as "state unchanged": the next line is
// always re-tokenized, and the degraded line itself is retried after
// RetryDelay, up to MaxRetries times, before its result is accepted. Until
// then it and every line below it are not final.
//
// Background and versions. Snapshots are immutable, so the background
// goroutine tokenizes a batch of lines outside the lock and applies the result
// under the lock. Each batch records the Highlighter's generation, which
// changes on every Edit, Reset and resynchronisation (a snapshot that did not
// come through Edit). A batch whose generation is outdated is discarded.
// Within the same generation (same text, same Lexer) each line's result is
// re-validated before it is applied: it is used only if the start state it
// was computed from still equals the stored end state of the line above
// (the foreground may have re-tokenized those lines meanwhile); the rest of
// the batch is dropped at the first mismatch.
//
// Contracts:
//
//   - One Highlighter follows one buffer.Buffer: call Edit for every new
//     snapshot (or pass it to Highlight). A snapshot whose Version differs
//     from the last one seen, without an Edit linking them, makes the whole
//     document dirty (correct, but a full re-tokenization).
//   - After monarch.Registry.Add replaces the grammar, call Reset with the new
//     Lexer: every stored State belongs to the old Lexer.
//   - Tokens that are not final are a display hint: they tokenize the current
//     line text from a guessed start state (the stored state of the line
//     above), so their offsets always match the text, but types may be stale.
package highlight

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// ViewKey identifies one view of the document for its token window (see
// HighlightFor). DefaultView is the key of Highlight, Tokens and WindowRange.
type ViewKey uint64

// DefaultView is the ViewKey of the single-view methods.
const DefaultView ViewKey = 0

var lastViewKey atomic.Uint64

// NewViewKey returns a ViewKey no other call returns (never DefaultView).
func NewViewKey() ViewKey { return ViewKey(lastViewKey.Add(1)) }

// Defaults for the exported tuning fields.
const (
	DefaultTokenWindow = 2000
	DefaultRetryDelay  = time.Second
	DefaultMaxRetries  = 3
)

// Background batch limits: a batch holds no lock while tokenizing; it ends
// after this many lines or this much time, whichever comes first.
const (
	batchLines = 2048
	batchTime  = 4 * time.Millisecond
)

// Highlighter tokenizes one document incrementally. Its methods are safe for
// concurrent use; the tuning fields must be set before the first call.
type Highlighter struct {
	// TokenWindow is how many lines above and below each view's most
	// recently requested line keep their token slices (default 2000).
	TokenWindow int
	// RetryDelay is the delay before a Degraded line is tokenized again
	// (default 1s).
	RetryDelay time.Duration
	// MaxRetries is how many times a Degraded line is retried before its
	// degraded tokens are accepted as final (default 3; 0 accepts at once).
	MaxRetries int

	mu      sync.Mutex
	lx      *monarch.Lexer
	snap    buffer.Snapshot
	have    bool   // snap is set
	gen     uint64 // see package doc, "Background and versions"
	st      store
	scratch []monarch.Token // foreground tokens for lines outside the window
	regs    []region        // Edit's scratch buffer

	wake chan struct{}

	tokenized atomic.Int64 // lines tokenized (tests)
}

// New returns a Highlighter for lx. A nil lx means plain text: every line is
// final and has no tokens.
func New(lx *monarch.Lexer) *Highlighter {
	return &Highlighter{
		TokenWindow: DefaultTokenWindow,
		RetryDelay:  DefaultRetryDelay,
		MaxRetries:  DefaultMaxRetries,
		lx:          lx,
		wake:        make(chan struct{}, 1),
	}
}

// Reset switches to lx (for example after monarch.Registry.Add replaced the
// grammar) and marks every line dirty.
func (h *Highlighter) Reset(lx *monarch.Lexer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lx = lx
	h.gen++
	if h.have {
		h.st.reset(h.snap.LineCount())
	}
	h.signal()
}

// Sync makes snap the current snapshot without tokenizing anything or
// moving a token window: a snapshot other than the last one seen (one that
// did not come through Edit) makes the whole document dirty, as Highlight
// would. It wakes the background goroutine. Callers that skipped Edit for
// a while (a document shown as plain text) call it after Reset.
func (h *Highlighter) Sync(snap buffer.Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sync(snap)
	h.signal()
}

func (h *Highlighter) signal() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// resync makes s the current snapshot with every line dirty.
func (h *Highlighter) resync(s buffer.Snapshot) {
	h.snap, h.have = s, true
	h.gen++
	h.st.reset(s.LineCount())
	h.signal()
}

// sync makes s current; a snapshot not seen through Edit resynchronises.
func (h *Highlighter) sync(s buffer.Snapshot) {
	if !h.have || s.Version() != h.snap.Version() || s.LineCount() != h.st.n() {
		h.resync(s)
	}
}

// Edit records that new was produced from old by edits (as passed to
// buffer.Snapshot.Apply or buffer.Buffer.Edit): lines touched by an edit
// become dirty, the per-line arrays shift by the number of inserted and
// deleted line breaks, and every other line keeps its state and tokens. If
// old is not the snapshot the Highlighter last saw, the whole document
// becomes dirty.
func (h *Highlighter) Edit(old, new buffer.Snapshot, edits []buffer.Edit) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.have || old.Version() != h.snap.Version() || old.LineCount() != h.st.n() {
		h.resync(new)
		return
	}
	regs := h.regs[:0]
	defer func() { h.regs = regs[:0] }()
	total := 0
	for _, e := range edits {
		if e.Start == e.End && e.Text == "" {
			continue
		}
		sl, el := old.LineOf(e.Start), old.LineOf(e.End)
		d := countBreaks(e.Text) - (el - sl)
		total += d
		if n := len(regs); n > 0 && sl <= regs[n-1].el { // 与上一段同行：合并
			regs[n-1].el = el
			regs[n-1].delta += d
			continue
		}
		nsl := sl
		if n := len(regs); n > 0 {
			p := regs[n-1]
			nsl = sl + p.nsl + p.m() - (p.el + 1)
		}
		regs = append(regs, region{sl: sl, el: el, delta: d, nsl: nsl})
	}
	if old.LineCount()+total != new.LineCount() {
		h.resync(new) // edits do not describe old → new
		return
	}
	h.snap = new
	h.gen++
	if len(regs) > 0 {
		h.st.splice(regs, h.keep())
	}
	h.signal()
}

// countBreaks counts the line breaks of an inserted text the way
// buffer.Snapshot.Apply normalises them ("\r\n", "\r" and "\n").
func countBreaks(t string) int {
	return strings.Count(t, "\n") + strings.Count(t, "\r") - strings.Count(t, "\r\n")
}

func (h *Highlighter) keep() int { return max(h.TokenWindow, 0) }

// Highlight tokenizes dirty lines before upToLine, starting at the first
// dirty line, until they are all done or budget has elapsed (it always
// finishes the line it started; a budget <= 0 tokenizes nothing). It reports
// whether every line before upToLine is final. Degraded lines whose retry is
// due are retried first. upToLine also moves the token window of
// DefaultView; it is HighlightFor(DefaultView, ...).
func (h *Highlighter) Highlight(snap buffer.Snapshot, upToLine int, budget time.Duration) bool {
	return h.HighlightFor(DefaultView, snap, upToLine, budget)
}

// HighlightFor is Highlight for view: upToLine moves view's token window.
func (h *Highlighter) HighlightFor(view ViewKey, snap buffer.Snapshot, upToLine int, budget time.Duration) bool {
	start := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lx == nil {
		return true
	}
	h.sync(snap)
	n := h.st.n()
	upTo := min(max(upToLine, 0), n)
	h.setFocus(view, max(upTo-1, 0))
	h.st.promoteDue(upTo, start)
	deadline := start.Add(budget)
	for budget > 0 {
		i := h.st.nextDirty()
		if i >= upTo {
			break
		}
		h.tokenizeLine(i)
		if !time.Now().Before(deadline) {
			break
		}
	}
	return h.st.nextDirty() >= upTo && h.st.minPending() >= upTo
}

// Done reports whether every line of snap is final: snap is the current
// snapshot, no line is dirty and no Degraded line awaits a retry. Unlike
// Highlight(snap, snap.LineCount(), 0) it changes nothing: it tokenizes
// nothing, does not move the token window and does not adopt snap (a
// snapshot the Highlighter has not seen is simply not done).
func (h *Highlighter) Done(snap buffer.Snapshot) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lx == nil {
		return true
	}
	if !h.have || snap.Version() != h.snap.Version() || snap.LineCount() != h.st.n() {
		return false
	}
	n := h.st.n()
	return h.st.nextDirty() >= n && h.st.minPending() >= n
}

// FinalBefore reports whether every line of snap before line is final,
// like Highlight(snap, line, 0) but without side effects: it tokenizes
// nothing, moves no token window and does not adopt snap (a snapshot the
// Highlighter has not seen is not final). The editor's typing assistance
// uses it as Monaco's isCheapToTokenize.
func (h *Highlighter) FinalBefore(snap buffer.Snapshot, line int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lx == nil {
		return true
	}
	if !h.have || snap.Version() != h.snap.Version() || snap.LineCount() != h.st.n() {
		return false
	}
	upTo := min(max(line, 0), h.st.n())
	return h.st.nextDirty() >= upTo && h.st.minPending() >= upTo
}

// FinalLines returns how many leading lines of snap are final, like the
// largest line for which FinalBefore reports true, without side effects:
// 0 for a snapshot the Highlighter has not seen, every line without a
// lexer. The editor's bracket pairs scan only final lines.
func (h *Highlighter) FinalLines(snap buffer.Snapshot) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lx == nil {
		return snap.LineCount()
	}
	if !h.have || snap.Version() != h.snap.Version() || snap.LineCount() != h.st.n() {
		return 0
	}
	return min(h.st.nextDirty(), h.st.minPending(), h.st.n())
}

// WindowRange returns the lines [lo, hi) whose token slices DefaultView's
// token window currently keeps (diagnostics and tests: a caller can check
// that it does not move the window back and forth); it is
// WindowRangeFor(DefaultView).
func (h *Highlighter) WindowRange() (lo, hi int) { return h.WindowRangeFor(DefaultView) }

// WindowRangeFor returns the lines [lo, hi) of view's token window, 0, 0
// when view has none (never requested, or evicted).
func (h *Highlighter) WindowRangeFor(view ViewKey) (lo, hi int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w := h.st.window(view); w != nil {
		return w.lo, w.lo + len(w.slots)
	}
	return 0, 0
}

func (h *Highlighter) setFocus(view ViewKey, line int) {
	h.st.focus(view, line, h.keep())
}

// DropView forgets view's token window (a closed editor), so it no longer
// holds token slices or takes one of the window slots. Unknown keys and
// DefaultView without a window are ignored.
func (h *Highlighter) DropView(view ViewKey) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.st.dropWindow(view)
}

func (h *Highlighter) startState(i int) monarch.State {
	if i == 0 {
		return h.lx.Initial()
	}
	return h.st.states[i-1]
}

// tokenizeLine tokenizes line i of the current snapshot under the lock.
func (h *Highlighter) tokenizeLine(i int) {
	start := h.startState(i)
	sl := h.st.slot(i)
	dst := h.scratch[:0]
	if sl != nil {
		dst = sl.toks[:0]
	}
	toks, end, info := h.lx.TokenizeLineInfo(h.snap.Line(i), start, dst)
	h.tokenized.Add(1)
	if sl == nil {
		h.scratch = toks
	}
	h.apply(i, start, end, info.Degraded, toks, sl != nil, time.Now())
}

// apply records the result of tokenizing line i from start. keep says toks
// may be stored in the window.
func (h *Highlighter) apply(i int, start, end monarch.State, isDegraded bool, toks []monarch.Token, keep bool, now time.Time) {
	st := &h.st
	old := st.states[i]
	st.states[i] = end
	st.dirty[i] = 0
	first := true
	for k := range st.wins {
		w := &st.wins[k]
		if !w.contains(i) {
			continue
		}
		sl := &w.slots[i-w.lo]
		switch {
		case !keep:
			sl.ok = false
		case first: // the slot tokenizeLine took toks' buffer from (store.slot)
			*sl = slot{toks: toks, start: start, ok: true}
		default: // overlapping windows: each slot owns its buffer
			*sl = slot{toks: append(sl.toks[:0], toks...), start: start, ok: true}
		}
		first = false
	}
	st.noteResult(i, isDegraded, now, h.MaxRetries, h.RetryDelay)
	// A degraded line returns its input state, which may differ from its true
	// end state: never stop below it.
	if (isDegraded || end != old) && i+1 < st.n() {
		st.markDirty(i + 1)
	}
}

// Tokens appends line's tokens to dst[:0] and reports whether they are final.
// Tokens that are not final tokenize the current text from the line above's
// stored state (a guess; see the package doc); a line outside every token
// window moves DefaultView's window to it. For a snapshot other than the
// current one, or a line out of range, it returns dst[:0] and false. It is
// TokensFor(DefaultView, ...).
func (h *Highlighter) Tokens(snap buffer.Snapshot, line int, dst []monarch.Token) ([]monarch.Token, bool) {
	return h.TokensFor(DefaultView, snap, line, dst)
}

// TokensFor is Tokens for view: a line in view's window, or else in another
// view's, uses that window's cached tokens; a line in none moves view's
// window to it.
func (h *Highlighter) TokensFor(view ViewKey, snap buffer.Snapshot, line int, dst []monarch.Token) ([]monarch.Token, bool) {
	dst = dst[:0]
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lx == nil {
		return dst, true
	}
	if !h.have || snap.Version() != h.snap.Version() || line < 0 || line >= h.st.n() {
		return dst, false
	}
	if w := h.st.window(view); w != nil {
		h.st.touch(w)
	}
	sl := h.st.slotFor(view, line)
	if sl == nil {
		h.setFocus(view, line)
		sl = h.st.slotFor(view, line)
	}
	start := h.startState(line)
	if !sl.ok || sl.start != start {
		toks, _, info := h.lx.TokenizeLineInfo(h.snap.Line(line), start, sl.toks[:0])
		h.tokenized.Add(1)
		// Cache the result either way: repeated calls must not pay the
		// line budget again.
		*sl = slot{toks: toks, start: start, ok: true}
		if info.Degraded && !h.st.acceptedDegraded(line) {
			// A degraded recompute (wall-clock budget) is not a final
			// result: hand the line to the normal retry flow (RetryDelay,
			// MaxRetries). It stays not final until a retry succeeds or the
			// retries are used up and the result is accepted. A line whose
			// degraded result was already accepted stays accepted.
			h.st.noteResult(line, true, time.Now(), h.MaxRetries, h.RetryDelay)
			h.signal() // the background goroutine re-reads the retry time
		}
	}
	return append(dst, sl.toks...), h.st.final(line)
}

// TokenizeSpan tokenizes span, a piece of line of snap that starts at the
// line's first byte or at a point the caller treats as such (the View's
// visible window of a long line), with the current lexer (so Reset is
// honoured) from the stored start state of line: the end state of the line
// above as last tokenized, final or not. Token offsets are relative to span.
// It is a display hint like non-final Tokens: nothing is stored. For a
// snapshot other than the current one, or a line out of range, it starts
// from the lexer's initial state. Plain text (nil lexer) yields dst[:0].
func (h *Highlighter) TokenizeSpan(snap buffer.Snapshot, line int, span string, dst []monarch.Token) []monarch.Token {
	dst = dst[:0]
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.lx == nil {
		return dst
	}
	start := h.lx.Initial()
	if h.have && snap.Version() == h.snap.Version() && line >= 0 && line < h.st.n() {
		start = h.startState(line)
	}
	toks, _, _ := h.lx.TokenizeLineInfo(span, start, dst)
	return toks
}

// Start runs the background catch-up goroutine until ctx is done or
// snapshots is closed. Each snapshot received wakes it (the first one also
// initialises a Highlighter that has not seen a snapshot yet); it then
// tokenizes every remaining dirty line of the current snapshot in batches,
// calling ready after a batch that changed lines inside the token window and
// when the document is fully tokenized. Snapshots that are not the current one
// are ignored: the caller is expected to pass them to Edit, which also wakes
// the goroutine. ready is called from the background goroutine. Call Start
// at most once.
func (h *Highlighter) Start(ctx context.Context, snapshots <-chan buffer.Snapshot, ready func()) {
	go h.run(ctx, snapshots, ready)
}

func (h *Highlighter) run(ctx context.Context, snapshots <-chan buffer.Snapshot, ready func()) {
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	pending := false // lines applied since the last ready call
	var res batchResult
	for {
		if ctx.Err() != nil {
			return
		}
	drain:
		for {
			select {
			case s, ok := <-snapshots:
				if !ok {
					return
				}
				h.offer(s)
			default:
				break drain
			}
		}
		res = h.batch()
		if res.applied > 0 {
			pending = true
			if res.inWindow {
				ready()
				pending = false
			}
		}
		if !res.done {
			continue
		}
		if pending {
			ready()
			pending = false
		}
		var tc <-chan time.Time
		if !res.retryAt.IsZero() {
			timer.Reset(max(time.Until(res.retryAt), 0))
			tc = timer.C
		}
		select {
		case <-ctx.Done():
			return
		case s, ok := <-snapshots:
			if !ok {
				return
			}
			h.offer(s)
		case <-h.wake:
		case <-tc:
		}
		if tc != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
}

// offer initialises a Highlighter that has not seen a snapshot yet.
func (h *Highlighter) offer(s buffer.Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.have {
		h.resync(s)
	}
}

type batchResult struct {
	applied  int
	inWindow bool
	done     bool      // nothing left to do now
	retryAt  time.Time // when done: the next degraded-line retry, if any
}

type lineResult struct {
	start, end monarch.State
	degraded   bool
	toks       []monarch.Token // nil: computed into scratch, not kept
}

// testHookBatchTokenized, when set by a test, runs after a background batch
// is tokenized and before it is applied (outside the lock).
var testHookBatchTokenized func()

// batch tokenizes one batch of dirty lines outside the lock and applies it.
func (h *Highlighter) batch() batchResult {
	h.mu.Lock()
	if h.lx == nil || !h.have {
		h.mu.Unlock()
		return batchResult{done: true}
	}
	n := h.st.n()
	retryAt := h.st.promoteDue(n, time.Now())
	first := h.st.nextDirty()
	if first >= n {
		h.mu.Unlock()
		return batchResult{done: true, retryAt: retryAt}
	}
	gen, snap, lx := h.gen, h.snap, h.lx
	st := h.startState(first)
	last := min(first+batchLines, n) // exclusive
	oldStates := append([]monarch.State(nil), h.st.states[first:last]...)
	oldDirty := append([]byte(nil), h.st.dirty[first:min(last+1, n)]...)
	ranges := h.st.ranges(nil) // the token windows, read outside the lock
	h.mu.Unlock()

	var scratch []monarch.Token
	results := make([]lineResult, 0, 64)
	t0 := time.Now()
	for j := first; j < last; j++ {
		var dst []monarch.Token
		inWin := false
		for _, r := range ranges {
			if j >= r[0] && j < r[1] {
				inWin = true
				break
			}
		}
		if !inWin {
			dst = scratch[:0]
		}
		toks, end, info := lx.TokenizeLineInfo(snap.Line(j), st, dst)
		h.tokenized.Add(1)
		r := lineResult{start: st, end: end, degraded: info.Degraded}
		if inWin {
			r.toks = toks
		} else {
			scratch = toks
		}
		results = append(results, r)
		st = end
		// Stop where the state settles and the next line is clean anyway.
		if !info.Degraded && end == oldStates[j-first] && (j+1 >= n || oldDirty[j+1-first] == 0) {
			break
		}
		if time.Since(t0) >= batchTime {
			break
		}
	}

	if testHookBatchTokenized != nil {
		testHookBatchTokenized()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.gen != gen {
		return batchResult{} // outdated: discard and look again
	}
	res := batchResult{}
	now := time.Now()
	for k, r := range results {
		j := first + k
		if h.startState(j) != r.start {
			break // the lines above changed meanwhile
		}
		if h.st.dirty[j] == 0 && h.st.states[j] == r.end {
			// The foreground already tokenized this line from the same start
			// state in this generation: applying again would count a degraded
			// line's retry twice (or reset its retry time).
			continue
		}
		h.apply(j, r.start, r.end, r.degraded, r.toks, r.toks != nil, now)
		res.applied++
		if h.st.slot(j) != nil {
			res.inWindow = true
		}
	}
	return res
}
