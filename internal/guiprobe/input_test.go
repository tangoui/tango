//go:build tango_guiprobe

package guiprobe

import (
	"github.com/tangoui/tango/ui/input"
	"testing"
)

func TestObserverOnlyReportsDeliveredEvents(t *testing.T) {
	var got []input.Event
	SetObserver(func(e input.Event) { got = append(got, e) })
	defer SetObserver(nil)
	Observe(input.Composition{Text: "ni", Cursor: 2})
	Observe(input.TextInput{Text: "你好"})
	if len(got) != 2 || got[0].(input.Composition).Text != "ni" || got[1].(input.TextInput).Text != "你好" {
		t.Fatalf("%v", got)
	}
	SetObserver(nil)
	Observe(input.TextInput{Text: "ignored"})
	if len(got) != 2 {
		t.Fatal("observer was not detached")
	}
}
