package tango

import (
	"path/filepath"
	"slices"

	"github.com/tangoui/tango/internal/platform"
)

// WebDialogKind identifies the script dialog to answer.
type WebDialogKind = platform.WebDialogKind

const (
	WebDialogAlert        = platform.WebDialogAlert
	WebDialogConfirm      = platform.WebDialogConfirm
	WebDialogPrompt       = platform.WebDialogPrompt
	WebDialogBeforeUnload = platform.WebDialogBeforeUnload
)

// WebDialogRequest describes a dialog and its requesting document.
type WebDialogRequest = platform.WebDialogRequest

// WebDialogResponse accepts or dismisses a dialog. Text supplies the
// accepted prompt's value. For beforeunload, Accepted allows navigation.
type WebDialogResponse = platform.WebDialogResponse

// WebFileChooserRequest describes the selection requested by a file input.
// Accept may be empty when the backend cannot read the requested filters.
type WebFileChooserRequest = platform.WebFileChooserRequest

// SetWebDialogHandler intercepts script dialogs. fn runs on the main
// thread and may answer later from any goroutine. Only the first reply
// is used. Closing the surface dismisses pending dialogs. A nil handler
// restores the platform's default behavior.
func (s *Surface) SetWebDialogHandler(fn func(WebDialogRequest, func(WebDialogResponse))) {
	s.mu.Lock()
	if !s.closed.Load() {
		s.web.onDialog = fn
	}
	s.mu.Unlock()
	s.app.Post(func() {
		if s.closed.Load() {
			return
		}
		if w, ok := s.ns.(platform.WebDialogs); ok {
			w.SetWebDialogs((host{s.app}).HasWebDialogHandler(s.id))
		}
	})
}

// SetWebFileChooserHandler intercepts webpage file selection. Reply with
// absolute local paths or nil to dismiss. The callback and reply have the
// same threading and lifetime as SetWebDialogHandler.
func (s *Surface) SetWebFileChooserHandler(fn func(WebFileChooserRequest, func([]string))) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed.Load() {
		s.web.onFileChooser = fn
	}
}

// SetWebPopupHandler lets a surface claim the engine's native popup. The
// handler runs synchronously on the main thread and returns an on-created
// callback or nil to let the legacy NewWindow policy handle the request.
// A popup from an embedded surface can be placed by uihost.WebViewOptions.Adopt.
// A popup from a window receives a new window. The engine retains
// window.opener when the page permits it. Use child.OnClosed to observe
// window.close(), user close, and App.Close.
func (s *Surface) SetWebPopupHandler(fn func(NewWindowRequest) func(child *Surface)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed.Load() {
		s.web.onPopup = fn
	}
}

func (h host) HasWebDialogHandler(id platform.SurfaceID) bool {
	s := h.a.surface(id)
	if s == nil || s.closed.Load() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.web.onDialog != nil
}

func (h host) OnWebDialog(id platform.SurfaceID, req WebDialogRequest, done func(WebDialogResponse)) bool {
	defer h.a.enterMain()()
	s := h.a.surface(id)
	if s == nil || s.closed.Load() {
		return false
	}
	s.mu.Lock()
	fn := s.web.onDialog
	s.mu.Unlock()
	if fn == nil {
		return false
	}
	finish := webReply(s, WebDialogResponse{}, done)
	req.ID = s.web.replySeq
	if !guardCall(h.a, whereEvent, false, func() bool {
		fn(req, finish)
		return true
	}) {
		finish(WebDialogResponse{})
	}
	return true
}

// WebFileChooserEvent reports a file input's request or completion. On
// completion Open is false and FileCount reports how many paths were chosen.
type WebFileChooserEvent struct {
	ID        uint64
	Request   WebFileChooserRequest
	Open      bool
	FileCount int
}

// OnWebFileChooser observes the file selection lifecycle on the main
// thread. With no custom handler Tango opens the platform file picker.
func (s *Surface) OnWebFileChooser(fn func(WebFileChooserEvent)) {
	if fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed.Load() {
		s.web.onFileEvent = append(s.web.onFileEvent, fn)
	}
}

