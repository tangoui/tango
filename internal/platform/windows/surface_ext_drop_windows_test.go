//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"
	"time"
)

func TestIsDropNavigation(t *testing.T) {
	now := time.Now()
	recent := now.Add(-300 * time.Millisecond)
	file := "file:///C:/Users/me/a.txt"
	cases := []struct {
		name      string
		accept    bool
		uri, self string
		seen      time.Time
		want      bool
	}{
		{"drop during a drag session", true, file, "", recent, true},
		{"surface does not accept files", false, file, "", recent, false},
		{"not a file URL", true, "https://example.com/", "", recent, false},
		{"no drag reported", true, file, "", time.Time{}, false},
		{"drag session expired", true, file, "", now.Add(-wxDragSessionWindow - time.Millisecond), false},
		{"report from the future (clock change)", true, file, "", now.Add(time.Second), false},
		{"the app loads the file itself", true, file, file, recent, false},
		{"the app loaded another file", true, file, "file:///C:/other.html", recent, true},
	}
	for _, c := range cases {
		if got := isDropNavigation(c.accept, c.uri, c.self, c.seen, now); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
