package windows

// Pure logic of the advanced WebView capabilities (webview_ext_windows.go):
// mapping WebView2 enums to the contract, parsing authentication
// challenges and download names, cookie helpers and find-in-page
// bookkeeping. Builds and tests on every OS.

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// COREWEBVIEW2_PERMISSION_KIND (WebView2.h).
const (
	wvPermUnknown          = 0
	wvPermMicrophone       = 1
	wvPermCamera           = 2
	wvPermGeolocation      = 3
	wvPermNotifications    = 4
	wvPermOtherSensors     = 5
	wvPermClipboardRead    = 6
	wvPermMultiDownloads   = 7
	wvPermFileReadWrite    = 8
	wvPermAutoplay         = 9
	wvPermLocalFonts       = 10
	wvPermMIDISysex        = 11
	wvPermWindowManagement = 12
	wvPermPersistStorage   = 13
)

// COREWEBVIEW2_PERMISSION_STATE.
const (
	wvPermStateDefault = 0
	wvPermStateAllow   = 1
	wvPermStateDeny    = 2
)

// webPermission maps a COREWEBVIEW2_PERMISSION_KIND to the contract's name.
func webPermission(kind int32) platform.WebPermission {
	switch kind {
	case wvPermMicrophone:
		return platform.WebPermMicrophone
	case wvPermCamera:
		return platform.WebPermCamera
	case wvPermGeolocation:
		return platform.WebPermGeolocation
	case wvPermNotifications:
		return platform.WebPermNotifications
	case wvPermOtherSensors: // generic sensors, device orientation and motion
		return platform.WebPermMotion
	case wvPermClipboardRead:
		return platform.WebPermClipboardRead
	case wvPermMIDISysex:
		return platform.WebPermMIDI
	}
	return platform.WebPermOther
}

// permissionState maps a decision to COREWEBVIEW2_PERMISSION_STATE.
// Prompt and Default are both the runtime's default, which shows its own
// permission prompt.
func permissionState(d platform.PermissionDecision) int32 {
	switch d {
	case platform.PermissionAllow:
		return wvPermStateAllow
	case platform.PermissionDeny:
		return wvPermStateDeny
	}
	return wvPermStateDefault
}

// requestOrigin is the origin of uri, or "" when it has none.
func requestOrigin(uri string) string {
	if o := originOf(uri); o != "null" {
		return o
	}
	return ""
}

// newWindowURL turns the Uri of NewWindowRequested into the contract's
// URL: window.open() without a URL is reported as about:blank.
func newWindowURL(uri string) string {
	if strings.EqualFold(uri, "about:blank") {
		return ""
	}
	return uri
}

// parseAuthChallenge splits the first challenge of a WWW-Authenticate
// value ("Basic realm=\"x\", charset=\"UTF-8\"") into its lower-case
// scheme and realm.
func parseAuthChallenge(challenge string) (scheme, realm string) {
	s := strings.TrimSpace(challenge)
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return strings.ToLower(s), ""
	}
	scheme, s = strings.ToLower(s[:i]), s[i+1:]
	for s != "" {
		s = strings.TrimLeft(s, " \t,")
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		name := strings.ToLower(strings.TrimSpace(s[:eq]))
		s = strings.TrimLeft(s[eq+1:], " \t")
		var value string
		if strings.HasPrefix(s, `"`) {
			var b strings.Builder
			j := 1
			for ; j < len(s) && s[j] != '"'; j++ {
				if s[j] == '\\' && j+1 < len(s) {
					j++
				}
				b.WriteByte(s[j])
			}
			value = b.String()
			if j < len(s) {
				j++
			}
			s = s[j:]
		} else {
			end := strings.IndexByte(s, ',')
			if end < 0 {
				end = len(s)
			}
			value, s = strings.TrimSpace(s[:end]), s[end:]
		}
		if name == "realm" {
			return scheme, value
		}
		if strings.Contains(name, " ") { // the next challenge started
			break
		}
	}
	return scheme, ""
}

// authChallenge builds the contract's challenge from the request URI and
// the challenge string of BasicAuthenticationRequested.
func authChallenge(uri, challenge string) platform.AuthChallenge {
	ch := platform.AuthChallenge{URL: uri}
	ch.Scheme, ch.Realm = parseAuthChallenge(challenge)
	switch ch.Scheme {
	case "basic", "digest", "ntlm", "negotiate":
	case "":
		ch.Scheme = "basic" // the event is raised for Basic authentication
	default:
		ch.Scheme = ""
	}
	if u, err := url.Parse(uri); err == nil {
		ch.Host = u.Hostname()
		if p, err := strconv.Atoi(u.Port()); err == nil {
			ch.Port = p
		} else {
			switch strings.ToLower(u.Scheme) {
			case "https", "wss":
				ch.Port = 443
			case "http", "ws":
				ch.Port = 80
			}
		}
	}
	return ch
}

