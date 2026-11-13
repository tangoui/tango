package fake

// Window capabilities of the fake backend (contract round 2, ext.go):
// CreatorEx on Backend; Geometry, Stateful, Styled, PageControl and Dragger
// on Surface; helpers that simulate the matching native events.

import (
	"slices"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CreatorEx   = (*Backend)(nil)
	_ platform.Geometry    = (*Surface)(nil)
	_ platform.Stateful    = (*Surface)(nil)
	_ platform.Styled      = (*Surface)(nil)
	_ platform.PageControl = (*Surface)(nil)
	_ platform.Dragger     = (*Surface)(nil)
)

// winState is a fake surface's window state, kept beside Surface so the
// round-1 type stays as it is.
type winState struct {
	mu          sync.Mutex
	ex          platform.SurfaceExtras
	hasEx       bool
	x, y        int
	minW, minH  int
	maxW, maxH  int
	state       platform.WindowState
	appearance  platform.Appearance
	appearances int
	zoom        float64
	reloads     int
	prints      int
	moves       int
	resizes     []string
	centers     int
}

var winStates sync.Map // *Surface -> *winState

func (s *Surface) win() *winState {
	if w, ok := winStates.Load(s); ok {
		return w.(*winState)
	}
	w, _ := winStates.LoadOrStore(s, &winState{zoom: 1})
	return w.(*winState)
}

// NewSurfaceEx implements platform.CreatorEx.
func (b *Backend) NewSurfaceEx(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras) (platform.Surface, error) {
	b.mustMain("NewSurfaceEx")
	ns, err := b.NewSurface(id, opts)
	if err != nil {
		return nil, err
	}
	s := ns.(*Surface)
	w := s.win()
	w.mu.Lock()
	w.ex, w.hasEx = ex, true
	if !ex.Centered {
		w.x, w.y = ex.Bounds.X, ex.Bounds.Y
	}
	w.minW, w.minH, w.maxW, w.maxH = opts.MinWidth, opts.MinHeight, ex.MaxWidth, ex.MaxHeight
	w.state = ex.State
	w.appearance = ex.Appearance
	w.mu.Unlock()
	return s, nil
}

// ---- platform.Geometry ----

func (s *Surface) Bounds() platform.Rect {
	s.b.mustMain("Bounds")
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return platform.Rect{X: w.x, Y: w.y, Width: s.w, Height: s.h}
}

func (s *Surface) SetBounds(r platform.Rect) {
	s.b.mustMain("SetBounds")
	w := s.win()
	w.mu.Lock()
	w.x, w.y = r.X, r.Y
	w.mu.Unlock()
	s.mu.Lock()
	s.w, s.h = r.Width, r.Height
	s.mu.Unlock()
}

func (s *Surface) Center() {
	s.b.mustMain("Center")
	w := s.win()
	w.mu.Lock()
	w.centers++
	w.mu.Unlock()
}

func (s *Surface) SetSizeLimits(minW, minH, maxW, maxH int) {
	s.b.mustMain("SetSizeLimits")
	w := s.win()
	w.mu.Lock()
	w.minW, w.minH, w.maxW, w.maxH = minW, minH, maxW, maxH
	w.mu.Unlock()
}

// ---- platform.Stateful ----

func (s *Surface) State() platform.WindowState {
	s.b.mustMain("State")
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// SetState changes the state and, like a real window, reports the change
// as a later native event.
func (s *Surface) SetState(st platform.WindowState) {
	s.b.mustMain("SetState")
	w := s.win()
	w.mu.Lock()
	changed := w.state != st
	w.state = st
	w.mu.Unlock()
	if changed {
		s.b.enqueue(func() {
			if !s.Closed() {
				s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceStateChanged, State: st})
			}
		})
	}
}

// ---- platform.Styled ----

func (s *Surface) SetAppearance(a platform.Appearance) {
	s.b.mustMain("SetAppearance")
	w := s.win()
	w.mu.Lock()
	w.appearance = a
	w.appearances++
	w.mu.Unlock()
}

// ---- platform.PageControl ----

