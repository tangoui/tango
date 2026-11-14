package tango

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/ipc"
)

var _ ipc.BinaryConn = (*Surface)(nil)

type laneHelloJSON struct {
	Gen  uint64   `json:"gen"`
	Tok  string   `json:"tok"`
	List []string `json:"list"`
}

// laneApp runs an app on a fresh fake backend with opts.
func laneApp(t *testing.T, opts Options) (*App, *fake.Backend) {
	t.Helper()
	fb := fake.New()
	a := newWithBackend(opts, fb)
	runApp(t, a)
	return a, fb
}

// laneSurface creates a loaded surface at u whose backend offers caps.
func laneSurface(t *testing.T, a *App, fb *fake.Backend, u string, caps ...string) *Surface {
	t.Helper()
	s, err := a.NewSurface(SurfaceOptions{URL: u})
	if err != nil {
		t.Fatal(err)
	}
	fb.SetLaneCaps(s.ID(), caps)
	fb.Load(s.ID())
	return s
}

func laneCtl(op, name string, gen uint64) string {
	return corehook.InternalPrefix + "lane:" + fmt.Sprintf(`{"op":%q,"lane":%q,"gen":%d}`, op, name, gen)
}

func TestSurfaceLaneHelloConditions(t *testing.T) {
	caps := []string{lane.DownAndroid, lane.UpAndroid}
	noBin := func(*Surface, uint64, []byte) {}
	cases := []struct {
		name  string
		opts  Options
		env   string
		bin   bool
		url   string
		caps  []string
		after func(fb *fake.Backend, s *Surface)
		ok    bool
	}{
		{name: "all", bin: true, caps: caps, ok: true},
		{name: "no OnBinary", caps: caps},
		{name: "dev server page", bin: true, url: "http://localhost:5173/", caps: caps},
		{name: "navigated away", bin: true, caps: caps, after: func(fb *fake.Backend, s *Surface) {
			fb.LoadURL(s.ID(), "https://example.com/")
		}},
		{name: "hello from another origin", bin: true, caps: caps, after: func(fb *fake.Backend, s *Surface) {
			s.Navigate("http://localhost:5173/")
			fb.MessageFrom(s.ID(), "http://localhost:5173", "x")
		}},
		{name: "DisableFastLanes", opts: Options{DisableFastLanes: true}, bin: true, caps: caps},
		{name: "TANGO_IPC_LANES=0", env: "0", bin: true, caps: caps},
		{name: "TANGO_IPC_LANES=1", env: "1", bin: true, caps: caps, ok: true},
		{name: "no caps", bin: true},
		{name: "upper-case origin", bin: true, caps: caps, ok: true, after: func(fb *fake.Backend, s *Surface) {
			fb.MessageFrom(s.ID(), "TANGO://APP/", "x")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.env != "" {
				t.Setenv("TANGO_IPC_LANES", c.env)
			}
			a, fb := laneApp(t, c.opts)
			if c.bin {
				a.OnBinary(noBin)
			}
			s := laneSurface(t, a, fb, c.url, c.caps...)
			if c.after != nil {
				c.after(fb, s)
			}
			raw := s.LaneHello()
			if !c.ok {
				if raw != nil {
					t.Fatalf("LaneHello = %s, want nil", raw)
				}
				if s.BinaryMinSize() != -1 || s.SendBinary([]byte{0, 0}) {
					t.Fatal("lanes usable without a lane hello")
				}
				return
			}
			var h laneHelloJSON
			if err := json.Unmarshal(raw, &h); err != nil {
				t.Fatalf("LaneHello %s: %v", raw, err)
			}
			if h.Gen != 1 || len(h.Tok) != 32 || strings.Join(h.List, ",") != strings.Join(caps, ",") {
				t.Fatalf("LaneHello = %s", raw)
			}
			want := fmt.Sprintf(`{"gen":1,"tok":%q,"list":["down:android","up:android"]}`, h.Tok)
			if string(raw) != want {
				t.Fatalf("LaneHello = %s, want %s", raw, want)
			}
		})
	}
}

