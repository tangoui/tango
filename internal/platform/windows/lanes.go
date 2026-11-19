package windows

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/lane"
	"github.com/tangoui/tango/internal/lane/shmring"
)

const (
	shmSwitchEnv = "TANGO_IPC_LANES_WINDOWS"
	shmBytesEnv  = "TANGO_IPC_SHM_BYTES"
	shmDefault   = true
	shmCapacity  = 4 << 20 // data bytes, in addition to the shared header
	shmQueueMax  = 8 << 20 // unsent down bytes per surface

	shmLanePrefix = corehook.InternalPrefix + "lane:"
)

// laneCapsFromEnv enables both shared-buffer lanes by default on a
// supported WebView2. TANGO_IPC_LANES_WINDOWS can disable them together.
func laneCapsFromEnv(getenv func(string) string, supported bool) []string {
	if !supported {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(getenv(shmSwitchEnv))) {
	case "0", "false", "off":
		return nil
	case "":
		if !shmDefault {
			return nil
		}
	}
	return []string{lane.DownShm, lane.UpShm}
}

func shmCapacityFromEnv(getenv func(string) string, warn func(string, ...any)) int {
	value := strings.TrimSpace(getenv(shmBytesEnv))
	if value == "" {
		return shmCapacity
	}
	size, err := strconv.ParseUint(value, 10, 32)
	// The ring's 32-bit cursors can represent at most 2^31 data bytes.
	// Also leave room for the header in a Go slice on the host.
	if err != nil || size < 64<<10 || size > 1<<31 || size&(size-1) != 0 ||
		size+shmring.HeaderSize > uint64(int(^uint(0)>>1)) {
		warn("tango: invalid %s=%q, using %d data bytes", shmBytesEnv, value, shmCapacity)
		return shmCapacity
	}
	return int(size)
}

type shmOps struct {
	create  func(size int) (mem []byte, release func(), err error)
	post    func(ring string, gen uint64, buf any) bool
	message func(msg string)
	onUI    func(fn func())
	deliver func(gen uint64, p []byte) bool
}

// shmLanes owns the shared mappings for one WebView. Navigation, posting
// and releasing mappings run on the UI thread. mu guards document and
// generation state plus the send queue. It is never held across COM calls,
// accesses to mapped payloads, or callbacks into the host.
type shmLanes struct {
	ops      shmOps
	capacity int

	mu      sync.Mutex
	doc     uint64
	origin  string
	latest  uint64
	closed  bool
	current *shmGeneration
}

type shmGeneration struct {
	gen  uint64
	stop chan struct{}

	// The UI thread can release the mappings as soon as all ring accesses
	// finish. A host callback holding a packet copy never pins a mapping.
	mapped  sync.RWMutex
	release func()
	down    *shmring.Ring
	up      *shmring.Ring

	downDone  chan struct{}
	upDone    chan struct{}
	downReady chan struct{} // queued work
	downSpace chan struct{} // space freed by the page
	upReady   chan struct{} // page published upstream records

	// Guarded by shmLanes.mu.
	queue       [][]byte
	queuedBytes int
	downWake    bool
	upWake      bool
}

func newShmLanes(capacity int, ops shmOps) *shmLanes {
	return &shmLanes{capacity: capacity, ops: ops}
}

func (s *shmLanes) LaneGeneration(gen uint64, lanes []string) {
	s.mu.Lock()
	if s.closed || gen <= s.latest {
		s.mu.Unlock()
		return
	}
	s.latest = gen
	old := s.current
	if old != nil {
		old.cancel()
	}
	doc, origin := s.doc, s.origin
	s.mu.Unlock()

	start := origin == assetOrigin && slices.Contains(lanes, lane.DownShm)
	if old != nil || start {
		s.ops.onUI(func() { s.setUp(doc, gen, start) })
	}
}

