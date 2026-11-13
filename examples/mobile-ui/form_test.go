package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// frames runs n frames (a keyboard reveal takes one frame to queue and one
// to apply; the Driver does not run requested frames by itself).
func frames(d *ui.Driver, n int) {
	for range n {
		d.Frame()
	}
}

func boundsOf(t *testing.T, d *ui.Driver, name string) geom.Rect {
	t.Helper()
	n, ok := d.A11yFind(a11y.RoleTextbox, name)
	if !ok {
		t.Fatalf("no text box %q", name)
	}
	return geom.Rect{X: n.Bounds.X, Y: n.Bounds.Y, W: n.Bounds.W, H: n.Bounds.H}
}

// The form tab on a phone-sized window: tapping the text area low on the
// screen and raising a 336pt keyboard lifts it above the keyboard; the
// search field gets its keyboard hints, typing and Enter reach it, and
// leaving it logs the committed value.
func TestFormTabKeyboard(t *testing.T) {
	var out bytes.Buffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)
	tab = "form"
	defer func() { tab = "list" }()
	d := ui.NewDriver(page, ui.DriverOptions{Width: 390, Height: 844, Accessibility: true})
	d.Settle()
	const kb, limit = 336, 844 - 336 - 8
	r := boundsOf(t, d, "留言")
	if r.Bottom() < 844-kb || r.Y > 844 {
		t.Fatalf("text area at %v: should start visible but under the keyboard", r)
	}
	d.Click(r.Center())
	d.Frame()
	d.SetKeyboardInset(geom.Insets{Bottom: kb})
	frames(d, 3)
	on, caret := d.TextInputActive()
	if !on || caret.Bottom() > limit+0.5 {
		t.Fatalf("caret %v (text input %v) not above the keyboard", caret, on)
	}
	if r := boundsOf(t, d, "留言"); r.Bottom() > limit+0.5 {
		t.Fatalf("text area %v not above the keyboard", r)
	}
	st, ok := d.TextInputState()
	if !ok || !st.Hints.Multiline || st.Hints.Enter != input.EnterNewline {
		t.Fatalf("text area hints %+v", st.Hints)
	}
	if p := os.Getenv("MOBILEUI_FORM_PNG"); p != "" {
		if err := d.SavePNG(p); err != nil {
			t.Fatal(err)
		}
	}
	d.Type("第一行")
	d.Key(input.KeyEnter, 0)
	d.Type("second")
	d.Frame()
	if form.note != "第一行\nsecond" {
		t.Fatalf("note %q", form.note)
	}

	// The search field: tap it (it is visible now), its hints, typing.
	d.Click(boundsOf(t, d, "搜索").Center())
	frames(d, 3)
	st, ok = d.TextInputState()
	if !ok || st.Hints.Kind != input.TextSearch || st.Hints.Enter != input.EnterSearch {
		t.Fatalf("search field hints %+v", st.Hints)
	}
	if !strings.Contains(out.String(), `form note = "第一行\nsecond"`) {
		t.Fatalf("no commit log for the note:\n%s", out.String())
	}
	d.Type("tango")
	d.Key(input.KeyEnter, 0)
	d.Frame()
	if form.search != "tango" || !strings.Contains(out.String(), `form search submitted "tango"`) {
		t.Fatalf("search %q, log:\n%s", form.search, out.String())
	}
	d.Key(input.KeyTab, 0) // leave the field
	frames(d, 2)
	if !strings.Contains(out.String(), `form search = "tango"`) {
		t.Fatalf("no commit log:\n%s", out.String())
	}

	// The keyboard hides.
	d.SetKeyboardInset(geom.Insets{})
	d.Settle()
	if !strings.Contains(out.String(), "keyboard inset 0") {
		t.Fatalf("no keyboard log:\n%s", out.String())
	}
}
