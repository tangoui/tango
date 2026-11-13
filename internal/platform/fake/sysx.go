package fake

import (
	"errors"
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Desktop system capabilities of the fake backend (sysx_ext.go):
// platform.Power, platform.Network, platform.AppShortcuts and
// platform.FileDragger on surfaces. Their state lives beside Backend
// (keyed by it) so this file needs no change to the Backend struct.

var (
	_ platform.Power        = (*Backend)(nil)
	_ platform.Network      = (*Backend)(nil)
	_ platform.AppShortcuts = (*Backend)(nil)
	_ platform.FileDragger  = (*Surface)(nil)
)

// FileDrag records one StartFileDrag call.
type FileDrag struct {
	Surface platform.SurfaceID
	Paths   []string
	Icon    []byte
	done    func(platform.DragOperation)
}

type sysxState struct {
	mu sync.Mutex

	power        platform.PowerStatus
	powerErr     error
	powerHandler func(platform.PowerEvent)
	blocks       map[uint64]platform.SleepBlock
	nextBlock    uint64

	net        platform.NetworkStatus
	netErr     error
	netHandler func(platform.NetworkStatus)

	shortcuts    []platform.ShortcutItem
	shortcutFn   func(string)
	shortcutErr  error
	shortcutSets int

	dragErr error
	drags   []*FileDrag
}

var sysxStates sync.Map // *Backend -> *sysxState

func (b *Backend) sysx() *sysxState {
	if v, ok := sysxStates.Load(b); ok {
		return v.(*sysxState)
	}
	v, _ := sysxStates.LoadOrStore(b, &sysxState{
		power:  platform.PowerStatus{Source: platform.PowerSourceAC, Battery: -1},
		net:    platform.NetworkStatus{Online: true, Kind: platform.NetworkWired},
		blocks: map[uint64]platform.SleepBlock{},
	})
	return v.(*sysxState)
}

// ---- platform.Power ----

func (b *Backend) PowerStatus() (platform.PowerStatus, error) {
	b.mustMain("PowerStatus")
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.power, s.powerErr
}

func (b *Backend) SetPowerHandler(fn func(platform.PowerEvent)) error {
	b.mustMain("SetPowerHandler")
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.powerErr != nil {
		return s.powerErr
	}
	s.powerHandler = fn
	return nil
}

var errBadBlock = errors.New("fake: unknown SleepBlock kind")

func (b *Backend) BlockSleep(kind platform.SleepBlock, reason string) (uint64, error) {
	b.mustMain("BlockSleep")
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.powerErr != nil {
		return 0, s.powerErr
	}
	if kind != platform.BlockSystemSleep && kind != platform.BlockDisplaySleep {
		return 0, errBadBlock
	}
	s.nextBlock++
	s.blocks[s.nextBlock] = kind
	return s.nextBlock, nil
}

func (b *Backend) UnblockSleep(token uint64) {
	b.mustMain("UnblockSleep")
	s := b.sysx()
	s.mu.Lock()
	delete(s.blocks, token)
	s.mu.Unlock()
}

// SetPowerErr makes every Power call fail with err (nil to recover).
func (b *Backend) SetPowerErr(err error) {
	s := b.sysx()
	s.mu.Lock()
	s.powerErr = err
	s.mu.Unlock()
}

// SleepBlocks returns the active sleep blocks by kind.
func (b *Backend) SleepBlocks() []platform.SleepBlock {
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []platform.SleepBlock
	for _, k := range s.blocks {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// SetPower changes the power status and, if a handler is set, reports
// PowerStatusChanged as a native event. It reports whether a handler ran.
func (b *Backend) SetPower(st platform.PowerStatus) bool {
	s := b.sysx()
	s.mu.Lock()
	s.power = st
	s.mu.Unlock()
	return b.EmitPower(platform.PowerStatusChanged)
}

// EmitPower reports ev to the power handler as a native event. It reports
// whether a handler ran.
func (b *Backend) EmitPower(ev platform.PowerEvent) bool {
	ran := false
	b.Do(func() {
		s := b.sysx()
		s.mu.Lock()
		fn := s.powerHandler
		s.mu.Unlock()
		if fn != nil {
			fn(ev)
			ran = true
		}
	})
	return ran
}

// ---- platform.Network ----

func (b *Backend) NetworkStatus() (platform.NetworkStatus, error) {
	b.mustMain("NetworkStatus")
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.net, s.netErr
}

func (b *Backend) SetNetworkHandler(fn func(platform.NetworkStatus)) error {
	b.mustMain("SetNetworkHandler")
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.netErr != nil {
		return s.netErr
	}
	s.netHandler = fn
	return nil
}

// SetNetworkErr makes every Network call fail with err (nil to recover).
func (b *Backend) SetNetworkErr(err error) {
	s := b.sysx()
	s.mu.Lock()
	s.netErr = err
	s.mu.Unlock()
}

// SetNetwork changes the network status and reports it to the handler as a
// native event (even when unchanged, to test coalescing). It reports
// whether a handler ran.
func (b *Backend) SetNetwork(st platform.NetworkStatus) bool {
	s := b.sysx()
	s.mu.Lock()
	s.net = st
	s.mu.Unlock()
	ran := false
	b.Do(func() {
		s.mu.Lock()
		fn, cur := s.netHandler, s.net
		s.mu.Unlock()
		if fn != nil {
			fn(cur)
			ran = true
		}
	})
	return ran
}

// ---- platform.AppShortcuts ----

func (b *Backend) SetAppShortcuts(items []platform.ShortcutItem, onClick func(string)) error {
	b.mustMain("SetAppShortcuts")
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shortcutErr != nil {
		return s.shortcutErr
	}
	s.shortcuts = slices.Clone(items)
	s.shortcutFn = onClick
	s.shortcutSets++
	return nil
}

// SetShortcutErr makes SetAppShortcuts fail with err (nil to recover).
func (b *Backend) SetShortcutErr(err error) {
	s := b.sysx()
	s.mu.Lock()
	s.shortcutErr = err
	s.mu.Unlock()
}

// Shortcuts returns the items last set and how many times they were set.
func (b *Backend) Shortcuts() ([]platform.ShortcutItem, int) {
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.shortcuts), s.shortcutSets
}

// ClickShortcut simulates choosing item id from the Dock menu, as a
// native event. It reports false if no item has that id.
func (b *Backend) ClickShortcut(id string) bool {
	s := b.sysx()
	s.mu.Lock()
	fn := s.shortcutFn
	found := slices.ContainsFunc(s.shortcuts, func(it platform.ShortcutItem) bool { return it.ID == id && !it.Separator })
	s.mu.Unlock()
	if !found || fn == nil {
		return false
	}
	return b.Do(func() { fn(id) })
}

// ---- platform.FileDragger (on Surface) ----

func (s *Surface) StartFileDrag(paths []string, icon []byte, done func(platform.DragOperation)) error {
	s.b.mustMain("StartFileDrag")
	st := s.b.sysx()
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.dragErr != nil {
		return st.dragErr
	}
	st.drags = append(st.drags, &FileDrag{Surface: s.id, Paths: slices.Clone(paths), Icon: icon, done: done})
	return nil
}

// SetDragErr makes StartFileDrag fail with err (nil to recover).
func (b *Backend) SetDragErr(err error) {
	s := b.sysx()
	s.mu.Lock()
	s.dragErr = err
	s.mu.Unlock()
}

// FileDrags returns the drags started so far.
func (b *Backend) FileDrags() []*FileDrag {
	s := b.sysx()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.drags)
}

// Finish ends the drag with op as a native event. It reports false if it
// had already ended.
func (d *FileDrag) Finish(b *Backend, op platform.DragOperation) bool {
	s := b.sysx()
	s.mu.Lock()
	done := d.done
	d.done = nil
	s.mu.Unlock()
	if done == nil {
		return false
	}
	return b.Do(func() { done(op) })
}
