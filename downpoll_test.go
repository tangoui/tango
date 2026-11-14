package tango

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/downq"
	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/internal/platform/fake"
)

// downApp runs an app with opts and creates a surface at u whose backend
// reports caps; the page has not loaded yet.
func downApp(t *testing.T, opts Options, u string, caps ...string) (*App, *fake.Backend, *Surface, *fake.Surface) {
	t.Helper()
	a, fb := laneApp(t, opts)
	s, err := a.NewSurface(SurfaceOptions{URL: u})
	if err != nil {
		t.Fatal(err)
	}
	fb.SetLaneCaps(s.ID(), caps)
	return a, fb, s, fb.Surface(s.ID())
}

type openJSON struct {
	Gen  uint64 `json:"gen"`
	Tok  string `json:"tok"`
	Path string `json:"path"`
}

const openPrefix = "window.__tango._open("

// opens returns the _open calls among evals.
func opens(t *testing.T, evals []string) []openJSON {
	t.Helper()
	var out []openJSON
	for _, e := range evals {
		rest, ok := strings.CutPrefix(e, openPrefix)
		if !ok {
			continue
		}
		var o openJSON
		if err := json.Unmarshal([]byte(strings.TrimSuffix(rest, ")")), &o); err != nil {
			t.Fatalf("_open %q: %v", e, err)
		}
		out = append(out, o)
	}
	return out
}

// lastOpen returns the last _open call, failing without one.
func lastOpen(t *testing.T, ns *fake.Surface) openJSON {
	t.Helper()
	o := opens(t, ns.Evals())
	if len(o) == 0 {
		t.Fatalf("no _open in %q", ns.Evals())
	}
	if o[len(o)-1].Path != lanePathPrefix+"poll" || len(o[len(o)-1].Tok) != 32 {
		t.Fatalf("_open %+v", o[len(o)-1])
	}
	return o[len(o)-1]
}

// pollReq serves one poll request on the asset origin.
func pollReq(ctx context.Context, a *App, s *Surface, gen uint64, tok string, ack uint64) *httptest.ResponseRecorder {
	q := fmt.Sprintf("?gen=%d&tok=%s&ack=%d", gen, tok, ack)
	r := httptest.NewRequestWithContext(ctx, "GET", a.assetOrigin()+lanePathPrefix+"poll"+q, nil)
	w := httptest.NewRecorder()
	host{a}.ServeAsset(s.ID(), w, r)
	return w
}

// goPollReq starts pollReq on its own goroutine.
func goPollReq(a *App, s *Surface, gen uint64, tok string, ack uint64) <-chan *httptest.ResponseRecorder {
	ch := make(chan *httptest.ResponseRecorder, 1)
	go func() { ch <- pollReq(context.Background(), a, s, gen, tok, ack) }()
	return ch
}

func awaitPoll(t *testing.T, ch <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case w := <-ch:
		return w
	case <-time.After(timeout):
		t.Fatal("poll did not end")
		return nil
	}
}

