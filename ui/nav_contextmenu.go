package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// ContextMenuAt: a context menu opened at a point, without a trigger widget.
//
// The menu is the Dropdown panel (same tokens, submenus, keyboard and
// shortcut column) built in an app-wide layer, like Confirm: the caller
// opens it once, for example when c.ContextMenuFor(id) reports a secondary
// press or the Menu key, and it stays until an item is chosen, Escape is
// pressed or the pointer goes down outside it. Only one context menu is
// open per window; opening another replaces it.

type contextMenuState struct {
	open     bool
	owner    ID
	pos      geom.Point
	items    []MenuItem
	onSelect func(key string)
	env      *env // scope the menu was opened in (theme, locale, direction)
	seq      int  // open count (restarts the enter animation)
	fresh    bool // (re)opened during this build: skip the outside checks
	panels   []ID
	popup    navPopupState
}

var contextMenuKey = hashID("context-menu")

func init() { globalLayers = append(globalLayers, renderContextMenu) }

// ContextMenuOptions configure ContextMenuAtWith.
type ContextMenuOptions struct {
	// NoAutoSelect opens the menu with no item highlighted, like the
	// context menus of code editors: keyboard focus moves to the menu
	// panel itself (role menu, not a Tab stop), Down/Home highlight the
	// first enabled item, Up/End the last, a letter or digit the first
	// item starting with it, and Enter/Space do nothing until an item is
	// highlighted. Pointer hover, Escape and closing work as usual.
	NoAutoSelect bool
}

// ContextMenuAt opens a context menu of items with its top-left corner at
// the window point pos (top-right in right-to-left scopes); it opens
// upwards / towards the inline start when it does not fit below / after
// the point. owner is the widget the menu belongs to: keyboard focus moves
// to the first enabled item on open (to the panel itself with
// ContextMenuOptions.NoAutoSelect, see ContextMenuAtWith) and returns to
// owner when the menu closes (an item is chosen, Escape, or a press
// outside the menu).
// onSelect receives the key of the chosen leaf item. The menu takes the
// theme, locale and direction of the scope ContextMenuAt is called in.
//
// Call it once to open the menu (not every frame):
//
//	if p, ok := c.ContextMenuFor(area.ID()); ok {
//		ui.ContextMenuAt(c, area.ID(), p, items, func(key string) { ... })
//	}
//
// Calling it again while the menu is open at the same point for the same
// owner only updates items and onSelect; a new point or owner reopens it.
func ContextMenuAt(c *Context, owner ID, pos geom.Point, items []MenuItem, onSelect func(key string)) {
	ContextMenuAtWith(c, owner, pos, items, onSelect, ContextMenuOptions{})
}

// ContextMenuAtWith is ContextMenuAt with options. Options take effect
// when the menu (re)opens.
func ContextMenuAtWith(c *Context, owner ID, pos geom.Point, items []MenuItem, onSelect func(key string), opts ContextMenuOptions) {
	st := State[contextMenuState](c, contextMenuKey, nil)
	st.items, st.onSelect, st.fresh = items, onSelect, true
	if st.open && st.owner == owner && st.pos == pos {
		return
	}
	st.open, st.owner, st.pos, st.env = true, owner, pos, c.env
	st.seq++
	st.panels = nil
	if opts.NoAutoSelect {
		st.popup = navPopupState{bare: true, focusRoot: true} // focus the panel once built
	} else {
		st.popup = navPopupState{focus: 1} // focus the first enabled item once built
	}
	c.invalid = true
}

// CloseContextMenu closes the context menu opened for owner, if any,
// returning focus to owner when it was inside the menu.
func CloseContextMenu(c *Context, owner ID) {
	if st := State[contextMenuState](c, contextMenuKey, nil); st.open && st.owner == owner {
		st.close(c)
	}
}

// ContextMenuOpen reports whether a context menu opened for owner is
// shown.
func ContextMenuOpen(c *Context, owner ID) bool {
	st := State[contextMenuState](c, contextMenuKey, nil)
	return st.open && st.owner == owner
}

// close hides the menu; focus left on one of its items (or nowhere) goes
// back to the owner, focus a press moved elsewhere stays there.
func (st *contextMenuState) close(c *Context) {
	inMenu := c.focus == 0 || st.popup.root != 0 && c.focus == st.popup.root
	for _, id := range st.popup.ids {
		if id == c.focus {
			inMenu = true
			break
		}
	}
	if inMenu && st.owner != 0 {
		c.SetFocus(st.owner)
	}
	st.open, st.fresh = false, false
	st.items, st.onSelect, st.env, st.panels = nil, nil, nil, nil
	st.popup = navPopupState{}
	c.invalid = true
}

// outside reports a press (primary or secondary) this frame outside the
// menu panels.
func (st *contextMenuState) outside(c *Context) bool {
	if len(st.panels) == 0 {
		return false
	}
	if c.PressedOutside(st.panels...) {
		return true
	}
	if !c.ctxPressed {
		return false
	}
	for _, id := range st.panels {
		if r, ok := c.rects[id]; ok && r.Contains(c.ctxPos) {
			return false
		}
	}
	return true
}

func renderContextMenu(c *Context) {
	st := State[contextMenuState](c, contextMenuKey, nil)
	if !st.open {
		return
	}
	fresh := st.fresh
	st.fresh = false
	if !fresh && st.outside(c) || st.popup.escape(c) {
		st.close(c)
		return
	}
	Key(c, contextMenuKey, func() {
		c.withEnv(st.env, func() { st.build(c) })
	})
}

func (st *contextMenuState) build(c *Context) {
	th := c.ThemeFor(theme.CompDropdown)
	a := c.AnimFrom(hashID(contextMenuKey, st.seq), "open", 0, 1, th.DurationMid)
	pop := &navMenuPopup{c: c, th: th, tk: &th.Components.Dropdown, st: &st.popup, element: navEl(c), opacity: a, edge: true}
	at := st.pos
	pop.panel(st.items, 0, 0, 0, 0, &at)
	st.panels = pop.panels
	if pop.clicked != "" {
		key, fn := pop.clicked, st.onSelect
		st.close(c)
		if fn != nil {
			fn(key)
		}
	}
}
