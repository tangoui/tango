//go:build darwin && !ios

package darwin

// Window capabilities (contract round 2, platform/ext.go): CreatorEx,
// Geometry, Stateful, Styled, PageControl, Dragger and the Navigating /
// LoadFailed / Moved / StateChanged / FilesDropped events.
//
// Design notes (approach informed by Wails v3's macOS window code, MIT,
// Copyright (c) Lea Anthony and contributors; no code copied):
//   - TitleBarNone keeps a titled, resizable window with a full-size content
//     view, a transparent title bar and the standard buttons hidden, so the
//     window keeps native edge resizing, rounded corners and its shadow.
//   - performWindowDragWithEvent: must be given the original mouse-down: by
//     the time the page's drag request arrives, [NSApp currentEvent] has
//     moved on. A local event monitor remembers the last left mouse-down.
//   - Dropped files are taken in a WKWebView subclass's NSDraggingDestination
//     methods when AcceptFiles is set, before WebKit would open them.

import (
	"math"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

const (
	nsWindowStyleMaskUtilityWindow      = 1 << 4
	nsWindowStyleMaskNonactivatingPanel = 1 << 7
	nsWindowStyleMaskFullScreen         = 1 << 14
	nsWindowStyleMaskFullSizeContent    = 1 << 15

	nsWindowCloseButton       = 0
	nsWindowMiniaturizeButton = 1
	nsWindowZoomButton        = 2

	nsWindowTitleVisible = 0
	nsWindowTitleHidden  = 1

	nsNormalWindowLevel   = 0
	nsFloatingWindowLevel = 3

	nsWindowCollectionBehaviorFullScreenPrimary   = 1 << 7
	nsWindowCollectionBehaviorFullScreenAuxiliary = 1 << 8

	nsVisualEffectMaterialWindowBackground      = 12
	nsVisualEffectMaterialUnderWindowBackground = 21
	nsVisualEffectBlendingModeBehindWindow      = 0
	nsVisualEffectStateActive                   = 1
	nsWindowBelowArg                            = ^uintptr(0) // NSWindowBelow (-1)

	nsEventMaskLeftMouseDown = 1 << 1
	nsDragOperationNone      = 0
	nsDragOperationCopy      = 1

	nsURLErrorCancelled = -999
)

// surfaceExt is a surface's round-2 state. Main thread.
type surfaceExt struct {
	resizable   bool
	panel       bool
	acceptFiles bool
	appearance  platform.Appearance
	effect      id // NSVisualEffectView, when a Material is set
	sheetParent id // NSWindow this sheet attaches to (KindSheet / KindModal)
	initState   platform.WindowState
	lastState   platform.WindowState
	lastX       int
	lastY       int
	moved       bool // lastX/lastY valid
	dropping    bool // the current drag carries files we take
}

var (
	msgSetFrameDisplay func(obj id, s objc.SEL, r nsRect, display bool)
	msgSetRect         func(obj id, s objc.SEL, r nsRect)
	msgSetFloat        func(obj id, s objc.SEL, v float64)
	msgGetFloat        func(obj id, s objc.SEL) float64
	msgColorRGBA       func(cls id, s objc.SEL, r, g, b, a float64) id
	msgGetPoint        func(obj id, s objc.SEL) nsPoint
	msgConvertPoint    func(obj id, s objc.SEL, p nsPoint, view id) nsPoint
	winExtOnce         sync.Once
)

func loadExt() {
	winExtOnce.Do(func() {
		purego.RegisterFunc(&msgSetFrameDisplay, msgSendAddr)
		purego.RegisterFunc(&msgSetRect, msgSendAddr)
		purego.RegisterFunc(&msgSetFloat, msgSendAddr)
		purego.RegisterFunc(&msgGetFloat, msgSendAddr)
		purego.RegisterFunc(&msgColorRGBA, msgSendAddr)
		purego.RegisterFunc(&msgGetPoint, msgSendAddr)
		purego.RegisterFunc(&msgConvertPoint, msgSendAddr)
	})
}

var (
	_ platform.CreatorEx   = (*backend)(nil)
	_ platform.Geometry    = (*surface)(nil)
	_ platform.Stateful    = (*surface)(nil)
	_ platform.Styled      = (*surface)(nil)
	_ platform.PageControl = (*surface)(nil)
	_ platform.Dragger     = (*surface)(nil)
)

// NewSurfaceEx implements platform.CreatorEx.
func (b *backend) NewSurfaceEx(sid platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras) (platform.Surface, error) {
	return b.newSurface(sid, opts, &ex)
}

func windowClass(ex *platform.SurfaceExtras) string {
	if ex != nil && ex.Kind == platform.KindPanel {
		return "TangoPanel"
	}
	return "TangoWindow"
}

func kindStyle(ex *platform.SurfaceExtras) uint {
	if ex != nil && ex.Kind == platform.KindPanel {
		return nsWindowStyleMaskNonactivatingPanel | nsWindowStyleMaskUtilityWindow
	}
	return 0
}

// defaultAppearance is what a surface created without extras looks like.
var defaultAppearance = platform.Appearance{}

// initExt applies the creation extras. Called once from newSurface, before
// the first navigation and Show.
func (s *surface) initExt(opts platform.SurfaceOptions, ex *platform.SurfaceExtras) {
	loadExt()
	ensureMouseMonitor()
	s.ext.resizable = opts.Resizable
	s.ext.appearance = defaultAppearance
	behavior := uint(nsWindowCollectionBehaviorFullScreenPrimary)
	if ex != nil && ex.Kind == platform.KindPanel {
		behavior = nsWindowCollectionBehaviorFullScreenAuxiliary
	}
	withPool(func() { send(s.win, "setCollectionBehavior:", uintptr(behavior)) })
	if ex == nil {
		s.ext.lastState = s.State()
		return
	}
	withPool(func() {
		switch ex.Kind {
		case platform.KindPanel:
			s.ext.panel = true
			send(s.win, "setFloatingPanel:", 1)
			send(s.win, "setHidesOnDeactivate:", 0)
			send(s.win, "setBecomesKeyOnlyIfNeeded:", 1)
		case platform.KindModal, platform.KindSheet:
			// AppKit has no window-modal windows other than sheets.
			if p := s.b.surfaces[ex.Parent]; p != nil && !p.closed {
				s.ext.sheetParent = p.win
			}
		}
	})
	s.ext.acceptFiles = ex.AcceptFiles
	if ex.MaxWidth > 0 || ex.MaxHeight > 0 {
		s.SetSizeLimits(opts.MinWidth, opts.MinHeight, ex.MaxWidth, ex.MaxHeight)
	}
	s.applyAppearance(ex.Appearance, true)
	if !ex.Centered {
		w, h := s.Size()
		if ex.Bounds.Width > 0 && ex.Bounds.Height > 0 {
			w, h = ex.Bounds.Width, ex.Bounds.Height
		}
		s.SetBounds(platform.Rect{X: ex.Bounds.X, Y: ex.Bounds.Y, Width: w, Height: h})
	}
	s.ext.initState = ex.State
	s.ext.lastState = s.State()
}

// showSheet presents a sheet surface on its parent; it reports whether the
// surface is a sheet.
func (s *surface) showSheet() bool {
	p := s.ext.sheetParent
	if p == 0 {
		return false
	}
	if send(s.win, "sheetParent") == 0 {
		withPool(func() { send(p, "beginSheet:completionHandler:", uintptr(s.win), 0) })
	}
	return true
}

// endSheet detaches a sheet before it closes.
func (s *surface) endSheet() {
	if p := s.ext.sheetParent; p != 0 && send(s.win, "sheetParent") != 0 {
		send(p, "endSheet:", uintptr(s.win))
	}
}

// afterShow applies the initial window state once the window is on screen.
func (s *surface) afterShow() {
	if st := s.ext.initState; st != platform.StateNormal {
		s.ext.initState = platform.StateNormal
		s.SetState(st)
	}
}

// ---- Geometry ----

// primaryHeight is the height of the primary display, for flipping AppKit's
// bottom-left screen coordinates.
func primaryHeight() float64 {
	var h float64
	withPool(func() {
		screens := send(class("NSScreen"), "screens")
		if screens != 0 && sendInt(screens, "count") > 0 {
			h = msgRect(send(screens, "objectAtIndex:", 0), sel("frame")).Size.Height
		}
	})
	return h
}

func (s *surface) Bounds() platform.Rect {
	if s.closed {
		return platform.Rect{}
	}
	frame := msgRect(s.win, sel("frame"))
	content := s.contentSize()
	x, y := topLeft(frame.Origin.X, frame.Origin.Y, frame.Size.Height, primaryHeight())
	return platform.Rect{X: x, Y: y, Width: int(math.Round(content.Width)), Height: int(math.Round(content.Height))}
}

func (s *surface) SetBounds(r platform.Rect) {
	if s.closed || r.Width <= 0 || r.Height <= 0 {
		return
	}
	frame := msgContentRectForRect(s.win, sel("frameRectForContentRect:"),
		nsRect{Size: nsSize{float64(r.Width), float64(r.Height)}})
	frame.Origin.X = float64(r.X)
	frame.Origin.Y = bottomLeftY(r.Y, frame.Size.Height, primaryHeight())
	msgSetFrameDisplay(s.win, sel("setFrame:display:"), frame, true)
	s.checkMoved()
}

func (s *surface) Center() {
	if !s.closed {
		send(s.win, "center")
	}
}

func (s *surface) SetSizeLimits(minW, minH, maxW, maxH int) {
	if s.closed {
		return
	}
	msgSetSize(s.win, sel("setContentMinSize:"), nsSize{float64(max(minW, 0)), float64(max(minH, 0))})
	mw, mh := float64(maxW), float64(maxH)
	if maxW <= 0 {
		mw = math.MaxFloat32
	}
	if maxH <= 0 {
		mh = math.MaxFloat32
	}
	msgSetSize(s.win, sel("setContentMaxSize:"), nsSize{mw, mh})
}

// ---- Stateful ----

func (s *surface) State() platform.WindowState {
	if s.closed {
		return platform.StateNormal
	}
	return windowState(
		uint(send(s.win, "styleMask"))&nsWindowStyleMaskFullScreen != 0,
		sendBool(s.win, "isMiniaturized"),
		sendBool(s.win, "isZoomed"))
}

func (s *surface) SetState(st platform.WindowState) {
	if s.closed {
		return
	}
	withPool(func() {
		cur := s.State()
		if cur == st {
			return
		}
		switch st {
		case platform.StateNormal:
			switch cur {
			case platform.StateFullscreen:
				send(s.win, "toggleFullScreen:", 0)
			case platform.StateMinimized:
				send(s.win, "deminiaturize:", 0)
			case platform.StateMaximized:
				send(s.win, "zoom:", 0)
			}
		case platform.StateMinimized:
			send(s.win, "miniaturize:", 0)
		case platform.StateMaximized:
			if cur == platform.StateMinimized {
				send(s.win, "deminiaturize:", 0)
			}
			if cur == platform.StateFullscreen {
				// Leaving fullscreen animates; zoom once it is done would
				// need a deferred step. Leave it restored.
				send(s.win, "toggleFullScreen:", 0)
				return
			}
			if !sendBool(s.win, "isZoomed") {
				send(s.win, "zoom:", 0)
			}
		case platform.StateFullscreen:
			if cur == platform.StateMinimized {
				send(s.win, "deminiaturize:", 0)
			}
			send(s.win, "toggleFullScreen:", 0)
		}
	})
	s.checkState()
}

// afterResize runs on every resize: zoom changes arrive as resizes, and
// resizing from the top or left edge moves the window without
// windowDidMove: (AppKit only reports moves of the whole frame).
func (s *surface) afterResize() {
	s.checkState()
	s.checkMoved()
}

// checkState reports a state change since the last report.
func (s *surface) checkState() {
	if s.closed {
		return
	}
	if st := s.State(); st != s.ext.lastState {
		s.ext.lastState = st
		s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceStateChanged, State: st})
	}
}

