//go:build tango_dev

package ui

// checkThread panics when the runner is bound to a UI thread (BindThread)
// and the caller is not on it. Unbound runners are never checked.
func (r *Runner) checkThread() {
	if t := r.thread.Load(); t != nil && !t.On() {
		panic(offThreadMsg)
	}
}

// checkThread checks the thread of the context's runner (none for a context
// without a runner).
func (c *Context) checkThread() {
	if c != nil && c.runner != nil {
		c.runner.checkThread()
	}
}