// COREWEBVIEW2_BROWSING_DATA_KINDS.
const (
	wvDataAllDOMStorage = 32 // file systems, IndexedDB, local storage, WebSQL, service workers, cache storage
	wvDataCookies       = 64
	wvDataDiskCache     = 256
)

func browsingDataKinds(k platform.BrowsingData) uint32 {
	var v uint32
	if k&platform.DataCookies != 0 {
		v |= wvDataCookies
	}
	if k&platform.DataCache != 0 {
		v |= wvDataDiskCache
	}
	if k&platform.DataStorage != 0 {
		v |= wvDataAllDOMStorage
	}
	return v
}

// COREWEBVIEW2_DOWNLOAD_STATE and the interrupt reasons used.
const (
	wvDownloadInProgress  = 0
	wvDownloadInterrupted = 1
	wvDownloadCompleted   = 2

	wvInterruptUserCanceled = 26
	wvInterruptUserShutdown = 27
	wvInterruptUserPaused   = 28
)

// downloadState classifies a DownloadOperation state change: ok is false
// while the download goes on (also when paused).
func downloadState(state, reason int32) (platform.DownloadState, bool) {
	switch state {
	case wvDownloadCompleted:
		return platform.DownloadDone, true
	case wvDownloadInterrupted:
		switch reason {
		case wvInterruptUserPaused:
			return platform.DownloadProgress, false
		case wvInterruptUserCanceled, wvInterruptUserShutdown:
			return platform.DownloadCanceled, true
		}
		return platform.DownloadFailed, true
	}
	return platform.DownloadProgress, false
}

var interruptNames = map[int32]string{
	1: "file failed", 2: "file access denied", 3: "no space", 4: "file name too long",
	5: "file too large", 6: "malicious file", 7: "transient file error", 8: "blocked by policy",
	9: "security check failed", 10: "file too short", 11: "hash mismatch", 12: "network failed",
	13: "network timeout", 14: "network disconnected", 15: "server down", 16: "invalid request",
	17: "server failed", 18: "server does not support range", 19: "bad content", 20: "unauthorized",
	21: "certificate problem", 22: "forbidden", 23: "unexpected response", 24: "content length mismatch",
	25: "cross-origin redirect", 29: "download process crashed",
}

// downloadError describes a COREWEBVIEW2_DOWNLOAD_INTERRUPT_REASON.
func downloadError(reason int32) string {
	if s, ok := interruptNames[reason]; ok {
		return "download interrupted: " + s
	}
	return "download interrupted (reason " + strconv.Itoa(int(reason)) + ")"
}

// baseName is the last element of a Windows or slash-separated path.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// downloadFilename picks the suggested file name of a download: the
// Content-Disposition file name, else the name of WebView2's default
// result path, else the last URL path element.
func downloadFilename(contentDisposition, resultPath, uri string) string {
	if contentDisposition != "" {
		if _, params, err := mime.ParseMediaType(contentDisposition); err == nil {
			if n := baseName(params["filename"]); n != "" && n != "." && n != ".." {
				return n
			}
		}
	}
	if n := baseName(resultPath); n != "" {
		return n
	}
	if u, err := url.Parse(uri); err == nil {
		if n, err := url.PathUnescape(baseName(u.Path)); err == nil {
			return baseName(n)
		}
	}
	return ""
}

// progressInterval throttles download progress events.
const progressInterval = 250 * time.Millisecond

// COREWEBVIEW2_COOKIE_SAME_SITE_KIND.
const (
	wvSameSiteNone   = 0
	wvSameSiteLax    = 1
	wvSameSiteStrict = 2
)

func sameSiteFromWV(v int32) http.SameSite {
	switch v {
	case wvSameSiteNone:
		return http.SameSiteNoneMode
	case wvSameSiteLax:
		return http.SameSiteLaxMode
	case wvSameSiteStrict:
		return http.SameSiteStrictMode
	}
	return http.SameSiteDefaultMode
}

