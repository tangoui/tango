package main

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"math"
	"testing"
)

type testEditorFrame struct{ closed int }

func (f *testEditorFrame) FrameInfo() ui.WindowFrameInfo { return ui.WindowFrameInfo{} }
func (f *testEditorFrame) SetWindowState(ui.WindowState) {}
func (f *testEditorFrame) CloseWindow()                  { f.closed++ }
func (f *testEditorFrame) StartWindowMove()              {}

func TestCustomTitlebarClosePreservesUnsavedGuard(t *testing.T) {
	a, fb := testDocumentApp(t, "")
	frame := &testEditorFrame{}
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return 2, nil }
	tango.Await(a, func() bool { guardedEditorFrame{frame}.CloseWindow(); return true })
	if frame.closed != 0 {
		t.Fatal("custom titlebar bypassed Cancel")
	}
	fb.OnMessageDialog = func(_ platform.SurfaceID, _ platform.MessageOptions) (int, error) { return 1, nil }
	tango.Await(a, func() bool { guardedEditorFrame{frame}.CloseWindow(); return true })
	if frame.closed != 1 {
		t.Fatal("Don't Save did not allow close")
	}
}

func TestWorkbenchHasNavigationInsteadOfSettingsForm(t *testing.T) {
	d := shellDriver(t, 1200)
	if _, ok := d.A11yFind(a11y.RoleCombobox, "Editor theme"); ok {
		t.Fatal("persistent theme form is still inside navigation")
	}
	if _, ok := d.FindText("EXPLORER"); !ok {
		t.Fatal("explorer header missing")
	}
	if _, ok := d.FindText("SHORTCUTS"); ok {
		t.Fatal("navigation remains a demo settings panel")
	}
	explorer, ok := d.FindText("EXPLORER")
	if !ok {
		t.Fatal("explorer header missing")
	}
	tab, ok := d.A11yFind(a11y.RoleTab, "main.go")
	if !ok {
		t.Fatal("file tab missing")
	}
	if math.Abs(float64(explorer.Y+explorer.H/2-(tab.Bounds.Y+tab.Bounds.H/2))) > 8 {
		t.Fatalf("headers misaligned: explorer=%+v tab=%+v", explorer, tab.Bounds)
	}
}
