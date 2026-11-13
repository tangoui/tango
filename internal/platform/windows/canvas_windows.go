//go:build windows && (amd64 || arm64)

package windows

// Canvas child window (docs/specs/canvas-surface.md, step B): a child HWND
// of class "TangoCanvas" filling a parent's client area, whose content is
// drawn by Tango's own UI. Frames arrive in Present as premultiplied RGBA,
// are copied into a 32-bit DIB section (BGRA) and blitted in WM_PAINT.
// Input handling is in canvas_input_windows.go, pure mappings in
// canvas_keys.go.
//
// Frame scheduling. Everything that wants a frame ends in requestFrame on
// the UI (main) thread, which allows at most one frame per display
// refresh:
//
//   - Invalidate / InvalidateAt may be called from any goroutine. They
//     record the request under c.mu and post one wmCanvasWake to the
//     window (further calls coalesce until it is handled). Posted messages
//     are also dispatched by the modal loops of window sizing and menus.
//   - requestFrame paints now (RedrawWindow with RDW_UPDATENOW, so
//     WM_PAINT runs synchronously) when at least one refresh period has
//     passed since the last frame; otherwise it sets a throttle deadline
//     at lastFrame + period. DwmFlush would align to vsync but blocks the
//     thread, so pacing follows the monitor's refresh rate instead
//     (EnumDisplaySettings); DWM composes the result tear-free.
//   - Deadlines (InvalidateAt and the throttle) share one time.Timer per
//     canvas, re-armed to the earliest; when it fires it posts
//     wmCanvasTick. A Go timer is used instead of SetTimer because
//     USER_TIMER_MINIMUM (10 ms) and the default 15.6 ms tick would round
//     a 16.7 ms frame interval to 31 ms. A still window has no timer
//     armed. Any frame satisfies a pending InvalidateAt deadline.
//   - WM_PAINT calls CanvasFrame only when a frame is wanted, the DIB
//     does not exist yet or the geometry changed; a plain expose just
//     blits the last frame. After UseGPUWindow (canvas_gpu_windows.go) it
//     never blits and runs only a frame that is due.
//   - Size and DPI changes (WM_SIZE, WM_DPICHANGED_AFTERPARENT) report
//     CanvasResized and redraw synchronously, so live resize shows no
//     stale or stretched content.
//
// The sink is never called before start (the canvas is created inside
// the backend's window creation) nor after destroy.
//
// Transparency (Appearance.Transparent or a Material on a canvas surface).
// The surface already extends the DWM frame over the whole client area
// for such appearances ("sheet of glass", frameMargins), which is what
// makes a transparent WebView2 see-through: DWM then composes the
// window's redirection surface with its alpha channel. The canvas child
// draws into that surface (a child without WS_EX_LAYERED shares its
// parent's), and BitBlt SRCCOPY of the 32-bit DIB copies all four bytes,
// so the premultiplied BGRA frame from Present keeps its per-pixel alpha
// on screen; what the frame does not cover is filled with transparent
// black instead of the frame's corner colour (setTransparent). This was
// chosen over WS_EX_LAYERED + UpdateLayeredWindow: a layered child only
// blends with its parent (and needs Windows 8 and a manifest), and
// UpdateLayeredWindow on the top-level window would replace the caption
// and frame, stop child windows from drawing and conflict with
// Appearance.Opacity (SetLayeredWindowAttributes). Pointer input is not
// alpha-tested: transparent pixels still belong to the window, as for a
// transparent WebView surface.
//
// Transparency of a CanvasLayer below a WebView is the attached canvas's
// business (canvas_attach_windows.go).

