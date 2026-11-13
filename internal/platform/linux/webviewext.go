package linux

// Pure logic of the WebView extensions (webview_ext_linux.go): mapping
// WebKitGTK enums and libsoup cookie fields to the platform contract
// (webview_ext.go). No native calls, so it is unit tested on any OS.

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// permissionKinds maps the GType name of a WebKitPermissionRequest to the
// contract's permission names. audio, video and display are the
// webkit_user_media_permission_is_for_*_device answers (user media only).
func permissionKinds(typeName string, audio, video, display bool) []platform.WebPermission {
	switch typeName {
	case "WebKitUserMediaPermissionRequest":
		var k []platform.WebPermission
		if display {
			k = append(k, platform.WebPermScreen)
		}
		if video && !display {
			k = append(k, platform.WebPermCamera)
		}
		if audio {
			k = append(k, platform.WebPermMicrophone)
		}
		if len(k) == 0 {
			k = append(k, platform.WebPermOther)
		}
		return k
	case "WebKitGeolocationPermissionRequest":
		return []platform.WebPermission{platform.WebPermGeolocation}
	case "WebKitNotificationPermissionRequest":
		return []platform.WebPermission{platform.WebPermNotifications}
	case "WebKitClipboardPermissionRequest":
		return []platform.WebPermission{platform.WebPermClipboardRead}
	case "WebKitMediaKeySystemPermissionRequest":
		return []platform.WebPermission{platform.WebPermMedia}
	case "WebKitWebsiteDataAccessPermissionRequest":
		return []platform.WebPermission{platform.WebPermStorage}
	}
	// WebKitDeviceInfoPermissionRequest (device labels for
	// enumerateDevices), WebKitPointerLockPermissionRequest,
	// WebKitInstallMissingMediaPluginsPermissionRequest, future types.
	return []platform.WebPermission{platform.WebPermOther}
}

var permissionLabels = map[platform.WebPermission]string{
	platform.WebPermCamera:        "your camera",
	platform.WebPermMicrophone:    "your microphone",
	platform.WebPermGeolocation:   "your location",
	platform.WebPermNotifications: "show notifications",
	platform.WebPermClipboardRead: "read the clipboard",
	platform.WebPermMIDI:          "MIDI devices",
	platform.WebPermScreen:        "capture your screen",
	platform.WebPermMotion:        "motion sensors",
	platform.WebPermStorage:       "its cookies and data on other sites",
	platform.WebPermMedia:         "protected media playback",
}

