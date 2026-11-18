//go:build darwin && !ios && !tango_gotext

package text

// The default macOS build has one engine, CoreText: there is no pure-Go
// path to switch to, so UseSystemEngine changes nothing and
// TANGO_TEXT_ENGINE is not read (see UseSystemEngine).

// engineMode keeps the engine on for good.
const engineMode = true

// coreTextOnly reports the default macOS build, where CoreText is the only
// engine and no go-text code is linked.
const coreTextOnly = true

func useSystemEngine(bool) (prev bool) { return true }

func engineName() string { return "coretext" }
