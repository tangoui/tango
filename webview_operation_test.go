package tango

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestWebViewOperationRejectsStaleLayout(t *testing.T) {
	a, _ := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	Await(a, func() struct{} {
		s.embedIn = s
		s.commitWebPlacement(platform.WebPlacement{
			Frame: platform.RectF{W: 100, H: 100},
			Clip:  platform.RectF{W: 100, H: 100},
		})
		h := host{a}
		h.OnWebNavigation(s.id, 1, platform.WebNavigationStarted, platform.WebPageState{}, "")
		h.OnWebNavigation(s.id, 1, platform.WebNavigationFinished, platform.WebPageState{}, "")
		return struct{}{}
	})
	stamp, err := s.WebViewStamp()
	if err != nil {
		t.Fatal(err)
	}
	Await(a, func() struct{} {
		s.commitWebPlacement(platform.WebPlacement{
			Frame: platform.RectF{W: 100, H: 100},
			Clip:  platform.RectF{W: 100, H: 80},
		})
		return struct{}{}
	})
	if _, err := s.CaptureWebView(stamp); !errors.Is(err, ErrStaleWebView) {
		t.Fatalf("capture with stale placement = %v", err)
	}
	stamp, err = s.WebViewStamp()
	if err != nil {
		t.Fatal(err)
	}
	Await(a, func() struct{} {
		(host{a}).OnWebNavigation(s.id, 2, platform.WebNavigationStarted, platform.WebPageState{}, "")
		return struct{}{}
	})
	if _, err := s.CaptureWebView(stamp); !errors.Is(err, ErrStaleWebView) {
		t.Fatalf("capture with stale navigation = %v", err)
	}
}
