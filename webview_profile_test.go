package tango

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestWebPageModeDisablesBridge(t *testing.T) {
	a, fb := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{URL: "https://page.test/", WebView: WebViewOptions{Page: true}})
	if err != nil {
		t.Fatal(err)
	}
	Await(a, func() struct{} {
		if s.trusts("https://page.test") || s.trusts(a.assetOrigin()) {
			t.Error("ordinary page can access the app bridge")
		}
		return struct{}{}
	})
	if !fb.ShouldNavigate(s.ID(), platform.NavigationRequest{URL: "https://other.test/"}) {
		t.Error("ordinary cross-origin navigation blocked")
	}
	if fb.ShouldNavigate(s.ID(), platform.NavigationRequest{URL: "file:///tmp/file"}) {
		t.Error("ordinary page is allowed to navigate to a local file")
	}
}

func TestCancelWebDownloadWhileChoosing(t *testing.T) {
	a, _ := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var reply func(string)
	s.SetDownloadDestinationHandler(func(_ DownloadRequest, answer func(string)) { reply = answer })
	results := make(chan string, 2)
	events := make(chan DownloadEvent, 2)
	s.OnDownloadEvent(func(ev DownloadEvent) { events <- ev })
	Await(a, func() struct{} {
		(host{a}).OnDownloadStarted(s.id, DownloadRequest{ID: 7}, func(path string) { results <- path })
		return struct{}{}
	})
	if len(results) != 0 || reply == nil {
		t.Fatal("destination was not deferred")
	}
	if err := s.CancelDownload(7); err != nil {
		t.Fatal(err)
	}
	reply("late.txt")
	sync0(a)
	if got := <-results; got != "" || len(results) != 0 {
		t.Fatalf("native answer = %q with %d extra", got, len(results))
	}
	if ev := <-events; ev.ID != 7 || ev.State != DownloadCanceled || len(events) != 0 {
		t.Fatalf("event = %+v with %d extra", ev, len(events))
	}
}
