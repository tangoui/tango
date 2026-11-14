package pty

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLingerDelay(t *testing.T) {
	exit := time.Unix(1000, 0)
	ms := time.Millisecond
	at := func(d time.Duration) time.Time { return exit.Add(d) }
	for _, tc := range []struct {
		name      string
		last, now time.Time
		want      time.Duration
	}{
		{"no output yet: quiet from the exit", time.Time{}, at(0), 100 * ms},
		{"no output, quiet long enough", time.Time{}, at(100 * ms), 0},
		{"output before the exit counts as at the exit", at(-50 * ms), at(30 * ms), 70 * ms},
		{"recent output extends the wait", at(250 * ms), at(300 * ms), 50 * ms},
		{"quiet after output", at(250 * ms), at(350 * ms), 0},
		{"bounded by maxLinger", at(980 * ms), at(990 * ms), 10 * ms},
		{"past maxLinger", at(1500 * ms), at(1600 * ms), -600 * ms},
	} {
		if got := lingerDelay(exit, tc.last, tc.now); got != tc.want {
			t.Errorf("%s: lingerDelay = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAddSystemRoot(t *testing.T) {
	join := func(e []string) string { return strings.Join(e, ";") }
	if got := addSystemRoot([]string{"A=1"}, `C:\Windows`); join(got) != `A=1;SYSTEMROOT=C:\Windows` {
		t.Errorf("missing: got %q", got)
	}
	if got := addSystemRoot([]string{"SystemRoot=D:\\W", "A=1"}, `C:\Windows`); join(got) != `SystemRoot=D:\W;A=1` {
		t.Errorf("present (other case): got %q", got)
	}
	if got := addSystemRoot([]string{"A=1"}, ""); join(got) != "A=1" {
		t.Errorf("no parent value: got %q", got)
	}
}

func TestResolvePath(t *testing.T) {
	dir := filepath.Join("base", "dir")
	rel := filepath.Join("bin", "tool")
	for _, tc := range []struct{ path, dir, want string }{
		{rel, dir, filepath.Join(dir, rel)},
		{rel, "", rel},
		{"/abs/tool", dir, "/abs/tool"},
		{string(filepath.Separator) + "rooted", dir, string(filepath.Separator) + "rooted"},
		{"tool", dir, filepath.Join(dir, "tool")}, // bare names are looked up before this
	} {
		if got := resolvePath(tc.path, tc.dir); got != tc.want {
			t.Errorf("resolvePath(%q, %q) = %q, want %q", tc.path, tc.dir, got, tc.want)
		}
	}
}
