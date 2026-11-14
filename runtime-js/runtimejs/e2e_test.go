package runtimejs_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/bind"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/runtime-js/runtimejs"
)

// pipeConn plays a Surface: Send queues one JSON-encoded line for node and
// never blocks. A writer goroutine drains the queue into node's stdin, so
// the test goroutine (which reads node's stdout and runs HandleMessage) can
// never deadlock against node blocking on a full stdout pipe while Go
// streams fill node's stdin.
type pipeConn struct {
	mu     sync.Mutex
	cond   *sync.Cond
	queue  [][]byte
	closed bool
	hello  []string // Go's hello replies, as sent
}

func newPipeConn(w io.WriteCloser) *pipeConn {
	c := &pipeConn{}
	c.cond = sync.NewCond(&c.mu)
	go c.run(w)
	return c
}

func (c *pipeConn) Send(msg string) {
	b, _ := json.Marshal(msg)
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.HasPrefix(msg, `{"tango":"hello"`) {
		c.hello = append(c.hello, msg)
	}
	if c.closed {
		return
	}
	c.queue = append(c.queue, append(b, '\n'))
	c.cond.Signal()
}

func (c *pipeConn) run(w io.WriteCloser) {
	defer w.Close()
	for {
		c.mu.Lock()
		for len(c.queue) == 0 && !c.closed {
			c.cond.Wait()
		}
		q := c.queue
		c.queue = nil
		closed := c.closed
		c.mu.Unlock()
		for _, line := range q {
			if _, err := w.Write(line); err != nil {
				return
			}
		}
		if closed {
			return
		}
	}
}

// close flushes the queue and then closes node's stdin.
func (c *pipeConn) close() {
	c.mu.Lock()
	c.closed = true
	c.cond.Signal()
	c.mu.Unlock()
}

func (c *pipeConn) helloReplies() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.hello...)
}

type svc struct{}

func (svc) Div(a, b int) (int, error) {
	if b == 0 {
		return 0, ipc.NewError(ipc.CodeBadArgs, "division by zero")
	}
	return a / b, nil
}

func TestScriptEmbedded(t *testing.T) {
	if !strings.Contains(runtimejs.Script, "__tango") || !strings.Contains(runtimejs.Script, "DO NOT EDIT") {
		t.Fatal("embedded runtime looks wrong; run npm run build in runtime-js")
	}
}

// Sizes and byte patterns shared with testdata/harness.mjs.
const (
	echoBigLen   = 1 << 20 // duplexEcho: byte i = i % 251
	pumpTotal    = 4 << 20 // backpressureBothWays, each way
	pumpFrame    = 64 << 10
	pumpWindow   = 64 << 10 // SetStreamWindow
	uploadTotal  = 3 << 20  // pipeUpload: byte i = (i*31 + 7) & 255
	newsCount    = 150
	resultPrefix = "result:"
)