import (
	"errors"
	"fmt"
	"image"
	"math"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var _ platform.Canvas = (*canvas)(nil)

const canvasClass = "TangoCanvas"

// Private messages of the canvas window class (WM_USER range: the class
// is ours, so they cannot clash with another class's meaning).
const (
	wmCanvasWake = 0x0400 + 0x7A1 // queued Invalidate/InvalidateAt/focus requests
	wmCanvasTick = 0x0400 + 0x7A2 // the deadline timer fired
)

const (
	canvasWmDestroy               = 0x0002
	canvasWmSize                  = 0x0005
	canvasWmPaint                 = 0x000F
	canvasWmEraseBkgnd            = 0x0014
	canvasWmNCDestroy             = 0x0082
	canvasWmDpiChangedAfterParent = 0x02E3

	canvasWsChild        = 0x40000000
	canvasWsVisible      = 0x10000000
	canvasWsClipSiblings = 0x04000000

	canvasRdwInvalidate = 0x0001
	canvasRdwUpdateNow  = 0x0100

	canvasSrcCopy       = 0x00CC0020
	canvasDIBRGBColors  = 0
	canvasColorWindow   = 5
	canvasEnumCurrent   = 0xFFFFFFFF // ENUM_CURRENT_SETTINGS
	canvasChildID       = 0x7A10     // control ID of the child window
	canvasTimerSlack    = time.Millisecond
	canvasRateRecheck   = 5 * time.Second
	canvasDefaultPeriod = time.Second / 60
)

var errCanvasClosed = errors.New("tango/windows: canvas is closed")

var (
	canvasGDI32 = systemDLL("gdi32.dll")
	canvasIMM32 = systemDLL("imm32.dll")

	canvasBeginPaint          = user32.NewProc("BeginPaint")
	canvasEndPaint            = user32.NewProc("EndPaint")
	canvasInvalidateRect      = user32.NewProc("InvalidateRect")
	canvasRedrawWindow        = user32.NewProc("RedrawWindow")
	canvasFillRect            = user32.NewProc("FillRect")
	canvasGetSysColorBrush    = user32.NewProc("GetSysColorBrush")
	canvasIsIconic            = user32.NewProc("IsIconic")
	canvasEnumDisplaySettings = user32.NewProc("EnumDisplaySettingsW")
	canvasGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
	canvasMonitorFromWindow   = user32.NewProc("MonitorFromWindow")
	canvasGetFocus            = user32.NewProc("GetFocus")
	canvasSetFocus            = user32.NewProc("SetFocus")

	canvasCreateDIBSection   = canvasGDI32.NewProc("CreateDIBSection")
	canvasCreateCompatibleDC = canvasGDI32.NewProc("CreateCompatibleDC")
	canvasSelectObject       = canvasGDI32.NewProc("SelectObject")
	canvasDeleteObject       = canvasGDI32.NewProc("DeleteObject")
	canvasDeleteDC           = canvasGDI32.NewProc("DeleteDC")
	canvasBitBlt             = canvasGDI32.NewProc("BitBlt")
	canvasCreateSolidBrush   = canvasGDI32.NewProc("CreateSolidBrush")
	canvasGdiFlush           = canvasGDI32.NewProc("GdiFlush")
)

// canvasPaintStruct is PAINTSTRUCT.
type canvasPaintStruct struct {
	HDC       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

// canvasBitmapInfo is BITMAPINFO with a BITMAPINFOHEADER and one (unused)
// color entry.
type canvasBitmapInfo struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	Colors        [1]uint32
}

// canvasMonitorInfoEx is MONITORINFOEXW.
type canvasMonitorInfoEx struct {
	monitorInfo
	Device [32]uint16
}

// canvasDevMode is DEVMODEW (display fields), 220 bytes.
type canvasDevMode struct {
	DeviceName       [32]uint16
	SpecVersion      uint16
	DriverVersion    uint16
	Size             uint16
	DriverExtra      uint16
	Fields           uint32
	Display          [16]byte // dmPosition, dmDisplayOrientation, dmDisplayFixedOutput
	Color            int16
	Duplex           int16
	YResolution      int16
	TTOption         int16
	Collate          int16
	FormName         [32]uint16
	LogPixels        uint16
	BitsPerPel       uint32
	PelsWidth        uint32
	PelsHeight       uint32
	DisplayFlags     uint32
	DisplayFrequency uint32
	ICM              [8]uint32 // dmICMMethod … dmPanningHeight
}

var (
	_ = [1]struct{}{}[unsafe.Sizeof(canvasDevMode{})-220]
	_ = [1]struct{}{}[unsafe.Offsetof(canvasDevMode{}.DisplayFrequency)-184]
	_ = [1]struct{}{}[unsafe.Sizeof(canvasMonitorInfoEx{})-104]
	_ = [1]struct{}{}[unsafe.Sizeof(canvasBitmapInfo{})-44]
)

// canvas is a canvas child window; it implements platform.Canvas. Fields
// are UI-thread only unless noted.
type canvas struct {
	hwnd   uintptr // set once during creation; read by the timer goroutine
	parent uintptr
	sink   platform.CanvasSink

	ready     bool // start ran: sink callbacks allowed
	closed    bool
	sinkDepth int  // inside a sink callback
	inPaint   bool // inside WM_PAINT

	pxW, pxH int32   // client size, physical pixels
	w, h     float64 // logical size last reported
	scale    float64
	dpi      int

	// Drawing.
	dib         uintptr // HBITMAP (DIB section) holding the last frame
	dibBits     uintptr
	dibW, dibH  int
	memDC       uintptr
	oldBitmap   uintptr
	bg          uint32 // COLORREF filling what the frame does not cover
	hasBG       bool
	transparent bool         // per-pixel alpha shows through (setTransparent)
	dmg         canvasDamage // partial frames (canvas_damage_windows.go)

	// GPU presentation (canvas_gpu_windows.go).
	gpu     canvasGPU
	sibling uintptr // CPU-fallback window above the canvas once switched

	// Frame scheduling.
	frameWanted bool
	lastFrame   time.Time
	timerAt     time.Time // pending InvalidateAt deadline
	throttleAt  time.Time // next frame allowed (refresh pacing)
	armedAt     time.Time // what the timer is set to
	timer       *time.Timer
	period      time.Duration
	periodMon   uintptr
	periodAt    time.Time

	// Pointer (canvas_input_windows.go).
	cursor     input.Cursor
	hcursor    uintptr
	inside     bool // pointer over the client area; TME_LEAVE requested
	leavePend  bool // left while captured: report Leave on release
	buttons    uint8
	clicks     canvasClickCounter
	lastClicks [3]int
	downKeys   map[input.Key]bool
	chars      canvasCharDecoder
	altGr      bool          // AltGr held: its synthesized left Ctrl was dropped
	touches    canvasTouches // fingers down (canvas_touch_windows.go)

	// File drop (canvas_drop_windows.go).
	dropTarget uintptr // our IDropTarget while registered
	dropHover  bool    // CanvasDragOver reported, CanvasDragLeave pending

	// Focus.
	hasFocus bool // the window has keyboard focus
	focused  bool // last value reported to the sink

	// Text input.
	textOn bool
	caret  geom.Rect
	marked string // IME composition string last reported

	// Requests from any goroutine.
	mu      sync.Mutex
	qPosted bool
	qFrame  bool
	qFocus  bool
	qAt     time.Time
	qClosed bool

	ax *canvasA11y // UI Automation, nil until a client asks (canvas_a11y_windows.go)
}

// Canvas windows by HWND; the one being created by CreateWindowExW.
var (
	canvasByHwnd   = map[uintptr]*canvas{}
	canvasCreating *canvas

	canvasClassOnce sync.Once
	canvasClassErr  error
	canvasProcCB    uintptr
)

func registerCanvasClass() error {
	canvasClassOnce.Do(func() {
		canvasProcCB = syscall.NewCallback(canvasWndProc)
		wc := wndClassEx{
			WndProc:   canvasProcCB,
			Instance:  instance(),
			ClassName: u16(canvasClass),
			// No class cursor (WM_SETCURSOR sets it) and no background
			// brush (WM_PAINT covers everything; WM_ERASEBKGND is eaten).
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			canvasClassErr = fmt.Errorf("tango: RegisterClassExW(%s): %v", canvasClass, err)
		}
	})
	return canvasClassErr
}

// newCanvasChild creates a canvas child window filling the client area of
// parent. UI thread. The canvas does not call sink until start; the caller
// finishes setting up the surface first (see the step-B notes).
func newCanvasChild(parent uintptr, sink platform.CanvasSink) (*canvas, error) {
	if sink == nil {
		return nil, errors.New("tango/windows: canvas: nil sink")
	}
	if parent == 0 {
		return nil, errors.New("tango/windows: canvas: no parent window")
	}
	if err := registerCanvasClass(); err != nil {
		return nil, err
	}
	c := &canvas{parent: parent, sink: sink, downKeys: map[input.Key]bool{}, scale: 1, dpi: baseDPI}
	var pr rect
	procGetClientRect.Call(parent, uintptr(unsafe.Pointer(&pr)))
	c.SetCursor(input.CursorDefault) // WM_SETCURSOR may come before CreateWindowExW returns
	canvasCreating = c
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(canvasClass))), 0,
		canvasWsChild|canvasWsVisible|canvasWsClipSiblings,
		0, 0, uintptr(pr.Right-pr.Left), uintptr(pr.Bottom-pr.Top),
		parent, canvasChildID, instance(), 0)
	canvasCreating = nil
	if hwnd == 0 {
		if c.hwnd != 0 {
			delete(canvasByHwnd, c.hwnd)
		}
		return nil, fmt.Errorf("tango: CreateWindowExW(%s): %v", canvasClass, err)
	}
	c.hwnd = hwnd
	canvasByHwnd[hwnd] = c
	c.disableIME()
	c.readGeometry()
	return c, nil
}

