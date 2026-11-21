//go:build darwin && !ios

package darwin

import "github.com/tangoui/tango/internal/platform"

var pageStateKeys = []string{"URL", "title", "canGoBack", "canGoForward"}

func (s *surface) observePageState(add bool) {
	for _, key := range pageStateKeys {
		if add {
			send(s.web, "addObserver:forKeyPath:options:context:", uintptr(s.delegate), uintptr(nsString(key)), 0, 0)
		} else {
			send(s.web, "removeObserver:forKeyPath:", uintptr(s.delegate), uintptr(nsString(key)))
		}
	}
}

func (s *surface) webNavigation(nav id, kind platform.WebNavigationKind, url, reason string) {
	if s.closed {
		return
	}
	if url == "" {
		url = s.url()
	}
	platform.ReportWebNavigation(s.b.h, s.id, uint64(nav), kind, platform.WebPageState{
		URL: url, Title: goString(send(s.web, "title")),
		CanGoBack: sendBool(s.web, "canGoBack"), CanGoForward: sendBool(s.web, "canGoForward"),
	}, reason)
}

func (s *surface) GoInHistory(back bool, done func(bool, error)) {
	if s.web == 0 {
		done(false, platform.ErrUnsupported)
		return
	}
	selector := "goForward"
	if back {
		selector = "goBack"
	}
	done(send(s.web, selector) != 0, nil)
}

func (e *embedded) GoInHistory(back bool, done func(bool, error)) {
	e.s.GoInHistory(back, done)
}
