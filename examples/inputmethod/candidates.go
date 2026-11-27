package main

import (
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/ime"
	"github.com/tangoui/tango/sys/screen"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

// IME callbacks and this window's page run on the app's main thread.
type candidateWindow struct {
	app     *tango.App
	surface *tango.Surface
	runner  *ui.Runner
	theme   *theme.Theme
	current *candidateList
}

type candidateList struct {
	session *ime.Session
	words   []string
	choose  func(int)
}

func newCandidateWindow(app *tango.App) (*candidateWindow, error) {
	th := theme.AntDesign()
	th.BgLayout = theme.Color{} // The corners outside the panel are transparent.
	w := &candidateWindow{app: app, theme: th}
	r, s, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
		Title: "Tango Pinyin Candidates", Kind: tango.KindOverlay, Hidden: true, Auxiliary: true,
		Width: int(th.ControlHeight), Height: int(th.ControlHeight),
		Appearance: tango.Appearance{Transparent: true},
	}}, w.page, ui.RunnerOptions{Theme: th})
	if err != nil {
		return nil, err
	}
	w.surface, w.runner = s, r
	return w, nil
}

func (w *candidateWindow) page(c *ui.Context) {
	list := w.current
	if list == nil {
		return
	}
	th := c.Theme
	ui.Row(c, func() {
		for i, word := range list.words {
			label := fmt.Sprintf("%d  %s", i+1, word)
			if ui.Button(c, label).Text().Small().Clicked() {
				w.app.Post(func() {
					if w.current == list {
						list.choose(i)
					}
				})
			}
		}
	}, layout.Fill(), layout.Middle(), layout.Pad(th.PaddingXS), layout.Gap(th.PaddingXXS)).
		Bg(th.BgElevated).Radius(th.RadiusLG)
}

func (w *candidateWindow) show(s *ime.Session, caret tango.Rect, words []string, choose func(int)) {
	if caret.Height <= 0 {
		w.dismiss()
		return
	}
	display, err := screen.At(w.app, caret.X, caret.Y)
	if err != nil {
		w.dismiss()
		return
	}
	words = words[:min(len(words), 9)]
	th := w.theme
	width := 2*th.PaddingXS + float32(len(words)-1)*th.PaddingXXS
	for _, word := range words {
		// Leave a full em per CJK glyph, plus room for the number and button padding.
		width += float32(utf8.RuneCountInString(word)+2)*th.FontSize + 2*th.PaddingXS
	}
	height := th.ControlHeightSM + 2*th.PaddingXS
	bounds := candidateBounds(caret, int(math.Ceil(float64(width))), int(height), display.WorkArea)
	w.current = &candidateList{session: s, words: words, choose: choose}
	w.surface.SetBounds(bounds)
	w.surface.Show()
	w.runner.Invalidate()
}

func (w *candidateWindow) hide(s *ime.Session) {
	if w != nil && w.current != nil && w.current.session == s {
		w.dismiss()
	}
}

func (w *candidateWindow) dismiss() {
	w.current = nil
	w.surface.Hide()
}

func candidateBounds(caret tango.Rect, w, h int, work tango.Rect) tango.Rect {
	const gap = 4
	w = min(w, work.Width)
	h = min(h, work.Height)
	x := min(max(caret.X, work.X), work.X+work.Width-w)
	y := caret.Y + caret.Height + gap
	if y+h > work.Y+work.Height {
		y = caret.Y - gap - h
	}
	y = min(max(y, work.Y), work.Y+work.Height-h)
	return tango.Rect{X: x, Y: y, Width: w, Height: h}
}