// start allows sink callbacks and asks for the first frame and a focus
// report. UI thread; called once the surface is set up (the end of
// NewCanvasSurface). The first CanvasFrame comes from WM_PAINT once the
// window is visible, never from inside start.
func (c *canvas) start() {
	if c.closed || c.ready {
		return
	}
	c.readGeometry() // silently: the first CanvasFrame carries the size
	c.ready = true
	c.frameWanted = true
	canvasInvalidateRect.Call(c.hwnd, 0, 0)
	c.enqueue(func() { c.qFocus = true })
}

// fitParent resizes the canvas to the parent's client area. Call from the
// parent's WM_SIZE (not when minimized) and after WM_DPICHANGED.
func (c *canvas) fitParent() {
	if c.closed {
		return
	}
	if r, _, _ := canvasIsIconic.Call(c.parent); r != 0 {
		return
	}
	var pr rect
	procGetClientRect.Call(c.parent, uintptr(unsafe.Pointer(&pr)))
	w, h := pr.Right-pr.Left, pr.Bottom-pr.Top
	if w <= 0 || h <= 0 {
		return
	}
	procSetWindowPos.Call(c.hwnd, 0, 0, 0, uintptr(w), uintptr(h), swpNoZOrder|swpNoActivate)
	if c.gpu.on && c.frameWanted && !c.closed {
		c.requestFrame() // restored: run a frame held back while minimized (gpuFrameNow)
	}
}

