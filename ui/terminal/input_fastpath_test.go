package terminal

import (
	"bytes"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestTerminalEventsSendsKeyBeforeNextFrame(t *testing.T) {
	g := newViewRig(t, 600, 300, nil)
	g.focus()
	g.d.Inject(input.KeyEvent{Key: input.KeyEnter, Down: true})
	g.waitInput("\r")
	g.d.Frame()
}

func TestTerminalEventsCompositionCommitEnterOrder(t *testing.T) {
	g := newViewRig(t, 600, 300, nil)
	g.focus()
	g.d.Inject(input.Composition{Text: "你", Cursor: 1})
	g.d.Inject(input.TextInput{Text: "你"})
	g.d.Inject(input.KeyEvent{Key: input.KeyEnter, Down: true})
	g.d.Frame()
	g.waitInput("你\r")
}

func TestTerminalEventsBuildTwiceDoesNotDuplicate(t *testing.T) {
	g := newViewRig(t, 600, 300, nil)
	g.focus()
	g.d.Inject(input.KeyEvent{Key: input.KeyEnter, Down: true})
	g.waitInput("\r")
	g.d.Frame()
	g.d.Frame()
}

func TestSendInputReservesInteractiveAllowance(t *testing.T) {
	term := New(Options{Conn: newPipeConn()})
	t.Cleanup(func() { term.Close() })
	term.qlimit = 8
	term.Send(bytes.Repeat([]byte{'x'}, 8))
	result := term.SendInput([]byte("\r"))
	if result.AcceptedBytes != 1 || result.Truncated {
		t.Fatalf("SendInput result = %+v, want one accepted byte", result)
	}
}

func TestSendInputReportsBulkTruncation(t *testing.T) {
	term := New(Options{Conn: newPipeConn()})
	t.Cleanup(func() { term.Close() })
	term.qlimit = 8
	result := term.SendInput(bytes.Repeat([]byte{'x'}, 65<<10))
	if result.AcceptedBytes != 0 || !result.Truncated {
		t.Fatalf("SendInput result = %+v, want truncated bulk input", result)
	}
}