// setUp checks the captured document and generation before allocating,
// before each post, and before handing the mappings to the workers.
func (s *shmLanes) setUp(doc, gen uint64, start bool) {
	s.mu.Lock()
	if !s.matches(doc, gen) {
		s.mu.Unlock()
		return
	}
	old := s.current
	s.current = nil
	s.mu.Unlock()
	if old != nil {
		old.closeMappings()
	}
	if !start || !s.canPost(doc, gen) {
		return
	}

	downMem, releaseDown, err := s.ops.create(s.capacity + shmring.HeaderSize)
	if err != nil {
		return
	}
	keep := false
	defer func() {
		if !keep {
			releaseDown()
		}
	}()
	if !s.canPost(doc, gen) {
		return
	}
	upMem, releaseUp, err := s.ops.create(s.capacity + shmring.HeaderSize)
	if err != nil {
		return
	}
	defer func() {
		if !keep {
			releaseUp()
		}
	}()
	down, downErr := shmring.Init(downMem, gen)
	up, upErr := shmring.Init(upMem, gen)
	if downErr != nil || upErr != nil {
		return
	}
	up.Sleep() // arm before posting so the first upstream write wakes Go
	if !s.canPost(doc, gen) || !s.ops.post("down", gen, downMem) ||
		!s.canPost(doc, gen) || !s.ops.post("up", gen, upMem) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.matches(doc, gen) {
		return
	}
	g := &shmGeneration{
		gen: gen, down: down, up: up, stop: make(chan struct{}),
		release:  func() { releaseUp(); releaseDown() },
		downDone: make(chan struct{}), upDone: make(chan struct{}),
		downReady: make(chan struct{}, 1), downSpace: make(chan struct{}, 1),
		upReady: make(chan struct{}, 1),
	}
	s.current = g
	keep = true
	go s.writeDown(g)
	go s.readUp(g)
}

func (s *shmLanes) matches(doc, gen uint64) bool {
	return !s.closed && s.doc == doc && s.latest == gen
}

func (s *shmLanes) canPost(doc, gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.matches(doc, gen) && s.origin == assetOrigin
}

func (s *shmLanes) navigationStarting() { s.endDocument(false) }
func (s *shmLanes) close()              { s.endDocument(true) }

func (s *shmLanes) documentLoaded(origin string) {
	s.mu.Lock()
	s.origin = origin
	s.mu.Unlock()
}

func (s *shmLanes) hasAssetDocument() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.origin == assetOrigin
}

func (s *shmLanes) originForGeneration(gen uint64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil || s.current.gen != gen || s.latest != gen {
		return ""
	}
	return s.origin
}

// endDocument runs on the UI thread, which also owns setup and all COM
// references. A pending setup sees the changed document sequence.
func (s *shmLanes) endDocument(closeSurface bool) {
	s.mu.Lock()
	s.doc++
	s.origin = ""
	s.closed = s.closed || closeSurface
	g := s.current
	s.current = nil
	if g != nil {
		g.cancel()
	}
	s.mu.Unlock()
	if g != nil {
		g.closeMappings()
	}
}

func (g *shmGeneration) cancel() {
	if !shmSignaled(g.stop) {
		close(g.stop)
	}
	g.queue, g.queuedBytes = nil, 0
}

func (g *shmGeneration) closeMappings() {
	g.mapped.Lock()
	g.mapped.Unlock()
	g.release()
}

