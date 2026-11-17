package main

import (
	"fmt"
	"math"
)

// Verdict is the outcome of comparing Tango with one opponent on one scene.
type Verdict string

const (
	// Faster: Tango met every condition of the "faster" bar.
	Faster Verdict = "Faster"
	// NotFaster: the comparison is valid and Tango missed a condition.
	NotFaster Verdict = "NotFaster"
	// Invalid: the rounds cannot be compared (no usable rounds, different
	// work, or a framework changed its drawing path between rounds; a
	// mixed round matches only rounds with the same switch sequence).
	Invalid Verdict = "Invalid"
	// Info: the comparison is valid but not judged, because the frameworks'
	// default motion differs in the scene (sceneMeta.unequalMotion): its
	// CPU would measure the motion as much as the component (R37).
	Info Verdict = "Info"
)

// sceneMeta is what the judge must know about a scene beyond its rounds.
type sceneMeta struct {
	// period is the number of frames between the scene's changes: 1 when
	// every frame changes something (the default), 10 or 30 for scenes
	// that change now and then.
	period int
	// unequalMotion: the frameworks animate the scene's change for
	// different times by default (scenes.md, "Default motion").
	unequalMotion bool
	// sparseFrames: the protocol defines the scene's frame rate below
	// 60 fps (it draws only now and then), so the R51 drawing check
	// (checkDrawing) does not apply. The periodic scenes (period 10, 30)
	// still draw every frame and are checked.
	sparseFrames bool
}

// scenes the judge treats specially; every other scene changes on every
// frame with no motion difference worth flagging.
var sceneMetas = map[string]sceneMeta{
	"tabs":   {period: 10, unequalMotion: true}, // indicator: tango 300 ms, nsgo 180 ms, mygo none
	"dialog": {period: 30, unequalMotion: true}, // open/close: tango 300 ms, nsgo 180 ms, mygo none
	"toast":  {period: 30, unequalMotion: true}, // in: tango 200 ms, mygo 200 ms, nsgo 400 ms spring
	// NSGo's ui.Progress eases its bar to a new value over 200 ms (layout
	// transition); Tango's and MyGo's bars jump (Task 11 fix round 1).
	"progress": {unequalMotion: true},
	// Nearest-counterpart scenes (Task 11).
	"tooltip":   {period: 30, unequalMotion: true}, // popover in: tango 200 ms, mygo and nsgo none
	"carousel":  {period: 30, unequalMotion: true}, // slide: tango 500 ms, nsgo 362 ms spring, mygo (composed) none
	"collapse":  {period: 30, unequalMotion: true}, // panel: tango 200 ms, mygo 200 ms, nsgo 265 ms spring
	"drawer":    {period: 30, unequalMotion: true}, // slide: tango 300 ms, nsgo 280/200 ms, mygo (composed) none
	"tour":      {period: 30},                      // step change: no motion in any (tango's mask fades in once)
	"segmented": {unequalMotion: true},             // thumb: tango 300 ms, nsgo 180 ms, mygo none
	"steps":     {unequalMotion: true},             // current: nsgo 160-400 ms, tango and mygo (composed) none
	"upload":    {unequalMotion: true},             // bar: nsgo Progress 200 ms, tango and mygo none
}

func metaOf(scene string) sceneMeta {
	m := sceneMetas[scene]
	if m.period < 1 {
		m.period = 1
	}
	return m
}

// The bar of the global constraints.
const (
	cpuRatio       = 0.50 // Tango's median CPU at most this share of the opponent's
	workTolerance  = 0.05 // work per frame may differ this much, at least (see workToleranceFor)
	minRoundsFloor = 3    // fewer usable rounds never judge: quartiles need them
	// minJudgedCPU (cores): an opponent cheaper than this is not judged
	// (Info): at that level proc + ws_net is timer and compositor noise.
	minJudgedCPU = 0.05
	// minCPUGap (cores): Faster also needs the opponent's median at least
	// this much above Tango's, not just a ratio of two small numbers.
	minCPUGap = 0.02
)

// judge compares Tango's rounds t with an opponent's rounds o of the same
// scene. Each side needs at least minRounds usable rounds (never fewer than
// minRoundsFloor): with one round, Q1 = Q3 = the median and the
// interquartile condition would hold by itself.
func judge(t, o []Round, minRounds int) Verdict {
	v, _ := judgeWhy(t, o, minRounds)
	return v
}