// focus gives the canvas keyboard focus. Call from the parent's
// WM_SETFOCUS and from Surface.Focus.
func (c *canvas) focus() {
	if !c.closed {
		canvasSetFocus.Call(c.hwnd)
	}
}

// ---- geometry ----

// readGeometry updates the size and scale without reporting them.
func (c *canvas) readGeometry() {
	var r rect
	procGetClientRect.Call(c.hwnd, uintptr(unsafe.Pointer(&r)))
	c.pxW, c.pxH = r.Right-r.Left, r.Bottom-r.Top
	c.dpi = dpiOf(c.hwnd)
	c.scale = float64(c.dpi) / baseDPI
	c.w, c.h = float64(c.pxW)/c.scale, float64(c.pxH)/c.scale
}

// checkGeometry reports a size or scale change; it returns whether there
// was one.
func (c *canvas) checkGeometry() bool {
	if c.closed {
		return false
	}
	w, h, s := c.w, c.h, c.scale
	c.readGeometry()
	if w == c.w && h == c.h && s == c.scale {
		return false
	}
	if c.ready {
		c.sinkDepth++
		c.sink.CanvasResized(float32(c.w), float32(c.h), float32(c.scale))
		c.sinkDepth--
	}
	return true
}

// geometryChanged handles WM_SIZE and WM_DPICHANGED_AFTERPARENT.
func (c *canvas) geometryChanged(dpi bool) {
	if dpi {
		c.periodMon = 0 // the window may be on another monitor now
	}
	if !c.checkGeometry() || c.closed {
		return
	}
	c.placeIME()
	c.fitSibling() // canvas_gpu_windows.go
	if c.ready {
		c.frameWanted = true
		c.paintNow()
	}
}

