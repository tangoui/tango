package shell_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/shell"
)

func TestAllowed(t *testing.T) {
	defer shell.AllowSchemes()
	for u, want := range map[string]bool{
		"https://example.com/x": true,
		"HTTP://example.com":    true,
		"mailto:a@b.c":          true,
		"https:///nohost":       false,
		"file:///etc/passwd":    false,
		"javascript:alert(1)":   false,
		"zoommtg://join":        false,
		"/relative":             false,
		"example.com":           false,
		"%zz":                   false,
	} {
		if got := shell.Allowed(u); got != want {
			t.Errorf("Allowed(%q) = %v", u, got)
		}
	}
	shell.AllowSchemes("https", "ZoomMtg:", "file")
	if !shell.Allowed("zoommtg://join") || shell.Allowed("http://a") || shell.Allowed("file:///x") {
		t.Fatal("AllowSchemes not applied")
	}
	shell.AllowSchemes()
	if !shell.Allowed("http://a") || shell.Allowed("zoommtg://join") {
		t.Fatal("defaults not restored")
	}
}

func TestCalls(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := shell.OpenURL(a, "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if err := shell.OpenURL(a, "file:///etc/passwd"); !errors.Is(err, shell.ErrSchemeNotAllowed) {
		t.Fatal(err)
	}
	if err := shell.OpenPath(a, file); err != nil {
		t.Fatal(err)
	}
	if err := shell.RevealPath(a, file); err != nil {
		t.Fatal(err)
	}
	if err := shell.RevealPath(a, filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	calls := fb.ShellCalls()
	if len(calls) != 3 || calls[0].Arg != "https://example.com" || calls[1].Op != "open" || calls[2].Op != "reveal" || calls[2].Arg != file {
		t.Fatalf("%+v", calls)
	}
	for _, c := range calls {
		if !c.Main {
			t.Fatalf("%s not on the main thread", c.Op)
		}
	}
	boom := errors.New("boom")
	fb.SetShellErr(boom)
	if err := shell.OpenPath(a, dir); err != boom {
		t.Fatal(err)
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if err := shell.OpenURL(a, "https://example.com"); !errors.Is(err, shell.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := shell.RevealPath(a, "."); !errors.Is(err, shell.ErrUnsupported) {
		t.Fatal(err)
	}
}
