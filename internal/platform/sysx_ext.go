package platform

// Contract extensions, desktop system capabilities ("sysx"): power,
// network, app shortcut menus (Dock menu / Jump List) and dragging files
// out of a surface. Like ext.go everything here is OPTIONAL and found with
// a type assertion; nothing changes what the minimum Backend must do.
//
// Threading is the backend.go rule: methods are called on the main thread
// and callbacks run on the main thread, unless stated otherwise.

// ---------------------------------------------------------------------------
// Power

// PowerSource is where the machine draws power from.
type PowerSource int

const (
	PowerSourceUnknown PowerSource = iota
	PowerSourceAC                  // mains (or a UPS reporting AC)
	PowerSourceBattery             // running on the internal battery
)

// PowerStatus is a snapshot of the machine's power state.
type PowerStatus struct {
	Source     PowerSource
	HasBattery bool    // the machine has an internal battery
	Battery    float64 // charge in [0,1]; -1 when unknown or no battery
	Charging   bool
	// LowPower is the system's power-saving mode: Low Power Mode (macOS
	// 12+), Battery / Energy Saver (Windows 10+), the "power-saver" power
	// profile (Linux power-profiles-daemon).
	LowPower bool
}

// PowerEvent is a change reported to the PowerEvents handler.
type PowerEvent int

const (
	PowerSuspending    PowerEvent = iota + 1 // the system is about to sleep
	PowerResumed                             // the system woke up
	PowerStatusChanged                       // PowerStatus changed (source, charge, low-power mode)
)

// SleepBlock is what BlockSleep keeps awake.
type SleepBlock int

const (
	// BlockSystemSleep keeps the system from idle-sleeping (long tasks,
	// downloads); the display may still turn off.
	BlockSystemSleep SleepBlock = iota + 1
	// BlockDisplaySleep keeps the display on as well (video playback,
	// presentations). It implies BlockSystemSleep.
	BlockDisplaySleep
)

// Power reports the power state, suspend/resume, and keeps the machine
// awake. A system sleep the user asks for (lid closed, Sleep menu) is not
// blocked by BlockSleep on any platform.
type Power interface {
	PowerStatus() (PowerStatus, error)
	// SetPowerHandler starts (fn != nil) or stops (nil) delivering power
	// events to fn on the main thread. PowerSuspending is delivered on a
	// best-effort basis before the system sleeps; handlers must be quick.
	SetPowerHandler(fn func(PowerEvent)) error
	// BlockSleep starts one sleep block and returns a nonzero token.
	// reason is shown by the system where it shows one (pmset -g
	// assertions, powercfg /requests, systemd-inhibit --list).
	BlockSleep(kind SleepBlock, reason string) (token uint64, err error)
	// UnblockSleep ends a block; unknown tokens are ignored.
	UnblockSleep(token uint64)
}

// ---------------------------------------------------------------------------
// Network

// NetworkKind is the interface type the default route uses.
type NetworkKind int

const (
	NetworkUnknown  NetworkKind = iota
	NetworkNone                 // offline
	NetworkWired                // Ethernet (and Thunderbolt / USB Ethernet)
	NetworkWiFi                 // 802.11
	NetworkCellular             // mobile broadband / tethered modem
	NetworkOther                // VPN-only, loopback, unknown kinds
)

// NetworkStatus is a snapshot of connectivity.
type NetworkStatus struct {
	Online bool // a route to the internet is believed to exist
	Kind   NetworkKind
	// Metered is the system's "expensive" flag: cellular, a personal
	// hotspot, or a connection the user marked as metered.
	Metered bool
	// Constrained is Low Data Mode (macOS) / an approaching data limit
	// (Windows); false elsewhere.
	Constrained bool
}

// Network reports connectivity and its changes.
type Network interface {
	NetworkStatus() (NetworkStatus, error)
	// SetNetworkHandler starts (fn != nil) or stops (nil) delivering
	// changes to fn on the main thread. Repeated identical states may be
	// coalesced by the backend; the core coalesces them too.
	SetNetworkHandler(fn func(NetworkStatus)) error
}

// ---------------------------------------------------------------------------
// App shortcut menu (macOS Dock menu, Windows Jump List tasks)

// ShortcutArgPrefix is the command-line argument a Windows Jump List task
// launches the executable with: ShortcutArgPrefix + ShortcutItem.ID. Clicks
// on such tasks arrive as a new process (forwarded to the primary instance
// by sys/single when the app uses it); sys/jumplist turns them into click
// callbacks.
const ShortcutArgPrefix = "--tango-shortcut="

// ShortcutItem is one entry of the app shortcut menu.
type ShortcutItem struct {
	ID          string // stable identifier, reported on click; nonempty unless Separator
	Label       string
	Description string // tooltip (Windows); ignored elsewhere
	// IconPath is an icon resource for the item: an .ico/.exe/.dll path,
	// optionally ",index" (Windows). Ignored elsewhere; empty uses the
	// executable's icon.
	IconPath  string
	Separator bool
	Disabled  bool // macOS only; Windows tasks cannot be disabled
}

// AppShortcuts sets the app's shortcut menu: the Dock icon's menu on macOS
// (items above the system's own), the Jump List "Tasks" category on
// Windows. items == nil clears it. On macOS a click calls onClick(id) in
// this process; on Windows the system relaunches the executable instead
// (see ShortcutArgPrefix) and onClick is not called by the backend.
// Linux has no runtime API (Desktop Actions are written into the .desktop
// file at packaging time) and does not implement it.
type AppShortcuts interface {
	SetAppShortcuts(items []ShortcutItem, onClick func(id string)) error
}

// ---------------------------------------------------------------------------
// Dragging files out of a surface (optional on Surface)

// DragOperation is what the drop target did with dragged files.
type DragOperation int

const (
	DragNone DragOperation = iota // canceled, or the target refused
	DragCopy
	DragMove
	DragLink
)

// FileDragger is implemented by surfaces that can start a native drag of
// files to other apps (Finder, Explorer, file managers, mail clients).
//
// StartFileDrag must be called while a mouse button is held over the
// surface — typically from the page's mousedown / dragstart, relayed to
// Go — because the platforms tie a drag session to the press that started
// it; without a press the drag ends at once with DragNone. paths are
// absolute and must exist. icon is an optional PNG for the drag image
// (nil: the system's file icon). done runs once on the main thread when
// the drag ends. Where the platform's drag loop is modal (Windows
// DoDragDrop) StartFileDrag returns after done has run.
//
// Backends offer the target copy and link only, never move: the dragged
// files are the app's own, and a file manager would otherwise take them
// away on a same-volume drop. DragMove is reported only if a target moves
// anyway. icon may be ignored where the system builds the drag image
// itself (Windows shell data object).
type FileDragger interface {
	StartFileDrag(paths []string, icon []byte, done func(op DragOperation)) error
}
