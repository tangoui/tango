package main

import "testing"

// rounds makes n rounds of one framework at cpu cores of process CPU
// (WindowServer adds a tenth of it), spread ±spread around it.
func rounds(fw string, n int, cpu, spread float64, mod func(i int, r *Round)) []Round {
	var rs []Round
	for i := range n {
		f := cpu * (1 + spread*float64(i-n/2)/float64(n))
		r := Round{
			Scene: "grid", Framework: fw, Round: i, Ready: true, Renderer: "native",
			Seconds: 10, CPUSeconds: f * 10, WindowServerSeconds: f,
			FPS: 120, WorkPerSec: 120 * 600, P50us: 8333, P99us: 9000, Footprint: 50 << 20,
		}
		if mod != nil {
			mod(i, &r)
		}
		rs = append(rs, r)
	}
	return rs
}

func TestJudgeFaster(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.45, 0.1, nil)
	if v := judge(tg, o); v != Faster {
		_, why := judgeWhy(tg, o)
		t.Fatalf("verdict %s (%s), want Faster", v, why)
	}
}

func TestJudgeHalfNotEnough(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.01, nil)
	tg := rounds("tango", 5, 0.55, 0.01, nil)
	if v := judge(tg, o); v != NotFaster {
		t.Fatalf("verdict %s, want NotFaster", v)
	}
}

func TestJudgeOverlap(t *testing.T) {
	// Medians 0.48 : 1, but one opponent round is as cheap as Tango's
	// dearest, so the interquartile ranges overlap.
	o := rounds("mygo", 5, 1.0, 0, func(i int, r *Round) {
		if i < 2 {
			r.CPUSeconds, r.WindowServerSeconds = 4.0, 0.4
		}
	})
	tg := rounds("tango", 5, 0.48, 0, func(i int, r *Round) {
		if i >= 3 {
			r.CPUSeconds, r.WindowServerSeconds = 6.0, 0.6
		}
	})
	if v := judge(tg, o); v != NotFaster {
		_, why := judgeWhy(tg, o)
		t.Fatalf("verdict %s (%s), want NotFaster", v, why)
	}
}

func TestJudgeSlowerFrames(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.FPS, r.WorkPerSec = 100, 100*600 })
	if v := judge(tg, o); v != NotFaster {
		t.Fatalf("lower fps: verdict %s, want NotFaster", v)
	}
	tg = rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.P99us = 20000 })
	if v := judge(tg, o); v != NotFaster {
		t.Fatalf("worse p99: verdict %s, want NotFaster", v)
	}
}

func TestWorkMismatch(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.WorkPerSec = 120 * 600 * 0.94 })
	if v := judge(tg, o); v != Invalid {
		t.Fatalf("6%% less work: verdict %s, want Invalid", v)
	}
	tg = rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.WorkPerSec = 120 * 600 * 0.97 })
	if v := judge(tg, o); v == Invalid {
		t.Fatalf("3%% less work: verdict Invalid, want a comparison")
	}
}

func TestRendererConsistent(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, func(i int, r *Round) {
		if i == 2 {
			r.Renderer = "metal"
		}
	})
	if v := judge(tg, o); v != Invalid {
		t.Fatalf("native and metal in one scene: verdict %s, want Invalid", v)
	}
}

func TestJudgeCounterIgnoresFrames(t *testing.T) {
	idle := func(fw string, cpu float64, mem int64) []Round {
		return rounds(fw, 5, cpu, 0.1, func(_ int, r *Round) {
			r.Scene, r.FPS, r.WorkPerSec, r.P50us, r.P99us, r.Footprint = "counter", 0, 0, 0, 0, mem
		})
	}
	if v := judge(idle("tango", 0.004, 30<<20), idle("mygo", 0.01, 40<<20)); v != Faster {
		t.Fatalf("counter, cheaper and smaller: verdict %s, want Faster", v)
	}
	if v := judge(idle("tango", 0.004, 50<<20), idle("mygo", 0.01, 40<<20)); v != NotFaster {
		t.Fatalf("counter, more memory: verdict %s, want NotFaster", v)
	}
}

func TestJudgeNoRounds(t *testing.T) {
	if v := judge(nil, rounds("mygo", 5, 1, 0.1, nil)); v != Invalid {
		t.Fatalf("no Tango rounds: verdict %s, want Invalid", v)
	}
}
