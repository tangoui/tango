package main

// App icons for iOS and Android, derived from the master icon (Config.Icon:
// square, full-bleed, ideally 1024×1024) and the per-platform overrides in
// Config.Icons. Both systems apply their own mask, so the master icon is
// used without adding a shape.

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// decodeIconPNG reads a project-relative PNG named by tan.json key.
func decodeIconPNG(cfg *Config, key, rel string) (image.Image, error) {
	f, err := os.Open(cfg.Path(rel))
	if err != nil {
		return nil, fmt.Errorf(`"%s": %w`, key, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf(`"%s" %s: %w`, key, rel, err)
	}
	return img, nil
}

// ---------------------------------------------------------------- iOS

// Asset catalog Contents.json (Xcode's asset catalog format reference).
type xcassetInfo struct {
	Author  string `json:"author"`
	Version int    `json:"version"`
}

type xcassetAppearance struct {
	Appearance string `json:"appearance"`
	Value      string `json:"value"`
}

type xcassetImage struct {
	Appearances []xcassetAppearance `json:"appearances,omitempty"`
	Filename    string              `json:"filename"`
	Idiom       string              `json:"idiom"`
	Platform    string              `json:"platform"`
	Size        string              `json:"size"`
}

type xcassetContents struct {
	Images []xcassetImage `json:"images,omitempty"`
	Info   xcassetInfo    `json:"info"`
}

var tanAssetInfo = xcassetInfo{Author: "tan", Version: 1}

// writeIOSAssetCatalog writes catalog (an .xcassets directory) holding
// AppIcon.appiconset: one 1024×1024 universal image (enough since Xcode
// 14), flattened onto white because iOS icons must be opaque. With
// appearances, icons.ios.dark and icons.ios.tinted are added as the iOS 18
// luminosity variants (Xcode 16+); they keep their transparency, which the
// system fills in. It reports whether there was an icon to write and
// whether variants were included.
func writeIOSAssetCatalog(cfg *Config, catalog string, appearances bool) (icon, variants bool, err error) {
	img, err := loadIcon(cfg)
	if err != nil || img == nil {
		return false, false, err
	}
	if err := os.RemoveAll(catalog); err != nil {
		return false, false, err
	}
	set := filepath.Join(catalog, "AppIcon.appiconset")
	if err := writePNG(filepath.Join(set, "icon.png"), resizeIcon(img, 1024, color.White)); err != nil {
		return false, false, err
	}
	contents := xcassetContents{Info: tanAssetInfo, Images: []xcassetImage{
		{Filename: "icon.png", Idiom: "universal", Platform: "ios", Size: "1024x1024"},
	}}
	if appearances {
		for _, v := range []struct{ value, key, path string }{
			{"dark", "icons.ios.dark", cfg.Icons.IOS.Dark},
			{"tinted", "icons.ios.tinted", cfg.Icons.IOS.Tinted},
		} {
			if v.path == "" {
				continue
			}
			vi, err := decodeIconPNG(cfg, v.key, v.path)
			if err != nil {
				return false, false, err
			}
			name := "icon-" + v.value + ".png"
			if err := writePNG(filepath.Join(set, name), resizeIcon(vi, 1024, nil)); err != nil {
				return false, false, err
			}
			contents.Images = append(contents.Images, xcassetImage{
				Appearances: []xcassetAppearance{{Appearance: "luminosity", Value: v.value}},
				Filename:    name, Idiom: "universal", Platform: "ios", Size: "1024x1024",
			})
			variants = true
		}
	}
	if err := writeJSONFile(filepath.Join(catalog, "Contents.json"), xcassetContents{Info: tanAssetInfo}); err != nil {
		return false, false, err
	}
	return true, variants, writeJSONFile(filepath.Join(set, "Contents.json"), contents)
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// hasIOSIconVariants reports whether tan.json asks for dark or tinted icons.
func hasIOSIconVariants(cfg *Config) bool {
	return cfg.Icons.IOS.Dark != "" || cfg.Icons.IOS.Tinted != ""
}

// iosIconAppearancesSupported reports whether the selected Xcode (16 or
// newer) compiles the iOS 18 icon appearances. When the version cannot be
// told, it assumes so (actool failures are handled by the caller).
func iosIconAppearancesSupported(ctx context.Context, cfg *Config) bool {
	if !hasIOSIconVariants(cfg) {
		return false
	}
	out, err := exec.CommandContext(ctx, "xcodebuild", "-version").Output()
	if err != nil {
		return true
	}
	if major := xcodeMajor(string(out)); major > 0 && major < 16 {
		warnf("Xcode %d cannot compile the iOS 18 dark/tinted app icons (icons.ios); they are left out. Install Xcode 16 or newer to include them", major)
		return false
	}
	return true
}

// xcodeMajor parses "Xcode 26.1\nBuild version 17B55".
func xcodeMajor(version string) int {
	line, _, _ := strings.Cut(strings.TrimSpace(version), "\n")
	v, ok := strings.CutPrefix(strings.TrimSpace(line), "Xcode ")
	if !ok {
		return 0
	}
	major, _, _ := strings.Cut(v, ".")
	n, _ := strconv.Atoi(major)
	return n
}

// ------------------------------------------------------------ Android

// androidDensities are the launcher densities and their scale (px per dp,
// mdpi = 1).
var androidDensities = []struct {
	name  string
	scale float64
}{{"mdpi", 1}, {"hdpi", 1.5}, {"xhdpi", 2}, {"xxhdpi", 3}, {"xxxhdpi", 4}}

const (
	androidLegacyDp     = 48  // ic_launcher.png before API 26
	androidLayerDp      = 108 // adaptive icon layers
	androidLayerVisible = 72  // what the mask shows of a layer
	androidLayerSafe    = 66  // never clipped, whatever the mask
	androidNotifyDp     = 24  // status-bar icon
	androidNotifyName   = "ic_stat_tango"
	androidFCMIconMeta  = "com.google.firebase.messaging.default_notification_icon"
)

func dpPx(dp int, scale float64) int { return int(float64(dp)*scale + 0.5) }

// writeAndroidIcons writes the launcher icons into res (when there is a
// master icon) and the notification icon (when icons.notification is set):
//
//   - mipmap-<density>/ic_launcher.png and ic_launcher_round.png, 48 dp,
//     for launchers before API 26 (minSdk is below that);
//   - mipmap-anydpi-v26/ic_launcher.xml and ic_launcher_round.xml, the
//     adaptive icon, with 108 dp layers in mipmap-<density>/
//     ic_launcher_{foreground,background,monochrome}.png or the colour
//     values/tango_icon.xml @color/ic_launcher_background;
//   - drawable-<density>/ic_stat_tango.png, 24 dp, white with the
//     artwork's alpha.
func writeAndroidIcons(cfg *Config, master image.Image, res string) error {
	if master != nil {
		if err := writeAndroidLauncherIcons(cfg, master, res); err != nil {
			return err
		}
	}
	if cfg.Icons.Notification == "" {
		return nil
	}
	n, err := decodeIconPNG(cfg, "icons.notification", cfg.Icons.Notification)
	if err != nil {
		return err
	}
	for _, d := range androidDensities {
		px := dpPx(androidNotifyDp, d.scale)
		p := filepath.Join(res, "drawable-"+d.name, androidNotifyName+".png")
		if err := writePNG(p, whiteSilhouette(resizeIcon(n, px, nil))); err != nil {
			return err
		}
	}
	return nil
}

func writeAndroidLauncherIcons(cfg *Config, master image.Image, res string) error {
	ai := cfg.Icons.Android
	var fg, bg, mono image.Image
	var err error
	if ai.Foreground != "" {
		if fg, err = decodeIconPNG(cfg, "icons.android.foreground", ai.Foreground); err != nil {
			return err
		}
	}
	if ai.Background != "" && !ai.IsColor() {
		if bg, err = decodeIconPNG(cfg, "icons.android.background", ai.Background); err != nil {
			return err
		}
	}
	if ai.Monochrome != "" {
		if mono, err = decodeIconPNG(cfg, "icons.android.monochrome", ai.Monochrome); err != nil {
			return err
		}
	}
	for _, d := range androidDensities {
		dir := filepath.Join(res, "mipmap-"+d.name)
		legacy := resizeIcon(master, dpPx(androidLegacyDp, d.scale), nil)
		if err := writePNG(filepath.Join(dir, "ic_launcher.png"), legacy); err != nil {
			return err
		}
		if err := writePNG(filepath.Join(dir, "ic_launcher_round.png"), circleMasked(legacy)); err != nil {
			return err
		}
		layer := dpPx(androidLayerDp, d.scale)
		// By default the full-bleed master fills exactly what the mask shows.
		front := centerOnLayer(master, layer, dpPx(androidLayerVisible, d.scale))
		if fg != nil {
			front = resizeIcon(fg, layer, nil)
		}
		if err := writePNG(filepath.Join(dir, "ic_launcher_foreground.png"), front); err != nil {
			return err
		}
		if bg != nil {
			if err := writePNG(filepath.Join(dir, "ic_launcher_background.png"), resizeIcon(bg, layer, nil)); err != nil {
				return err
			}
		}
		if mono != nil {
			if err := writePNG(filepath.Join(dir, "ic_launcher_monochrome.png"), resizeIcon(mono, layer, nil)); err != nil {
				return err
			}
		}
	}
	background := "@mipmap/ic_launcher_background"
	if bg == nil {
		hex := ai.Background
		if hex == "" {
			hex = colorHex(borderColor(master))
		}
		values := "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<!-- Generated by tan build from tan.json \"icon\" / \"icons.android\". -->\n<resources>\n    <color name=\"ic_launcher_background\">" + strings.ToUpper(hex) + "</color>\n</resources>\n"
		if err := os.MkdirAll(filepath.Join(res, "values"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(res, "values", "tango_icon.xml"), []byte(values), 0o644); err != nil {
			return err
		}
		background = "@color/ic_launcher_background"
	}
	adaptive := androidAdaptiveIconXML(background, mono != nil)
	anydpi := filepath.Join(res, "mipmap-anydpi-v26")
	if err := os.MkdirAll(anydpi, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"ic_launcher.xml", "ic_launcher_round.xml"} {
		if err := os.WriteFile(filepath.Join(anydpi, name), []byte(adaptive), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// androidAdaptiveIconXML renders an <adaptive-icon> drawable (API 26; the
// <monochrome> layer is read from API 33 and ignored before).
func androidAdaptiveIconXML(background string, monochrome bool) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<!-- Generated by tan build from tan.json \"icon\" / \"icons.android\". -->\n")
	b.WriteString("<adaptive-icon xmlns:android=\"http://schemas.android.com/apk/res/android\">\n")
	fmt.Fprintf(&b, "    <background android:drawable=\"%s\" />\n", background)
	b.WriteString("    <foreground android:drawable=\"@mipmap/ic_launcher_foreground\" />\n")
	if monochrome {
		b.WriteString("    <monochrome android:drawable=\"@mipmap/ic_launcher_monochrome\" />\n")
	}
	b.WriteString("</adaptive-icon>\n")
	return b.String()
}

// ------------------------------------------------------- image helpers

// centerOnLayer scales src to inner×inner in the middle of a transparent
// size×size image.
func centerOnLayer(src image.Image, size, inner int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	off := (size - inner) / 2
	draw.Draw(dst, image.Rect(off, off, off+inner, off+inner), resizeIcon(src, inner, nil), image.Point{}, draw.Src)
	return dst
}

// circleMasked clips img to the circle inscribed in it, antialiased by
// 4×4 supersampling.
func circleMasked(img *image.NRGBA) *image.NRGBA {
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	r := float64(b.Dx()) / 2
	cx, cy := float64(b.Min.X)+r, float64(b.Min.Y)+float64(b.Dy())/2
	const ss = 4
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			in := 0
			for sy := range ss {
				for sx := range ss {
					dx := float64(x) + (float64(sx)+0.5)/ss - cx
					dy := float64(y) + (float64(sy)+0.5)/ss - cy
					if dx*dx+dy*dy <= r*r {
						in++
					}
				}
			}
			c := img.NRGBAAt(x, y)
			c.A = uint8((int(c.A)*in + ss*ss/2) / (ss * ss))
			dst.SetNRGBA(x, y, c)
		}
	}
	return dst
}

// whiteSilhouette keeps only the alpha of img, on white: Android draws
// notification icons as a mask.
func whiteSilhouette(img *image.NRGBA) *image.NRGBA {
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.SetNRGBA(x, y, color.NRGBA{255, 255, 255, img.NRGBAAt(x, y).A})
		}
	}
	return dst
}

