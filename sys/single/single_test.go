package single

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

const timeout = 5 * time.Second

// removeFiles deletes what a key leaves behind (the Unix lock file).
var removeFiles = func(name string) {}

func uniqueKey(t *testing.T) string {
	k := fmt.Sprintf("tango-test-%s-%d-%d", t.Name(), os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { removeFiles(name(k)) })
	return k
}

func TestListenSend(t *testing.T) {
	n := name(uniqueKey(t))
	got := make(chan message, 1)
	srv, err := listen(n, func(m message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := listen(n, func(message) {}); err != errBusy {
		t.Fatalf("second listen: %v, want errBusy", err)
	}
	if err := send(n, []byte(`{"args":["a","b"],"cwd":"/x"}`), timeout); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if !slices.Equal(m.Args, []string{"a", "b"}) || m.Cwd != "/x" {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(timeout):
		t.Fatal("no message")
	}
	// Garbage is dropped without an ack.
	if err := send(n, []byte(`not json`), 500*time.Millisecond); err == nil {
		t.Fatal("garbage acknowledged")
	}
	srv.Close()
	srv.Close() // idempotent
	// The key is free again once the primary is gone.
	srv2, err := listen(n, func(message) {})
	if err != nil {
		t.Fatalf("listen after close: %v", err)
	}
	srv2.Close()
}

func TestSendNoPrimary(t *testing.T) {
	if err := send(name(uniqueKey(t)), []byte(`{}`), 200*time.Millisecond); err == nil {
		t.Fatal("send without primary succeeded")
	}
}

// Many claimants at once: exactly one wins, the rest reach it.
func TestClaimRace(t *testing.T) {
	n := name(uniqueKey(t))
	var delivered atomic.Int32
	var wg sync.WaitGroup
	var mu sync.Mutex
	var servers []server
	forwarded := 0
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv, fwd, err := claim(n, []string{strconv.Itoa(i)}, func(message) { delivered.Add(1) }, timeout)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case srv != nil:
				servers = append(servers, srv)
			case fwd:
				forwarded++
			default:
				t.Errorf("claim: %v", err)
			}
		}()
	}
	wg.Wait()
	for _, s := range servers {
		defer s.Close()
	}
	if len(servers) != 1 || forwarded != 7 {
		t.Fatalf("%d primaries, %d forwarded", len(servers), forwarded)
	}
	deadline := time.Now().Add(timeout)
	for delivered.Load() != 7 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if d := delivered.Load(); d != 7 {
		t.Fatalf("delivered %d", d)
	}
}

func TestClaimNoKey(t *testing.T) {
	a, _ := faketest.NewApp(tango.Options{})
	if primary, err := Claim(a, Options{}); !primary || err == nil {
		t.Fatalf("primary=%v err=%v", primary, err)
	}
}

const helperEnv = "TANGO_SINGLE_TEST_HELPER"

// TestHelperProcess is the "second instance" of the process tests.
func TestHelperProcess(t *testing.T) {
	key := os.Getenv(helperEnv)
	if key == "" {
		t.Skip("helper process only")
	}
	a, _ := faketest.NewApp(tango.Options{ID: key})
	primary, err := Claim(a, Options{Timeout: timeout})
	fmt.Printf("HELPER primary=%v err=%v\n", primary, err)
	if primary {
		// A primary that won a race stays up a bit for the others.
		time.Sleep(1500 * time.Millisecond)
	}
}

func helper(t *testing.T, key string, args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), helperEnv+"="+key)
	cmd.Dir = t.TempDir()
	return cmd
}

func TestSecondProcessForwards(t *testing.T) {
	key := uniqueKey(t)
	a, fb := faketest.NewApp(tango.Options{ID: key, URLSchemes: []string{"demo"}})
	type second struct {
		args []string
		cwd  string
	}
	seconds := make(chan second, 1)
	hooked := make(chan []string, 1)
	OnSecondInstance(a, func(args []string, _ string) { hooked <- args })
	primary, err := Claim(a, Options{OnSecondInstance: func(args []string, cwd string) {
		seconds <- second{args, cwd}
	}})
	if !primary || err != nil {
		t.Fatalf("primary=%v err=%v", primary, err)
	}
	urls := make(chan string, 1)
	a.OnOpenURL(func(u string) { urls <- u })
	faketest.Start(t, a, fb)
	s, err := a.NewSurface(tango.SurfaceOptions{Hidden: true})
	if err != nil {
		t.Fatal(err)
	}

	cmd := helper(t, key, "demo://hello?x=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "HELPER primary=false err=<nil>") {
		t.Fatalf("helper output:\n%s", out)
	}
	select {
	case sec := <-seconds:
		if !slices.Contains(sec.args, "demo://hello?x=1") || sec.cwd == "" {
			t.Fatalf("got %+v", sec)
		}
	case <-time.After(timeout):
		t.Fatal("OnSecondInstance not called")
	}
	select {
	case args := <-hooked:
		if !slices.Contains(args, "demo://hello?x=1") {
			t.Fatalf("hook got %q", args)
		}
	case <-time.After(timeout):
		t.Fatal("single.OnSecondInstance handler not called")
	}
	select {
	case u := <-urls:
		if u != "demo://hello?x=1" {
			t.Fatalf("url %q", u)
		}
	case <-time.After(timeout):
		t.Fatal("no EventOpenURL")
	}
	ns := fb.Surface(s.ID())
	if !ns.Visible() || ns.Focuses() == 0 {
		t.Fatalf("main surface not brought forward: visible=%v focuses=%d", ns.Visible(), ns.Focuses())
	}
}

// Processes started together: exactly one becomes primary.
func TestProcessesRace(t *testing.T) {
	key := uniqueKey(t)
	const n = 4
	outs := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := helper(t, key).CombinedOutput()
			if err != nil {
				t.Errorf("helper %d: %v\n%s", i, err, out)
			}
			outs[i] = string(out)
		}()
	}
	wg.Wait()
	primaries := 0
	for _, o := range outs {
		sc := bufio.NewScanner(strings.NewReader(o))
		for sc.Scan() {
			switch sc.Text() {
			case "HELPER primary=true err=<nil>":
				primaries++
			case "HELPER primary=false err=<nil>":
			default:
				if strings.HasPrefix(sc.Text(), "HELPER") {
					t.Errorf("unexpected: %s", sc.Text())
				}
			}
		}
	}
	if primaries != 1 {
		t.Fatalf("%d primaries:\n%s", primaries, strings.Join(outs, "\n"))
	}
}
