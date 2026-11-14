package runtimejs_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
// side, the real runtime-js (IIFE, down:scheme and up:post modules) in node
// on the other. Messages travel over stdio as in TestEndToEndWithNode; the
// scheme lanes are real HTTP requests to an httptest server serving
// lane.State.ServeDown/ServeUp, as the core serves them on the asset origin.
// Sizes and byte patterns must match the lane scenarios in
// testdata/harness.mjs.
const (
	laneGapMs     = 300 // harness.mjs LANE_GAP_MS: the page's gap timeout
	bigTotal      = 8 << 20
	bigFrames     = bigTotal / (64 << 10)
	mixedFrames   = 40
	laneCtlPrefix = corehook.InternalPrefix + "lane:"
)

// laneCounts counts stream byte frames per stream id, by the path they
// took: a lane packet (up: delivered by ServeUpSession to HandleBinarySession; down:
// written by ServeDown) or a "bin" message on the message channel.
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
	armed    bool     // cut the down:scheme response at its next packet
	lost     []ipc.PacketFrame
	atTrip   *laneCounts
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
	if down {
		p.n.downMsg[m.ID]++
	} else {
		p.n.upMsg[m.ID]++
	}
	p.mu.Unlock()
}

// laneConn is a pipeConn with fast lanes: it implements ipc.BinaryConn by
// delegating to a lane.State offering down:scheme and up:post, as
// tango.Surface does for a page on the asset origin.
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
	return c.st.Hello([]string{lane.DownScheme, lane.UpPost}, c.enabled)
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

var errDownCut = errors.New("test: down:scheme response cut")

// downWriter is the down:scheme ResponseWriter: it counts the frames of
// every packet ServeDown writes and, when the probe is armed, refuses the
// next packet that carries frames. ServeDown then treats the response as
// broken: the lane fails, the response ends, and that packet plus any
// queued behind it are lost.
type downWriter struct {
	http.ResponseWriter
	probe *laneProbe
	trip  func()
}

// Unwrap lets ServeDown (and http.ResponseController) find the Flusher.
func (w downWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w downWriter) Write(b []byte) (int, error) {
	frames, err := ipc.ParsePacket(b)
	if err != nil {
		w.probe.problem("ServeDown wrote %d bytes that are not one packet: %v", len(b), err)
		return w.ResponseWriter.Write(b)
	}
	p := w.probe
	p.mu.Lock()
	if p.armed && len(frames) > 0 {
		p.armed = false
		for _, f := range frames {
			p.lost = append(p.lost, ipc.PacketFrame{Stream: f.Stream, Seq: f.Seq})
		}
		p.mu.Unlock()
		go w.trip()
		return 0, errDownCut
	}
	for _, f := range frames {
		p.n.downLane[f.Stream]++
	}
	p.mu.Unlock()
	return w.ResponseWriter.Write(b)
}

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
	t.Run("scheme", func(t *testing.T) { runLanes(t, node, script, true) })
	t.Run("disabled", func(t *testing.T) { runLanes(t, node, script, false) })
}

