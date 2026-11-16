package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/icon/filetype"
	"github.com/tangoui/tango/ui/layout"
	uitheme "github.com/tangoui/tango/ui/theme"
)

// The shell borrows the IDE layout; every control and visual token is Tango's
// existing Ant Design UI. Presentation state never owns document text.
type shellState struct {
	sidebar           bool
	tab               string
	settings, details bool
	palette           *uitheme.Theme
	themeName         string
}

var shellUI = shellState{sidebar: true, tab: "document"}

func (s *shellState) theme(name string) *uitheme.Theme {
	if s.palette == nil || s.themeName != name {
		s.palette = uitheme.AntDesign()
		if name == "vs-dark" || name == "hc-black" {
			s.palette = uitheme.AntDesign(uitheme.Dark())
		}
		if strings.HasPrefix(name, "hc-") {
			s.palette = uitheme.HighContrast(s.palette)
		}
		s.themeName = name
	}
	return s.palette
}

func renderShell(c *ui.Context) {
	th := c.Theme
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Icon(c, icon.Code).Size(th.FontSizeXL).Color(th.Primary.Base)
			if c.Size().W >= 760 {
				ui.Text(c, "Tango Editor").Strong()
			}
			switch key := ui.Menubar(c, shellMenus()).A11yLabel("Editor menu").Clicked(); key {
			case "":
			default:
				runShellAction(key)
				ui.Invalidate(c.Window())
			}
			ui.Spacer(c)
			shellIcon(c, icon.Search, "Command Palette", func() { runShellAction("palette") })
			shellIcon(c, icon.Setting, "Settings", func() { runShellAction("settings") })
		}, layout.H(th.Components.Menubar.Height+th.PaddingXXS), layout.Middle(), layout.Gap(th.PaddingXS), layout.PadVH(0, th.PaddingSM)).Bg(th.BgContainer)
		ui.Row(c, func() {
			ui.Column(c, func() {
				col := th.TextTertiary
				if shellUI.sidebar && c.Size().W >= 820 {
					col = th.Primary.Base
				}
				ui.Tooltip(c, "Toggle sidebar", func() {
					if ui.IconButton(c, icon.PanelLeft, "Toggle sidebar").Size(th.ControlHeight, th.FontSizeLG).Color(col).Clicked() {
						runShellAction("sidebar")
						ui.Invalidate(c.Window())
					}
				})
				shellIcon(c, icon.Search, "Find in document", func() { runShellAction("find") })
				ui.Spacer(c)
				shellIcon(c, icon.Setting, "Editor settings", func() { runShellAction("settings") })
			}, layout.W(th.ControlHeight+th.PaddingXS*2), layout.Pad(th.PaddingXS), layout.Gap(th.PaddingSM), layout.MinH(0)).Bg(th.BgContainer)
			if shellUI.sidebar && c.Size().W >= 820 {
				renderSidebar(c)
			}
			ui.Key(c, "document-workspace", func() {
				ui.Column(c, func() {
					label := documentDisplayName(documents.session.path)
					if documents.session.dirty() {
						label += " •"
					}
					ic := filetype.ForFlavor(th.Flavor).Icon(st.Model.URI(), false, false)
					ui.Tabs(c, &shellUI.tab, []ui.TabItem{{Key: "document", Label: label, Icon: ic}}).Size(ui.Small).NoMargin().TabBarStyle(layout.PadVH(0, th.PaddingSM)).Extra(func() {
						ui.Row(c, func() {
							ui.Disabled(c, !documents.session.dirty(), func() { shellIcon(c, icon.Save, "Save file", func() { postFileAction(func() { saveDocument() }) }) })
							shellIcon(c, icon.SplitPane, "Split editor", func() { runShellAction("split"); ui.Invalidate(c.Window()) })
						}, layout.Middle(), layout.Gap(th.PaddingXS))
					}).Show()
					if documents.session.path != "" && c.Size().W >= 820 {
						ui.Row(c, func() {
							ui.Text(c, filepath.Dir(documents.session.path)).Size(th.FontSizeSM).Secondary().NoWrap().Ellipsis().Layout(layout.Grow(1))
						}, layout.H(th.ControlHeightSM), layout.PadVH(0, th.PaddingSM), layout.Middle()).Bg(th.BgContainer)
					}
					renderEditorPanes(c)
				}, layout.Grow(1), layout.MinW(0), layout.MinH(0)).Bg(th.BgContainer).Clip()
			})
		}, layout.Grow(1), layout.MinW(0), layout.MinH(0))
		renderStatusBar(c)
	}, layout.Fill()).Bg(th.BgLayout)
	renderShellDialogs(c)
}

func shellIcon(c *ui.Context, ic *icon.Icon, label string, fn func()) {
	ui.Tooltip(c, label, func() {
		if ui.IconButton(c, ic, label).Size(c.Theme.ControlHeightSM, c.Theme.FontSizeLG).Clicked() {
			fn()
		}
	})
}

