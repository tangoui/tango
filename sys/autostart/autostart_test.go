package autostart

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileRoundTrip(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("file-based platforms only")
	}
	dir := t.TempDir()
	o := Options{ID: "com.example.test", Name: "Test App", Exec: "/opt/My App/bin/app", Args: []string{"--x=1"}, Dir: dir}
	if on, err := IsEnabled(o); err != nil || on {
		t.Fatal(on, err)
	}
	if err := Enable(o); err != nil {
		t.Fatal(err)
	}
	if on, err := IsEnabled(o); err != nil || !on {
		t.Fatal(on, err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("%v", ents)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ents[0].Name()))
	if !strings.Contains(string(data), "My App") || !strings.Contains(string(data), LoginArg) {
		t.Fatalf("%s", data)
	}
	if err := Enable(o); err != nil { // replace
		t.Fatal(err)
	}
	if err := Disable(o); err != nil {
		t.Fatal(err)
	}
	if err := Disable(o); err != nil { // idempotent
		t.Fatal(err)
	}
	if on, _ := IsEnabled(o); on {
		t.Fatal("still enabled")
	}
}

func TestBadID(t *testing.T) {
	for _, id := range []string{"", "../x", `a\b`, ".."} {
		if err := Enable(Options{ID: id, Dir: t.TempDir()}); err == nil {
			t.Errorf("%q accepted", id)
		}
	}
}

func TestDefaultExec(t *testing.T) {
	cmd, err := Options{ID: "x", Args: []string{"a"}}.command()
	if err != nil || len(cmd) != 3 || !filepath.IsAbs(cmd[0]) || cmd[1] != "a" || cmd[2] != LoginArg {
		t.Fatal(cmd, err)
	}
}

func TestPlist(t *testing.T) {
	p := launchAgentPlist("com.a&b", []string{"/Apps/A <1>.app/Contents/MacOS/a", LoginArg})
	for _, want := range []string{
		"<string>com.a&amp;b</string>",
		"<string>/Apps/A &lt;1&gt;.app/Contents/MacOS/a</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in\n%s", want, p)
		}
	}
}

func TestDesktopEntry(t *testing.T) {
	e := desktopEntry("Notes\nX", []string{"/opt/my app/notes", "100%", `a"b$c`, "--plain"})
	want := "Exec=\"/opt/my app/notes\" 100%% \"a\\\\\"b\\\\$c\" --plain\n"
	if !strings.Contains(e, want) {
		t.Fatalf("got\n%s\nwant line %q", e, want)
	}
	if !strings.Contains(e, `Name=Notes\nX`) {
		t.Fatal(e)
	}
	if !desktopEntryEnabled(e) {
		t.Fatal("fresh entry disabled")
	}
	for _, off := range []string{e + "Hidden=true\n", strings.Replace(e, "enabled=true", "enabled=false", 1)} {
		if desktopEntryEnabled(off) {
			t.Errorf("enabled: %s", off)
		}
	}
	if !desktopEntryEnabled(e + "[Desktop Action x]\nHidden=true\n") {
		t.Error("other group counted")
	}
}

func TestWindowsCommandLine(t *testing.T) {
	got := windowsCommandLine([]string{`C:\Program Files\App\app.exe`, "plain", "with space", `q"uote`, `end\`, `sp ace\`, ""})
	want := `"C:\Program Files\App\app.exe" plain "with space" "q\"uote" end\ "sp ace\\" ""`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if !startupApprovedEnabled(nil) || !startupApprovedEnabled([]byte{2, 0}) || startupApprovedEnabled([]byte{3, 0}) {
		t.Fatal("StartupApproved")
	}
}
