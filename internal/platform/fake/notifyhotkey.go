package fake

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// Round-2 system integration capabilities of the fake backend:
// platform.Notifier, platform.Hotkeys and platform.ClipboardRich. Their
// state lives beside Backend (keyed by it) so this file needs no change to
// the Backend struct.

type sysState struct {
	mu sync.Mutex

	// Notifier
	permGranted bool
	permErr     error
	permAsked   int
	notifyErr   error
	notes       []platform.Notification
	noteClicks  map[string]func()

	// Hotkeys
	hotkeyErr error
	hotkeys   map[uint32]hotkeyEntry

	// ClipboardRich
	image []byte
	files []string
}

type hotkeyEntry struct {
	accel string
	fire  func()
}

var sysStates sync.Map // *Backend -> *sysState

func (b *Backend) sys() *sysState {
	if v, ok := sysStates.Load(b); ok {
		return v.(*sysState)
	}
	v, _ := sysStates.LoadOrStore(b, &sysState{
		permGranted: true,
		noteClicks:  map[string]func(){},
		hotkeys:     map[uint32]hotkeyEntry{},
	})
	return v.(*sysState)
}

var (
	_ platform.Notifier      = (*Backend)(nil)
	_ platform.Hotkeys       = (*Backend)(nil)
	_ platform.ClipboardRich = (*Backend)(nil)
)

// ---- platform.Notifier ----

// SetNotifyPermission sets what RequestNotifyPermission answers (granted by
// default).
func (b *Backend) SetNotifyPermission(granted bool, err error) {
	s := b.sys()
	s.mu.Lock()
	s.permGranted, s.permErr = granted, err
	s.mu.Unlock()
}

// SetNotifyErr makes Notify fail with err (nil to succeed again).
func (b *Backend) SetNotifyErr(err error) {
	s := b.sys()
	s.mu.Lock()
	s.notifyErr = err
	s.mu.Unlock()
}

// RequestNotifyPermission answers asynchronously, as a later native event.
func (b *Backend) RequestNotifyPermission(done func(granted bool, err error)) {
	b.mustMain("RequestNotifyPermission")
	s := b.sys()
	s.mu.Lock()
	s.permAsked++
	g, err := s.permGranted, s.permErr
	s.mu.Unlock()
	b.enqueue(func() { done(g, err) })
}

func (b *Backend) Notify(n platform.Notification, onClick func()) error {
	b.mustMain("Notify")
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notifyErr != nil {
		return s.notifyErr
	}
	s.notes = append(s.notes, n)
	s.noteClicks[n.ID] = onClick
	return nil
}

// Notifications returns every notification shown, in order.
func (b *Backend) Notifications() []platform.Notification {
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.notes)
}

// NotifyPermissionRequests counts RequestNotifyPermission calls.
func (b *Backend) NotifyPermissionRequests() int {
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.permAsked
}

// ClickNotification simulates the user clicking notification id. It
// reports false if no such notification was shown.
func (b *Backend) ClickNotification(id string) bool {
	s := b.sys()
	s.mu.Lock()
	fn, ok := s.noteClicks[id]
	s.mu.Unlock()
	if !ok {
		return false
	}
	return b.Do(func() {
		if fn != nil {
			fn()
		}
	})
}

// ---- platform.Hotkeys ----

// SetHotkeyErr makes RegisterHotkey fail with err (nil to succeed again).
func (b *Backend) SetHotkeyErr(err error) {
	s := b.sys()
	s.mu.Lock()
	s.hotkeyErr = err
	s.mu.Unlock()
}

func (b *Backend) RegisterHotkey(id uint32, accelerator string, fire func()) error {
	b.mustMain("RegisterHotkey")
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hotkeyErr != nil {
		return s.hotkeyErr
	}
	s.hotkeys[id] = hotkeyEntry{accelerator, fire}
	return nil
}

func (b *Backend) UnregisterHotkey(id uint32) {
	b.mustMain("UnregisterHotkey")
	s := b.sys()
	s.mu.Lock()
	delete(s.hotkeys, id)
	s.mu.Unlock()
}

// Hotkeys returns the registered accelerators by id.
func (b *Backend) Hotkeys() map[uint32]string {
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	m := make(map[uint32]string, len(s.hotkeys))
	for id, e := range s.hotkeys {
		m[id] = e.accel
	}
	return m
}

// PressHotkey simulates the user pressing every registered hotkey whose
// accelerator equals accel. It reports how many fired.
func (b *Backend) PressHotkey(accel string) int {
	s := b.sys()
	s.mu.Lock()
	var fns []func()
	for _, e := range s.hotkeys {
		if e.accel == accel {
			fns = append(fns, e.fire)
		}
	}
	s.mu.Unlock()
	if len(fns) == 0 {
		return 0
	}
	b.Do(func() {
		for _, fn := range fns {
			fn()
		}
	})
	return len(fns)
}

// ---- platform.ClipboardRich ----

func (b *Backend) ClipboardReadImage() ([]byte, error) {
	b.mustMain("ClipboardReadImage")
	if b.ClipboardErr != nil {
		return nil, b.ClipboardErr
	}
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.image), nil
}

// ClipboardWriteImage replaces the clipboard (text and files included).
func (b *Backend) ClipboardWriteImage(png []byte) error {
	b.mustMain("ClipboardWriteImage")
	if b.ClipboardErr != nil {
		return b.ClipboardErr
	}
	b.setClipboard("", slices.Clone(png), nil)
	return nil
}

func (b *Backend) ClipboardReadFiles() ([]string, error) {
	b.mustMain("ClipboardReadFiles")
	if b.ClipboardErr != nil {
		return nil, b.ClipboardErr
	}
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.files), nil
}

// ClipboardWriteFiles replaces the clipboard with a file list.
func (b *Backend) ClipboardWriteFiles(paths []string) error {
	b.mustMain("ClipboardWriteFiles")
	if b.ClipboardErr != nil {
		return b.ClipboardErr
	}
	b.setClipboard("", nil, slices.Clone(paths))
	return nil
}

func (b *Backend) setClipboard(text string, image []byte, files []string) {
	b.mu.Lock()
	b.clip = text
	b.mu.Unlock()
	s := b.sys()
	s.mu.Lock()
	s.image, s.files = image, files
	s.mu.Unlock()
}

// ClipboardImage returns the fake clipboard's image bytes.
func (b *Backend) ClipboardImage() []byte {
	s := b.sys()
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.image)
}

// SetClipboardFiles simulates another app copying files: it replaces the
// clipboard's content with paths.
func (b *Backend) SetClipboardFiles(paths []string) { b.setClipboard("", nil, slices.Clone(paths)) }

// SetClipboardImage simulates another app copying an image.
func (b *Backend) SetClipboardImage(png []byte) { b.setClipboard("", slices.Clone(png), nil) }
