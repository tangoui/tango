package tango

// Dragging files out of a surface to other apps (contract: sysx_ext.go,
// platform.FileDragger).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tangoui/tango/internal/platform"
)

// DragOperation is what the drop target did with dragged files.
type DragOperation = platform.DragOperation

const (
	DragNone = platform.DragNone // canceled or refused
	DragCopy = platform.DragCopy
	DragMove = platform.DragMove
	DragLink = platform.DragLink
)

// ErrNoFiles is returned by DragFiles without paths.
var ErrNoFiles = errors.New("tango: DragFiles needs at least one file")

// DragFiles starts a native drag of the files at paths out of the surface,
// to Finder, Explorer, a file manager or any app that accepts files. paths
// must be absolute and exist. icon is an optional PNG drag image (nil: the
// system's file icons). done, if not nil, runs once on the main thread when
// the drop ends, with what the target did (DragNone when canceled).
//
// Call it while the mouse button is held over the surface — from the
// page's mousedown or dragstart handler relayed to Go — since the
// platforms tie a drag to the press that starts it; without a press the
// drag ends at once with DragNone. In the page, call event.preventDefault()
// in dragstart so the WebView does not start its own drag.
//
// Platforms: macOS NSDraggingSession, Windows OLE DoDragDrop (modal: the
// call returns after the drop), Linux GTK drag-and-drop (text/uri-list).
// It returns ErrUnsupported where the surface cannot start a drag (mobile,
// canvas-only surfaces on some platforms).
func (s *Surface) DragFiles(paths []string, icon []byte, done func(DragOperation)) error {
	abs, err := dragPaths(paths)
	if err != nil {
		return err
	}
	var startErr error
	err = capDo(s, func(d platform.FileDragger) {
		finished := false
		startErr = d.StartFileDrag(abs, icon, func(op DragOperation) {
			if finished {
				return
			}
			finished = true
			if done != nil {
				s.app.guard(whereEvent, func() { done(op) })
			}
		})
	})
	if err != nil {
		return err
	}
	return startErr
}

// dragPaths checks the paths for DragFiles and cleans them.
func dragPaths(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, ErrNoFiles
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("tango: DragFiles: %q is not an absolute path", p)
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("tango: DragFiles: %w", err)
		}
		out = append(out, filepath.Clean(p))
	}
	return out, nil
}
