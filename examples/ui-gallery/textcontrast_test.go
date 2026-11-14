package main

// TestGalleryTextContrast finds text whose rendered contrast against the
// pixels actually behind it is too low to read (below 1.6:1), on every
// gallery page in the Ant Design, Element Plus and shadcn/ui themes, light
// and dark: the bugs token-level contrast checks cannot see, such as white
// text on shadcn's near-white dark-mode primary. Each page is rendered
// without text (the background) and with every Text op in a unique tag
// color; pixels equal to a tag color are that op's glyph cores, and the op's
// real color is compared with the background under them. Text inside
// opacity groups (disabled controls) cannot be tagged exactly and is
// skipped; intentionally faint text (alpha below 80) and the decorations in
// textContrastAllowed are not reported.

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func tagColor(i int) color.NRGBA {
	return color.NRGBA{uint8(17 + i%200), uint8(61 + (i/200)%150), 211, 255}
}

func relLum(c color.NRGBA) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

func contrast(a, b color.NRGBA) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func over(fg color.NRGBA, bg color.NRGBA) color.NRGBA {
	a := float64(fg.A) / 255
	mix := func(f, b uint8) uint8 { return uint8(math.Round(float64(f)*a + float64(b)*(1-a))) }
	return color.NRGBA{mix(fg.R, bg.R), mix(fg.G, bg.G), mix(fg.B, bg.B), 255}
}

// textContrastAllowed lists pages whose low-contrast text is by design.
var textContrastAllowed = map[string]string{
	"Watermark": "watermarks are faint on purpose",
	"Rate":      "star glyphs: yellow and empty stars on white, as in Ant Design",
	"Result":    "the large decorative status numerals of the illustrations",
	"MobileApp": "the pale placeholder of the phone mockup's search field",
}

func TestGalleryTextContrast(t *testing.T) {
	if testing.Short() {
		t.Skip("renders every gallery page in six themes three times")
	}
	variants := []struct {
		name string
		th   func() *theme.Theme
	}{
		{"antd", func() *theme.Theme { return theme.AntDesign() }},
		{"antd-dark", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
		{"element", func() *theme.Theme { return theme.ElementPlus() }},
		{"element-dark", func() *theme.Theme { return theme.ElementPlus(theme.Dark()) }},
		{"shadcn", func() *theme.Theme { return theme.Shadcn() }},
		{"shadcn-dark", func() *theme.Theme { return theme.Shadcn(theme.Dark()) }},
	}
	const scale = 2
	var report []string
	pages, ops, checked := 0, 0, 0
	for _, v := range variants {
		for _, p := range gallery.Pages() {
			if p.Variant || p.Name == "Mobile" {
				continue
			}
			w := p.Width
			if w == 0 {
				w = 960
			}
			p := p
			page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
			if p.Bare {
				page = p.Render
			}
			dash, ov = newDash(), newOverlayState()
			d := ui.NewDriver(page, ui.DriverOptions{Width: w, Height: 1400, Scale: scale, Theme: v.th()})
			d.Settle()
			l := d.Runner().List()
			size := geom.Size{W: w, H: 1400}
			pages++
			noText := &paint.List{Background: l.Background}
			tagged := &paint.List{Background: l.Background}
			var texts []paint.Text
			for _, op := range l.Ops {
				if tx, ok := op.(paint.Text); ok {
					tt := tx
					tt.Color = tagColor(len(texts))
					tagged.Ops = append(tagged.Ops, tt)
					texts = append(texts, tx)
					continue
				}
				noText.Ops = append(noText.Ops, op)
				tagged.Ops = append(tagged.Ops, op)
			}
			ops += len(texts)
			bgR, tgR := paint.NewCPU(), paint.NewCPU()
			if err := bgR.Render(noText, size, scale); err != nil {
				t.Fatal(err)
			}
			if err := tgR.Render(tagged, size, scale); err != nil {
				t.Fatal(err)
			}
			bg, tg := bgR.Image(), tgR.Image()
			index := map[[3]uint8]int{}
			for i := range texts {
				c := tagColor(i)
				index[[3]uint8{c.R, c.G, c.B}] = i
			}
			type acc struct {
				n                int
				r, g, b          float64
				minX, minY, maxX int
				maxY             int
			}
			accs := make([]acc, len(texts))
			for i := range accs {
				accs[i].minX, accs[i].minY = 1<<30, 1<<30
			}
			bd := tg.Bounds()
			for y := bd.Min.Y; y < bd.Max.Y; y++ {
				for x := bd.Min.X; x < bd.Max.X; x++ {
					o := tg.PixOffset(x, y)
					px := tg.Pix[o : o+4]
					if px[3] != 255 {
						continue
					}
					i, ok := index[[3]uint8{px[0], px[1], px[2]}]
					if !ok {
						continue
					}
					b := bg.Pix[o : o+4]
					if b[0] == px[0] && b[1] == px[1] && b[2] == px[2] {
						continue // background already has this exact color
					}
					a := &accs[i]
					a.n++
					a.r, a.g, a.b = a.r+float64(b[0]), a.g+float64(b[1]), a.b+float64(b[2])
					a.minX, a.minY, a.maxX, a.maxY = min(a.minX, x), min(a.minY, y), max(a.maxX, x), max(a.maxY, y)
				}
			}
			if _, ok := textContrastAllowed[p.Name]; ok {
				continue
			}
			for i, a := range accs {
				if a.n < 6 || texts[i].Color.A < 80 {
					continue
				}
				checked++
				bgc := color.NRGBA{uint8(a.r / float64(a.n)), uint8(a.g / float64(a.n)), uint8(a.b / float64(a.n)), 255}
				fg := over(texts[i].Color, bgc)
				cr := contrast(fg, bgc)
				if cr < 1.6 {
					report = append(report, fmt.Sprintf("%-13s %-26s contrast %.2f text %02x%02x%02x(a%d) on %02x%02x%02x at %d,%d-%d,%d (logical %d,%d)",
						v.name, p.Name, cr, texts[i].Color.R, texts[i].Color.G, texts[i].Color.B, texts[i].Color.A, bgc.R, bgc.G, bgc.B,
						a.minX, a.minY, a.maxX, a.maxY, a.minX/scale, a.minY/scale))
				}
			}
		}
	}
	sort.Strings(report)
	t.Logf("%d page renders, %d text runs, %d measured", pages, ops, checked)
	if len(report) > 0 {
		t.Errorf("%d text runs below 1.6:1 against their rendered background:\n%s", len(report), strings.Join(report, "\n"))
	}
}
