package kv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/tangoui/tango/bind"
	"github.com/tangoui/tango/ipc"
)

func TestMemory(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	if s.Get("x") != nil || s.Has("x") {
		t.Fatal("empty store has x")
	}
	must(t, s.Set("b", 1))
	must(t, s.Set("a", "one"))
	if got := s.Keys(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("Keys = %q", got)
	}
	if s.Get("a") != "one" || !s.Has("b") {
		t.Fatal("bad Get/Has")
	}
	must(t, s.Delete("a"))
	must(t, s.Delete("missing"))
	if s.Has("a") {
		t.Fatal("a not deleted")
	}
	must(t, s.Clear())
	if len(s.Keys()) != 0 {
		t.Fatal("not cleared")
	}
	if err := s.Set("bad", make(chan int)); err == nil {
		t.Fatal("Set accepted an unencodable value")
	}
}

func TestPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "store.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	must(t, s.Set("n", 2))
	must(t, s.Set("obj", map[string]any{"k": []int{1, 2}}))
	must(t, s.Set("gone", true))
	must(t, s.Delete("gone"))

	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Keys(); !slices.Equal(got, []string{"n", "obj"}) {
		t.Fatalf("reloaded keys = %q", got)
	}
	if s2.Get("n") != float64(2) {
		t.Fatalf("n = %#v", s2.Get("n"))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temporary files left: %v", entries)
	}

	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("corrupt file loaded")
	}
}

func TestFailedWriteLeavesStoreUnchanged(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("relies on Unix directory permissions")
	}
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	must(t, s.Set("a", 1))
	must(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if err := s.Set("b", 2); err == nil {
		t.Fatal("write to read-only dir succeeded")
	}
	if s.Has("b") {
		t.Fatal("store changed despite failed write")
	}
}

func TestConcurrent(t *testing.T) {
	s, _ := New(filepath.Join(t.TempDir(), "s.json"))
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 20 {
				k := fmt.Sprintf("k%d-%d", i, j)
				_ = s.Set(k, j)
				_ = s.Get(k)
				_ = s.Keys()
			}
		})
	}
	wg.Wait()
	if n := len(s.Keys()); n != 160 {
		t.Fatalf("%d keys", n)
	}
}

type conn struct{ ch chan string }

func (c *conn) Send(m string) { c.ch <- m }

func TestBindable(t *testing.T) {
	s, _ := New("")
	r := ipc.NewRouter()
	if err := bind.Register(r, "KV", s); err != nil {
		t.Fatal(err)
	}
	c := &conn{ch: make(chan string, 1)}
	r.HandleMessage(c, `{"tango":"call","id":1,"method":"KV.Set","args":["k",{"a":1}]}`)
	<-c.ch
	r.HandleMessage(c, `{"tango":"call","id":2,"method":"KV.Get","args":["k"]}`)
	var o struct{ Result json.RawMessage }
	if err := json.Unmarshal([]byte(<-c.ch), &o); err != nil {
		t.Fatal(err)
	}
	if string(o.Result) != `{"a":1}` {
		t.Fatalf("Get over ipc = %s", o.Result)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
