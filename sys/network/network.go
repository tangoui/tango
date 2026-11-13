// Package network reports whether the machine is online, over what kind of
// link, whether the connection is metered, and when that changes.
//
//	st, err := network.Get(a)
//	if st.Online && !st.Metered { startBigDownload() }
//	network.OnChange(a, func(st network.Status) { banner.SetVisible(!st.Online) })
//
// Platforms: macOS uses Network.framework's path monitor (nw_path_monitor);
// Windows the Network List Manager (INetworkListManager connectivity and
// connection cost) with IP interface change notifications; Linux
// NetworkManager over the system D-Bus; iOS the same path monitor as
// macOS; Android ConnectivityManager's default-network callback (Online
// needs a validated internet connection, Constrained is Data Saver on a
// metered network; the host manifest declares ACCESS_NETWORK_STATE). "Online" is the system's own
// judgement (a usable default route, or NetworkManager's connectivity
// check), not a probe of any particular server.
//
// Functions are safe from any goroutine once the app runs; handlers run
// on the main thread and only for real changes. OnChange may be called
// before Run. Get returns ErrUnsupported where the backend cannot tell
// (minimal backends, Linux without NetworkManager) and
// tango.ErrNotRunning after the app quit.
package network

import (
	"slices"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

// ErrUnsupported is platform.ErrUnsupported.
var ErrUnsupported = platform.ErrUnsupported

// Kind is the type of link the default route uses.
type Kind = platform.NetworkKind

const (
	Unknown  = platform.NetworkUnknown
	None     = platform.NetworkNone
	Wired    = platform.NetworkWired
	WiFi     = platform.NetworkWiFi
	Cellular = platform.NetworkCellular
	Other    = platform.NetworkOther
)

// Status is a snapshot of connectivity.
type Status = platform.NetworkStatus

// Get returns the current connectivity.
func Get(a *tango.App) (Status, error) {
	n, err := capcall.Backend[platform.Network](a)
	if err != nil {
		return Status{}, err
	}
	return capcall.Sync(a, n.NetworkStatus)
}

// KindString names k ("wired", "wifi", "cellular", "none", "other",
// "unknown"), for logs and the JS API.
func KindString(k Kind) string {
	switch k {
	case None:
		return "none"
	case Wired:
		return "wired"
	case WiFi:
		return "wifi"
	case Cellular:
		return "cellular"
	case Other:
		return "other"
	}
	return "unknown"
}

type appState struct {
	a        *tango.App
	mu       sync.Mutex
	handlers []func(Status)
	hooked   bool

	last    Status // main thread
	hasLast bool
}

var states sync.Map // *tango.App -> *appState

// OnChange calls fn whenever connectivity, the link kind or the metered
// flags change.
func OnChange(a *tango.App, fn func(Status)) {
	v, _ := states.LoadOrStore(a, &appState{a: a})
	st := v.(*appState)
	st.mu.Lock()
	st.handlers = append(st.handlers, fn)
	hook := !st.hooked
	st.hooked = true
	st.mu.Unlock()
	if !hook {
		return
	}
	n, ok := a.Backend().(platform.Network)
	if !ok {
		return
	}
	a.OnReady(func() {
		if s, err := n.NetworkStatus(); err == nil {
			st.last, st.hasLast = s, true
		}
		_ = n.SetNetworkHandler(st.changed)
	})
}

// changed runs on the main thread.
func (st *appState) changed(s Status) {
	if st.hasLast && s == st.last {
		return
	}
	st.last, st.hasLast = s, true
	st.mu.Lock()
	fns := slices.Clone(st.handlers)
	st.mu.Unlock()
	// The backend calls changed directly: guard each handler on its own.
	for _, fn := range fns {
		corehook.Guard(st.a, "event handler", func() { fn(s) })
	}
}
