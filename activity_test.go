package tango

import (
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

func TestSurfaceActivityFromEmbeddedView(t *testing.T) {
	a, _ := newTestApp(t)
	parent, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	Await(a, func() bool {
		child, _, err := parent.embedWebView(SurfaceOptions{}, "")
		if err != nil {
			t.Error(err)
			return false
		}
		h := host{a: a}
		h.OnSurfaceEvent(parent.id, platform.SurfaceEvent{Kind: platform.SurfaceFocused})
		h.OnMessage(child.id, "https://example.org", activityPrefix+"invalid")
		if !parent.LastActivityAt().IsZero() {
			t.Error("focus or invalid WebView signal counted as activity")
		}
		before := time.Now()
		h.OnMessage(child.id, "https://example.org", activityPrefix+a.activityToken)
		at := parent.LastActivityAt()
		if at.Before(before) || !at.Equal(child.LastActivityAt()) || !other.LastActivityAt().IsZero() {
			t.Errorf("wrong activity: parent %v, child %v, other %v", at, child.LastActivityAt(), other.LastActivityAt())
		}
		parent.Hide()
		if !parent.LastActivityAt().Equal(at) {
			t.Error("hiding the parent changed its activity")
		}
		return true
	})
}
