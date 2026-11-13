package linux

// Platform-independent helpers for the window extensions (ext.go: Geometry,
// Stateful, Styled, Dragger, file drops). Kept free of build tags so they are
// unit-tested on any host; the GTK side is surface_ext_linux.go.

import (
	"net/url"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// GdkWindowState flags (gdk/gdkevents.h).
const (
	gdkStateIconified  uint32 = 1 << 1
	gdkStateMaximized  uint32 = 1 << 2
	gdkStateFullscreen uint32 = 1 << 4
)

// stateFromGdk derives the contract state from GdkWindowState flags. A
// minimized window reports Minimized even if it is also maximized.
func stateFromGdk(flags uint32) platform.WindowState {
	switch {
	case flags&gdkStateIconified != 0:
		return platform.StateMinimized
	case flags&gdkStateFullscreen != 0:
		return platform.StateFullscreen
	case flags&gdkStateMaximized != 0:
		return platform.StateMaximized
	}
	return platform.StateNormal
}

// stateOp is one gtk_window_* state call.
type stateOp int

const (
	opDeiconify stateOp = iota + 1
	opUnfullscreen
	opUnmaximize
	opMaximize
	opFullscreen
	opIconify
)

// stateOps returns the GTK calls, in order, that move a window whose current
// GdkWindowState is cur to want.
func stateOps(cur uint32, want platform.WindowState) []stateOp {
	icon := cur&gdkStateIconified != 0
	full := cur&gdkStateFullscreen != 0
	maxi := cur&gdkStateMaximized != 0
	var ops []stateOp
	switch want {
	case platform.StateMinimized:
		if !icon {
			ops = append(ops, opIconify)
		}
	case platform.StateMaximized:
		if icon {
			ops = append(ops, opDeiconify)
		}
		if full {
			ops = append(ops, opUnfullscreen)
		}
		if !maxi {
			ops = append(ops, opMaximize)
		}
	case platform.StateFullscreen:
		if icon {
			ops = append(ops, opDeiconify)
		}
		if !full {
			ops = append(ops, opFullscreen)
		}
	default: // StateNormal
		if icon {
			ops = append(ops, opDeiconify)
		}
		if full {
			ops = append(ops, opUnfullscreen)
		}
		if maxi {
			ops = append(ops, opUnmaximize)
		}
	}
	return ops
}

// GdkWindowEdge values (gdk/gdkwindow.h).
const (
	gdkEdgeNorthWest int32 = iota
	gdkEdgeNorth
	gdkEdgeNorthEast
	gdkEdgeWest
	gdkEdgeEast
	gdkEdgeSouthWest
	gdkEdgeSouth
	gdkEdgeSouthEast
)

// resizeEdge maps a Dragger edge name to a GdkWindowEdge.
func resizeEdge(edge string) (int32, bool) {
	switch strings.ToLower(edge) {
	case "n":
		return gdkEdgeNorth, true
	case "s":
		return gdkEdgeSouth, true
	case "e":
		return gdkEdgeEast, true
	case "w":
		return gdkEdgeWest, true
	case "ne":
		return gdkEdgeNorthEast, true
	case "nw":
		return gdkEdgeNorthWest, true
	case "se":
		return gdkEdgeSouthEast, true
	case "sw":
		return gdkEdgeSouthWest, true
	}
	return 0, false
}

// gdkGeometry mirrors GdkGeometry (8 gint, 2 gdouble, GdkGravity; 56 bytes
// on LP64).
type gdkGeometry struct {
	MinWidth, MinHeight   int32
	MaxWidth, MaxHeight   int32
	BaseWidth, BaseHeight int32
	WidthInc, HeightInc   int32
	MinAspect, MaxAspect  float64
	WinGravity            int32
	_                     int32
}

// GdkWindowHints bits.
const (
	gdkHintMinSize uint32 = 1 << 1
	gdkHintMaxSize uint32 = 1 << 2
)

// noLimit stands in for "unbounded" in a max-size hint whose other axis is
// limited (GDK has no per-axis mask).
const noLimit = 1<<15 - 1

// geometryHints builds the GdkGeometry and mask for size limits (0 = none).
func geometryHints(minW, minH, maxW, maxH int) (gdkGeometry, uint32) {
	var g gdkGeometry
	var mask uint32
	if minW > 0 || minH > 0 {
		mask |= gdkHintMinSize
		g.MinWidth, g.MinHeight = int32(max(minW, 0)), int32(max(minH, 0))
	}
	if maxW > 0 || maxH > 0 {
		mask |= gdkHintMaxSize
		g.MaxWidth, g.MaxHeight = noLimit, noLimit
		if maxW > 0 {
			g.MaxWidth = int32(max(maxW, minW))
		}
		if maxH > 0 {
			g.MaxHeight = int32(max(maxH, minH))
		}
	}
	return g, mask
}

// rgbaColor converts 0xRRGGBBAA to GdkRGBA components.
func rgbaColor(c uint32) [4]float64 {
	return [4]float64{
		float64(c>>24&0xff) / 255,
		float64(c>>16&0xff) / 255,
		float64(c>>8&0xff) / 255,
		float64(c&0xff) / 255,
	}
}

// fileURIToPath converts a file:// URI to a local path. Only local files
// qualify (empty host or "localhost"); other schemes and remote hosts
// return false. Used when g_filename_from_uri is unavailable.
func fileURIToPath(uri string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(uri))
	if err != nil || !strings.EqualFold(u.Scheme, "file") {
		return "", false
	}
	if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
		return "", false
	}
	if u.Path == "" || !strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	return u.Path, true
}

