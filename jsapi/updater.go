package jsapi

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/service/updater"
)

// Updates lets a page drive the app's updater: check for a release,
// download it and restart into it, and receive the updater's progress
// events. Off by default; it needs both Options.Allow and a call to
// API.InstallUpdater. (The bit is kept clear of the iota list in
// jsapi.go.)
const Updates Cap = 1 << 30

// UpdaterOptions configures API.InstallUpdater.
type UpdaterOptions struct {
	// AllowRestart lets the page install a downloaded update and quit the
	// app into it ("updater.restart"). Without it the page can check and
	// download, and the app restarts when it decides to, applying the
	// payload OnDownloaded receives.
	AllowRestart bool
	// OnDownloaded, if set, receives each verified download.
	OnDownloaded func(*updater.Payload)
}

// updaterState is the runtime API's view of one updater.
type updaterState struct {
	u    *updater.Updater
	opts UpdaterOptions

	mu      sync.Mutex
	busy    bool
	release *updater.Release
	payload *updater.Payload
}

// InstallUpdater exposes u to pages with the Updates capability:
//
//	tango:updater.check    () → release | null   {version, notes, pubDate, format, full, patch}
//	tango:updater.download () → {version, patched}   (after a check found a release)
//	tango:updater.restart  ()                       (UpdaterOptions.AllowRestart)
//	tango:updater.event    event: {kind, release?, patch?, done?, total?, error?}
//
// The updater's OnEvent hook is wrapped so its events also reach every
// page as "tango:updater.event"; a hook set before stays called. Only one
// check or download runs at a time; a second call fails with code "busy".
func (api *API) InstallUpdater(u *updater.Updater, opts UpdaterOptions) {
	st := &updaterState{u: u, opts: opts}
	prev := u.OnEvent
	u.OnEvent = func(e updater.Event) {
		if prev != nil {
			prev(e)
		}
		if api.Allowed(Updates) {
			api.Broadcast("updater.event", e)
		}
	}
	api.Handle(Updates, "updater.check", func(ctx context.Context, _ *tango.Surface, _ json.RawMessage) (any, error) {
		if err := st.begin(); err != nil {
			return nil, err
		}
		defer st.end()
		rel, err := u.Check(ctx)
		if err != nil {
			return nil, err
		}
		st.mu.Lock()
		st.release, st.payload = rel, nil
		st.mu.Unlock()
		if rel == nil {
			return nil, nil
		}
		return rel, nil
	})
	api.Handle(Updates, "updater.download", func(ctx context.Context, _ *tango.Surface, _ json.RawMessage) (any, error) {
		if err := st.begin(); err != nil {
			return nil, err
		}
		defer st.end()
		st.mu.Lock()
		rel := st.release
		st.mu.Unlock()
		if rel == nil {
			return nil, ipc.NewError(ipc.CodeNotFound, "no release to download: call updater.check first")
		}
		p, err := u.Download(ctx, rel, "")
		if err != nil {
			return nil, err
		}
		st.mu.Lock()
		st.payload = p
		st.mu.Unlock()
		if st.opts.OnDownloaded != nil {
			st.opts.OnDownloaded(p)
		}
		return map[string]any{"version": rel.Version, "patched": p.Patched}, nil
	})
	api.Handle(Updates, "updater.restart", noArgs(func() (any, error) {
		if !st.opts.AllowRestart {
			return nil, ipc.NewError(CodeForbidden, "updater.restart is not enabled (jsapi.UpdaterOptions.AllowRestart)")
		}
		st.mu.Lock()
		p := st.payload
		st.mu.Unlock()
		if p == nil {
			return nil, ipc.NewError(ipc.CodeNotFound, "nothing downloaded: call updater.download first")
		}
		if err := u.Apply(p); err != nil {
			return nil, err
		}
		later(api.app.Quit)
		return nil, nil
	}))
}

var errBusy = ipc.NewError("busy", "an update check or download is already running")

func (st *updaterState) begin() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.busy {
		return errBusy
	}
	st.busy = true
	return nil
}

func (st *updaterState) end() {
	st.mu.Lock()
	st.busy = false
	st.mu.Unlock()
}
