package terminal

import (
	"image"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/terminal/vt"
)

// verifyWindow is a ui.Window without a screen for the redraw hint test.
type verifyWindow struct {
	size  geom.Size
	scale float32
}

func (w *verifyWindow) Size() geom.Size              { return w.size }
func (w *verifyWindow) Scale() float32               { return w.scale }
func (w *verifyWindow) Invalidate()                  {}
func (w *verifyWindow) InvalidateAt(time.Time)       {}
func (w *verifyWindow) SetCursor(input.Cursor)       {}
func (w *verifyWindow) ReadClipboard() string        { return "" }
func (w *verifyWindow) WriteClipboard(string)        {}
func (w *verifyWindow) SetTextInput(bool, geom.Rect) {}
func (w *verifyWindow) Present(*image.RGBA) error    { return nil }

// TestViewDamageVerified draws the view through a gpu.Renderer on a
// cpuexec.Device in strict verify mode: every partial frame is compared
// with a full redraw, so a redraw hint that misses a change fails the
// frame. Glyphs such as "|", "_" and descenders reach past the line box;
// their ink below the last row, in the next row and in the row a scroll
// carries them to must be redrawn too.
func TestViewDamageVerified(t *testing.T) {
	conn := newPipeConn()
	term := New(Options{Conn: conn})
	drained := make(chan struct{})
	go func() { // the program side: discard what the terminal sends
		defer close(drained)
		buf := make([]byte, 4096)
		for {
			if _, err := conn.inR.Read(buf); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		term.Close()
		conn.inR.Close()
		<-drained
	})

	dev := cpuexec.NewDevice()
	dev.SampledOnly = true
	ren := gpu.NewRenderer(dev)
	ren.SetVerify(true)
	ren.SetVerifyStrict(true)
	// 900×600 at 2x as in the example's acceptance run: the grid leaves a
	// strip below its last row.
	w := &verifyWindow{size: geom.Size{W: 900, H: 600}, scale: 2}
	r := ui.NewRunner(func(c *ui.Context) {
		View(c, term).Layout(layout.Grow(1))
	}, ui.RunnerOptions{Renderer: ren, NoAnimations: true})
	r.Attach(w)

	frames, partial := 0, 0
	frame := func(what string) {
		t.Helper()
		before := dev.Stats().Partial
		if err := r.Frame(); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		frames++
		if dev.Stats().Partial > before {
			partial++
		}
	}
	output := func(s string, cond func(v *vt.Terminal) bool) {
		t.Helper()
		if _, err := conn.outW.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
		waitFor(t, 5*time.Second, "output "+strings.TrimSpace(s), func() bool {
			v := term.Lock()
			defer term.Unlock()
			return cond(v)
		})
		frame("output " + strings.TrimSpace(s))
	}

	frame("first")
	frame("sized")
	v := term.Lock()
	_, rows := v.Screen().Size()
	term.Unlock()
	if rows < 12 {
		t.Fatalf("grid has %d rows", rows)
	}
	const tall = "a | gjpqy_ |"
	has := func(y int, s string) func(v *vt.Terminal) bool {
		return func(v *vt.Terminal) bool { return strings.Contains(rowText(v, y), s) }
	}

	// The last row: ink below it lands in the strip under the grid.
	output("\x1b["+strconv.Itoa(rows)+";1H"+tall, has(rows-1, tall))
	// A middle row: ink below it lands in the next row.
	output("\x1b[5;1H"+tall+" mid", has(4, "mid"))
	// Edit the middle row again: the old ink must go.
	output("\x1b[5;1H\x1b[2K"+"plain", has(4, "plain"))
	// Scroll with tall glyphs on every row, one line at a time: the row
	// scrolled out of the top leaves its overhang on row 0.
	var all strings.Builder
	for y := range rows {
		all.WriteString("\x1b[" + strconv.Itoa(y+1) + ";1H" + tall + " r" + strconv.Itoa(y))
	}
	output(all.String(), has(rows-1, "r"+strconv.Itoa(rows-1)))
	for i := range 4 {
		s := "\x1b[" + strconv.Itoa(rows) + ";1H\r\n" + tall + " s" + strconv.Itoa(i)
		output(s, has(rows-1, "s"+strconv.Itoa(i)))
	}
	// Scroll in a blank row: nothing is written on the new last row.
	output("\x1b["+strconv.Itoa(rows)+";1H\r\n", func(v *vt.Terminal) bool {
		return strings.TrimSpace(rowText(v, rows-1)) == "" && strings.Contains(rowText(v, rows-2), "s3")
	})
	// Scroll up (SU) with the cursor away from the last row: the overhang
	// the old last row left below the grid goes with the row scrolled in.
	output("\x1b["+strconv.Itoa(rows)+";1H"+tall+" su\x1b[3;1H\x1b[S", func(v *vt.Terminal) bool {
		return strings.Contains(rowText(v, rows-2), "su")
	})
	if partial == 0 {
		t.Fatalf("no partial frame in %d frames: the hints were not exercised", frames)
	}
}
