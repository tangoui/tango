package text

// systemFamilies is the pure-Go chain on iOS (UseSystemEngine(false), or
// CoreText unavailable). iOS keeps its fonts in subdirectories of
// /System/Library/Fonts (Core, CoreUI, CoreAddition), not in the macOS
// locations sys_darwin.go lists; the app sandbox may read them. PingFang
// lives in a private framework and its .ttc is not readable by go-text,
// so Chinese falls back to Hiragino Sans (which covers the common Han
// characters) and the Heiti/CJK fallback files; the CoreText engine
// (engine_darwin.go) provides PingFang.
func systemFamilies() (sans, mono []*family) {
	const (
		core  = "/System/Library/Fonts/Core/"
		ui    = "/System/Library/Fonts/CoreUI/"
		addon = "/System/Library/Fonts/CoreAddition/"
	)
	for _, c := range []sysFont{
		{name: "SF Pro", paths: []string{core + "SFUI.ttf"}, extra: []string{core + "SFUIItalic.ttf"}},
		{name: "Helvetica Neue", paths: []string{core + "HelveticaNeue.ttc"}, families: []string{"Helvetica Neue"}},
		{name: "Hiragino Sans", paths: []string{core + "HiraginoKakuGothic.ttc"}},
		{name: "CJK Symbols Fallback", paths: []string{core + "CJKSymbolsFallback.ttc"}},
		{name: "Apple SD Gothic Neo", paths: []string{core + "AppleSDGothicNeo.ttc"}},
		{name: "Apple Symbols", paths: []string{core + "AppleSymbols.ttf"}},
		{name: "STIX", paths: []string{addon + "STIXGeneral.otf"}},
	} {
		sans = append(sans, c.family())
	}
	for _, c := range []sysFont{
		{name: "SF Mono", paths: []string{core + "SFUIMono.ttf"}, extra: []string{core + "SFUIMonoItalic.ttf"}},
		{name: "Menlo", paths: []string{ui + "Menlo.ttc"}, families: []string{"Menlo"}},
		{name: "Courier New", paths: []string{core + "CourierNew.ttf"}, extra: []string{core + "CourierNewBold.ttf"}},
	} {
		mono = append(mono, c.family())
	}
	return
}
