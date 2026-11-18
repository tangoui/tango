package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
)

func TestFixtureAuthenticationAndExactSetup(t *testing.T) {
	calls := 0
	h := controlHandler("secret", func(req request) (any, error) { calls++; return map[string]any{"scenario": req.Scenario}, nil })
	r := httptest.NewRequest("POST", "/setup", strings.NewReader(`{"scenario":"ime"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || calls != 0 {
		t.Fatal("unauthenticated command reached UI")
	}
	r = httptest.NewRequest("POST", "/setup", strings.NewReader(`{"scenario":"ime"}`))
	r.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || calls != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	text := typingDocument("aa\nbb\ncc\n", 1, 2)
	if text != "aa\n// qq"+strings.Repeat(" ", 160)+"\ncc\n" {
		t.Fatalf("%q", text)
	}
	r = httptest.NewRequest("POST", "/setup", strings.NewReader(`{"scenario":"ime","unknown":1}`))
	r.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 400 || calls != 1 {
		b, _ := io.ReadAll(w.Result().Body)
		t.Fatalf("unknown fields accepted: %s", b)
	}
	for _, path := range []string{"/undo", "/redo"} {
		r = httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer secret")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	if calls != 3 {
		t.Fatalf("undo/redo did not reach UI handler: %d", calls)
	}
	st := editor.NewState("ab", "a.txt", theme.Base("vs"))
	defer st.Close()
	st.SetSelections([]editor.Selection{{Anchor: 2, Head: 2}})
	st.Insert("你好")
	if got := historyText(st, "undo"); got != "ab" {
		t.Fatalf("undo text = %q", got)
	}
	if got := historyText(st, "redo"); got != "ab你好" {
		t.Fatalf("redo text = %q", got)
	}
}

// The observer's host time shares the probe's and the Metal trace's clock
// (uptime nanoseconds on macOS), so input records line up with frames.
func TestHostNSIsMonotonic(t *testing.T) {
	a := hostNS()
	b := hostNS()
	if a <= 0 || b < a {
		t.Fatalf("hostNS %d then %d", a, b)
	}
}
