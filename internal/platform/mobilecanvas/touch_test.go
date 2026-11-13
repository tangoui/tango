package mobilecanvas

import (
	"reflect"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func smp(x, y float32, p float32, t int) Sample {
	return Sample{Pos: geom.Pt(x, y), Pressure: p, Time: ms(t)}
}

func tev(id int64, ph input.TouchPhase, x, y, p float32, t int) input.TouchEvent {
	return input.TouchEvent{ID: id, Phase: ph, Pos: geom.Pt(x, y), Pressure: p, Time: ms(t)}
}

func TestTouches(t *testing.T) {
	type call struct {
		op   string // begin, move, end, cancel, cancelAll
		key  uintptr
		hist []Sample
		s    Sample
	}
	tests := []struct {
		name  string
		calls []call
		want  []input.TouchEvent
		live  int
	}{
		{"tap", []call{
			{op: "begin", key: 0xa0, s: smp(10, 20, 0.5, 0)},
			{op: "end", key: 0xa0, s: smp(11, 21, 0.4, 50)},
		}, []input.TouchEvent{
			tev(1, input.TouchBegan, 10, 20, 0.5, 0),
			tev(1, input.TouchEnded, 11, 21, 0.4, 50),
		}, 0},
		{"two fingers get distinct ids; ids are not reused", []call{
			{op: "begin", key: 0xa0, s: smp(1, 1, 0, 0)},
			{op: "begin", key: 0xb0, s: smp(2, 2, 0, 1)},
			{op: "end", key: 0xa0, s: smp(1, 1, 0, 2)},
			{op: "begin", key: 0xa0, s: smp(3, 3, 0, 3)}, // same UITouch address again
			{op: "move", key: 0xb0, s: smp(4, 4, 0, 4)},
		}, []input.TouchEvent{
			tev(1, input.TouchBegan, 1, 1, 0, 0),
			tev(2, input.TouchBegan, 2, 2, 0, 1),
			tev(1, input.TouchEnded, 1, 1, 0, 2),
			tev(3, input.TouchBegan, 3, 3, 0, 3),
			tev(2, input.TouchMoved, 4, 4, 0, 4),
		}, 2},
		{"unknown fingers are ignored", []call{
			{op: "move", key: 1, s: smp(1, 1, 0, 0)},
			{op: "end", key: 1, s: smp(1, 1, 0, 0)},
			{op: "cancel", key: 1, s: smp(1, 1, 0, 0)},
		}, nil, 0},
		{"begin on a live key cancels the stale finger", []call{
			{op: "begin", key: 7, s: smp(1, 1, 0.3, 0)},
			{op: "move", key: 7, s: smp(5, 5, 0.3, 5)},
			{op: "begin", key: 7, s: smp(9, 9, 0.2, 10)},
		}, []input.TouchEvent{
			tev(1, input.TouchBegan, 1, 1, 0.3, 0),
			tev(1, input.TouchMoved, 5, 5, 0.3, 5),
			tev(1, input.TouchCancelled, 5, 5, 0, 10),
			tev(2, input.TouchBegan, 9, 9, 0.2, 10),
		}, 1},
		{"coalesced history before the current sample", []call{
			{op: "begin", key: 1, s: smp(0, 0, 0, 0)},
			{op: "move", key: 1, hist: []Sample{smp(1, 0, 0, 1), smp(2, 0, 0, 2)}, s: smp(3, 0, 0, 3)},
		}, []input.TouchEvent{
			tev(1, input.TouchBegan, 0, 0, 0, 0),
			tev(1, input.TouchMoved, 1, 0, 0, 1),
			tev(1, input.TouchMoved, 2, 0, 0, 2),
			tev(1, input.TouchMoved, 3, 0, 0, 3),
		}, 1},
		{"cancel drops pressure", []call{
			{op: "begin", key: 1, s: smp(0, 0, 0.9, 0)},
			{op: "cancel", key: 1, s: smp(2, 2, 0.9, 1)},
		}, []input.TouchEvent{
			tev(1, input.TouchBegan, 0, 0, 0.9, 0),
			tev(1, input.TouchCancelled, 2, 2, 0, 1),
		}, 0},
		{"cancel all, in down order, at last positions", []call{
			{op: "begin", key: 30, s: smp(1, 1, 0, 0)},
			{op: "begin", key: 10, s: smp(2, 2, 0, 1)},
			{op: "begin", key: 20, s: smp(3, 3, 0, 2)},
			{op: "move", key: 10, s: smp(22, 22, 0.5, 3)},
			{op: "end", key: 20, s: smp(3, 3, 0, 4)},
			{op: "cancelAll", s: smp(0, 0, 0, 9)},
			{op: "cancelAll", s: smp(0, 0, 0, 10)}, // nothing left
		}, []input.TouchEvent{
			tev(1, input.TouchBegan, 1, 1, 0, 0),
			tev(2, input.TouchBegan, 2, 2, 0, 1),
			tev(3, input.TouchBegan, 3, 3, 0, 2),
			tev(2, input.TouchMoved, 22, 22, 0.5, 3),
			tev(3, input.TouchEnded, 3, 3, 0, 4),
			tev(1, input.TouchCancelled, 1, 1, 0, 9),
			tev(2, input.TouchCancelled, 22, 22, 0, 9),
		}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tr Touches[uintptr]
			var got []input.TouchEvent
			for _, c := range tt.calls {
				switch c.op {
				case "begin":
					got = tr.Begin(got, c.key, c.s)
				case "move":
					got = tr.Move(got, c.key, c.hist, c.s)
				case "end":
					got = tr.End(got, c.key, c.s)
				case "cancel":
					got = tr.Cancel(got, c.key, c.s)
				case "cancelAll":
					got = tr.CancelAll(got, c.s.Time)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("events:\n got %v\nwant %v", got, tt.want)
			}
			if tr.Len() != tt.live {
				t.Fatalf("live = %d, want %d", tr.Len(), tt.live)
			}
		})
	}
}

func TestTouchesID(t *testing.T) {
	var tr Touches[int32] // Android pointer ids
	tr.Begin(nil, 0, Sample{})
	tr.Begin(nil, 1, Sample{})
	if id, ok := tr.ID(1); !ok || id != 2 {
		t.Fatalf("ID(1) = %d, %v", id, ok)
	}
	tr.End(nil, 1, Sample{})
	if _, ok := tr.ID(1); ok {
		t.Fatal("ended finger still known")
	}
}

func TestCoalesce(t *testing.T) {
	h := func(n int) []Sample {
		s := make([]Sample, n)
		for i := range s {
			s[i] = smp(float32(i), 0, 0, i)
		}
		return s
	}
	xs := func(s []Sample) []float32 {
		var out []float32
		for _, v := range s {
			out = append(out, v.Pos.X)
		}
		return out
	}
	tests := []struct {
		n    int
		want []float32
	}{
		{0, nil},
		{1, []float32{0}},
		{4, []float32{0, 1, 2, 3}},
		{5, []float32{1, 2, 3, 4}},
		{8, []float32{1, 3, 5, 7}},
		{9, []float32{2, 4, 6, 8}},
		{100, []float32{24, 49, 74, 99}},
	}
	for _, tt := range tests {
		got := Coalesce(h(tt.n))
		if len(got) > MaxCoalesced {
			t.Fatalf("n=%d: %d samples", tt.n, len(got))
		}
		if !reflect.DeepEqual(xs(got), tt.want) {
			t.Errorf("n=%d: got %v, want %v", tt.n, xs(got), tt.want)
		}
	}
}

func TestPressure(t *testing.T) {
	nan := float32(0)
	nan = nan / nan
	tests := []struct{ f, m, want float32 }{
		{0.5, 0, 0},  // maximumPossibleForce 0: not reported
		{0.5, -1, 0}, // bogus max
		{1, 2, 0.5},
		{3, 2, 1},
		{-1, 1, 0},
		{0.25, 1, 0.25},
		{nan, 1, 0},
		{1, nan, 0},
	}
	for _, tt := range tests {
		if got := Pressure(tt.f, tt.m); got != tt.want {
			t.Errorf("Pressure(%v, %v) = %v, want %v", tt.f, tt.m, got, tt.want)
		}
	}
}

func TestClock(t *testing.T) {
	now := ms(10_000)
	c := Clock{Now: func() time.Time { return now }}
	// First event: uptime 500s maps to now.
	if got := c.Time(500 * time.Second); !got.Equal(now) {
		t.Fatalf("first = %v", got)
	}
	// Later events keep exact spacing, even with a lagging Now.
	now = now.Add(time.Second)
	if got := c.Time(500*time.Second + 16*time.Millisecond); !got.Equal(ms(10_016)) {
		t.Fatalf("second = %v, want %v", got, ms(10_016))
	}
	// Slightly in the future (event stamped after Now was read) is kept.
	if got := c.Time(501*time.Second + 40*time.Millisecond); !got.Equal(ms(11_040)) {
		t.Fatalf("near future = %v", got)
	}
	// Far in the future: the clocks jumped; re-anchor at now.
	if got := c.Time(600 * time.Second); !got.Equal(now) {
		t.Fatalf("jump = %v, want %v", got, now)
	}
	if got := c.Time(600*time.Second + time.Millisecond); !got.Equal(now.Add(time.Millisecond)) {
		t.Fatalf("after jump = %v", got)
	}
	c.Reset()
	if got := c.Time(0); !got.Equal(now) {
		t.Fatalf("after reset = %v", got)
	}
	var d Clock // nil Now: time.Now
	if got := d.Time(time.Second); time.Since(got) > time.Minute {
		t.Fatalf("default clock = %v", got)
	}
}

func TestTimestampUnits(t *testing.T) {
	tests := []struct {
		got, want time.Duration
	}{
		{Seconds(1.5), 1500 * time.Millisecond},
		{Seconds(12345.000001), 12345*time.Second + time.Microsecond},
		{Seconds(-1), 0},
		{Seconds(1e300), time.Duration(1<<63 - 1)},
		{Millis(1234), 1234 * time.Millisecond},
		{Nanos(42), 42},
	}
	for i, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%d: %v, want %v", i, tt.got, tt.want)
		}
	}
}
