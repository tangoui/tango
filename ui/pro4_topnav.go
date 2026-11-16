package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// TopNavHeaderWideWidth is Pro v3's Fixed top navigation content width.
const TopNavHeaderWideWidth = 1152

// TopNavHeaderW exposes ProLayout's horizontal header as a reusable entry.
// The desktop top menu is always expanded; overflow is handled by Menu,
// not by an additional sidebar collapse button (matching Pro v3).
type TopNavHeaderW struct {
	_             [0]func()
	c             *Context
	cfg           ProLayoutConfig
	pl            *proLayout
	onHeaderClick func()
}

// TopNavHeader starts a top navigation header; chain options, then Show.
// ConfigProvider.Defaults("TopNavHeader", ...): "title", "contentWidth",
// "userName" (string). Navigation selection is owned by selected.
func TopNavHeader(c *Context, menu []MenuItem, selected *string) *TopNavHeaderW {
	return &TopNavHeaderW{c: c, cfg: ProLayoutConfig{Layout: "top", Menu: menu, Selected: selected,
		Title: Prop(c, "TopNavHeader", "title", ""), UserName: Prop(c, "TopNavHeader", "userName", ""),
		ContentWidth: Prop(c, "TopNavHeader", "contentWidth", "Fluid"), LightSider: true}}
}

// Title sets the logo area's title.
func (w *TopNavHeaderW) Title(s string) *TopNavHeaderW { w.cfg.Title = s; return w }

// Logo replaces the default logo mark.
func (w *TopNavHeaderW) Logo(ic *icon.Icon) *TopNavHeaderW { w.cfg.Logo = ic; return w }

// UserName adds the user avatar and name in the trailing action area.
func (w *TopNavHeaderW) UserName(s string) *TopNavHeaderW { w.cfg.UserName = s; return w }

// Actions builds the trailing actions before the avatar.
func (w *TopNavHeaderW) Actions(fn func()) *TopNavHeaderW { w.cfg.Actions = fn; return w }

// ContentWidth selects Fluid or Fixed (1152px, centred).
func (w *TopNavHeaderW) ContentWidth(s string) *TopNavHeaderW { w.cfg.ContentWidth = s; return w }

// OnMenuHeaderClick makes the logo/title area a keyboard-accessible button.
func (w *TopNavHeaderW) OnMenuHeaderClick(fn func()) *TopNavHeaderW { w.onHeaderClick = fn; return w }
func (w *TopNavHeaderW) fromLayout(pl *proLayout) *TopNavHeaderW    { w.pl = pl; w.cfg = pl.cfg; return w }

// Show builds the header using ProLayout's existing header implementation.
func (w *TopNavHeaderW) Show() *TopNavHeaderW {
	c := w.c
	pl := w.pl
	if pl == nil {
		pl = &proLayout{c: c, th: c.ThemeFor(theme.CompLayout), cfg: w.cfg}
	}
	// Use a copy so a standalone callback does not mutate the enclosing shell.
	cp := *pl
	if w.onHeaderClick != nil {
		original := cp.cfg.MenuHeaderRender
		cp.cfg.MenuHeaderRender = func(def func(), collapsed bool) {
			b := Row(c, func() {
				if original != nil {
					original(def, collapsed)
				} else {
					def()
				}
			})
			b.Semantics(a11y.RoleButton, cp.cfg.Title)
			if b.Focusable(0).Clicked {
				w.onHeaderClick()
				c.Invalidate()
			}
		}
	}
	build := func() { cp.header(true).Semantics(a11y.RoleNavigation, c.Locale().A11y.Navigation) }
	if w.cfg.ContentWidth == "Fixed" {
		Row(c, func() {
			Column(c, build, layout.Grow(1), layout.MinW(0), layout.MaxW(c.ThemeFor(theme.CompLayout).Components.Layout.ProFixedWidth))
		}, layout.Center())
	} else {
		build()
	}
	return w
}
