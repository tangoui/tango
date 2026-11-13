package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tangoui/tango/ipc"
)

func TestRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "a.log")
	r, err := OpenRotating(path, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"1111\n", "2222\n", "3333\n", "4444\n", "5555\n", "6666\n", "77777777777777\n"} {
		if _, err := r.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	if got := read(path); got != "77777777777777\n" {
		t.Errorf("current = %q", got)
	}
	if got := read(path + ".1"); got != "5555\n6666\n" {
		t.Errorf(".1 = %q", got)
	}
	if got := read(path + ".2"); got != "3333\n4444\n" {
		t.Errorf(".2 = %q", got)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf(".3 kept: %v", err)
	}
	if _, err := r.Write([]byte("x")); err == nil {
		t.Error("write after Close succeeded")
	}

	// Reopening appends and counts the existing size.
	r, _ = OpenRotating(path, 20, 2)
	r.Write([]byte("8\n"))
	r.Write([]byte("9999\n"))
	r.Close()
	if got := read(path + ".1"); got != "77777777777777\n8\n" {
		t.Errorf("after reopen .1 = %q", got)
	}
}

func TestLogger(t *testing.T) {
	dir := t.TempDir()
	var console bytes.Buffer
	l, err := New(Options{Dir: dir, Console: &console, Level: slog.LevelDebug})
	if err != nil {
		t.Fatal(err)
	}
	l.Debug("hello", "n", 1)
	if err := l.Shutdown(); err != nil {
		t.Fatal(err)
	}
	l.Info("after close") // console only, no error
	b, _ := os.ReadFile(filepath.Join(dir, DefaultFile))
	if !strings.Contains(string(b), "msg=hello n=1") || strings.Contains(string(b), "after close") {
		t.Errorf("file = %q", b)
	}
	if !strings.Contains(console.String(), "msg=hello") || !strings.Contains(console.String(), "after close") {
		t.Errorf("console = %q", console.String())
	}
	if l.Path() != filepath.Join(dir, DefaultFile) {
		t.Errorf("Path = %q", l.Path())
	}
	if _, err := New(Options{}); err == nil {
		t.Error("New without Dir or AppID succeeded")
	}
}

func TestDefaultDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "local"))
	l, err := New(Options{AppID: "com.example.logtest", Console: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Shutdown()
	if !strings.HasPrefix(l.Path(), home) || !strings.Contains(l.Path(), "com.example.logtest") {
		t.Errorf("Path = %q", l.Path())
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

type conn struct{ ch chan string }

func (c *conn) Send(m string) { c.ch <- m }

func TestHandler(t *testing.T) {
	var out syncBuf
	r := ipc.NewRouter()
	Handler(r, slog.New(slog.NewTextHandler(&out, nil)))
	c := &conn{ch: make(chan string, 1)}
	call := func(args string) *ipc.Error {
		r.HandleMessage(c, `{"tango":"call","id":1,"method":"Log.Write","args":`+args+`}`)
		var o struct{ Error *ipc.Error }
		if err := json.Unmarshal([]byte(<-c.ch), &o); err != nil {
			t.Fatal(err)
		}
		return o.Error
	}
	if e := call(`["warn","upload failed",{"file":"a.png"}]`); e != nil {
		t.Fatal(e)
	}
	if e := call(`["bogus","plain"]`); e != nil {
		t.Fatal(e)
	}
	got := out.String()
	for _, want := range []string{`level=WARN msg="upload failed" source=page file=a.png`, `level=INFO msg=plain source=page`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	for _, bad := range []string{`[]`, `["info"]`, `[1,"x"]`, `["info","x",[1]]`} {
		if e := call(bad); e == nil || e.Code != ipc.CodeBadArgs {
			t.Errorf("%s: %v", bad, e)
		}
	}
}