func (h host) OnWebFileChooser(id platform.SurfaceID, req WebFileChooserRequest, done func([]string)) bool {
	defer h.a.enterMain()()
	s := h.a.surface(id)
	if s == nil || s.closed.Load() {
		return false
	}
	s.mu.Lock()
	fn, observed := s.web.onFileChooser, len(s.web.onFileEvent) != 0
	s.mu.Unlock()
	dialogs, nativePicker := h.a.b.(platform.Dialogs)
	if fn == nil && !(observed && nativePicker) {
		return false
	}
	finish := webReply(s, []string(nil), func(paths []string) {
		done(paths)
		ev := WebFileChooserEvent{ID: req.ID, Request: req, FileCount: len(paths)}
		fire(s, &s.web.onFileEvent, func(fn func(WebFileChooserEvent)) { fn(ev) })
	})
	req.ID = s.web.replySeq
	ev := WebFileChooserEvent{ID: req.ID, Request: req, Open: true}
	fire(s, &s.web.onFileEvent, func(fn func(WebFileChooserEvent)) { fn(ev) })
	if s.closed.Load() {
		return true
	}
	reply := func(paths []string) {
		if (!req.Multiple && len(paths) > 1) || slices.ContainsFunc(paths, func(p string) bool { return !filepath.IsAbs(p) }) {
			paths = nil
		}
		finish(slices.Clone(paths))
	}
	if fn != nil {
		if !guardCall(h.a, whereEvent, false, func() bool {
			fn(req, reply)
			return true
		}) {
			reply(nil)
		}
	} else {
		parent := s.id
		if s.embedIn != nil {
			parent = s.embedIn.id
		}
		h.a.Post(func() {
			if !s.closed.Load() {
				dialogs.OpenFileDialog(parent, platform.FileOptions{Multiple: req.Multiple, PickFolders: req.Directories}, func(paths []string, _ error) {
					reply(paths)
				})
			}
		})
	}
	return true
}

func (h host) OnWebPopup(id platform.SurfaceID, req NewWindowRequest, create func(platform.SurfaceID, platform.SurfaceID, platform.SurfaceOptions) (platform.Surface, error)) bool {
	defer h.a.enterMain()()
	s := h.a.surface(id)
	if s == nil || s.closed.Load() {
		return false
	}
	s.mu.Lock()
	fn := s.web.onPopup
	s.mu.Unlock()
	if fn == nil {
		return false
	}
	ready := guardCall(h.a, whereEvent, nil, func() func(*Surface) { return fn(req) })
	if ready == nil || s.closed.Load() {
		return false
	}
	opts := SurfaceOptions{Title: s.Title(), Width: req.Width, Height: req.Height, Resizable: true, Debug: s.web.opts.Debug, Hidden: true, WebView: s.web.opts.WebView}
	if opts.Width <= 0 || opts.Height <= 0 {
		opts.Width, opts.Height = s.Size()
	}
	if s.embedIn != nil {
		opts.Hidden = false
	}
	child, err := h.a.addSurface(opts, "", true, func(child *Surface) (platform.Surface, error) {
		child.web.opts = opts
		child.web.coreNav = req.URL
		child.embedIn = s.embedIn
		var parentID platform.SurfaceID
		if child.embedIn != nil {
			parentID = child.embedIn.id
		}
		return create(child.id, parentID, platform.SurfaceOptions{
			Title: opts.Title, Width: opts.Width, Height: opts.Height,
			Resizable: true, Debug: opts.Debug,
		})
	})
	if err != nil || child.closed.Load() {
		return false
	}
	// The engine first attaches the child as a popup. Showing it and
	// running the application's callback happen outside the delegate.
	h.a.Post(func() {
		if child.closed.Load() {
			return
		}
		h.a.guard(whereEvent, func() { ready(child) })
		if child.embedIn == nil && !child.closed.Load() {
			child.Show()
		}
	})
	return true
}

// webReply ties a deferred native response to the surface. All native
// continuations are released on main, including on close or quit.
func webReply[T any](s *Surface, cancel T, done func(T)) func(T) {
	if s.web.replies == nil {
		s.web.replies = map[uint64]func(){}
	}
	s.web.replySeq++
	id := s.web.replySeq
	s.web.replies[id] = func() { done(cancel) }
	return func(value T) {
		s.app.Post(func() {
			if s.web.replies[id] != nil {
				delete(s.web.replies, id)
				done(value)
			}
		})
	}
}
