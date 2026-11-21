package tango

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/platform"
)

// ErrStaleWebView indicates that a navigation, URL, or applied placement
// changed since the caller acquired a stamp.
var ErrStaleWebView = errors.New("tango: embedded WebView changed")

// ErrWebViewNotReady indicates that the embedded page has not finished
// loading or has no visible placement.
var ErrWebViewNotReady = errors.New("tango: embedded WebView is not ready")

// WebViewStamp identifies a document and applied layout of one embedded
// WebView. Obtain it again when the page navigates or the layout changes.
type WebViewStamp struct {
	SurfaceID              platform.SurfaceID
	NavigationID, LayoutID uint64
	URL                    string
	revision               uint64
}

// WebViewStamp returns the identity of the currently loaded document and
// its applied native placement. It can be called from any goroutine.
// The dev host proxy does not expose stamped operations.
func (s *Surface) WebViewStamp() (WebViewStamp, error) {
	type result struct {
		stamp WebViewStamp
		err   error
	}
	r, ok := await(s.app, func() result {
		stamp, err := s.currentWebStamp()
		return result{stamp, err}
	})
	if !ok {
		return WebViewStamp{}, ErrNotRunning
	}
	return r.stamp, r.err
}

func (s *Surface) currentWebStamp() (WebViewStamp, error) {
	if s.closed.Load() {
		return WebViewStamp{}, ErrSurfaceClosed
	}
	if s.embedIn == nil || s.proxyEmbed {
		return WebViewStamp{}, ErrUnsupported
	}
	page := s.WebNavigation()
	p := s.web.placement
	if page.Kind != WebNavigationFinished || !s.visible || p.Frame.Intersect(p.Clip).Empty() {
		return WebViewStamp{}, ErrWebViewNotReady
	}
	return WebViewStamp{SurfaceID: s.id, NavigationID: page.ID, LayoutID: s.web.layoutID, URL: page.URL, revision: s.web.pageRevision}, nil
}

func (s *Surface) commitWebPlacement(p platform.WebPlacement) {
	if s.web.placement != p {
		s.web.placement = p
		s.web.layoutID++
	}
}

func (s *Surface) checkWebStamp(stamp WebViewStamp) error {
	got, err := s.currentWebStamp()
	if errors.Is(err, ErrWebViewNotReady) || (err == nil && got != stamp) {
		return ErrStaleWebView
	}
	return err
}

func webOperation[C, T any](s *Surface, stamp WebViewStamp, fn func(C, func(T, error))) (T, error) {
	return webAsync(s, func(w C, done func(T, error)) {
		var zero T
		if err := s.checkWebStamp(stamp); err != nil {
			done(zero, err)
			return
		}
		fn(w, func(value T, err error) {
			if stale := s.checkWebStamp(stamp); stale != nil {
				done(zero, stale)
				return
			}
			done(value, err)
		})
	})
}

// EvalWebView evaluates a synchronous JavaScript expression in the main
// frame and returns its JSON value. The stamp must be current when the
// request starts and when the result arrives.
func (s *Surface) EvalWebView(stamp WebViewStamp, expression string) (json.RawMessage, error) {
	return webOperation(s, stamp, func(w platform.WebEvaluator, done func(json.RawMessage, error)) {
		w.EvalWebJSON(expression, func(data []byte, err error) {
			if err == nil && !json.Valid(data) {
				err = errors.New("tango: WebView expression produced no JSON value")
			}
			done(data, err)
		})
	})
}

// CaptureWebView returns a PNG from the WebView's native screenshot API.
// It discards the result if the stamp expires before completion.
func (s *Surface) CaptureWebView(stamp WebViewStamp) ([]byte, error) {
	return webOperation(s, stamp, func(w platform.WebSnapshot, done func([]byte, error)) {
		w.Screenshot(done)
	})
}

// WebInputKind selects a native click, text entry, or scroll.
type WebInputKind = platform.WebInputKind

const (
	WebClick  = platform.WebClick
	WebType   = platform.WebType
	WebScroll = platform.WebScroll
)

// WebInput describes input in the native WebView's local logical pixels,
// from its top-left. The click and scroll point must lie within its
// visible clip. Positive scroll deltas reveal content further down/right.
type WebInput = platform.WebInput

// SendWebInput dispatches native input to an embedded WebView. An error
// after dispatch describes the current state. It cannot undo page effects.
func (s *Surface) SendWebInput(stamp WebViewStamp, in WebInput) error {
	_, err := webOperation(s, stamp, func(w platform.WebInputSink, done func(struct{}, error)) {
		if err := s.checkWebInput(in); err != nil {
			done(struct{}{}, err)
			return
		}
		w.DispatchWebInput(in, func(err error) { done(struct{}{}, err) })
	})
	return err
}

func (s *Surface) checkWebInput(in WebInput) error {
	switch in.Kind {
	case WebClick, WebScroll:
		p := s.web.placement
		clip := p.Frame.Intersect(p.Clip)
		x, y := p.Frame.X+in.X, p.Frame.Y+in.Y
		if !(x >= clip.X && y >= clip.Y && x < clip.X+clip.W && y < clip.Y+clip.H) {
			return errors.New("tango: WebView input point outside visible area")
		}
		if in.Kind == WebScroll && !(math.Abs(in.DeltaX) <= 1e6 && math.Abs(in.DeltaY) <= 1e6) {
			return errors.New("tango: invalid WebView scroll delta")
		}
	case WebType:
		if !utf8.ValidString(in.Text) || strings.ContainsRune(in.Text, 0) {
			return errors.New("tango: invalid WebView text")
		}
	default:
		return errors.New("tango: unknown WebView input kind")
	}
	return nil
}