func shellMenus() []ui.MenubarMenu {
	themeItems := []ui.MenubarItem{}
	for _, option := range themeOptions {
		themeItems = append(themeItems, ui.MenubarItem{Key: "theme:" + option.Value, Label: option.Label})
	}
	return []ui.MenubarMenu{
		{Key: "file", Label: "File", Items: []ui.MenubarItem{
			{Key: "open", Label: "Open…", Icon: icon.FolderOpen, Shortcut: ui.ShortcutLabel("CmdOrCtrl+O")},
			{Key: "save", Label: "Save", Icon: icon.Save, Shortcut: ui.ShortcutLabel("CmdOrCtrl+S")},
			{Key: "save-as", Label: "Save As…", Shortcut: ui.ShortcutLabel("CmdOrCtrl+Shift+S")},
		}},
		{Key: "edit", Label: "Edit", Items: []ui.MenubarItem{
			{Key: "undo", Label: "Undo", Shortcut: ui.ShortcutLabel("CmdOrCtrl+Z")},
			{Key: "redo", Label: "Redo", Shortcut: ui.ShortcutLabel("CmdOrCtrl+Shift+Z")},
			{Divider: true}, {Key: "find", Label: "Find", Icon: icon.Search, Shortcut: ui.ShortcutLabel("CmdOrCtrl+F")},
			{Key: "replace", Label: "Replace", Shortcut: ui.ShortcutLabel("CmdOrCtrl+H")},
			{Divider: true}, {Key: "palette", Label: "Command Palette", Shortcut: ui.ShortcutLabel("CmdOrCtrl+Shift+P")},
		}},
		{Key: "view", Label: "View", Items: []ui.MenubarItem{
			{Key: "sidebar", Label: "Sidebar"},
			{Key: "split", Label: "Split editor", Icon: icon.SplitPane},
			{Key: "wrap", Label: "Word wrap"},
			{Divider: true}, {Key: "themes", Label: "Color theme", Children: themeItems},
			{Key: "settings", Label: "Editor settings…", Icon: icon.Setting},
		}},
		{Key: "help", Label: "Help", Items: []ui.MenubarItem{{Key: "details", Label: "Editor details…"}}},
	}
}

var themeOptions = []ui.Option{{Label: "Light", Value: "vs"}, {Label: "Dark", Value: "vs-dark"}, {Label: "High contrast dark", Value: "hc-black"}, {Label: "High contrast light", Value: "hc-light"}}

func runShellAction(key string) {
	switch key {
	case "open":
		postFileAction(openDocumentDialog)
	case "save":
		postFileAction(func() { saveDocument() })
	case "save-as":
		postFileAction(func() { saveAsDocument() })
	case "sidebar":
		shellUI.sidebar = !shellUI.sidebar
		restoreEditorFocus()
	case "split":
		documents.setSplit(!documents.split)
	case "wrap":
		setDocumentWrap(!documents.wrap)
	case "find", "replace":
		search.open = true
	case "palette":
		paletteTarget, paletteOpen = active, true
	case "settings":
		shellUI.settings = true
	case "details":
		shellUI.details = true
	case "undo", "redo":
		paletteTarget, pendingCommand = active, key
		restoreEditorFocus()
	default:
		if name, ok := strings.CutPrefix(key, "theme:"); ok {
			documents.setTheme(name)
		}
	}
}

func setDocumentWrap(wrap bool) {
	documents.wrap = wrap
	st.Options.WordWrap = editor.WrapOff
	if wrap {
		st.Options.WordWrap = editor.WrapOn
	}
	if second != nil {
		second.Options.WordWrap = st.Options.WordWrap
	}
	restoreEditorFocus()
}

func renderSidebar(c *ui.Context) {
	th := c.Theme
	width := th.PaddingXL * 7
	ui.Column(c, func() {
		ui.Text(c, "OPEN DOCUMENT").Size(th.FontSizeSM).Secondary().Strong()
		name := documentDisplayName(documents.session.path)
		ic := filetype.ForFlavor(th.Flavor).Icon(st.Model.URI(), false, false)
		if ui.Button(c, name).Text().Icon(ic).Block().A11yLabel("Focus active document").Clicked() {
			restoreEditorFocus()
		}
		if documents.session.dirty() {
			ui.Text(c, "Unsaved changes").Size(th.FontSizeSM).Color(th.Warning.Text)
		} else {
			ui.Text(c, "All changes saved").Size(th.FontSizeSM).Secondary()
		}
		ui.Div(c, layout.H(th.PaddingXS))
		ui.Text(c, "EDITOR").Size(th.FontSizeSM).Secondary().Strong()
		renderEditorOptions(c, width-th.PaddingSM*2)
		ui.Spacer(c)
		ui.Text(c, "SHORTCUTS").Size(th.FontSizeSM).Secondary().Strong()
		for _, shortcut := range []struct{ label, accel string }{{"Open file", "CmdOrCtrl+O"}, {"Save file", "CmdOrCtrl+S"}, {"Find", "CmdOrCtrl+F"}} {
			ui.Row(c, func() {
				ui.Text(c, shortcut.label).Size(th.FontSizeSM).Secondary()
				ui.Spacer(c)
				ui.Text(c, ui.ShortcutLabel(shortcut.accel)).Size(th.FontSizeSM).Tertiary()
			}, layout.Middle())
		}
	}, layout.W(width), layout.Pad(th.PaddingSM), layout.Gap(th.PaddingXS), layout.MinH(0)).Bg(th.BgLayout).Clip()
}

