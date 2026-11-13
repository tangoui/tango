// Package power reports the machine's power state (AC or battery, charge,
// power-saving mode), tells the app when the system sleeps and wakes, and
// keeps the machine awake while the app plays video or runs a long task.
//
//	st, err := power.Get(a)              // st.Source == power.Battery, st.Battery == 0.42
//	power.OnSuspend(a, pauseSync)
//	power.OnResume(a, resumeSync)
//	blk, err := power.KeepDisplayOn(a, "Playing video")
//	defer blk.Release()
//
// Platforms: macOS uses IOKit (IOPowerSources, IOPMAssertion, sleep/wake
// notifications) and NSProcessInfo's Low Power Mode; Windows
// GetSystemPowerStatus, SetThreadExecutionState and WM_POWERBROADCAST;
// Linux UPower and systemd-logind (Inhibit, PrepareForSleep) over the
// system D-Bus. A sleep the user asks for (closing the lid, the Sleep
// command) is never blocked: blocks only stop idle sleep.
//
// On iOS, UIDevice battery monitoring and NSProcessInfo's Low Power Mode;
// both block kinds disable the idle timer (the screen stays on: an app
// cannot keep the device awake with the screen off). On Android, the
// battery broadcast and PowerManager's power-saver mode; KeepDisplayOn
// sets FLAG_KEEP_SCREEN_ON, KeepAwake holds a partial wake lock when the
// app declares android.permission.WAKE_LOCK and keeps the screen on
// otherwise. Neither mobile system tells apps about sleep and wake, so
// OnSuspend and OnResume never fire there; use the app lifecycle.
//
// Functions are safe from any goroutine once the app runs; handlers run
// on the main thread. On* may be called before Run. Get returns
// ErrUnsupported where the backend has no power support (minimal
// backends) and tango.ErrNotRunning after the app quit; handlers then
// simply never fire.
package power

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

// Source is where the machine draws power from.
type Source = platform.PowerSource

const (
	Unknown = platform.PowerSourceUnknown
	AC      = platform.PowerSourceAC
	Battery = platform.PowerSourceBattery
)

// Status is a snapshot of the power state. Battery is the charge in [0,1],
// -1 when unknown or there is no battery.
type Status = platform.PowerStatus

// Get returns the current power state.
func Get(a *tango.App) (Status, error) {
	p, err := capcall.Backend[platform.Power](a)
	if err != nil {
		return Status{Battery: -1}, err
	}
	return capcall.Sync(a, p.PowerStatus)
}

type appState struct {
	a         *tango.App
	mu        sync.Mutex
	onChange  []func(Status)
	onSuspend []func()
	onResume  []func()
	hooked    bool

	last    Status // main thread
	hasLast bool
}

var states sync.Map // *tango.App -> *appState

func state(a *tango.App) *appState {
	v, _ := states.LoadOrStore(a, &appState{a: a})
	return v.(*appState)
}

// OnChange calls fn with the new status whenever the power source, the
// charge or the power-saving mode changes.
func OnChange(a *tango.App, fn func(Status)) {
	add(a, func(st *appState) { st.onChange = append(st.onChange, fn) })
}

// OnSuspend calls fn when the system is about to sleep. The system does
// not wait long: save state quickly.
func OnSuspend(a *tango.App, fn func()) {
	add(a, func(st *appState) { st.onSuspend = append(st.onSuspend, fn) })
}

// OnResume calls fn after the system woke up.
func OnResume(a *tango.App, fn func()) {
	add(a, func(st *appState) { st.onResume = append(st.onResume, fn) })
}

func add(a *tango.App, reg func(*appState)) {
	st := state(a)
	st.mu.Lock()
	reg(st)
	hook := !st.hooked
	st.hooked = true
	st.mu.Unlock()
	if !hook {
		return
	}
	p, ok := a.Backend().(platform.Power)
	if !ok {
		return
	}
	a.OnReady(func() {
		if s, err := p.PowerStatus(); err == nil {
			st.last, st.hasLast = s, true
		}
		_ = p.SetPowerHandler(func(ev platform.PowerEvent) { st.dispatch(p, ev) })
	})
}

// dispatch runs on the main thread, called by the backend directly: each
// handler is guarded on its own (Where "event handler") so a panic in one
// does not skip the rest.
func (st *appState) dispatch(p platform.Power, ev platform.PowerEvent) {
	st.mu.Lock()
	change := slices.Clone(st.onChange)
	suspend := slices.Clone(st.onSuspend)
	resume := slices.Clone(st.onResume)
	st.mu.Unlock()
	switch ev {
	case platform.PowerSuspending:
		for _, fn := range suspend {
			corehook.Guard(st.a, "event handler", fn)
		}
	case platform.PowerResumed:
		for _, fn := range resume {
			corehook.Guard(st.a, "event handler", fn)
		}
		// The source may have changed while asleep.
		st.report(p, change)
	case platform.PowerStatusChanged:
		st.report(p, change)
	}
}

func (st *appState) report(p platform.Power, fns []func(Status)) {
	s, err := p.PowerStatus()
	if err != nil || (st.hasLast && s == st.last) {
		return
	}
	st.last, st.hasLast = s, true
	for _, fn := range fns {
		corehook.Guard(st.a, "event handler", func() { fn(s) })
	}
}

// Block is one active sleep block. Release it when the work is done; the
// system drops it when the process exits.
type Block struct {
	a     *tango.App
	p     platform.Power
	mu    sync.Mutex
	token uint64
}

// KeepAwake stops the system from idle-sleeping (the display may still
// turn off) until the block is released. Use it for long tasks.
func KeepAwake(a *tango.App, reason string) (*Block, error) {
	return block(a, platform.BlockSystemSleep, reason)
}

// KeepDisplayOn keeps the display on and the system awake until the
// block is released. Use it for video playback and presentations.
func KeepDisplayOn(a *tango.App, reason string) (*Block, error) {
	return block(a, platform.BlockDisplaySleep, reason)
}

func block(a *tango.App, kind platform.SleepBlock, reason string) (*Block, error) {
	p, err := capcall.Backend[platform.Power](a)
	if err != nil {
		return nil, err
	}
	if reason == "" {
		reason = a.Name()
	}
	if reason == "" {
		reason = "Tango app"
	}
	tok, err := capcall.Sync(a, func() (uint64, error) { return p.BlockSleep(kind, reason) })
	if err != nil {
		return nil, err
	}
	return &Block{a: a, p: p, token: tok}, nil
}

// Release ends the block. Releasing twice does nothing.
func (b *Block) Release() {
	if b == nil {
		return
	}
	b.mu.Lock()
	tok := b.token
	b.token = 0
	b.mu.Unlock()
	if tok == 0 {
		return
	}
	b.a.Post(func() { b.p.UnblockSleep(tok) })
}

// Active reports whether the block has not been released.
func (b *Block) Active() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.token != 0
}