// borderColor is the alpha-weighted mean colour of img's outer ring
// (1/64 of its width), white when the ring is transparent.
func borderColor(img image.Image) color.NRGBA {
	b := img.Bounds()
	ring := max(1, b.Dx()/64)
	var r, g, bl, a float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if x >= b.Min.X+ring && x < b.Max.X-ring && y >= b.Min.Y+ring && y < b.Max.Y-ring {
				continue
			}
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			w := float64(c.A)
			r, g, bl, a = r+w*float64(c.R), g+w*float64(c.G), bl+w*float64(c.B), a+w
		}
	}
	if a < 1 {
		return color.NRGBA{255, 255, 255, 255}
	}
	return color.NRGBA{uint8(r/a + 0.5), uint8(g/a + 0.5), uint8(bl/a + 0.5), 255}
}

func colorHex(c color.NRGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

// setXMLAttr sets name="value" in tag, the text of a start tag without its
// closing '>': the value is replaced when the attribute is there, else the
// attribute is appended on a new line.
func setXMLAttr(tag, name, value string) string {
	re := regexp.MustCompile(`(\s` + regexp.QuoteMeta(name) + `=")[^"]*"`)
	if loc := re.FindStringSubmatchIndex(tag); loc != nil {
		return tag[:loc[3]] + value + `"` + tag[loc[1]:]
	}
	return tag + "\n        " + name + `="` + value + `"`
}