// judgeWhy is judge with the reason, for the results file.
//
// Faster needs all of: the median of process + net WindowServer CPU
// (Round.cpu: the WindowServer delta less the round's idle compositor) at most half
// the opponent's; Tango's third quartile below the opponent's first (the
// interquartile ranges do not overlap); median fps at least the
// opponent's; median frame-interval p99 at most the opponent's; and the
// opponent's median at least minCPUGap above Tango's. An opponent below
// minJudgedCPU is too cheap to compare (Info). The counter
// scene draws no frames after ready, so there fps and p99 give way to the
// median physical footprint, which must be at most the opponent's.
func judgeWhy(t, o []Round, minRounds int) (Verdict, string) {
	t, o = usable(t), usable(o)
	need := max(minRounds, minRoundsFloor)
	if len(t) < need || len(o) < need {
		return Invalid, fmt.Sprintf("usable rounds: tango %d, opponent %d, need %d each", len(t), len(o), need)
	}
	// The same path in every round, by the per-round summary: a mixed
	// round matches only rounds that switched in the same sequence.
	for _, side := range [][]Round{t, o} {
		for _, r := range side[1:] {
			if rendererKey(r) != rendererKey(side[0]) {
				return Invalid, fmt.Sprintf("%s drew with %q and %q", r.Framework, rendererKey(side[0]), rendererKey(r))
			}
		}
	}
	paths := fmt.Sprintf("; renderer tango %s, opponent %s", describeRenderer(t), describeRenderer(o))
	meta := metaOf(t[0].Scene)
	tol, ok := workToleranceFor(meta.period, t, o)
	if !ok {
		return Invalid, fmt.Sprintf("too few sampled frames for a change every %d frames", meta.period)
	}
	if d := relDiff(median(each(t, Round.workPerFrame)), median(each(o, Round.workPerFrame))); d > tol {
		return Invalid, fmt.Sprintf("work per frame differs by %.1f%% (tolerance %.1f%%)", d*100, tol*100)
	}

	tc, oc := median(each(t, Round.cpu)), median(each(o, Round.cpu))
	_, tq3 := quartiles(each(t, Round.cpu))
	oq1, _ := quartiles(each(o, Round.cpu))
	label := "proc + ws_net"
	if t[0].CPUMetric == metricProcess {
		label = "process only"
	}
	why := fmt.Sprintf("cpu (%s) %.4f vs %.4f cores (%.2fx), tango q3 %.4f vs opponent q1 %.4f", label, tc, oc, ratio(tc, oc), tq3, oq1)
	tf, of := median(each(t, Round.fps)), median(each(o, Round.fps))
	tp, op := median(each(t, Round.p99)), median(each(o, Round.p99))
	frames := fmt.Sprintf("; fps %.1f vs %.1f; p99 %.0f vs %.0f µs", tf, of, tp, op)
	if meta.unequalMotion {
		return Info, why + frames + paths + "; not judged (unequal default motion)"
	}
	if oc < minJudgedCPU {
		return Info, why + frames + paths + fmt.Sprintf("; too cheap to compare (opponent %.4f < %.2f cores)", oc, minJudgedCPU)
	}
	if miss := cpuBarMissed(tc, oc); miss != "" {
		return NotFaster, why + paths + miss
	}
	if !(tq3 < oq1) {
		return NotFaster, why + paths + ": interquartile ranges overlap"
	}
	if t[0].Scene == "counter" {
		tm, om := median(each(t, Round.footprint)), median(each(o, Round.footprint))
		why += fmt.Sprintf("; footprint %.1f vs %.1f MiB", tm/(1<<20), om/(1<<20))
		why += paths
		if tm > om {
			return NotFaster, why + ": more memory"
		}
		return Faster, why
	}
	why += frames + paths
	if tf < of {
		return NotFaster, why + ": fewer frames"
	}
	if tp > op {
		return NotFaster, why + ": worse p99"
	}
	return Faster, why
}

// cpuBarMissed is why Tango's median tc misses the CPU bar against the
// opponent's oc ("" when it meets it): at most cpuRatio of it, and at least
// minCPUGap cores below it. With the present constants the ratio implies
// the gap for any judged opponent (oc ≥ 0.05 and tc ≤ oc/2 give a gap ≥
// 0.025); the gap guards the bar should either constant change.
func cpuBarMissed(tc, oc float64) string {
	if !(tc <= oc*cpuRatio) {
		return ": cpu above half"
	}
	if !(oc-tc >= minCPUGap) {
		return fmt.Sprintf(": cpu difference %.4f below %.2f cores", oc-tc, minCPUGap)
	}
	return ""
}

// workToleranceFor is R35's work tolerance: max(5%, 1/expected events),
// where expected events are the frames a round's reports cover divided by
// the scene's period, the fewer of the two sides' medians. A scene that
// changes every 30th frame sees about 6 changes in 3 s, so one change
// more or less at the edges of the reports moves its work per frame by
// a sixth with no difference in what the programs do. ok is false when
// the reports cover less than one period: no change is expected at all,
// so there is no work to compare and the rounds are Invalid.
func workToleranceFor(period int, t, o []Round) (tol float64, ok bool) {
	if period <= 1 {
		return workTolerance, true
	}
	events := math.Min(median(each(t, Round.framesSampled)), median(each(o, Round.framesSampled))) / float64(period)
	if events < 1 {
		return 0, false
	}
	return math.Max(workTolerance, 1/events), true
}

// relDiff is |a-b| relative to the larger; 0 when both are 0.
func relDiff(a, b float64) float64 {
	m := math.Max(math.Abs(a), math.Abs(b))
	if m == 0 {
		return 0
	}
	return math.Abs(a-b) / m
}

func ratio(a, b float64) float64 {
	if b == 0 {
		return math.Inf(1)
	}
	return a / b
}