func TestLaneControlChannel(t *testing.T) {
	a, fb := laneApp(t, Options{})
	a.OnBinary(func(*Surface, uint64, []byte) {})
	var mu sync.Mutex
	var msgs []string
	a.OnMessage(func(_ *Surface, m string) {
		mu.Lock()
		msgs = append(msgs, m)
		mu.Unlock()
	})
	s := laneSurface(t, a, fb, "", lane.DownAndroid)
	var h laneHelloJSON
	if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
		t.Fatal(err)
	}
	if s.BinaryMinSize() != -1 {
		t.Fatal("lane ready before the page said so")
	}
	fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h.Gen))
	if s.BinaryMinSize() != 0 {
		t.Fatalf("BinaryMinSize = %d, want 0", s.BinaryMinSize())
	}
	p := []byte{0, 1, 9}
	if !s.SendBinary(p) {
		t.Fatal("SendBinary false")
	}
	if got := fb.LanePackets(s.ID()); len(got) != 1 || string(got[0]) != string(p) {
		t.Fatalf("LanePackets = %v", got)
	}
	if gens := fb.LanePacketGens(s.ID()); len(gens) != 1 || gens[0] != h.Gen {
		t.Fatalf("LanePacketGens = %v, want [%d]", gens, h.Gen)
	}
	// An untrusted page's control message is ignored.
	fb.MessageFrom(s.ID(), "https://evil.example", laneCtl("fail", lane.DownAndroid, h.Gen))
	if s.BinaryMinSize() != 0 {
		t.Fatal("untrusted page failed the lane")
	}
	fb.Message(s.ID(), laneCtl("fail", lane.DownAndroid, h.Gen))
	if s.BinaryMinSize() != -1 || s.SendBinary(p) {
		t.Fatal("lane still used after fail")
	}
	fb.Message(s.ID(), "plain")
	mu.Lock()
	defer mu.Unlock()
	if len(msgs) != 1 || msgs[0] != "plain" {
		t.Fatalf("OnMessage got %q, want only the plain message", msgs)
	}
}

func TestOnLanePacketRouting(t *testing.T) {
	a, fb := laneApp(t, Options{})
	type got struct {
		s *Surface
		p string
	}
	ch := make(chan got, 8)
	a.OnBinary(func(s *Surface, gen uint64, p []byte) { ch <- got{s, string(p)} })
	a.OnBinary(func(s *Surface, gen uint64, p []byte) { ch <- got{s, "second:" + string(p)} })
	s := laneSurface(t, a, fb, "", lane.DownScheme, lane.UpAndroid)
	var h laneHelloJSON
	if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
		t.Fatal(err)
	}
	fb.Message(s.ID(), laneCtl("ready", lane.UpAndroid, h.Gen))

	fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, []byte("pkt"))
	for _, want := range []string{"pkt", "second:pkt"} {
		select {
		case g := <-ch:
			if g.s != s || g.p != want {
				t.Fatalf("OnBinary got (%v, %q), want (%v, %q)", g.s, g.p, s, want)
			}
		default:
			t.Fatalf("OnBinary not called synchronously for %q", want)
		}
	}
	fb.LaneUp(s.ID(), "https://evil.example", h.Gen, []byte("bad"))
	fb.LaneUp(s.ID()+100, fb.AssetOrigin(), h.Gen, []byte("nobody"))
	// After the up lane failed its packets are dropped.
	fb.Message(s.ID(), laneCtl("fail", lane.UpAndroid, h.Gen))
	fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, []byte("late"))
	select {
	case g := <-ch:
		t.Fatalf("unexpected OnBinary call with %q", g.p)
	default:
	}
}

func TestOnLanePacketBeforeHello(t *testing.T) {
	a, fb := laneApp(t, Options{})
	called := false
	a.OnBinary(func(*Surface, uint64, []byte) { called = true })
	s := laneSurface(t, a, fb, "", lane.UpAndroid)
	fb.LaneUp(s.ID(), fb.AssetOrigin(), 0, []byte("early"))
	if called {
		t.Fatal("lane packet delivered before any lane hello")
	}
}