func batchItems(t *testing.T, w *httptest.ResponseRecorder) []downq.Item {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("poll: status %d %q", w.Code, w.Body.String())
	}
	_, items, err := downq.DecodeBatch(w.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// waitActive waits until the surface's downstream queue polls.
func waitActive(t *testing.T, s *Surface, want bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for s.downq.Active() != want {
		if time.Now().After(deadline) {
			t.Fatalf("Active never became %v", want)
		}
		time.Sleep(time.Millisecond)
	}
}

// activate makes the page's first poll and leaves it held; send the next
// item to end it.
func activate(t *testing.T, a *App, s *Surface, o openJSON) <-chan *httptest.ResponseRecorder {
	t.Helper()
	ch := goPollReq(a, s, o.Gen, o.Tok, 0)
	waitActive(t, s, true)
	return ch
}

func downOp(format string, args ...any) string {
	return corehook.InternalPrefix + downChannel + ":" + fmt.Sprintf(format, args...)
}

func wantRecvSeq(gen uint64, items string) string {
	return "window.__tango._recvSeq(" + strconv.FormatUint(gen, 10) + "," + items + ")"
}

func wantRelease(gen, id uint64) string {
	return fmt.Sprintf("window.__tango._release(%d,%d)", gen, id)
}

// blockMain occupies the main thread until the returned function is called.
func blockMain(a *App) (release func()) {
	in, out := make(chan struct{}), make(chan struct{})
	a.Post(func() { close(in); <-out })
	<-in
	return func() { close(out) }
}

func TestSendFallsBackBeforePoll(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	n := len(ns.Evals())
	// Off the main thread, one round of Sends makes one Eval.
	release := blockMain(a)
	s.Send("a")
	s.Send("b")
	release()
	sync0(a)
	ev := ns.Evals()[n:]
	if want := wantRecvSeq(o.Gen, `[[1,0,"a"],[2,0,"b"]]`); len(ev) != 1 || ev[0] != want {
		t.Fatalf("evals %q, want [%q]", ev, want)
	}
}

func TestSendUsesPollWhenActive(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ch := activate(t, a, s, o)
	n := len(ns.Evals())
	s.Send("hello")
	items := batchItems(t, awaitPoll(t, ch))
	if len(items) != 1 || items[0].Seq != 1 || items[0].Kind != downq.Msg || string(items[0].Data) != "hello" {
		t.Fatalf("batch %+v", items)
	}
	sync0(a)
	if ev := ns.Evals()[n:]; len(ev) != 0 {
		t.Fatalf("evals while polling: %q", ev)
	}
}

// TestSendIdleRule: while the page's poll request waits on an idle lane, a
// small Send and an Eval take the fallback (one Eval of _recvSeq; the
// barrier's script runs at its place, with no page round trip) until
// IdleEvalItems are waiting for the main thread; Sends past the cap take
// the poll (the size limit: internal/downq's TestIdleRule).
func TestSendIdleRule(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	setDownVar(t, &downq.IdleEvalItems, 2)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ch := activate(t, a, s, o)
	n := len(ns.Evals())
	release := blockMain(a)
	s.Send("a")
	s.Eval("doThing()")
	s.Send("b") // the cap is reached: it ends the poll
	items := batchItems(t, awaitPoll(t, ch))
	release()
	sync0(a)
	if len(items) != 1 || items[0].Seq != 3 || string(items[0].Data) != "b" {
		t.Fatalf("batch %+v", items)
	}
	id := lastBarrier(s)
	want := []string{wantRecvSeq(o.Gen, `[[1,0,"a"]]`), "doThing()", wantRecvSeq(o.Gen, fmt.Sprintf(`[[2,2,%d]]`, id))}
	if ev := ns.Evals()[n:]; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want %q", ev, want)
	}

}

// expectNoDown checks that the loaded surface's page got no _open and that
// Send goes by _recvSeq.
func expectNoDown(t *testing.T, a *App, s *Surface, ns *fake.Surface) {
	t.Helper()
	if o := opens(t, ns.Evals()); len(o) != 0 {
		t.Fatalf("_open sent: %+v", o)
	}
	n := len(ns.Evals())
	Await(a, func() int { s.Send("x"); return 0 })
	sync0(a)
	ev := ns.Evals()[n:]
	if len(ev) != 1 || ev[0] != wantRecvSeq(1, `[[1,0,"x"]]`) {
		t.Fatalf("evals %q", ev)
	}
	if s.downq.Active() {
		t.Fatal("active")
	}
}

func TestDownDisabledWithoutCap(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.UpPost)
	fb.Load(s.ID())
	expectNoDown(t, a, s, ns)
}

func TestDownDisabledByKillSwitch(t *testing.T) {
	t.Run("env", func(t *testing.T) {
		t.Setenv("TANGO_IPC_LANES", "0")
		a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
		fb.Load(s.ID())
		expectNoDown(t, a, s, ns)
	})
	t.Run("option", func(t *testing.T) {
		a, fb, s, ns := downApp(t, Options{DisableFastLanes: true}, "/", lane.DownPoll)
		fb.Load(s.ID())
		expectNoDown(t, a, s, ns)
	})
}

func TestDownDisabledForForeignOrigin(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "http://localhost:5173/", lane.DownPoll)
	fb.Load(s.ID())
	expectNoDown(t, a, s, ns)
}

func TestEvalBarrierWhenActive(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ch := activate(t, a, s, o)
	n := len(ns.Evals())
	s.Eval("doThing()")
	items := batchItems(t, awaitPoll(t, ch))
	if len(items) != 1 || items[0].Kind != downq.Barrier || len(items[0].Data) != 8 {
		t.Fatalf("batch %+v", items)
	}
	sync0(a)
	if ev := ns.Evals()[n:]; len(ev) != 0 {
		t.Fatalf("script ran before the page reached the barrier: %q", ev)
	}
	id := itemBarrierID(items[0])
	fb.Message(s.ID(), downOp(`{"op":"barrier","gen":%d,"id":%d}`, o.Gen, id))
	if ev, want := ns.Evals()[n:], []string{"doThing()", wantRelease(o.Gen, id)}; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want %q", ev, want)
	}
	// A repeated report only releases again.
	fb.Message(s.ID(), downOp(`{"op":"barrier","gen":%d,"id":%d}`, o.Gen, id))
	if ev, want := ns.Evals()[n:], []string{"doThing()", wantRelease(o.Gen, id), wantRelease(o.Gen, id)}; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want %q", ev, want)
	}
}

