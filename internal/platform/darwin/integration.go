//go:build darwin && !ios

package darwin

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// Desktop integration (contract round 2): displays, system theme, shell,
// dock control, plus the change notifications behind AppThemeChanged and
// AppDisplaysChanged.

var (
	_ platform.Displays    = (*backend)(nil)
	_ platform.SystemTheme = (*backend)(nil)
	_ platform.Shell       = (*backend)(nil)
	_ platform.AppControl  = (*backend)(nil)
)

var (
	integOnce sync.Once

	msgGetDouble func(obj id, s objc.SEL) float64
	msgSetDouble func(obj id, s objc.SEL, v float64)
	msgGetSize   func(obj id, s objc.SEL) nsSize
)

// integState is main-thread state; one backend per process.
var integState struct {
	observer     id
	lastTheme    platform.Theme
	lastDisplays []platform.Display
	dockView     id // dock tile content view while progress shows
	dockProgress id
}

func bindInteg() {
	integOnce.Do(func() {
		for _, fn := range []any{&msgGetDouble, &msgSetDouble, &msgGetSize} {
			purego.RegisterFunc(fn, msgSendAddr)
		}
		onScreens := func(_ id, _ objc.SEL, _ id) { theBackend.displaysMaybeChanged() }
		onColors := func(_ id, _ objc.SEL, _ id) { theBackend.themeMaybeChanged() }
		onKVO := func(_ id, _ objc.SEL, _, _, _ id, _ uintptr) { theBackend.themeMaybeChanged() }
		classDef("TangoSystemObserver", "NSObject", nil, []objc.MethodDef{
			method("screensChanged:", onScreens),
			method("colorsChanged:", onColors),
			method("observeValueForKeyPath:ofObject:change:context:", onKVO),
		})
	})
}

// systemNotifications are the NSNotificationCenter names the observer
// listens to, with the selector each one triggers.
var systemNotifications = []struct{ name, selector string }{
	// Screens added, removed, rearranged or rescaled.
	{"NSApplicationDidChangeScreenParametersNotification", "screensChanged:"},
	// Accent / highlight colour picked in System Settings.
	{"NSSystemColorsDidChangeNotification", "colorsChanged:"},
}

// installSystemObservers starts watching for display and theme changes.
// Called from Init on the main thread.
func (b *backend) installSystemObservers() {
	bindInteg()
	withPool(func() {
		watcher := alloc("TangoSystemObserver")
		integState.observer = watcher
		nc := send(class("NSNotificationCenter"), "defaultCenter")
		for _, n := range systemNotifications {
			send(nc, "addObserver:selector:name:object:",
				uintptr(watcher), uintptr(sel(n.selector)), uintptr(nsString(n.name)), 0)
		}
		// Light / dark switches are observed with KVO on
		// NSApp.effectiveAppearance (macOS 10.14+), which only changes once
		// AppKit has applied the new appearance. The distributed
		// AppleInterfaceThemeChangedNotification can fire earlier than that.
		if respondsTo(b.app, "effectiveAppearance") {
			send(b.app, "addObserver:forKeyPath:options:context:",
				uintptr(watcher), uintptr(nsString("effectiveAppearance")), 0, 0)
		}
		integState.lastTheme = b.SystemTheme()
		integState.lastDisplays = b.Displays()
	})
}

func (b *backend) themeMaybeChanged() {
	if b == nil || b.h == nil {
		return
	}
	t := b.SystemTheme()
	if t == integState.lastTheme {
		return
	}
	integState.lastTheme = t
	b.appEvent(platform.AppEvent{Kind: platform.AppThemeChanged, Theme: t})
}

func (b *backend) displaysMaybeChanged() {
	if b == nil || b.h == nil {
		return
	}
	ds := b.Displays()
	if slices.Equal(ds, integState.lastDisplays) {
		return
	}
	integState.lastDisplays = ds
	b.appEvent(platform.AppEvent{Kind: platform.AppDisplaysChanged})
}

// ---- platform.Displays ----

// Displays describes every NSScreen. AppKit measures from the bottom-left
// corner of the primary screen (index 0 of +screens, the menu-bar screen)
// with y growing upwards; the contract wants the primary's top-left corner
// with y growing downwards.
func (b *backend) Displays() []platform.Display {
	bindInteg()
	var list []platform.Display
	withPool(func() {
		all := arrayItems(send(class("NSScreen"), "screens"))
		if len(all) == 0 {
			return
		}
		top := msgRect(all[0], sel("frame")).Size.Height
		list = make([]platform.Display, 0, len(all))
		for idx, scr := range all {
			list = append(list, describeScreen(scr, top, idx == 0))
		}
	})
	return list
}

