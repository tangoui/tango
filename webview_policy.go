package tango

// The default WebView security policy: which pages are trusted, and how
// permission requests, navigations, new windows and HTTP authentication
// are decided (the surface's handler, then the app's, then the default).
// See webview.go for the public API.

import (
	"net/url"
	"slices"
	"strings"

	"github.com/tangoui/tango/internal/platform"
)

// ---------------------------------------------------------------------------
// Page origin and trust

// noteCoreNav records a navigation the core starts itself, which the
// navigation policy is not asked about. Main thread.
func (s *Surface) noteCoreNav(abs string) { s.web.coreNav = abs }

// notePageLoaded records the origin of the page that loaded (u "" when
// the platform does not tell: taken as the page the core navigated to).
// Main thread.
func (s *Surface) notePageLoaded(u string) {
	if u == "" {
		s.web.pageOrigin = s.navOrigin
		return
	}
	s.web.pageOrigin = originOf(u)
}

// pageTrusted reports whether the page currently shown is one of the
// app's own (Surface.trusts). Main thread.
func (s *Surface) pageTrusted() bool { return s.trusts(s.web.pageOrigin) }

// TrustsOrigin tells backends that act on some page messages themselves
// (the Windows file-drop message) whether origin is a trusted page of
// surface id. Duck-typed by the backends, outside the platform contract.
func (h host) TrustsOrigin(id platform.SurfaceID, origin string) bool {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	return s != nil && !s.closed.Load() && s.trusts(origin)
}

// ---------------------------------------------------------------------------
// Host callbacks (platform.WebHost)

var _ platform.WebHost = host{}

func (h host) OnPermissionRequest(id platform.SurfaceID, req platform.PermissionRequest, done func(platform.PermissionDecision)) {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	if s == nil || s.closed.Load() {
		done(PermissionDeny)
		return
	}
	done(a.decidePermission(s, req))
}

func (a *App) decidePermission(s *Surface, req PermissionRequest) PermissionDecision {
	req.Kinds = slices.Clone(req.Kinds)
	// The requesting frame's origin; where the platform does not tell, the
	// page's.
	trusted := s.pageTrusted()
	if req.Origin != "" {
		trusted = s.trusts(req.Origin)
	}
	s.mu.Lock()
	sfn := s.web.onPermission
	s.mu.Unlock()
	if sfn != nil {
		if d := guardCall(a, whereEvent, PermissionDefault, func() PermissionDecision { return sfn(req) }); d != PermissionDefault {
			return d
		}
	}
	a.mu.Lock()
	afn := a.web.onPermission
	a.mu.Unlock()
	if afn != nil {
		if d := guardCall(a, whereEvent, PermissionDefault, func() PermissionDecision { return afn(s, req) }); d != PermissionDefault {
			return d
		}
	}
	if !trusted {
		// Other sites get nothing by default: the app's policy grants
		// its own pages, not whatever page the surface ended up on.
		return PermissionDeny
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return combinePolicy(a.web.policy, req.Kinds)
}

// combinePolicy decides a request from the per-kind policy.
func combinePolicy(policy map[WebPermission]PermissionDecision, kinds []WebPermission) PermissionDecision {
	if len(kinds) == 0 {
		kinds = []WebPermission{WebPermOther}
	}
	allow := true
	for _, k := range kinds {
		d, ok := policy[k]
		if !ok {
			d = policy[""]
		}
		switch d {
		case PermissionDeny:
			return PermissionDeny
		case PermissionAllow:
		default:
			allow = false
		}
	}
	if allow {
		return PermissionAllow
	}
	return PermissionPrompt
}

func (h host) ShouldNavigate(id platform.SurfaceID, nav platform.NavigationRequest) bool {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	if s == nil || s.closed.Load() {
		return true
	}
	if s.web.coreNav != "" && sameURL(s.web.coreNav, nav.URL) {
		s.web.coreNav = ""
		return true
	}
	s.mu.Lock()
	sfn := s.web.onNavigate
	s.mu.Unlock()
	a.mu.Lock()
	afn := a.web.onNavigate
	a.mu.Unlock()
	if sfn == nil && afn == nil {
		return a.defaultNavigation(s, nav)
	}
	// A panicking handler blocks the navigation.
	if sfn != nil && !guardCall(a, whereEvent, false, func() bool { return sfn(nav) }) {
		return false
	}
	return afn == nil || guardCall(a, whereEvent, false, func() bool { return afn(s, nav) })
}

// defaultNavigation is the navigation decision without a policy: other
// sites open in the system browser (as window.open does), the app's own
// pages and non-web URLs load in the surface. Main thread.
func (a *App) defaultNavigation(s *Surface, nav NavigationRequest) bool {
	if !isWebURL(nav.URL) || s.trusts(originOf(nav.URL)) {
		return true
	}
	req := NewWindowRequest{URL: nav.URL, UserInitiated: nav.UserInitiated}
	// Out of the native navigation callback, like OnNewWindow.
	a.Post(func() { a.doNewWindow(s, req, NewWindowBrowser) })
	return false
}

// isWebURL reports whether u is an absolute http or https URL.
func isWebURL(u string) bool {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return false
	}
	switch strings.ToLower(p.Scheme) {
	case "http", "https":
		return true
	}
	return false
}

