//go:build !tango_dev

package devfeat_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

// In release builds TANGO_DEV_WINDOW_STATE is ignored.
func TestDevStateReleaseOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "windows.json")
	data, _ := json.Marshal(map[string]map[string]int{"W": {"x": 300, "y": 200, "width": 640, "height": 480}})
	os.WriteFile(path, data, 0o644)
	t.Setenv("TANGO_DEV_WINDOW_STATE", path)
	a, fb := faketest.Run(t, tango.Options{})
	s, err := a.NewSurface(tango.SurfaceOptions{Title: "W", Width: 400, Height: 300, X: 10, Y: 30})
	if err != nil {
		t.Fatal(err)
	}
	if b := s.Bounds(); b != (tango.Rect{X: 10, Y: 30, Width: 400, Height: 300}) {
		t.Fatalf("restored in a release build: %+v", b)
	}
	fb.Move(s.ID(), 1, 2)
	time.Sleep(400 * time.Millisecond)
	if got, _ := os.ReadFile(path); string(got) != string(data) {
		t.Fatalf("release build wrote the file: %s", got)
	}
}
