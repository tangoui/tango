// Command dock-demo is an IDE-style workbench built with ui.Dock in a real
// window.
//
//	go run ./examples/dock-demo
//
// Drag tabs between groups, onto the edges of a group to split it and onto
// its middle to merge; drag the bars between groups; maximize a group from
// its header. Drag a tab out of the window and release it there: it opens
// in a window of its own (ui.DockW.OnTearOff), whose "dock back" button
// returns it. The layout is saved as JSON in the user config directory and
// restored on the next start (ui.ParseDockLayout).
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

var (
	app     *tango.App
	mainWin *tango.Surface
	dock    *ui.DockLayout
	popouts = map[string]*tango.Surface{} // torn-off tabs by key
	notes   = map[string]string{}         // per-tab text, to show state survives moves
)

var tabs = map[string]ui.TabItem{
	"main.go":   {Label: "main.go", Icon: icon.Code},
	"dock.go":   {Label: "dock.go", Icon: icon.Code},
	"README.md": {Label: "README.md", Icon: icon.FileText},
	"terminal":  {Label: "终端", Icon: icon.Terminal, NoClose: true},
	"problems":  {Label: "问题", Icon: icon.Warning},
	"outline":   {Label: "大纲", Icon: icon.Appstore},
}

func layoutFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "tango-dock-demo", "layout.json")
}

func loadLayout() *ui.DockLayout {
	if data, err := os.ReadFile(layoutFile()); err == nil {
		if l, err := ui.ParseDockLayout(data); err == nil {
			return l
		}
	}
	l := ui.NewDockLayout("main.go", "dock.go", "README.md", "terminal", "problems", "outline")
	l.SplitTab("terminal", "g1", ui.DockBottom)
	l.MoveTab("problems", "g2", -1)
	l.SplitTab("outline", "g1", ui.DockRight)
	l.Root.Sizes = []float64{0.7, 0.3}
	l.Activate("main.go")
	return l
}

func saveLayout() {
	data, err := json.MarshalIndent(dock, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(layoutFile()), 0o755)
	_ = os.WriteFile(layoutFile(), data, 0o644)
}

// content builds a tab: a title and a text area whose text lives in notes
// (the text field's own state, such as the caret, follows the tab when it
// moves to another group).
func content(c *ui.Context, key string) {
	ui.Column(c, func() {
		ui.Text(c, tabs[key].Label).Strong()
		ui.Text(c, "拖动标签页到其它组的边缘拆分、中间合并，或拖出窗口撕出成独立窗口。").Tertiary()
		v := notes[key]
		if ui.TextArea(c, &v).Rows(8).Changed() {
			notes[key] = v
		}
	}, layout.Pad(12), layout.Gap(8), layout.Grow(1))
}

func page(c *ui.Context) {
	d := ui.Dock(c, dock).
		Tab(func(key string) ui.TabItem { return tabs[key] }).
		Content(func(key string) { content(c, key) }).
		OnTearOff(popOut).
		Style(layout.Grow(1)).
		Show()
	if d.Changed() {
		saveLayout()
	}
}

// popOut opens a torn-off tab in a window of its own, its title bar under
// the pointer: the release point is in the main window's coordinates, so
// add the main window's position.
func popOut(key string, pos geom.Point) {
	b := mainWin.Bounds()
	x, y := b.X+int(pos.X)-60, b.Y+int(pos.Y)-14
	app.Post(func() { // after this frame: opening a window runs callbacks
		_, s, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: tabs[key].Label, Width: 560, Height: 420, Resizable: true, X: x, Y: y,
		}}, func(c *ui.Context) { popoutPage(c, key) }, ui.RunnerOptions{})
		if err != nil {
			log.Printf("dock-demo: %v", err)
			dockBack(key)
			return
		}
		popouts[key] = s
		s.OnClosed(func() {
			if popouts[key] == s {
				delete(popouts, key)
				dockBack(key)
			}
		})
	})
}

func popoutPage(c *ui.Context, key string) {
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Text(c, tabs[key].Label).Strong()
			ui.Spacer(c)
			if ui.Button(c, "停靠回主窗口").Small().Clicked() {
				if s := popouts[key]; s != nil {
					delete(popouts, key)
					s.Close()
				}
				dockBack(key)
			}
		}, layout.Middle(), layout.Pad(8))
		content(c, key)
	}, layout.Fill())
}

// dockBack returns a torn-off tab to the focused group of the main window.
func dockBack(key string) {
	app.Post(func() {
		dock.AddTab(key, "")
		saveLayout()
		if mainWin != nil {
			mainWin.Focus()
		}
		if r := mainRunner; r != nil {
			r.Invalidate()
		}
	})
}

var mainRunner *ui.Runner

func main() {
	dock = loadLayout()
	// Tabs torn off in an earlier run are back in the layout (or lost):
	// make sure every known tab is somewhere.
	for k := range tabs {
		if dock.GroupOf(k) == nil {
			dock.AddTab(k, "")
		}
	}
	quit := true
	app = tango.New(tango.Options{Name: "Tango Dock", ID: "dev.tango.dockdemo", QuitOnLastClose: &quit})
	app.OnReady(func() {
		r, s, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "Tango Dock", Width: 1100, Height: 720, MinWidth: 640, MinHeight: 420,
			Resizable: true, Centered: true,
		}}, func(c *ui.Context) {
			ui.Column(c, func() { page(c) }, layout.Fill())
		}, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		mainWin, mainRunner = s, r
		s.OnClosed(app.Quit)
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