func renderEditorOptions(c *ui.Context, width float32) {
	ui.Text(c, "Language").Size(c.Theme.FontSizeSM).Secondary()
	language := st.Model.Language()
	if ui.Select(c, &language, languageOptions).Searchable().Width(width).A11yLabel("Document language").Changed() {
		st.Model.SetLanguage(language)
		hlDone = false
	}
	ui.Text(c, "Theme").Size(c.Theme.FontSizeSM).Secondary()
	name := documents.theme.BaseName()
	if ui.Select(c, &name, themeOptions).Width(width).A11yLabel("Editor theme").Changed() {
		documents.setTheme(name)
		ui.Invalidate(c.Window())
	}
	split := documents.split
	if ui.Checkbox(c, "Split view", &split).Changed() {
		documents.setSplit(split)
		ui.Invalidate(c.Window())
	}
	wrap := documents.wrap
	if ui.Checkbox(c, "Word wrap", &wrap).Changed() {
		setDocumentWrap(wrap)
		ui.Invalidate(c.Window())
	}
}

func renderStatusBar(c *ui.Context) {
	th := c.Theme
	target := active
	if target == nil {
		target = st
	}
	snap := target.Snapshot()
	pos := snap.PosOf(target.Primary().Head)
	ui.Row(c, func() {
		if c.Size().W >= 820 {
			state := "Saved"
			if documents.session.path == "" {
				state = "Untitled"
			} else if documents.session.dirty() {
				state = "Modified"
			}
			ui.Text(c, state).Size(th.FontSizeSM).Secondary()
			ui.Text(c, fmt.Sprintf("%d lines", snap.LineCount())).Size(th.FontSizeSM).Tertiary()
		}
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("Ln %d, Col %d", pos.Line+1, snap.UTF16Col(pos.Line, pos.Col)+1)).Size(th.FontSizeSM).Secondary().NoWrap()
		if c.Size().W >= 640 {
			ui.Text(c, indentStatus(target.Model)).Size(th.FontSizeSM).Secondary()
		}
		eol := "LF"
		if snap.EOL() == "\r\n" {
			eol = "CRLF"
		}
		ui.Text(c, eol).Size(th.FontSizeSM).Secondary()
		if ui.Button(c, target.Model.Language()).Text().Small().A11yLabel("Change document language").Clicked() {
			shellUI.settings = true
		}
	}, layout.H(th.ControlHeightSM+th.PaddingXXS), layout.Middle(), layout.Gap(th.PaddingSM), layout.PadVH(0, th.PaddingSM)).Bg(th.BgContainer)
}

func renderShellDialogs(c *ui.Context) {
	settingsWasOpen := shellUI.settings
	ui.Modal(c, &shellUI.settings, "Editor settings").Width(min(380, c.Size().W-32)).NoFooter().Body(func() { renderEditorOptions(c, min(332, c.Size().W-80)) })
	ui.Modal(c, &shellUI.details, "Editor details").Width(min(500, c.Size().W-32)).NoFooter().Body(func() { ui.Text(c, status()) })
	if settingsWasOpen && !shellUI.settings {
		restoreEditorFocus()
		ui.Invalidate(c.Window())
	}
}

func renderEditorPanes(c *ui.Context) {
	restoring, focusTarget := !focused, active
	ui.Row(c, func() {
		show := func(target *editor.State) {
			vw := editor.View(c, target)
			if !focused && (target == focusTarget || focusTarget == nil) {
				c.SetFocus(vw.Box().ID())
				focused = true
			}
			if vw.Focused() && (!restoring || target == focusTarget || focusTarget == nil) {
				active = target
			}
			if pendingCommand != "" && target == paletteTarget {
				c.SetFocus(vw.Box().ID())
				if vw.Focused() {
					id := pendingCommand
					pendingCommand = ""
					if !target.Execute(id, nil) {
						restoreEditorFocus()
					}
				} else {
					ui.Invalidate(c.Window())
				}
			}
		}
		show(st)
		if second != nil {
			show(second)
		}
	}, layout.Grow(1), layout.MinW(0), layout.MinH(0))
}
