//go:build tango_dev

package uihost_test

import (
	"image"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/uihost"
)

// The host window's canvas exposes the worker's semantics tree through the
// platform bridge; actions reach the worker's widgets; a hot reload
// replaces the tree and drops the old focus.
func TestRemoteAccessibility(t *testing.T) {
	addr, hfb := startHost(t)
	wa, wfb := faketest.Run(t, tango.Options{})
	if err := uihost.ConnectDev(addr, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(uihost.ResetDev)
	clicks := 0
	o := tango.Await(wa, func() opened {
		r, s, err := uihost.Open(wa, uihost.Options{Surface: tango.SurfaceOptions{Title: "A11y", Width: 300, Height: 200}},
			func(c *ui.Context) {
				if ui.Button(c, "OK").Clicked() {
					clicks++
				}
			}, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	eventually(t, "host window", func() bool { return len(hfb.Surfaces()) == 1 })
	cv := hfb.Canvas(hfb.Surfaces()[0].ID())
	eventually(t, "first frame", func() bool { w, _ := cv.LastFrameSize(); return w > 0 })
	if cv.A11yTree() != nil {
		t.Fatal("tree without an assistive technology")
	}

	// A screen reader attaches to the host window.
	cv.SetA11yActive(true)
	var btn *a11y.Node
	eventually(t, "tree on the host canvas", func() bool {
		cv.FlushFrame()
		if tr := cv.A11yTree(); tr != nil {
			btn = tr.Find(a11y.RoleButton, "OK")
		}
		return btn != nil
	})
	for _, c := range cv.A11yCalls() {
		if !c.Main {
			t.Fatalf("%s off the main thread", c.Op)
		}
	}

	// Its actions reach the button in the worker.
	if err := cv.A11yAction(btn.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, "focus on the host canvas", func() bool { cv.FlushFrame(); return cv.A11yFocus() == btn.ID })
	if err := cv.A11yAction(btn.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, "click", func() bool {
		cv.FlushFrame()
		n := 0
		wfb.Do(func() { n = clicks })
		return n == 1
	})

	// Hot reload: a replacement worker (raw protocol) takes the window
	// over with its own tree.
	msgs := make(chan any, 100)
	cl, err := devhost.Connect(addr, "", func(m any) { msgs <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if _, err := cl.Request(func(id int) any { return devhost.SnapshotGet{Req: id} }, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	m, err := cl.Request(func(id int) any {
		return devhost.OpenWindow{Req: id, Opts: devhost.WindowOpts{Title: "A11y", Width: 300, Height: 200}}
	}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	win := m.(devhost.Opened).Win
	fw := devhost.NewFrameWriter(win, "", cl.Send)
	var st devhost.A11yState
	var req devhost.FrameRequest
	for got := 0; got < 2; {
		select {
		case m := <-msgs:
			switch m := m.(type) {
			case devhost.A11yState:
				st = m
				got++
			case devhost.FrameRequest:
				req = m
				got++
			case devhost.FrameAck:
				fw.Ack(m.Seg, m.Slot)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("replacement worker: no A11yState / FrameRequest")
		}
	}
	if !st.On || st.Win != win {
		t.Fatalf("state %+v", st)
	}
	nt := a11y.NewTree(1)
	nt.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{7}})
	nt.Add(&a11y.Node{ID: 7, Parent: 1, Role: a11y.RoleButton, Name: "Reloaded", Actions: a11y.Press})
	if err := cl.Send(devhost.NewA11yTree(win, st.Epoch, nt, nil, true)); err != nil {
		t.Fatal(err)
	}
	cv.ResetA11yCalls()
	sz := func(v, s float32) int { return int(v*s + 0.999) }
	if err := fw.Write(image.NewRGBA(image.Rect(0, 0, sz(req.W, req.Scale), sz(req.H, req.Scale))), req.Scale); err != nil {
		t.Fatal(err)
	}
	eventually(t, "replaced tree", func() bool {
		cv.FlushFrame()
		tr := cv.A11yTree()
		return tr != nil && tr.Find(a11y.RoleButton, "Reloaded") != nil
	})
	if f := cv.A11yFocus(); f != 0 {
		t.Fatalf("focus %d left from the old tree", f)
	}
	calls := cv.A11yCalls()
	if len(calls) < 2 || calls[0].Op != "set" || calls[0].Tree != nil || calls[1].Op != "set" {
		t.Fatalf("replacement calls %+v", calls)
	}
	if err := cv.A11yAction(btn.ID, a11y.Press, ""); err == nil {
		t.Fatal("action on a node of the old tree accepted")
	}
	if err := cv.A11yAction(7, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case m := <-msgs:
			if a, ok := m.(devhost.A11yAction); ok {
				if a.ID != 7 || a.Act != a11y.Press || a.Win != win {
					t.Fatalf("action %+v", a)
				}
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("replacement worker: no A11yAction")
		}
	}
}
