//go:build linux && !android

// Menu bar placement (a GtkMenuBar packed first in the window's vertical
// box, a fresh GtkAccelGroup per install) follows MyGo's
// internal/linux/menu.go (https://github.com/egoist/mygo, MIT License,
// Copyright (c) 2026 MyGo contributors).

package linux

import "github.com/tangoui/tango/internal/platform"

const gtkAccelVisible = 1

type appMenu struct {
	items   []platform.MenuItem
	onClick func(uint32)
}

// SetAppMenu installs a GtkMenuBar at the top of the surface's window
// (parent != 0) or of every current and future window (parent == 0).
func (b *backend) SetAppMenu(parent platform.SurfaceID, items []platform.MenuItem, onClick func(uint32)) {
	m := &appMenu{items: items, onClick: onClick}
	if parent == 0 {
		b.appMenu = m
		for _, s := range b.surfaces {
			s.installMenu(m)
		}
		return
	}
	if s := b.surfaces[parent]; s != nil {
		s.installMenu(m)
	}
}

func (s *surface) installMenu(m *appMenu) {
	if s.closed {
		return
	}
	if s.menubar != 0 {
		gtkWidgetDestroy(s.menubar) // disconnects the items' handlers
		s.menubar = 0
		// Accelerators live in the group: start with a fresh one.
		gtkWindowRemoveAccelGroup(s.win, s.accel)
		gObjectUnref(s.accel)
		s.accel = gtkAccelGroupNew()
		gtkWindowAddAccelGroup(s.win, s.accel)
	}
	entries := buildMenuEntries(m.items, s.b.opts.Name)
	if len(entries) == 0 {
		return
	}
	s.menubar = gtkMenuBarNew()
	mb := menuBuilder{accel: s.accel, activate: func(e menuEntry) { s.menuActivate(e, m) }}
	mb.fill(s.menubar, entries) // menubuild_linux.go
	gtkBoxPackStart(s.box, s.menubar, false, false, 0)
	gtkBoxReorderChild(s.box, s.menubar, 0)
	gtkWidgetShowAll(s.menubar)
}

func (s *surface) menuActivate(e menuEntry, m *appMenu) {
	if s.closed {
		return
	}
	switch e.Action {
	case actClick:
		if e.ID != 0 && m.onClick != nil {
			m.onClick(e.ID)
		}
	case actQuit:
		s.b.userQuit() // asks HostEx.ShouldQuit; appevents_linux.go
	case actAbout:
		s.b.showAbout(s.win)
	case actEdit:
		if s.canvas != nil {
			s.canvas.editCommand(e.Command) // canvas_linux.go
		} else if s.web != 0 {
			webkitWebViewExecuteEditingCommand(s.web, e.Command)
		}
	case actMinimize:
		gtkWindowIconify(s.win)
	case actClose:
		gtkWindowClose(s.win) // delete-event -> Host.ShouldClose
	}
}

func (b *backend) showAbout(parent ptr) {
	d := gtkAboutDialogNew()
	name := b.opts.Name
	if name == "" {
		name = b.opts.ID
	}
	gtkAboutDialogSetProgramName(d, name)
	if parent != 0 {
		gtkWindowSetTransientFor(d, parent)
	}
	connect(d, "response", 1, func(_, _ ptr) ptr {
		gtkWidgetDestroy(d)
		return 0
	})
	gtkWindowPresent(d)
}
