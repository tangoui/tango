package main

import (
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/text"
)

func TestDemoSplitSharedModel(t *testing.T) {
	prev := text.UseSystemEngine(false)
	defer text.UseSystemEngine(prev)
	setupDocument("sample.ts", "const value = 1;\n", theme.Base("vs-dark"), true, true)
	defer st.Close()
	defer background.Close()
	defer second.Close()
	if st.Model != second.Model || st == second {
		t.Fatal("split must use distinct views on shared model")
	}
	if st.Options.WordWrap != editor.WrapOn || second.Options.WordWrap != editor.WrapOn {
		t.Fatal("-wrap not applied to both views")
	}
	d := ui.NewDriver(page, ui.DriverOptions{Width: 1000, Height: 600})
	d.Frame()
	st.Execute("type", map[string]any{"text": "x"})
	d.Frame()
	if second.Snapshot().String() != st.Snapshot().String() {
		t.Fatal("split not synchronized")
	}
	st.Model.SetLanguage("go")
	if second.Model.Language() != "go" {
		t.Fatal("language switch not shared")
	}
	if !st.Execute("editor.action.quickCommand", nil) || !paletteOpen || paletteTarget != st {
		t.Fatal("palette action unavailable")
	}
	paletteOpen = false
}

func TestDemoCommandOptions(t *testing.T) {
	opts := commandOptions()
	cmds := editor.Commands()
	if len(opts) != len(cmds) {
		t.Fatalf("palette has %d commands, registry %d", len(opts), len(cmds))
	}
	for i, cmd := range cmds {
		if opts[i].Value != cmd.ID || opts[i].Label == "" {
			t.Fatalf("missing %s", cmd.ID)
		}
	}
}

func TestDemoPaletteRestoresTargetFocus(t *testing.T) {
	prev := text.UseSystemEngine(false)
	defer text.UseSystemEngine(prev)
	setupDocument("sample.ts", "const value = 1;\n", theme.Base("vs-dark"), true, false)
	defer st.Close()
	defer background.Close()
	defer second.Close()
	d := ui.NewDriver(page, ui.DriverOptions{Width: 1000, Height: 600})
	d.Frame()
	d.Click(geom.Pt(750, 150))
	d.Frame()
	if active != second {
		t.Fatal("right pane did not acquire independent focus")
	}
	second.Execute("editor.action.quickCommand", nil)
	d.Frame()
	pendingCommand, paletteOpen = "editor.action.selectAll", false
	for range 4 {
		d.Frame()
	}
	if pendingCommand != "" || second.Primary().Anchor == second.Primary().Head {
		t.Fatal("palette command not executed on target after restoring focus")
	}
	if st.Primary().Anchor != st.Primary().Head {
		t.Fatal("palette changed other pane's selection")
	}
}

func TestAutoCountsOncePerFrame(t *testing.T) {
	// Preserve the peer's c.Now guard: repeated page builds in one runner
	// frame must not count twice or post two input events.
	a := &autoRun{}
	d := ui.NewDriver(func(c *ui.Context) { a.frameNow = c.Now(); a.frame(c) }, ui.DriverOptions{})
	d.Frame()
	if a.n != 0 {
		t.Fatalf("same frame counted %d times", a.n)
	}
}

// TestIndentStatus checks the status line's indentation item shows the
// indentation detected when the document is opened.
func TestIndentStatus(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"a {\n  b\n}\n", "Spaces: 2"},
		{"a {\n\tb\n}\n", "Tab Size: 4"},
		{"a\nb\n", "Spaces: 4"},
	} {
		if got := indentStatus(editor.NewModel("a.ts", c.text, nil)); got != c.want {
			t.Errorf("%q: %q, want %q", c.text, got, c.want)
		}
	}
}
