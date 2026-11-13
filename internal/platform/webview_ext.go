package platform

import "net/http"

// Contract extensions for advanced WebView features (round 4): page
// permission requests, downloads, cookies and website data, HTTP
// authentication, find in page, user agent and extra request headers,
// new-window requests, PDF and screenshot capture, and a cancellable
// navigation policy.
//
// Like ext.go everything here is OPTIONAL and discovered with type
// assertions:
//
//   - The core's Host always also implements WebHost. Backends type-assert
//     their Host to WebHost (fake hosts may not implement it) and call it
//     from the native WebView delegates. A backend that cannot observe an
//     event simply never calls the method; the platform then behaves as it
//     does by default.
//   - The per-surface controls (WebFinder, WebCookies, ...) are optional
//     interfaces on the platform.Surface value of a WebView surface. Canvas
//     surfaces do not implement them.
//
// Threading is the same as backend.go: every method here is called on the
// main thread and every callback (done) must be invoked on the main
// thread, exactly once, either before the method returns or later.

// ---------------------------------------------------------------------------
// Callbacks from the backend (implemented by the core's Host)

// WebHost is implemented by the core's Host in addition to Host and HostEx.
type WebHost interface {
	// OnPermissionRequest asks whether the page may use a device or
	// feature. done receives the decision; PermissionPrompt (and
	// PermissionDefault) mean "do what the platform does without an
	// embedder decision", which is asking the user where the platform has
	// a prompt. Backends without a native prompt show their own simple
	// allow/deny question or deny.
	OnPermissionRequest(id SurfaceID, req PermissionRequest, done func(PermissionDecision))

	// ShouldNavigate is asked before every main-frame navigation the
	// platform lets the embedder decide on (links, form submissions,
	// location changes, redirects; on most platforms also Surface.Navigate
	// and reloads — the core always allows the navigations it started
	// itself). Returning false cancels it; the backend then emits neither
	// SurfaceNavigating nor SurfaceLoadFailed for it.
	ShouldNavigate(id SurfaceID, nav NavigationRequest) bool

	// OnNewWindow reports a request to open a new window (window.open,
	// target=_blank links and forms). The backend always suppresses the
	// platform's own popup (the page's window.open returns null); the core
	// decides what to do with the URL: open it in the system browser, in a
	// new surface, in this surface, or nothing.
	OnNewWindow(id SurfaceID, req NewWindowRequest)

	// OnAuthChallenge reports an HTTP authentication challenge (Basic,
	// Digest, NTLM, Negotiate — not TLS client certificates or server
	// trust, which keep the platform default). done receives the answer.
	OnAuthChallenge(id SurfaceID, ch AuthChallenge, done func(AuthResponse))

	// OnDownloadStarted asks where to save a download the page started
	// (a download attribute, Content-Disposition: attachment, or a
	// response the WebView cannot show). done receives an absolute file
	// path, or "" to cancel the download. The backend creates missing
	// parent directories only if the platform does; the core passes paths
	// whose directory exists. Existing files at path are replaced.
	OnDownloadStarted(id SurfaceID, d DownloadRequest, done func(path string))

	// OnDownloadEvent reports progress of a download accepted with a
	// non-empty path. Every accepted download ends with exactly one event
	// whose State is DownloadDone, DownloadFailed or DownloadCanceled.
	// Progress events are throttled by the backend (at most a few per
	// second) and are optional where the platform does not report bytes.
	OnDownloadEvent(id SurfaceID, ev DownloadEvent)
}

// WebPermission names a feature a page asks for. Backends report the
// names below; anything else is reported as WebPermOther.
type WebPermission string

const (
	WebPermCamera        WebPermission = "camera"
	WebPermMicrophone    WebPermission = "microphone"
	WebPermGeolocation   WebPermission = "geolocation"
	WebPermNotifications WebPermission = "notifications"
	WebPermClipboardRead WebPermission = "clipboard-read"
	WebPermMIDI          WebPermission = "midi"            // Web MIDI with system exclusive messages
	WebPermScreen        WebPermission = "screen"          // display capture (getDisplayMedia)
	WebPermMotion        WebPermission = "device-motion"   // device orientation and motion (iOS)
	WebPermStorage       WebPermission = "storage-access"  // Storage Access API
	WebPermMedia         WebPermission = "protected-media" // EME / protected media identifier
	WebPermOther         WebPermission = "other"
)

// PermissionRequest is one page permission request. A request may name
// several kinds at once (camera and microphone for one getUserMedia call);
// the decision covers all of them.
type PermissionRequest struct {
	Origin string // scheme://host[:port] of the requesting frame; "" if unknown
	Kinds  []WebPermission
}

// PermissionDecision answers a PermissionRequest.
type PermissionDecision int

const (
	PermissionDefault PermissionDecision = iota // no decision: the platform default (same as Prompt)
	PermissionPrompt                            // ask the user (platform prompt, or the backend's own)
	PermissionAllow
	PermissionDeny
)

// NavigationRequest is a main-frame navigation about to start.
type NavigationRequest struct {
	URL           string
	UserInitiated bool // a user gesture caused it, where the platform tells
}

// NewWindowRequest is a request of the page to open a new window.
type NewWindowRequest struct {
	URL           string // absolute; "" for window.open() without URL
	UserInitiated bool   // a user gesture caused it, where the platform tells
	// Width and Height are the window features the page asked for in
	// logical pixels, 0 when not given or not reported.
	Width, Height int
}

// AuthChallenge is an HTTP authentication challenge.
type AuthChallenge struct {
	URL    string // the request that was challenged, where the platform tells
	Host   string
	Port   int
	Realm  string
	Scheme string // "basic", "digest", "ntlm", "negotiate" or "" when unknown
	Proxy  bool   // a proxy asked
	// Retry is the number of failed attempts before this one, where the
	// platform tells (0 otherwise).
	Retry int
}

