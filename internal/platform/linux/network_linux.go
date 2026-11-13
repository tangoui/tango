//go:build linux && !android

package linux

// platform.Network through NetworkManager on the system bus: State,
// Connectivity, Metered and PrimaryConnectionType of
// /org/freedesktop/NetworkManager, re-read on its PropertiesChanged and
// StateChanged signals. Systems managed by something else (systemd-
// networkd alone, ConnMan) report ErrUnsupported.

import (
	"fmt"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.Network = (*backend)(nil)

const (
	nmBus  = "org.freedesktop.NetworkManager"
	nmPath = "/org/freedesktop/NetworkManager"
)

var nm struct {
	handler func(platform.NetworkStatus)
	unsubs  []func()
}

func (b *backend) NetworkStatus() (platform.NetworkStatus, error) {
	conn, err := bus(gBusTypeSystem)
	if err != nil {
		return platform.NetworkStatus{}, err
	}
	d, err := dbusGetAll(conn, nmBus, nmPath, nmBus)
	if err != nil {
		if isMissing(err) {
			return platform.NetworkStatus{}, fmt.Errorf("%w: NetworkManager is not running", platform.ErrUnsupported)
		}
		return platform.NetworkStatus{}, err
	}
	defer gVariantUnref(d)
	state, _ := dictUint32(d, "State")
	conn4, _ := dictUint32(d, "Connectivity")
	metered, _ := dictUint32(d, "Metered")
	typ, ok := dictString(d, "PrimaryConnectionType") // NetworkManager 1.0+
	if !ok {
		typ = primaryConnectionType(conn, d)
	}
	return nmStatus(state, conn4, metered, typ), nil
}

// primaryConnectionType reads the Type of the PrimaryConnection object
// (NetworkManager before 1.0 has no PrimaryConnectionType).
func primaryConnectionType(conn, d ptr) string {
	path, _ := dictString(d, "PrimaryConnection")
	if path == "" || path == "/" {
		return ""
	}
	v, err := dbusGet(conn, nmBus, path, nmBus+".Connection.Active", "Type")
	if err != nil {
		return ""
	}
	defer gVariantUnref(v)
	return gvStr(v)
}

func (b *backend) SetNetworkHandler(fn func(platform.NetworkStatus)) error {
	conn, err := bus(gBusTypeSystem)
	if err != nil {
		return err
	}
	nm.handler = fn
	if fn == nil {
		for _, u := range nm.unsubs {
			u()
		}
		nm.unsubs = nil
		return nil
	}
	if nm.unsubs != nil {
		return nil
	}
	if !nameHasOwner(conn, nmBus) {
		nm.handler = nil
		return fmt.Errorf("%w: NetworkManager is not running", platform.ErrUnsupported)
	}
	changed := func(_, _, _, _ string, _ ptr) {
		if nm.handler == nil {
			return
		}
		if st, err := b.NetworkStatus(); err == nil {
			nm.handler(st) // the core drops repeats
		}
	}
	nm.unsubs = append(nm.unsubs,
		subscribe(conn, nmBus, propsIface, "PropertiesChanged", nmPath, "", changed),
		subscribe(conn, nmBus, nmBus, "StateChanged", nmPath, "", changed),
	)
	return nil
}