func (s *surface) checkMoved() {
	if s.closed {
		return
	}
	r := s.Bounds()
	if s.ext.moved && r.X == s.ext.lastX && r.Y == s.ext.lastY {
		return
	}
	s.ext.moved, s.ext.lastX, s.ext.lastY = true, r.X, r.Y
	s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceMoved, X: r.X, Y: r.Y})
}

// ---- Styled ----

func (s *surface) SetAppearance(a platform.Appearance) {
	if !s.closed {
		s.applyAppearance(a, false)
	}
}

var _ platform.EdgeResizer = (*surface)(nil)

// HandlesEdgeResize tells the core that frameless windows keep native
// resize borders here (TitleBarNone is a titled window with its title bar
// hidden).
func (s *surface) HandlesEdgeResize() bool { return true }

func (s *surface) applyAppearance(a platform.Appearance, initial bool) {
	if !initial && a == s.ext.appearance {
		return
	}
	s.ext.appearance = a
	withPool(func() {
		cur := uint(send(s.win, "styleMask"))
		style := uint(nsWindowStyleMaskTitled) | cur&(nsWindowStyleMaskFullScreen|nsWindowStyleMaskNonactivatingPanel|nsWindowStyleMaskUtilityWindow)
		if !a.NoClose {
			style |= nsWindowStyleMaskClosable
		}
		if !a.NoMinimize {
			style |= nsWindowStyleMaskMiniaturizable
		}
		if s.ext.resizable {
			style |= nsWindowStyleMaskResizable
		}
		custom := a.TitleBar != platform.TitleBarDefault
		if custom {
			style |= nsWindowStyleMaskFullSizeContent
		}
		if style != cur {
			send(s.win, "setStyleMask:", uintptr(style))
		}
		send(s.win, "setTitlebarAppearsTransparent:", boolArg(custom))
		vis := uintptr(nsWindowTitleVisible)
		if custom {
			vis = nsWindowTitleHidden
		}
		send(s.win, "setTitleVisibility:", vis)
		for _, kind := range []uintptr{nsWindowCloseButton, nsWindowMiniaturizeButton, nsWindowZoomButton} {
			if btn := send(s.win, "standardWindowButton:", kind); btn != 0 {
				send(btn, "setHidden:", boolArg(a.TitleBar == platform.TitleBarNone))
				if kind == nsWindowZoomButton {
					send(btn, "setEnabled:", boolArg(!a.NoMaximize && s.ext.resizable))
				}
			}
		}
		send(s.win, "setHasShadow:", boolArg(a.TitleBar != platform.TitleBarNone || !a.NoShadow))

		see := a.Transparent || a.Material != platform.MaterialNone
		send(s.win, "setOpaque:", boolArg(!see))
		var bg id
		switch {
		case see:
			bg = send(class("NSColor"), "clearColor")
		case a.Background != 0:
			r, g, b, al := rgbaParts(a.Background)
			bg = msgColorRGBA(class("NSColor"), sel("colorWithSRGBRed:green:blue:alpha:"), r, g, b, al)
		default:
			bg = send(class("NSColor"), "windowBackgroundColor")
		}
		send(s.win, "setBackgroundColor:", uintptr(bg))
		s.applyWebBackground(see, bg) // canvas_attach.go
		s.applyMaterial(a.Material)

		level := uintptr(nsNormalWindowLevel)
		if a.AlwaysOnTop || s.ext.panel {
			level = nsFloatingWindowLevel
		}
		send(s.win, "setLevel:", level)
		op := a.Opacity
		if op <= 0 || op > 1 {
			op = 1
		}
		msgSetFloat(s.win, sel("setAlphaValue:"), op)
	})
}

