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
	if v := judge(tg, o, 5); v != Faster {
		_, why := judgeWhy(tg, o, 5)
		t.Fatalf("verdict %s (%s), want Faster", v, why)
	}
}

func TestJudgeHalfNotEnough(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.01, nil)
	tg := rounds("tango", 5, 0.55, 0.01, nil)
	if v := judge(tg, o, 5); v != NotFaster {
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
	if v := judge(tg, o, 5); v != NotFaster {
		_, why := judgeWhy(tg, o, 5)
		t.Fatalf("verdict %s (%s), want NotFaster", v, why)
	}
}

func TestJudgeSlowerFrames(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.FPS, r.WorkPerSec = 100, 100*600 })
	if v := judge(tg, o, 5); v != NotFaster {
		t.Fatalf("lower fps: verdict %s, want NotFaster", v)
	}
	tg = rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.P99us = 20000 })
	if v := judge(tg, o, 5); v != NotFaster {
		t.Fatalf("worse p99: verdict %s, want NotFaster", v)
	}
}

func TestWorkMismatch(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.WorkPerSec = 120 * 600 * 0.94 })
	if v := judge(tg, o, 5); v != Invalid {
		t.Fatalf("6%% less work: verdict %s, want Invalid", v)
	}
	tg = rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.WorkPerSec = 120 * 600 * 0.97 })
	if v := judge(tg, o, 5); v == Invalid {
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
	if v := judge(tg, o, 5); v != Invalid {
		t.Fatalf("native and metal in one scene: verdict %s, want Invalid", v)
	}
}

func TestJudgeCounterIgnoresFrames(t *testing.T) {
	idle := func(fw string, cpu float64, mem int64) []Round {
		return rounds(fw, 5, cpu, 0.1, func(_ int, r *Round) {
			r.Scene, r.FPS, r.WorkPerSec, r.P50us, r.P99us, r.Footprint = "counter", 0, 0, 0, 0, mem
		})
	}
	if v := judge(idle("tango", 0.004, 30<<20), idle("mygo", 0.01, 40<<20), 5); v != Faster {
		t.Fatalf("counter, cheaper and smaller: verdict %s, want Faster", v)
	}
	if v := judge(idle("tango", 0.004, 50<<20), idle("mygo", 0.01, 40<<20), 5); v != NotFaster {
		t.Fatalf("counter, more memory: verdict %s, want NotFaster", v)
	}
}

func TestJudgeNoRounds(t *testing.T) {
	if v := judge(nil, rounds("mygo", 5, 1, 0.1, nil), 5); v != Invalid {
		t.Fatalf("no Tango rounds: verdict %s, want Invalid", v)
	}
}

func TestJudgeTooFewRounds(t *testing.T) {
	// One round each at 0.3x: a single round has Q1 = Q3 = median, so the
	// interquartile condition would hold by itself.
	o := rounds("mygo", 1, 1.0, 0, nil)
	tg := rounds("tango", 1, 0.3, 0, nil)
	if v := judge(tg, o, 1); v != Invalid {
		t.Fatalf("1 round each, minRounds 1 (floored to 3): verdict %s, want Invalid", v)
	}
	// 2 of 5 Tango rounds usable.
	o = rounds("mygo", 5, 1.0, 0.1, nil)
	tg = rounds("tango", 5, 0.3, 0.1, func(i int, r *Round) { r.Disturbed = i >= 2 })
	if v := judge(tg, o, 5); v != Invalid {
		t.Fatalf("2 of 5 usable: verdict %s, want Invalid", v)
	}
	// 3 of 5 usable meets the floor.
	tg = rounds("tango", 5, 0.3, 0.1, func(i int, r *Round) { r.Disturbed = i >= 3 })
	if v := judge(tg, o, 5); v != Invalid {
		t.Fatalf("3 of 5 usable with minRounds 5: verdict %s, want Invalid", v)
	}
	if v := judge(tg, o, 3); v != Faster {
		_, why := judgeWhy(tg, o, 3)
		t.Fatalf("3 of 5 usable with minRounds 3: verdict %s (%s), want Faster", v, why)
	}
}

