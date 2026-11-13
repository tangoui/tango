package android

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tangoui/tango/internal/platform"
)

// Pure logic of the system settings the backend follows
// (system_android.go, docs/specs/mobile-canvas.md §3.3): decoding what
// Mobile.java answers.

// javaTheme is the "systemTheme" answer: night mode and, on Android 12+,
// the wallpaper-derived accent color as "#RRGGBB" ("" when unknown).
type javaTheme struct {
	Dark   bool   `json:"dark"`
	Accent string `json:"accent"`
}

func decodeTheme(s string) platform.Theme {
	var j javaTheme
	if json.Unmarshal([]byte(s), &j) != nil {
		return platform.Theme{}
	}
	t := platform.Theme{Dark: j.Dark}
	if h, ok := strings.CutPrefix(j.Accent, "#"); ok && len(h) == 6 {
		if v, err := strconv.ParseUint(h, 16, 32); err == nil {
			t.Accent = uint32(v)<<8 | 0xff
		}
	}
	return t
}

// decodeLocales reads the "locales" answer: a JSON array of BCP 47 tags,
// most preferred first. Empty and duplicate tags are dropped.
func decodeLocales(s string) []string {
	var tags []string
	if json.Unmarshal([]byte(s), &tags) != nil {
		return nil
	}
	out := tags[:0]
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || t == "und" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
