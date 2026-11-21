package tango

import (
	"errors"
	"fmt"

	"github.com/tangoui/tango/internal/platform"
)

// webStartURL resolves the initial URL before the backend receives it.
func (a *App) webStartURL(raw string, opts WebViewOptions) (string, error) {
	if !opts.Page {
		return a.resolveURL(raw), nil
	}
	if raw == "" || raw == "about:blank" {
		return "about:blank", nil
	}
	if !a.webPageURL(raw) {
		return "", fmt.Errorf("tango: ordinary WebView requires an HTTP or HTTPS URL")
	}
	return raw, nil
}

func (a *App) webPageURL(raw string) bool {
	return isWebURL(raw) && !sameURL(originOf(raw), a.assetOrigin())
}

func (a *App) webNativeOptions(w WebViewOptions, script string) (platform.WebOptions, string, error) {
	web := platform.WebOptions{UserAgent: w.UserAgent}
	if w.HeaderScope == HeadersAll && len(w.Headers) > 0 {
		web.Headers = w.Headers.Clone()
	}
	if w.Profile != "" {
		if !platform.ValidWebProfile(w.Profile) {
			return web, "", errors.New("tango: invalid WebView profile name")
		}
		p, ok := a.b.(platform.WebProfileSupport)
		if !ok || !p.SupportsWebProfiles() {
			return web, "", fmt.Errorf("tango: WebView profile: %w", ErrUnsupported)
		}
		web.Profile = w.Profile
	}
	if w.Page {
		script = ""
	}
	return web, script, nil
}

// DevToolsOpen reports the native inspector's current visibility. It
// returns ErrUnsupported if the engine does not expose that state.
func (s *Surface) DevToolsOpen() (bool, error) {
	return webAsync(s, func(w platform.WebInspector, done func(bool, error)) {
		done(w.DevToolsOpen())
	})
}

// CloseDevTools closes the native inspector if open. It returns
// ErrUnsupported if the engine does not expose a close operation.
func (s *Surface) CloseDevTools() error {
	return webAsyncErr(s, func(w platform.WebInspector, done func(error)) {
		done(w.CloseDevTools())
	})
}
