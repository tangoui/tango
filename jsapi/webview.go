package jsapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ipc"
)

// Web module: the calling page's WebView — find in page, PDF and PNG
// capture, cookies and website data, downloads, user agent. Every part is
// off by default. (The bits are kept clear of the iota list in jsapi.go.)
//
// Permission requests, HTTP authentication, new windows and the navigation
// policy are decided in Go only (tango.App / Surface handlers); a page
// cannot answer them for itself.
const (
	// WebFind lets a page search itself: find, findNext, findPrevious,
	// stopFind.
	WebFind Cap = 1 << (20 + iota)
	// WebCapture lets a page render itself as PDF or PNG (printToPDF,
	// screenshot).
	WebCapture
	// WebCookies lets a page read and change every cookie of the app's
	// WebView store, HttpOnly ones and other sites' included, and clear
	// website data.
	WebCookies
	// WebDownloads lets a page receive the progress of its downloads
	// ("web.download" events) and cancel them.
	WebDownloads
	// WebControl lets a page change its User-Agent.
	WebControl

	// WebAll is every Web capability.
	WebAll = WebFind | WebCapture | WebCookies | WebDownloads | WebControl
)

type findOptsJS struct {
	MatchCase bool `json:"matchCase"`
	Backward  bool `json:"backward"`
	NoWrap    bool `json:"noWrap"`
}

type findResultJS struct {
	Matches int  `json:"matches"`
	Active  int  `json:"active"`
	Found   bool `json:"found"`
}

func toFind(r tango.FindResult) findResultJS { return findResultJS{r.Matches, r.Active, r.Found} }

type pdfOptsJS struct {
	Landscape    bool    `json:"landscape"`
	NoBackground bool    `json:"noBackground"`
	PageWidth    float64 `json:"pageWidth"`
	PageHeight   float64 `json:"pageHeight"`
}

