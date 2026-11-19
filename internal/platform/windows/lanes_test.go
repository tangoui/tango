package windows

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/internal/lane/shmring"
)

func TestLaneCapsFromEnvWindows(t *testing.T) {
	for _, tt := range []struct {
		value     string
		supported bool
		want      []string
	}{
		{"", true, nil},
		{"1", true, []string{"down:shm", "up:shm"}},
		{"1", false, nil},
		{" OFF ", true, nil},
		{"false", true, nil},
		{"0", true, nil},
		{"on", true, []string{"down:shm", "up:shm"}},
	} {
		getenv := func(key string) string {
			if key == "TANGO_IPC_LANES_WINDOWS" {
				return tt.value
			}
			return ""
		}
		if got := laneCapsFromEnv(getenv, tt.supported); !slices.Equal(got, tt.want) {
			t.Errorf("value %q, supported %v: got %v, want %v", tt.value, tt.supported, got, tt.want)
		}
	}
}

func TestShmCapacity(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  int
		warn  bool
	}{
		{"", 4 << 20, false},
		{"65536", 64 << 10, false},
		{"8388608", 8 << 20, false},
		{"32768", 4 << 20, true},
		{"65537", 4 << 20, true},
		{"-65536", 4 << 20, true},
		{"4294967296", 4 << 20, true},
		{"bad", 4 << 20, true},
	} {
		getenv := func(key string) string {
			if key == "TANGO_IPC_SHM_BYTES" {
				return tt.value
			}
			return ""
		}
		var warnings []string
		warn := func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }
		if got := shmCapacityFromEnv(getenv, warn); got != tt.want {
			t.Errorf("value %q: got %d, want %d", tt.value, got, tt.want)
		}
		if tt.warn {
			if len(warnings) != 1 || !strings.Contains(warnings[0], "TANGO_IPC_SHM_BYTES") {
				t.Errorf("value %q: warnings %q, want one mentioning the variable", tt.value, warnings)
			}
		} else if len(warnings) != 0 {
			t.Errorf("value %q: unexpected warnings %q", tt.value, warnings)
		}
	}
}

type shmTestPage struct {
	t         *testing.T
	lanes     *shmLanes
	mu        sync.Mutex
	ui        []func()
	created   int
	released  int
	posts     int
	rings     map[string]*shmring.Ring
	mappings  map[string][]byte
	started   []*shmGeneration
	wakes     []string
	onCreate  func()
	onPost    func(string, *shmring.Ring) bool
	delivered chan []byte
}

func newShmTestPage(t *testing.T, deliver func(uint64, []byte) bool) *shmTestPage {
	t.Helper()
	p := &shmTestPage{
		t: t, rings: make(map[string]*shmring.Ring), mappings: make(map[string][]byte),
		delivered: make(chan []byte, 8),
	}
	if deliver == nil {
		deliver = func(_ uint64, packet []byte) bool { p.delivered <- packet; return true }
	}
	p.lanes = newShmLanes(64<<10, shmOps{
		create: func(size int) ([]byte, func(), error) {
			if size != 64<<10+64 {
				t.Errorf("mapping size = %d", size)
			}
			p.created++
			if p.onCreate != nil {
				p.onCreate()
			}
			return make([]byte, size), func() { p.released++ }, nil
		},
		post: func(ring string, gen uint64, buf any) bool {
			p.posts++
			mem := buf.([]byte)
			r, err := shmring.Open(mem, gen)
			if err != nil {
				t.Fatal(err)
			}
			p.rings[ring] = r
			p.mappings[ring] = mem
			return p.onPost == nil || p.onPost(ring, r)
		},
		message: func(msg string) { p.wakes = append(p.wakes, msg) },
		onUI: func(fn func()) {
			p.mu.Lock()
			p.ui = append(p.ui, fn)
			p.mu.Unlock()
		},
		deliver: deliver,
	})
	p.lanes.documentLoaded(assetOrigin)
	t.Cleanup(func() {
		p.lanes.close()
		for _, g := range p.started {
			waitShmExit(t, g.downDone)
			waitShmExit(t, g.upDone)
		}
		p.drainUI()
	})
	return p
}

