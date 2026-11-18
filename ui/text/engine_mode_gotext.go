//go:build !darwin || ios || tango_gotext

package text

import (
	"os"
	"strings"
)

// The switchable engine (every platform but macOS, and macOS with
// -tags tango_gotext):
// UseSystemEngine and TANGO_TEXT_ENGINE choose between the system engine
// and the pure-Go path.

// engineMode is the requested engine: TANGO_TEXT_ENGINE=go|system sets the
// initial value, UseSystemEngine changes it.
var engineMode = engineFromEnv(os.Getenv("TANGO_TEXT_ENGINE"))

// engineFromEnv parses TANGO_TEXT_ENGINE: "go" (or off/0/false) selects the
// pure-Go path; anything else, including unset and "system", the engine.
func engineFromEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "go", "off", "0", "false":
		return false
	}
	return true
}

func useSystemEngine(on bool) (prev bool) {
	reg.mu.Lock()
	prev = reg.engineOn
	changed := prev != on
	reg.engineOn = on
	if changed {
		reg.version++
	}
	reg.mu.Unlock()
	if changed {
		clearCache()
	}
	return prev
}

func engineName() string {
	reg.chain(false)
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.eng != nil {
		return reg.eng.eng.name()
	}
	return "go"
}

// coreTextOnly reports the macOS build (not tango_gotext), where CoreText is
// the only engine and no go-text code is linked.
const coreTextOnly = false
