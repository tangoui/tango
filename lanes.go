package tango

// IPC fast lanes: *Surface implements ipc.BinaryConn on top of
// internal/lane, the page's lane control messages arrive on the internal
// channel "lane", and upstream packets reach App.OnBinary handlers. See the
// ipc package comment ("Fast lanes") and internal/lane.

import (
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ipc"
)

// laneChannel is the internal channel of lane control messages
// (corehook.InternalPrefix + "lane:" + JSON; lane.State.Control).
const laneChannel = "lane"

// surfaceLanes is the part of a surface's lane wiring read off the main
// thread (ipc's stream writers call SendBinary, BinaryMinSize and
// LaneHello from their goroutines).
type surfaceLanes struct {
	mu sync.Mutex
	bl platform.BinaryLane // the native surface's lanes; nil if none or closed
	// origin is the latest evidence of where the page comes from: the
	// creation URL, the URL of the last SurfaceLoaded, then the origin of
	// each trusted message (the hello itself included). Lowercase, no
	// trailing slash.
	origin string
}

// OnBinary registers fn for upstream fast lane packets from any surface's
// page; it is usually ipc.Router.HandleBinarySession (not HandleBinary,
// which drops every packet of a Surface):
//
//	app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })
//
// Fast lanes are offered to a page only while at least one handler is
// registered (and the page comes from the asset origin, the backend has
// lanes and they are not turned off: Options.DisableFastLanes,
// TANGO_IPC_LANES=0). fn runs on the goroutine that received the packet,
// not on the main thread: it must be safe there and must not block. packet
// is only valid during the call, and may carry no frames (count 0: the
// up:post start probe); a handler that forwards to
// Router.HandleBinarySession need not care. generation is the immutable
// identity captured at authorization; forward it to
// Router.HandleBinarySession, never replace it with the surface current
// generation inside a delayed callback.
func (a *App) OnBinary(fn func(s *Surface, generation uint64, packet []byte)) {
	a.mu.Lock()
	a.onBin = append(a.onBin, fn)
	a.mu.Unlock()
}

func (a *App) binaryHandlers() []func(*Surface, uint64, []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.onBin)
}

// lanesOff reports whether fast lanes are turned off for the app.
func (a *App) lanesOff() bool {
	return a.opts.DisableFastLanes || os.Getenv("TANGO_IPC_LANES") == "0"
}

// isAssetOrigin reports whether origin is the app's asset origin.
func (a *App) isAssetOrigin(origin string) bool {
	ao := a.assetOrigin()
	return ao != "" && normOrigin(origin) == strings.ToLower(ao)
}

func normOrigin(origin string) string {
	return strings.ToLower(strings.TrimSuffix(origin, "/"))
}

// initLanes sets up s's lane state; navOrigin is the creation URL's
// origin. Before the native surface exists.
func (s *Surface) initLanes(navOrigin string) {
	s.lane = lane.New(s.sendLanePacket)
	s.lanes.origin = navOrigin
}

// setLaneNative records ns's lanes (nil: the surface has none or closed).
func (s *Surface) setLaneNative(ns platform.Surface) {
	bl, _ := ns.(platform.BinaryLane)
	s.lanes.mu.Lock()
	s.lanes.bl = bl
	s.lanes.mu.Unlock()
}

// lanesLeaving ends the page's lane generation when the surface is going
// to (Navigate) or has loaded (SurfaceLoaded) a page outside the asset
// origin: a lane must never carry the app's bytes to such a page, and it
// will send no hello the core would answer. Loads of asset-origin pages
// leave the generation alone, since their hello may arrive before the
// load event. Main thread.
func (s *Surface) lanesLeaving(origin string) {
	if !s.app.isAssetOrigin(origin) {
		s.lane.Reset()
	}
}

// noteLaneOrigin records the origin the page was last seen at.
func (s *Surface) noteLaneOrigin(origin string) {
	s.lanes.mu.Lock()
	s.lanes.origin = normOrigin(origin)
	s.lanes.mu.Unlock()
}

func (s *Surface) laneNative() (platform.BinaryLane, string) {
	s.lanes.mu.Lock()
	defer s.lanes.mu.Unlock()
	return s.lanes.bl, s.lanes.origin
}

// sendLanePacket is lane.State's platformSend.
func (s *Surface) sendLanePacket(gen uint64, p []byte) bool {
	bl, _ := s.laneNative()
	return bl != nil && bl.SendLanePacket(gen, p)
}

