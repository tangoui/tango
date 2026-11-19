//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// tan mcp's own TANGO_LAUNCH_BACKGROUND=0 does not reach a started app:
// the background launch is forced. The "app" is a script that names the
// channel's variable (so it passes for a tango_dev build), writes what it
// got and exits.
func TestUIStartForcesLaunchBackground(t *testing.T) {
	t.Setenv("TANGO_LAUNCH_BACKGROUND", "0")
	s, cs := projectServer(t)
	out := filepath.Join(t.TempDir(), "env")
	script := "#!/bin/sh\n# TANGO_AUTOMATION_TOKEN_FILE\nprintf %s \"$TANGO_LAUNCH_BACKGROUND\" > \"$OUT\"\n"
	if err := os.WriteFile(filepath.Join(s.root, "app.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	callErr(t, cs, "tango_app_start", map[string]any{"target": "app.sh", "env": []string{"OUT=" + out}, "timeoutMs": 5000})
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "1" {
		t.Errorf("TANGO_LAUNCH_BACKGROUND=%q in the app, want 1", b)
	}
}
