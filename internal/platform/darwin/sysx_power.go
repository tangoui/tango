//go:build darwin && !ios

package darwin

// platform.Power on macOS: IOKit power sources (IOPSCopyPowerSourcesInfo
// and friends, IOPSNotificationCreateRunLoopSource for changes), power
// assertions (IOPMAssertionCreateWithName) to block idle sleep, NSWorkspace
// sleep/wake notifications and NSProcessInfo's Low Power Mode (macOS 12+).

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.Power = (*backend)(nil)

type powerAPI struct {
	once sync.Once
	err  error

	copyInfo       func() uintptr                                    // IOPSCopyPowerSourcesInfo
	copyList       func(info uintptr) uintptr                        // IOPSCopyPowerSourcesList
	description    func(info, ps uintptr) uintptr                    // IOPSGetPowerSourceDescription (Get rule)
	providingType  func(info uintptr) uintptr                        // IOPSGetProvidingPowerSourceType (Get rule)
	notifySource   func(cb, ctx uintptr) uintptr                     // IOPSNotificationCreateRunLoopSource
	assertCreate   func(typ, level, name uintptr, out *uint32) int32 // IOPMAssertionCreateWithName
	assertRelease  func(aid uint32) int32                            // IOPMAssertionRelease
	cfRelease      func(cf uintptr)
	cfAddSource    func(rl, src, mode uintptr)
	cfRemoveSource func(rl, src, mode uintptr)
}

var iokit powerAPI

func loadPower() error {
	p := &iokit
	p.once.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			p.err = fmt.Errorf("tango/darwin: IOKit unavailable: %w", err)
			return
		}
		purego.RegisterLibFunc(&p.copyInfo, lib, "IOPSCopyPowerSourcesInfo")
		purego.RegisterLibFunc(&p.copyList, lib, "IOPSCopyPowerSourcesList")
		purego.RegisterLibFunc(&p.description, lib, "IOPSGetPowerSourceDescription")
		purego.RegisterLibFunc(&p.providingType, lib, "IOPSGetProvidingPowerSourceType")
		purego.RegisterLibFunc(&p.notifySource, lib, "IOPSNotificationCreateRunLoopSource")
		purego.RegisterLibFunc(&p.assertCreate, lib, "IOPMAssertionCreateWithName")
		purego.RegisterLibFunc(&p.assertRelease, lib, "IOPMAssertionRelease")
		purego.RegisterLibFunc(&p.cfRelease, libCF, "CFRelease")
		purego.RegisterLibFunc(&p.cfAddSource, libCF, "CFRunLoopAddSource")
		purego.RegisterLibFunc(&p.cfRemoveSource, libCF, "CFRunLoopRemoveSource")
	})
	return p.err
}

// powerSource is one entry of IOPSCopyPowerSourcesList, reduced to what
// PowerStatus needs.
type powerSource struct {
	internal bool // Type == "InternalBattery"
	present  bool
	current  int // Current Capacity
	max      int // Max Capacity
	charging bool
}

// powerStatusFrom combines the providing source and the batteries into a
// PowerStatus. Pure, for tests.
func powerStatusFrom(providing string, sources []powerSource, lowPower bool) platform.PowerStatus {
	st := platform.PowerStatus{Battery: -1, LowPower: lowPower}
	switch providing {
	case "AC Power", "UPS Power":
		st.Source = platform.PowerSourceAC
	case "Battery Power":
		st.Source = platform.PowerSourceBattery
	}
	for _, s := range sources {
		if !s.internal || !s.present {
			continue
		}
		st.HasBattery = true
		if s.max > 0 && st.Battery < 0 {
			st.Battery = min(1, float64(s.current)/float64(s.max))
		}
		st.Charging = st.Charging || s.charging
	}
	return st
}

func dictInt(d id, key string) (int, bool) {
	v := send(d, "objectForKey:", uintptr(nsString(key)))
	if v == 0 || !respondsTo(v, "intValue") {
		return 0, false
	}
	return sendInt(v, "intValue"), true
}

func dictBool(d id, key string) bool {
	v := send(d, "objectForKey:", uintptr(nsString(key)))
	return v != 0 && respondsTo(v, "boolValue") && sendBool(v, "boolValue")
}

func dictString(d id, key string) string {
	v := send(d, "objectForKey:", uintptr(nsString(key)))
	if v == 0 || !respondsTo(v, "UTF8String") {
		return ""
	}
	return goString(v)
}

func lowPowerMode() bool {
	pi := send(class("NSProcessInfo"), "processInfo")
	return respondsTo(pi, "isLowPowerModeEnabled") && sendBool(pi, "isLowPowerModeEnabled")
}

func (b *backend) PowerStatus() (platform.PowerStatus, error) {
	if err := loadPower(); err != nil {
		return platform.PowerStatus{Battery: -1}, err
	}
	p := &iokit
	var st platform.PowerStatus
	withPool(func() {
		info := p.copyInfo()
		if info == 0 {
			st = powerStatusFrom("", nil, lowPowerMode())
			return
		}
		defer p.cfRelease(info)
		providing := goString(id(p.providingType(info)))
		var sources []powerSource
		if list := p.copyList(info); list != 0 {
			for _, ps := range arrayItems(id(list)) {
				d := id(p.description(info, uintptr(ps)))
				if d == 0 {
					continue
				}
				cur, _ := dictInt(d, "Current Capacity")
				mx, _ := dictInt(d, "Max Capacity")
				sources = append(sources, powerSource{
					internal: dictString(d, "Type") == "InternalBattery",
					present:  dictBool(d, "Is Present"),
					current:  cur,
					max:      mx,
					charging: dictBool(d, "Is Charging"),
				})
			}
			p.cfRelease(list)
		}
		st = powerStatusFrom(providing, sources, lowPowerMode())
	})
	return st, nil
}