func (p *shmTestPage) drainUI() {
	p.mu.Lock()
	ui := p.ui
	p.ui = nil
	p.mu.Unlock()
	for _, fn := range ui {
		fn()
	}
}

func (p *shmTestPage) start(gen uint64) *shmGeneration {
	p.t.Helper()
	p.lanes.LaneGeneration(gen, []string{lane.DownShm, lane.UpShm})
	p.drainUI()
	g := p.lanes.current
	if g == nil || g.gen != gen || p.rings["down"] == nil || p.rings["up"] == nil {
		p.t.Fatalf("generation %d did not post both rings", gen)
	}
	p.started = append(p.started, g)
	return g
}

func (p *shmTestPage) wake(origin, ring string, gen uint64) bool {
	msg := fmt.Sprintf("\x00tango-int:lane:"+`{"op":"wake","ring":%q,"gen":%d}`, ring, gen)
	return p.lanes.handleWake(origin, msg)
}

func waitShmExit(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("worker still running after 100 ms")
	}
}

func (p *shmTestPage) waitForWake(ring string, gen uint64) {
	p.t.Helper()
	want := fmt.Sprintf(`{"tango":"lane","op":"wake","ring":%q,"gen":%d}`, ring, gen)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.drainUI()
		if slices.Contains(p.wakes, want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	p.t.Fatalf("wakes = %q, want %s", p.wakes, want)
}

func TestShmPostSkippedAfterNavigation(t *testing.T) {
	t.Run("before UI callback", func(t *testing.T) {
		p := newShmTestPage(t, nil)
		p.lanes.LaneGeneration(1, []string{lane.DownShm, lane.UpShm})
		p.lanes.navigationStarting()
		p.lanes.documentLoaded(assetOrigin)
		p.drainUI()
		if p.posts != 0 || p.created != 0 || p.released != 0 {
			t.Fatalf("stale UI callback created %d, posted %d, released %d", p.created, p.posts, p.released)
		}
	})
	t.Run("during allocation", func(t *testing.T) {
		p := newShmTestPage(t, nil)
		p.onCreate = func() {
			p.lanes.navigationStarting()
			p.lanes.documentLoaded(assetOrigin)
		}
		p.lanes.LaneGeneration(1, []string{lane.DownShm, lane.UpShm})
		p.drainUI()
		if p.posts != 0 || p.created == 0 || p.created != p.released {
			t.Fatalf("stale allocation created %d, posted %d, released %d", p.created, p.posts, p.released)
		}
	})
}

func (p *shmTestPage) fillDown() *shmGeneration {
	p.t.Helper()
	p.onPost = func(name string, ring *shmring.Ring) bool {
		if name == "down" && ring.Sleep() {
			p.t.Fatal("new down ring is nonempty")
		}
		return true
	}
	g := p.start(1)
	packet := make([]byte, 32764) // exactly two records fit including their headers
	if !p.lanes.SendLanePacket(1, packet) || !p.lanes.SendLanePacket(1, packet) {
		p.t.Fatal("could not queue packets to fill down ring")
	}
	p.waitForWake("down", 1)
	return g
}

func TestShmQueueDroppedOnNavigation(t *testing.T) {
	p := newShmTestPage(t, nil)
	g := p.fillDown()
	if !p.lanes.SendLanePacket(1, []byte("old")) {
		t.Fatal("could not queue the old packet")
	}
	p.waitForDownWriter()
	p.lanes.navigationStarting()
	waitShmExit(t, g.downDone)
	waitShmExit(t, g.upDone)
	if p.released != 2 {
		t.Fatalf("old mappings released = %d, want 2", p.released)
	}
	p.lanes.documentLoaded(assetOrigin)
	p.start(2)
	if p.lanes.SendLanePacket(1, []byte("late")) || !p.lanes.SendLanePacket(2, []byte("new")) {
		t.Fatal("down queue admitted the wrong generation")
	}
	p.waitForWake("down", 2)
	var packets []string
	n, _, err := p.rings["down"].Read(func(packet []byte) {
		packets = append(packets, string(packet))
	})
	if n != 1 || err != nil || !slices.Equal(packets, []string{"new"}) {
		t.Fatalf("new ring = %q (%d records, %v)", packets, n, err)
	}
}

func TestShmQueueFullRefuses(t *testing.T) {
	p := newShmTestPage(t, nil)
	p.fillDown()
	packet := make([]byte, 32768)
	// The first two packets may still be counted in the queue when it first
	// fills. Let the writer finish filling the mapping and check the final
	// count without ever giving it back any read space.
	deadline := time.Now().Add(time.Second)
	accepted := 0
	for accepted < 256 && time.Now().Before(deadline) {
		if p.lanes.SendLanePacket(1, packet) {
			accepted++
		} else {
			time.Sleep(time.Millisecond)
		}
	}
	if accepted != 256 || p.lanes.SendLanePacket(1, []byte{1}) {
		t.Fatalf("accepted %d 32 KiB packets before checking the 8 MiB limit", accepted)
	}
	if p.lanes.SendLanePacket(1, make([]byte, 32769)) {
		t.Fatal("oversize packet admitted")
	}
}

func TestShmUpCopiesBeforeDeliver(t *testing.T) {
	p := newShmTestPage(t, nil)
	p.start(1)
	r := p.rings["up"]
	if ok, _ := r.TryWrite([]byte("original")); !ok {
		t.Fatal("page could not write packet")
	}
	p.wake(assetOrigin, "up", 1)
	var packet []byte
	select {
	case packet = <-p.delivered:
	case <-time.After(time.Second):
		t.Fatal("no upstream packet delivered")
	}
	// Fill to the midpoint and then the end, and reuse the original
	// record's bytes at offset zero.
	for _, data := range [][]byte{make([]byte, 32752), make([]byte, 32764), []byte("replaced")} {
		if ok, _ := r.TryWrite(data); !ok {
			t.Fatal("page could not reuse the upstream mapping")
		}
	}
	if !bytes.Equal(packet, []byte("original")) {
		t.Fatalf("retained packet changed to %q", packet)
	}
}

func TestShmWakeRejectsOtherOrigin(t *testing.T) {
	p := newShmTestPage(t, nil)
	p.start(1)
	if ok, _ := p.rings["up"].TryWrite([]byte("payload")); !ok {
		t.Fatal("page could not write packet")
	}
	if !p.wake("https://elsewhere.invalid", "up", 1) || !p.wake(assetOrigin, "up", 2) {
		t.Fatal("invalid wake was forwarded to the core")
	}
	if p.lanes.handleWake(assetOrigin, "\x00tango-int:lane:"+`{"op":"ready","lane":"up:shm","gen":1}`) {
		t.Fatal("core's ready control message was intercepted")
	}
	select {
	case <-p.delivered:
		t.Fatal("wrong origin or generation woke the reader")
	case <-time.After(20 * time.Millisecond):
	}
	p.wake(assetOrigin, "up", 1)
	select {
	case packet := <-p.delivered:
		if string(packet) != "payload" {
			t.Fatalf("delivered %q", packet)
		}
	case <-time.After(time.Second):
		t.Fatal("the current document's wake was ignored")
	}
}

func TestShmUpStopsAfterCoreRefuses(t *testing.T) {
	calls := make(chan string, 3)
	p := newShmTestPage(t, func(_ uint64, packet []byte) bool {
		calls <- string(packet)
		return false
	})
	g := p.start(1)
	for _, packet := range []string{"refused", "later"} {
		if ok, _ := p.rings["up"].TryWrite([]byte(packet)); !ok {
			t.Fatal("page could not write packet")
		}
	}
	p.wake(assetOrigin, "up", 1)
	waitShmExit(t, g.upDone)
	if len(calls) != 1 || <-calls != "refused" {
		t.Fatal("reader kept delivering after the core refused a packet")
	}
	p.drainUI()
	if want := `{"tango":"lane","op":"fail","ring":"up","gen":1}`; !slices.Equal(p.wakes, []string{want}) {
		t.Fatalf("messages = %q, want only %s", p.wakes, want)
	}
	p.rings["down"].Sleep()
	if !p.lanes.SendLanePacket(1, []byte("down still works")) {
		t.Fatal("upstream failure stopped the downstream ring")
	}
	p.waitForWake("down", 1)
}

func TestShmUpFailureSkippedAfterNavigation(t *testing.T) {
	p := newShmTestPage(t, func(uint64, []byte) bool { return false })
	g := p.start(1)
	if ok, _ := p.rings["up"].TryWrite([]byte("refused")); !ok {
		t.Fatal("page could not write packet")
	}
	p.wake(assetOrigin, "up", 1)
	waitShmExit(t, g.upDone)
	p.lanes.navigationStarting()
	p.lanes.documentLoaded(assetOrigin)
	p.start(2)
	if len(p.wakes) != 0 {
		t.Fatalf("new document got messages from the old generation: %q", p.wakes)
	}
}

func (p *shmTestPage) waitForDownWriter() {
	p.t.Helper()
	// Header byte 12 is writerWaiting. Wait for the write attempt before
	// the fake page frees any space.
	waiting := (*uint32)(unsafe.Pointer(&p.mappings["down"][12]))
	deadline := time.Now().Add(time.Second)
	for atomic.LoadUint32(waiting) == 0 {
		if time.Now().After(deadline) {
			p.t.Fatal("writer did not wait for space")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestShmFullRingWaitsForPageWake(t *testing.T) {
	p := newShmTestPage(t, nil)
	p.fillDown()
	if !p.lanes.SendLanePacket(1, []byte("queued")) {
		t.Fatal("could not queue the packet")
	}
	p.waitForDownWriter()
	p.drainUI()
	r := p.rings["down"]
	r.Woken()
	n, wake, err := r.Read(func([]byte) {})
	if n != 2 || !wake || err != nil {
		t.Fatalf("full ring read: n=%d, wake=%v, err=%v", n, wake, err)
	}
	p.wakes = nil
	if r.Sleep() {
		t.Fatal("record published before page wake")
	}
	if !p.lanes.SendLanePacket(1, []byte("later")) {
		t.Fatal("could not enqueue another packet")
	}
	time.Sleep(20 * time.Millisecond)
	p.drainUI()
	if len(p.wakes) != 0 || r.Sleep() {
		t.Fatal("enqueue alone woke a writer waiting for the page")
	}
	p.wake(assetOrigin, "down", 1)
	p.waitForWake("down", 1)
}

func TestShmUpStopsAfterReadError(t *testing.T) {
	p := newShmTestPage(t, nil)
	g := p.start(1)
	// The page damaged the ring's magic. It may corrupt the writable
	// mapping regardless of whether it followed the SPSC protocol.
	clear(p.mappings["up"][24:28])
	p.wake(assetOrigin, "up", 1)
	waitShmExit(t, g.upDone)
	p.drainUI()
	if want := `{"tango":"lane","op":"fail","ring":"up","gen":1}`; len(p.delivered) != 0 || !slices.Equal(p.wakes, []string{want}) {
		t.Fatalf("corrupt ring caused %d deliveries and messages %q, want only %s", len(p.delivered), p.wakes, want)
	}
}

func TestShmDownWakesForWrapMarker(t *testing.T) {
	p := newShmTestPage(t, nil)
	p.onPost = func(name string, r *shmring.Ring) bool {
		if name != "down" {
			return true
		}
		// Prime the fake mapping before the worker starts. Subsequent
		// writes begin halfway through an empty ring.
		ok, _ := r.TryWrite(make([]byte, 32764))
		n, _, err := r.Read(func([]byte) {})
		if !ok || n != 1 || err != nil || r.Sleep() {
			t.Fatal("could not position reader and writer at the midpoint")
		}
		return true
	}
	p.start(1)
	want := bytes.Repeat([]byte{0x76}, 32768)
	packet := bytes.Clone(want)
	if !p.lanes.SendLanePacket(1, packet) {
		t.Fatal("could not queue the largest packet")
	}
	clear(packet)
	p.waitForWake("down", 1)
	r := p.rings["down"]
	n, wake, err := r.Read(func([]byte) {})
	if n != 0 || !wake || err != nil || r.Sleep() {
		t.Fatalf("wrap-only read: n=%d, wake=%v, err=%v", n, wake, err)
	}
	p.wakes = nil
	p.wake(assetOrigin, "down", 1)
	p.waitForWake("down", 1)
	n, _, err = r.Read(func(packet []byte) {
		if !bytes.Equal(packet, want) {
			t.Fatal("down packet changed after enqueue")
		}
	})
	if n != 1 || err != nil {
		t.Fatalf("retry read: n=%d, err=%v", n, err)
	}
}