// ---- platform.Canvas ----

func (c *canvas) Size() (float32, float32) { return float32(c.w), float32(c.h) }
func (c *canvas) Scale() float32           { return float32(c.scale) }

// enqueue records a request for the UI thread and posts one wake. Any
// goroutine.
func (c *canvas) enqueue(set func()) {
	c.mu.Lock()
	if c.qClosed {
		c.mu.Unlock()
		return
	}
	set()
	post := !c.qPosted
	c.qPosted = true
	c.mu.Unlock()
	if post && !postMessage(c.hwnd, wmCanvasWake, 0, 0) {
		c.mu.Lock() // queue full: the next request posts again
		c.qPosted = false
		c.mu.Unlock()
	}
}

// Invalidate implements platform.Canvas. Any goroutine.
func (c *canvas) Invalidate() { c.enqueue(func() { c.qFrame = true }) }

// InvalidateAt implements platform.Canvas. Any goroutine.
func (c *canvas) InvalidateAt(t time.Time) {
	c.enqueue(func() {
		if c.qAt.IsZero() || t.Before(c.qAt) {
			c.qAt = t
		}
	})
}

// wake handles wmCanvasWake.
func (c *canvas) wake() {
	c.mu.Lock()
	frame, at, focus := c.qFrame, c.qAt, c.qFocus
	c.qPosted, c.qFrame, c.qAt, c.qFocus = false, false, time.Time{}, false
	c.mu.Unlock()
	if c.closed {
		return
	}
	if focus {
		c.reportFocus()
	}
	if !at.IsZero() && (c.timerAt.IsZero() || at.Before(c.timerAt)) {
		c.timerAt = at
		c.armTimer()
	}
	if frame && !c.closed {
		c.requestFrame()
	}
}

// requestFrame asks for a CanvasFrame at the next refresh.
func (c *canvas) requestFrame() {
	if c.closed {
		return
	}
	c.frameWanted = true
	next := c.lastFrame.Add(c.refreshPeriod())
	if !time.Now().Add(canvasTimerSlack).Before(next) {
		c.throttleAt = time.Time{}
		c.armTimer()
		c.paintNow()
		return
	}
	c.throttleAt = next
	c.armTimer()
}

// paintNow repaints synchronously, or schedules WM_PAINT when called from
// inside a sink callback or a paint (no re-entrant frames).
func (c *canvas) paintNow() {
	if a := attachedByCanvas[c]; a != nil && a.layered {
		a.paintLayered() // layered windows are not drawn in WM_PAINT (canvas_attach_windows.go)
		return
	}
	if c.gpu.on {
		c.gpuFrameNow() // not through WM_PAINT: canvas_gpu_windows.go
		return
	}
	if c.inPaint || c.sinkDepth > 0 {
		canvasInvalidateRect.Call(c.hwnd, 0, 0)
		return
	}
	c.notePendingUpdate() // canvas_damage_windows.go: what the system wanted repainted
	canvasRedrawWindow.Call(c.hwnd, 0, 0, canvasRdwInvalidate|canvasRdwUpdateNow)
	c.dmg.own = false
}

