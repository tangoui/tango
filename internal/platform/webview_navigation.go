package platform

// WebNavigationKind is a main-frame load phase or a page metadata change.
type WebNavigationKind uint8

const (
	WebNavigationStarted WebNavigationKind = iota + 1
	WebNavigationCommitted
	WebNavigationFinished
	WebNavigationFailed
	WebPageChanged
)

// WebPageState is metadata read from a native WebView.
type WebPageState struct {
	URL, Title              string
	CanGoBack, CanGoForward bool
}

// WebNavigationHost receives main-frame events. The native token stays
// the same through redirects. Metadata changes have no token.
type WebNavigationHost interface {
	OnWebNavigation(SurfaceID, uint64, WebNavigationKind, WebPageState, string)
}

func ReportWebNavigation(h Host, id SurfaceID, token uint64, kind WebNavigationKind, page WebPageState, err string) {
	if w, ok := h.(WebNavigationHost); ok {
		w.OnWebNavigation(id, token, kind, page, err)
	}
}

// WebHistory controls the native back-forward list. GoInHistory reports
// whether there was an entry in the requested direction.
type WebHistory interface {
	GoInHistory(back bool, done func(bool, error))
}