func (s *surface) applyMaterial(m platform.Material) {
	if m == platform.MaterialNone {
		if s.ext.effect != 0 {
			send(s.ext.effect, "removeFromSuperview")
			release(s.ext.effect)
			s.ext.effect = 0
		}
		return
	}
	container := send(s.view, "superview")
	if container == 0 {
		return
	}
	if s.ext.effect == 0 {
		bounds := msgRect(container, sel("bounds"))
		s.ext.effect = msgInitRect(send(class("NSVisualEffectView"), "alloc"), sel("initWithFrame:"), bounds)
		send(s.ext.effect, "setAutoresizingMask:", nsViewWidthHeightSizable)
		send(s.ext.effect, "setBlendingMode:", nsVisualEffectBlendingModeBehindWindow)
		send(s.ext.effect, "setState:", nsVisualEffectStateActive)
		send(container, "addSubview:positioned:relativeTo:", uintptr(s.ext.effect), uintptr(nsWindowBelowArg), uintptr(s.view))
	}
	mat := uintptr(nsVisualEffectMaterialUnderWindowBackground)
	if m == platform.MaterialMica {
		mat = nsVisualEffectMaterialWindowBackground
	}
	send(s.ext.effect, "setMaterial:", mat)
}

// ---- PageControl ----

func (s *surface) Reload() {
	if !s.closed {
		send(s.web, "reload")
	}
}

