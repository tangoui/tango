//go:build tango_dev

package uihost

// Whole-app automation of self-drawn windows (docs/automation.md): every
// window Open creates is registered with the control channel, which the
// root package starts when TANGO_AUTOMATION is set. The semantics tree
// comes from the window's accessibility bridge (a11y.go): automation is
// one of its subscribers (tap) and keeps the tree built (force) since no
// assistive technology asks for it. Everything here runs on the main
// thread. Only built with -tags tango_dev; production builds, the default,
// compile it out (automation_release.go).

import (
	"image"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/automation"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// automationWatch registers the window s showing r; h is nil for windows
// shown by the tan dev host, which automation does not support.
func automationWatch(app *tango.App, s *tango.Surface, r *ui.Runner, h *host) {
	c := &autoCanvas{r: r, h: h}
	unregister := automation.Register(app, uint64(s.ID()), c)
	s.OnClosed(func() {
		unregister()
		c.stop()
	})
}

var errAutoClosed = &automation.Error{Code: automation.CodeNoWindow, Msg: "automation: the window has closed"}

// autoCanvas implements automation.Canvas. Main thread.
type autoCanvas struct {
	r *ui.Runner
	h *host // nil under tan dev

	on      bool
	tree    *a11y.Tree
	gen     uint64
	release func() // drops the force reference
	cancel  func() // ends the subscription
}

func (c *autoCanvas) bridge() (*a11yBridge, error) {
	if c.h == nil {
		return nil, automation.ErrHotReload
	}
	if c.h.closed {
		return nil, errAutoClosed
	}
	b := a11yFor(c.r)
	if b == nil {
		return nil, automation.ErrHotReload
	}
	return b, nil
}

func (c *autoCanvas) Tree() (*a11y.Tree, uint64, error) {
	b, err := c.bridge()
	if err != nil {
		return nil, 0, err
	}
	if !c.on {
		c.on = true
		// Subscribe first: an existing tree (assistive technology already
		// active) arrives synchronously as the baseline.
		c.cancel = b.tap(func(t *a11y.Tree, _ []a11y.NodeID) {
			c.tree = t
			c.gen++
		})
		c.release = b.force()
	}
	return c.tree, c.gen, nil
}

func (c *autoCanvas) Perform(id a11y.NodeID, act a11y.Action, arg string) error {
	if _, err := c.bridge(); err != nil {
		return err
	}
	return c.r.PerformAction(id, act, arg)
}

func (c *autoCanvas) Input(evs []input.Event) error {
	if _, err := c.bridge(); err != nil {
		return err
	}
	for _, ev := range evs {
		c.h.CanvasInput(ev) // window shortcuts first, as for platform input
	}
	return nil
}

// Snapshot paints the last frame's display list on the CPU, so it works
// the same whether the window renders with the CPU or the GPU.
func (c *autoCanvas) Snapshot() (*image.RGBA, error) {
	if _, err := c.bridge(); err != nil {
		return nil, err
	}
	w, h := c.h.cv.Size()
	cpu := paint.NewCPU()
	if err := cpu.Render(c.r.List(), geom.Size{W: w, H: h}, c.h.cv.Scale()); err != nil {
		return nil, err
	}
	return cpu.Image(), nil
}

func (c *autoCanvas) stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.release != nil {
		c.release()
	}
	c.cancel, c.release, c.tree = nil, nil, nil
}
