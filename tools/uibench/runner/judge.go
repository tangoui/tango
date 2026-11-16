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
	// work, or a framework changed its drawing path between rounds).
	Invalid Verdict = "Invalid"
)

// The bar of the global constraints.
const (
	cpuRatio       = 0.50 // Tango's median CPU at most this share of the opponent's
	workTolerance  = 0.05 // work per frame may differ this much
	minRoundsFloor = 3    // fewer usable rounds never judge: quartiles need them
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
// Faster needs all of: the median of process+WindowServer CPU at most half
// the opponent's; Tango's third quartile below the opponent's first (the
// interquartile ranges do not overlap); median fps at least the
// opponent's; median frame-interval p99 at most the opponent's. The counter
// scene draws no frames after ready, so there fps and p99 give way to the
// median physical footprint, which must be at most the opponent's.
func judgeWhy(t, o []Round, minRounds int) (Verdict, string) {
	t, o = usable(t), usable(o)
	need := max(minRounds, minRoundsFloor)
	if len(t) < need || len(o) < need {
		return Invalid, fmt.Sprintf("usable rounds: tango %d, opponent %d, need %d each", len(t), len(o), need)
	}
	for _, side := range [][]Round{t, o} {
		for _, r := range side[1:] {
			if r.Renderer != side[0].Renderer {
				return Invalid, fmt.Sprintf("%s drew with %q and %q", r.Framework, side[0].Renderer, r.Renderer)
			}
		}
	}
	if d := relDiff(median(each(t, Round.workPerFrame)), median(each(o, Round.workPerFrame))); d > workTolerance {
		return Invalid, fmt.Sprintf("work per frame differs by %.1f%%", d*100)
	}

	tc, oc := median(each(t, Round.cpu)), median(each(o, Round.cpu))
	_, tq3 := quartiles(each(t, Round.cpu))
	oq1, _ := quartiles(each(o, Round.cpu))
	why := fmt.Sprintf("cpu %.4f vs %.4f cores (%.2fx), tango q3 %.4f vs opponent q1 %.4f", tc, oc, ratio(tc, oc), tq3, oq1)
	if !(tc <= oc*cpuRatio) {
		return NotFaster, why + ": cpu above half"
	}
	if !(tq3 < oq1) {
		return NotFaster, why + ": interquartile ranges overlap"
	}
	if t[0].Scene == "counter" {
		tm, om := median(each(t, Round.footprint)), median(each(o, Round.footprint))
		why += fmt.Sprintf("; footprint %.1f vs %.1f MiB", tm/(1<<20), om/(1<<20))
		if tm > om {
			return NotFaster, why + ": more memory"
		}
		return Faster, why
	}
	tf, of := median(each(t, Round.fps)), median(each(o, Round.fps))
	tp, op := median(each(t, Round.p99)), median(each(o, Round.p99))
	why += fmt.Sprintf("; fps %.1f vs %.1f; p99 %.0f vs %.0f µs", tf, of, tp, op)
	if tf < of {
		return NotFaster, why + ": fewer frames"
	}
	if tp > op {
		return NotFaster, why + ": worse p99"
	}
	return Faster, why
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