func itemBarrierID(it downq.Item) uint64 {
	var id uint64
	for _, b := range it.Data {
		id = id<<8 | uint64(b)
	}
	return id
}

func TestEvalSequencedWhenInactive(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	n := len(ns.Evals())
	s.Eval("plain()")
	sync0(a)
	// Sequenced: the script runs at its place, then the page moves past
	// its barrier item.
	id := lastBarrier(s)
	if ev, want := ns.Evals()[n:], []string{"plain()", wantRecvSeq(o.Gen, fmt.Sprintf("[[1,2,%d]]", id))}; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want %q", ev, want)
	}
	// On the main thread it has run when Eval returns.
	n = len(ns.Evals())
	if ev := Await(a, func() []string { s.Eval("now()"); return ns.Evals()[n:] }); len(ev) == 0 || ev[0] != "now()" {
		t.Fatalf("evals %q", ev)
	}
	// Before a page has loaded it runs as is.
	s.Navigate("/next.html")
	n = len(ns.Evals())
	s.Eval("early()")
	sync0(a)
	if ev := ns.Evals()[n:]; !slices.Equal(ev, []string{"early()"}) {
		t.Fatalf("evals %q", ev)
	}
}

// An Eval off the main thread before the first poll, then a Send that the
// poll carries: the page gets the script's barrier at seq 1 (by Eval, with
// the script run there) and the message at seq 2, so js runs first even if
// the poll response is handled before the Eval.
func TestEvalBeforeSwitchStaysFirst(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	n := len(ns.Evals())
	release := blockMain(a)
	s.Eval("first()")
	ch := activate(t, a, s, o)
	s.Send("m")
	items := batchItems(t, awaitPoll(t, ch))
	release()
	sync0(a)
	if len(items) != 1 || items[0].Seq != 2 || string(items[0].Data) != "m" {
		t.Fatalf("batch %+v", items)
	}
	id := lastBarrier(s)
	if ev, want := ns.Evals()[n:], []string{"first()", wantRecvSeq(o.Gen, fmt.Sprintf("[[1,2,%d]]", id))}; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want %q", ev, want)
	}
}

// A Send on the main thread reaches the page before a Navigate that
// follows it.
func TestMainThreadSendBeforeNavigate(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	Await(a, func() int { s.Send("m"); s.Navigate("/next.html"); return 0 })
	sync0(a)
	if ev := ns.Evals(); !slices.Contains(ev, wantRecvSeq(o.Gen, `[[1,0,"m"]]`)) {
		t.Fatalf("evals %q", ev)
	}
}

func TestNavigateEndsGeneration(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ch := activate(t, a, s, o)
	fb.Navigating(s.ID(), "tango://app/next.html")
	if w := awaitPoll(t, ch); w.Code != http.StatusGone {
		t.Fatalf("held poll: status %d, want 410", w.Code)
	}
	if s.downq.Active() {
		t.Fatal("active after SurfaceNavigating")
	}
	fb.Load(s.ID())
	o2 := lastOpen(t, ns)
	if o2.Gen <= o.Gen || o2.Tok == o.Tok {
		t.Fatalf("_open %+v after %+v", o2, o)
	}

	// Surface.Navigate ends it too.
	ch = activate(t, a, s, o2)
	s.Navigate("/other.html")
	if w := awaitPoll(t, ch); w.Code != http.StatusGone {
		t.Fatalf("held poll after Navigate: status %d, want 410", w.Code)
	}
	fb.Load(s.ID())
	if o3 := lastOpen(t, ns); o3.Gen <= o2.Gen {
		t.Fatalf("_open %+v after %+v", o3, o2)
	}
}

func TestPendingBeforeLoadDelivered(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	s.Send("a")
	sync0(a)
	if ev := ns.Evals(); len(ev) != 0 {
		t.Fatalf("evals before Loaded: %q", ev)
	}
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ev := ns.Evals()
	if len(ev) != 2 || ev[1] != wantRecvSeq(o.Gen, `[[1,0,"a"]]`) {
		t.Fatalf("evals %q", ev)
	}
}

