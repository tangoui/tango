//go:build windows && (amd64 || arm64)

package windows

// platform.CanvasCreator: a normal Tango window (same creation, extras,
// events and optional interfaces as a WebView surface) whose client area
// is filled by a canvas child window (canvas_windows.go) instead of
// WebView2. The surface keeps the child sized (WM_SIZE → fitParent),
// forwards keyboard focus to it (WM_SETFOCUS, Focus) and stops it before
// reporting SurfaceClosed; WebView-only methods do nothing (whenReady).

import (
	"errors"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var _ platform.CanvasCreator = (*Backend)(nil)

// NewCanvasSurface implements platform.CanvasCreator. Main thread. The
// sink is not called before it returns: the first CanvasFrame comes from
// WM_PAINT and focus is reported from a posted message.
func (b *Backend) NewCanvasSurface(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, sink platform.CanvasSink) (platform.Surface, platform.Canvas, error) {
	if sink == nil {
		return nil, nil, errors.New("tango/windows: NewCanvasSurface: nil sink")
	}
	s, err := b.newWindow(id, opts, &ex)
	if err != nil {
		return nil, nil, err
	}
	c, err := newCanvasChild(s.hwnd, sink)
	if err != nil {
		s.Close() // not created yet: no SurfaceClosed
		return nil, nil, err
	}
	s.canvas = c
	c.setTransparent(canvasSeeThrough(s.ext.app))
	if s.ext.acceptFiles {
		c.enableFileDrop(func(paths []string, x, y int) {
			s.event(platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: paths, X: x, Y: y})
		})
	}
	c.fitParent() // extras may have resized the window since the child was made
	s.finishSurface()
	c.start() // no-op if the window was already closed

	return s, c, nil
}

// editRole runs an Edit menu role (undo, redo, cut, copy, paste,
// selectAll) as the matching Ctrl+key press and release, as the macOS
// canvas reports Edit menu actions as Cmd+key.
func (c *canvas) editRole(role string) {
	k, m, ok := canvasRoleKey(role)
	if !ok || c.closed {
		return
	}
	c.input(input.KeyEvent{Key: k, Down: true, Mods: m})
	c.input(input.KeyEvent{Key: k, Down: false, Mods: m})
}
