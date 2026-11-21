package tango

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestWebNavigationGeneration(t *testing.T) {
	a, _ := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var events []WebNavigationEvent
	s.OnWebNavigation(func(ev WebNavigationEvent) { events = append(events, ev) })
	Await(a, func() struct{} {
		h := host{a}
		h.OnWebNavigation(s.id, 10, platform.WebNavigationStarted, platform.WebPageState{URL: "https://example.test/"}, "")
		h.OnWebNavigation(s.id, 10, platform.WebNavigationStarted, platform.WebPageState{URL: "https://example.test/final"}, "")
		h.OnWebNavigation(s.id, 10, platform.WebNavigationCommitted, platform.WebPageState{URL: "https://example.test/final"}, "")
		h.OnWebNavigation(s.id, 10, platform.WebNavigationFinished, platform.WebPageState{URL: "https://example.test/final", Title: "Final"}, "")
		h.OnWebNavigation(s.id, 0, platform.WebPageChanged, platform.WebPageState{URL: "https://example.test/final#spa", Title: "SPA", CanGoBack: true}, "")
		h.OnWebNavigation(s.id, 11, platform.WebNavigationStarted, platform.WebPageState{URL: "https://example.test/next"}, "")
		h.OnWebNavigation(s.id, 10, platform.WebNavigationFinished, platform.WebPageState{URL: "https://example.test/stale"}, "")
		h.OnWebNavigation(s.id, 11, platform.WebNavigationFailed, platform.WebPageState{URL: "https://example.test/final#spa", Title: "SPA", CanGoBack: true}, "connection refused")
		return struct{}{}
	})
	kinds := []WebNavigationKind{WebNavigationStarted, WebNavigationCommitted, WebNavigationFinished, WebPageChanged, WebNavigationStarted, WebNavigationFailed}
	if len(events) != len(kinds) {
		t.Fatalf("events = %+v", events)
	}
	for i, k := range kinds {
		wantID := uint64(1)
		if i >= 4 {
			wantID = 2
		}
		if events[i].Kind != k || events[i].ID != wantID {
			t.Errorf("event %d = %+v", i, events[i])
		}
	}
	if got := s.WebNavigation(); got.Kind != WebNavigationFailed || got.Error == "" || !got.CanGoBack || got.Title != "SPA" {
		t.Fatalf("snapshot = %+v", got)
	}
}
