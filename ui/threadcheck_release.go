//go:build !tango_dev

package ui

// Production builds do not check the caller's thread (threadcheck_dev.go).

func (r *Runner) checkThread()  {}
func (c *Context) checkThread() {}