// permissionPrompt is the question of the backend's own permission dialog
// (WebKitGTK has no built-in prompt).
func permissionPrompt(origin string, kinds []platform.WebPermission) string {
	who := origin
	if who == "" {
		who = "This page"
	}
	var parts []string
	for _, k := range kinds {
		if l, ok := permissionLabels[k]; ok {
			parts = append(parts, l)
		}
	}
	what := "a restricted feature"
	switch len(parts) {
	case 0:
	case 1:
		what = parts[0]
	default:
		what = strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
	return who + " wants to use " + what + "."
}

// WebKitAuthenticationScheme values.
const (
	webkitAuthDefault       = 1
	webkitAuthBasic         = 2
	webkitAuthDigest        = 3
	webkitAuthHTMLForm      = 4
	webkitAuthNTLM          = 5
	webkitAuthNegotiate     = 6
	webkitAuthClientCert    = 7
	webkitAuthServerTrust   = 8
	webkitAuthClientCertPIN = 9
)

// authSchemeName maps a WebKitAuthenticationScheme to the contract's
// scheme name. ok is false for TLS client certificates and server trust,
// which keep the platform default.
func authSchemeName(scheme int32) (name string, ok bool) {
	switch scheme {
	case webkitAuthBasic:
		return "basic", true
	case webkitAuthDigest:
		return "digest", true
	case webkitAuthNTLM:
		return "ntlm", true
	case webkitAuthNegotiate:
		return "negotiate", true
	case webkitAuthClientCert, webkitAuthServerTrust, webkitAuthClientCertPIN:
		return "", false
	}
	return "", true // DEFAULT, HTML_FORM, UNKNOWN
}

// shouldDownload decides whether a response becomes a download: a MIME
// type the web view cannot show, or a main-frame response with
// Content-Disposition: attachment.
func shouldDownload(mimeSupported bool, disposition string, mainFrame bool) bool {
	if !mimeSupported {
		return true
	}
	if !mainFrame {
		return false
	}
	d := strings.ToLower(strings.TrimSpace(disposition))
	return d == "attachment" || strings.HasPrefix(d, "attachment;") || strings.HasPrefix(d, "attachment ")
}

// WebKitWebsiteDataTypes flags.
const (
	wkDataMemoryCache    = 1 << 0
	wkDataDiskCache      = 1 << 1
	wkDataOfflineAppCach = 1 << 2
	wkDataSessionStorage = 1 << 3
	wkDataLocalStorage   = 1 << 4
	wkDataWebSQL         = 1 << 5
	wkDataIndexedDB      = 1 << 6
	wkDataCookies        = 1 << 8
	wkDataServiceWorkers = 1 << 12
	wkDataDOMCache       = 1 << 13
)

// websiteDataTypes maps BrowsingData to WebKitWebsiteDataTypes.
func websiteDataTypes(k platform.BrowsingData) uint32 {
	var t uint32
	if k&platform.DataCookies != 0 {
		t |= wkDataCookies
	}
	if k&platform.DataCache != 0 {
		t |= wkDataMemoryCache | wkDataDiskCache
	}
	if k&platform.DataStorage != 0 {
		t |= wkDataOfflineAppCach | wkDataSessionStorage | wkDataLocalStorage | wkDataWebSQL |
			wkDataIndexedDB | wkDataServiceWorkers | wkDataDOMCache
	}
	return t
}

// WebKitFindOptions flags.
const (
	wkFindCaseInsensitive = 1 << 0
	wkFindBackwards       = 1 << 3
	wkFindWrapAround      = 1 << 4

	findMaxMatches = 1000
)

func findOptions(o platform.FindOptions) uint32 {
	var f uint32
	if !o.MatchCase {
		f |= wkFindCaseInsensitive
	}
	if o.Backward {
		f |= wkFindBackwards
	}
	if !o.NoWrap {
		f |= wkFindWrapAround
	}
	return f
}

// findMatches converts the match_count of "found-text": G_MAXUINT means
// more than the maximum asked for (not counted).
func findMatches(n uint32) int {
	if n == ^uint32(0) || n > findMaxMatches {
		return -1
	}
	return int(n)
}

// SoupSameSitePolicy (libsoup 3).
const (
	soupSameSiteNone   = 0
	soupSameSiteLax    = 1
	soupSameSiteStrict = 2
)

// cookieFields is a SoupCookie as plain values.
type cookieFields struct {
	Name, Value, Domain, Path string
	Session                   bool  // no expiry
	Expires                   int64 // Unix seconds, when !Session
	Secure, HTTPOnly          bool
	SameSite                  int // SoupSameSitePolicy, -1: leave libsoup's default
}

// cookieToFields converts an http.Cookie for soup_cookie_new. Domain
// passes through unchanged: a leading dot is a domain cookie, none a
// host-only one, in both libsoup and the contract. MaxAge wins over
// Expires as in HTTP; MaxAge < 0 expires the cookie now.
func cookieToFields(c *http.Cookie, now time.Time) cookieFields {
	f := cookieFields{
		Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path,
		Secure: c.Secure, HTTPOnly: c.HttpOnly, SameSite: -1, Session: true,
	}
	if f.Path == "" {
		f.Path = "/"
	}
	switch {
	case c.MaxAge > 0:
		f.Session, f.Expires = false, now.Add(time.Duration(c.MaxAge)*time.Second).Unix()
	case c.MaxAge < 0:
		f.Session, f.Expires = false, now.Add(-24*time.Hour).Unix()
	case !c.Expires.IsZero():
		f.Session, f.Expires = false, c.Expires.Unix()
	}
	switch c.SameSite {
	case http.SameSiteNoneMode:
		f.SameSite = soupSameSiteNone
	case http.SameSiteLaxMode:
		f.SameSite = soupSameSiteLax
	case http.SameSiteStrictMode:
		f.SameSite = soupSameSiteStrict
	}
	return f
}

// httpCookie converts a SoupCookie read back from WebKit.
func (f cookieFields) httpCookie() *http.Cookie {
	c := &http.Cookie{
		Name: f.Name, Value: f.Value, Domain: f.Domain, Path: f.Path,
		Secure: f.Secure, HttpOnly: f.HTTPOnly,
	}
	if !f.Session {
		c.Expires = time.Unix(f.Expires, 0).UTC()
	}
	switch f.SameSite {
	case soupSameSiteNone:
		c.SameSite = http.SameSiteNoneMode
	case soupSameSiteLax:
		c.SameSite = http.SameSiteLaxMode
	case soupSameSiteStrict:
		c.SameSite = http.SameSiteStrictMode
	}
	return c
}

// cookieDomainMatch reports whether a cookie of cookieDomain belongs to
// domain or a subdomain of it ("" matches everything).
func cookieDomainMatch(cookieDomain, domain string) bool {
	d := strings.ToLower(strings.TrimLeft(domain, "."))
	if d == "" {
		return true
	}
	cd := strings.ToLower(strings.TrimLeft(cookieDomain, "."))
	return cd == d || strings.HasSuffix(cd, "."+d)
}

// sameCookie reports whether f is the cookie named by c (Name, Domain and
// Path; an empty Path means "/"). Domains compare case-insensitively and
// keep the host-only / domain distinction of the leading dot.
func sameCookie(f cookieFields, c *http.Cookie) bool {
	p := c.Path
	if p == "" {
		p = "/"
	}
	return f.Name == c.Name && strings.EqualFold(f.Domain, c.Domain) && f.Path == p
}

// cookieLookupURL is a URL whose cookies include the cookie c names, for
// webkit_cookie_manager_get_cookies (https so Secure cookies are listed).
func cookieLookupURL(c *http.Cookie) string {
	p := c.Path
	if p == "" {
		p = "/"
	}
	u := url.URL{Scheme: "https", Host: strings.TrimLeft(c.Domain, "."), Path: p}
	return u.String()
}

// pdfPaper returns a custom paper size in inches, ok false for the
// default (both dimensions must be given).
func pdfPaper(o platform.PDFOptions) (w, h float64, ok bool) {
	if o.PageWidth > 0 && o.PageHeight > 0 {
		return o.PageWidth, o.PageHeight, true
	}
	return 0, 0, false
}

// downloadSize converts a Content-Length (0: unknown in WebKit) to the
// contract's size (-1 unknown).
func downloadSize(n uint64) int64 {
	if n == 0 {
		return -1
	}
	return int64(n)
}

// WebKitDownloadError codes.
const wkDownloadErrorCancelled = 400

// downloadFailState maps a WebKitDownload failure.
func downloadFailState(code int32, isDownloadDomain bool) platform.DownloadState {
	if isDownloadDomain && code == wkDownloadErrorCancelled {
		return platform.DownloadCanceled
	}
	return platform.DownloadFailed
}

// progressInterval throttles DownloadProgress events.
const progressInterval = 250 * time.Millisecond
