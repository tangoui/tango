package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// iconTestImage draws a w×h test image: fill everywhere, then inner (if
// not nil) on the centre square of side w*innerFrac.
func iconTestImage(w, h int, fill color.Color, inner color.Color, innerFrac float64) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(fill), image.Point{}, draw.Src)
	if inner != nil {
		side := int(float64(w) * innerFrac)
		o := (w - side) / 2
		draw.Draw(img, image.Rect(o, o, o+side, o+side), image.NewUniform(inner), image.Point{}, draw.Src)
	}
	return img
}

func saveTestPNG(t *testing.T, cfg *Config, rel string, img image.Image) {
	t.Helper()
	if err := writePNG(cfg.Path(rel), img); err != nil {
		t.Fatal(err)
	}
}

func pngSize(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return c.Width, c.Height
}

func readTestPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

var (
	testBlue   = color.NRGBA{0x16, 0x77, 0xFF, 0xFF}
	testOrange = color.NRGBA{0xFF, 0x80, 0x00, 0xFF}
	testClear  = color.NRGBA{}
)

func TestWriteIOSAssetCatalog(t *testing.T) {
	cfg := &Config{Name: "A", ID: "a.b", Version: "1", Icon: "icon.png",
		Icons: Icons{IOS: IOSIcons{Dark: "dark.png", Tinted: "tinted.png"}}}
	cfg.dir = t.TempDir()
	// Master with transparent corners: iOS must still get an opaque icon.
	saveTestPNG(t, cfg, "icon.png", iconTestImage(256, 256, testClear, testBlue, 0.5))
	saveTestPNG(t, cfg, "dark.png", iconTestImage(512, 512, testClear, testOrange, 0.5))
	saveTestPNG(t, cfg, "tinted.png", iconTestImage(1024, 1024, color.Gray{0x80}, nil, 0))
	catalog := filepath.Join(t.TempDir(), "Assets.xcassets")

	icon, variants, err := writeIOSAssetCatalog(cfg, catalog, true)
	if err != nil || !icon || !variants {
		t.Fatalf("icon=%v variants=%v err=%v", icon, variants, err)
	}
	set := filepath.Join(catalog, "AppIcon.appiconset")
	var got xcassetContents
	b, err := os.ReadFile(filepath.Join(set, "Contents.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Info.Version != 1 || len(got.Images) != 3 {
		t.Fatalf("Contents.json = %s", b)
	}
	want := map[string]string{"icon.png": "", "icon-dark.png": "dark", "icon-tinted.png": "tinted"}
	for _, im := range got.Images {
		v, ok := want[im.Filename]
		if !ok || im.Idiom != "universal" || im.Platform != "ios" || im.Size != "1024x1024" {
			t.Errorf("unexpected image %+v", im)
		}
		switch {
		case v == "" && len(im.Appearances) != 0:
			t.Errorf("%s has appearances %+v", im.Filename, im.Appearances)
		case v != "" && (len(im.Appearances) != 1 || im.Appearances[0] != xcassetAppearance{"luminosity", v}):
			t.Errorf("%s appearances = %+v, want luminosity/%s", im.Filename, im.Appearances, v)
		}
		if w, h := pngSize(t, filepath.Join(set, im.Filename)); w != 1024 || h != 1024 {
			t.Errorf("%s is %dx%d", im.Filename, w, h)
		}
	}
	// The raw JSON uses Apple's key names.
	if !strings.Contains(string(b), `"appearances": [`) || !strings.Contains(string(b), `"appearance": "luminosity"`) {
		t.Errorf("Contents.json keys:\n%s", b)
	}
	if _, _, _, a := readTestPNG(t, filepath.Join(set, "icon.png")).At(0, 0).RGBA(); a != 0xffff {
		t.Error("iOS icon corner is transparent; it must be flattened onto white")
	}
	if _, _, _, a := readTestPNG(t, filepath.Join(set, "icon-dark.png")).At(0, 0).RGBA(); a != 0 {
		t.Error("dark variant lost its transparency")
	}
	var top xcassetContents
	if b, err := os.ReadFile(filepath.Join(catalog, "Contents.json")); err != nil || json.Unmarshal(b, &top) != nil || top.Info.Author != "tan" {
		t.Errorf("catalog Contents.json: %s %v", b, err)
	}

	// Fallback for an Xcode without appearances: the variants go away.
	if _, variants, err := writeIOSAssetCatalog(cfg, catalog, false); err != nil || variants {
		t.Fatalf("variants=%v err=%v", variants, err)
	}
	b, _ = os.ReadFile(filepath.Join(set, "Contents.json"))
	got = xcassetContents{}
	if err := json.Unmarshal(b, &got); err != nil || len(got.Images) != 1 || strings.Contains(string(b), "appearances") {
		t.Errorf("without appearances: %s", b)
	}
	if fileExists(filepath.Join(set, "icon-dark.png")) {
		t.Error("stale dark variant kept")
	}

	// No icon, nothing written.
	if icon, _, err := writeIOSAssetCatalog(&Config{}, filepath.Join(t.TempDir(), "x.xcassets"), true); icon || err != nil {
		t.Errorf("no icon: icon=%v err=%v", icon, err)
	}
	// A missing variant is an error.
	cfg.Icons.IOS.Dark = "missing.png"
	if _, _, err := writeIOSAssetCatalog(cfg, catalog, true); err == nil || !strings.Contains(err.Error(), "icons.ios.dark") {
		t.Errorf("missing dark variant: %v", err)
	}
}

func TestXcodeMajor(t *testing.T) {
	for in, want := range map[string]int{
		"Xcode 26.1\nBuild version 17B55":  26,
		"Xcode 15.4\nBuild version 15F31d": 15,
		"Xcode 16\n":                       16,
		"garbage":                          0,
	} {
		if got := xcodeMajor(in); got != want {
			t.Errorf("xcodeMajor(%q) = %d, want %d", in, got, want)
		}
	}
}

func readFileT(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestWriteAndroidIconsDefaults(t *testing.T) {
	cfg := &Config{Name: "A", ID: "a.b", Version: "1", Icon: "icon.png"}
	cfg.dir = t.TempDir()
	// Blue border, orange centre: the background colour is the border's.
	master := iconTestImage(1024, 1024, testBlue, testOrange, 0.5)
	res := t.TempDir()
	if err := writeAndroidIcons(cfg, master, res); err != nil {
		t.Fatal(err)
	}
	for _, d := range []struct {
		name          string
		legacy, layer int
	}{{"mdpi", 48, 108}, {"hdpi", 72, 162}, {"xhdpi", 96, 216}, {"xxhdpi", 144, 324}, {"xxxhdpi", 192, 432}} {
		dir := filepath.Join(res, "mipmap-"+d.name)
		for _, f := range []string{"ic_launcher.png", "ic_launcher_round.png"} {
			if w, h := pngSize(t, filepath.Join(dir, f)); w != d.legacy || h != d.legacy {
				t.Errorf("%s/%s is %dx%d, want %d", d.name, f, w, h, d.legacy)
			}
		}
		fg := filepath.Join(dir, "ic_launcher_foreground.png")
		if w, h := pngSize(t, fg); w != d.layer || h != d.layer {
			t.Errorf("%s foreground is %dx%d, want %d", d.name, w, h, d.layer)
		}
		// The master fills the visible 72 dp: transparent outside, opaque inside.
		img := readTestPNG(t, fg)
		edge := d.layer / 6
		if _, _, _, a := img.At(edge-1, edge-1).RGBA(); a != 0 {
			t.Errorf("%s foreground opaque outside 72 dp", d.name)
		}
		if _, _, _, a := img.At(edge+1, edge+1).RGBA(); a != 0xffff {
			t.Errorf("%s foreground transparent inside 72 dp", d.name)
		}
		round := readTestPNG(t, filepath.Join(dir, "ic_launcher_round.png"))
		if _, _, _, a := round.At(0, 0).RGBA(); a != 0 {
			t.Errorf("%s round icon corner is not transparent", d.name)
		}
		if _, _, _, a := round.At(d.legacy/2, d.legacy/2).RGBA(); a != 0xffff {
			t.Errorf("%s round icon centre is not opaque", d.name)
		}
		for _, f := range []string{"ic_launcher_background.png", "ic_launcher_monochrome.png"} {
			if fileExists(filepath.Join(dir, f)) {
				t.Errorf("%s/%s written without being asked for", d.name, f)
			}
		}
	}
	for _, f := range []string{"ic_launcher.xml", "ic_launcher_round.xml"} {
		x := readFileT(t, filepath.Join(res, "mipmap-anydpi-v26", f))
		for _, want := range []string{
			`<adaptive-icon xmlns:android="http://schemas.android.com/apk/res/android">`,
			`<background android:drawable="@color/ic_launcher_background" />`,
			`<foreground android:drawable="@mipmap/ic_launcher_foreground" />`,
		} {
			if !strings.Contains(x, want) {
				t.Errorf("%s lacks %q:\n%s", f, want, x)
			}
		}
		if strings.Contains(x, "monochrome") {
			t.Errorf("%s has a monochrome layer:\n%s", f, x)
		}
	}
	if v := readFileT(t, filepath.Join(res, "values", "tango_icon.xml")); !strings.Contains(v, `<color name="ic_launcher_background">#1677FF</color>`) {
		t.Errorf("background colour:\n%s", v)
	}
	if fileExists(filepath.Join(res, "drawable-mdpi")) {
		t.Error("notification icon written without icons.notification")
	}
}

func TestWriteAndroidIconsOverrides(t *testing.T) {
	cfg := &Config{Name: "A", ID: "a.b", Version: "1", Icon: "icon.png", Icons: Icons{
		Android:      AndroidIcon{Foreground: "fg.png", Background: "bg.png", Monochrome: "mono.png"},
		Notification: "notify.png",
	}}
	cfg.dir = t.TempDir()
	saveTestPNG(t, cfg, "fg.png", iconTestImage(432, 432, testClear, testOrange, 0.5))
	saveTestPNG(t, cfg, "bg.png", iconTestImage(432, 432, testBlue, nil, 0))
	saveTestPNG(t, cfg, "mono.png", iconTestImage(432, 432, testClear, color.White, 0.5))
	saveTestPNG(t, cfg, "notify.png", iconTestImage(96, 96, testClear, testOrange, 0.5))
	res := t.TempDir()
	if err := writeAndroidIcons(cfg, iconTestImage(1024, 1024, testBlue, nil, 0), res); err != nil {
		t.Fatal(err)
	}
	for _, d := range []struct {
		name          string
		layer, notify int
	}{{"mdpi", 108, 24}, {"hdpi", 162, 36}, {"xhdpi", 216, 48}, {"xxhdpi", 324, 72}, {"xxxhdpi", 432, 96}} {
		for _, f := range []string{"ic_launcher_foreground.png", "ic_launcher_background.png", "ic_launcher_monochrome.png"} {
			if w, h := pngSize(t, filepath.Join(res, "mipmap-"+d.name, f)); w != d.layer || h != d.layer {
				t.Errorf("%s/%s is %dx%d, want %d", d.name, f, w, h, d.layer)
			}
		}
		p := filepath.Join(res, "drawable-"+d.name, "ic_stat_tango.png")
		if w, h := pngSize(t, p); w != d.notify || h != d.notify {
			t.Errorf("%s notification icon is %dx%d, want %d", d.name, w, h, d.notify)
		}
		// White with the artwork's alpha.
		img := readTestPNG(t, p)
		if c := color.NRGBAModel.Convert(img.At(d.notify/2, d.notify/2)).(color.NRGBA); c != (color.NRGBA{255, 255, 255, 255}) {
			t.Errorf("%s notification centre = %v, want opaque white", d.name, c)
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Errorf("%s notification corner is not transparent", d.name)
		}
	}
	x := readFileT(t, filepath.Join(res, "mipmap-anydpi-v26", "ic_launcher.xml"))
	for _, want := range []string{
		`<background android:drawable="@mipmap/ic_launcher_background" />`,
		`<monochrome android:drawable="@mipmap/ic_launcher_monochrome" />`,
	} {
		if !strings.Contains(x, want) {
			t.Errorf("adaptive icon lacks %q:\n%s", want, x)
		}
	}
	if fileExists(filepath.Join(res, "values", "tango_icon.xml")) {
		t.Error("colour resource written for a PNG background")
	}

	// A colour background.
	cfg.Icons.Android = AndroidIcon{Background: "#00aa11"}
	res = t.TempDir()
	if err := writeAndroidIcons(cfg, iconTestImage(64, 64, testBlue, nil, 0), res); err != nil {
		t.Fatal(err)
	}
	if v := readFileT(t, filepath.Join(res, "values", "tango_icon.xml")); !strings.Contains(v, ">#00AA11</color>") {
		t.Errorf("colour background:\n%s", v)
	}

	// Missing files are errors naming the key.
	cfg.Icons.Notification = "nope.png"
	if err := writeAndroidIcons(cfg, nil, t.TempDir()); err == nil || !strings.Contains(err.Error(), "icons.notification") {
		t.Errorf("missing notification icon: %v", err)
	}
}

func TestAndroidManifestIcons(t *testing.T) {
	c := &Config{Name: "A", ID: "a.b", Version: "1", Icons: Icons{Notification: "n.png"}}
	got, err := androidManifest(manifestTemplate, c, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`android:icon="@mipmap/ic_launcher"`,
		`android:roundIcon="@mipmap/ic_launcher_round"`,
		`<meta-data
            android:name="com.google.firebase.messaging.default_notification_icon"
            android:resource="@drawable/ic_stat_tango" />`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("manifest lacks %q:\n%s", want, got)
		}
	}
	app := strings.Index(got, "<application")
	appEnd := app + strings.Index(got[app:], ">")
	if meta := strings.Index(got, "<meta-data"); meta < appEnd || meta > strings.Index(got, "</application>") {
		t.Errorf("meta-data not inside <application>:\n%s", got)
	}
	if strings.Contains(got, "sym_def_app_icon") {
		t.Error("template icon kept")
	}
	// Again on its own output: nothing is duplicated.
	again, err := androidManifest(got, c, true)
	if err != nil || strings.Count(again, "default_notification_icon") != 1 || strings.Count(again, "roundIcon") != 1 {
		t.Errorf("not idempotent: %v\n%s", err, again)
	}
	// Notification only (no launcher icon).
	only, _ := androidManifest(manifestTemplate, c, false)
	if strings.Contains(only, "roundIcon") || !strings.Contains(only, "default_notification_icon") {
		t.Errorf("notification-only manifest:\n%s", only)
	}
}

func TestSetXMLAttr(t *testing.T) {
	tag := "<application\n        android:label=\"x\"\n        android:icon=\"old\""
	if got := setXMLAttr(tag, "android:icon", "new"); got != "<application\n        android:label=\"x\"\n        android:icon=\"new\"" {
		t.Errorf("replace: %q", got)
	}
	if got := setXMLAttr("<application android:label=\"x\"", "android:roundIcon", "r"); got != "<application android:label=\"x\"\n        android:roundIcon=\"r\"" {
		t.Errorf("append: %q", got)
	}
}

func TestBorderColor(t *testing.T) {
	if got := borderColor(iconTestImage(100, 100, testBlue, testOrange, 0.8)); got != testBlue {
		t.Errorf("borderColor = %v, want %v", got, testBlue)
	}
	if got := borderColor(iconTestImage(100, 100, testClear, testOrange, 0.5)); got != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("transparent border = %v, want white", got)
	}
}
