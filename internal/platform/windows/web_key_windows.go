//go:build windows && (amd64 || arm64) && tango_dev

package windows

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

func (s *surface) DispatchWebKey(key input.Key, mods input.Modifiers, done func(error)) {
	p, ok := planWebKey(key, mods)
	switch {
	case s.closed:
		done(platform.ErrWebViewClosed)
		return
	case !ok || s.canvas != nil:
		done(platform.ErrUnsupported)
		return
	case s.failed != nil:
		done(s.failed)
		return
	case !s.ready || s.webview == 0:
		// Like Eval, wait for the controller; the automation request
		// times out if the WebView fails or closes first.
		s.whenReady(func() { s.DispatchWebKey(key, mods, done) })
		return
	}
	m := s.menuOwner()
	var edit string
	if m.spec != nil {
		cmd, action := p.menu(m.spec.plan)
		if cmd != 0 {
			m.runCommand(cmd)
			done(nil)
			return
		}
		edit = action
	}

	// Submit both events in order while the current document is targeted.
	// A failed press still leaves the release queued.
	remaining := 2
	var firstErr error
	report := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
		remaining--
		if remaining == 0 {
			done(firstErr)
		}
	}
	s.webInputCDP("Input.dispatchKeyEvent", p.event(true, edit), report)
	s.webInputCDP("Input.dispatchKeyEvent", p.event(false, ""), report)
}
