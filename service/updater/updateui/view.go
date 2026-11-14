package updateui

import (
	"fmt"
	"runtime"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

// actions are what the window's buttons do.
type actions struct {
	skip, later, install, cancel   func()
	restartNow, installNextLaunch  func()
	retry, close, openDownloadPage func()
}

// canInstall reports platforms where Updater.Apply can update the app.
func canInstall() bool {
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
		return true
	}
	return false
}

// page draws the window for the model's current phase; s supplies the
// texts each frame, so a locale change shows on the next frame.
func page(m *model, s func() *Strings, a actions) func(*ui.Context) {
	return func(c *ui.Context) {
		v, t := m.snapshot(), s()
		ui.Column(c, func() {
			ui.Title(c, t.Title, 4)
			body(c, v, t)
			ui.Row(c, func() { buttons(c, v, t, a) }, layout.End(), layout.Gap(8))
		}, layout.Grow(1), layout.Pad(24), layout.Gap(12))
	}
}

// body is the phase's message (and notes or progress).
func body(c *ui.Context, v view, t *Strings) {
	grow := func(fn func()) { ui.Column(c, fn, layout.Grow(1), layout.Gap(8)) }
	switch v.phase {
	case phaseChecking:
		grow(func() { ui.Spin(c).Tip(t.Checking) })
	case phaseAvailable:
		grow(func() {
			ui.Text(c, fmt.Sprintf(t.NewVersion, v.rel.Version, v.current))
			if !v.rel.PubDate.IsZero() {
				ui.Text(c, fmt.Sprintf(t.Released, v.rel.PubDate.Format("2006-01-02"))).Secondary()
			}
			if v.rel.Notes != "" {
				ui.Text(c, t.Notes).Strong()
				ui.Scroll(c, func() { ui.Markdown(c, v.rel.Notes) }, layout.Grow(1))
			}
		})
	case phaseDownloading:
		grow(func() {
			ui.Text(c, t.Downloading)
			if p := v.progress(); p < 0 {
				ui.Progress(c, 0).Indeterminate()
			} else {
				ui.Progress(c, p*100)
			}
			kind := t.Full
			if v.patch {
				kind = t.Patch
			}
			ui.Text(c, kind).Secondary()
		})
	case phaseReady:
		grow(func() { ui.Text(c, t.Ready) })
	case phaseDownloaded:
		grow(func() { ui.Text(c, t.Downloaded) })
	case phaseError:
		grow(func() {
			if v.unsupported {
				ui.Text(c, t.Unsupported)
			} else {
				ui.Text(c, t.Error).Type("danger")
			}
			if v.errText != "" {
				ui.Text(c, v.errText).Secondary()
			}
		})
	case phaseUpToDate:
		grow(func() { ui.Text(c, fmt.Sprintf(t.UpToDate, v.current)) })
	case phaseRolledBack:
		grow(func() { ui.Text(c, fmt.Sprintf(t.RolledBack, v.current)) })
	default:
		grow(func() {})
	}
}

// buttons are the phase's actions, primary last (trailing edge).
func buttons(c *ui.Context, v view, t *Strings, a actions) {
	btn := func(label string, fn func(), primary bool) {
		b := ui.Button(c, label)
		if primary {
			b.Primary()
		}
		if b.Clicked() && fn != nil {
			fn()
		}
	}
	switch v.phase {
	case phaseAvailable:
		btn(t.Skip, a.skip, false)
		btn(t.Later, a.later, false)
		if canInstall() {
			btn(t.Install, a.install, true)
		}
	case phaseDownloading:
		btn(t.Cancel, a.cancel, false)
	case phaseReady:
		btn(t.InstallNextLaunch, a.installNextLaunch, false)
		btn(t.RestartNow, a.restartNow, true)
	case phaseDownloaded:
		btn(t.Later, a.later, false)
		btn(t.RestartNow, a.restartNow, true)
	case phaseError:
		btn(t.Close, a.close, false)
		switch {
		case v.unsupported && v.rel != nil && v.rel.Full.URL != "":
			btn(t.OpenDownloadPage, a.openDownloadPage, true)
		case !v.unsupported:
			btn(t.Retry, a.retry, true)
		}
	default:
		btn(t.Close, a.close, true)
	}
}