// OnLanePacket reports whether it handed the packet to OnBinary, so a
// backend can tell the page its upstream lane refused it.
func TestOnLanePacketReportsRefusal(t *testing.T) {
	a, fb := laneApp(t, Options{})
	type got struct {
		gen uint64
		p   string
	}
	var mu sync.Mutex
	var calls []got
	a.OnBinary(func(_ *Surface, gen uint64, p []byte) {
		mu.Lock()
		calls = append(calls, got{gen, string(p)})
		mu.Unlock()
	})
	s := laneSurface(t, a, fb, "", lane.UpAndroid)
	var h1, h2 laneHelloJSON
	if err := json.Unmarshal(s.LaneHello(), &h1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(s.LaneHello(), &h2); err != nil {
		t.Fatal(err)
	}
	if !fb.LaneUp(s.ID(), fb.AssetOrigin(), h2.Gen, []byte("cur")) {
		t.Fatal("current generation's packet reported refused")
	}
	if fb.LaneUp(s.ID(), fb.AssetOrigin(), h1.Gen, []byte("old")) {
		t.Fatal("old generation's packet reported delivered")
	}
	if fb.LaneUp(s.ID(), "https://evil.example", h2.Gen, []byte("evil")) {
		t.Fatal("packet from outside the asset origin reported delivered")
	}
	if fb.LaneUp(s.ID()+100, fb.AssetOrigin(), h2.Gen, []byte("nobody")) {
		t.Fatal("packet for an unknown surface reported delivered")
	}
	s.lane.Reset()
	if fb.LaneUp(s.ID(), fb.AssetOrigin(), h2.Gen, []byte("reset")) {
		t.Fatal("packet after Reset reported delivered")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || calls[0] != (got{h2.Gen, "cur"}) {
		t.Fatalf("OnBinary got %+v, want only (%d, cur)", calls, h2.Gen)
	}
}

func TestLaneClosedSurface(t *testing.T) {
	a, fb := laneApp(t, Options{})
	a.OnBinary(func(*Surface, uint64, []byte) {})
	s := laneSurface(t, a, fb, "", lane.DownAndroid)
	var h laneHelloJSON
	json.Unmarshal(s.LaneHello(), &h)
	fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h.Gen))
	s.Close()
	if s.SendBinary([]byte{0, 0}) || s.BinaryMinSize() != -1 || s.LaneHello() != nil {
		t.Fatal("lanes usable on a closed surface")
	}
}

