package main

import (
	"fmt"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/terminal"
	"github.com/tangoui/tango/ui/theme"
)

type terminalApp struct {
	workspace    workspace
	dark         bool
	fontSize     float32
	focusPending bool
	err          string
	palette      *theme.Theme
	invalidate   func()
}

func (a *terminalApp) theme() *theme.Theme {
	if a.palette == nil || a.palette.Dark != a.dark {
		if a.dark {
			a.palette = theme.AntDesign(theme.Dark())
		} else {
			a.palette = theme.AntDesign()
		}
	}
	return a.palette
}

func (a *terminalApp) action(fn func() error) {
	if err := fn(); err != nil {
		a.err = err.Error()
	} else {
		a.err = ""
		a.focusPending = true
	}
	if a.invalidate != nil {
		a.invalidate()
	}
}

func (a *terminalApp) changed(fn func()) {
	fn()
	a.focusPending = true
	if a.invalidate != nil {
		a.invalidate()
	}
}

func (a *terminalApp) page(c *ui.Context) {
	// A ConfigProvider owns the page theme. Changing Runner.SetTheme inside
	// a build would be undone when that build restores its root environment.
	ui.ConfigProvider(c).Theme(a.theme()).Body(func() { a.content(c) })
}

func (a *terminalApp) content(c *ui.Context) {
	in := c.SafeArea()
	in.Bottom = max(in.Bottom, c.KeyboardInset().Bottom)
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Text(c, "Tango Terminal").Strong()
			ui.Spacer(c)
			if ui.Button(c, "New tab").Small().Disabled(len(a.workspace.tabs) >= maxTabs).Clicked() {
				a.action(a.workspace.newTab)
			}
			tab := a.workspace.activeTab()
			canSplit := tab != nil && len(tab.panes()) < maxPanes
			if ui.Button(c, "Split right").Small().Disabled(!canSplit).Clicked() {
				a.action(func() error { return a.workspace.split(splitRight) })
			}
			if ui.Button(c, "Split down").Small().Disabled(!canSplit).Clicked() {
				a.action(func() error { return a.workspace.split(splitDown) })
			}
			if ui.Button(c, "Close pane").Small().Disabled(tab == nil).Clicked() {
				a.changed(a.workspace.closePane)
			}
			themeLabel := "Light theme"
			if !a.dark {
				themeLabel = "Dark theme"
			}
			if ui.Button(c, themeLabel).Small().Clicked() {
				a.dark = !a.dark
				a.focusPending = true
				c.WakeAt(c.Now())
			}
		}, layout.Pad(8), layout.Gap(8), layout.WrapItems(), layout.AlignItems(layout.AlignCenter))
		if a.err != "" {
			ui.Row(c, func() {
				ui.Text(c, a.err).Color(c.Theme.Error.Base).Layout(layout.Grow(1))
				if ui.Button(c, "Dismiss error").Small().Clicked() {
					a.err = ""
				}
			}, layout.PadVH(4, 8), layout.Gap(8))
		}
		items := make([]ui.TabItem, 0, len(a.workspace.tabs))
		for _, tab := range a.workspace.tabs {
			label := tab.name
			if count := len(tab.panes()); count > 1 {
				label += fmt.Sprintf(" +%d", count-1)
			}
			items = append(items, ui.TabItem{Key: tab.id, Label: label})
		}
		if len(items) > 0 {
			tabs := ui.Tabs(c, &a.workspace.activeTabID, items).Closable().Show()
			if tabs.Changed() {
				a.focusPending = true
			}
			if id := tabs.Closed(); id != "" {
				a.changed(func() { a.workspace.closeTab(id) })
			}
		}
		if tab := a.workspace.activeTab(); tab != nil {
			// Views cache their last dimensions. Moving a session into/out of
			// split ancestors must not resurrect an old view with stale sizes.
			ui.Key(c, fmt.Sprintf("%s-layout-%d", tab.id, tab.layoutVersion), func() { a.renderNode(c, tab, tab.root) })
		} else {
			ui.Column(c, func() {
				ui.Text(c, "No terminals open").Strong()
				ui.Text(c, "Open a local shell to start. Each tab and split owns a separate session.").Secondary()
				if ui.Button(c, "New terminal").Primary().Clicked() {
					a.action(a.workspace.newTab)
				}
			}, layout.Fill(), layout.Gap(12), layout.AlignItems(layout.AlignCenter), layout.JustifyContent(layout.JustifyCenter))
		}
		ui.Row(c, func() {
			if pane := a.workspace.activeSession(); pane != nil {
				cols, rows, _ := pane.info()
				ui.Text(c, fmt.Sprintf("%s · PID %d · %d × %d", pane.name, pane.pid, cols, rows)).Secondary()
			}
			ui.Spacer(c)
			ui.Text(c, "New tab: Cmd+T / Ctrl+Shift+T · Next pane: Ctrl+Shift+Right").Tertiary()
		}, layout.PadVH(5, 8), layout.Gap(8), layout.WrapItems())
	}, layout.Fill(), layout.Padding(in)).Bg(c.Theme.BgLayout)
}

