package main

import (
	"context"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

// coalescer hands items produced on other goroutines to the UI thread in
// batches. Waking the UI thread once per item would pile work up there
// when items come faster than frames; the coalescer wakes it once, then
// not again until the UI thread has taken what is buffered.
type coalescer struct {
	wake func() // asks the UI thread for a frame, which calls drain

	mu      sync.Mutex
	items   []string
	pending bool // wake was called and drain has not run since
}

// newCoalescer returns a coalescer that calls wake when buffered items
// wait for the UI thread. wake must make the UI thread run a frame (for
// the panel: app.Post(runner.Invalidate)), and that frame calls drain.
func newCoalescer(wake func()) *coalescer {
	return &coalescer{wake: wake}
}

// push buffers item. It may be called on any goroutine and never blocks
// on the UI thread.
func (c *coalescer) push(item string) {
	c.mu.Lock()
	c.items = append(c.items, item)
	first := !c.pending
	c.pending = true
	c.mu.Unlock()
	if first {
		c.wake() // outside the lock: wake may even call drain
	}
}

// drain takes every buffered item and clears the pending flag, so the
// next push wakes the UI thread again. Call it on the UI thread.
func (c *coalescer) drain() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := c.items
	c.items, c.pending = nil, false
	return items
}

// echoPanel is the Go-drawn panel of -panel: it uses the same "Echo"
// service as the page, in process through Router.Open.
type echoPanel struct {
	echoes *coalescer
	recent []string // the last few echoes; UI thread only
}

const recentEchoes = 5

// attachPanel draws an echoPanel over the page of s and starts its Echo
// stream on r. Call it on the main thread (OnReady).
func attachPanel(app *tango.App, s *tango.Surface, r *ipc.Router) error {
	var runner *ui.Runner
	p := &echoPanel{}
	// Each wake asks for one frame on the UI thread; page drains there.
	p.echoes = newCoalescer(func() { app.Post(runner.Invalidate) })

	// A full-window canvas above the page whose transparent pixels let
	// the pointer through: only the card is opaque.
	th := theme.AntDesign()
	th.BgLayout = theme.Color{}
	var err error
	runner, _, err = uihost.Attach(s, uihost.Layer{Above: true, PassThrough: true}, p.page, ui.RunnerOptions{Theme: th})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.OnClosed(cancel)
	go p.run(ctx, r)
	return nil
}

// run sends {"t": time} on an Echo stream every second and pushes each
// echo to the panel. Open, Write and Receive block, so this runs on its
// own goroutine, never on the UI thread.
func (p *echoPanel) run(ctx context.Context, r *ipc.Router) {
	s, err := r.Open(ctx, "Echo")
	if err != nil {
		p.echoes.push("Open: " + err.Error())
		return
	}
	defer s.Close()

	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				if s.Write(map[string]string{"t": now.Format("15:04:05")}) != nil {
					return
				}
			}
		}
	}()

	for {
		var m struct {
			T string `json:"t"`
		}
		if err := s.ReceiveJSON(&m); err != nil {
			p.echoes.push("closed: " + err.Error())
			return
		}
		p.echoes.push("← " + m.T)
	}
}

// page draws the panel; it runs on the UI thread, which is where recent
// is read and written.
func (p *echoPanel) page(c *ui.Context) {
	p.recent = append(p.recent, p.echoes.drain()...)
	if n := len(p.recent); n > recentEchoes {
		p.recent = p.recent[n-recentEchoes:]
	}

	ui.Card(c, "Go 面板 · Echo").Shadow("always").
		Layout(layout.Abs(layout.Unset, 16, 16, layout.Unset), layout.W(260)).
		Body(func() {
			ui.Column(c, func() {
				ui.Text(c, `r.Open(ctx, "Echo")，每秒一帧`).Color(c.Theme.TextSecondary)
				if len(p.recent) == 0 {
					ui.Text(c, "等待回显…")
				}
				for _, e := range p.recent {
					ui.Text(c, e)
				}
			}, layout.Gap(6))
		})
}
