//go:build tango_dev

package uihost_test

import (
	"sync/atomic"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

// Under tan dev the key comes from the host window over the socket
// (remoteWin.input); the worker's table matches it there.
func TestRemoteShortcut(t *testing.T) {
	_, hfb, hs, o := openRemote(t, tango.SurfaceOptions{Title: "Keys", Width: 200, Height: 100})
	cv := hfb.Canvas(hs.ID())
	eventually(t, "first frame", func() bool { w, _ := cv.LastFrameSize(); return w > 0 })
	var n atomic.Int32
	shortcut(t, o.s, "CmdOrCtrl+N", uihost.ShortcutOptions{}, func() { n.Add(1) })
	press(cv, input.KeyN, cmd())
	eventually(t, "remote shortcut", func() bool { return n.Load() == 1 })
	press(cv, input.KeyM, cmd())
	press(cv, input.KeyN, cmd())
	eventually(t, "second press", func() bool { return n.Load() == 2 })
}

func TestRemoteHotkeyRecording(t *testing.T) {
	addr, hfb := startHost(t)
	wa, wfb := faketest.Run(t, tango.Options{})
	if err := uihost.ConnectDev(addr, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(uihost.ResetDev)
	value := ""
	focused := false
	o := tango.Await(wa, func() opened {
		r, s, err := uihost.Open(wa, uihost.Options{Surface: tango.SurfaceOptions{Title: "Recorder", Width: 300, Height: 160}}, func(c *ui.Context) {
			hk := ui.HotkeyInput(c, &value)
			if !focused {
				c.SetFocus(hk.ID())
				focused = true
			}
		}, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	eventually(t, "host window", func() bool { return len(hfb.Surfaces()) == 1 })
	cv := hfb.Canvas(hfb.Surfaces()[0].ID())
	eventually(t, "host recorder state", func() bool { cv.FlushFrame(); return cv.ShortcutCapture() })
	var hits atomic.Int32
	shortcut(t, o.s, "CmdOrCtrl+K", uihost.ShortcutOptions{InTextInput: true}, func() { hits.Add(1) })
	press(cv, input.KeyK, cmd())
	eventually(t, "remote recording", func() bool {
		cv.FlushFrame()
		recorded := false
		wfb.Do(func() { recorded = value == "CmdOrCtrl+K" })
		return recorded && !cv.ShortcutCapture()
	})
	if hits.Load() != 0 {
		t.Fatal("window shortcut fired during recording")
	}
	press(cv, input.KeyK, cmd())
	eventually(t, "restored remote shortcut", func() bool { return hits.Load() == 1 })
}