func echoBig() []byte {
	b := make([]byte, echoBigLen)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func pumpGoByte(i int) byte { return byte(i % 251) } // Go → JS
func pumpJSByte(i int) byte { return byte(i % 253) } // JS → Go
func uploadByte(i int) byte { return byte((i*31 + 7) & 255) }

// echoSummary describes the frames received from Echo, identically in Go
// and in the harness: "json:<encoded value>" or "bytes:<len>:<sha256>",
// plus whether the 1 MiB frame matched its pattern byte for byte.
type echoSummary struct {
	Frames     []string `json:"frames"`
	BigPattern bool     `json:"bigPattern"`
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func summarize(msgs []ipc.Message) echoSummary {
	s := echoSummary{Frames: []string{}}
	for _, m := range msgs {
		if m.Binary {
			s.Frames = append(s.Frames, fmt.Sprintf("bytes:%d:%s", len(m.Bytes), sha(m.Bytes)))
		} else {
			s.Frames = append(s.Frames, "json:"+string(m.JSON))
		}
	}
	s.BigPattern = len(msgs) == 4 && msgs[3].Binary && bytes.Equal(msgs[3].Bytes, echoBig())
	return s
}

// echoInputs are duplexEcho's frames: JSON {"k":"v"}, bytes [0,255], an
// empty byte frame and 1 MiB.
func echoInputs() []ipc.Message {
	return []ipc.Message{
		{JSON: json.RawMessage(`{"k":"v"}`)},
		{Binary: true, Bytes: []byte{0, 255}},
		{Binary: true, Bytes: []byte{}},
		{Binary: true, Bytes: echoBig()},
	}
}

// echoInProcess runs duplexEcho's inputs through Echo with Router.Open.
func echoInProcess(ctx context.Context, r *ipc.Router) (echoSummary, error) {
	s, err := r.Open(ctx, "Echo")
	if err != nil {
		return echoSummary{}, err
	}
	type res struct {
		msgs []ipc.Message
		err  error
	}
	got := make(chan res, 1)
	go func() {
		var msgs []ipc.Message
		for {
			m, err := s.Receive()
			if err == io.EOF {
				got <- res{msgs, nil}
				return
			}
			if err != nil {
				got <- res{msgs, err}
				return
			}
			msgs = append(msgs, m)
		}
	}()
	for _, m := range echoInputs() {
		if m.Binary {
			err = s.WriteBytes(m.Bytes)
		} else {
			err = s.Write(map[string]string{"k": "v"})
		}
		if err != nil {
			return echoSummary{}, err
		}
	}
	s.CloseSend()
	g := <-got
	if g.err != nil {
		return echoSummary{}, g.err
	}
	return summarize(g.msgs), nil
}

type pumpStats struct {
	N      int  `json:"n"`
	Last   int  `json:"last"`
	OK     bool `json:"ok"`
	Frames int  `json:"frames"`
}

func TestEndToEndWithNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "tango.iife.js")
	if err := os.WriteFile(script, []byte(runtimejs.Script), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/harness.mjs", script)
	cmd.Stderr = os.Stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	c := newPipeConn(stdin)
	defer c.close()

	r := ipc.NewRouter()
	// Advertised in Go's hello reply, so it applies to every JS → Go
	// stream (backpressureBothWays relies on it).
	r.SetStreamWindow(pumpWindow)
	r.Handle("Greeter.Hello", func(_ context.Context, args json.RawMessage) (any, error) {
		var a []string
		if err := json.Unmarshal(args, &a); err != nil || len(a) != 1 {
			return nil, ipc.NewError(ipc.CodeBadArgs, "want one string")
		}
		return "Hello " + a[0], nil
	})
	if err := bind.Register(r, "Svc", svc{}); err != nil {
		t.Fatal(err)
	}
	r.HandleStream("Count", func(ctx context.Context, args json.RawMessage, w ipc.StreamWriter) error {
		var a []int
		json.Unmarshal(args, &a)
		for i := 0; i < a[0]; i++ {
			if err := w.Write(i); err != nil {
				return err
			}
		}
		return nil
	})
	r.HandleStream("Forever", func(ctx context.Context, _ json.RawMessage, w ipc.StreamWriter) error {
		for i := 0; ; i++ {
			if err := w.Write(i); err != nil {
				var e *ipc.Error
				code := ""
				if errors.As(err, &e) {
					code = e.Code
				}
				r.Emit(c, "canceled", code)
				return err
			}
		}
	})
	r.On("ping", func(cc ipc.Conn, data json.RawMessage) {
		var d struct{ N int }
		json.Unmarshal(data, &d)
		r.Emit(cc, "pong", d.N+1)
	})

	// news: Go opens, writes 150 values, ends its side, then receives the
	// one frame JS writes back.
	newsBack := make(chan string, 1)
	r.On("open-news", func(cc ipc.Conn, _ json.RawMessage) {
		go func() {
			s, err := r.OpenStream(cc, "news", "tech")
			if err != nil {
				newsBack <- "open: " + err.Error()
				return
			}
			for i := 0; i < newsCount; i++ {
				if err := s.Write(i); err != nil {
					newsBack <- "write: " + err.Error()
					return
				}
			}
			s.CloseSend()
			m, err := s.Receive()
			_, err2 := s.Receive()
			newsBack <- fmt.Sprintf("binary=%v json=%s err=%v next=%v", m.Binary, m.JSON, err, err2)
		}()
	})

	// duplexEcho, socketEcho, inProcessParity: echo every frame as is.
	r.HandleDuplex("Echo", func(s *ipc.Stream, _ json.RawMessage) error {
		for {
			m, err := s.Receive()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if m.Binary {
				err = s.WriteBytes(m.Bytes)
			} else {
				err = s.Write(m.JSON)
			}
			if err != nil {
				return err
			}
		}
	})
	r.Handle("Parity.Run", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return echoInProcess(ctx, r)
	})

	// backpressureBothWays: 4 MiB each way in 64 KiB frames, concurrently.
	pumpGo := make(chan any, 1)
	r.HandleDuplex("Pump", func(s *ipc.Stream, _ json.RawMessage) error {
		werr := make(chan error, 1)
		go func() {
			buf := make([]byte, pumpFrame)
			for off := 0; off < pumpTotal; off += pumpFrame {
				for i := range buf {
					buf[i] = pumpGoByte(off + i)
				}
				if err := s.WriteBytes(buf); err != nil {
					werr <- err
					return
				}
			}
			werr <- nil
		}()
		st := pumpStats{Last: -1, OK: true}
		for {
			b, err := s.ReceiveBytes()
			if err == io.EOF {
				break
			}
			if err != nil {
				pumpGo <- "receive: " + err.Error()
				return err
			}
			for i, x := range b {
				if x != pumpJSByte(st.N+i) {
					st.OK = false
				}
			}
			st.N += len(b)
			if len(b) > 0 {
				st.Last = int(b[len(b)-1])
			}
			st.Frames++
		}
		if err := <-werr; err != nil {
			pumpGo <- "write: " + err.Error()
			return err
		}
		pumpGo <- st
		return nil
	})

	// pipeUpload: count (and hash) what JS pipes in, reply with both.
	r.HandleDuplex("Upload", func(s *ipc.Stream, _ json.RawMessage) error {
		h := sha256.New()
		n, err := io.Copy(io.Discard, io.TeeReader(s.ReadWriter(), h))
		if err != nil {
			return err
		}
		if err := s.Write(n); err != nil {
			return err
		}
		return s.Write(hex.EncodeToString(h.Sum(nil)))
	})

	results := map[string]json.RawMessage{}
	lastStarted := ""
	r.On("e2e:start", func(_ ipc.Conn, data json.RawMessage) {
		json.Unmarshal(data, &lastStarted)
	})
	for _, name := range []string{"duplexEcho", "backpressureBothWays", "pipeUpload", "socketEcho", "inProcessParity"} {
		r.On(resultPrefix+name, func(_ ipc.Conn, data json.RawMessage) {
			results[name] = append(json.RawMessage(nil), data...)
		})
	}

	sc := bufio.NewScanner(stdout)
	// A 1 MiB byte frame is ~1.4 MB of base64, plus JSON string escaping.
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	sawHello := false
	for sc.Scan() {
		var msg string
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			t.Fatalf("bad line %q", sc.Text())
		}
		if msg == `{"tango":"hello","proto":2}` {
			sawHello = true
		}
		if r.HandleMessage(c, msg) {
			continue
		}
		switch {
		case strings.HasPrefix(msg, "FAIL "):
			t.Fatalf("scenario %s: %s", lastStarted, msg)
		case strings.HasPrefix(msg, "DONE "):
			check(t, msg[5:])
			if !sawHello {
				t.Error(`no {"tango":"hello","proto":2} from JS`)
			}
			if got, want := c.helloReplies(), []string{`{"tango":"hello","proto":2,"win":65536}`}; !equalJSON(got, want) {
				t.Errorf("Go hello replies = %q, want %q", got, want)
			}
			checkNews(t, newsBack)
			checkResults(t, results, pumpGo)
			return
		default:
			c.Send("echo:" + msg) // raw message round trip
		}
	}
	t.Fatalf("node exited early (last scenario started: %q): %v", lastStarted, sc.Err())
}