// cookieJS is a cookie on the wire; expires is milliseconds since the
// epoch, null for a session cookie.
type cookieJS struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  *int64 `json:"expires"`
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"httpOnly"`
	SameSite string `json:"sameSite"` // "", "lax", "strict", "none"
}

func toCookieJS(c *http.Cookie) cookieJS {
	j := cookieJS{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HttpOnly}
	if !c.Expires.IsZero() {
		ms := c.Expires.UnixMilli()
		j.Expires = &ms
	}
	switch c.SameSite {
	case http.SameSiteLaxMode:
		j.SameSite = "lax"
	case http.SameSiteStrictMode:
		j.SameSite = "strict"
	case http.SameSiteNoneMode:
		j.SameSite = "none"
	}
	return j
}

func (j cookieJS) cookie() (*http.Cookie, error) {
	c := &http.Cookie{Name: j.Name, Value: j.Value, Domain: j.Domain, Path: j.Path, Secure: j.Secure, HttpOnly: j.HTTPOnly}
	if j.Expires != nil {
		c.Expires = time.UnixMilli(*j.Expires)
	}
	switch strings.ToLower(j.SameSite) {
	case "":
	case "lax":
		c.SameSite = http.SameSiteLaxMode
	case "strict":
		c.SameSite = http.SameSiteStrictMode
	case "none":
		c.SameSite = http.SameSiteNoneMode
	default:
		return nil, ipc.NewError(ipc.CodeBadArgs, "sameSite must be lax, strict or none")
	}
	if c.Name == "" || c.Domain == "" {
		return nil, ipc.NewError(ipc.CodeBadArgs, "a cookie needs a name and a domain")
	}
	return c, nil
}

var dataKinds = map[string]tango.BrowsingData{
	"cookies": tango.DataCookies,
	"cache":   tango.DataCache,
	"storage": tango.DataStorage,
	"all":     tango.DataAll,
}

type downloadJS struct {
	ID       uint64 `json:"id"`
	State    string `json:"state"` // "progress", "done", "failed", "canceled"
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Path     string `json:"path"`
	Error    string `json:"error,omitempty"`
}

var downloadStates = map[tango.DownloadState]string{
	tango.DownloadProgress: "progress",
	tango.DownloadDone:     "done",
	tango.DownloadFailed:   "failed",
	tango.DownloadCanceled: "canceled",
}

// webErr maps WebView control errors to wire errors.
func webErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, tango.ErrSurfaceClosed):
		return ipc.NewError(ipc.CodeNotFound, err.Error())
	}
	return err
}

func (api *API) registerWeb() {
	h := func(c Cap, name string, fn func(s *tango.Surface, args json.RawMessage) (any, error)) {
		api.Handle(c, "web."+name, windowHandler(fn))
	}

	// ---- find ----
	h(WebFind, "find", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var text string
		var o findOptsJS
		if err := decode(args, &text, &o); err != nil {
			return nil, err
		}
		r, err := s.Find(text, tango.FindOptions{MatchCase: o.MatchCase, Backward: o.Backward, NoWrap: o.NoWrap})
		return toFind(r), webErr(err)
	})
	h(WebFind, "findNext", func(s *tango.Surface, _ json.RawMessage) (any, error) {
		r, err := s.FindNext()
		return toFind(r), webErr(err)
	})
	h(WebFind, "findPrevious", func(s *tango.Surface, _ json.RawMessage) (any, error) {
		r, err := s.FindPrevious()
		return toFind(r), webErr(err)
	})
	h(WebFind, "stopFind", func(s *tango.Surface, _ json.RawMessage) (any, error) {
		return nil, webErr(s.StopFind())
	})

	// ---- capture ----
	h(WebCapture, "printToPDF", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var o pdfOptsJS
		if err := decode(args, &o); err != nil {
			return nil, err
		}
		b, err := s.PrintToPDF(tango.PDFOptions{Landscape: o.Landscape, NoBackground: o.NoBackground, PageWidth: o.PageWidth, PageHeight: o.PageHeight})
		if err != nil {
			return nil, webErr(err)
		}
		return base64.StdEncoding.EncodeToString(b), nil
	})
	h(WebCapture, "screenshot", func(s *tango.Surface, _ json.RawMessage) (any, error) {
		b, err := s.Screenshot()
		if err != nil {
			return nil, webErr(err)
		}
		return base64.StdEncoding.EncodeToString(b), nil
	})

	// ---- cookies and data ----
	h(WebCookies, "cookies", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var u string
		if err := decode(args, &u); err != nil {
			return nil, err
		}
		cs, err := s.Cookies(u)
		if err != nil {
			if !errors.Is(err, tango.ErrUnsupported) && !errors.Is(err, tango.ErrSurfaceClosed) {
				return nil, ipc.NewError(ipc.CodeBadArgs, err.Error())
			}
			return nil, webErr(err)
		}
		out := make([]cookieJS, 0, len(cs))
		for _, c := range cs {
			out = append(out, toCookieJS(c))
		}
		return out, nil
	})
	h(WebCookies, "setCookie", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var j cookieJS
		if err := decode(args, &j); err != nil {
			return nil, err
		}
		c, err := j.cookie()
		if err != nil {
			return nil, err
		}
		return nil, webErr(s.SetCookie(c))
	})
	h(WebCookies, "deleteCookie", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var j cookieJS
		if err := decode(args, &j); err != nil {
			return nil, err
		}
		c, err := j.cookie()
		if err != nil {
			return nil, err
		}
		return nil, webErr(s.DeleteCookie(c))
	})
	h(WebCookies, "clearCookies", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var domain string
		if err := decode(args, &domain); err != nil {
			return nil, err
		}
		return nil, webErr(s.ClearCookies(domain))
	})
	h(WebCookies, "clearData", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var kinds []string
		if err := decode(args, &kinds); err != nil {
			return nil, err
		}
		var k tango.BrowsingData
		if kinds == nil {
			k = tango.DataAll
		}
		for _, n := range kinds {
			v, ok := dataKinds[n]
			if !ok {
				return nil, ipc.NewError(ipc.CodeBadArgs, "unknown data kind "+n+" (cookies, cache, storage, all)")
			}
			k |= v
		}
		return nil, webErr(s.ClearBrowsingData(k))
	})

	// ---- downloads ----
	h(WebDownloads, "cancelDownload", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var id uint64
		if err := decode(args, &id); err != nil {
			return nil, err
		}
		return nil, webErr(s.CancelDownload(id))
	})
	api.app.OnDownloadEvent(func(s *tango.Surface, ev tango.DownloadEvent) {
		if s == nil || !api.Allowed(WebDownloads) || api.page(s) == nil {
			return
		}
		api.Emit(s, "web.download", downloadJS{ev.ID, downloadStates[ev.State], ev.Received, ev.Total, ev.Path, ev.Error})
	})

	// ---- control ----
	h(WebControl, "setUserAgent", func(s *tango.Surface, args json.RawMessage) (any, error) {
		var ua string
		if err := decode(args, &ua); err != nil {
			return nil, err
		}
		return nil, webErr(s.SetUserAgent(ua))
	})
}
