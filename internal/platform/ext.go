package platform

// Contract extensions (round 2). Everything here is OPTIONAL: a backend or a
// native surface implements an interface only if the platform supports it,
// and the core discovers support with a type assertion. Nothing in this file
// changes what the minimum Backend / Surface must do.
//
// Threading is the same as backend.go: methods are called on the main thread,
// callbacks are invoked on the main thread, unless stated otherwise.

// ---------------------------------------------------------------------------
// Geometry and window state (optional on Surface)

// Rect is a rectangle in logical (DIP) pixels on the desktop.
//
// Coordinates (revised round 3, all desktop backends): the origin is the
// top-left corner of the primary display, X grows right and Y grows down,
// so displays left of or above the primary have negative coordinates. For
// a window (Geometry.Bounds/SetBounds, SurfaceExtras.Bounds,
// SurfaceMoved), X/Y is the top-left corner of the window's outer frame —
// title bar and borders included — and Width/Height is the content size,
// the same size SurfaceOptions, SetSize and Size use. Display.Bounds,
// Display.WorkArea and Tray.Bounds use the same space. Backends whose
// native coordinates differ (AppKit's bottom-left origin, GDK's origin at
// the top-left of the whole screen layout) convert.
type Rect struct{ X, Y, Width, Height int }

type WindowState int

const (
	StateNormal WindowState = iota
	StateMinimized
	StateMaximized
	StateFullscreen
)

// Geometry is implemented by surfaces that are free-floating windows.
type Geometry interface {
	Bounds() Rect
	SetBounds(r Rect)
	Center()
	SetSizeLimits(minW, minH, maxW, maxH int) // 0 = no limit
}

// Stateful is implemented by surfaces that can be minimized / maximized /
// fullscreen. Changes made by the user are reported with SurfaceStateChanged.
type Stateful interface {
	State() WindowState
	SetState(s WindowState)
}

// TitleBar styles.
type TitleBar int

const (
	TitleBarDefault TitleBar = iota
	TitleBarHidden           // content extends under the title area, traffic lights / caption buttons stay
	TitleBarNone             // frameless: no title bar, no system buttons
)

// Material is a system backdrop behind transparent content.
type Material int

const (
	MaterialNone Material = iota
	MaterialBlur          // macOS vibrancy / Windows acrylic / Linux: ignored
	MaterialMica          // Windows 11 Mica; macOS: window background vibrancy
)

// Appearance is applied as a whole: the core always sends the full desired
// state, backends diff against what they have. Unsupported fields are ignored.
//
// The zero value is an ordinary window (revised round 3: the switches are
// negative, so a zero Appearance no longer means "no close button, no
// shadow").
type Appearance struct {
	TitleBar    TitleBar
	Transparent bool   // WebView and window background transparent
	Background  uint32 // 0xRRGGBBAA, used when !Transparent; 0 = platform default
	Material    Material
	AlwaysOnTop bool
	Opacity     float64 // 0 means 1.0
	NoShadow    bool    // drop the window shadow; only meaningful for TitleBarNone
	NoClose     bool    // disable the close button / close command
	NoMinimize  bool    // disable minimizing
	NoMaximize  bool    // disable maximizing / zoom
}

type Styled interface {
	SetAppearance(a Appearance)
}

// PageControl covers WebView-level controls.
type PageControl interface {
	Reload()
	SetZoom(factor float64) // 1.0 = 100%
	Zoom() float64
	Print()
}

// Dragger lets frameless windows be moved/resized from page content. The
// core injects JS that detects elements with CSS `--tango-drag: drag` (or
// `app-region: drag`) and asks the backend to start a native drag on the
// current mouse-down. Backends that can detect drag regions natively may
// ignore these calls.
type Dragger interface {
	StartMove()
	StartResize(edge string) // "n","s","e","w","ne","nw","se","sw"
}

// EdgeResizer is implemented by surfaces that tell the core whether their
// frameless (TitleBarNone) resizable windows keep native resize borders.
// When HandlesEdgeResize reports true the core does not turn the page's
// edges into resize handles (Dragger.StartResize is then never called).
// Surfaces without it are treated as reporting false. (Round 3.)
type EdgeResizer interface {
	HandlesEdgeResize() bool
}

