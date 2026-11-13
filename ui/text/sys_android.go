//go:build android

package text

import (
	"os"
)

// Android font configuration files, newest format first
// (docs/specs/mobile-canvas.md §4.2). Only the first one that parses is
// used; the product customization adds named families to it.
var (
	androidFontConfigs = []struct{ path, dir string }{
		{"/system/etc/font_fallback.xml", "/system/fonts"}, // Android 15+
		{"/system/etc/fonts.xml", "/system/fonts"},
	}
	androidFontCustomization = struct{ path, dir string }{"/product/etc/fonts_customization.xml", "/product/fonts"}
)

// systemFamilies reads the system font configuration: the default
// sans-serif family, then the fallback families in the system's order
// with the CJK family of the default language first among the CJK ones,
// rasterized by the pure-Go path. Without a readable configuration it
// falls back to Roboto and Noto Sans CJK by file name.
func systemFamilies() (sans, mono []*family) {
	var cfg *afConfig
	for _, c := range androidFontConfigs {
		f, err := os.Open(c.path)
		if err != nil {
			continue // missing, or not readable by apps (SELinux)
		}
		parsed, err := parseFontsXML(f, c.dir)
		f.Close()
		if err == nil && len(parsed.families) > 0 {
			cfg = parsed
			break
		}
	}
	if cfg != nil {
		if f, err := os.Open(androidFontCustomization.path); err == nil {
			if custom, err := parseFontsXML(f, androidFontCustomization.dir); err == nil {
				cfg.merge(custom)
			}
			f.Close()
		}
		sp, mp := cfg.plan(DefaultLang())
		if len(sp) > 0 {
			return afFamilies(sp), afFamilies(mp)
		}
	}
	sans = []*family{
		sysFont{name: "Roboto", paths: []string{"/system/fonts/Roboto-Regular.ttf"}}.family(),
		sysFont{name: "Noto Sans CJK", paths: []string{"/system/fonts/NotoSansCJK-Regular.ttc"}, families: []string{"Noto Sans CJK SC"}}.family(),
		sysFont{name: "Droid Sans Fallback", paths: []string{"/system/fonts/DroidSansFallback.ttf"}}.family(),
	}
	mono = []*family{sysFont{name: "Droid Sans Mono", paths: []string{"/system/fonts/DroidSansMono.ttf"}}.family()}
	return sans, mono
}