// extHandler is the Go side of a signal with more than two arguments. args
// holds the C arguments before user_data (unused ones are 0).
type extHandler func(args [7]uintptr) uintptr

// extTable is dispatchTable for extHandler: the fixed many-argument
// trampolines look handlers up by the user_data id. Ids start at 1 and are
// never reused.
type extTable struct {
	mu   sync.Mutex
	next uintptr
	m    map[uintptr]extHandler
}

func newExtTable() *extTable { return &extTable{m: map[uintptr]extHandler{}} }

func (t *extTable) add(h extHandler) uintptr {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.next++
	t.m[t.next] = h
	return t.next
}

func (t *extTable) call(id uintptr, args [7]uintptr) uintptr {
	t.mu.Lock()
	h := t.m[id]
	t.mu.Unlock()
	if h == nil {
		return 0
	}
	return h(args)
}

func (t *extTable) remove(id uintptr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.m, id)
}

func (t *extTable) size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.m)
}

// screenOffset is where the contract's origin — the top-left corner of the
// primary display — lies in GDK's coordinate space, whose origin is the
// top-left corner of the whole screen layout (the bounding box of all
// monitors). Contract coordinates are GDK coordinates minus the offset.
type screenOffset struct{ X, Y int }

// primaryOffset picks the origin of the primary monitor from the monitor
// geometries in GDK order; primary < 0 (no primary monitor, common on
// Wayland) or out of range falls back to monitor 0, no monitors to zero.
func primaryOffset(geos []platform.Rect, primary int) screenOffset {
	if primary < 0 || primary >= len(geos) {
		primary = 0
	}
	if len(geos) == 0 {
		return screenOffset{}
	}
	return screenOffset{geos[primary].X, geos[primary].Y}
}

// toContract converts a GDK position to contract coordinates.
func (o screenOffset) toContract(x, y int) (int, int) { return x - o.X, y - o.Y }

// toGdk converts a contract position to GDK coordinates.
func (o screenOffset) toGdk(x, y int) (int, int) { return x + o.X, y + o.Y }

// rect converts a GDK rectangle to contract coordinates (sizes unchanged).
func (o screenOffset) rect(r platform.Rect) platform.Rect {
	r.X, r.Y = o.toContract(r.X, r.Y)
	return r
}