// sameURL compares URLs ignoring a trailing slash on an empty path, the
// case of scheme and host, and the fragment.
func sameURL(x, y string) bool {
	norm := func(u string) string {
		p, err := url.Parse(u)
		if err != nil {
			return u
		}
		p.Scheme, p.Host, p.Fragment = strings.ToLower(p.Scheme), strings.ToLower(p.Host), ""
		if p.Path == "" && p.Host != "" {
			p.Path = "/"
		}
		return p.String()
	}
	return norm(x) == norm(y)
}

func (h host) OnNewWindow(id platform.SurfaceID, req platform.NewWindowRequest) {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	if s == nil || s.closed.Load() {
		return
	}
	act := a.decideNewWindow(s, req)
	// Out of the native delegate callback: creating windows or navigating
	// from inside it is not safe everywhere.
	a.Post(func() { a.doNewWindow(s, req, act) })
}

func (a *App) decideNewWindow(s *Surface, req NewWindowRequest) NewWindowAction {
	s.mu.Lock()
	sfn := s.web.onNewWindow
	s.mu.Unlock()
	if sfn != nil {
		if act := guardCall(a, whereEvent, NewWindowDefault, func() NewWindowAction { return sfn(req) }); act != NewWindowDefault {
			return act
		}
	}
	a.mu.Lock()
	afn := a.web.onNewWindow
	a.mu.Unlock()
	if afn != nil {
		if act := guardCall(a, whereEvent, NewWindowDefault, func() NewWindowAction { return afn(s, req) }); act != NewWindowDefault {
			return act
		}
	}
	if req.URL == "" {
		return NewWindowDeny
	}
	if s.trusts(originOf(req.URL)) {
		return NewWindowSurface
	}
	if p, err := url.Parse(req.URL); err == nil {
		switch strings.ToLower(p.Scheme) {
		case "http", "https", "mailto":
			return NewWindowBrowser
		}
	}
	return NewWindowDeny
}

func (a *App) doNewWindow(s *Surface, req NewWindowRequest, act NewWindowAction) {
	switch act {
	case NewWindowBrowser:
		if sh, ok := a.b.(platform.Shell); ok && req.URL != "" {
			_ = sh.OpenURL(req.URL)
		}
	case NewWindowCurrent:
		if req.URL != "" {
			s.Navigate(req.URL)
		}
	case NewWindowSurface:
		opts := SurfaceOptions{
			Title:     s.Title(),
			Width:     req.Width,
			Height:    req.Height,
			Resizable: true,
			URL:       req.URL,
			Debug:     s.web.opts.Debug,
			WebView:   s.web.opts.WebView,
		}
		if opts.Width <= 0 || opts.Height <= 0 {
			opts.Width, opts.Height = s.Size()
		}
		_, _ = a.NewSurface(opts)
	}
}

func (h host) OnAuthChallenge(id platform.SurfaceID, ch platform.AuthChallenge, done func(platform.AuthResponse)) {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	if s == nil || s.closed.Load() {
		done(AuthResponse{Action: AuthCancel})
		return
	}
	s.mu.Lock()
	sfn := s.web.onAuth
	s.mu.Unlock()
	if sfn != nil {
		if r := guardCall(a, whereEvent, AuthResponse{}, func() AuthResponse { return sfn(ch) }); r.Action != AuthDefault {
			done(r)
			return
		}
	}
	a.mu.Lock()
	afn := a.web.onAuth
	a.mu.Unlock()
	if afn != nil {
		done(guardCall(a, whereEvent, AuthResponse{}, func() AuthResponse { return afn(s, ch) }))
		return
	}
	done(AuthResponse{})
}