// ipcReplies decodes the ipc messages the fake surface received through
// Eval (window.__tango._recv(...)).
func ipcReplies(t *testing.T, fb *fake.Backend, s *Surface) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range fb.Surface(s.ID()).Evals() {
		lit, ok := strings.CutPrefix(e, "window.__tango._recv(")
		if !ok {
			continue
		}
		var msg string
		if err := json.Unmarshal([]byte(strings.TrimSuffix(lit, ")")), &msg); err != nil {
			t.Fatalf("eval %q: %v", e, err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(msg), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

// routerLaneApp runs an app wired to an ipc.Router like an application
// would, with a stream "W" that writes what the test sends on the returned
// channel ([]byte via WriteBytes, anything else via Write), and a loaded
// surface that offered caps, said hello and reported down ready.
func routerLaneApp(t *testing.T, down string, caps ...string) (*fake.Backend, *Surface, chan<- any) {
	t.Helper()
	a, fb := laneApp(t, Options{})
	r := ipc.NewRouter()
	a.OnMessage(func(s *Surface, m string) { r.HandleMessage(s, m) })
	a.OnBinary(func(s *Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })
	writes := make(chan any)
	r.HandleStream("W", func(_ context.Context, _ json.RawMessage, w ipc.StreamWriter) error {
		s := w.(*ipc.Stream)
		for v := range writes {
			var err error
			if p, ok := v.([]byte); ok {
				err = s.WriteBytes(p)
			} else {
				err = s.Write(v)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	s := laneSurface(t, a, fb, "", caps...)
	fb.Message(s.ID(), fmt.Sprintf(`{"tango":"hello","proto":%d}`, ipc.ProtoVersion))
	var lanes map[string]any
	waitUntil(t, "hello reply", func() bool {
		for _, m := range ipcReplies(t, fb, s) {
			if m["tango"] == "hello" {
				lanes, _ = m["lanes"].(map[string]any)
				return true
			}
		}
		return false
	})
	if lanes == nil || lanes["gen"].(float64) < 1 || len(lanes["tok"].(string)) != 32 {
		t.Fatalf("hello reply lanes = %v", lanes)
	}
	fb.Message(s.ID(), laneCtl("ready", down, uint64(lanes["gen"].(float64))))
	fb.Message(s.ID(), `{"tango":"stream","id":1,"op":"open","method":"W","args":[]}`)
	return fb, s, writes
}

// streamData returns the data messages of stream 1 by seq, once the
// stream's close (seq closeSeq) has been sent.
func streamData(t *testing.T, fb *fake.Backend, s *Surface, closeSeq int) map[int]map[string]any {
	t.Helper()
	out := map[int]map[string]any{}
	waitUntil(t, "stream close", func() bool {
		clear(out)
		closed := false
		for _, m := range ipcReplies(t, fb, s) {
			switch m["op"] {
			case "data":
				out[int(m["seq"].(float64))] = m
			case "close":
				closed = m["seq"] == float64(closeSeq)
			}
		}
		return closed
	})
	return out
}

func pattern(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i % 251)
	}
	return p
}

func expectLanePacket(t *testing.T, p []byte, seq uint64, want []byte) {
	t.Helper()
	fs, err := ipc.ParsePacket(p)
	if err != nil || len(fs) != 1 || fs[0].Stream != 1 || fs[0].Seq != seq || string(fs[0].Payload) != string(want) {
		t.Fatalf("lane packet (want seq %d, %d bytes): %v %+v", seq, len(want), err, fs)
	}
}

// down:android takes every byte frame (BinaryMinSize 0); JSON frames stay
// on the message channel.
func TestRouterOverSurfaceLanes(t *testing.T) {
	fb, s, writes := routerLaneApp(t, lane.DownAndroid, lane.DownAndroid, lane.UpAndroid)
	small, big := pattern(100), pattern(64<<10)
	writes <- "hi"  // seq 0: JSON, message channel
	writes <- small // seq 1: lane
	writes <- big   // seq 2: lane
	close(writes)
	data := streamData(t, fb, s, 3)
	if len(data) != 1 || data[0] == nil || data[0]["data"] != "hi" {
		t.Fatalf("message channel data = %v, want only the JSON frame", data)
	}
	ps := fb.LanePackets(s.ID())
	if len(ps) != 2 {
		t.Fatalf("%d lane packets, want 2", len(ps))
	}
	expectLanePacket(t, ps[0], 1, small)
	expectLanePacket(t, ps[1], 2, big)
}

// down:scheme (BinaryMinSize 4096): a 100 B frame goes as a message, a
// 64 KiB one on the lane (queued for the scheme response, so nothing
// reaches the platform's SendLanePacket).
func TestRouterOverSurfaceSchemeLane(t *testing.T) {
	fb, s, writes := routerLaneApp(t, lane.DownScheme, lane.DownScheme, lane.UpPost)
	if s.BinaryMinSize() != lane.SchemeMinSize {
		t.Fatalf("BinaryMinSize = %d", s.BinaryMinSize())
	}
	writes <- pattern(100)      // seq 0: message channel
	writes <- pattern(64 << 10) // seq 1: down:scheme queue
	close(writes)
	data := streamData(t, fb, s, 2)
	if len(data) != 1 || data[0] == nil || data[0]["bin"] == nil {
		t.Fatalf("message channel data = %v, want only the 100 B frame", data)
	}
	if ps := fb.LanePackets(s.ID()); len(ps) != 0 {
		t.Fatalf("down:scheme packet sent through the platform lane")
	}
	// The 64 KiB frame is waiting for the scheme response, as a one-frame
	// packet: 2 + 20 header bytes and the payload.
	if n, b := s.lane.Queued(); n != 1 || b != 2+20+64<<10 {
		t.Fatalf("down:scheme queue holds %d packets, %d bytes", n, b)
	}
}

// A page outside the asset origin never gets the previous page's lanes:
// no hello comes from it that the core would answer, so the generation
// ends when the surface goes there.
func TestLanesOffAfterNavigateAway(t *testing.T) {
	for _, c := range []struct {
		name string
		away func(fb *fake.Backend, s *Surface)
	}{
		{"page followed a link", func(fb *fake.Backend, s *Surface) { fb.LoadURL(s.ID(), "https://evil.example/") }},
		{"core navigated", func(fb *fake.Backend, s *Surface) { s.Navigate("http://localhost:5173/") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, fb := laneApp(t, Options{})
			var mu sync.Mutex
			var got []string
			a.OnBinary(func(_ *Surface, _ uint64, p []byte) { mu.Lock(); got = append(got, string(p)); mu.Unlock() })
			s := laneSurface(t, a, fb, "", lane.DownAndroid, lane.UpAndroid)
			var h laneHelloJSON
			if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
				t.Fatal(err)
			}
			fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h.Gen))
			fb.Message(s.ID(), laneCtl("ready", lane.UpAndroid, h.Gen))
			if !s.SendBinary([]byte{0, 0}) {
				t.Fatal("lane not usable before navigating away")
			}
			fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, []byte("before"))

			c.away(fb, s)
			if s.SendBinary([]byte{0, 1}) || s.BinaryMinSize() != -1 {
				t.Fatal("down lane still usable after navigating away")
			}
			if n := len(fb.LanePackets(s.ID())); n != 1 {
				t.Fatalf("%d lane packets, want only the one before navigating", n)
			}
			fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, []byte("after"))
			// A late control message cannot revive the old generation.
			fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h.Gen))
			if s.BinaryMinSize() != -1 {
				t.Fatal("old generation revived")
			}
			mu.Lock()
			defer mu.Unlock()
			if len(got) != 1 || got[0] != "before" {
				t.Fatalf("OnBinary got %q, want only the packet before navigating", got)
			}
		})
	}
}