func shmSignal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func shmSignaled(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (s *shmLanes) SendLanePacket(gen uint64, p []byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.current
	if g == nil || s.latest != gen || g.gen != gen || len(p) > g.down.MaxPacket() ||
		max(1, len(p)) > shmQueueMax-g.queuedBytes {
		return false
	}
	g.queue = append(g.queue, bytes.Clone(p))
	g.queuedBytes += max(1, len(p)) // empty records also occupy a queue entry
	shmSignal(g.downReady)
	return true
}

func (s *shmLanes) writeDown(g *shmGeneration) {
	defer close(g.downDone)
	woken := false
	for {
		s.mu.Lock()
		havePacket := len(g.queue) != 0
		var packet []byte
		if havePacket {
			packet = g.queue[0]
		}
		s.mu.Unlock()

		if havePacket {
			g.mapped.RLock()
			if shmSignaled(g.stop) {
				g.mapped.RUnlock()
				return
			}
			if woken {
				g.down.WriterWoken()
				woken = false
			}
			ok, wake := g.down.TryWrite(packet)
			g.mapped.RUnlock()
			if ok {
				s.mu.Lock()
				if len(g.queue) != 0 { // cancellation may have cleared it
					g.queue[0] = nil
					g.queue = g.queue[1:]
					g.queuedBytes -= max(1, len(packet))
				}
				s.mu.Unlock()
			}
			if wake { // a wrap marker alone also needs a doorbell
				s.wakePage(g, "down")
			}
			if !ok {
				select {
				case <-g.stop:
					return
				case <-g.downSpace:
					woken = true
				}
			}
			continue
		}

		select {
		case <-g.stop:
			return
		case <-g.downReady:
		}
	}
}

func (s *shmLanes) readUp(g *shmGeneration) {
	defer close(g.upDone)
	for {
		select {
		case <-g.stop:
			return
		case <-g.upReady:
		}
		for {
			if !s.drainUp(g) {
				s.failUp(g)
				return
			}
			g.mapped.RLock()
			if shmSignaled(g.stop) {
				g.mapped.RUnlock()
				return
			}
			again := g.up.Sleep()
			g.mapped.RUnlock()
			if !again {
				break
			}
		}
	}
}

// drainUp reads one snapshot of the write cursor. Read releases each
// record before returning, so deliver only sees copies. The UI can then
// close the mapping while the copies are being delivered.
func (s *shmLanes) drainUp(g *shmGeneration) bool {
	g.mapped.RLock()
	if shmSignaled(g.stop) {
		g.mapped.RUnlock()
		return false
	}
	g.up.Woken()
	var packets [][]byte
	_, wake, err := g.up.Read(func(p []byte) {
		if !shmSignaled(g.stop) {
			packets = append(packets, bytes.Clone(p))
		}
	})
	g.mapped.RUnlock()

	for _, p := range packets {
		if shmSignaled(g.stop) || !s.ops.deliver(g.gen, p) {
			return false
		}
	}
	if err != nil {
		return false
	}
	if wake {
		s.wakePage(g, "up")
	}
	return true
}

type shmMessage struct {
	Tango string `json:"tango,omitempty"`
	Op    string `json:"op,omitempty"`
	Ring  string `json:"ring"`
	Gen   uint64 `json:"gen"`
}

// failUp tells the page to stop writing a ring this worker no longer reads.
// A navigation or a replacement hello may retire it before the UI callback.
func (s *shmLanes) failUp(g *shmGeneration) {
	if shmSignaled(g.stop) {
		return
	}
	s.ops.onUI(func() {
		if s.originForGeneration(g.gen) == assetOrigin {
			msg, _ := json.Marshal(shmMessage{Tango: "lane", Op: "fail", Ring: "up", Gen: g.gen})
			s.ops.message(string(msg))
		}
	})
}

func (s *shmLanes) wakePage(g *shmGeneration, ring string) {
	pending := &g.downWake
	if ring == "up" {
		pending = &g.upWake
	}
	s.mu.Lock()
	if *pending || shmSignaled(g.stop) {
		s.mu.Unlock()
		return
	}
	*pending = true
	s.mu.Unlock()
	s.ops.onUI(func() {
		s.mu.Lock()
		*pending = false
		active := !shmSignaled(g.stop) && (ring != "up" || !shmSignaled(g.upDone))
		s.mu.Unlock()
		if active {
			msg, _ := json.Marshal(shmMessage{Tango: "lane", Op: "wake", Ring: ring, Gen: g.gen})
			s.ops.message(string(msg))
		}
	})
}

// handleWake consumes only the wake operation. The core handles ready,
// fail and all other messages.
func (s *shmLanes) handleWake(origin, text string) bool {
	payload, ok := strings.CutPrefix(text, shmLanePrefix)
	if !ok {
		return false
	}
	var msg shmMessage
	err := json.Unmarshal([]byte(payload), &msg)
	if msg.Op != "wake" {
		return false
	}
	if err != nil || s == nil || origin != assetOrigin {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.current
	if s.origin != origin || g == nil || msg.Gen != s.latest || msg.Gen != g.gen {
		return true
	}
	switch msg.Ring {
	case "down":
		shmSignal(g.downSpace)
	case "up":
		if !shmSignaled(g.upDone) {
			shmSignal(g.upReady)
		}
	}
	return true
}
