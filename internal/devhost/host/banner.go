package host

import (
	"image"
	"strings"

	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/ui"
)

// The error banner is drawn by the host over the frames it shows: a red
// strip along the top edge and an error alert (rendered with package ui
// on a headless driver) with the first lines of the message.

const (
	bannerStrip = 4  // logical px of solid red at the very top
	bannerLines = 4  // message lines shown
	bannerLine  = 22 // logical px per message line
)

// bannerRed is the strip colour (Ant Design error red), premultiplied RGBA.
var bannerRed = [4]byte{0xff, 0x4d, 0x4f, 0xff}

type bannerCache struct {
	st    devhost.Status
	w     int
	scale float32
	img   *image.RGBA
}

func bannerTitle(kind string) string {
	switch kind {
	case "build":
		return "编译失败：仍在显示上一次成功编译的界面"
	case "crash":
		return "应用已退出：等待下一次编译"
	case "start":
		return "新版本启动失败：仍在显示上一次的界面"
	}
	return "热加载出错"
}

func bannerText(s string) (string, int) {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > bannerLines {
		lines = append(lines[:bannerLines-1], "…")
	}
	for i, l := range lines {
		if r := []rune(l); len(r) > 160 {
			lines[i] = string(r[:160]) + "…"
		}
	}
	return strings.Join(lines, "\n"), len(lines)
}

// draw paints the banner for st over the top of img (device pixels at
// scale), reusing the last rendering when nothing changed.
func (b *bannerCache) draw(img *image.RGBA, scale float32, st devhost.Status) {
	if scale <= 0 {
		scale = 1
	}
	dw := img.Rect.Dx()
	if b.img == nil || b.st != st || b.w != dw || b.scale != scale {
		text, n := bannerText(st.Text)
		lw := float32(dw) / scale
		lh := float32(56 + n*bannerLine)
		if text == "" {
			lh = 56
		}
		title := bannerTitle(st.Kind)
		d := ui.NewDriver(func(c *ui.Context) {
			a := ui.Alert(c, "error", title).Banner()
			if text != "" {
				a = a.Description(text)
			}
			a.Show()
		}, ui.DriverOptions{Width: lw, Height: lh, Scale: scale})
		b.img = d.Frame()
		b.st, b.w, b.scale = st, dw, scale
	}
	src := b.img
	rows := min(src.Rect.Dy(), img.Rect.Dy())
	cols := min(src.Rect.Dx(), dw) * 4
	for y := 0; y < rows; y++ {
		copy(img.Pix[y*img.Stride:y*img.Stride+cols], src.Pix[y*src.Stride:y*src.Stride+cols])
	}
	strip := min(int(bannerStrip*scale+0.5), img.Rect.Dy())
	for y := 0; y < strip; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+dw*4]
		for i := 0; i < len(row); i += 4 {
			copy(row[i:i+4], bannerRed[:])
		}
	}
}