// SurfaceStatus is implemented by surfaces that can report their window's
// visibility and keyboard focus as the window system sees it (hidden
// together with the app, minimized windows still count as visible). The
// core falls back to its own bookkeeping (Show/Hide, SurfaceFocused /
// SurfaceBlurred) for surfaces without it. (Round 3.)
type SurfaceStatus interface {
	IsVisible() bool
	IsFocused() bool
}

// SurfaceKind and Parent are in SurfaceOptions (types.go additions below are
// fields the core may set; backends that do not support a kind create a
// normal window).
type SurfaceKind int

const (
	KindWindow SurfaceKind = iota
	KindPanel              // floating utility panel, does not activate the app (NSPanel / WS_EX_TOOLWINDOW)
	KindModal              // modal to Parent; Parent must be set
	KindSheet              // macOS sheet attached to Parent; other platforms: KindModal
)

// SurfaceExtras carries round-2 creation options. The core passes it to
// backends implementing CreatorEx; others get the plain SurfaceOptions.
type SurfaceExtras struct {
	Kind        SurfaceKind
	Parent      SurfaceID
	Bounds      Rect // zero Width/Height = use SurfaceOptions size; X/Y ignored when Centered
	Centered    bool
	MaxWidth    int
	MaxHeight   int
	State       WindowState
	Appearance  Appearance
	AcceptFiles bool // report SurfaceFilesDropped instead of letting the page navigate to dropped files
}

type CreatorEx interface {
	NewSurfaceEx(id SurfaceID, opts SurfaceOptions, ex SurfaceExtras) (Surface, error)
}

// ---------------------------------------------------------------------------
// App-level events (backends call Host via the optional HostEx; the core's
// Host always implements HostEx).

type AppEventKind int

const (
	AppOpenURL         AppEventKind = iota + 1 // URL set (deep link / custom scheme)
	AppOpenFiles                               // Paths set (file association, dock drop, "Open With")
	AppThemeChanged                            // Theme set
	AppDisplaysChanged                         // display added/removed/rearranged/rescaled
	AppReopen                                  // macOS dock icon clicked with no visible windows

	// AppNotificationOpened (round 3): the user clicked a notification
	// for which the backend has no live Notifier.Notify onClick — one sent
	// by an earlier run of the app, typically the click that launched it.
	// Notification is set to its Notification.ID.
	AppNotificationOpened
)

type Theme struct {
	Dark   bool
	Accent uint32 // 0xRRGGBBAA; 0 = unknown
}

// AppEvent is one app-level event. Which fields are set depends on Kind
// (documented with each kind); the others are zero. Round 3 added typed
// payloads for the mobile kinds (mobile_ext.go), which used to borrow URL
// and Paths.
type AppEvent struct {
	Kind  AppEventKind
	URL   string   // AppOpenURL; AppWebViewRestored: the URL reloaded
	Paths []string // AppOpenFiles
	Theme Theme    // AppThemeChanged

	Surface      SurfaceID   // AppWebViewRestored: the surface whose page was reloaded
	Insets       Insets      // AppSafeAreaChanged: the new insets
	Orientation  Orientation // AppOrientationChanged: the new orientation
	Push         PushEvent   // AppPushToken, AppPushReceived, AppPushOpened
	Notification string      // AppNotificationOpened: the notification's ID
}

// HostEx is implemented by the core's Host. Backends type-assert their Host
// to HostEx (always succeeds for the real core; fake hosts may not).
type HostEx interface {
	Host
	OnAppEvent(ev AppEvent)
	// ShouldQuit is asked when the user quits the app (Cmd+Q, Quit menu role,
	// last-window policy is the core's business). Return false to cancel.
	ShouldQuit() bool
}

// ---------------------------------------------------------------------------
// Optional Backend capabilities

type Display struct {
	ID       string
	Name     string
	Bounds   Rect
	WorkArea Rect
	Scale    float64
	Primary  bool
}

type Displays interface {
	Displays() []Display
}

type SystemTheme interface {
	SystemTheme() Theme
}

