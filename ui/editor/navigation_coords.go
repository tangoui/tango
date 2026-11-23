package editor

// Fixed navigation surfaces use the already-established view-line mapping.
// These readers never measure a line, including a folded/remote marker line.
func (w *ViewW) navigationRow(line int) int {
	vl := &w.st.vl
	line = max(0, min(line, vl.n-1))
	shown, hidden := vl.visibleLine(line)
	row := vl.viewOf(shown)
	if hidden {
		row += vl.rowsOf(shown) - 1
	}
	return row
}

func (w *ViewW) navigationOffset(off int, up bool) int {
	snap := w.st.Snapshot()
	off = clampOff(snap, off)
	line := snap.LineOf(off)
	vl := &w.st.vl
	if vl.hidden(line) {
		return w.navigationRow(line)
	}
	row := vl.viewOf(line)
	if lb := vl.breaks[line]; lb != nil {
		row += lb.rowOf(off-snap.LineStart(line), up)
	}
	return row
}

// navigationHeight includes scrollBeyondLastLine's trailing viewport, exactly
// like Viewport.setScrollY: the last view row can reach the viewport top.
func (w *ViewW) navigationHeight() float32 {
	v := w.st.Viewport
	if v.NoScrollBeyondLastLine {
		return max(1, float32(w.st.vl.Count())*v.LineHeight+v.PadTop+v.PadBottom)
	}
	return max(1, float32(max(0, w.st.vl.Count()-1))*v.LineHeight+v.PadTop+v.Height)
}

func (w *ViewW) jumpViewRow(row int) {
	st := w.st
	vl := st.lines()
	row = max(0, min(row, vl.Count()-1))
	line, part := vl.find(row)
	col := 0
	if lb := vl.breaks[line]; lb != nil {
		col, _ = lb.rowRange(max(0, min(part, lb.rows()-1)), lineEnd(st.Snapshot(), line)-st.Snapshot().LineStart(line))
	}
	st.MoveCaret(st.Snapshot().LineStart(line)+col, false)
	// Input may measure the target, but paint's mapping is never changed here.
	vl = st.lines()
	st.Viewport.setScrollY(float64(vl.viewOf(line)+min(part, vl.rowsOf(line)-1))*float64(st.Viewport.LineHeight), vl.Count())
}