// armTimer sets the timer to the earliest pending deadline, or stops it.
func (c *canvas) armTimer() {
	t := c.timerAt
	if !c.throttleAt.IsZero() && (t.IsZero() || c.throttleAt.Before(t)) {
		t = c.throttleAt
	}
	if t.Equal(c.armedAt) && (t.IsZero() || time.Until(t) > 0) {
		return // already armed (a past deadline is re-armed in case its tick was lost)
	}
	c.armedAt = t
	if t.IsZero() {
		if c.timer != nil {
			c.timer.Stop()
		}
		return
	}
	d := max(time.Until(t), 0)
	if c.timer == nil {
		hwnd := c.hwnd
		c.timer = time.AfterFunc(d, func() { postMessage(hwnd, wmCanvasTick, 0, 0) })
		return
	}
	c.timer.Reset(d)
}

// tick handles wmCanvasTick.
func (c *canvas) tick() {
	if c.closed {
		return
	}
	c.armedAt = time.Time{} // the timer has fired (or is stale): re-arm below
	now := time.Now().Add(canvasTimerSlack)
	if !c.throttleAt.IsZero() && !now.Before(c.throttleAt) {
		c.throttleAt = time.Time{}
		if c.frameWanted {
			c.paintNow()
		}
	}
	if !c.closed && !c.timerAt.IsZero() && !now.Before(c.timerAt) {
		c.timerAt = time.Time{}
		c.requestFrame()
	}
	if !c.closed {
		c.armTimer()
	}
}

// refreshPeriod is the frame interval of the window's monitor, cached per
// monitor and re-read now and then (the rate can be changed in Settings).
func (c *canvas) refreshPeriod() time.Duration {
	mon, _, _ := canvasMonitorFromWindow.Call(c.hwnd, monitorDefaultToNearest)
	now := time.Now()
	if c.period != 0 && mon == c.periodMon && now.Sub(c.periodAt) < canvasRateRecheck {
		return c.period
	}
	c.period, c.periodMon, c.periodAt = canvasDefaultPeriod, mon, now
	if mon == 0 {
		return c.period
	}
	mi := canvasMonitorInfoEx{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := canvasGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return c.period
	}
	dm := canvasDevMode{}
	dm.Size = uint16(unsafe.Sizeof(dm))
	if r, _, _ := canvasEnumDisplaySettings.Call(uintptr(unsafe.Pointer(&mi.Device[0])), canvasEnumCurrent, uintptr(unsafe.Pointer(&dm))); r != 0 {
		c.period = canvasRefreshPeriod(dm.DisplayFrequency)
	}
	return c.period
}

// ---- painting ----

// paint handles WM_PAINT: render a frame if one is due, then blit.
func (c *canvas) paint() {
	var ps canvasPaintStruct
	hdc, _, _ := canvasBeginPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	c.inPaint = true
	changed := c.ready && c.checkGeometry()
	if changed {
		c.fitSibling()
	}
	frame, blit := c.gpu.paint(c.ready && !c.closed, changed, c.frameWanted, c.dib != 0) // canvas_gpu_plan.go
	// What the system wants repainted: the paint rectangle, or what was
	// pending before paintNow invalidated everything.
	sys := ps.Paint
	if c.dmg.own {
		sys = c.dmg.sys
	}
	c.dmg.own, c.dmg.partial, c.dmg.rects = false, false, c.dmg.rects[:0]
	if frame && !c.closed {
		c.frame()
	}
	if blit && !c.closed {
		if c.dmg.partial && !changed {
			c.blitDamage(hdc, sys) // canvas_damage_windows.go
		} else {
			c.blit(hdc)
		}
	}
	c.dmg.partial = false
	c.inPaint = false
	canvasEndPaint.Call(c.hwnd, uintptr(unsafe.Pointer(&ps)))
}