// sameSiteToWV maps a SameSite mode; ok is false for the default, which
// keeps the runtime's (Lax).
func sameSiteToWV(s http.SameSite) (int32, bool) {
	switch s {
	case http.SameSiteNoneMode:
		return wvSameSiteNone, true
	case http.SameSiteLaxMode:
		return wvSameSiteLax, true
	case http.SameSiteStrictMode:
		return wvSameSiteStrict, true
	}
	return 0, false
}

// cookieExpiry resolves Expires and MaxAge: zero expires is a session
// cookie; del means the cookie is to be deleted (MaxAge < 0).
func cookieExpiry(c *http.Cookie, now time.Time) (expires time.Time, del bool) {
	switch {
	case c.MaxAge < 0:
		return time.Time{}, true
	case c.MaxAge > 0:
		return now.Add(time.Duration(c.MaxAge) * time.Second), false
	}
	return c.Expires, false
}

// unixSeconds converts a time to WebView2's / CDP's cookie expiry
// (seconds since the epoch).
func unixSeconds(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }

func fromUnixSeconds(f float64) time.Time {
	sec := int64(f)
	return time.Unix(sec, int64((f-float64(sec))*1e9))
}

// cookieInDomain reports whether a cookie of cookieDomain belongs to
// domain or one of its subdomains.
func cookieInDomain(cookieDomain, domain string) bool {
	d := strings.ToLower(strings.TrimPrefix(cookieDomain, "."))
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))
	return d == domain || strings.HasSuffix(d, "."+domain)
}

// cdpSetCookieParams returns the parameters of the DevTools method
// Network.setCookie for c (used where a double argument cannot be passed
// to ICoreWebView2Cookie::put_Expires). A domain without a leading dot is
// a host-only cookie, which CDP sets through a URL.
func cdpSetCookieParams(c *http.Cookie, expires time.Time) string {
	path := c.Path
	if path == "" {
		path = "/"
	}
	p := map[string]any{
		"name":     c.Name,
		"value":    c.Value,
		"path":     path,
		"secure":   c.Secure,
		"httpOnly": c.HttpOnly,
	}
	if strings.HasPrefix(c.Domain, ".") {
		p["domain"] = c.Domain
	} else {
		scheme := "http"
		if c.Secure {
			scheme = "https"
		}
		p["url"] = scheme + "://" + c.Domain + path
	}
	switch c.SameSite {
	case http.SameSiteNoneMode:
		p["sameSite"] = "None"
	case http.SameSiteLaxMode:
		p["sameSite"] = "Lax"
	case http.SameSiteStrictMode:
		p["sameSite"] = "Strict"
	}
	if !expires.IsZero() {
		p["expires"] = unixSeconds(expires)
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// isAssetURI reports whether uri is on the asset origin.
func isAssetURI(uri string) bool {
	if len(uri) < len(assetOrigin) || !strings.EqualFold(uri[:len(assetOrigin)], assetOrigin) {
		return false
	}
	rest := uri[len(assetOrigin):]
	return rest == "" || rest[0] == '/' || rest[0] == '?' || rest[0] == '#'
}

// headerValues flattens extra headers to the name/value pairs set on each
// request; empty names are dropped.
func headerValues(h http.Header) [][2]string {
	var out [][2]string
	for name, vs := range h {
		if strings.TrimSpace(name) == "" || len(vs) == 0 {
			continue
		}
		v := strings.Join(vs, ", ")
		v = strings.NewReplacer("\r", " ", "\n", " ").Replace(v)
		out = append(out, [2]string{http.CanonicalHeaderKey(name), v})
	}
	return out
}

// findResult builds the result of a find session from WebView2's match
// count and 1-based active match index (-1: none).
func findResult(count, active int) platform.FindResult {
	if count < 0 {
		count = 0
	}
	if active < 1 || active > count {
		active = 0
	}
	if active == 0 && count > 0 {
		active = 1 // Start highlights the first match
	}
	return platform.FindResult{Matches: count, Active: active, Found: active > 0}
}

// nextFindIndex is the active match after moving forward or backward
// (WebView2 always wraps); move is false when noWrap stops at the end.
func nextFindIndex(count, active int, backward, noWrap bool) (next int, move bool) {
	if count <= 0 {
		return 0, false
	}
	if active < 1 || active > count {
		if backward {
			return count, true
		}
		return 1, true
	}
	if backward {
		if active == 1 {
			if noWrap {
				return active, false
			}
			return count, true
		}
		return active - 1, true
	}
	if active == count {
		if noWrap {
			return active, false
		}
		return 1, true
	}
	return active + 1, true
}
