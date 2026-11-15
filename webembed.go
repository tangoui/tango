package tango

// WebViews embedded in a canvas surface's layout
// (docs/specs/webview-embed.md), created through
// corehook.NewEmbeddedWebView by package uihost, which places them.

import (
	"errors"
	"fmt"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

var (
	errEmbedNotMain = errors.New("tango: embedded WebViews must be created on the main thread")
	errEmbedClosed  = errors.New("tango: embedded WebView closed while being created")
)

func init() {
	corehook.NewEmbeddedWebView = func(parent any, opts any, script string) (any, platform.EmbeddedWebView, error) {
		p, ok := parent.(*Surface)
		if !ok || p == nil {
			return nil, nil, errors.New("tango: NewEmbeddedWebView: not a *tango.Surface")
		}
		s, ev, err := p.embedWebView(opts.(SurfaceOptions), script)
		if err != nil {
			return nil, nil, err
		}
		return s, ev, nil
	}
	corehook.EmbeddedParent = func(surface any) any {
		if s, ok := surface.(*Surface); ok && s != nil && s.embedIn != nil {
			return s.embedIn
		}
		return nil
	}
}

// embedWebView creates a WebView surface inside p. Main thread.
func (p *Surface) embedWebView(opts SurfaceOptions, script string) (*Surface, platform.EmbeddedWebView, error) {
	a := p.app
	if a.b == nil {
		return nil, nil, ErrNoBackend
	}
	if !a.isMain() {
		return nil, nil, errEmbedNotMain
	}
	if p.closed.Load() || p.ns == nil {
		return nil, nil, errAttachClosed
	}
	em, ok := a.b.(platform.WebViewEmbedder)
	if !ok {
		return nil, nil, fmt.Errorf("tango: embedded WebView: %w", ErrUnsupported)
	}
	opts.Title, opts.Hidden = "", false
	u := a.resolveURL(opts.URL)
	var ev platform.EmbeddedWebView
	s, err := a.addSurface(opts, originOf(u), true, func(s *Surface) (platform.Surface, error) {
		s.embedIn = p
		wv := opts.WebView
		s.web.opts = opts
		s.web.userAgent = wv.UserAgent
		s.web.coreNav = u
		s.web.pageOrigin = originOf(u)
		s.setHeaders(wv.Headers, wv.HeaderScope)
		web := platform.WebOptions{UserAgent: wv.UserAgent}
		if wv.HeaderScope == HeadersAll && len(wv.Headers) > 0 {
			web.Headers = wv.Headers.Clone()
		}
		bridge := a.bridgeScript()
		if script != "" {
			bridge += "\n;" + script
		}
		e, err := em.NewEmbeddedWebView(s.id, p.id, platform.SurfaceOptions{
			URL:          u,
			Debug:        opts.Debug,
			BridgeScript: bridge,
		}, web)
		if err != nil {
			return nil, err
		}
		ev = e
		return e, nil
	})
	if err != nil {
		return nil, nil, err
	}
	if s.closed.Load() {
		return nil, nil, errEmbedClosed
	}
	return s, ev, nil
}
