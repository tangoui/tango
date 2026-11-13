//go:build windows && (amd64 || arm64)

package windows

// platform.Network on Windows.
//
//   - Online: the Network List Manager's GetConnectivity (internet
//     connectivity as Windows' NCSI judges it); cost: INetworkCostManager
//     GetCost for the machine (NLM_CONNECTION_COST_* → Metered,
//     Constrained).
//   - Kind: the interface of the best route to a public address
//     (GetBestInterfaceEx) looked up in GetAdaptersAddresses (IfType).
//   - Changes: NotifyIpInterfaceChange and NotifyRouteChange2 (called on
//     system threads) post to the main thread, debounced by 500 ms since
//     NCSI needs a moment to re-evaluate connectivity after a link change.
//
// Without the Network List Manager (stripped systems, COM failures) a
// default route on an operational interface counts as online.

import (
	"encoding/binary"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.Network = (*Backend)(nil)

var (
	iphlpapi = systemDLL("iphlpapi.dll")

	sxProcGetAdaptersAddresses   = iphlpapi.NewProc("GetAdaptersAddresses")
	sxProcGetBestInterfaceEx     = iphlpapi.NewProc("GetBestInterfaceEx")
	sxProcNotifyIpInterfaceChang = iphlpapi.NewProc("NotifyIpInterfaceChange")
	sxProcNotifyRouteChange2     = iphlpapi.NewProc("NotifyRouteChange2")
	sxProcCancelMibChangeNotify2 = iphlpapi.NewProc("CancelMibChangeNotify2")
)

var (
	sxCLSIDNetworkListManager = guid("dcb00c01-570f-4a9b-8d69-199fdba5723b")
	sxIIDINetworkListManager  = guid("dcb00000-570f-4a9b-8d69-199fdba5723b")
	sxIIDINetworkCostManager  = guid("dcb00008-570f-4a9b-8d69-199fdba5723b")
)

const (
	// INetworkListManager (IDispatch: 3-6).
	sxNLMGetConnectivity = 13
	// INetworkCostManager.
	sxNCMGetCost = 3

	sxAFUnspec = 0
	sxAFInet   = 2

	sxGAAFlags = 0x2 | 0x4 | 0x8 // SKIP_ANYCAST | SKIP_MULTICAST | SKIP_DNS_SERVER

	sxNetDebounceMS = 500
)

var sxNet struct {
	nlm     uintptr // INetworkListManager, main thread
	nlmTry  bool
	handler func(platform.NetworkStatus)
	handles []uintptr
	cb      uintptr
	cbOnce  sync.Once
}

func (b *Backend) sxNLM() uintptr {
	if !sxNet.nlmTry {
		sxNet.nlmTry = true
		sxNet.nlm, _ = sxCoCreate(&sxCLSIDNetworkListManager, &sxIIDINetworkListManager)
	}
	return sxNet.nlm
}

func (b *Backend) NetworkStatus() (platform.NetworkStatus, error) {
	var in sxNetInput
	if nlm := b.sxNLM(); nlm != 0 {
		var c uint32
		if !failed(comCall(nlm, sxNLMGetConnectivity, uintptr(unsafe.Pointer(&c)))) {
			in.haveNLM, in.connectivity = true, c
		}
		if ncm := queryInterface(nlm, &sxIIDINetworkCostManager); ncm != 0 {
			var cost uint32
			if !failed(comCall(ncm, sxNCMGetCost, uintptr(unsafe.Pointer(&cost)), 0)) {
				in.haveCost, in.cost = true, cost
			}
			release(ncm)
		}
	}
	if idx, ok := sxBestInterface(); ok {
		in.ifType, in.ifUp, in.haveIf = sxInterfaceType(idx)
	}
	return sxNetStatus(in), nil
}

// sxBestInterface is the interface index of the best route to a public
// IPv4 address (no traffic is sent).
func sxBestInterface() (uint32, bool) {
	if !has(sxProcGetBestInterfaceEx) {
		return 0, false
	}
	var sa [16]byte // sockaddr_in for 8.8.8.8
	binary.LittleEndian.PutUint16(sa[0:], sxAFInet)
	sa[4], sa[5], sa[6], sa[7] = 8, 8, 8, 8
	var idx uint32
	r, _, _ := sxProcGetBestInterfaceEx.Call(uintptr(unsafe.Pointer(&sa[0])), uintptr(unsafe.Pointer(&idx)))
	return idx, r == 0
}

// sxInterfaceType looks up the IfType and operational state of interface
// idx (have is false if it is not found).
func sxInterfaceType(idx uint32) (ifType uint32, up, have bool) {
	size := uint32(16 << 10)
	var buf []uint64 // 8-byte aligned
	for range 3 {
		buf = make([]uint64, (size+7)/8)
		r, _, _ := sxProcGetAdaptersAddresses.Call(sxAFUnspec, sxGAAFlags, 0,
			uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		if r == 111 { // ERROR_BUFFER_OVERFLOW: size was updated
			continue
		}
		if r != 0 {
			return 0, false, false
		}
		for p := unsafe.Pointer(&buf[0]); p != nil; {
			length := *(*uint32)(p)
			if length >= sxAAMinSize && *(*uint32)(unsafe.Add(p, sxAAOffIfIndex)) == idx {
				return *(*uint32)(unsafe.Add(p, sxAAOffIfType)),
					*(*uint32)(unsafe.Add(p, sxAAOffOperStatus)) == sxIfOperStatusUp, true
			}
			p = *(*unsafe.Pointer)(unsafe.Add(p, sxAAOffNext))
		}
		return 0, false, false
	}
	return 0, false, false
}

func (b *Backend) SetNetworkHandler(fn func(platform.NetworkStatus)) error {
	if _, err := sxWindow(); err != nil {
		return err
	}
	sxNet.handler = fn
	if fn == nil {
		for _, h := range sxNet.handles {
			sxProcCancelMibChangeNotify2.Call(h)
		}
		sxNet.handles = nil
		return nil
	}
	if sxNet.handles != nil {
		return nil
	}
	// The callback runs on a system thread: only post to the main thread.
	sxNet.cbOnce.Do(func() {
		sxNet.cb = syscall.NewCallback(func(ctx, row, typ uintptr) uintptr {
			sxPost(sxNetChanged)
			return 0
		})
	})
	for _, p := range []*syscall.LazyProc{sxProcNotifyIpInterfaceChang, sxProcNotifyRouteChange2} {
		if !has(p) {
			continue
		}
		var h uintptr
		if r, _, _ := p.Call(sxAFUnspec, sxNet.cb, 0, 0, uintptr(unsafe.Pointer(&h))); r == 0 {
			sxNet.handles = append(sxNet.handles, h)
		}
	}
	return nil
}

// sxNetChanged runs on the main thread after a change notification and
// reports the status once things settle.
func sxNetChanged() {
	sxAfter(sxTimerNetwork, sxNetDebounceMS, func() {
		if sxNet.handler == nil || theBackend == nil {
			return
		}
		if st, err := theBackend.NetworkStatus(); err == nil {
			sxNet.handler(st)
		}
	})
}