// runLanes runs the harness's lane scenarios: with enabled, LaneHello
// offers down:scheme and up:post; without, it returns nil (no lanes field).
func runLanes(t *testing.T, node, script string, enabled bool) {
	probe := &laneProbe{n: newLaneCounts()}
	st := lane.New(nil)
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
	mux.HandleFunc("/__tango/lane/down", func(w http.ResponseWriter, req *http.Request) {
		probe.mu.Lock()
		probe.requests++
		probe.mu.Unlock()
		st.ServeDown(downWriter{w, probe, func() { tripped(t, r, conn, st, probe) }}, req)
	})
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
	defer st.Close()   // ends the down:scheme response, so srv.Close returns
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
				checkSchemeLanes(t, conn, st, probe, results)
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

// tripped runs after downWriter refused a packet: once ServeDown has
// failed down:scheme (MinSize -1: no down lane), it notes the counts and
// tells the page, which then sends the later frames.
func tripped(t *testing.T, r *ipc.Router, c *laneConn, st *lane.State, probe *laneProbe) {
	deadline := time.Now().Add(5 * time.Second)
	for st.MinSize() != -1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if st.MinSize() != -1 {
		probe.problem("down:scheme still usable 5s after its response broke")
	}
	probe.mu.Lock()
	n := probe.n.clone()
	probe.atTrip = &n
	probe.mu.Unlock()
	r.Emit(c, "lanefail:tripped", nil)
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

func checkSchemeLanes(t *testing.T, c *laneConn, st *lane.State, probe *laneProbe, res map[string]laneResult) {
	// Go's hello reply offers both lanes.
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
		!hexToken.MatchString(h.Lanes.Tok) || strings.Join(h.Lanes.List, ",") != "down:scheme,up:post" {
		t.Errorf("hello reply %s: want lanes {gen, 32 hex tok, list [down:scheme up:post]}", hellos[0])
	}

	// lanesUp: the page reports both lanes ready, and told Go so.
	var l lanesResult
	if _, ok := decodeResult(t, res, "lanesUp", &l); ok && (l != lanesResult{"down:scheme", "up:post"}) {
		t.Errorf("lanesUp: tango.lanes() = %+v, want {down:scheme up:post}", l)
	}
	probe.mu.Lock()
	ctl := append([]string(nil), probe.ctl...)
	probe.mu.Unlock()
	gen := uint64(0)
	if h.Lanes != nil {
		gen = h.Lanes.Gen
	}
	for _, want := range []string{
		fmt.Sprintf(`{"op":"ready","lane":"down:scheme","gen":%d}`, gen),
		fmt.Sprintf(`{"op":"ready","lane":"up:post","gen":%d}`, gen),
		fmt.Sprintf(`{"op":"fail","lane":"down:scheme","gen":%d}`, gen), // laneFailure
	} {
		if !containsJSON(ctl, want) {
			t.Errorf("lane control messages %q: missing %s", ctl, want)
		}
	}

	// bigBothWays: 8 MiB echoed intact, at least half the frames on a lane
	// each way, and every frame counted exactly once.
	var big bigResult
	if d, ok := decodeResult(t, res, "bigBothWays", &big); ok {
		if (big != bigResult{bigTotal, bigFrames, true}) {
			t.Errorf("bigBothWays = %+v, want {%d %d true}", big, bigTotal, bigFrames)
		}
		if total(d.upLane) < bigFrames/2 || total(d.downLane) < bigFrames/2 {
			t.Errorf("bigBothWays: %v, want at least %d frames on each lane", d, bigFrames/2)
		}
		if total(d.upLane)+total(d.upMsg) != bigFrames || total(d.downLane)+total(d.downMsg) != bigFrames {
			t.Errorf("bigBothWays: %v, want %d frames each way", d, bigFrames)
		}
	}

	// mixedSizesOrder: 100 B frames take messages, 64 KiB frames the lanes
	// (JS sends them after Go's accept; Go is the acceptor), and the echo
	// keeps the order.
	var mixed struct {
		Frames  int  `json:"frames"`
		InOrder bool `json:"inOrder"`
	}
	if d, ok := decodeResult(t, res, "mixedSizesOrder", &mixed); ok {
		if mixed.Frames != mixedFrames || !mixed.InOrder {
			t.Errorf("mixedSizesOrder = %+v, want %d frames in order", mixed, mixedFrames)
		}
		half := mixedFrames / 2
		if total(d.downLane) != half || total(d.downMsg) != half || total(d.upLane) != half || total(d.upMsg) != half {
			t.Errorf("mixedSizesOrder: %v, want %d frames on the lane and %d as messages each way", d, half, half)
		}
	}

	checkLaneFailure(t, st, probe, res)
}

// checkLaneFailure: A's echo of its frame 2 was handed to down:scheme and
// lost when the response broke, so A fails with transport within the
// (300 ms) gap timeout; A's later echoes and all of B's take messages; B,
// open throughout, gets every frame.
func checkLaneFailure(t *testing.T, st *lane.State, probe *laneProbe, res map[string]laneResult) {
	var lf struct {
		ACode      *string     `json:"aCode"`
		AReceived  int         `json:"aReceived"`
		AElapsedMs int         `json:"aElapsedMs"`
		BOK        bool        `json:"bOk"`
		BFrames    int         `json:"bFrames"`
		Lanes      lanesResult `json:"lanes"`
	}
	if _, ok := decodeResult(t, res, "laneFailure", &lf); !ok {
		return
	}
	if lf.ACode == nil || *lf.ACode != ipc.CodeTransport {
		t.Errorf("laneFailure: stream A ended with code %v, want %q", lf.ACode, ipc.CodeTransport)
	}
	if lf.AReceived != 2 {
		t.Errorf("laneFailure: A received %d frames, want 2 (frame 2 lost)", lf.AReceived)
	}
	if lf.AElapsedMs >= 5000 {
		t.Errorf("laneFailure: A failed %d ms after the cut, want < 5000", lf.AElapsedMs)
	}
	if !lf.BOK || lf.BFrames != 12 {
		t.Errorf("laneFailure: B ok=%v frames=%d, want all 12 intact and in order", lf.BOK, lf.BFrames)
	}
	if (lf.Lanes != lanesResult{"fallback", "up:post"}) {
		t.Errorf("laneFailure: tango.lanes() after the cut = %+v, want {fallback up:post}", lf.Lanes)
	}
	if st.MinSize() != -1 {
		t.Errorf("laneFailure: BinaryMinSize = %d after the cut, want -1", st.MinSize())
	}

	probe.mu.Lock()
	lost := append([]ipc.PacketFrame(nil), probe.lost...)
	atTrip := probe.atTrip
	now := probe.n.clone()
	probe.mu.Unlock()
	if len(lost) != 1 || lost[0].Seq != 2 || atTrip == nil {
		t.Errorf("laneFailure: lost frames %+v (tripped=%v), want A's echo seq 2", lost, atTrip != nil)
		return
	}
	a := lost[0].Stream
	b := uint64(0)
	for id := range now.downLane {
		if id != a && now.downLane[id] == 4 && (now.downMsg[id]-atTrip.downMsg[id]) > 0 {
			b = id
		}
	}
	after := now.minus(*atTrip)
	t.Logf("laneFailure: A=%d B=%d; down lane before the cut A=%d B=%d; after the cut: %v, A msgs=%d B msgs=%d",
		a, b, atTrip.downLane[a], atTrip.downLane[b], after, after.downMsg[a], after.downMsg[b])
	if atTrip.downLane[a] != 2 {
		t.Errorf("laneFailure: A had %d frames on down:scheme before the cut, want 2", atTrip.downLane[a])
	}
	if b == 0 {
		t.Errorf("laneFailure: no stream B with 4 lane frames before the cut and messages after: %v", now)
	} else if after.downMsg[b] != 8 {
		t.Errorf("laneFailure: B's 8 later echoes: %d as messages, want 8", after.downMsg[b])
	}
	if total(after.downLane) != 0 {
		t.Errorf("laneFailure: %d frames written on down:scheme after the cut", total(after.downLane))
	}
	if after.downMsg[a] < 1 {
		t.Errorf("laneFailure: none of A's later echoes went as messages")
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
