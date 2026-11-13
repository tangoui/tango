//go:build windows && (amd64 || arm64)

package windows

// platform.Power on Windows.
//
//   - Status: GetSystemPowerStatus (AC line, battery flag and percentage,
//     SystemStatusFlag = Battery Saver).
//   - Events: WM_POWERBROADCAST to a hidden top-level window of our own
//     class (message-only windows do not receive broadcasts):
//     PBT_APMSUSPEND, PBT_APMRESUMEAUTOMATIC / PBT_APMRESUMESUSPEND,
//     PBT_APMPOWERSTATUSCHANGE, and PBT_POWERSETTINGCHANGE for
//     GUID_POWER_SAVING_STATUS (Battery Saver switched) and
//     GUID_ACDC_POWER_SOURCE. On Modern Standby machines the suspend
//     notification may arrive late or not at all; resume is reliable.
//   - Blocks: power requests (PowerCreateRequest / PowerSetRequest with a
//     simple reason string, listed by `powercfg /requests`); on systems
//     without them SetThreadExecutionState on the main thread with the
//     flags of every active block.

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.Power = (*Backend)(nil)

var (
	sxProcGetSystemPowerStatus        = kernel32.NewProc("GetSystemPowerStatus")
	sxProcSetThreadExecutionState     = kernel32.NewProc("SetThreadExecutionState")
	sxProcPowerCreateRequest          = kernel32.NewProc("PowerCreateRequest")
	sxProcPowerSetRequest             = kernel32.NewProc("PowerSetRequest")
	sxProcPowerClearRequest           = kernel32.NewProc("PowerClearRequest")
	sxProcRegisterPowerSettingNotif   = user32.NewProc("RegisterPowerSettingNotification")
	sxProcUnregisterPowerSettingNotif = user32.NewProc("UnregisterPowerSettingNotification")
	sxProcSetTimer                    = user32.NewProc("SetTimer")
	sxProcKillTimer                   = user32.NewProc("KillTimer")
)

const (
	sxWmPowerBroadcast = 0x0218
	sxWmTimer          = 0x0113

	sxTimerNetwork = 1
)

var (
	sxGUIDPowerSavingStatus = guid("e00958c0-c213-4ace-ac77-fecced2eeea5")
	sxGUIDACDCPowerSource   = guid("5d3e9a59-e9d5-4b00-a6bd-ff34ff516548")
)

// sxWin is the main-thread state of the sysx window.
var sxWin struct {
	once sync.Once
	hwnd uintptr
	err  error

	power        func(platform.PowerEvent)
	powerTracker sxPowerTracker
	powerNotify  []uintptr // HPOWERNOTIFY

	onTimer map[uintptr]func()
}

func sxWindow() (uintptr, error) {
	sxWin.once.Do(func() {
		const class = "TangoSysxWindow"
		wc := wndClassEx{WndProc: syscall.NewCallback(sxWndProc), Instance: instance(), ClassName: u16(class)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			sxWin.err = fmt.Errorf("tango: RegisterClassExW(%s): %v", class, err)
			return
		}
		hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16(class))), 0,
			0x80000000 /* WS_POPUP */, 0, 0, 0, 0, 0, 0, instance(), 0)
		if hwnd == 0 {
			sxWin.err = fmt.Errorf("tango: cannot create the sysx window: %v", err)
			return
		}
		sxWin.hwnd = hwnd
		sxWin.onTimer = map[uintptr]func(){}
	})
	return sxWin.hwnd, sxWin.err
}

// sxPost runs fn on the main thread later; safe from any thread (system
// callbacks, WinRT thread-pool events).
func sxPost(fn func()) {
	if b := theBackend; b != nil {
		b.runDeferred(fn)
	}
}

// sxAfter runs fn on the main thread after ms milliseconds; a pending
// timer with the same id is restarted (debouncing). Main thread.
func sxAfter(id uintptr, ms uint32, fn func()) {
	if sxWin.hwnd == 0 {
		return
	}
	sxWin.onTimer[id] = fn
	sxProcSetTimer.Call(sxWin.hwnd, id, uintptr(ms), 0)
}