// Reload emits SurfaceNavigating (and SurfaceLoaded with AutoLoad) as
// queued native events.
func (s *Surface) Reload() {
	s.b.mustMain("Reload")
	w := s.win()
	w.mu.Lock()
	w.reloads++
	w.mu.Unlock()
	url := s.URL()
	s.b.enqueue(func() {
		if s.Closed() {
			return
		}
		s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceNavigating, URL: url})
		if s.b.AutoLoad {
			s.b.emitLoaded(s, url)
		}
	})
}

func (s *Surface) SetZoom(f float64) {
	s.b.mustMain("SetZoom")
	w := s.win()
	w.mu.Lock()
	w.zoom = f
	w.mu.Unlock()
}

func (s *Surface) Zoom() float64 {
	s.b.mustMain("Zoom")
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.zoom
}

func (s *Surface) Print() {
	s.b.mustMain("Print")
	w := s.win()
	w.mu.Lock()
	w.prints++
	w.mu.Unlock()
}

// ---- platform.Dragger ----

func (s *Surface) StartMove() {
	s.b.mustMain("StartMove")
	w := s.win()
	w.mu.Lock()
	w.moves++
	w.mu.Unlock()
}

func (s *Surface) StartResize(edge string) {
	s.b.mustMain("StartResize")
	w := s.win()
	w.mu.Lock()
	w.resizes = append(w.resizes, edge)
	w.mu.Unlock()
}

// ---- accessors (any goroutine) ----

// Extras returns the extras the surface was created with, and whether it
// was created with NewSurfaceEx.
func (s *Surface) Extras() (platform.SurfaceExtras, bool) {
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ex, w.hasEx
}

// SizeLimits returns the limits last set.
func (s *Surface) SizeLimits() (minW, minH, maxW, maxH int) {
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.minW, w.minH, w.maxW, w.maxH
}

// CurrentAppearance returns the appearance last applied and how many times
// SetAppearance was called.
func (s *Surface) CurrentAppearance() (platform.Appearance, int) {
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.appearance, w.appearances
}

// WindowCounts returns how often Reload, Print, Center and StartMove were
// called.
func (s *Surface) WindowCounts() (reloads, prints, centers, moves int) {
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reloads, w.prints, w.centers, w.moves
}

// Resizes returns the edges StartResize was called with.
func (s *Surface) Resizes() []string {
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.resizes)
}

// CurrentState returns the window state without the main-thread check.
func (s *Surface) CurrentState() platform.WindowState {
	w := s.win()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// ---- simulated native events (any goroutine) ----

func (b *Backend) surfaceEvent(id platform.SurfaceID, ev platform.SurfaceEvent, apply func(s *Surface)) bool {
	return b.Do(func() {
		s := b.Surface(id)
		if s == nil || s.Closed() {
			return
		}
		if apply != nil {
			apply(s)
		}
		b.h.OnSurfaceEvent(id, ev)
	})
}

// Navigating simulates the main frame starting a navigation to url.
func (b *Backend) Navigating(id platform.SurfaceID, url string) bool {
	return b.surfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceNavigating, URL: url}, nil)
}

// LoadFailed simulates a failed main-frame navigation.
func (b *Backend) LoadFailed(id platform.SurfaceID, url, err string) bool {
	return b.surfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceLoadFailed, URL: url, Error: err}, nil)
}

// Move simulates the user moving the window.
func (b *Backend) Move(id platform.SurfaceID, x, y int) bool {
	return b.surfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceMoved, X: x, Y: y}, func(s *Surface) {
		w := s.win()
		w.mu.Lock()
		w.x, w.y = x, y
		w.mu.Unlock()
	})
}

// ChangeState simulates the user minimizing / maximizing / restoring.
func (b *Backend) ChangeState(id platform.SurfaceID, st platform.WindowState) bool {
	return b.surfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceStateChanged, State: st}, func(s *Surface) {
		w := s.win()
		w.mu.Lock()
		w.state = st
		w.mu.Unlock()
	})
}

// DropFiles simulates files dropped on the page at x, y.
func (b *Backend) DropFiles(id platform.SurfaceID, paths []string, x, y int) bool {
	return b.surfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: slices.Clone(paths), X: x, Y: y}, nil)
}