func TestUnknownRendererUnusable(t *testing.T) {
	for _, name := range []string{"", "unknown"} {
		o := rounds("mygo", 5, 1.0, 0.1, func(_ int, r *Round) { r.Renderer = name })
		if got := usable(o); len(got) != 0 {
			t.Fatalf("renderer %q: usable kept %d rounds, want 0", name, len(got))
		}
		if v := judge(rounds("tango", 5, 0.4, 0.1, nil), o, 3); v != Invalid {
			t.Fatalf("opponent renderer %q: verdict %s, want Invalid", name, v)
		}
	}
}

// sparse makes n rounds of a scene that changes every period frames: 3 s
// at 60 fps, 180 frames reported, events changes among them.
func sparse(scene, fw string, n int, cpu float64, events int) []Round {
	return rounds(fw, n, cpu, 0.1, func(_ int, r *Round) {
		r.Scene, r.Seconds, r.FPS, r.Intervals = scene, 3, 60, 180
		r.CPUSeconds, r.WindowServerSeconds = cpu*3, cpu*0.3
		r.WorkPerSec = float64(events) / 3
	})
}

// R35: a period-30 scene over 3 s expects 6 changes; one more at the
// report edges (7 vs 6, 14% apart) is within max(5%, 1/6) and not Invalid.
func TestJudgeSparseEdgeEventNotInvalid(t *testing.T) {
	saved := sceneMetas
	defer func() { sceneMetas = saved }()
	sceneMetas = map[string]sceneMeta{"sparse": {period: 30}}
	tg := sparse("sparse", "tango", 5, 0.4, 6)
	o := sparse("sparse", "mygo", 5, 1.0, 7)
	if v, why := judgeWhy(tg, o, 5); v == Invalid {
		t.Fatalf("verdict Invalid (%s), want a judged verdict", why)
	}
	// Two changes apart (6 vs 8, 25%) is beyond 1/6: Invalid.
	o = sparse("sparse", "mygo", 5, 1.0, 8)
	if v := judge(tg, o, 5); v != Invalid {
		t.Fatalf("verdict %s for 6 vs 8 changes, want Invalid", v)
	}
}

// A scene changing every frame keeps the 5% bar: 6% apart is Invalid.
func TestJudgeDenseSixPercentInvalid(t *testing.T) {
	o := rounds("mygo", 5, 1.0, 0.1, nil)
	tg := rounds("tango", 5, 0.4, 0.1, func(_ int, r *Round) { r.WorkPerSec *= 1.06 })
	if v, why := judgeWhy(tg, o, 5); v != Invalid {
		t.Fatalf("verdict %s (%s), want Invalid", v, why)
	}
	if tol := workToleranceFor(1, tg, o); tol != workTolerance {
		t.Fatalf("dense tolerance %v, want %v", tol, workTolerance)
	}
}

// R37: a scene with unequal default motion is reported, not judged.
func TestJudgeUnequalMotionInfo(t *testing.T) {
	for _, scene := range []string{"tabs", "dialog", "toast"} {
		if !metaOf(scene).unequalMotion {
			t.Fatalf("%s not flagged unequal motion", scene)
		}
		period := metaOf(scene).period
		tg := sparse(scene, "tango", 5, 0.1, 180/period)
		o := sparse(scene, "nsgo", 5, 1.0, 180/period)
		if v, why := judgeWhy(tg, o, 5); v != Info {
			t.Fatalf("%s: verdict %s (%s), want Info", scene, v, why)
		}
		// Too few rounds and unequal work still make it Invalid.
		if v := judge(tg[:2], o, 5); v != Invalid {
			t.Fatalf("%s with 2 rounds: verdict %s, want Invalid", scene, v)
		}
	}
	if metaOf("grid").unequalMotion || metaOf("grid").period != 1 {
		t.Fatal("grid has special meta")
	}
}
