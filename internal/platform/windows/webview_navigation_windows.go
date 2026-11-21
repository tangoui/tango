//go:build windows && (amd64 || arm64)

package windows

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

const (
	wvAddSourceChanged            = int(unsafe.Offsetof(vtICoreWebView2{}.AddSourceChanged) / slotSize)
	wvAddHistoryChanged           = int(unsafe.Offsetof(vtICoreWebView2{}.AddHistoryChanged) / slotSize)
	wvAddDocumentTitleChanged     = int(unsafe.Offsetof(vtICoreWebView2{}.AddDocumentTitleChanged) / slotSize)
	wvGetDocumentTitle            = int(unsafe.Offsetof(vtICoreWebView2{}.GetDocumentTitle) / slotSize)
	wvGetCanGoBack                = int(unsafe.Offsetof(vtICoreWebView2{}.GetCanGoBack) / slotSize)
	wvGetCanGoForward             = int(unsafe.Offsetof(vtICoreWebView2{}.GetCanGoForward) / slotSize)
	wvGoBack                      = int(unsafe.Offsetof(vtICoreWebView2{}.GoBack) / slotSize)
	wvGoForward                   = int(unsafe.Offsetof(vtICoreWebView2{}.GoForward) / slotSize)
	contentLoadingGetNavigationID = 4
)

func (s *surface) initWebNavigation() {
	addWebViewEvent(s.webview, wvAddContentLoading, func(_, args uintptr) {
		id, _ := comOut[uint64](args, contentLoadingGetNavigationID)
		s.webNavigation(id, platform.WebNavigationCommitted, "", "")
	})
	changed := func(_, _ uintptr) { s.webNavigation(0, platform.WebPageChanged, "", "") }
	for _, slot := range []int{wvAddSourceChanged, wvAddHistoryChanged, wvAddDocumentTitleChanged} {
		addWebViewEvent(s.webview, slot, changed)
	}
}

func (s *surface) webNavigation(id uint64, kind platform.WebNavigationKind, url, reason string) {
	if s.closed || !s.created {
		return
	}
	if url == "" {
		url = comString(s.webview, wvGetSource)
	}
	platform.ReportWebNavigation(s.b.h, s.id, id, kind, platform.WebPageState{
		URL: url, Title: comString(s.webview, wvGetDocumentTitle),
		CanGoBack: comBool(s.webview, wvGetCanGoBack), CanGoForward: comBool(s.webview, wvGetCanGoForward),
	}, reason)
}

func (s *surface) GoInHistory(back bool, done func(bool, error)) {
	if !s.ready || s.webview == 0 {
		done(false, platform.ErrUnsupported)
		return
	}
	can, goTo := wvGetCanGoForward, wvGoForward
	if back {
		can, goTo = wvGetCanGoBack, wvGoBack
	}
	if !comBool(s.webview, can) {
		done(false, nil)
		return
	}
	if hr := comCall(s.webview, goTo); failed(hr) {
		done(false, hresultError("GoInHistory", hr))
		return
	}
	done(true, nil)
}

func (e *embedded) GoInHistory(back bool, done func(bool, error)) {
	e.w.GoInHistory(back, done)
}
