package windows

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestMessageBoxMapping(t *testing.T) {
	if got := messageButtons(nil); len(got) != 1 || got[0] != "OK" {
		t.Errorf("default buttons = %v", got)
	}
	cases := []struct {
		kind  platform.MessageKind
		n     int
		style uint32
	}{
		{platform.MessageInfo, 1, mbOK | mbIconInformation},
		{platform.MessageWarning, 2, mbOKCancel | mbIconWarning},
		{platform.MessageQuestion, 2, mbYesNo | mbIconQuestion},
		{platform.MessageError, 3, mbYesNoCancel | mbIconError},
		{platform.MessageError, 5, mbYesNoCancel | mbIconError},
	}
	for _, c := range cases {
		if got := messageBoxStyle(c.kind, c.n); got != c.style {
			t.Errorf("style(%v,%d) = %#x, want %#x", c.kind, c.n, got, c.style)
		}
	}
	idx := []struct {
		ret  uintptr
		n    int
		want int
		err  error
	}{
		{idOK, 1, 0, nil},
		{idCancel, 1, 0, nil},
		{idOK, 2, 0, nil},
		{idCancel, 2, -1, platform.ErrCanceled},
		{idYes, 2, 0, nil},
		{idNo, 2, 1, nil},
		{idYes, 3, 0, nil},
		{idNo, 3, 1, nil},
		{idCancel, 3, -1, platform.ErrCanceled},
		{0, 2, -1, platform.ErrCanceled},
	}
	for _, c := range idx {
		got, err := messageBoxIndex(c.ret, c.n)
		if got != c.want || !errors.Is(err, c.err) && err != c.err {
			t.Errorf("index(%d,%d) = %d %v", c.ret, c.n, got, err)
		}
	}
}

func TestTaskDialogMapping(t *testing.T) {
	if i, err := taskDialogIndex(firstButtonID+2, 3); i != 2 || err != nil {
		t.Errorf("third = %d %v", i, err)
	}
	if i, err := taskDialogIndex(idCancel, 1); i != 0 || err != nil {
		t.Errorf("single cancel = %d %v", i, err)
	}
	if _, err := taskDialogIndex(idCancel, 2); !errors.Is(err, platform.ErrCanceled) {
		t.Errorf("multi cancel = %v", err)
	}
	if _, err := taskDialogIndex(firstButtonID+5, 2); !errors.Is(err, platform.ErrCanceled) {
		t.Errorf("out of range = %v", err)
	}
	if taskDialogIcon(platform.MessageError) != tdErrorIcon || taskDialogIcon(platform.MessageWarning) != tdWarningIcon ||
		taskDialogIcon(platform.MessageQuestion) != tdInformationIcon {
		t.Error("icons")
	}
}

func TestFileFilters(t *testing.T) {
	filters := []platform.FileFilter{
		{Name: "Images", Patterns: []string{"*.png", ".jpg", "gif", " "}},
		{Name: "", Patterns: []string{"*.txt"}},
		{Name: "Nothing", Patterns: nil},
		{Name: "All", Patterns: []string{"*"}},
	}
	got := filterSpecs(filters)
	want := []filterSpec{{"Images", "*.png;*.jpg;*.gif"}, {"*.txt", "*.txt"}, {"All", "*.*"}}
	if len(got) != len(want) {
		t.Fatalf("specs = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("spec %d = %v, want %v", i, got[i], want[i])
		}
	}
	if ext := defaultExtension(filters); ext != "png" {
		t.Errorf("default ext = %q", ext)
	}
	if ext := defaultExtension([]platform.FileFilter{{Patterns: []string{"*"}}}); ext != "" {
		t.Errorf("wildcard ext = %q", ext)
	}
	if ext := defaultExtension([]platform.FileFilter{{Patterns: []string{"*.tar.*"}}}); ext != "" {
		t.Errorf("glob ext = %q", ext)
	}
	if ext := defaultExtension(nil); ext != "" {
		t.Errorf("no filters ext = %q", ext)
	}
}

func TestFileDialogOptions(t *testing.T) {
	o := openDialogOptions(platform.FileOptions{})
	if o&fosFileMustExist == 0 || o&fosPickFolders != 0 || o&fosAllowMultiselect != 0 || o&fosForceFileSystem == 0 {
		t.Errorf("open = %#x", o)
	}
	o = openDialogOptions(platform.FileOptions{PickFolders: true, Multiple: true})
	if o&fosPickFolders == 0 || o&fosAllowMultiselect == 0 || o&fosFileMustExist != 0 {
		t.Errorf("folders = %#x", o)
	}
	if s := saveDialogOptions(platform.FileOptions{}); s&fosOverwritePrompt == 0 || s&fosForceShowHidden != 0 {
		t.Errorf("save = %#x", s)
	}
	if o := openDialogOptions(platform.FileOptions{ShowHidden: true}); o&fosForceShowHidden == 0 {
		t.Errorf("hidden = %#x", o)
	}
	if s := saveDialogOptions(platform.FileOptions{ShowHidden: true}); s&fosForceShowHidden == 0 {
		t.Errorf("save hidden = %#x", s)
	}
}