// ---- events ----

type powerWatch struct {
	handler  func(platform.PowerEvent) // main thread
	source   uintptr                   // IOPS run loop source, installed while a handler is set
	observer id                        // TangoPowerObserver
	classes  sync.Once
}

var powerW powerWatch

// iopsChanged is the IOPowerSourceCallbackType; it runs on the main run
// loop, where the source is installed.
var iopsChanged = purego.NewCallback(func(ctx uintptr) uintptr {
	if fn := powerW.handler; fn != nil {
		withPool(func() { fn(platform.PowerStatusChanged) })
	}
	return 0
})

func (w *powerWatch) registerClasses() {
	w.classes.Do(func() {
		fire := func(ev platform.PowerEvent) {
			call := func() {
				if fn := powerW.handler; fn != nil {
					fn(ev)
				}
			}
			if isMainThread() { // NSWorkspace posts on the main thread
				call()
				return
			}
			hopMain(call)
		}
		classDef("TangoPowerObserver", "NSObject", nil, []objc.MethodDef{
			method("willSleep:", func(self id, _ objc.SEL, n id) { fire(platform.PowerSuspending) }),
			method("didWake:", func(self id, _ objc.SEL, n id) { fire(platform.PowerResumed) }),
			// NSProcessInfoPowerStateDidChangeNotification is posted on
			// an arbitrary thread.
			method("powerStateChanged:", func(self id, _ objc.SEL, n id) { fire(platform.PowerStatusChanged) }),
		})
	})
}

func (b *backend) SetPowerHandler(fn func(platform.PowerEvent)) error {
	if err := loadPower(); err != nil {
		return err
	}
	w := &powerW
	p := &iokit
	w.handler = fn
	if fn == nil {
		if w.source != 0 {
			p.cfRemoveSource(b.mainLoop, w.source, kCFRunLoopCommonModes)
			p.cfRelease(w.source)
			w.source = 0
		}
		if w.observer != 0 {
			withPool(func() {
				ws := send(send(class("NSWorkspace"), "sharedWorkspace"), "notificationCenter")
				send(ws, "removeObserver:", uintptr(w.observer))
				send(send(class("NSNotificationCenter"), "defaultCenter"), "removeObserver:", uintptr(w.observer))
			})
			release(w.observer)
			w.observer = 0
		}
		return nil
	}
	if w.source == 0 {
		w.source = p.notifySource(iopsChanged, 0)
		if w.source != 0 {
			p.cfAddSource(b.mainLoop, w.source, kCFRunLoopCommonModes)
		}
	}
	if w.observer == 0 {
		w.registerClasses()
		withPool(func() {
			w.observer = alloc("TangoPowerObserver")
			ws := send(send(class("NSWorkspace"), "sharedWorkspace"), "notificationCenter")
			send(ws, "addObserver:selector:name:object:", uintptr(w.observer), uintptr(sel("willSleep:")),
				uintptr(nsString("NSWorkspaceWillSleepNotification")), 0)
			send(ws, "addObserver:selector:name:object:", uintptr(w.observer), uintptr(sel("didWake:")),
				uintptr(nsString("NSWorkspaceDidWakeNotification")), 0)
			send(send(class("NSNotificationCenter"), "defaultCenter"), "addObserver:selector:name:object:",
				uintptr(w.observer), uintptr(sel("powerStateChanged:")),
				uintptr(nsString("NSProcessInfoPowerStateDidChangeNotification")), 0)
		})
	}
	return nil
}

// ---- sleep blocks ----

// IOPMAssertion types (IOPMLib.h) and level.
const (
	pmAssertPreventIdleSystemSleep  = "PreventUserIdleSystemSleep"
	pmAssertPreventIdleDisplaySleep = "PreventUserIdleDisplaySleep"
	pmAssertionLevelOn              = 255
)

var sleepBlocks = struct {
	next  uint64
	byTok map[uint64]uint32 // token -> IOPMAssertionID
}{byTok: map[uint64]uint32{}}

var errBadSleepBlock = errors.New("tango/darwin: unknown sleep block kind")

func (b *backend) BlockSleep(kind platform.SleepBlock, reason string) (uint64, error) {
	if err := loadPower(); err != nil {
		return 0, err
	}
	typ := ""
	switch kind {
	case platform.BlockSystemSleep:
		typ = pmAssertPreventIdleSystemSleep
	case platform.BlockDisplaySleep:
		typ = pmAssertPreventIdleDisplaySleep // keeps the system awake too
	default:
		return 0, errBadSleepBlock
	}
	var aid uint32
	var st int32
	withPool(func() {
		st = iokit.assertCreate(uintptr(nsString(typ)), pmAssertionLevelOn, uintptr(nsString(reason)), &aid)
	})
	if st != 0 {
		return 0, fmt.Errorf("tango/darwin: IOPMAssertionCreateWithName: IOReturn 0x%x", uint32(st))
	}
	sleepBlocks.next++
	sleepBlocks.byTok[sleepBlocks.next] = aid
	return sleepBlocks.next, nil
}

func (b *backend) UnblockSleep(token uint64) {
	aid, ok := sleepBlocks.byTok[token]
	if !ok {
		return
	}
	delete(sleepBlocks.byTok, token)
	iokit.assertRelease(aid)
}

// SmokeSleepBlocks reports how many sleep blocks are active, for the
// sysx smoke program.
func (b *backend) SmokeSleepBlocks() int { return len(sleepBlocks.byTok) }