// frame calls CanvasFrame; the sink calls Present from inside it.
func (c *canvas) frame() {
	c.frameWanted = false
	c.throttleAt = time.Time{}
	c.timerAt = time.Time{} // this frame satisfies the deadline
	c.armTimer()
	c.lastFrame = time.Now()
	c.sinkDepth++
	c.sink.CanvasFrame()
	c.sinkDepth--
}

// blit copies the last frame to hdc and fills what it does not cover with
// the frame's top-left color (or the system window color before the
// first frame).
func (c *canvas) blit(hdc uintptr) {
	if c.dib != 0 {
		canvasBitBlt.Call(hdc, 0, 0, uintptr(c.dibW), uintptr(c.dibH), c.memDC, 0, 0, canvasSrcCopy)
	}
	w, h := int32(c.dibW), int32(c.dibH)
	if w >= c.pxW && h >= c.pxH {
		return
	}
	var brush uintptr
	if c.transparent {
		brush, _, _ = canvasCreateSolidBrush.Call(0) // GDI writes alpha 0: see-through
		defer canvasDeleteObject.Call(brush)
	} else if c.hasBG {
		brush, _, _ = canvasCreateSolidBrush.Call(uintptr(c.bg))
		defer canvasDeleteObject.Call(brush)
	} else {
		brush, _, _ = canvasGetSysColorBrush.Call(canvasColorWindow)
	}
	if w < c.pxW {
		r := rect{Left: w, Top: 0, Right: c.pxW, Bottom: c.pxH}
		canvasFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
	}
	if h < c.pxH {
		r := rect{Left: 0, Top: h, Right: min(w, c.pxW), Bottom: c.pxH}
		canvasFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
	}
}

// ensureDIB (re)creates the DIB section for a w×h frame.
func (c *canvas) ensureDIB(w, h int) error {
	if c.dib != 0 && c.dibW == w && c.dibH == h {
		return nil
	}
	c.freeDIB()
	if c.memDC == 0 {
		c.memDC, _, _ = canvasCreateCompatibleDC.Call(0)
		if c.memDC == 0 {
			return errors.New("tango/windows: Present: CreateCompatibleDC failed")
		}
	}
	bi := canvasBitmapInfo{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32} // top-down, BI_RGB
	bi.Size = 40                                                                        // BITMAPINFOHEADER
	var bits uintptr
	hbm, _, _ := canvasCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), canvasDIBRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbm == 0 || bits == 0 {
		if hbm != 0 {
			canvasDeleteObject.Call(hbm)
		}
		return errors.New("tango/windows: Present: CreateDIBSection failed")
	}
	c.dib, c.dibBits, c.dibW, c.dibH = hbm, bits, w, h
	c.oldBitmap, _, _ = canvasSelectObject.Call(c.memDC, hbm)
	return nil
}

func (c *canvas) freeDIB() {
	if c.dib == 0 {
		return
	}
	canvasSelectObject.Call(c.memDC, c.oldBitmap)
	canvasDeleteObject.Call(c.dib)
	c.dib, c.dibBits, c.dibW, c.dibH, c.oldBitmap = 0, 0, 0, 0, 0
}