// Shell opens things with the OS default handler.
type Shell interface {
	OpenURL(url string) error
	OpenPath(path string) error   // open file/folder with default app
	RevealPath(path string) error // show in Finder / Explorer / file manager
}

// AppControlOp is a set of AppControl operations.
type AppControlOp uint32

const (
	CtlHide      AppControlOp = 1 << iota // HideApp / ShowApp
	CtlBadge                              // SetBadge with counts ("3", "99+")
	CtlBadgeText                          // SetBadge with any text
	CtlProgress                           // SetProgress
	CtlAttention                          // RequestAttention
	CtlDock                               // SetDockVisible

	CtlAll = CtlHide | CtlBadge | CtlBadgeText | CtlProgress | CtlAttention | CtlDock
)

// AppControl is app-wide window-manager integration. Methods for
// operations not in AppControlOps are no-ops; capability packages check
// AppControlOps first and report ErrUnsupported (round 3: replaces the
// duck-typed AppControlSupports(op string)).
type AppControl interface {
	// AppControlOps reports which operations the platform performs. It may
	// be called from any goroutine and must not change while the app runs.
	AppControlOps() AppControlOp

	HideApp()
	ShowApp()
	// SetBadge sets the dock/taskbar badge; "" clears.
	SetBadge(text string)
	// SetProgress shows dock/taskbar progress in [0,1]; negative hides.
	SetProgress(p float64)
	RequestAttention(critical bool)
	// SetDockVisible false makes a tray-only (accessory) app on macOS.
	SetDockVisible(visible bool)
}

// ClipboardRich extends Clipboard with images (PNG bytes) and file lists.
//
// Reading a clipboard that holds no item of the kind asked for (empty, or
// only text) returns (nil, nil), not an error; errors are for clipboards
// that could not be read or converted. Writing replaces the whole
// clipboard content.
type ClipboardRich interface {
	ClipboardReadImage() ([]byte, error)
	ClipboardWriteImage(png []byte) error
	ClipboardReadFiles() ([]string, error)
	// ClipboardWriteFiles puts absolute paths on the clipboard as a file
	// list (Finder / Explorer / file manager copy). Round 3.
	ClipboardWriteFiles(paths []string) error
}

// PopupMenu shows a context menu at (x,y) in the surface's content
// coordinates (logical px). onClick(id) is called on the main thread if an
// item is chosen; onClose always follows.
type PopupMenu interface {
	PopupMenu(surface SurfaceID, x, y int, items []MenuItem, onClick func(id uint32), onClose func())
}

// TrayMenuTrigger chooses which click opens a tray icon's menu.
type TrayMenuTrigger int

const (
	// TrayMenuDefault follows the platform: primary click on macOS and
	// Linux, secondary click on Windows.
	TrayMenuDefault TrayMenuTrigger = iota
	// TrayMenuPrimary opens the menu on a primary (left) click; OnClick
	// does not fire while a menu is set.
	TrayMenuPrimary
	// TrayMenuSecondary opens the menu on a secondary (right) click and
	// reports primary clicks with OnClick.
	TrayMenuSecondary
)

type TrayOptions struct {
	Icon     []byte // PNG
	Template bool   // macOS template image (adapts to light/dark menu bar)
	Tooltip  string
	Title    string // text next to the icon (macOS only)
	// MenuOn chooses the click that opens the menu (round 3). Linux
	// StatusNotifierItem honours it (sysx); the AppIndicator fallback
	// always opens the menu on any click and ignores it.
	MenuOn TrayMenuTrigger
}

type TrayHandlers struct {
	OnClick       func() // primary click, unless it opens the menu (see TrayMenuTrigger)
	OnRightClick  func()
	OnDoubleClick func()
	OnMenu        func(id uint32) // menu item chosen
}

type Tray interface {
	SetOptions(o TrayOptions)
	SetMenu(items []MenuItem) // nil removes menu
	// Bounds is the icon rect on screen if the platform knows it. Linux
	// StatusNotifierItem knows only where the last Activate click was and
	// reports that as a 1x1 rect (sysx).
	Bounds() (Rect, bool)
	Remove()
}

type Trays interface {
	NewTray(o TrayOptions, h TrayHandlers) (Tray, error)
}