func TestFailInternalMessage(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ch := activate(t, a, s, o)
	Await(a, func() int { s.Send("a"); s.Send("b"); return 0 })
	awaitPoll(t, ch) // may end after "a" alone
	// The page polls again without acknowledging: both are written.
	if items := batchItems(t, pollReq(context.Background(), a, s, o.Gen, o.Tok, 0)); len(items) != 2 {
		t.Fatalf("batch %+v", items)
	}
	n := len(ns.Evals())
	fb.Message(s.ID(), downOp(`{"op":"fail","gen":%d,"acked":1}`, o.Gen))
	sync0(a)
	if ev := ns.Evals()[n:]; !slices.Equal(ev, []string{wantRecvSeq(o.Gen, `[[2,0,"b"]]`)}) {
		t.Fatalf("evals %q", ev)
	}
	if s.downq.Active() {
		t.Fatal("active after fail")
	}
}

func TestBarrierRunsOnceWhenFallbackWins(t *testing.T) {
	setDownVar(t, &downq.WatchdogAfter, 20*time.Millisecond)
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ch := activate(t, a, s, o)
	n := len(ns.Evals())
	s.Eval("once()")
	items := batchItems(t, awaitPoll(t, ch)) // the barrier, unacknowledged
	id := itemBarrierID(items[0])
	s.Send("after") // no poll comes: the watchdog hands both to the fallback
	want := []string{
		"once()",
		wantRecvSeq(o.Gen, fmt.Sprintf(`[[1,2,%d],[2,0,"after"]]`, id)),
	}
	deadline := time.Now().Add(timeout)
	for len(ns.Evals()) < n+len(want) {
		if time.Now().After(deadline) {
			t.Fatalf("evals %q", ns.Evals()[n:])
		}
		time.Sleep(time.Millisecond)
	}
	sync0(a)
	if ev := ns.Evals()[n:]; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want %q", ev, want)
	}
	// The page, paused on the barrier from the poll, reports it late.
	fb.Message(s.ID(), downOp(`{"op":"barrier","gen":%d,"id":%d}`, o.Gen, id))
	want = append(want, wantRelease(o.Gen, id))
	if ev := ns.Evals()[n:]; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want %q", ev, want)
	}
}

func TestBarrierScriptErrorStillReleases(t *testing.T) {
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	ch := activate(t, a, s, o)
	const bad = "this is not ((( javascript"
	s.Eval(bad)
	id := itemBarrierID(batchItems(t, awaitPoll(t, ch))[0])
	n := len(ns.Evals())
	fb.Message(s.ID(), downOp(`{"op":"barrier","gen":%d,"id":%d}`, o.Gen, id))
	if ev, want := ns.Evals()[n:], []string{bad, wantRelease(o.Gen, id)}; !slices.Equal(ev, want) {
		t.Fatalf("evals %q, want the script and then the release, separately: %q", ev, want)
	}
}

var recvSeqRE = regexp.MustCompile(`^window\.__tango\._recvSeq\((\d+),(.*)\)$`)

