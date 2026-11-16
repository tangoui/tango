package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
	uitheme "github.com/tangoui/tango/ui/theme"
)

func shellDriver(t *testing.T, width float32) *ui.Driver {
	t.Helper()
	prev := text.UseSystemEngine(false)
	shellUI = shellState{sidebar: true, tab: "document"}
	search = searchPanel{}
	setupDocument("main.go", "package main\n\nfunc main() {\n    println(\"Hello, Tango\")\n}\n", theme.Base("vs-dark"), false, false)
	d := ui.NewDriver(page, ui.DriverOptions{Width: width, Height: 640, Accessibility: true})
	t.Cleanup(func() {
		background.Close()
		st.Close()
		if second != nil {
			second.Close()
		}
		shellUI = shellState{sidebar: true, tab: "document"}
		search = searchPanel{}
		text.UseSystemEngine(prev)
	})
	d.Frame()
	return d
}

func TestShellRealDocumentTabTracksSaveAsAndDirty(t *testing.T) {
	d := shellDriver(t, 1200)
	if _, ok := d.A11yFind(a11y.RoleTab, "main.go"); !ok {
		t.Fatal("active file tab missing")
	}
	st.Insert("// edit\n")
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleTab, "main.go •"); !ok {
		t.Fatal("dirty marker missing")
	}
	if err := documents.saveAs(filepath.Join(t.TempDir(), "saved.go")); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleTab, "saved.go"); !ok {
		t.Fatal("tab did not follow Save As")
	}
}

func TestShellMenuTogglesWrapAndSidebar(t *testing.T) {
	d := shellDriver(t, 1200)
	if _, ok := d.A11yFind(a11y.RoleMenuBar, "Editor menu"); !ok {
		t.Fatal("compact menu missing")
	}
	if err := d.ClickLabel("View"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if err := d.ClickLabel("Word wrap"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if !documents.wrap {
		t.Fatal("View menu did not enable wrapping")
	}
	if err := d.ClickLabel("Toggle sidebar"); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if shellUI.sidebar {
		t.Fatal("sidebar toggle did not collapse")
	}
}

func TestShellKeepsCodeViewportAtNarrowWidths(t *testing.T) {
	for _, width := range []float32{480, 1200} {
		t.Run(fmt.Sprintf("width-%.0f", width), func(t *testing.T) {
			d := shellDriver(t, width)
			n, ok := d.A11yFind(a11y.RoleTextbox, "main.go")
			if !ok {
				t.Fatal("code editor missing")
			}
			r := n.Bounds
			if r.W < width*0.55 || r.H < 400 || r.X < 0 || r.X+r.W > width+1 {
				t.Fatalf("unusable editor viewport at width %.0f: %+v", width, r)
			}
			if _, ok := d.Find("Settings"); !ok {
				t.Fatal("settings unavailable")
			}
		})
	}
}

func TestShellThemePersistsDuringBuild(t *testing.T) {
	d := shellDriver(t, 1200)
	if th := shellUI.theme("vs-dark"); th.Flavor != uitheme.Ant || !th.Dark {
		t.Fatal("shell lost existing Ant dark theme")
	}
	runShellAction("theme:vs")
	d.Frame()
	d.Frame()
	if documents.theme.BaseName() != "vs" || shellUI.theme("vs").Dark {
		t.Fatal("theme change did not persist")
	}
	if _, ok := d.A11yFind(a11y.RoleTab, "main.go"); !ok {
		t.Fatal("theme change replaced tab")
	}
}

func TestShellSplitMenuPreservesTypingTarget(t *testing.T) {
	d := shellDriver(t, 1200)
	runShellAction("split")
	d.Frame()
	if second == nil {
		t.Fatal("split action missing")
	}
	active = second
	restoreEditorFocus()
	d.Frame()
	d.Frame()
	d.Key(input.KeyA, 0)
	if active != second {
		t.Fatal("focus did not return to second pane")
	}
}

func TestUntitledSampleDoesNotClaimToBeSaved(t *testing.T) {
	d := shellDriver(t, 1200)
	documents.session.path = ""
	d.Frame()
	if _, ok := d.FindText("Not saved to disk"); !ok {
		t.Fatal("untitled document must not claim its content is saved")
	}
}

func TestNarrowShellKeepsCommandPaletteOnscreen(t *testing.T) {
	d := shellDriver(t, 480)
	runShellAction("palette")
	d.Frame()
	d.Frame()
	n, ok := d.A11yFind(a11y.RoleDialog, "Editor commands")
	if !ok {
		t.Fatal("command palette missing")
	}
	r := n.Bounds
	if r.X < 0 || r.W > 480 || r.X+r.W > 480 {
		t.Fatalf("palette extends beyond window: %+v", r)
	}
	paletteOpen = false
}
