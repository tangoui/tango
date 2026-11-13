package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/theme"
)

// Node environment capture (docs/specs/rtl-and-component-tokens.md §2).
//
// Paint and measure callbacks run after the page function has returned,
// when every ConfigProvider scope is long closed. Each node therefore
// records the scope it was built in (theme, resolved direction, locale);
// the Runner switches c.Theme, c.Locale() and c.IsRTL() to that scope
// while it paints or measures the node. Components need no changes: they
// keep reading c.Theme.

// env is the configuration scope a node was built in. Envs are immutable
// once handed out (nodes keep pointers to them), except the window's root
// env, which the Runner refreshes at the start of every build.
type env struct {
	th  *theme.Theme
	loc *Locale // nil: the default locale (SetDefaultLocale)
	rtl bool
	c   *Context // owner, used by measure wrappers
}

// envSlot caches the env of one ConfigProvider across frames, so a scope
// whose values did not change allocates nothing.
type envSlot struct {
	e *env
}

// rootEnvPtr returns the window's root env.
func (c *Context) rootEnvPtr() *env { return &c.envRoot }

// resetRootEnv refreshes the root env from the window theme and the
// default direction; called at the start of every build pass.
func (c *Context) resetRootEnv() {
	e := &c.envRoot
	e.c = c
	e.th = c.Theme
	e.loc = nil
	e.rtl = resolveRTL(DirAuto, nil)
	c.useEnv(e)
}

// useEnv makes e the current scope: c.env, c.Theme and the direction.
func (c *Context) useEnv(e *env) {
	c.env, c.Theme, c.dir.rtl = e, e.th, e.rtl
}

// enterEnv switches to e and returns what restore needs. The theme is
// saved separately from the env because a root ConfigProvider may replace
// the window theme while a scope is open.
func (c *Context) enterEnv(e *env) (prev *env, prevTh *theme.Theme) {
	prev, prevTh = c.env, c.Theme
	c.useEnv(e)
	return prev, prevTh
}

// leaveEnv undoes enterEnv.
func (c *Context) leaveEnv(prev *env, prevTh *theme.Theme) {
	c.env, c.Theme = prev, prevTh
	if prev != nil {
		c.dir.rtl = prev.rtl
	}
}

// deriveEnv returns the env of a scope nested in the current one with the
// given values. Without overrides it is the current env itself; otherwise
// the env cached under key is reused while its values stay the same.
func (c *Context) deriveEnv(key ID, th *theme.Theme, rtl bool, loc *Locale) *env {
	p := c.env
	if p != nil && p.th == th && p.rtl == rtl && p.loc == loc {
		return p
	}
	slot := State[envSlot](c, key, nil)
	if e := slot.e; e != nil && e.th == th && e.rtl == rtl && e.loc == loc {
		return e
	}
	// A fresh object, never an in-place update: nodes built earlier in
	// this pass may still point to the old one.
	slot.e = &env{th: th, loc: loc, rtl: rtl, c: c}
	return slot.e
}

// stampEnv records the current scope on n and passes the direction to the
// layout engine. c.add calls it; nodes created without c.add (overlay
// roots) call it explicitly.
func (c *Context) stampEnv(n *node) {
	if e := c.env; e != nil {
		n.env = e
		n.ln.RTL = e.rtl
	}
}

// wrapMeasure makes the measure function of a node built in a nested
// scope run with that scope current. Nodes of the root scope (the common
// case) are left alone, so they pay nothing.
func wrapMeasure(n *node) {
	e := n.env
	if e == nil || e.c == nil || e == &e.c.envRoot || n.ln.Measure == nil {
		return
	}
	m := n.ln.Measure
	n.ln.Measure = func(maxW, maxH float32) geom.Size {
		c := e.c
		if c.env == e {
			return m(maxW, maxH)
		}
		prev, prevTh := c.enterEnv(e)
		sz := m(maxW, maxH)
		c.leaveEnv(prev, prevTh)
		return sz
	}
}
