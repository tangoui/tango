//go:build darwin

package metal

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

type codeBlockScrollWindow struct{}

func (codeBlockScrollWindow) Size() geom.Size              { return geom.Size{W: 720, H: 500} }
func (codeBlockScrollWindow) Scale() float32               { return 2 }
func (codeBlockScrollWindow) Invalidate()                  {}
func (codeBlockScrollWindow) InvalidateAt(time.Time)       {}
func (codeBlockScrollWindow) SetCursor(input.Cursor)       {}
func (codeBlockScrollWindow) ReadClipboard() string        { return "" }
func (codeBlockScrollWindow) WriteClipboard(string)        {}
func (codeBlockScrollWindow) SetTextInput(bool, geom.Rect) {}
func (codeBlockScrollWindow) Present(*image.RGBA) error    { return nil }

func BenchmarkCodeBlockWrappedScroll(b *testing.B) {
	const line = "The quick brown fox jumps over the lazy dog. This deterministic input exercises text layout in Tango."
	body := strings.Repeat(line+"\n", 25000)
	d := newDevice(b)
	renderer := gpu.NewRenderer(d)
	renderer.SetVerify(false)
	w := codeBlockScrollWindow{}
	r := ui.NewRunner(func(c *ui.Context) {
		ui.Column(c, func() {
			ui.CodeBlock(c, body).Wrap(true).NoCopy().Highlighter(nil).Layout(layout.Fill())
		}, layout.Fill())
	}, ui.RunnerOptions{Renderer: renderer, NoAnimations: true})
	r.Attach(w)
	defer r.Close()
	frame := func() {
		b.Helper()
		if err := r.Frame(); err != nil {
			b.Fatal(err)
		}
		if err := d.Wait(); err != nil {
			b.Fatal(err)
		}
	}
	frame()
	frame()
	pos := geom.Pt(300, 200)
	i := 0
	for b.Loop() {
		dy := float32(116)
		if i/12%2 == 1 {
			dy = -dy
		}
		r.HandleEvent(input.Scroll{Pos: pos, DY: dy})
		frame()
		i++
	}
}
