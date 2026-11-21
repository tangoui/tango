package tango

import (
	"strings"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// LastActivityAt returns the time of the latest input observed on the
// surface, including input in embedded WebViews. It returns the zero time
// before any input and retains its value while hidden or after Close. It is
// safe to call from any goroutine and does not wait for the UI thread.
// Coverage depends on the host: WebView input in a separate browser process
// may only be observable in the main document.
func (s *Surface) LastActivityAt() time.Time {
	if at := s.activity.Load(); at != nil {
		return *at
	}
	return time.Time{}
}

func (s *Surface) noteActivity(at time.Time) {
	for p := s; p != nil && !p.closed.Load(); p = p.embedIn {
		for {
			old := p.activity.Load()
			if old != nil && !at.After(*old) || p.activity.CompareAndSwap(old, &at) {
				break
			}
		}
	}
}

func init() {
	corehook.NoteSurfaceActivity = func(surface any, at time.Time) {
		if s, ok := surface.(*Surface); ok && !at.IsZero() {
			s.noteActivity(at)
		}
	}
}

func (h host) OnSurfaceActivity(id platform.SurfaceID) {
	if s := h.a.surface(id); s != nil {
		s.noteActivity(time.Now())
	}
}

const activityPrefix = "\x00tango-int:surface-activity:"

// The script runs before page code. Keep the token and the native post
// reference private, and avoid sending pointer motion at display rate.
func (a *App) activityScript() string {
	return `
;(function () {
  if (!window.__tangoNative || window !== window.top) return;
  const post = window.__tangoNative.post.bind(window.__tangoNative);
  const now = performance.now.bind(performance);
  let lastMotion = -Infinity;
  for (const type of ['keydown', 'keyup', 'compositionstart', 'compositionupdate', 'compositionend', 'pointerdown', 'pointerup', 'pointermove', 'wheel', 'touchstart', 'touchmove', 'touchend']) {
    window.addEventListener(type, function (e) {
      if (!e.isTrusted) return;
      if (type === 'pointermove' || type === 'touchmove' || type === 'wheel') {
        const t = now();
        if (t - lastMotion < 100) return;
        lastMotion = t;
      }
      post('\u0000tango-int:surface-activity:` + a.activityToken + `');
    }, {capture: true, passive: true});
  }
})();`
}

func (a *App) handleActivity(s *Surface, msg string) bool {
	token, ok := strings.CutPrefix(msg, activityPrefix)
	if ok && token == a.activityToken {
		s.noteActivity(time.Now())
	}
	return ok
}