func (s *surface) SetZoom(f float64) {
	if !s.closed && f > 0 && respondsTo(s.web, "setPageZoom:") { // macOS 11+
		msgSetFloat(s.web, sel("setPageZoom:"), f)
	}
}

func (s *surface) Zoom() float64 {
	if s.closed || !respondsTo(s.web, "pageZoom") {
		return 1
	}
	return msgGetFloat(s.web, sel("pageZoom"))
}

// Print runs the print panel as a sheet on the window. WebKit's print view
// starts with a zero frame and prints blank pages unless it is given one.
func (s *surface) Print() {
	if s.closed || !respondsTo(s.web, "printOperationWithPrintInfo:") { // macOS 11+
		return
	}
	withPool(func() {
		op := send(s.web, "printOperationWithPrintInfo:", uintptr(send(class("NSPrintInfo"), "sharedPrintInfo")))
		if op == 0 {
			return
		}
		if v := send(op, "view"); v != 0 {
			msgSetRect(v, sel("setFrame:"), msgRect(s.web, sel("bounds")))
		}
		send(op, "setShowsPrintPanel:", 1)
		send(op, "setShowsProgressPanel:", 1)
		send(op, "runOperationModalForWindow:delegate:didRunSelector:contextInfo:", uintptr(s.win), 0, 0, 0)
	})
}

