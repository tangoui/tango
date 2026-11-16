//go:build tango_dev

package downq

import (
	"encoding/binary"
	"os"
	"sort"
	"sync"
	"time"
)

var traceEnabled = sync.OnceValue(func() bool { return os.Getenv("TANGO_IPC_TRACE") == "1" })

func TraceEnabled() bool { return traceEnabled() }

type traceState struct {
	pushes    map[uint64]time.Time
	lastWrite time.Time
}

var traceData = struct {
	sync.Mutex
	next   uint64
	states map[*State]*traceState
	rows   []TraceBatch
}{states: make(map[*State]*traceState)}

func TraceReset() {
	if !TraceEnabled() {
		return
	}
	traceData.Lock()
	traceData.next = 0
	traceData.states = make(map[*State]*traceState)
	traceData.rows = nil
	traceData.Unlock()
}

func TraceStats() []TraceBatch {
	if !TraceEnabled() {
		return nil
	}
	traceData.Lock()
	defer traceData.Unlock()
	rows := append([]TraceBatch(nil), traceData.rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].RequestAt < rows[j].RequestAt })
	return rows
}

func traceEnd(s *State) {
	if !TraceEnabled() {
		return
	}
	traceData.Lock()
	delete(traceData.states, s)
	traceData.Unlock()
}

func stateTrace(s *State) *traceState {
	st := traceData.states[s]
	if st == nil {
		st = &traceState{pushes: make(map[uint64]time.Time)}
		traceData.states[s] = st
	}
	return st
}

// tracePush is called with s.mu held.
func tracePush(s *State, seq uint64) {
	if !TraceEnabled() {
		return
	}
	traceData.Lock()
	stateTrace(s).pushes[seq] = time.Now()
	traceData.Unlock()
}

// traceAck is called with s.mu held.
func traceAck(s *State, ack uint64) {
	if !TraceEnabled() {
		return
	}
	traceData.Lock()
	st := stateTrace(s)
	for seq := range st.pushes {
		if seq <= ack {
			delete(st.pushes, seq)
		}
	}
	traceData.Unlock()
}

// tracePollStart is called with s.mu held after the request was accepted.
func tracePollStart(s *State, gen, ack uint64, at time.Time) *traceRound {
	if !TraceEnabled() {
		return nil
	}
	traceData.Lock()
	defer traceData.Unlock()
	st := stateTrace(s)
	var gap float64
	if !st.lastWrite.IsZero() {
		gap = at.Sub(st.lastWrite).Seconds() * 1e6
	}
	traceData.next++
	return &traceRound{state: s, batch: TraceBatch{
		Round: traceData.next, Generation: gen, RequestAt: at.UnixNano(), Ack: ack, PollGapUs: gap,
	}}
}

// traceTake is called after takeLocked, while the request still owns its
// round. It is intentionally observational: it does not alter queue state.
func traceTake(r *traceRound, s *State, first uint64, items []Item, start, end time.Time) {
	if r == nil || !TraceEnabled() {
		return
	}
	traceData.Lock()
	defer traceData.Unlock()
	st := stateTrace(s)
	b := &r.batch
	b.FirstSeq, b.Items = first, len(items)
	b.TakeUs = end.Sub(start).Seconds() * 1e6
	var firstPush time.Time
	for _, it := range items {
		b.DataBytes += int64(len(it.Data))
		if it.Kind == Packet && len(it.Data) >= 2 {
			b.Frames += int(binary.BigEndian.Uint16(it.Data))
		}
		if p, ok := st.pushes[it.Seq]; ok && (firstPush.IsZero() || p.Before(firstPush)) {
			firstPush = p
		}
	}
	if !firstPush.IsZero() {
		b.PushToTakeUs = start.Sub(firstPush).Seconds() * 1e6
	}
}

func traceEncoded(r *traceRound, n int, start, end time.Time) {
	if r == nil || !TraceEnabled() {
		return
	}
	r.batch.BatchBytes = n
	r.batch.EncodeUs = end.Sub(start).Seconds() * 1e6
}

func traceWritten(r *traceRound, start, end time.Time) {
	if r == nil || !TraceEnabled() {
		return
	}
	r.batch.WriteUs = end.Sub(start).Seconds() * 1e6
	traceData.Lock()
	traceData.rows = append(traceData.rows, r.batch)
	if len(traceData.rows) > 4096 {
		traceData.rows = traceData.rows[len(traceData.rows)-4096:]
	}
	if st := traceData.states[r.state]; st != nil {
		st.lastWrite = end
	}
	traceData.Unlock()
}
