package tango

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestWebDialogReplyAfterCallback(t *testing.T) {
	a, _ := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var answer func(WebDialogResponse)
	results := make(chan WebDialogResponse, 2)
	s.SetWebDialogHandler(func(req WebDialogRequest, reply func(WebDialogResponse)) {
		if req.Kind != WebDialogPrompt {
			t.Errorf("dialog kind = %v", req.Kind)
		}
		answer = reply
	})
	Await(a, func() struct{} {
		if !(host{a}).OnWebDialog(s.id, platform.WebDialogRequest{Kind: platform.WebDialogPrompt}, func(r platform.WebDialogResponse) { results <- r }) {
			t.Error("dialog wasn't intercepted")
		}
		return struct{}{}
	})
	if len(results) != 0 {
		t.Fatal("dialog answered before explicit reply")
	}
	answer(WebDialogResponse{Accepted: true, Text: "value"})
	answer(WebDialogResponse{})
	sync0(a)
	if got := <-results; !got.Accepted || got.Text != "value" || len(results) != 0 {
		t.Fatalf("unexpected reply: %+v (%d extra)", got, len(results))
	}
}
