package widget_test

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/widget"
)

func TestUpdate(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	c := widget.Content{Title: "Inbox", Text: "3 unread ✓"}
	if err := widget.Update(a, c); err != nil {
		t.Fatal(err)
	}
	if got, ok := fb.Widget(); !ok || got != c {
		t.Fatal(got, ok)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if err := widget.Update(m, c); !errors.Is(err, widget.ErrUnsupported) {
		t.Fatal(err)
	}
}
