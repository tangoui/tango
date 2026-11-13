//go:build darwin && !ios

package darwin

// platform.Network on macOS: Network.framework's path monitor through its C
// API (nw_path_monitor_create, nw_path_monitor_set_update_handler, ...).
// The monitor delivers paths on a private serial dispatch queue; the latest
// status is kept under a mutex and changes hop to the main thread.

import (
	"fmt"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.Network = (*backend)(nil)

// nw_path_status_t and nw_interface_type_t (Network/path.h, interface.h).
const (
	nwPathStatusInvalid     = 0
	nwPathStatusSatisfied   = 1
	nwPathStatusUnsatisfied = 2
	nwPathStatusSatisfiable = 3

	nwInterfaceOther    = 0
	nwInterfaceWiFi     = 1
	nwInterfaceCellular = 2
	nwInterfaceWired    = 3
	nwInterfaceLoopback = 4
)

type netMonitor struct {
	once sync.Once
	err  error

	monitorCreate  func() uintptr
	setQueue       func(m, q uintptr)
	setHandler     func(m, blk uintptr)
	start          func(m uintptr)
	pathStatus     func(p uintptr) int32
	usesInterface  func(p uintptr, typ int32) bool
	isExpensive    func(p uintptr) bool
	isConstrained  func(p uintptr) bool // macOS 10.15+; nil before
	queueCreate    func(label *byte, attr uintptr) uintptr
	monitor, queue uintptr
	block          objc.Block // kept alive for the monitor's lifetime

	mu      sync.Mutex
	status  platform.NetworkStatus
	have    bool
	first   chan struct{}                // closed with the first update
	handler func(platform.NetworkStatus) // main thread
}

var netMon = netMonitor{first: make(chan struct{})}

// netPath is what the update handler reads from an nw_path_t.
type netPath struct {
	status                      int32
	wifi, cellular, wired, loop bool
	other                       bool
	expensive, constrained      bool
}

// networkStatusFrom maps a path to a NetworkStatus. Pure, for tests.
func networkStatusFrom(p netPath) platform.NetworkStatus {
	st := platform.NetworkStatus{Metered: p.expensive, Constrained: p.constrained}
	switch p.status {
	case nwPathStatusSatisfied:
		st.Online = true
	case nwPathStatusUnsatisfied, nwPathStatusSatisfiable:
		st.Kind = platform.NetworkNone
		st.Metered, st.Constrained = false, false
		return st
	default:
		return st // invalid: unknown
	}
	switch {
	case p.wired:
		st.Kind = platform.NetworkWired
	case p.wifi:
		st.Kind = platform.NetworkWiFi
	case p.cellular:
		st.Kind = platform.NetworkCellular
	default:
		st.Kind = platform.NetworkOther
	}
	return st
}

func (m *netMonitor) load() error {
	m.once.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/Network.framework/Network", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			m.err = fmt.Errorf("tango/darwin: Network.framework unavailable: %w", err)
			return
		}
		purego.RegisterLibFunc(&m.monitorCreate, lib, "nw_path_monitor_create")
		purego.RegisterLibFunc(&m.setQueue, lib, "nw_path_monitor_set_queue")
		purego.RegisterLibFunc(&m.setHandler, lib, "nw_path_monitor_set_update_handler")
		purego.RegisterLibFunc(&m.start, lib, "nw_path_monitor_start")
		purego.RegisterLibFunc(&m.pathStatus, lib, "nw_path_get_status")
		purego.RegisterLibFunc(&m.usesInterface, lib, "nw_path_uses_interface_type")
		purego.RegisterLibFunc(&m.isExpensive, lib, "nw_path_is_expensive")
		if sym, err := purego.Dlsym(lib, "nw_path_is_constrained"); err == nil && sym != 0 {
			purego.RegisterFunc(&m.isConstrained, sym)
		}
		purego.RegisterLibFunc(&m.queueCreate, purego.RTLD_DEFAULT, "dispatch_queue_create")

		label := []byte("tango.network\x00")
		m.queue = m.queueCreate(&label[0], 0) // serial
		m.monitor = m.monitorCreate()
		if m.monitor == 0 || m.queue == 0 {
			m.err = fmt.Errorf("tango/darwin: nw_path_monitor_create failed")
			return
		}
		m.setQueue(m.monitor, m.queue)
		// void (^)(nw_path_t path), called on m.queue.
		m.block = objc.NewBlock(func(_ objc.Block, path uintptr) { m.update(path) })
		m.setHandler(m.monitor, uintptr(m.block))
		m.start(m.monitor)
	})
	return m.err
}

// update runs on the monitor's queue.
func (m *netMonitor) update(path uintptr) {
	p := netPath{
		status:    m.pathStatus(path),
		wifi:      m.usesInterface(path, nwInterfaceWiFi),
		cellular:  m.usesInterface(path, nwInterfaceCellular),
		wired:     m.usesInterface(path, nwInterfaceWired),
		loop:      m.usesInterface(path, nwInterfaceLoopback),
		other:     m.usesInterface(path, nwInterfaceOther),
		expensive: m.isExpensive(path),
	}
	if m.isConstrained != nil {
		p.constrained = m.isConstrained(path)
	}
	st := networkStatusFrom(p)
	m.mu.Lock()
	changed := !m.have || st != m.status
	first := !m.have
	m.status, m.have = st, true
	m.mu.Unlock()
	if first {
		close(m.first)
	}
	if changed {
		hopMain(func() {
			if fn := netMon.handler; fn != nil {
				fn(st)
			}
		})
	}
}

// netFirstWait bounds how long NetworkStatus waits for the monitor's first
// path (it arrives within milliseconds).
const netFirstWait = 2 * time.Second

func (b *backend) NetworkStatus() (platform.NetworkStatus, error) {
	m := &netMon
	if err := m.load(); err != nil {
		return platform.NetworkStatus{}, err
	}
	select {
	case <-m.first:
	case <-time.After(netFirstWait):
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, nil
}

func (b *backend) SetNetworkHandler(fn func(platform.NetworkStatus)) error {
	m := &netMon
	if err := m.load(); err != nil {
		return err
	}
	m.handler = fn
	return nil
}
