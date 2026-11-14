package updateui

import (
	"log"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/shell"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

// Window size, in logical pixels.
const (
	windowWidth  = 480
	windowHeight = 360
)

// window is the session's update window, if open. Main thread only.
type window struct {
	surf   *tango.Surface
	runner *ui.Runner
}

// show opens the window, or brings it forward when it is open. Safe from
// any goroutine.
func (s *session) show(manual bool) {
	s.app.Post(func() {
		if s.win.surf != nil {
			s.win.surf.Show()
			s.win.surf.Focus()
			return
		}
		opts := s.opts.Window
		if opts.Surface.Title == "" {
			opts.Surface.Title = s.strings().Title
		}
		if opts.Surface.Width == 0 && opts.Surface.Height == 0 {
			opts.Surface.Width, opts.Surface.Height = windowWidth, windowHeight
		}
		r, surf, err := uihost.Open(s.app, opts, page(s.m, s.strings, s.actions()), ui.RunnerOptions{})
		if err != nil {
			log.Printf("updateui: opening the window: %v", err)
			return
		}
		s.win = window{surf: surf, runner: r}
		s.m.set(func(m *model) { m.onChange = r.Invalidate })
		surf.OnClosed(func() {
			s.win = window{}
			s.m.set(func(m *model) { m.onChange = nil })
		})
	})
}

// closeWindow closes the window if it is open. Main thread.
func (s *session) closeWindow() {
	if s.win.surf != nil {
		s.win.surf.Close()
	}
}

// actions wires the window's buttons. They run on the main thread, during
// a frame: anything slow or window-closing is handed off.
func (s *session) actions() actions {
	closeLater := func() { s.app.Post(s.closeWindow) }
	return actions{
		skip:    func() { s.f.skip(); closeLater() },
		later:   func() { s.f.later(); closeLater() },
		install: s.f.download,
		cancel:  s.f.cancelDownload,
		restartNow: func() {
			go func() {
				if err := s.f.restartNow(); err != nil {
					log.Printf("updateui: installing: %v", err)
				}
			}()
		},
		installNextLaunch: func() { s.f.installNextLaunch(); closeLater() },
		retry:             func() { go s.f.check(s.ctx, true) },
		close:             closeLater,
		openDownloadPage: func() {
			if rel := s.m.snapshot().rel; rel != nil && rel.Full.URL != "" {
				if err := shell.OpenURL(s.app, rel.Full.URL); err != nil {
					log.Printf("updateui: %v", err)
				}
			}
			closeLater()
		},
	}
}
