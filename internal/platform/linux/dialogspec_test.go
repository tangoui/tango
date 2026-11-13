package linux

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestGTKMessageType(t *testing.T) {
	cases := map[platform.MessageKind]int32{
		platform.MessageInfo:     gtkMessageInfo,
		platform.MessageWarning:  gtkMessageWarning,
		platform.MessageError:    gtkMessageError,
		platform.MessageQuestion: gtkMessageQuestion,
		platform.MessageKind(42): gtkMessageInfo,
	}
	for k, want := range cases {
		if got := gtkMessageType(k); got != want {
			t.Errorf("gtkMessageType(%d) = %d, want %d", k, got, want)
		}
	}
}

func TestMessageButtonsAndResult(t *testing.T) {
	if got := messageButtons(platform.MessageOptions{}); !reflect.DeepEqual(got, []string{"_OK"}) {
		t.Errorf("default buttons = %q", got)
	}
	if got := messageButtons(platform.MessageOptions{Buttons: []string{"Save_As", "No"}}); !reflect.DeepEqual(got, []string{"Save__As", "No"}) {
		t.Errorf("buttons = %q", got)
	}
	if b, err := messageResult(1, 2); b != 1 || err != nil {
		t.Errorf("messageResult(1,2) = %d, %v", b, err)
	}
	for _, resp := range []int32{gtkResponseDeleteEvent, -1, 2} {
		if b, err := messageResult(resp, 2); b != -1 || !errors.Is(err, platform.ErrCanceled) {
			t.Errorf("messageResult(%d,2) = %d, %v", resp, b, err)
		}
	}
}

func TestFileChooserAction(t *testing.T) {
	if fileChooserAction(platform.FileOptions{}, false) != gtkFileChooserOpen ||
		fileChooserAction(platform.FileOptions{PickFolders: true}, false) != gtkFileChooserSelectFolder ||
		fileChooserAction(platform.FileOptions{PickFolders: true}, true) != gtkFileChooserSave {
		t.Error("wrong file chooser action")
	}
}

func TestSaveLocation(t *testing.T) {
	d, n := saveLocation(platform.FileOptions{Filename: "/home/u/doc.txt"})
	if d != "/home/u" || n != "doc.txt" {
		t.Errorf("abs = %q %q", d, n)
	}
	d, n = saveLocation(platform.FileOptions{Directory: "/tmp", Filename: "/home/u/doc.txt"})
	if d != "/tmp" || n != "doc.txt" {
		t.Errorf("abs+dir = %q %q", d, n)
	}
	d, n = saveLocation(platform.FileOptions{Directory: "/tmp", Filename: "a.txt"})
	if d != "/tmp" || n != "a.txt" {
		t.Errorf("rel = %q %q", d, n)
	}
}
