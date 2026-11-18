package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Goldens from the Tango fixture's /setup (tools/editor-gui-bench/fixture).
func TestDocumentsMatchTangoFixture(t *testing.T) {
	for _, c := range []struct {
		scenario string
		carets   int
		bytes    int
		sum      string
		offs     []int
	}{
		{"type", 1, 122779, "dbee5b8a51b8222e", []int{1115}},
		{"multi", 3, 374779, "0217bdb73deec4ae", []int{1115, 1449, 1677}},
	} {
		text, offs := document(c.scenario, c.carets)
		if len(text) != c.bytes || checksum(text) != c.sum {
			t.Errorf("%s: %d bytes %s, want %d %s", c.scenario, len(text), checksum(text), c.bytes, c.sum)
		}
		if len(offs) != len(c.offs) {
			t.Fatalf("%s: carets %v", c.scenario, offs)
		}
		for i := range offs {
			if offs[i] != c.offs[i] {
				t.Errorf("%s: carets %v, want %v", c.scenario, offs, c.offs)
			}
		}
	}
}

func TestControlRequiresToken(t *testing.T) {
	h := controlHandler("secret", func(r request) (any, error) { return map[string]string{"scenario": r.Scenario}, nil })
	for _, c := range []struct {
		auth, method string
		want         int
	}{
		{"", "POST", 401}, {"Bearer wrong", "POST", 401}, {"Bearer secret", "GET", 405}, {"Bearer secret", "POST", 200},
	} {
		req := httptest.NewRequest(c.method, "/state", strings.NewReader("{}"))
		if c.auth != "" {
			req.Header.Set("Authorization", c.auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s %q: %d, want %d", c.method, c.auth, rec.Code, c.want)
		}
	}
	req := httptest.NewRequest("POST", "/setup", strings.NewReader(`{"scenario":"type","bogus":1}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field: %d, want 400", rec.Code)
	}
}