// Present implements platform.Canvas: it copies img into the DIB section
// and arranges for it to be shown (by WM_PAINT, or after UseGPUWindow by
// the sibling window). After UseGPUWindow, Present(nil) means "the GPU
// presented" and hides the sibling.
func (c *canvas) Present(img *image.RGBA) error {
	if c.closed {
		return errCanvasClosed
	}
	if img == nil {
		if !c.gpu.on {
			return errors.New("tango/windows: Present: nil image")
		}
		c.presentGPU(false)
		return nil
	}
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	off := img.PixOffset(r.Min.X, r.Min.Y)
	n := img.Stride*(h-1) + w*4
	if off < 0 || off+n > len(img.Pix) {
		return errors.New("tango/windows: Present: image buffer too small")
	}
	if err := c.ensureDIB(w, h); err != nil {
		return err
	}
	canvasGdiFlush.Call() // GDI may still read the bits from the last BitBlt
	dst := unsafe.Slice((*byte)(native(c.dibBits)), w*h*4)
	src := img.Pix[off : off+n]
	canvasRGBAToBGRA(dst, w*4, src, img.Stride, w, h)
	c.bg, c.hasBG = canvasColorRef(src[0], src[1], src[2]), true
	if c.gpu.on {
		c.presentGPU(true) // never GDI on the swap chain's window
		return nil
	}
	if !c.inPaint {
		canvasInvalidateRect.Call(c.hwnd, 0, 0) // WM_PAINT blits without a new frame
	}
	return nil
}

// setTransparent switches the see-through mode for the surface's
// appearance (Transparent or a Material). UI thread.
func (c *canvas) setTransparent(on bool) {
	if c.closed || c.transparent == on {
		return
	}
	c.transparent = on
	canvasInvalidateRect.Call(c.hwnd, 0, 0) // refill the uncovered area
}

// ---- lifetime ----

// destroy stops all sink callbacks and frees the drawing resources. It is
// idempotent; the backend calls it before reporting SurfaceClosed, and
// WM_DESTROY calls it again.
func (c *canvas) destroy() {
	if c.closed {
		return
	}
	c.closed = true
	c.mu.Lock()
	c.qClosed = true
	c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.disableFileDrop()
	c.a11yDestroy() // canvas_a11y_windows.go
	c.freeDIB()
	if c.memDC != 0 {
		canvasDeleteDC.Call(c.memDC)
		c.memDC = 0
	}
}

// canvasWndProc is the window procedure of the canvas class.
func canvasWndProc(hwnd, m, wp, lp uintptr) uintptr {
	c := canvasByHwnd[hwnd]
	if c == nil {
		if c = canvasCreating; c != nil && c.hwnd == 0 {
			// First messages, inside CreateWindowExW.
			c.hwnd = hwnd
			canvasByHwnd[hwnd] = c
		}
	}
	if c != nil {
		if r, ok := c.message(uint32(m), wp, lp); ok {
			return r
		}
		if uint32(m) == canvasWmNCDestroy {
			delete(canvasByHwnd, hwnd)
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// message handles a canvas window message; ok false passes it to
// DefWindowProc.
func (c *canvas) message(m uint32, wp, lp uintptr) (uintptr, bool) {
	if a := attachedByCanvas[c]; a != nil { // canvas_attach_windows.go
		if r, ok := a.message(m, wp, lp); ok {
			return r, true
		}
	}
	switch m {
	case wmCanvasWake:
		c.wake()
		return 0, true
	case wmCanvasTick:
		c.tick()
		return 0, true
	case canvasWmPaint:
		c.paint()
		return 0, true
	case canvasWmEraseBkgnd:
		return 1, true // WM_PAINT covers the whole client area
	case canvasWmSize:
		c.geometryChanged(false)
		return 0, true
	case canvasWmDpiChangedAfterParent:
		c.geometryChanged(true)
		return 0, true
	case canvasWmDestroy:
		c.destroy()
		return 0, true
	case canvasWmGetObject, wmCanvasA11y: // UI Automation (canvas_a11y_windows.go)
		if r, ok := c.a11yMessage(m, wp, lp); ok {
			return r, true
		}
	}
	if c.closed {
		return 0, false
	}
	return c.inputMessage(m, wp, lp)
}

// ---- helpers shared with canvas_input_windows.go ----

// toLogical converts physical client pixels to logical pixels.
func (c *canvas) toLogical(x, y int32) geom.Point {
	return canvasPhysToLogical(x, y, c.scale) // canvas_touch.go
}

// toPhysical converts a logical coordinate to physical pixels.
func (c *canvas) toPhysical(v float32) int32 { return int32(math.Round(float64(v) * c.scale)) }