// ---- Dragger ----

var (
	monitorOnce   sync.Once
	lastMouseDown id // retained NSEvent of the last left mouse-down
)

func ensureMouseMonitor() {
	monitorOnce.Do(func() {
		block := objc.NewBlock(func(_ objc.Block, ev objc.ID) objc.ID {
			if ev != 0 {
				retain(ev)
				release(lastMouseDown)
				lastMouseDown = ev
			}
			return ev
		})
		send(class("NSEvent"), "addLocalMonitorForEventsMatchingMask:handler:", nsEventMaskLeftMouseDown, uintptr(block))
		block.Release() // the monitor keeps its own copy
	})
}

// StartMove drags the window from the mouse-down that is still held.
func (s *surface) StartMove() {
	if s.closed || uint(send(class("NSEvent"), "pressedMouseButtons"))&1 == 0 {
		return
	}
	ev := lastMouseDown
	if ev == 0 || send(ev, "window") != s.win {
		ev = send(s.b.app, "currentEvent")
	}
	if ev != 0 {
		withPool(func() { send(s.win, "performWindowDragWithEvent:", uintptr(ev)) })
	}
}

// StartResize does nothing: frameless windows keep native resize edges
// (HandlesEdgeResize), so the core never asks.
func (s *surface) StartResize(string) {}

// ---- classes and delegate methods ----

func registerExtClasses() {
	classDef("TangoPanel", "NSPanel", nil, []objc.MethodDef{
		method("canBecomeKeyWindow", func(self id, _ objc.SEL) bool { return true }),
	})

	of := func(self id) *surface {
		if b := theBackend; b != nil {
			if s := b.byWebView[self]; s != nil && !s.closed {
				return s
			}
		}
		return nil
	}
	// taking reports whether s takes the drag described by info itself.
	taking := func(s *surface, info id) bool {
		return s != nil && s.ext.acceptFiles && len(draggedFiles(info)) > 0
	}
	super := func(self id, name string, info id) id { return webSuperArg(self, name, info) }
	classDef("TangoWebView", "WKWebView", nil, []objc.MethodDef{
		// Embedded web views report keyboard focus (webembed.go).
		method("becomeFirstResponder", func(self id, _ objc.SEL) bool {
			ok := webSuperBool(self, "becomeFirstResponder")
			if ok {
				webFirstResponder(self, true)
			}
			return ok
		}),
		method("resignFirstResponder", func(self id, _ objc.SEL) bool {
			ok := webSuperBool(self, "resignFirstResponder")
			if ok {
				webFirstResponder(self, false)
			}
			return ok
		}),
		method("draggingEntered:", func(self id, _ objc.SEL, info id) uint {
			s := of(self)
			if taking(s, info) {
				s.ext.dropping = true
				return nsDragOperationCopy
			}
			if s != nil {
				s.ext.dropping = false
			}
			return uint(super(self, "draggingEntered:", info))
		}),
		method("draggingUpdated:", func(self id, _ objc.SEL, info id) uint {
			if s := of(self); s != nil && s.ext.dropping {
				return nsDragOperationCopy
			}
			return uint(super(self, "draggingUpdated:", info))
		}),
		method("draggingExited:", func(self id, _ objc.SEL, info id) {
			if s := of(self); s != nil && s.ext.dropping {
				s.ext.dropping = false
				return
			}
			super(self, "draggingExited:", info)
		}),
		method("prepareForDragOperation:", func(self id, _ objc.SEL, info id) bool {
			if s := of(self); s != nil && s.ext.dropping {
				return true
			}
			return byte(super(self, "prepareForDragOperation:", info)) != 0
		}),
		method("performDragOperation:", func(self id, _ objc.SEL, info id) bool {
			s := of(self)
			if s == nil || !s.ext.dropping {
				return byte(super(self, "performDragOperation:", info)) != 0
			}
			s.ext.dropping = false
			paths := draggedFiles(info)
			if len(paths) == 0 {
				return false
			}
			x, y := s.dropPoint(info)
			s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: paths, X: x, Y: y})
			return true
		}),
		method("concludeDragOperation:", func(self id, _ objc.SEL, info id) {
			if s := of(self); s != nil && s.ext.acceptFiles && len(draggedFiles(info)) > 0 {
				return
			}
			super(self, "concludeDragOperation:", info)
		}),
	})
}

