package uihost

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

type stateCanvas struct {
	plainCanvas
	got []input.TextInputState
}

func (c *stateCanvas) SetTextInputState(st input.TextInputState) { c.got = append(c.got, st) }

func TestWindowTextInputState(t *testing.T) {
	st := input.TextInputState{Hints: input.TextHints{Kind: input.TextEmail}, Text: "a@b", SelStart: 3, SelEnd: 3, CompStart: -1, CompEnd: -1}
	cv := &stateCanvas{}
	(&window{cv: cv}).SetTextInputState(st)
	if len(cv.got) != 1 || cv.got[0] != st {
		t.Fatalf("forwarded %v", cv.got)
	}
	// A canvas without platform.CanvasTextInputState ignores it.
	(&window{cv: plainCanvas{}}).SetTextInputState(st)
}
