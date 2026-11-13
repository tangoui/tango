//go:build linux && !android

package linux

import "github.com/tangoui/tango/internal/platform"

// PreferredLocales implements platform.Locales. On Linux the user's
// languages are the POSIX locale variables (LANGUAGE, LC_ALL, LC_MESSAGES,
// LANG), which is also all GLib's g_get_language_names consults, so no
// library call is needed.
func (b *backend) PreferredLocales() []string { return platform.EnvLocales() }