// When the page starts navigating (a link click: the next page's script
// may run before its load event), downstream lanes stop until the next
// hello; the uplink keeps working (the navigation may leave the page in
// place, a download say).
func TestLanesDownSuspendedOnNavigating(t *testing.T) {
	a, fb := laneApp(t, Options{})
	var mu sync.Mutex
	var got []string
	a.OnBinary(func(_ *Surface, _ uint64, p []byte) { mu.Lock(); got = append(got, string(p)); mu.Unlock() })
	s := laneSurface(t, a, fb, "", lane.DownAndroid, lane.UpAndroid)
	var h laneHelloJSON
	if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
		t.Fatal(err)
	}
	fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h.Gen))
	fb.Message(s.ID(), laneCtl("ready", lane.UpAndroid, h.Gen))
	if !s.SendBinary([]byte{0, 0}) {
		t.Fatal("lane not usable before navigating")
	}

	fb.Navigating(s.ID(), "https://evil.example/")
	if s.SendBinary([]byte{0, 1}) || s.BinaryMinSize() != -1 {
		t.Fatal("down lane usable while the page navigates")
	}
	if n := len(fb.LanePackets(s.ID())); n != 1 {
		t.Fatalf("%d lane packets, want only the one before navigating", n)
	}
	fb.LaneUp(s.ID(), fb.AssetOrigin(), h.Gen, []byte("upload"))
	mu.Lock()
	if len(got) != 1 || got[0] != "upload" {
		mu.Unlock()
		t.Fatalf("OnBinary got %q, want the upload delivered", got)
	}
	mu.Unlock()
	// A late ready for the suspended generation does not lift it.
	fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h.Gen))
	if s.SendBinary([]byte{0, 2}) || s.BinaryMinSize() != -1 {
		t.Fatal("late ready re-enabled the down lane")
	}

	// Same-origin reload: the new page's hello starts a new generation.
	h2 := laneHelloJSON{}
	if err := json.Unmarshal(s.LaneHello(), &h2); err != nil {
		t.Fatal(err)
	}
	if h2.Gen != h.Gen+1 {
		t.Fatalf("gen %d after reload, want %d", h2.Gen, h.Gen+1)
	}
	fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h2.Gen))
	if !s.SendBinary([]byte{0, 3}) || s.BinaryMinSize() != 0 {
		t.Fatal("down lane not usable in the new generation")
	}
	if n := len(fb.LanePackets(s.ID())); n != 2 {
		t.Fatalf("%d lane packets, want 2", n)
	}
}

// Loading an asset-origin page keeps the generation: its hello may have
// come before the load event.
func TestLanesKeptOnAssetLoad(t *testing.T) {
	a, fb := laneApp(t, Options{})
	a.OnBinary(func(*Surface, uint64, []byte) {})
	s := laneSurface(t, a, fb, "", lane.DownAndroid)
	var h laneHelloJSON
	if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
		t.Fatal(err)
	}
	fb.Message(s.ID(), laneCtl("ready", lane.DownAndroid, h.Gen))
	fb.LoadURL(s.ID(), "TANGO://APP/other.html")
	if s.BinaryMinSize() != 0 {
		t.Fatal("asset-origin load ended the lane generation")
	}
}

