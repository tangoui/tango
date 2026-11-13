package main

import (
	_ "embed"
	"log"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/tray"
	"github.com/tangoui/tango/ui"
)

// Tray icons: a coloured one for Windows and Linux, a black template for
// the macOS menu bar (it follows the light/dark menu bar by itself).
var (
	//go:embed icons/tray.png
	trayPNG []byte
	//go:embed icons/tray-mac.png
	trayMacPNG []byte
)

// setupTray adds the tray icon and makes closing the window hide it to the
// tray instead of quitting. Without a tray (e.g. Linux without
// AppIndicator) the app keeps the normal close-to-quit behaviour.
func (l *Ledger) setupTray(r *ui.Runner) {
	t, err := tray.New(l.app, tray.Options{
		Icon: trayPNG, MacIcon: trayMacPNG, Template: true, Tooltip: l.trayTip(),
	})
	if err != nil {
		log.Printf("tray unavailable (%v): closing the window quits", err)
		return
	}
	l.tray, l.runner = t, r

	m := menu.New()
	m.Add("显示记账本", l.showWindow)
	m.Add("记一笔…", func() {
		l.showWindow()
		l.openEditor(Entry{})
		r.Invalidate()
	})
	m.Separator()
	m.Add("退出", func() {
		l.quitting = true
		l.app.Quit()
	})
	if err := t.SetMenu(m); err != nil {
		log.Print(err)
	}
	t.OnClick(l.showWindow)       // Windows: left click (the menu is on the right button)
	t.OnDoubleClick(l.showWindow) // Windows: double click

	l.win.OnClose(func() bool {
		if l.quitting {
			return true
		}
		l.win.Hide() // keep running in the tray
		return false
	})
	// macOS: clicking the Dock icon brings the hidden window back.
	l.app.OnEventKind(tango.EventReopen, func(tango.AppEvent) { l.showWindow() })
}

func (l *Ledger) showWindow() {
	if l.win == nil {
		return
	}
	l.win.Show()
	l.win.Focus()
}

// trayTip summarises the current month for the tray tooltip.
func (l *Ledger) trayTip() string {
	month := time.Now().Format("2006-01")
	var in, out int64
	for _, e := range l.store.Entries {
		if e.Month() != month {
			continue
		}
		if e.Kind == Income {
			in += e.Cents
		} else {
			out += e.Cents
		}
	}
	return "记账本 · 本月支出 " + money(out) + " · 结余 " + money(in-out)
}

// refreshTray updates the tooltip after the data changed.
func (l *Ledger) refreshTray() {
	if l.tray != nil {
		l.tray.SetTooltip(l.trayTip())
	}
}