// LaneHello implements ipc.BinaryConn: it starts a new lane generation for
// the page's hello and returns the hello reply's "lanes" field, or nil when
// the page gets no fast lanes. It does not concern applications; use
// ipc.Router. Safe from any goroutine.
func (s *Surface) LaneHello() json.RawMessage {
	if s.closed.Load() {
		return nil
	}
	a := s.app
	bl, origin := s.laneNative()
	if bl == nil || a.lanesOff() || len(a.binaryHandlers()) == 0 || !a.isAssetOrigin(origin) {
		return s.lane.Hello(nil, false)
	}
	return s.lane.Hello(bl.LaneCaps(), true)
}

// SendBinary implements ipc.BinaryConn: it hands packet to the page's ready
// downstream lane, false when there is none or it refused. Safe from any
// goroutine.
func (s *Surface) SendBinary(packet []byte) bool {
	return !s.closed.Load() && s.lane.Send(packet)
}

// BinaryMinSize implements ipc.BinaryConn: the smallest byte frame worth
// the downstream lane, -1 without one. Safe from any goroutine.
func (s *Surface) BinaryMinSize() int {
	if s.closed.Load() {
		return -1
	}
	return s.lane.MinSize()
}

var _ platform.LaneHost = host{}

// OnLanePacket delivers a packet a backend's upstream lane received to the
// OnBinary handlers, on the calling goroutine (any thread; it does not
// claim the main thread). Lanes are only offered to pages from the asset
// origin, so packets from any other origin are dropped (trusts would also
// accept the origin the core navigated to, but it reads main-thread state).
// gen is the generation the page sent p in; packets of any other
// generation are dropped (lane.State.AllowUpGen). It reports whether p
// reached the handlers.
func (h host) OnLanePacket(id platform.SurfaceID, origin string, gen uint64, p []byte) bool {
	a := h.a
	s := a.surface(id)
	if s == nil || s.closed.Load() || !a.isAssetOrigin(origin) {
		return false
	}
	if !s.lane.AllowUpGen(gen, p) {
		return false
	}
	a.deliverBinary(s, gen, p)
	return true
}

// deliverBinary runs the OnBinary handlers for p from s.
func (a *App) deliverBinary(s *Surface, gen uint64, p []byte) {
	for _, fn := range a.binaryHandlers() {
		a.guard(whereEvent, func() { fn(s, gen, p) })
	}
}

// lanePathPrefix is the asset origin path under which the scheme lanes are
// served (down:scheme, up:post); host.ServeAsset hands these requests to
// serveLane instead of Options.Assets.
const lanePathPrefix = "/__tango/lane/"

// serveLane serves GET /__tango/lane/down and POST /__tango/lane/up for
// surface id (lane.State.ServeDown, ServeUp; upstream packets go to the
// OnBinary handlers). A request whose Origin header names another origin
// is refused (403) whatever its method and whatever
// Options.AllowCrossOriginAssets says: the lanes are the core's, not the
// application's. The token check is ServeDown's and ServeUp's; a surface
// whose page has no lane generation refuses every request. Any thread,
// like ServeAsset.
func (a *App) serveLane(id platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	if !a.laneRequestAllowed(r) {
		http.Error(w, "tango: cross-origin request to a lane refused", http.StatusForbidden)
		return
	}
	s := a.surface(id)
	if s == nil || s.closed.Load() {
		http.NotFound(w, r)
		return
	}
	switch r.URL.Path {
	case lanePathPrefix + "down":
		s.lane.ServeDown(w, r)
	case lanePathPrefix + "up":
		s.lane.ServeUpSession(w, r, func(gen uint64, p []byte) {
			if !s.closed.Load() {
				a.deliverBinary(s, gen, p)
			}
		})
	default:
		http.NotFound(w, r)
	}
}

// laneRequestAllowed is assetRequestAllowed for lane requests, also
// applied to GET: an Origin header, if present, must be the asset origin.
func (a *App) laneRequestAllowed(r *http.Request) bool {
	if !a.assetRequestAllowed(r) {
		return false
	}
	o, ok := r.Header["Origin"]
	if !ok {
		return true
	}
	return len(o) == 1 && o[0] != "" && a.isAssetOrigin(o[0])
}

// BinarySession snapshots the document identity for a Router connState.
func (s *Surface) BinarySession() (uint64, ipc.BinaryConn) {
	gen := s.lane.Generation()
	return gen, &surfaceBinarySession{Surface: s, gen: gen}
}

type surfaceBinarySession struct {
	*Surface
	gen uint64
}

func (s *surfaceBinarySession) SendBinary(p []byte) bool {
	return !s.closed.Load() && s.lane.SendGeneration(s.gen, p)
}
func (s *surfaceBinarySession) BinaryMinSize() int {
	if s.closed.Load() || s.gen != s.lane.Generation() {
		return -1
	}
	return s.lane.MinSize()
}

// DropBinarySession retires the document before Router forgets its streams.
// It only takes State.mu and never invokes application callbacks.
func (s *Surface) DropBinarySession() { s.lane.Reset() }
