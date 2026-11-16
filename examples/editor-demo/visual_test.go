package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/text"
)

// Optional visual QA uses the real example page and the platform text engine.
// Ordinary tests do not create screenshot files.
func TestShellVisualQA(t *testing.T) {
	dir := os.Getenv("TANGO_EDITOR_SCREENSHOTS")
	if dir == "" {
		t.Skip("set TANGO_EDITOR_SCREENSHOTS to render the editor states")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, theme          string
		width                float32
		split, search, dirty bool
	}{
		{"dark", "vs-dark", 1200, false, false, false},
		{"light", "vs", 1200, false, false, false},
		{"high-contrast", "hc-black", 1200, false, false, false},
		{"narrow", "vs-dark", 480, false, false, false},
		{"split", "vs-dark", 1200, true, false, false},
		{"search", "vs-dark", 1200, false, true, false},
		{"dirty", "vs-dark", 1200, false, false, true},
		{"narrow-search", "vs-dark", 480, false, true, false},
		{"narrow-sidebar", "vs-dark", 480, false, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			old := text.UseSystemEngine(true)
			defer text.UseSystemEngine(old)
			shellUI = shellState{sidebar: true, tab: "document"}
			search = searchPanel{}
			setupDocument("welcome.ts", welcomeSample, theme.Base(tt.theme), tt.split, false)
			documents.session.path = ""
			if err := shellUI.explorer.openFolder("."); err != nil {
				t.Fatal(err)
			}
			defer func() {
				background.Close()
				st.Close()
				if second != nil {
					second.Close()
				}
				shellUI = shellState{sidebar: true, tab: "document"}
				search = searchPanel{}
			}()
			if tt.name == "narrow-sidebar" {
				shellUI.sidebarOverlay = true
			}
			if tt.dirty {
				st.Insert("// My editor\n")
			}
			if tt.search {
				showSearch(true)
				search.query = "Task"
				search.replacement = "Item"
			}
			d := ui.NewDriver(page, ui.DriverOptions{Width: tt.width, Height: 800, Accessibility: true})
			d.Frame()
			d.Frame()
			if err := d.SavePNG(filepath.Join(dir, tt.name+".png")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