// describeScreen converts one NSScreen; top is the primary screen's height,
// the line the y axis is mirrored about.
func describeScreen(scr id, top float64, primary bool) platform.Display {
	d := platform.Display{
		Primary:  primary,
		Scale:    msgGetDouble(scr, sel("backingScaleFactor")),
		Bounds:   flipRect(msgRect(scr, sel("frame")), top),
		WorkArea: flipRect(msgRect(scr, sel("visibleFrame")), top),
	}
	// deviceDescription["NSScreenNumber"] is the CGDirectDisplayID, stable
	// while the display stays connected.
	info := send(scr, "deviceDescription")
	if n := send(info, "objectForKey:", uintptr(nsString("NSScreenNumber"))); n != 0 {
		displayID := uint32(sendInt(n, "unsignedIntValue"))
		d.ID = strconv.FormatUint(uint64(displayID), 10)
	}
	if respondsTo(scr, "localizedName") { // macOS 10.15+
		d.Name = goString(send(scr, "localizedName"))
	}
	return d
}

func flipRect(r nsRect, primaryH float64) platform.Rect {
	bottom := r.Origin.Y + r.Size.Height
	return platform.Rect{
		X:      int(math.Round(r.Origin.X)),
		Y:      int(math.Round(primaryH - bottom)),
		Width:  int(math.Round(r.Size.Width)),
		Height: int(math.Round(r.Size.Height)),
	}
}

// ---- platform.SystemTheme ----

const (
	appearanceLight = "NSAppearanceNameAqua"
	appearanceDark  = "NSAppearanceNameDarkAqua"
)

func (b *backend) SystemTheme() platform.Theme {
	bindInteg()
	var t platform.Theme
	withPool(func() {
		t.Dark = appIsDark(b.app)
		colors := class("NSColor")
		if respondsTo(colors, "controlAccentColor") { // macOS 10.14+
			t.Accent = rgba(send(colors, "controlAccentColor"))
		}
	})
	return t
}

// appIsDark asks the app's effective appearance which of the two base
// appearances it is closest to, so high-contrast and vibrant variants
// resolve correctly too.
func appIsDark(app id) bool {
	if !respondsTo(app, "effectiveAppearance") {
		return false
	}
	look := send(app, "effectiveAppearance")
	if !respondsTo(look, "bestMatchFromAppearancesWithNames:") {
		return false
	}
	candidates := nsArray(nsString(appearanceLight), nsString(appearanceDark))
	return goString(send(look, "bestMatchFromAppearancesWithNames:", uintptr(candidates))) == appearanceDark
}

// rgba converts an NSColor to 0xRRGGBBAA in sRGB; 0 if it has no sRGB form
// (pattern colours, for example).
func rgba(c id) uint32 {
	srgb := send(class("NSColorSpace"), "sRGBColorSpace")
	if c = send(c, "colorUsingColorSpace:", uintptr(srgb)); c == 0 {
		return 0
	}
	var comp [4]float64 // red, green, blue, alpha
	send(c, "getRed:green:blue:alpha:",
		uintptr(unsafe.Pointer(&comp[0])), uintptr(unsafe.Pointer(&comp[1])),
		uintptr(unsafe.Pointer(&comp[2])), uintptr(unsafe.Pointer(&comp[3])))
	var packed uint32
	for _, v := range comp {
		packed = packed<<8 | uint32(channel(v))
	}
	return packed
}

func channel(v float64) uint8 {
	return uint8(math.Round(min(max(v, 0), 1) * 255))
}

// ---- platform.Shell ----

var errWorkspace = errors.New("tango/darwin: the workspace could not open it")

func (b *backend) OpenURL(u string) error {
	ok := false
	withPool(func() {
		url := nsURL(u)
		if url == 0 {
			return
		}
		ok = sendBool(send(class("NSWorkspace"), "sharedWorkspace"), "openURL:", uintptr(url))
	})
	if !ok {
		return errWorkspace
	}
	return nil
}

func (b *backend) OpenPath(path string) error {
	ok := false
	withPool(func() {
		ok = sendBool(send(class("NSWorkspace"), "sharedWorkspace"), "openURL:", uintptr(fileURL(path)))
	})
	if !ok {
		return errWorkspace
	}
	return nil
}

