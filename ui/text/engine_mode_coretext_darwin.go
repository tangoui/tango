//go:build darwin && !ios && tango_coretext

package text

// The macOS build with -tags tango_coretext has one engine, CoreText:
// there is no pure-Go path to switch to, so UseSystemEngine changes nothing
// and TANGO_TEXT_ENGINE is not read (see UseSystemEngine).

// engineMode keeps the engine on for good.
const engineMode = true

// coreTextOnly reports the macOS tango_coretext build, where CoreText is
// the only engine and no go-text code is linked.
const coreTextOnly = true

func useSystemEngine(bool) (prev bool) { return true }

func engineName() string { return "coretext" }
