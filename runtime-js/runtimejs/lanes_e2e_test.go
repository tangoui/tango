package runtimejs_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/runtime-js/runtimejs"
)

// The fast lane end-to-end test: Go's ipc.Router and internal/lane on one
// side, the real runtime-js (IIFE, up:post module) in node on the other.
// Messages travel over stdio as in TestEndToEndWithNode; up:post requests
// are real HTTP requests to an httptest server serving
// lane.State.ServeUpSession, as the core serves it on the asset origin.
// Sizes and byte patterns must match the lane scenarios in
// testdata/harness.mjs.
//
// Only up:post is offered: down:scheme is gone (down:poll, its successor,
// is the core's and is polled by the page's bridge, not by this harness),
// so every downstream frame goes as a message. The harness's "lanes" mode
// still waits for a down lane in lanesUp (5 s, then reports the fallback)
// and runs laneFailure, for which there is no downstream lane to cut: once
// armed, Go sends lanefail:tripped after the next downstream byte frame (as
// a message) instead of cutting it, and checks that every frame arrives.
const (
	laneGapMs     = 300 // harness.mjs LANE_GAP_MS: the page's gap timeout
	bigTotal      = 8 << 20
	bigFrames     = bigTotal / (64 << 10)
	mixedFrames   = 40
	laneCtlPrefix = corehook.InternalPrefix + "lane:"
)

// laneCounts counts stream byte frames per stream id, by the path they
// took: a lane packet (up: delivered by ServeUpSession to
// HandleBinarySession; down: always 0, there is no downstream lane) or a
// "bin" message on the message channel.
type laneCounts struct {
	upLane, downLane, upMsg, downMsg map[uint64]int
}

func newLaneCounts() laneCounts {
	return laneCounts{map[uint64]int{}, map[uint64]int{}, map[uint64]int{}, map[uint64]int{}}
}