func (b *backend) RevealPath(path string) error {
	withPool(func() {
		send(send(class("NSWorkspace"), "sharedWorkspace"), "activateFileViewerSelectingURLs:", uintptr(nsArray(fileURL(path))))
	})
	return nil
}

// ---- platform.AppControl ----

const (
	nsCriticalRequest                      = 0
	nsInformationalRequest                 = 10
	nsApplicationActivationPolicyAccessory = 1
	nsProgressIndicatorStyleBar            = 0
)

// AppControlOps: the Dock does everything.
func (b *backend) AppControlOps() platform.AppControlOp { return platform.CtlAll }

func (b *backend) HideApp() { send(b.app, "hide:", 0) }

func (b *backend) ShowApp() {
	send(b.app, "unhide:", 0)
	b.bringToFront()
}

// bringToFront activates the app even if another one is frontmost.
func (b *backend) bringToFront() {
	send(b.app, "activateIgnoringOtherApps:", boolArg(true))
}

func (b *backend) SetBadge(text string) {
	withPool(func() {
		var label id
		if text != "" {
			label = nsString(text)
		}
		send(send(b.app, "dockTile"), "setBadgeLabel:", uintptr(label))
	})
}

// SetProgress shows p (0…1) as a bar over the app icon in the Dock by giving
// the NSDockTile a custom content view; a negative p goes back to the plain
// icon. NSDockTile only redraws a content view when told to (-display).
func (b *backend) SetProgress(p float64) {
	bindInteg()
	withPool(func() {
		tile := send(b.app, "dockTile")
		switch {
		case p < 0 && integState.dockView == 0:
			return // nothing to remove
		case p < 0:
			send(tile, "setContentView:", 0)
			release(integState.dockView)
			release(integState.dockProgress)
			integState.dockView, integState.dockProgress = 0, 0
		default:
			if integState.dockView == 0 {
				integState.dockView, integState.dockProgress = b.makeDockProgress(tile)
				send(tile, "setContentView:", uintptr(integState.dockView))
			}
			msgSetDouble(integState.dockProgress, sel("setDoubleValue:"), min(p, 1))
		}
		send(tile, "display")
	})
}

// makeDockProgress builds the tile view: the app icon filling the tile and a
// bar-style NSProgressIndicator across its lower part. Both are owned (+1).
func (b *backend) makeDockProgress(tile id) (view, bar id) {
	tileSize := msgGetSize(tile, sel("size"))
	view = msgInitRect(send(class("NSImageView"), "alloc"), sel("initWithFrame:"), nsRect{Size: tileSize})
	send(view, "setImage:", uintptr(send(b.app, "applicationIconImage")))

	w, h := tileSize.Width, tileSize.Height
	barFrame := nsRect{Origin: nsPoint{X: 0.1 * w, Y: 0.08 * h}, Size: nsSize{Width: 0.8 * w, Height: 0.14 * h}}
	bar = msgInitRect(send(class("NSProgressIndicator"), "alloc"), sel("initWithFrame:"), barFrame)
	send(bar, "setStyle:", nsProgressIndicatorStyleBar)
	send(bar, "setIndeterminate:", boolArg(false))
	msgSetDouble(bar, sel("setMinValue:"), 0)
	msgSetDouble(bar, sel("setMaxValue:"), 1)
	send(view, "addSubview:", uintptr(bar))
	return view, bar
}

func (b *backend) RequestAttention(critical bool) {
	kind := uintptr(nsInformationalRequest)
	if critical {
		kind = nsCriticalRequest
	}
	send(b.app, "requestUserAttention:", kind)
}

func (b *backend) SetDockVisible(visible bool) {
	if visible {
		send(b.app, "setActivationPolicy:", nsApplicationActivationPolicyRegular)
		b.bringToFront()
		return
	}
	send(b.app, "setActivationPolicy:", nsApplicationActivationPolicyAccessory)
}

var _ platform.WebViewInfo = (*backend)(nil)

// WebViewVersion reports WebKit.framework's version.
func (b *backend) WebViewVersion() (engine, version string) {
	withPool(func() {
		bundle := send(class("NSBundle"), "bundleWithIdentifier:", uintptr(nsString("com.apple.WebKit")))
		if bundle == 0 {
			return
		}
		v := send(bundle, "objectForInfoDictionaryKey:", uintptr(nsString("CFBundleVersion")))
		if v == 0 {
			v = send(bundle, "objectForInfoDictionaryKey:", uintptr(nsString("CFBundleShortVersionString")))
		}
		version = goString(v)
	})
	return "WebKit", version
}
