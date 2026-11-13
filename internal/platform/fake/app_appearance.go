package fake

import (
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.AppAppearance = (*Backend)(nil)

type appAppearanceState struct {
	mu    sync.Mutex
	modes []string
}

var appAppearances sync.Map // *Backend -> *appAppearanceState

func (b *Backend) appAppearance() *appAppearanceState {
	s, _ := appAppearances.LoadOrStore(b, &appAppearanceState{})
	return s.(*appAppearanceState)
}

// SetAppAppearance implements platform.AppAppearance by recording mode; it
// does not change SystemTheme (tests call SetTheme for that).
func (b *Backend) SetAppAppearance(mode string) {
	b.mustMain("SetAppAppearance")
	s := b.appAppearance()
	s.mu.Lock()
	s.modes = append(s.modes, mode)
	s.mu.Unlock()
}

// AppAppearanceModes returns the modes SetAppAppearance received, in order.
func (b *Backend) AppAppearanceModes() []string {
	s := b.appAppearance()
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.modes...)
}