func (c laneCounts) clone() laneCounts {
	cp := func(m map[uint64]int) map[uint64]int {
		out := make(map[uint64]int, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	return laneCounts{cp(c.upLane), cp(c.downLane), cp(c.upMsg), cp(c.downMsg)}
}

// minus returns c - before, per stream.
func (c laneCounts) minus(before laneCounts) laneCounts {
	sub := func(a, b map[uint64]int) map[uint64]int {
		out := map[uint64]int{}
		for k, v := range a {
			if d := v - b[k]; d != 0 {
				out[k] = d
			}
		}
		return out
	}
	return laneCounts{sub(c.upLane, before.upLane), sub(c.downLane, before.downLane),
		sub(c.upMsg, before.upMsg), sub(c.downMsg, before.downMsg)}
}

func total(m map[uint64]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func (c laneCounts) String() string {
	return fmt.Sprintf("upLane=%d upMsg=%d downLane=%d downMsg=%d",
		total(c.upLane), total(c.upMsg), total(c.downLane), total(c.downMsg))
}

// laneProbe records what crossed the lanes and holds the laneFailure hook.
type laneProbe struct {
	mu       sync.Mutex
	n        laneCounts
	requests int      // HTTP requests to the lane endpoints
	ctl      []string // lane control messages from the page
	problems []string // harness-level failures, reported by the test
	armed    bool     // trip after the next downstream byte frame
	trip     func()   // tells the page lanefail:tripped
}

func (p *laneProbe) problem(format string, args ...any) {
	p.mu.Lock()
	p.problems = append(p.problems, fmt.Sprintf(format, args...))
	p.mu.Unlock()
}

func (p *laneProbe) counts() laneCounts {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n.clone()
}

// noteMsg counts a stream "bin" data message (down: Go → JS).
func (p *laneProbe) noteMsg(msg string, down bool) {
	if !strings.HasPrefix(msg, "{") || !strings.Contains(msg, `"bin"`) {
		return
	}
	var m struct {
		Tango string  `json:"tango"`
		ID    uint64  `json:"id"`
		Op    string  `json:"op"`
		Bin   *string `json:"bin"`
	}
	if json.Unmarshal([]byte(msg), &m) != nil || m.Tango != "stream" || m.Op != "data" || m.Bin == nil {
		return
	}
	p.mu.Lock()
	trip := false
	if down {
		p.n.downMsg[m.ID]++
		trip, p.armed = p.armed, false
	} else {
		p.n.upMsg[m.ID]++
	}
	p.mu.Unlock()
	if trip {
		go p.trip()
	}
}

// laneConn is a pipeConn with fast lanes: it implements ipc.BinaryConn by
// delegating to a lane.State offering up:post, as tango.Surface does for a
// page on the asset origin.
type laneConn struct {
	*pipeConn
	st      *lane.State
	enabled bool
	probe   *laneProbe
}

var _ ipc.BinaryConn = (*laneConn)(nil)

func (c *laneConn) Send(msg string) {
	c.probe.noteMsg(msg, true)
	c.pipeConn.Send(msg)
}

func (c *laneConn) LaneHello() json.RawMessage {
	return c.st.Hello([]string{lane.UpPost}, c.enabled)
}
func (c *laneConn) SendBinary(p []byte) bool { return c.st.Send(p) }
func (c *laneConn) BinaryMinSize() int       { return c.st.MinSize() }

// The harness exercises the same immutable-session seam as Surface.
func (c *laneConn) BinarySession() (uint64, ipc.BinaryConn) {
	gen := c.st.Generation()
	return gen, &laneSession{laneConn: c, gen: gen}
}

type laneSession struct {
	*laneConn
	gen uint64
}

func (c *laneSession) SendBinary(p []byte) bool { return c.st.SendGeneration(c.gen, p) }

type laneResult struct {
	data  json.RawMessage
	delta laneCounts // frames per path while the scenario ran
}

func TestLanesEndToEnd(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	script := filepath.Join(t.TempDir(), "tango.iife.js")
	if err := os.WriteFile(script, []byte(runtimejs.Script), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Run("post", func(t *testing.T) { runLanes(t, node, script, true) })
	t.Run("disabled", func(t *testing.T) { runLanes(t, node, script, false) })
}

// runLanes runs the harness's lane scenarios: with enabled, LaneHello
// offers up:post; without, it returns nil (no lanes field).
func runLanes(t *testing.T, node, script string, enabled bool) {
	probe := &laneProbe{n: newLaneCounts()}
	st := lane.New(nil, nil)
	r := ipc.NewRouter()

	mode := "nolanes"
	if enabled {
		mode = "lanes"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/harness.mjs", script, mode) // + server URL below
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	// Built before the server starts, so its handlers see it.
	conn := &laneConn{pipeConn: newPipeConn(stdin), st: st, enabled: enabled, probe: probe}

	mux := http.NewServeMux()
	mux.HandleFunc("/__tango/lane/up", func(w http.ResponseWriter, req *http.Request) {
		probe.mu.Lock()
		probe.requests++
		probe.mu.Unlock()
		st.ServeUpSession(w, req, func(gen uint64, p []byte) {
			if frames, err := ipc.ParsePacket(p); err == nil {
				probe.mu.Lock()
				for _, f := range frames {
					probe.n.upLane[f.Stream]++
				}
				probe.mu.Unlock()
			} else {
				probe.problem("up:post packet: %v", err)
			}
			r.HandleBinarySession(conn, gen, p)
		})
	})
	srv := httptest.NewServer(mux)
	cmd.Args = append(cmd.Args, srv.URL)
	if err := cmd.Start(); err != nil {
		srv.Close()
		t.Fatal(err)
	}
	defer cmd.Wait()
	defer srv.Close()
	defer st.Close()
	defer conn.close() // closes node's stdin: the harness exits

	r.HandleDuplex("Echo", func(s *ipc.Stream, _ json.RawMessage) error {
		for {
			m, err := s.Receive()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if err := s.WriteBytes(m.Bytes); err != nil {
				return err
			}
		}
	})
	// No downstream lane to cut (see the file comment): trip after the
	// next downstream byte frame, A's echo of its frame 2.
	probe.trip = func() { r.Emit(conn, "lanefail:tripped", nil) }
	r.On("lanefail:arm", func(c ipc.Conn, _ json.RawMessage) {
		probe.mu.Lock()
		probe.armed = true
		probe.mu.Unlock()
		r.Emit(c, "lanefail:armed", nil)
	})

	results := map[string]laneResult{}
	var started laneCounts
	lastStarted := ""
	r.On("e2e:start", func(_ ipc.Conn, data json.RawMessage) {
		json.Unmarshal(data, &lastStarted)
		started = probe.counts()
	})
	for _, name := range []string{"lanesUp", "bigBothWays", "mixedSizesOrder", "laneFailure"} {
		r.On(resultPrefix+name, func(_ ipc.Conn, data json.RawMessage) {
			d := probe.counts().minus(started)
			results[name] = laneResult{append(json.RawMessage(nil), data...), d}
			t.Logf("%s: %s; frames by path: %v", name, data, d)
		})
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var msg string
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			t.Fatalf("bad line %q", sc.Text())
		}
		if rest, ok := strings.CutPrefix(msg, laneCtlPrefix); ok {
			// The core's internal channel: never reaches the Router.
			probe.mu.Lock()
			probe.ctl = append(probe.ctl, rest)
			probe.mu.Unlock()
			st.Control(rest)
			continue
		}
		probe.noteMsg(msg, false)
		if r.HandleMessage(conn, msg) {
			continue
		}
		switch {
		case strings.HasPrefix(msg, "FAIL "):
			t.Fatalf("scenario %s: %s", lastStarted, msg)
		case strings.HasPrefix(msg, "DONE "):
			if enabled {
				checkPostLane(t, conn, st, probe, results)
			} else {
				checkLanesDisabled(t, conn, probe, results)
			}
			probe.mu.Lock()
			for _, p := range probe.problems {
				t.Error(p)
			}
			probe.mu.Unlock()
			return
		default:
			t.Errorf("unexpected message %.200q", msg)
		}
	}
	t.Fatalf("node exited early (last scenario started: %q): %v", lastStarted, sc.Err())
}

func decodeResult(t *testing.T, res map[string]laneResult, name string, v any) (laneCounts, bool) {
	t.Helper()
	got, ok := res[name]
	if !ok {
		t.Errorf("%s: no result", name)
		return laneCounts{}, false
	}
	if err := json.Unmarshal(got.data, v); err != nil {
		t.Errorf("%s: %v", name, err)
		return laneCounts{}, false
	}
	return got.delta, true
}

type bigResult struct {
	N      int  `json:"n"`
	Frames int  `json:"frames"`
	OK     bool `json:"ok"`
}

type lanesResult struct {
	Down string `json:"down"`
	Up   string `json:"up"`
}

var hexToken = regexp.MustCompile(`^[0-9a-f]{32}$`)

func checkPostLane(t *testing.T, c *laneConn, st *lane.State, probe *laneProbe, res map[string]laneResult) {
	// Go's hello reply offers up:post.
	hellos := c.helloReplies()
	if len(hellos) != 1 {
		t.Fatalf("Go hello replies = %q, want one", hellos)
	}
	var h struct {
		Lanes *struct {
			Gen  uint64   `json:"gen"`
			Tok  string   `json:"tok"`
			List []string `json:"list"`
		} `json:"lanes"`
	}
	if err := json.Unmarshal([]byte(hellos[0]), &h); err != nil || h.Lanes == nil ||
		!hexToken.MatchString(h.Lanes.Tok) || strings.Join(h.Lanes.List, ",") != "up:post" {
		t.Errorf("hello reply %s: want lanes {gen, 32 hex tok, list [up:post]}", hellos[0])
	}

	// lanesUp: the page reports up:post ready, and told Go so.
	var l lanesResult
	if _, ok := decodeResult(t, res, "lanesUp", &l); ok && (l != lanesResult{"fallback", "up:post"}) {
		t.Errorf("lanesUp: tango.lanes() = %+v, want {fallback up:post}", l)
	}
	probe.mu.Lock()
	ctl := append([]string(nil), probe.ctl...)
	probe.mu.Unlock()
	gen := uint64(0)
	if h.Lanes != nil {
		gen = h.Lanes.Gen
	}
	if want := fmt.Sprintf(`{"op":"ready","lane":"up:post","gen":%d}`, gen); !containsJSON(ctl, want) {
		t.Errorf("lane control messages %q: missing %s", ctl, want)
	}

	// bigBothWays: 8 MiB echoed intact, at least half the frames on up:post,
	// every echo as a message, and every frame counted exactly once.
	var big bigResult
	if d, ok := decodeResult(t, res, "bigBothWays", &big); ok {
		if (big != bigResult{bigTotal, bigFrames, true}) {
			t.Errorf("bigBothWays = %+v, want {%d %d true}", big, bigTotal, bigFrames)
		}
		if total(d.upLane) < bigFrames/2 {
			t.Errorf("bigBothWays: %v, want at least %d frames on up:post", d, bigFrames/2)
		}
		if total(d.upLane)+total(d.upMsg) != bigFrames || total(d.downLane) != 0 || total(d.downMsg) != bigFrames {
			t.Errorf("bigBothWays: %v, want %d frames each way, downstream as messages", d, bigFrames)
		}
	}

	// mixedSizesOrder: upstream, 100 B frames take messages and 64 KiB
	// frames up:post (JS sends them after Go's accept; Go is the acceptor);
	// every echo is a message, and the echo keeps the order.
	var mixed struct {
		Frames  int  `json:"frames"`
		InOrder bool `json:"inOrder"`
	}
	if d, ok := decodeResult(t, res, "mixedSizesOrder", &mixed); ok {
		if mixed.Frames != mixedFrames || !mixed.InOrder {
			t.Errorf("mixedSizesOrder = %+v, want %d frames in order", mixed, mixedFrames)
		}
		half := mixedFrames / 2
		if total(d.upLane) != half || total(d.upMsg) != half || total(d.downLane) != 0 || total(d.downMsg) != mixedFrames {
			t.Errorf("mixedSizesOrder: %v, want %d frames on up:post and %d as messages, all %d echoes as messages", d, half, half, mixedFrames)
		}
	}

	checkLaneFailure(t, st, res)
}

// checkLaneFailure: with no downstream lane nothing is cut, so both
// streams get every frame and end cleanly; the page keeps up:post.
func checkLaneFailure(t *testing.T, st *lane.State, res map[string]laneResult) {
	var lf struct {
		ACode     *string     `json:"aCode"`
		AReceived int         `json:"aReceived"`
		BOK       bool        `json:"bOk"`
		BFrames   int         `json:"bFrames"`
		Lanes     lanesResult `json:"lanes"`
	}
	d, ok := decodeResult(t, res, "laneFailure", &lf)
	if !ok {
		return
	}
	if lf.ACode != nil || lf.AReceived != 8 {
		t.Errorf("laneFailure: stream A ended with code %v after %d frames, want no error and 8", lf.ACode, lf.AReceived)
	}
	if !lf.BOK || lf.BFrames != 12 {
		t.Errorf("laneFailure: B ok=%v frames=%d, want all 12 intact and in order", lf.BOK, lf.BFrames)
	}
	if (lf.Lanes != lanesResult{"fallback", "up:post"}) {
		t.Errorf("laneFailure: tango.lanes() = %+v, want {fallback up:post}", lf.Lanes)
	}
	if total(d.downLane) != 0 {
		t.Errorf("laneFailure: %v, want no downstream lane frames", d)
	}
	if st.MinSize() != -1 {
		t.Errorf("laneFailure: BinaryMinSize = %d, want -1 (no downstream lane)", st.MinSize())
	}
}

func checkLanesDisabled(t *testing.T, c *laneConn, probe *laneProbe, res map[string]laneResult) {
	if got, want := c.helloReplies(), []string{`{"tango":"hello","proto":2,"win":1048576}`}; !equalJSON(got, want) {
		t.Errorf("Go hello replies = %q, want %q (no lanes field)", got, want)
	}
	var l lanesResult
	if _, ok := decodeResult(t, res, "lanesUp", &l); ok && (l != lanesResult{"fallback", "fallback"}) {
		t.Errorf("lanesUp: tango.lanes() = %+v, want fallback both ways", l)
	}
	var big bigResult
	if d, ok := decodeResult(t, res, "bigBothWays", &big); ok {
		if (big != bigResult{bigTotal, bigFrames, true}) {
			t.Errorf("bigBothWays = %+v, want {%d %d true}", big, bigTotal, bigFrames)
		}
		if total(d.upLane) != 0 || total(d.downLane) != 0 || total(d.upMsg) != bigFrames || total(d.downMsg) != bigFrames {
			t.Errorf("bigBothWays: %v, want all %d frames as messages each way", d, bigFrames)
		}
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.requests != 0 || len(probe.ctl) != 0 {
		t.Errorf("lanes off: %d lane requests, control messages %q; want none", probe.requests, probe.ctl)
	}
}

// containsJSON reports whether one of list equals want as JSON.
func containsJSON(list []string, want string) bool {
	for _, s := range list {
		if equalJSON(json.RawMessage(s), json.RawMessage(want)) {
			return true
		}
	}
	return false
}

func (c *laneConn) DropBinarySession() { c.st.Reset() }
