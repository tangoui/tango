package tango

import "github.com/tangoui/tango/internal/platform"

// WebNavigationKind identifies a main-frame load phase or a page metadata event.
type WebNavigationKind = platform.WebNavigationKind

const (
	WebNavigationStarted   = platform.WebNavigationStarted
	WebNavigationCommitted = platform.WebNavigationCommitted
	WebNavigationFinished  = platform.WebNavigationFinished
	WebNavigationFailed    = platform.WebNavigationFailed
	WebPageChanged         = platform.WebPageChanged
)

// WebPageState describes the WebView URL, document title and native history.
type WebPageState = platform.WebPageState

// WebNavigationEvent describes one main-frame load phase or a metadata
// change. ID increases per surface for document navigations. Redirects and
// same-document changes keep the ID. ID zero means no load has been observed.
type WebNavigationEvent struct {
	WebPageState
	ID    uint64
	Kind  WebNavigationKind
	Error string // failure reason, including a superseded navigation
}

// WebNavigation returns the latest load phase and current page metadata.
// It can be called from any goroutine. Read it after registering a handler
// to initialize the address bar even if the initial load already started.
func (s *Surface) WebNavigation() WebNavigationEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.web.navigation
}

// OnWebNavigation observes main-frame events and changes to URL, title
// and history on the main thread. A nil handler is ignored.
func (s *Surface) OnWebNavigation(fn func(WebNavigationEvent)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed.Load() {
		s.web.onNavigation = append(s.web.onNavigation, fn)
	}
}

// GoBack navigates to the preceding entry in the native history. It
// returns false when there is no entry. A navigation policy may veto it.
func (s *Surface) GoBack() (bool, error) {
	return s.goInHistory(true)
}

// GoForward navigates to the following entry in the native history. It
// returns false when there is no entry. A navigation policy may veto it.
func (s *Surface) GoForward() (bool, error) {
	return s.goInHistory(false)
}

func (s *Surface) goInHistory(back bool) (bool, error) {
	return webAsync(s, func(w platform.WebHistory, done func(bool, error)) {
		w.GoInHistory(back, done)
	})
}

func pendingWebNavigation(kind WebNavigationKind) bool {
	return kind == WebNavigationStarted || kind == WebNavigationCommitted
}

func (h host) OnWebNavigation(id platform.SurfaceID, token uint64, kind platform.WebNavigationKind, page platform.WebPageState, reason string) {
	defer h.a.enterMain()()
	s := h.a.surface(id)
	if s == nil || s.closed.Load() {
		return
	}
	s.mu.Lock()
	current := s.web.navigation
	var events []WebNavigationEvent
	switch kind {
	case WebNavigationStarted:
		if pendingWebNavigation(current.Kind) {
			if s.web.navigationToken == token {
				s.web.navigation.URL = page.URL
				s.mu.Unlock()
				return
			}
			current.Kind, current.Error = WebNavigationFailed, "navigation superseded"
			events = append(events, current)
		}
		s.web.navigationToken = token
		current = WebNavigationEvent{ID: current.ID + 1}
	case WebPageChanged:
		if current.WebPageState == page {
			s.mu.Unlock()
			return
		}
		current.WebPageState = page
		s.web.navigation = current
		current.Kind, current.Error = kind, ""
	default:
		if s.web.navigationToken != token || !pendingWebNavigation(current.Kind) || current.Kind == kind {
			s.mu.Unlock()
			return
		}
	}
	if kind != WebPageChanged {
		current.WebPageState, current.Kind, current.Error = page, kind, reason
		s.web.navigation = current
	}
	s.mu.Unlock()
	events = append(events, current)
	for _, ev := range events {
		fire(s, &s.web.onNavigation, func(fn func(WebNavigationEvent)) { fn(ev) })
	}
}
