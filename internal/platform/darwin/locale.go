//go:build darwin && !ios

package darwin

import "github.com/tangoui/tango/internal/platform"

// The system languages do not depend on a running app, so the package
// registers its source for i18n when imported (i18n then works before
// tango.Run and without a Backend value).
func init() { platform.SetLocaleSource(preferredLocales) }

// PreferredLocales implements platform.Locales.
func (b *backend) PreferredLocales() []string { return preferredLocales() }

// preferredLocales returns [NSLocale preferredLanguages]: the user's
// language list from System Settings, as BCP 47 tags ("zh-Hans-CN").
// NSLocale is thread-safe, so any goroutine may call it.
func preferredLocales() []string {
	load()
	var out []string
	withPool(func() {
		for _, s := range arrayItems(send(class("NSLocale"), "preferredLanguages")) {
			if t := goString(s); t != "" {
				out = append(out, t)
			}
		}
	})
	return out
}
