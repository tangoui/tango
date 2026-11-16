package highlight

import (
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"sort"
)

// LexerIdentity returns the current immutable lexer. Preview caches use its
// identity to invalidate after Reset without discarding unchanged lines on Edit.
func (h *Highlighter) LexerIdentity() *monarch.Lexer {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lx
}

// PeekTokens copies tokens already cached for line, without tokenizing,
// creating or moving a token window, or changing window LRU order. found
// distinguishes an empty token list from an uncached line. Consumers such
// as the minimap must not evict the visible editor's token window.
func (h *Highlighter) PeekTokens(snap buffer.Snapshot, line int, dst []monarch.Token) (tokens []monarch.Token, found bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	dst = dst[:0]
	if !h.have || h.snap.Version() != snap.Version() || line < 0 || line >= h.st.n() {
		return dst, false
	}
	start := h.startState(line)
	for k := range h.st.wins {
		w := &h.st.wins[k]
		if w.contains(line) {
			sl := &w.slots[line-w.lo]
			if sl.ok && sl.start == start {
				return append(dst, sl.toks...), true
			}
		}
	}
	return dst, false
}

// PeekCompleteTokens copies cached whole-line tokens only when they are final
// for this exact snapshot and tokenizer start state. Accepted degraded previews
// are final for scheduling, but are not complete token results and must miss.
// Like PeekTokens, this never lexes, moves a window or touches window LRU order;
// the returned tokens belong to dst, not to the cache. A miss returns dst[:0].
func (h *Highlighter) PeekCompleteTokens(snap buffer.Snapshot, line int, dst []monarch.Token) (tokens []monarch.Token, found bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	dst = dst[:0]
	// Versions are local to a Buffer: compare the immutable snapshot identity,
	// not only Version, so another document with the same version cannot hit.
	if h.lx == nil || !h.have || h.snap != snap || line < 0 || line >= h.st.n() ||
		!h.st.final(line) || h.st.acceptedDegraded(line) {
		return dst, false
	}
	start := h.startState(line)
	for k := range h.st.wins {
		w := &h.st.wins[k]
		if w.contains(line) {
			sl := &w.slots[line-w.lo]
			if sl.ok && sl.start == start {
				return append(dst, sl.toks...), true
			}
		}
	}
	return dst, false
}

// PeekTokenRange copies only tokens intersecting [from,to), preserving their
// model-line byte and UTF-16 coordinates. It includes the token covering from
// and never moves a window or lexes a line; lookup is logarithmic in the tokens.
func (h *Highlighter) PeekTokenRange(snap buffer.Snapshot, line, from, to int, dst []monarch.Token) ([]monarch.Token, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	dst = dst[:0]
	if !h.have || h.snap.Version() != snap.Version() || line < 0 || line >= h.st.n() {
		return dst, false
	}
	if h.lx == nil || to <= from {
		return dst, true
	}
	start := h.startState(line)
	for k := range h.st.wins {
		w := &h.st.wins[k]
		if !w.contains(line) {
			continue
		}
		sl := &w.slots[line-w.lo]
		if !sl.ok || sl.start != start {
			continue
		}
		a := max(0, sort.Search(len(sl.toks), func(i int) bool { return sl.toks[i].ByteStart > from })-1)
		b := sort.Search(len(sl.toks), func(i int) bool { return sl.toks[i].ByteStart >= to })
		for _, tk := range sl.toks[a:max(a, b)] {
			dst = append(dst, tk)
		}
		return dst, true
	}
	return dst, false
}
