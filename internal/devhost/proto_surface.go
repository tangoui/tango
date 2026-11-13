package devhost

import "encoding/gob"

// Window calls and events (the worker's stand-in tango.Surface forwards to
// the host window) and menus (sys/menu in the worker shows on the host).

// Rect is a window rectangle in logical pixels (tango.Rect).
type Rect struct{ X, Y, W, H int }

// Appearance is the serializable tango.Appearance.
type Appearance struct {
	TitleBar    int
	Transparent bool
	Background  uint32
	Material    int
	AlwaysOnTop bool
	Opacity     float64
	NoShadow    bool
	NoClose     bool
	NoMinimize  bool
	NoMaximize  bool
}

// SurfaceOp kinds.
const (
	OpGet = iota // only answer SurfaceInfo
	OpShow
	OpHide
	OpFocus
	OpSetTitle
	OpSetSize
	OpSetBounds
	OpCenter
	OpSetSizeLimits
	OpSetState
	OpSetAppearance
)

// SurfaceOp is a tango.Surface call on the window Win (worker → host).
// With Req != 0 the host answers SurfaceInfo after applying it. Calls from
// a worker that is not on screen yet are applied when it takes over.
type SurfaceOp struct {
	Req        int
	Win        int
	Op         int
	Title      string     // OpSetTitle
	Rect       Rect       // OpSetSize (W, H), OpSetBounds
	Limits     [4]int     // OpSetSizeLimits: minW, minH, maxW, maxH
	State      int        // OpSetState
	Appearance Appearance // OpSetAppearance
}

// SurfaceInfo answers SurfaceOp: the host window's current state.
type SurfaceInfo struct {
	Req     int
	Err     string
	Title   string
	Bounds  Rect // tango.Surface.Bounds
	W, H    int  // tango.Surface.Size
	State   int
	Visible bool
	Focused bool
}

// SurfaceEvent kinds.
const (
	EvResized = iota + 1
	EvMoved
	EvStateChanged
	EvFocused
	EvBlurred
)

// SurfaceEvent reports a change of the host window (host → worker).
type SurfaceEvent struct {
	Win   int
	Kind  int
	X, Y  int // EvMoved
	W, H  int // EvResized
	State int // EvStateChanged
}

// CloseRequest says the user tried to close the host window (host →
// worker). The worker asks its OnClose handlers and answers CloseWindow
// if the window may close.
type CloseRequest struct{ Win int }

// MenuItem is the serializable platform.MenuItem.
type MenuItem struct {
	ID          uint32
	Label       string
	Accelerator string
	Role        string
	Separator   bool
	Disabled    bool
	Checked     bool
	Checkable   bool
	RadioGroup  string
	Icon        []byte
	Submenu     []MenuItem
}

// SetMenu shows a menu on the host (worker → host): the application menu
// when Win is 0, else the window menu of Win. Clear removes it (the host
// goes back to its default menu).
type SetMenu struct {
	Win   int
	Items []MenuItem
	Clear bool
}

// MenuClick reports a click on item ID of a menu set with SetMenu (host →
// worker).
type MenuClick struct {
	Win int
	ID  uint32
}

func init() {
	for _, m := range []any{SurfaceOp{}, SurfaceInfo{}, SurfaceEvent{}, CloseRequest{}, SetMenu{}, MenuClick{}} {
		gob.Register(m)
	}
}
