package updateui

// Options configures Start and CheckNow.
type Options struct {
	// Text replaces every text of the window; nil uses the built-in
	// English, Simplified Chinese, Traditional Chinese or Japanese texts
	// for i18n.Locale().
	Text *Strings
}
