package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// statusError marks a project problem that breaks an icon (as opposed to
// statusWarn, a result that will probably not look as intended).
const statusError checkStatus = "error"

// iconChecks looks at the icons tan.json in dir names, when there is one.
// Each problem is a row; a project without problems gets one ok row.
func iconChecks(dir string) []check {
	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); err != nil {
		return nil
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		return []check{{name: "Project icons", status: statusError, detail: err.Error()}}
	}
	var cs []check
	add := func(name string, st checkStatus, format string, a ...any) {
		cs = append(cs, check{name: name, status: st, detail: fmt.Sprintf(format, a...)})
	}
	// open decodes an icon file; problems are reported under name.
	open := func(name, key, rel string) image.Image {
		f, err := os.Open(cfg.Path(rel))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				add(name, statusError, `"%s": %s does not exist`, key, rel)
			} else {
				add(name, statusError, `"%s": %v`, key, err)
			}
			return nil
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			add(name, statusError, `"%s": %s is not a valid PNG (%v); export it as PNG`, key, rel, err)
			return nil
		}
		return img
	}

	const master = "Icon"
	if cfg.Icon == "" {
		add(master, statusWarn, `tan.json has no "icon": apps get a default icon. Add a 1024x1024 PNG that fills the whole square (no transparent margin, no rounded corners)`)
	} else if img := open(master, "icon", cfg.Icon); img != nil {
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		switch {
		case w != h:
			add(master, statusError, `%s is %dx%d: the icon must be square, or it is stretched`, cfg.Icon, w, h)
		case w < 1024:
			add(master, statusWarn, `%s is %dx%d: use 1024x1024 (the App Store icon and macOS need it; smaller icons look blurry)`, cfg.Icon, w, h)
		}
		if cornersTransparent(img) {
			add(master, statusWarn, `%s has transparent corners, so it is not full-bleed: iOS fills them with white and desktop builds add a second rounded shape inside it. Use artwork that fills the square, or give the shaped file as "icons.macos" (and "icons.windows", "icons.linux.app")`, cfg.Icon)
		}
	}

	ic := cfg.Icons
	if ic.MacOS != "" {
		if img := open("Icon (macOS)", "icons.macos", ic.MacOS); img != nil {
			if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != 1024 || h != 1024 {
				add("Icon (macOS)", statusWarn, `%s is %dx%d: "icons.macos" is used as is and should be 1024x1024`, ic.MacOS, w, h)
			}
			if !cornersTransparent(img) {
				add("Icon (macOS)", statusWarn, `%s has opaque corners, so it does not look like a macOS icon (a rounded square inside a transparent margin, 824 of 1024 px). Remove "icons.macos" to let tan derive the shape from "icon"`, ic.MacOS)
			}
		}
	}
	if ic.Windows != "" && strings.EqualFold(filepath.Ext(ic.Windows), ".png") {
		if img := open("Icon (Windows)", "icons.windows", ic.Windows); img != nil {
			if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w < 256 || h < 256 {
				add("Icon (Windows)", statusWarn, `%s is %dx%d: Windows uses up to 256x256; give a PNG of 256 px or more (or an .ico)`, ic.Windows, w, h)
			}
		}
	} else if ic.Windows != "" {
		if _, err := os.Stat(cfg.Path(ic.Windows)); err != nil {
			add("Icon (Windows)", statusError, `"icons.windows": %s does not exist`, ic.Windows)
		}
	}
	if ic.Linux.App != "" {
		open("Icon (Linux)", "icons.linux.app", ic.Linux.App)
	}
	if ic.Linux.Symbolic != "" {
		if _, err := os.Stat(cfg.Path(ic.Linux.Symbolic)); err != nil {
			add("Icon (Linux)", statusError, `"icons.linux.symbolic": %s does not exist`, ic.Linux.Symbolic)
		}
	}

	for _, v := range []struct{ key, rel string }{{"icons.ios.dark", ic.IOS.Dark}, {"icons.ios.tinted", ic.IOS.Tinted}} {
		if v.rel == "" {
			continue
		}
		if img := open("Icon (iOS)", v.key, v.rel); img != nil {
			w, h := img.Bounds().Dx(), img.Bounds().Dy()
			if w != h {
				add("Icon (iOS)", statusError, `%s is %dx%d: "%s" must be square (1024x1024)`, v.rel, w, h, v.key)
			} else if w != 1024 {
				add("Icon (iOS)", statusWarn, `%s is %dx%d: "%s" should be 1024x1024 like the main icon`, v.rel, w, h, v.key)
			}
		}
	}

	const android = "Icon (Android)"
	layer := func(key, rel string) image.Image {
		img := open(android, key, rel)
		if img != nil && img.Bounds().Dx() != img.Bounds().Dy() {
			add(android, statusError, `%s is %dx%d: "%s" must be square (a 108 dp layer, ideally 432x432)`, rel, img.Bounds().Dx(), img.Bounds().Dy(), key)
			return nil
		}
		return img
	}
	if a := ic.Android; a.Foreground != "" {
		if img := layer("icons.android.foreground", a.Foreground); img != nil && !insideSafeZone(img) {
			add(android, statusWarn, `%s has artwork outside the centre 66/108 of the layer: launchers may clip it. Keep it within the middle %d%% (%d px of %d)`, a.Foreground, 66*100/108, img.Bounds().Dx()*66/108, img.Bounds().Dx())
		}
	}
	if a := ic.Android; a.Background != "" && !a.IsColor() {
		layer("icons.android.background", a.Background)
	}
	if a := ic.Android; a.Monochrome != "" {
		if img := layer("icons.android.monochrome", a.Monochrome); img != nil && hasColour(img) {
			add(android, statusWarn, `%s has coloured pixels: Android uses only the alpha of "icons.android.monochrome" and tints it with the theme; draw the shape in white or black on transparent`, a.Monochrome)
		}
	}
	if ic.Notification != "" {
		const name = "Icon (notification)"
		if img := open(name, "icons.notification", ic.Notification); img != nil {
			switch {
			case fullyOpaque(img):
				add(name, statusError, `%s has no transparency: Android draws only the alpha of a status-bar icon, so it shows as a white square. Use a white shape on a transparent background`, ic.Notification)
			case hasColour(img):
				add(name, statusWarn, `%s has coloured pixels: Android keeps only the alpha (tan turns it white). Use a white shape on a transparent background`, ic.Notification)
			}
		}
	}
	if len(cs) == 0 {
		detail := "no problems"
		if cfg.Icon != "" {
			detail = cfg.Icon + ": no problems"
		}
		add("Project icons", statusOK, "%s", detail)
	}
	return cs
}

