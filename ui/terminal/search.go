package terminal

// Search.
//
// ViewW.Search finds a query across the scrollback and the active area
// (vt.Screen.Search), scrolls the viewport to a hit and highlights it
// through the selection machinery, so the current match is drawn in the
// selection color and copied by Cmd+C like any selection. SearchStep
// moves to the next/previous hit (wrapping); ClearSearch ends the search.
// Only the current hit is highlighted; matches are recomputed when the
// query changes. The host drives these from a search bar and reads the
// hit count and position from the returned values or SearchInfo.

// searchHit is a match in stable line numbers (see selPos).
type searchHit struct{ row, from, to int }

// Search runs query and highlights the first hit. It returns the number
// of hits and the 1-based position of the current one (0, 0 when none).
// An empty query clears the hits. The view need not be painting.
func (w *ViewW) Search(query string, caseSensitive bool) (count, at int) {
	vs := w.vs
	vs.searchOn, vs.searchQuery, vs.searchCase = true, query, caseSensitive
	vs.searchHits, vs.searchAt = nil, 0
	if query == "" {
		vs.sel.clear()
		return 0, 0
	}
	v := w.t.Lock()
	s := v.Screen()
	ev := int(s.Evicted())
	for _, m := range s.Search(query, caseSensitive) {
		vs.searchHits = append(vs.searchHits, searchHit{row: ev + m.Line, from: m.From, to: m.To})
	}
	w.t.Unlock()
	if len(vs.searchHits) == 0 {
		vs.sel.clear()
		return 0, 0
	}
	w.searchGoto()
	return len(vs.searchHits), vs.searchAt + 1
}

// SearchStep moves to the next (dir > 0) or previous (dir < 0) hit,
// wrapping around, and returns the count and 1-based position.
func (w *ViewW) SearchStep(dir int) (count, at int) {
	vs := w.vs
	n := len(vs.searchHits)
	if !vs.searchOn || n == 0 {
		return 0, 0
	}
	if dir < 0 {
		vs.searchAt = (vs.searchAt - 1 + n) % n
	} else {
		vs.searchAt = (vs.searchAt + 1) % n
	}
	w.searchGoto()
	return n, vs.searchAt + 1
}

// ClearSearch ends the search and removes its highlight.
func (w *ViewW) ClearSearch() {
	vs := w.vs
	vs.searchOn, vs.searchQuery, vs.searchCase = false, "", false
	vs.searchHits, vs.searchAt = nil, 0
	vs.sel.clear()
}

// SearchInfo returns the hit count and 1-based current position, or 0, 0
// when no search is active or it has no hits.
func (w *ViewW) SearchInfo() (count, at int) {
	if !w.vs.searchOn || len(w.vs.searchHits) == 0 {
		return 0, 0
	}
	return len(w.vs.searchHits), w.vs.searchAt + 1
}

// searchGoto scrolls the current hit into view and selects it.
func (w *ViewW) searchGoto() {
	vs := w.vs
	if len(vs.searchHits) == 0 {
		return
	}
	h := vs.searchHits[vs.searchAt]
	vs.sel.anchor, vs.sel.head = selPos{h.row, h.from}, selPos{h.row, h.to}
	vs.sel.on, vs.sel.dragging, vs.sel.unit = true, false, unitChar
	v := w.t.Lock()
	s := v.Screen()
	abs := h.row - int(s.Evicted())
	s.SetViewOffset(s.ScrollbackLen() - abs + 2)
	w.t.Unlock()
}