func check(t *testing.T, raw string) {
	var got map[string]any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"hello":    "Hello world",
		"div":      3.0,
		"divErr":   "bad_args",
		"badArgs":  "bad_args",
		"missing":  "not_found",
		"count":    map[string]any{"n": 300.0, "sum": 300.0 * 299 / 2},
		"canceled": "canceled",
		"pong":     42.0,
		"news":     map[string]any{"topic": "tech", "count": float64(newsCount), "last": float64(newsCount - 1)},
		"raw":      "echo:raw hello",
	}
	for k, w := range want {
		if !equalJSON(got[k], w) {
			gb, _ := json.Marshal(got[k])
			wb, _ := json.Marshal(w)
			t.Errorf("%s = %s, want %s", k, gb, wb)
		}
	}
}

// checkNews checks the frame JS wrote back on the news stream.
func checkNews(t *testing.T, newsBack <-chan string) {
	want := fmt.Sprintf(`binary=false json={"got":%d,"last":%d} err=<nil> next=EOF`, newsCount, newsCount-1)
	select {
	case got := <-newsBack:
		if got != want {
			t.Errorf("news reply: %s, want %s", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Error("news: Go never received JS's frame")
	}
}

func checkResults(t *testing.T, results map[string]json.RawMessage, pumpGo <-chan any) {
	echo := summarize(echoInputs())
	want := map[string]any{
		"duplexEcho": echo,
		"backpressureBothWays": pumpStats{
			N: pumpTotal, Last: int(pumpGoByte(pumpTotal - 1)), OK: true, Frames: pumpTotal / pumpFrame,
		},
		"pipeUpload": map[string]any{
			"n": uploadTotal, "sha256": uploadSHA(), "local": uploadSHA(),
		},
		"socketEcho": map[string]any{
			"messages": []any{"hello", []int{1, 2, 3}}, "code": 1000, "wasClean": true, "readyState": 3,
		},
		"inProcessParity": map[string]any{"same": true, "go": echo},
	}
	for k, w := range want {
		got, ok := results[k]
		if !ok {
			t.Errorf("%s: no result", k)
			continue
		}
		if !equalJSON(got, w) {
			wb, _ := json.Marshal(w)
			t.Errorf("%s = %.2000s, want %s", k, got, wb)
		}
	}
	if !echo.BigPattern || len(echo.Frames) != 4 {
		t.Errorf("expected echo summary is wrong: %+v", echo)
	}

	// Go's side of backpressureBothWays: what JS sent it.
	wantGo := pumpStats{N: pumpTotal, Last: int(pumpJSByte(pumpTotal - 1)), OK: true, Frames: pumpTotal / pumpFrame}
	select {
	case got := <-pumpGo:
		st, ok := got.(pumpStats)
		if !ok {
			t.Errorf("backpressureBothWays (Go side): %v", got)
		} else if st != wantGo {
			t.Errorf("backpressureBothWays (Go side) = %+v, want %+v", st, wantGo)
		}
	case <-time.After(5 * time.Second):
		t.Error("backpressureBothWays: Go handler never finished")
	}
}

func uploadSHA() string {
	b := make([]byte, uploadTotal)
	for i := range b {
		b[i] = uploadByte(i)
	}
	return sha(b)
}

// equalJSON compares a and b by their JSON encodings, decoded and
// re-encoded so that key order and number formatting do not matter.
func equalJSON(a, b any) bool {
	return canonJSON(a) == canonJSON(b)
}

func canonJSON(v any) string {
	raw, ok := v.(json.RawMessage)
	if !ok {
		var err error
		if raw, err = json.Marshal(v); err != nil {
			return "!" + err.Error()
		}
	}
	var x any
	if err := json.Unmarshal(raw, &x); err != nil {
		return "!" + err.Error()
	}
	out, _ := json.Marshal(x)
	return string(out)
}