func TestServeAssetInterceptsLanePaths(t *testing.T) {
	assetHits := 0
	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { assetHits++ })
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"no Assets", Options{}},
		{"Assets", Options{Assets: assets}},
		{"AllowCrossOriginAssets", Options{Assets: assets, AllowCrossOriginAssets: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, fb := laneApp(t, tc.opts)
			var mu sync.Mutex
			var got []string
			a.OnBinary(func(_ *Surface, _ uint64, p []byte) { mu.Lock(); got = append(got, string(p)); mu.Unlock() })
			s := laneSurface(t, a, fb, "", lane.DownScheme, lane.UpPost)
			var h laneHelloJSON
			if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
				t.Fatal(err)
			}
			origin := fb.AssetOrigin()
			q := fmt.Sprintf("?gen=%d&tok=%s", h.Gen, h.Tok)
			serve := func(id platform.SurfaceID, method, path, from string, body string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, origin+path, strings.NewReader(body))
				if from != "" {
					r.Header.Set("Origin", from)
				}
				w := httptest.NewRecorder()
				host{a}.ServeAsset(id, w, r)
				return w
			}
			before := assetHits

			if w := serve(s.ID(), "POST", "/__tango/lane/up"+q, origin, "pkt"); w.Code != http.StatusNoContent {
				t.Fatalf("POST up: %d %s", w.Code, w.Body)
			}
			if w := serve(s.ID(), "POST", "/__tango/lane/up"+q, "", "pkt2"); w.Code != http.StatusNoContent {
				t.Fatalf("POST up without Origin: %d %s", w.Code, w.Body)
			}
			mu.Lock()
			if len(got) != 2 || got[0] != "pkt" || got[1] != "pkt2" {
				t.Fatalf("OnBinary got %q", got)
			}
			mu.Unlock()
			// Cross-origin requests are refused whatever the options say,
			// GETs of the down lane included.
			for _, m := range []string{"POST", "GET"} {
				path := map[string]string{"POST": "/__tango/lane/up", "GET": "/__tango/lane/down"}[m]
				for _, from := range []string{"http://evil", "null", origin + ".evil"} {
					if w := serve(s.ID(), m, path+q, from, "bad"); w.Code != http.StatusForbidden {
						t.Errorf("%s %s from %s: %d, want 403", m, path, from, w.Code)
					}
				}
			}
			// Bad token, unknown surface, unknown lane path.
			if w := serve(s.ID(), "POST", "/__tango/lane/up?gen=1&tok=00", origin, "bad"); w.Code != http.StatusForbidden {
				t.Errorf("bad token: %d, want 403", w.Code)
			}
			if w := serve(s.ID()+100, "POST", "/__tango/lane/up"+q, origin, "bad"); w.Code != http.StatusNotFound {
				t.Errorf("unknown surface: %d, want 404", w.Code)
			}
			if w := serve(s.ID(), "GET", "/__tango/lane/other"+q, "", ""); w.Code != http.StatusNotFound {
				t.Errorf("unknown lane path: %d, want 404", w.Code)
			}
			mu.Lock()
			if len(got) != 2 {
				t.Fatalf("refused requests delivered: %q", got)
			}
			mu.Unlock()
			if assetHits != before {
				t.Fatal("a lane request reached Options.Assets")
			}

			// The down lane streams through ServeAsset too.
			s.lane.Control(fmt.Sprintf(`{"op":"ready","lane":%q,"gen":%d}`, lane.DownScheme, h.Gen))
			ctx, cancel := context.WithCancel(context.Background())
			r := httptest.NewRequestWithContext(ctx, "GET", origin+"/__tango/lane/down"+q, nil)
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); host{a}.ServeAsset(s.ID(), w, r) }()
			time.Sleep(20 * time.Millisecond)
			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("down response did not end")
			}
			if w.Code != 200 || w.Body.String() != "\x00\x00" {
				t.Fatalf("down: %d %q", w.Code, w.Body.String())
			}

			// A surface with no lane generation serves no lanes.
			s.lane.Reset()
			if w := serve(s.ID(), "POST", "/__tango/lane/up"+q, origin, "late"); w.Code != http.StatusForbidden {
				t.Errorf("after Reset: %d, want 403", w.Code)
			}

			// Other paths still go to Options.Assets.
			w = serve(s.ID(), "GET", "/index.html", "", "")
			if tc.opts.Assets == nil {
				if w.Code != http.StatusNotFound {
					t.Errorf("/index.html without Assets: %d", w.Code)
				}
			} else if assetHits != before+1 {
				t.Error("/index.html did not reach Options.Assets")
			}
		})
	}
}