type Notification struct {
	ID       string // core-assigned
	Title    string
	Subtitle string
	Body     string
	Silent   bool
}

// Notifier shows notifications. Clicks on a notification sent in this run
// call its onClick. A click the backend cannot route to an onClick — on a
// notification an earlier run sent, including the click that launched
// the app — is reported as an AppNotificationOpened event with the
// notification's ID, where the platform tells (macOS). Backends keep such
// a launch click until LifecycleStarted, like other launch events.
type Notifier interface {
	// RequestNotifyPermission resolves to whether notifications may be shown.
	RequestNotifyPermission(done func(granted bool, err error))
	Notify(n Notification, onClick func()) error
}

// Hotkeys are system-wide shortcuts that fire even when the app is inactive.
type Hotkeys interface {
	RegisterHotkey(id uint32, accelerator string, fire func()) error
	UnregisterHotkey(id uint32)
}

// ---------------------------------------------------------------------------
// Round 3: capabilities that were duck-typed method names before.

// WebViewInfo reports the page engine and its version, e.g. ("WebKit",
// "20621.1.15"), ("WebView2", "131.0.2903.70"), ("WebKitGTK", "2.46.5").
// version may be "" when unknown. Any goroutine.
type WebViewInfo interface {
	WebViewVersion() (engine, version string)
}

// RecentDocuments is the system's list of recently opened documents (the
// Dock menu's Open Recent on macOS). Main thread.
type RecentDocuments interface {
	NoteRecentDocument(path string) // path is absolute
	ClearRecentDocuments()
}

// NestedLoop is implemented by backends to say whether Step(true), called
// from inside a main-thread callback, processes native events (input,
// dialogs, the callbacks that would call done). Where it cannot (Android:
// the Looper cannot be re-entered), waiting there would never end, so the
// core refuses AwaitAsync on the main thread at once with an error
// instead. Backends without it are treated as supporting nested loops.
type NestedLoop interface {
	NestedLoopSupported() bool
}

// AppAppearance is an optional Backend capability: force the light or dark
// look of the whole app — window chrome, menus, system controls and the
// pages' prefers-color-scheme — or follow the system again. mode is
// "light", "dark" or "" (follow the system). While forced, SystemTheme
// reports the forced look and AppThemeChanged fires for it, as for a
// system switch. Main thread.
type AppAppearance interface {
	SetAppAppearance(mode string)
}

// MotionPreference is an optional Backend capability: the system "reduce
// motion" accessibility preference (macOS
// accessibilityDisplayShouldReduceMotion, Windows
// SPI_GETCLIENTAREAANIMATION off, GTK gtk-enable-animations off). When it
// changes the backend reports AppReduceMotionChanged through
// HostEx.OnAppEvent, as AppThemeChanged for the theme. Any goroutine.
type MotionPreference interface {
	ReduceMotion() bool
}

// AppReduceMotionChanged: the system "reduce motion" preference changed
// (MotionPreference). A signal without payload: read the new value with
// MotionPreference.ReduceMotion. Its value lies outside the desktop
// (1...) and mobile (100...) ranges.
const AppReduceMotionChanged AppEventKind = 200

// ContrastPreference is an optional Backend capability: the system "increase
// contrast" / high-contrast accessibility preference (macOS
// accessibilityDisplayShouldIncreaseContrast, Windows SPI_GETHIGHCONTRAST
// with HCF_HIGHCONTRASTON — the contrast themes —, the XDG settings
// portal's org.freedesktop.appearance contrast or a GTK theme named like
// "HighContrast" on Linux). false also means the platform does not say.
// When it changes the backend reports AppContrastChanged through
// HostEx.OnAppEvent, as AppReduceMotionChanged for MotionPreference. Any
// goroutine.
type ContrastPreference interface {
	HighContrast() bool
}

// AppContrastChanged: the system high-contrast preference changed
// (ContrastPreference). A signal without payload: read the new value with
// ContrastPreference.HighContrast. Next to AppReduceMotionChanged, outside
// the desktop (1...) and mobile (100...) ranges.
const AppContrastChanged AppEventKind = 201
