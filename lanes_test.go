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

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/downq"
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
	// Items take the poll lane even while it idles; TestSendIdleRule
	// turns the latency rule back on.
	setDownVar(t, &downq.IdleEvalItems, 0)
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
	s := laneSurface(t, a, fb, "", lane.DownPoll, lane.UpAndroid)
	var h laneHelloJSON
	if err := json.Unmarshal(s.LaneHello(), &h); err != nil {
		t.Fatal(err)
	}
	fb.Message(s.ID(), laneCtl("ready", lane.UpAndroid, h.Gen))

	fb.LaneUp(s.ID(), fb.AssetOrigin(), []byte("pkt"))
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
	fb.LaneUp(s.ID(), "https://evil.example", []byte("bad"))
	fb.LaneUp(s.ID()+100, fb.AssetOrigin(), []byte("nobody"))
	// After the up lane failed its packets are dropped.
	fb.Message(s.ID(), laneCtl("fail", lane.UpAndroid, h.Gen))
	fb.LaneUp(s.ID(), fb.AssetOrigin(), []byte("late"))
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
	fb.LaneUp(s.ID(), fb.AssetOrigin(), []byte("early"))
	if called {
		t.Fatal("lane packet delivered before any lane hello")
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
// Eval (window.__tango._recvSeq(...)).
func ipcReplies(t *testing.T, fb *fake.Backend, s *Surface) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, msg := range sentMsgs(t, fb.Surface(s.ID()).Evals()) {
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
	// down:android is not ordered with the message channel: each lane frame
	// is followed by a boundary, so a lost packet is detected.
	var bounds []float64
	for _, m := range ipcReplies(t, fb, s) {
		if m["op"] == "boundary" {
			bounds = append(bounds, m["seq"].(float64))
		}
	}
	if len(bounds) != 2 || bounds[0] != 2 || bounds[1] != 3 {
		t.Fatalf("boundaries %v, want [2 3]", bounds)
	}
}

// down:poll (BinaryMinSize 0) while the page polls: every byte frame, small
// or large, is a packet item of the poll queue (nothing reaches the
// platform's SendLanePacket), and the stream's JSON close follows it in
// the same queue.
func TestRouterOverSurfacePollLane(t *testing.T) {
	fb, s, writes := routerLaneApp(t, lane.DownPoll, lane.DownPoll, lane.UpPost)
	if s.BinaryMinSize() != 0 {
		t.Fatalf("BinaryMinSize = %d", s.BinaryMinSize())
	}
	a := s.app
	o := lastOpen(t, fb.Surface(s.ID()))
	ch := activate(t, a, s, o)
	small, big := pattern(100), pattern(64<<10)
	writes <- small // seq 0: down:poll
	writes <- big   // seq 1: down:poll
	close(writes)
	var pkts [][]byte
	closed := false
	for !closed {
		w := awaitPoll(t, ch)
		items := batchItems(t, w)
		var ack uint64
		for _, it := range items {
			ack = it.Seq
			switch it.Kind {
			case downq.Packet:
				pkts = append(pkts, it.Data)
			case downq.Msg:
				var m map[string]any
				if json.Unmarshal(it.Data, &m) == nil && m["op"] == "close" {
					closed = true
				}
				if m["op"] == "data" {
					t.Fatalf("stream data on the message channel: %s", it.Data)
				}
				if m["op"] == "boundary" { // down:poll is ordered: none needed
					t.Fatalf("boundary on down:poll: %s", it.Data)
				}
			}
		}
		if !closed {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			next := make(chan *httptest.ResponseRecorder, 1)
			go func() { next <- pollReq(ctx, a, s, o.Gen, o.Tok, ack) }()
			ch = next
		}
	}
	if len(pkts) != 2 {
		t.Fatalf("%d packet items, want 2", len(pkts))
	}
	expectLanePacket(t, pkts[0], 0, small)
	expectLanePacket(t, pkts[1], 1, big)
	if ps := fb.LanePackets(s.ID()); len(ps) != 0 {
		t.Fatal("down:poll packet sent through the platform lane")
	}
}

// down:poll ready but the page not polling (yet, or after it failed):
// pollSend refuses, so the byte frames go as messages.
func TestRouterOverSurfacePollLaneNotPolling(t *testing.T) {
	fb, s, writes := routerLaneApp(t, lane.DownPoll, lane.DownPoll, lane.UpPost)
	if s.downq.Active() {
		t.Fatal("poll queue active without a poll")
	}
	writes <- pattern(100)      // seq 0: message channel
	writes <- pattern(64 << 10) // seq 1: message channel
	close(writes)
	data := streamData(t, fb, s, 2)
	if len(data) != 2 || data[0]["bin"] == nil || data[1]["bin"] == nil {
		t.Fatalf("message channel data = %v, want both frames", data)
	}
	if ps := fb.LanePackets(s.ID()); len(ps) != 0 {
		t.Fatal("packet sent through the platform lane")
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
			fb.LaneUp(s.ID(), fb.AssetOrigin(), []byte("before"))

			c.away(fb, s)
			if s.SendBinary([]byte{0, 1}) || s.BinaryMinSize() != -1 {
				t.Fatal("down lane still usable after navigating away")
			}
			if n := len(fb.LanePackets(s.ID())); n != 1 {
				t.Fatalf("%d lane packets, want only the one before navigating", n)
			}
			fb.LaneUp(s.ID(), fb.AssetOrigin(), []byte("after"))
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
	fb.LaneUp(s.ID(), fb.AssetOrigin(), []byte("upload"))
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
			s := laneSurface(t, a, fb, "", lane.DownPoll, lane.UpPost)
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
			// GETs of the poll lane included.
			for _, m := range []string{"POST", "GET"} {
				path := map[string]string{"POST": "/__tango/lane/up", "GET": "/__tango/lane/poll"}[m]
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

			// down:scheme's endpoint is gone: no lane serves it, and it
			// does not fall through to Options.Assets either.
			if w := serve(s.ID(), "GET", "/__tango/lane/down"+q, "", ""); w.Code != http.StatusNotFound {
				t.Errorf("GET down: %d, want 404", w.Code)
			}
			if assetHits != before {
				t.Fatal("the old down path reached Options.Assets")
			}

			// A surface with no lane generation serves no lanes.
			s.lane.Reset()
			if w := serve(s.ID(), "POST", "/__tango/lane/up"+q, origin, "late"); w.Code != http.StatusForbidden {
				t.Errorf("after Reset: %d, want 403", w.Code)
			}

			// Other paths still go to Options.Assets.
			w := serve(s.ID(), "GET", "/index.html", "", "")
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