func (s *session) info() (cols, rows int, title string) {
	if s.term == nil {
		return 80, 24, ""
	}
	v := s.term.Lock()
	cols, rows = v.Screen().Size()
	title = v.Title()
	s.term.Unlock()
	return
}

func (a *terminalApp) renderNode(c *ui.Context, tab *terminalTab, n *splitNode) {
	if n.pane != nil {
		ui.Key(c, n.pane.id, func() { a.renderPane(c, tab, n.pane) })
		return
	}
	ui.Key(c, n.id, func() {
		split := ui.Splitter(c).Style(layout.Fill(), layout.MinW(0), layout.MinH(0))
		if n.axis == splitDown {
			split.Vertical()
		}
		split.Body(func() {
			ui.SplitterPanel(c).DefaultPercent(n.ratio).Min(100).Body(func() { a.renderNode(c, tab, n.first) })
			ui.SplitterPanel(c).DefaultPercent(1 - n.ratio).Min(100).Body(func() { a.renderNode(c, tab, n.second) })
		})
		if split.Changed() {
			// Hidden widget state can be evicted; the tab still owns its layout.
			if sizes := split.Sizes(); len(sizes) == 2 {
				n.ratio = sizes[0]
			}
		}
	})
}

func (a *terminalApp) renderPane(c *ui.Context, tab *terminalTab, pane *session) {
	active := pane.id == tab.activePaneID
	border := c.Theme.BorderSecondary
	if active {
		border = c.Theme.Primary.Base
	}
	ui.Div(c, layout.Fill(), layout.MinW(0), layout.MinH(0)).Border(1, border).Clip().Body(func() {
		_, _, title := pane.info()
		if title == "" {
			title = pane.name
		}
		status := "running"
		if exited, _ := pane.term.Exited(); exited {
			status = "exited"
		}
		ui.Row(c, func() {
			ui.Text(c, title).Layout(layout.Grow(1)).Strong()
			ui.Text(c, status).Secondary()
		}, layout.PadVH(4, 8), layout.Gap(8))
		var view *terminal.ViewW
		ui.Div(c, layout.Fill(), layout.MinW(0), layout.MinH(0), layout.Padding(geom.All(4))).Body(func() {
			view = terminal.View(c, pane.term).FontSize(a.fontSize).Layout(layout.Fill())
			view.Box().Semantics(a11y.RoleTextbox, "Terminal "+pane.name)
		})
		id := view.Box().ID()
		if !a.focusPending && c.Focused() == id {
			tab.activePaneID = pane.id
		}
		// Set focus only after View collected this frame's events. A queued key
		// from the old pane must never be delivered to a newly selected tab.
		if a.focusPending && pane.id == tab.activePaneID {
			c.SetFocus(id)
			a.focusPending = false
		}
	})
}
