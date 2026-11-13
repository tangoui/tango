//go:build windows && (amd64 || arm64)

package windows

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var procGetUserPreferredUILanguages = kernel32.export("GetUserPreferredUILanguages")

// The display-language list needs no running app, so importing the
// package is enough to make it the locale source for i18n.
func init() { platform.SetLocaleSource(preferredLocales) }

// PreferredLocales implements platform.Locales.
func (b *Backend) PreferredLocales() []string { return preferredLocales() }

// preferredLocales lists the user's Windows display languages, most
// preferred first, as BCP 47 tags such as "zh-CN". It returns nil when
// Windows cannot say, and i18n then falls back to the environment.
func preferredLocales() []string {
	// MUI_LANGUAGE_NAME: ask for names rather than LANGIDs.
	const byName = 0x8
	// Call with *buf (nil the first time, to learn the size in units) and
	// report success.
	query := func(count, size *uint32, buf *uint16) bool {
		ok, _, _ := procGetUserPreferredUILanguages.Call(byName, uintptr(unsafe.Pointer(count)),
			uintptr(unsafe.Pointer(buf)), uintptr(unsafe.Pointer(size)))
		return ok != 0
	}
	var count, units uint32
	if !query(&count, &units, nil) || units == 0 {
		return nil
	}
	list := make([]uint16, units)
	if !query(&count, &units, &list[0]) {
		return nil
	}
	return splitMultiSZ(list) // a double-NUL-terminated string list
}