// cornersTransparent reports whether the four corner pixels are (nearly)
// fully transparent.
func cornersTransparent(img image.Image) bool {
	b := img.Bounds()
	for _, p := range []image.Point{{b.Min.X, b.Min.Y}, {b.Max.X - 1, b.Min.Y}, {b.Min.X, b.Max.Y - 1}, {b.Max.X - 1, b.Max.Y - 1}} {
		if _, _, _, a := img.At(p.X, p.Y).RGBA(); a > 0x0800 {
			return false
		}
	}
	return true
}

// fullyOpaque reports whether no pixel is noticeably transparent.
func fullyOpaque(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xf000 {
				return false
			}
		}
	}
	return true
}

// hasColour reports whether more than 1% of the visible pixels are
// clearly not grey.
func hasColour(img image.Image) bool {
	b := img.Bounds()
	var visible, coloured int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < 64 {
				continue
			}
			visible++
			hi := max(c.R, c.G, c.B)
			lo := min(c.R, c.G, c.B)
			if hi-lo > 40 {
				coloured++
			}
		}
	}
	return visible > 0 && coloured*100 > visible
}

// insideSafeZone reports whether the visible pixels of an adaptive icon
// layer stay within its centre 66/108 (one pixel of slack for resampling).
func insideSafeZone(img image.Image) bool {
	b := img.Bounds()
	n := b.Dx()
	margin := float64(n) * float64(androidLayerDp-androidLayerSafe) / 2 / float64(androidLayerDp)
	lo, hi := int(margin)-1, n-int(margin)+1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0x2000 {
				continue
			}
			if rx, ry := x-b.Min.X, y-b.Min.Y; rx < lo || rx >= hi || ry < lo || ry >= hi {
				return false
			}
		}
	}
	return true
}
