//go:build linux && !android

package linux

import "github.com/tangoui/tango/internal/platform"

var webHistory struct {
	title               func(ptr) ptr
	canBack, canForward func(ptr) bool
	back, forward       func(ptr)
	list                func(ptr) ptr
}

func bindWebHistory() {
	var bd binder
	bd.opt(&webHistory.list, "webkit_web_view_get_back_forward_list", libWebKit)
	bd.opt(&webHistory.title, "webkit_web_view_get_title", libWebKit)
	bd.opt(&webHistory.canBack, "webkit_web_view_can_go_back", libWebKit)
	bd.opt(&webHistory.canForward, "webkit_web_view_can_go_forward", libWebKit)
	bd.opt(&webHistory.back, "webkit_web_view_go_back", libWebKit)
	bd.opt(&webHistory.forward, "webkit_web_view_go_forward", libWebKit)
}

func (s *surface) observePageState() {
	changed := func(_, _ ptr) ptr {
		s.webNavigation(platform.WebPageChanged, "", "")
		return 0
	}
	connect(s.web, "notify::uri", 1, changed)
	connect(s.web, "notify::title", 1, changed)
	if webHistory.list != nil {
		connectExt2(webHistory.list(s.web), "changed", func(_ [7]uintptr) uintptr {
			return changed(0, 0)
		})
	}
}

func (s *surface) webNavigation(kind platform.WebNavigationKind, url, reason string) {
	if s.closed {
		return
	}
	if url == "" {
		url = goStr(webkitWebViewGetURI(s.web))
	}
	p := platform.WebPageState{URL: url}
	if webHistory.title != nil {
		p.Title = goStr(webHistory.title(s.web))
	}
	if webHistory.canBack != nil {
		p.CanGoBack = webHistory.canBack(s.web)
	}
	if webHistory.canForward != nil {
		p.CanGoForward = webHistory.canForward(s.web)
	}
	platform.ReportWebNavigation(s.b.host, s.id, s.webNavID, kind, p, reason)
}

func (s *surface) GoInHistory(back bool, done func(bool, error)) {
	can, goTo := webHistory.canForward, webHistory.forward
	if back {
		can, goTo = webHistory.canBack, webHistory.back
	}
	if s.web == 0 || can == nil || goTo == nil {
		done(false, platform.ErrUnsupported)
		return
	}
	ok := can(s.web)
	if ok {
		goTo(s.web)
	}
	done(ok, nil)
}

func (e *embeddedWeb) GoInHistory(back bool, done func(bool, error)) {
	e.w.GoInHistory(back, done)
}
