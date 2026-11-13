package linux

import (
	"slices"
	"strings"
	"testing"
)

func TestGVString(t *testing.T) {
	cases := map[string]string{
		"plain":     "'plain'",
		"it's":      `'it\'s'`,
		`a\b`:       `'a\\b'`,
		"l1\nl2\tx": `'l1\nl2\tx'`,
		"bell\x07":  `'bell\u0007'`,
		"中文 ok":     "'中文 ok'",
	}
	for in, want := range cases {
		if got := gvString(in); got != want {
			t.Errorf("gvString(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestNotifyArgs(t *testing.T) {
	got := notifyArgs("My App", "com.example.app", "Title", "Sub", "a<b & c", true)
	want := `('My App', uint32 0, '', 'Title', 'Sub\na&lt;b &amp; c', ['default', 'Open'], {'urgency': <byte 1>, 'suppress-sound': <true>, 'desktop-entry': <'com.example.app'>}, -1)`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	got = notifyArgs("", "", "T", "", "", false)
	want = `('', uint32 0, '', 'T', '', ['default', 'Open'], {'urgency': <byte 1>}, -1)`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestURIPaths(t *testing.T) {
	got := uriPaths([]string{"file:///home/u/a%20b.txt", "# comment", "", "https://x/y", "file://localhost/tmp/x", "file://otherhost/z"})
	if !slices.Equal(got, []string{"/home/u/a b.txt", "/tmp/x"}) {
		t.Fatal(got)
	}
}

func TestX11Mods(t *testing.T) {
	if m := x11GrabMods(gdkControlMask | gdkShiftMask | gdkSuperMask); m != xControlMask|xShiftMask|xMod4Mask {
		t.Fatalf("%#x", m)
	}
	if !x11Matches(xControlMask|xLockMask|xMod2Mask, xControlMask) || x11Matches(xControlMask|xShiftMask, xControlMask) {
		t.Fatal("match")
	}
}

func TestMessageResultSingleButton(t *testing.T) {
	if b, err := messageResult(gtkResponseDeleteEvent, 1); b != 0 || err != nil {
		t.Fatalf("%d %v", b, err)
	}
}

func TestFileListPayloads(t *testing.T) {
	d, err := fileListPayloads([]string{"/home/u/a b.txt", "/tmp/中/"})
	if err != nil {
		t.Fatal(err)
	}
	if d.uriList != "file:///home/u/a%20b.txt\r\nfile:///tmp/%E4%B8%AD\r\n" {
		t.Errorf("uriList = %q", d.uriList)
	}
	if d.gnome != "copy\nfile:///home/u/a%20b.txt\nfile:///tmp/%E4%B8%AD" {
		t.Errorf("gnome = %q", d.gnome)
	}
	if d.text != "/home/u/a b.txt\n/tmp/中/" {
		t.Errorf("text = %q", d.text)
	}
	if got := uriPaths(strings.Split(d.uriList, "\r\n")); !slices.Equal(got, []string{"/home/u/a b.txt", "/tmp/中"}) {
		t.Errorf("round trip = %q", got)
	}
	if _, err := fileListPayloads(nil); err == nil {
		t.Error("empty list accepted")
	}
	if _, err := fileListPayloads([]string{"rel/x"}); err == nil {
		t.Error("relative path accepted")
	}
}
