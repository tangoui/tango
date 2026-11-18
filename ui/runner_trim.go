package ui

// TrimCaches asks the runner's renderer to discard rebuildable caches and
// requests another frame. Call on the UI thread, between frames. It does not
// clear application state, editor LineCaches, current Layouts or the current
// presented pixels. A renderer without Trim() is left alone.
func (r *Runner) TrimCaches() {
	r.checkThread()
	if r.closed.Load() {
		return
	}
	if trimmer, ok := r.renderer.(interface{ Trim() }); ok {
		trimmer.Trim()
	}
	r.Invalidate()
}