func sxWndProc(hwnd, m, wp, lp uintptr) uintptr {
	switch uint32(m) {
	case sxWmTimer:
		sxProcKillTimer.Call(hwnd, wp)
		if fn := sxWin.onTimer[wp]; fn != nil {
			delete(sxWin.onTimer, wp)
			fn()
		}
		return 0
	case sxWmPowerBroadcast:
		if ev, ok := sxWin.powerTracker.event(uint32(wp)); ok && sxWin.power != nil {
			sxWin.power(ev)
		}
		return 1 // TRUE
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// sxSystemPowerStatus is SYSTEM_POWER_STATUS.
type sxSystemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func (b *Backend) PowerStatus() (platform.PowerStatus, error) {
	var s sxSystemPowerStatus
	if r, _, err := sxProcGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s))); r == 0 {
		return platform.PowerStatus{Battery: -1}, fmt.Errorf("tango: GetSystemPowerStatus: %v", err)
	}
	return sxPowerStatus(s.ACLineStatus, s.BatteryFlag, s.BatteryLifePercent, s.SystemStatusFlag), nil
}

func (b *Backend) SetPowerHandler(fn func(platform.PowerEvent)) error {
	hwnd, err := sxWindow()
	if err != nil {
		return err
	}
	sxWin.power = fn
	if fn != nil && sxWin.powerNotify == nil && has(sxProcRegisterPowerSettingNotif) {
		for _, g := range []*GUID{&sxGUIDPowerSavingStatus, &sxGUIDACDCPowerSource} {
			if h, _, _ := sxProcRegisterPowerSettingNotif.Call(hwnd, uintptr(unsafe.Pointer(g)), 0 /* DEVICE_NOTIFY_WINDOW_HANDLE */); h != 0 {
				sxWin.powerNotify = append(sxWin.powerNotify, h)
			}
		}
	}
	if fn == nil {
		for _, h := range sxWin.powerNotify {
			sxProcUnregisterPowerSettingNotif.Call(h)
		}
		sxWin.powerNotify = nil
	}
	return nil
}

// sxReasonContext is REASON_CONTEXT with a simple reason string (the
// union is sized for its larger, detailed form).
type sxReasonContext struct {
	Version uint32
	Flags   uint32
	Reason  *uint16
	_       [16]byte
}

type sxSleepBlock struct {
	kind   platform.SleepBlock
	handle uintptr  // power request; 0 with the execution-state fallback
	reason []uint16 // kept alive while the request exists
}

var sxBlocks = struct {
	next  uint64
	items map[uint64]sxSleepBlock
}{items: map[uint64]sxSleepBlock{}}

var errSxBlockKind = errors.New("tango: unknown sleep block kind")

func (b *Backend) BlockSleep(kind platform.SleepBlock, reason string) (uint64, error) {
	if kind != platform.BlockSystemSleep && kind != platform.BlockDisplaySleep {
		return 0, errSxBlockKind
	}
	blk := sxSleepBlock{kind: kind}
	if has(sxProcPowerCreateRequest) {
		blk.reason = utf16z(reason)
		ctx := sxReasonContext{Version: 0, Flags: 1 /* POWER_REQUEST_CONTEXT_SIMPLE_STRING */, Reason: &blk.reason[0]}
		h, _, err := sxProcPowerCreateRequest.Call(uintptr(unsafe.Pointer(&ctx)))
		if h == 0 || h == ^uintptr(0) {
			return 0, fmt.Errorf("tango: PowerCreateRequest: %v", err)
		}
		for _, t := range sxPowerRequestTypes(kind) {
			if r, _, err := sxProcPowerSetRequest.Call(h, uintptr(t)); r == 0 {
				sxProcCloseHandle.Call(h)
				return 0, fmt.Errorf("tango: PowerSetRequest: %v", err)
			}
		}
		blk.handle = h
	}
	sxBlocks.next++
	tok := sxBlocks.next
	sxBlocks.items[tok] = blk
	if blk.handle == 0 {
		sxApplyExecState()
	}
	return tok, nil
}

func (b *Backend) UnblockSleep(token uint64) {
	blk, ok := sxBlocks.items[token]
	if !ok {
		return
	}
	delete(sxBlocks.items, token)
	if blk.handle != 0 {
		for _, t := range sxPowerRequestTypes(blk.kind) {
			sxProcPowerClearRequest.Call(blk.handle, uintptr(t))
		}
		sxProcCloseHandle.Call(blk.handle)
		return
	}
	sxApplyExecState()
}

// sxApplyExecState sets the main thread's execution state from the
// fallback blocks (those without a power request).
func sxApplyExecState() {
	m := map[uint64]platform.SleepBlock{}
	for tok, blk := range sxBlocks.items {
		if blk.handle == 0 {
			m[tok] = blk.kind
		}
	}
	sxProcSetThreadExecutionState.Call(uintptr(sxExecState(m)))
}
