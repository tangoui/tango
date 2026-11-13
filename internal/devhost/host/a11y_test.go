package host

import (
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/a11y"
)

// fakeBridge records what the host hands the platform bridge.
type fakeBridge struct {
	mu     sync.Mutex
	active bool
	calls  []bridgeCall
}

type bridgeCall struct {
	op      string // "update", "replace", "reset", "close"
	tree    *a11y.Tree
	changed []a11y.NodeID
}

func (b *fakeBridge) setActive(on bool) { b.mu.Lock(); b.active = on; b.mu.Unlock() }
func (b *fakeBridge) Active() bool      { b.mu.Lock(); defer b.mu.Unlock(); return b.active }
func (b *fakeBridge) rec(c bridgeCall)  { b.mu.Lock(); b.calls = append(b.calls, c); b.mu.Unlock() }
func (b *fakeBridge) Update(t *a11y.Tree, ch []a11y.NodeID) {
	b.rec(bridgeCall{"update", t, slices.Clone(ch)})
}
func (b *fakeBridge) Replace(t *a11y.Tree) { b.rec(bridgeCall{op: "replace", tree: t}) }
func (b *fakeBridge) Reset()               { b.rec(bridgeCall{op: "reset"}) }
func (b *fakeBridge) Close()               { b.rec(bridgeCall{op: "close"}) }

func (b *fakeBridge) last() bridgeCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.calls) == 0 {
		return bridgeCall{}
	}
	return b.calls[len(b.calls)-1]
}

func (c bridgeCall) name(id a11y.NodeID) string {
	if n := c.tree.Node(id); n != nil {
		return n.Name
	}
	return ""
}

func buttonTree(name string, focus a11y.NodeID) *a11y.Tree {
	t := a11y.NewTree(1)
	t.Add(&a11y.Node{ID: 1, Role: a11y.RoleWindow, Children: []a11y.NodeID{2}})
	t.Add(&a11y.Node{ID: 2, Parent: 1, Role: a11y.RoleButton, Name: name, Actions: a11y.Press | a11y.Focus})
	t.Focus = focus
	return t
}

func TestA11yAcrossSwap(t *testing.T) {
	br := &fakeBridge{}
	old := NewA11yBridge
	NewA11yBridge = func(platform.Canvas) A11yBridge { return br }
	t.Cleanup(func() { NewA11yBridge = old })

	e := start(t)
	w1 := e.worker()
	o1 := w1.open("Main", 0)
	w1.frame(o1.Win, red)
	cv := e.canvas()
	eventually(t, "first frame", func() bool { return pixel(cv, 1, 1) == red })
	ctlNext[devhost.Swapped](e)

	// No assistive technology: no tree wanted, actions refused.
	if err := cv.A11yAction(2, a11y.Press, ""); err == nil {
		t.Fatal("action without a tree accepted")
	}

	// An assistive technology attaches: the worker is asked for a tree.
	br.setActive(true)
	cv.Invalidate()
	cv.FlushFrame()
	st := next[devhost.A11yState](w1)
	if st.Win != o1.Win || !st.On || st.Epoch != 1 {
		t.Fatalf("state %+v", st)
	}
	t1 := buttonTree("OK", 2)
	w1.cl.Send(devhost.NewA11yTree(o1.Win, st.Epoch, t1, nil, true))
	eventually(t, "full tree", func() bool {
		cv.FlushFrame()
		c := br.last()
		return c.op == "replace" && c.name(2) == "OK" && c.tree.Focus == 2
	})

	// A delta updates it.
	t2 := buttonTree("Save", 2)
	w1.cl.Send(devhost.NewA11yTree(o1.Win, st.Epoch, t2, a11y.Diff(t1, t2), false))
	eventually(t, "delta", func() bool {
		cv.FlushFrame()
		c := br.last()
		return c.op == "update" && c.name(2) == "Save" && slices.Equal(c.changed, []a11y.NodeID{2})
	})

	// Actions go back to the worker; unsupported ones are refused here.
	if err := cv.A11yAction(2, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if act := next[devhost.A11yAction](w1); act != (devhost.A11yAction{Win: o1.Win, ID: 2, Act: a11y.Press}) {
		t.Fatalf("action %+v", act)
	}
	if err := cv.A11yAction(2, a11y.Increment, ""); err == nil {
		t.Fatal("unsupported action accepted")
	}
	if err := cv.A11yAction(9, a11y.Press, ""); err == nil {
		t.Fatal("action on a missing node accepted")
	}

	// Hot reload: the new worker is asked for a tree when it opens the
	// window; the platform keeps the old tree until the new frame shows.
	w2 := e.worker()
	go w2.cl.Request(func(id int) any { return devhost.SnapshotGet{Req: id} }, wait)
	req := next[devhost.SnapshotReq](w1)
	w1.cl.Send(devhost.SnapshotReply{Req: req.Req})
	o2 := w2.open("Main", 0)
	st2 := next[devhost.A11yState](w2)
	if st2.Win != o2.Win || !st2.On || st2.Epoch != 1 {
		t.Fatalf("new worker state %+v", st2)
	}
	w2.cl.Send(devhost.NewA11yTree(o2.Win, st2.Epoch, buttonTree("New", 0), nil, true))
	cv.Invalidate()
	cv.FlushFrame()
	if c := br.last(); c.name(2) != "Save" {
		t.Fatalf("tree replaced before the swap: %+v", c)
	}
	w2.frame(o2.Win, blue)
	eventually(t, "swap", func() bool { return pixel(cv, 1, 1) == blue })
	ctlNext[devhost.Swapped](e)
	eventually(t, "tree replaced", func() bool {
		cv.FlushFrame()
		c := br.last()
		return c.op == "replace" && c.name(2) == "New" && c.tree.Focus == 0
	})
	if err := cv.A11yAction(2, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if act := next[devhost.A11yAction](w2); act.ID != 2 || act.Win != o2.Win {
		t.Fatalf("action after swap %+v", act)
	}

	// The technology leaves and comes back: a new epoch; stale deltas are
	// ignored until the full tree of the new request.
	br.setActive(false)
	cv.Invalidate()
	cv.FlushFrame()
	if st := next[devhost.A11yState](w2); st.On {
		t.Fatalf("state %+v", st)
	}
	br.setActive(true)
	cv.Invalidate()
	cv.FlushFrame()
	st3 := next[devhost.A11yState](w2)
	if !st3.On || st3.Epoch != 2 {
		t.Fatalf("state %+v", st3)
	}
	n := len(br.calls)
	w2.cl.Send(devhost.NewA11yTree(o2.Win, 1, buttonTree("Stale", 0), []a11y.NodeID{2}, false))
	w2.cl.Send(devhost.NewA11yTree(o2.Win, 2, buttonTree("Again", 0), nil, true))
	eventually(t, "tree after reactivation", func() bool {
		cv.FlushFrame()
		c := br.last()
		return c.op == "replace" && c.name(2) == "Again"
	})
	br.mu.Lock()
	for _, c := range br.calls[n:] {
		if c.tree != nil && c.name(2) == "Stale" {
			t.Error("stale delta applied")
		}
	}
	br.mu.Unlock()

	// The worker on screen crashes: its tree goes away with it.
	w2.cl.Close()
	ctlNext[devhost.WorkerLeft](e)
	eventually(t, "tree dropped", func() bool { cv.FlushFrame(); return br.last().op == "reset" })
	if err := cv.A11yAction(2, a11y.Press, ""); err == nil {
		t.Fatal("action without a worker accepted")
	}
}
