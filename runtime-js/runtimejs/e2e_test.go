package runtimejs_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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

// pipeConn plays a Surface: Send writes one JSON-encoded line to node.
type pipeConn struct {
	mu sync.Mutex
	w  io.Writer
}

func (c *pipeConn) Send(msg string) {
	b, _ := json.Marshal(msg)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.w.Write(append(b, '\n'))
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
	defer stdin.Close()

	c := &pipeConn{w: stdin}
	r := ipc.NewRouter()
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
	r.On("open-news", func(cc ipc.Conn, _ json.RawMessage) {
		go func() {
			s, err := r.OpenStream(cc, "news", "tech")
			if err != nil {
				t.Error(err)
				return
			}
			for i := 0; i < 150; i++ {
				if err := s.Write(i); err != nil {
					t.Error(err)
					return
				}
			}
			s.Close()
		}()
	})

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	sawHello := false
	for sc.Scan() {
		var msg string
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			t.Fatalf("bad line %q", sc.Text())
		}
		if msg == `{"tango":"hello"}` {
			sawHello = true
		}
		if r.HandleMessage(c, msg) {
			continue
		}
		switch {
		case strings.HasPrefix(msg, "FAIL "):
			t.Fatal(msg)
		case strings.HasPrefix(msg, "DONE "):
			check(t, msg[5:])
			if !sawHello {
				t.Error("no hello handshake")
			}
			return
		default:
			c.Send("echo:" + msg) // raw message round trip
		}
	}
	t.Fatalf("node exited early: %v", sc.Err())
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
		"news":     map[string]any{"topic": "tech", "count": 150.0, "last": 149.0},
		"raw":      "echo:raw hello",
	}
	for k, w := range want {
		gb, _ := json.Marshal(got[k])
		wb, _ := json.Marshal(w)
		if string(gb) != string(wb) {
			t.Errorf("%s = %s, want %s", k, gb, wb)
		}
	}
}
