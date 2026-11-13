//go:build !ios

package text

// systemFamilies is the pure-Go chain (UseSystemEngine(false), or CoreText
// unavailable). PingFang is missing here: its .ttc is not readable by
// go-text; the CoreText engine (engine_darwin.go) provides it.
func systemFamilies() (sans, mono []*family) {
	cand := []sysFont{
		{name: "SF Pro", paths: []string{"/System/Library/Fonts/SFNS.ttf"}, extra: []string{"/System/Library/Fonts/SFNSItalic.ttf"}},
		{name: "Helvetica Neue", paths: []string{"/System/Library/Fonts/HelveticaNeue.ttc"}, families: []string{"Helvetica Neue"}},
		{name: "Hiragino Sans GB", paths: []string{"/System/Library/Fonts/Hiragino Sans GB.ttc"}, families: []string{"Hiragino Sans GB"}},
		{name: "Heiti SC", paths: []string{"/System/Library/Fonts/STHeiti Light.ttc"}, families: []string{"Heiti SC"}},
		{name: "Heiti SC Medium", paths: []string{"/System/Library/Fonts/STHeiti Medium.ttc"}, families: []string{"Heiti SC"}},
		{name: "Arial Unicode", paths: []string{"/Library/Fonts/Arial Unicode.ttf", "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"}},
		{name: "Apple Symbols", paths: []string{"/System/Library/Fonts/Apple Symbols.ttf"}},
	}
	for _, c := range cand {
		sans = append(sans, c.family())
	}
	for _, c := range []sysFont{
		{name: "SF Mono", paths: []string{"/System/Library/Fonts/SFNSMono.ttf"}, extra: []string{"/System/Library/Fonts/SFNSMonoItalic.ttf"}},
		{name: "Menlo", paths: []string{"/System/Library/Fonts/Menlo.ttc"}, families: []string{"Menlo"}},
	} {
		mono = append(mono, c.family())
	}
	return
}
