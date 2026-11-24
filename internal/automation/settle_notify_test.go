package automation

import (
	"image"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// notifyCanvas announces each new tree generation (TreeWaiter).
type notifyCanvas struct {
	mu      sync.Mutex
	tree    *a11y.Tree
	gen     uint64
	changed chan struct{}
}

func (c *notifyCanvas) Tree() (*a11y.Tree, uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tree, c.gen, nil
}
func (c *notifyCanvas) Perform(a11y.NodeID, a11y.Action, string) error { return nil }
func (c *notifyCanvas) Input([]input.Event) error                      { return nil }
func (c *notifyCanvas) Snapshot() (*image.RGBA, error)                 { return nil, nil }
func (c *notifyCanvas) OffMain() bool                                  { return true }

func (c *notifyCanvas) TreeChanged(gen uint64) <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	if c.changed == nil {
		c.changed = make(chan struct{})
	}
	return c.changed
}

func (c *notifyCanvas) publish(t *a11y.Tree) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tree, c.gen = t, c.gen+1
	if c.changed != nil {
		close(c.changed)
		c.changed = nil
	}
}

// A canvas that announces new trees ends settle and waitTree when the
// frame lands, not at the next poll (15 ms in production): that wait was
// added to every action an automation client timed.
func TestSettleAndWaitTreeFollowTreeNotifications(t *testing.T) {
	h := &fakeHost{quit: make(chan struct{})}
	s := &Server{host: h, pollEvery: time.Hour, settleFor: 5 * time.Second}
	c := &notifyCanvas{}

	go func() { time.Sleep(20 * time.Millisecond); c.publish(&a11y.Tree{}) }()
	start := time.Now()
	s.settle(c, 0)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("settle waited %v for a notified tree", d)
	}

	go func() { time.Sleep(20 * time.Millisecond); c.publish(&a11y.Tree{Root: 7}) }()
	start = time.Now()
	timedOut, err := s.waitTree(c, 5*time.Second, func(t *a11y.Tree) bool { return t.Root == 7 })
	if err != nil || timedOut {
		t.Fatalf("waitTree: timedOut %v, err %v", timedOut, err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("waitTree waited %v for a notified tree", d)
	}

	// Without a change the timeouts still hold.
	s.settleFor = 30 * time.Millisecond
	start = time.Now()
	s.settle(c, c.gen)
	if d := time.Since(start); d < 30*time.Millisecond || d > time.Second {
		t.Fatalf("settle without a change returned after %v", d)
	}
}