// draggedFiles returns the local file paths on a drag's pasteboard.
func draggedFiles(info id) []string {
	var out []string
	withPool(func() {
		pb := send(info, "draggingPasteboard")
		if pb == 0 {
			return
		}
		key := constPtr(libAppKit, "NSPasteboardURLReadingFileURLsOnlyKey")
		if key == 0 {
			return
		}
		opts := send(class("NSDictionary"), "dictionaryWithObject:forKey:", uintptr(nsNumberBool(true)), key)
		urls := send(pb, "readObjectsForClasses:options:", uintptr(nsArray(class("NSURL"))), uintptr(opts))
		for _, u := range arrayItems(urls) {
			if p := goString(send(u, "path")); p != "" {
				out = append(out, p)
			}
		}
	})
	return out
}

// dropPoint converts a drag's location to page coordinates (top-left).
func (s *surface) dropPoint(info id) (int, int) {
	p := msgConvertPoint(s.web, sel("convertPoint:fromView:"), msgGetPoint(info, sel("draggingLocation")), 0)
	if !sendBool(s.web, "isFlipped") {
		p.Y = msgRect(s.web, sel("bounds")).Size.Height - p.Y
	}
	return int(math.Round(p.X)), int(math.Round(p.Y))
}

// extDelegateMethods are the round-2 methods of TangoSurfaceDelegate.
func extDelegateMethods(of func(self id) *surface) []objc.MethodDef {
	state := func(self id, _ objc.SEL, n id) {
		if s := of(self); s != nil {
			s.checkState()
		}
	}
	failed := func(self id, _ objc.SEL, web, nav, err id) {
		s := of(self)
		if s == nil || err == 0 {
			return
		}
		// Cancelled: another navigation replaced this one and reports
		// for itself.
		if sendInt(err, "code") == nsURLErrorCancelled || s.ignoreLoadError(err) {
			return
		}
		var msg, u string
		withPool(func() {
			msg = goString(send(err, "localizedDescription"))
			if info := send(err, "userInfo"); info != 0 {
				u = goString(send(info, "objectForKey:", uintptr(nsString("NSErrorFailingURLStringKey"))))
				if fu := send(info, "objectForKey:", uintptr(nsString("NSErrorFailingURLKey"))); u == "" && fu != 0 && sendBool(fu, "isKindOfClass:", uintptr(class("NSURL"))) {
					u = goString(send(fu, "absoluteString"))
				}
			}
		})
		if u == "" {
			u = s.url() // after a provisional failure: the page still shown
		}
		s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceLoadFailed, URL: u, Error: msg})
	}
	return []objc.MethodDef{
		method("windowDidMove:", func(self id, _ objc.SEL, n id) {
			if s := of(self); s != nil {
				s.checkMoved()
			}
		}),
		method("windowDidMiniaturize:", state),
		method("windowDidDeminiaturize:", state),
		method("windowDidEnterFullScreen:", state),
		method("windowDidExitFullScreen:", state),
		method("webView:didStartProvisionalNavigation:", func(self id, _ objc.SEL, web, nav id) {
			if s := of(self); s != nil {
				s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceNavigating, URL: s.url()})
			}
		}),
		method("webView:didFailProvisionalNavigation:withError:", failed),
		method("webView:didFailNavigation:withError:", failed),
	}
}
