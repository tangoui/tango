//go:build tango_dev

package uihost_test

import (
	"sync/atomic"
	"testing"

	"github.com/tangoui/tango"
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