func TestSendOrderFromGoroutine(t *testing.T) {
	const total = 1000
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)

	var mu sync.Mutex
	var polled []downq.Item
	ctx, cancel := context.WithCancel(context.Background())
	startPoll := make(chan struct{})
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		<-startPoll
		var ack uint64
		for ack < total && ctx.Err() == nil {
			w := pollReq(ctx, a, s, o.Gen, o.Tok, ack)
			if ctx.Err() != nil || w.Code != http.StatusOK {
				return
			}
			_, items, err := downq.DecodeBatch(w.Body.Bytes())
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			polled = append(polled, items...)
			mu.Unlock()
			if len(items) > 0 {
				ack = max(ack, items[len(items)-1].Seq)
			}
		}
	}()
	defer func() { cancel(); <-pollDone }()

	sent := make(chan struct{})
	go func() {
		defer close(sent)
		for i := range total {
			switch i {
			case total / 2:
				close(startPoll) // the page starts polling mid-way
			case total * 3 / 4:
				// At the latest, the rest is polled.
				for deadline := time.Now().Add(timeout); !s.downq.Active() && time.Now().Before(deadline); {
					time.Sleep(time.Millisecond)
				}
			}
			s.Send("m" + strconv.Itoa(i))
		}
	}()
	<-sent

	// collect merges what came by Eval and by polling: seq -> payload.
	collect := func() map[uint64]string {
		got := map[uint64]string{}
		put := func(seq uint64, data string) {
			if prev, ok := got[seq]; ok && prev != data {
				t.Fatalf("seq %d: %q and %q", seq, prev, data)
			}
			got[seq] = data
		}
		mu.Lock()
		for _, it := range polled {
			put(it.Seq, string(it.Data))
		}
		mu.Unlock()
		var last uint64
		for _, e := range ns.Evals() {
			m := recvSeqRE.FindStringSubmatch(e)
			if m == nil {
				continue
			}
			if m[1] != strconv.FormatUint(o.Gen, 10) {
				t.Fatalf("eval of another generation: %q", e)
			}
			var list [][]any
			if err := json.Unmarshal([]byte(m[2]), &list); err != nil {
				t.Fatal(err)
			}
			for _, it := range list {
				seq := uint64(it[0].(float64))
				if seq <= last {
					t.Fatalf("fallback seq %d after %d", seq, last)
				}
				last = seq
				put(seq, it[2].(string))
			}
		}
		return got
	}
	deadline := time.Now().Add(timeout)
	got := collect()
	for len(got) < total && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		sync0(a)
		got = collect()
	}
	for seq := uint64(1); seq <= total; seq++ {
		if want := "m" + strconv.FormatUint(seq-1, 10); got[seq] != want {
			t.Fatalf("seq %d = %q, want %q", seq, got[seq], want)
		}
	}
	if len(got) != total {
		t.Fatalf("%d seqs", len(got))
	}
	mu.Lock()
	n := len(polled)
	mu.Unlock()
	if n < total/4 {
		t.Fatalf("only %d items polled", n)
	}
}

// TestFallbackEvalSizeCap: one round of the fallback holding ~3 MiB of
// small messages goes in several _recvSeq Evals of at most recvSeqMaxBytes
// of items each (an item over the cap goes alone), seqs contiguous and in
// order across them.
func TestFallbackEvalSizeCap(t *testing.T) {
	const total, size = 3000, 1 << 10
	a, fb, s, ns := downApp(t, Options{}, "/", lane.DownPoll)
	fb.Load(s.ID())
	o := lastOpen(t, ns)
	n := len(ns.Evals())
	msg := strings.Repeat("x", size)
	release := blockMain(a)
	for range total {
		s.Send(msg)
	}
	big := strings.Repeat("y", recvSeqMaxBytes+1)
	s.Send(big)
	s.Send("tail")
	release()
	sync0(a)
	var seqs []uint64
	calls := 0
	for _, e := range ns.Evals()[n:] {
		m := recvSeqRE.FindStringSubmatch(e)
		if m == nil {
			t.Fatalf("eval %.80q", e)
		}
		if m[1] != strconv.FormatUint(o.Gen, 10) {
			t.Fatalf("eval of generation %s", m[1])
		}
		calls++
		var list [][]any
		if err := json.Unmarshal([]byte(m[2]), &list); err != nil {
			t.Fatal(err)
		}
		if len(m[2]) > recvSeqMaxBytes+len("[]") && len(list) > 1 {
			t.Fatalf("eval of %d bytes holds %d items", len(m[2]), len(list))
		}
		for _, it := range list {
			seqs = append(seqs, uint64(it[0].(float64)))
		}
	}
	if calls < 4 {
		t.Fatalf("%d evals, want at least 4", calls)
	}
	if len(seqs) != total+2 {
		t.Fatalf("%d items, want %d", len(seqs), total+2)
	}
	for i, seq := range seqs {
		if seq != uint64(i+1) {
			t.Fatalf("item %d: seq %d", i, seq)
		}
	}
	if got := sentMsgs(t, ns.Evals()[n:]); got[total] != big || got[total+1] != "tail" {
		t.Fatal("big or tail message wrong")
	}
}

// sentMsgs returns the messages the fallback delivered by Eval
// (_recvSeq kind 0 items), in order.
func sentMsgs(t *testing.T, evals []string) []string {
	t.Helper()
	var out []string
	for _, e := range evals {
		m := recvSeqRE.FindStringSubmatch(e)
		if m == nil {
			continue
		}
		var list [][]any
		if err := json.Unmarshal([]byte(m[2]), &list); err != nil {
			t.Fatalf("eval %q: %v", e, err)
		}
		for _, it := range list {
			if it[1].(float64) == 0 {
				out = append(out, it[2].(string))
			}
		}
	}
	return out
}

// lastBarrier returns the last barrier id given out.
func lastBarrier(s *Surface) uint64 {
	s.down.mu.Lock()
	defer s.down.mu.Unlock()
	return s.down.nextID
}

func setDownVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}