// AuthAction is what to do with an AuthChallenge.
type AuthAction int

const (
	AuthDefault     AuthAction = iota // platform default (usually: fail with 401, or its own dialog)
	AuthCredentials                   // answer with User / Password
	AuthCancel                        // cancel the request
)

type AuthResponse struct {
	Action   AuthAction
	User     string
	Password string
}

// DownloadRequest describes a download asking for a destination.
type DownloadRequest struct {
	ID       uint64 // allocated by the backend, unique for the app's run, never 0
	URL      string
	Filename string // suggested file name (no directory); may be ""
	MIME     string // may be ""
	Size     int64  // expected size in bytes, -1 when unknown
}

type DownloadState int

const (
	DownloadProgress DownloadState = iota + 1
	DownloadDone
	DownloadFailed
	DownloadCanceled
)

// DownloadEvent reports progress or the end of an accepted download.
type DownloadEvent struct {
	ID       uint64
	State    DownloadState
	Received int64  // bytes written so far
	Total    int64  // expected total, -1 when unknown
	Path     string // destination
	Error    string // DownloadFailed
}

// ---------------------------------------------------------------------------
// Creation options

// WebOptions are WebView settings that must be in place before the first
// request. The core passes them to backends implementing WebCreator; for
// others it applies them right after creation through WebUserAgent /
// WebHeaders, which may miss the first request.
type WebOptions struct {
	UserAgent string      // "" keeps the platform's
	Headers   http.Header // extra headers for every request; see WebHeaders
}

// WebCreator is implemented by backends that take WebOptions at creation.
// ex is as for CreatorEx.
type WebCreator interface {
	NewSurfaceWeb(id SurfaceID, opts SurfaceOptions, ex SurfaceExtras, web WebOptions) (Surface, error)
}

// ---------------------------------------------------------------------------
// Optional Surface controls

// FindOptions configures a find-in-page search.
type FindOptions struct {
	MatchCase bool
	Backward  bool // search towards the start of the page
	NoWrap    bool // stop at the end (start) instead of wrapping around
}

// FindResult reports a find-in-page search.
type FindResult struct {
	// Matches is the number of matches in the page, -1 when the platform
	// does not count them.
	Matches int
	// Active is the 1-based index of the highlighted match, 0 when nothing
	// is highlighted or the platform does not tell.
	Active int
	// Found reports whether a match is highlighted.
	Found bool
}

// WebFinder is find in page. Find starts a new search (and highlights the
// first match); FindNext moves to the next (backward: previous) match of
// the current search; StopFind clears the highlight. done of FindNext
// without a current search receives FindResult{} and no error.
type WebFinder interface {
	Find(text string, opts FindOptions, done func(FindResult, error))
	FindNext(backward bool, done func(FindResult, error))
	StopFind()
}

// WebCookies manages the cookies of the surface's website data store.
// Surfaces of one app share one store on every platform, so changes are
// seen by all of them.
//
// Cookies are net/http cookies: Expires zero means a session cookie;
// SameSite is mapped where the platform has it. Domain without a leading
// dot is a host-only cookie where the platform distinguishes.
type WebCookies interface {
	// Cookies lists the cookies that would be sent to url, or all cookies
	// when url is "". Backends that cannot enumerate every cookie report
	// ErrUnsupported for "". A backend may return more cookies than match
	// url; the core filters.
	Cookies(url string, done func([]*http.Cookie, error))
	// SetCookie adds or replaces a cookie. Domain must be set.
	SetCookie(c *http.Cookie, done func(error))
	// DeleteCookie deletes the cookie matching c's Name, Domain and Path.
	DeleteCookie(c *http.Cookie, done func(error))
	// ClearCookies deletes every cookie whose domain is domain or a
	// subdomain of it; "" deletes all cookies.
	ClearCookies(domain string, done func(error))
}

// BrowsingData selects kinds of website data.
type BrowsingData uint32

const (
	DataCookies BrowsingData = 1 << iota
	DataCache                // memory and disk cache
	DataStorage              // localStorage, sessionStorage, IndexedDB, service workers, WebSQL, cache storage

	DataAll = DataCookies | DataCache | DataStorage
)

// WebData clears website data of the surface's store (all sites).
type WebData interface {
	ClearBrowsingData(kinds BrowsingData, done func(error))
}

// WebUserAgent changes the User-Agent of later requests; "" restores the
// platform default.
type WebUserAgent interface {
	SetUserAgent(ua string)
}

// WebHeaders adds headers to every request the WebView makes (navigations
// and subresources, every origin). nil or empty removes them. Requests to
// the asset origin get the core's own extra headers on every platform
// (see the root package), so only backends that can change every request
// implement this.
type WebHeaders interface {
	SetExtraHeaders(h http.Header)
}

// PDFOptions configures PrintToPDF. Fields a platform cannot honour are
// ignored.
type PDFOptions struct {
	Landscape    bool
	NoBackground bool    // omit background colours and images
	PageWidth    float64 // inches; 0 = platform default (A4 or Letter)
	PageHeight   float64 // inches; 0 = platform default
}

// WebPDF renders the current page as a PDF document.
type WebPDF interface {
	PrintToPDF(opts PDFOptions, done func(pdf []byte, err error))
}

// WebSnapshot captures the visible part of the page as a PNG image.
type WebSnapshot interface {
	Screenshot(done func(png []byte, err error))
}

// WebDownloads cancels an accepted download (unknown ids are ignored);
// the backend then reports DownloadCanceled.
type WebDownloads interface {
	CancelDownload(id uint64)
}
