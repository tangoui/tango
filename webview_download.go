package tango

// Downloads: choosing and preparing a download's destination and
// delivering download events (platform.WebHost). See webview.go for the
// public API.

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// ---------------------------------------------------------------------------
// Host callbacks: downloads (platform.WebHost)

func (h host) OnDownloadStarted(id platform.SurfaceID, d platform.DownloadRequest, done func(string)) {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	if s == nil || s.closed.Load() {
		done("")
		return
	}
	s.probeAfterDownload()
	s.mu.Lock()
	fn := s.web.onDownloadDestination
	s.mu.Unlock()
	if fn == nil || d.CannotDefer {
		done(a.downloadDest(s, d))
		return
	}
	if s.web.pendingDownloads == nil {
		s.web.pendingDownloads = make(map[uint64]func())
	}
	finish := webReply(s, "", func(path string) {
		if s.web.pendingDownloads[d.ID] == nil {
			done("") // the engine already ended the download
			return
		}
		delete(s.web.pendingDownloads, d.ID)
		dest := a.prepareDownloadDest(path)
		done(dest)
		if dest == "" {
			ev := DownloadEvent{ID: d.ID, State: DownloadCanceled, Total: d.Size}
			if path != "" {
				ev.State, ev.Error = DownloadFailed, "could not prepare the destination"
			}
			h.OnDownloadEvent(id, ev)
		}
	})
	s.web.pendingDownloads[d.ID] = func() { finish("") }
	if !guardCall(a, whereEvent, false, func() bool {
		fn(d, finish)
		return true
	}) {
		finish("")
	}
}

// SetDownloadDestinationHandler lets the caller choose a destination
// after the native callback returns. Reply once with an absolute path,
// a name relative to App.DownloadDir, or an empty string to cancel.
// CancelDownload also cancels a pending choice. A nil handler restores
// the synchronous download policy, which also applies on older engines.
func (s *Surface) SetDownloadDestinationHandler(fn func(DownloadRequest, func(string))) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed.Load() {
		s.web.onDownloadDestination = fn
	}
}

// probeAfterDownload: a main-frame navigation whose response became a
// download leaves the previous page in place, but SurfaceNavigating made
// the core hold messages for a page that will not come. Like after
// SurfaceLoadFailed, ask the current page whether it is alive; if it
// answers, messages flow to it again. Main thread.
func (s *Surface) probeAfterDownload() {
	if s.loaded.Load() || s.ns == nil {
		return
	}
	s.ext.navSeq++
	s.ns.Eval("window.__tango&&window.__tango.post(" +
		jsString(corehook.InternalPrefix+frameChannel+":alive:"+strconv.FormatUint(s.ext.navSeq, 10)) + ")")
}

// downloadDest picks and prepares a download's destination; "" cancels.
func (a *App) downloadDest(s *Surface, d DownloadRequest) string {
	s.mu.Lock()
	sfn := s.web.onDownload
	s.mu.Unlock()
	a.mu.Lock()
	afn := a.web.onDownload
	a.mu.Unlock()
	var p string
	switch {
	case sfn != nil: // a panicking handler ("") cancels the download
		p = guardCall(a, whereEvent, "", func() string { return sfn(d) })
	case afn != nil:
		p = guardCall(a, whereEvent, "", func() string { return afn(s, d) })
	case s.pageTrusted():
		p = a.DownloadPath(d)
	default:
		// A page of another site does not get to write files without
		// asking: no handler, no download.
		return ""
	}
	return a.prepareDownloadDest(p)
}

func (a *App) prepareDownloadDest(p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(a.DownloadDir(), p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return ""
	}
	return p
}

func (h host) DownloadPending(id platform.SurfaceID, download uint64) bool {
	s := h.a.surface(id)
	return s != nil && s.web.pendingDownloads[download] != nil
}

func (h host) OnDownloadEvent(id platform.SurfaceID, ev platform.DownloadEvent) {
	a := h.a
	defer a.enterMain()()
	s := a.surface(id)
	if s != nil {
		if cancel := s.web.pendingDownloads[ev.ID]; cancel != nil && ev.State != DownloadProgress {
			delete(s.web.pendingDownloads, ev.ID)
			cancel()
		}
		fire(s, &s.web.onDownloadEv, func(fn func(DownloadEvent)) { fn(ev) })
	}
	a.mu.Lock()
	fns := slices.Clone(a.web.onDownloadEv)
	a.mu.Unlock()
	for _, fn := range fns {
		a.guard(whereEvent, func() { fn(s, ev) })
	}
}

// downloadName derives a safe file name for a download.
func downloadName(req DownloadRequest) string {
	name := req.Filename
	if name == "" {
		if p, err := url.Parse(req.URL); err == nil {
			name = p.Path
			if i := strings.LastIndexByte(name, '/'); i >= 0 {
				name = name[i+1:]
			}
			if n, err := url.PathUnescape(name); err == nil {
				name = n
			}
		}
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7f, strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(strings.TrimSpace(name), ".")
	if name == "" {
		name = "download"
	}
	return name
}

// uniquePath returns p, or p with " (n)" before the extension, so that no
// file exists there.
func uniquePath(p string) string {
	if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; i < 10000; i++ {
		q := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Lstat(q); errors.Is(err, os.ErrNotExist) {
			return q
		}
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
}
