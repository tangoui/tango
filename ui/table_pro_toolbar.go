package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
)

// ListToolBarW composes the title, subtitle, tooltip, search, actions,
// settings, filter, tabs and header menu of a list. ProTable uses this
// same public component for its toolbar, passing its existing settings.
type ListToolBarW struct {
	_                         [0]func()
	c                         *Context
	title, sub, tip           string
	search                    *string
	searchDefault             bool
	onSearch                  func(string)
	actions, settings, filter func()
	multiple                  bool
	tabs, menu                []TabItem
	tabKey, menuKey           *string
	menuType                  string
	id                        ID
	// ProTable retains its existing geometry while sharing composition.
	minH float32
}

type listToolBarState struct{ search, tab, menu string }

// ListToolBar starts a list toolbar; chain options, then Show.
// ConfigProvider.Defaults keys: "title", "subTitle", "tooltip" (strings),
// "search" (bool, default false), "multipleLine" (bool, default false).
func ListToolBar(c *Context) *ListToolBarW {
	return &ListToolBarW{c: c, title: Prop(c, "ListToolBar", "title", ""), sub: Prop(c, "ListToolBar", "subTitle", ""), tip: Prop(c, "ListToolBar", "tooltip", ""), searchDefault: Prop(c, "ListToolBar", "search", false), multiple: Prop(c, "ListToolBar", "multipleLine", false), menuType: "inline"}
}

// Title sets the heading.
func (w *ListToolBarW) Title(s string) *ListToolBarW { w.title = s; return w }

// SubTitle sets the secondary text next to the heading.
func (w *ListToolBarW) SubTitle(s string) *ListToolBarW { w.sub = s; return w }

// Tooltip sets the heading information tooltip.
func (w *ListToolBarW) Tooltip(s string) *ListToolBarW { w.tip = s; return w }

// Search binds the search input; nil hides it.
func (w *ListToolBarW) Search(s *string) *ListToolBarW {
	w.search = s
	w.searchDefault = false
	return w
}

// OnSearch receives a submitted keyword (Enter or search button).
func (w *ListToolBarW) OnSearch(fn func(string)) *ListToolBarW { w.onSearch = fn; return w }

// Actions builds the right-hand actions.
func (w *ListToolBarW) Actions(fn func()) *ListToolBarW { w.actions = fn; return w }

// Settings builds the right-hand settings (use named IconButtons).
func (w *ListToolBarW) Settings(fn func()) *ListToolBarW { w.settings = fn; return w }

// Filter builds the filter area (for example LightFilter).
func (w *ListToolBarW) Filter(fn func()) *ListToolBarW { w.filter = fn; return w }

// MultipleLine places filters and tabs on a separate line.
func (w *ListToolBarW) MultipleLine(on bool) *ListToolBarW { w.multiple = on; return w }

// Tabs sets the second-line tabs, shown only with MultipleLine(true).
// A nil active pointer keeps the active key internally.
func (w *ListToolBarW) Tabs(active *string, items []TabItem) *ListToolBarW {
	w.tabKey, w.tabs = active, items
	return w
}

// Menu sets the header menu, replacing the heading; kind is "inline"
// (default), "tab" or "dropdown". A nil active pointer is uncontrolled.
func (w *ListToolBarW) Menu(kind string, active *string, items []TabItem) *ListToolBarW {
	w.menuType, w.menuKey, w.menu = kind, active, items
	return w
}

// ID returns the toolbar ID after Show.
func (w *ListToolBarW) ID() ID { return w.id }

// Show builds the toolbar.
func (w *ListToolBarW) Show() *ListToolBarW {
	c, th := w.c, w.c.Theme
	root := c.box("list-toolbar", layout.Column, nil)
	w.id = root.n.id
	name := w.title
	if name == "" {
		name = c.Locale().A11y.MoreActions
	}
	root.Semantics(a11y.RoleToolbar, name)
	st := State[listToolBarState](c, w.id, nil)
	if w.searchDefault {
		w.search = &st.search
	}
	root.Body(func() {
		opts := []layout.Option{layout.Middle(), layout.Gap(8)}
		if w.minH > 0 {
			opts = append(opts, layout.MinH(w.minH))
		} else {
			opts = append(opts, layout.PadVH(16, 8))
		}
		Row(c, func() {
			if len(w.menu) > 0 {
				w.headerMenu(st)
			} else {
				if w.title != "" {
					Text(c, w.title).Size(th.FontSizeLG).Weight(text.Semibold).Color(th.TextHeading)
				}
				if w.sub != "" {
					Text(c, w.sub).Color(th.TextSecondary)
				}
				if w.tip != "" {
					Tooltip(c, w.tip, func() { IconButton(c, icon.Info, w.tip) })
				}
			}
			// Without a title/menu, search belongs to the left.
			hasTitle := w.title != "" || w.sub != "" || w.tip != "" || len(w.menu) > 0
			if !hasTitle {
				w.searchView()
			}
			Spacer(c)
			if !w.multiple && w.filter != nil {
				Row(c, w.filter, layout.Gap(8), layout.Middle())
			}
			if hasTitle {
				w.searchView()
			}
			if w.actions != nil {
				Row(c, w.actions, layout.Gap(8), layout.Middle())
			}
			if w.settings != nil {
				Row(c, w.settings, layout.Gap(4), layout.Middle())
			}
		}, opts...)
		if w.multiple {
			if len(w.tabs) > 0 {
				active := w.tabKey
				if active == nil {
					active = &st.tab
				}
				Tabs(c, active, w.tabs).Extra(w.filter).Show()
			} else if w.filter != nil {
				Row(c, w.filter, layout.Gap(8), layout.Middle())
			}
		}
	})
	return w
}
func (w *ListToolBarW) searchView() {
	if w.search == nil {
		return
	}
	c := w.c
	if Search(c, w.search).Placeholder(c.Locale().Placeholder).A11yLabel(c.Locale().Search).Width(200).Searched() && w.onSearch != nil {
		w.onSearch(*w.search)
	}
}
func (w *ListToolBarW) headerMenu(st *listToolBarState) {
	c := w.c
	active := w.menuKey
	if active == nil {
		active = &st.menu
	}
	if *active == "" {
		for _, it := range w.menu {
			if !it.Disabled {
				*active = it.Key
				break
			}
		}
	}
	if w.menuType != "dropdown" {
		Tabs(c, active, w.menu).Show()
		return
	}
	items := make([]MenuItem, 0, len(w.menu))
	label := ""
	for _, it := range w.menu {
		items = append(items, MenuItem{Key: it.Key, Label: it.Label, Disabled: it.Disabled})
		if it.Key == *active {
			label = it.Label
		}
	}
	if label == "" {
		label = w.menu[0].Label
	}
	TableDropdown(c, items).Button(label).Trigger(TriggerClick).OnSelect(func(k string) { *active = k; c.invalid = true }).Show()
}
