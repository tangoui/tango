package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

type recordingWindow struct {
	*driverWindow
	capture bool
}

func (w *recordingWindow) SetShortcutCapture(on bool) { w.capture = on }

func TestHotkeyCaptureLifecycle(t *testing.T) {
	value := "Ctrl+S"
	show, disabled := true, false
	var hk *HotkeyInputW
	var next ID
	d := NewDriver(func(c *Context) {
		if show {
			hk = HotkeyInput(c, &value).Disabled(disabled)
		}
		next = Button(c, "next").n.id
	}, DriverOptions{Width: 500, Height: 180})
	w := &recordingWindow{driverWindow: d.w}
	d.r.Attach(w)
	d.Settle()
	start := func() {
		t.Helper()
		rect := d.r.Context().rects[hk.ID()]
		d.Click(geom.Pt(rect.X+12, rect.Y+rect.H/2))
		if !w.capture || !hk.Recording() {
			t.Fatal("recording did not capture shortcuts")
		}
		if w.textOn {
			t.Fatal("recorder enabled the IME")
		}
	}
	start()
	d.Key(input.KeyF5, 0)
	if w.capture || value != "F5" {
		t.Fatalf("completion: capture=%v value=%s", w.capture, value)
	}
	start()
	d.Key(input.KeyEscape, 0)
	if w.capture || hk.Recording() || value != "F5" {
		t.Fatal("Escape did not release capture and preserve value")
	}
	start()
	d.Key(input.KeyTab, 0)
	if w.capture || d.r.Context().Focused() != next {
		t.Fatal("Tab did not release recording focus")
	}
	start()
	d.Inject(input.WindowFocus{Focused: false})
	d.Settle()
	if w.capture || hk.Recording() {
		t.Fatal("window blur kept capture")
	}
	d.Inject(input.WindowFocus{Focused: true})
	d.Settle()
	if w.capture || hk.Recording() {
		t.Fatal("window activation restarted a cancelled recording")
	}
	start()
	disabled = true
	d.Frame()
	if w.capture {
		t.Fatal("disabled recorder kept capture")
	}
	disabled = false
	d.Settle()
	start()
	show = false
	d.Frame()
	if w.capture {
		t.Fatal("removed recorder kept capture")
	}
}

func TestHotkeyRecordingPreemptsMenubar(t *testing.T) {
	for _, before := range []bool{false, true} {
		t.Run(map[bool]string{true: "bar-first", false: "recorder-first"}[before], func(t *testing.T) {
			value := ""
			var hk *HotkeyInputW
			var bar *MenubarW
			d := NewDriver(func(c *Context) {
				menu := func() {
					bar = Menubar(c, []MenubarMenu{{Key: "file", Label: "File", Items: []MenubarItem{{Key: "save", Label: "Save"}}}})
				}
				if before {
					menu()
				}
				hk = HotkeyInput(c, &value)
				if !before {
					menu()
				}
			}, DriverOptions{Width: 500, Height: 200})
			d.Settle()
			rect := d.r.Context().rects[hk.ID()]
			d.Click(geom.Pt(rect.X+10, rect.Y+rect.H/2))
			d.Key(input.KeyF10, 0)
			if value != "F10" || hk.Recording() || bar.OpenMenu() != "" || d.r.Context().Focused() != hk.ID() {
				t.Fatalf("F10 stolen: value=%q recording=%v menu=%q", value, hk.Recording(), bar.OpenMenu())
			}
		})
	}
}

func TestHotkeyCaptureCancelsBatchedBlur(t *testing.T) {
	value, focus := "Ctrl+S", false
	var hk *HotkeyInputW
	d := NewDriver(func(c *Context) {
		hk = HotkeyInput(c, &value)
		if !focus {
			c.SetFocus(hk.ID())
			focus = true
		}
	}, DriverOptions{Width: 300, Height: 150})
	w := &recordingWindow{driverWindow: d.w}
	d.r.Attach(w)
	d.Settle()
	if !w.capture {
		t.Fatal("recorder not started")
	}
	// Both native focus notifications can arrive before the next vsync.
	d.Inject(input.WindowFocus{Focused: false})
	d.Inject(input.WindowFocus{Focused: true})
	d.Settle()
	if w.capture || hk.Recording() {
		t.Fatal("batched blur/focus kept recording active")
	}
	d.Key(input.KeyF5, 0)
	if value != "Ctrl+S" {
		t.Fatal("cancelled recorder changed value")
	}
}
