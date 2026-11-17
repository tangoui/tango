package main

import (
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The idle compositor (R44). WindowServer burns CPU when nothing draws (a
// remote-desktop viewer keeps it busy), so the raw delta carries a constant
// shared by every framework. Each scene round samples WindowServer for the
// sampling duration with no scene program running, before and after the
// round's launches; the judge reads process CPU + max(0, raw - idle).
const (
	idleWSLimit    = 0.60 // cores: a busier idle compositor disturbs the round
	idleDriftLimit = 0.30 // before/after differ by more than this share of the larger
	// idleDriftFloor: below this absolute difference (cores) drift is ps's
	// 10 ms resolution and ordinary jitter, not a background change.
	idleDriftFloor = 0.02
)

// WSBaseline is one scene round's idle WindowServer, shared by its rounds.
type WSBaseline struct {
	BeforeCores float64 `json:"before_cores"`
	AfterCores  float64 `json:"after_cores"`
	Cores       float64 `json:"cores"` // mean of before and after
	Drift       float64 `json:"drift"` // |before - after| / the larger
	// Disturbed says why the background compositor disturbed the round.
	Disturbed string `json:"disturbed,omitempty"`
}

// idleSample is one idle measurement, in cores.
type idleSample struct {
	cores float64
	err   error
}

// roundDeps is what a scene round needs from the machine; tests fake it.
type roundDeps struct {
	seconds  time.Duration
	wsClock  func() (time.Duration, error) // WindowServer's cumulative CPU
	now      func() time.Time
	sleep    func(time.Duration)
	children func() int                   // scene programs running
	measure  func(fw, scene string) Round // one attempt (runOnce)
}

// wsNet is the compositor CPU a round added over the idle compositor.
func wsNet(raw, idle float64) float64 { return math.Max(0, raw-idle) }

// sampleIdle measures WindowServer for d.seconds; it refuses while a scene
// program runs.
func sampleIdle(d roundDeps) idleSample {
	if n := d.children(); n > 0 {
		return idleSample{err: fmt.Errorf("idle WindowServer sample with %d scene program(s) running", n)}
	}
	w0, err := d.wsClock()
	if err != nil {
		return idleSample{err: err}
	}
	t0 := d.now()
	d.sleep(d.seconds)
	w1, err := d.wsClock()
	if err != nil {
		return idleSample{err: err}
	}
	el := d.now().Sub(t0).Seconds()
	if el <= 0 {
		return idleSample{err: fmt.Errorf("idle WindowServer sample: no time elapsed")}
	}
	return idleSample{cores: (w1 - w0).Seconds() / el}
}

// combineBaseline is the round's baseline from its before and after samples.
func combineBaseline(before, after idleSample) WSBaseline {
	b := WSBaseline{BeforeCores: before.cores, AfterCores: after.cores}
	var why []string
	for _, s := range []struct {
		name string
		idleSample
	}{{"before", before}, {"after", after}} {
		if s.err != nil {
			why = append(why, s.name+": "+s.err.Error())
		}
	}
	switch {
	case before.err == nil && after.err == nil:
		b.Cores = (before.cores + after.cores) / 2
	case before.err == nil:
		b.Cores = before.cores
	case after.err == nil:
		b.Cores = after.cores
	}
	if before.err == nil && after.err == nil {
		b.Drift = relDiff(before.cores, after.cores)
		if b.Drift > idleDriftLimit && math.Abs(before.cores-after.cores) >= idleDriftFloor {
			why = append(why, fmt.Sprintf("idle drift %.0f%% (%.3f → %.3f cores)", b.Drift*100, before.cores, after.cores))
		}
	}
	if m := math.Max(before.cores, after.cores); m > idleWSLimit {
		why = append(why, fmt.Sprintf("idle %.3f cores above %.2f", m, idleWSLimit))
	}
	b.Disturbed = strings.Join(why, "; ")
	return b
}

// applyBaseline records the idle and net WindowServer CPU of r (the raw
// delta stays) and marks r disturbed when the background compositor was.
func applyBaseline(r *Round, b WSBaseline) {
	bb := b
	r.WSBaseline = &bb
	r.WindowServerIdleSeconds = b.Cores * r.Seconds
	r.WindowServerNetSeconds = wsNet(r.WindowServerSeconds, r.WindowServerIdleSeconds)
	if b.Disturbed != "" {
		r.Disturbed = true
		r.Notes = append(r.Notes, "background WindowServer: "+b.Disturbed)
	}
}

// runSceneRound measures one round of a scene: an idle WindowServer
// sample, each framework in turn (a load-disturbed attempt is measured
// again, up to maxRetries more times), another idle sample. A round the
// background compositor disturbed is measured again as a whole, up to
// maxRetries more times. emit receives every attempt.
func runSceneRound(d roundDeps, scene string, round int, fws []string, emit func(Round) error) error {
	next := map[string]int{} // each framework's next attempt number
	for try := 0; try <= maxRetries; try++ {
		before := sampleIdle(d)
		var rs []Round
		for _, fw := range rotate(fws, round) {
			for a := 0; a <= maxRetries; a++ {
				r := d.measure(fw, scene)
				r.Round, r.Attempt = round, next[fw]
				next[fw]++
				rs = append(rs, r)
				if !r.Disturbed {
					break
				}
			}
		}
		b := combineBaseline(before, sampleIdle(d))
		for i := range rs {
			applyBaseline(&rs[i], b)
			if err := emit(rs[i]); err != nil {
				return err
			}
		}
		if b.Disturbed == "" {
			return nil
		}
	}
	return nil
}

// screenSharingNames are processes of known screen-sharing and remote-
// desktop tools, matched case-insensitively as a prefix of the program's
// name. The runner records them and never touches them.
var screenSharingNames = []string{
	"uuremote", "screensharingd", "screen sharing", "screensharingagent", "ardagent",
	"anydesk", "teamviewer", "todesk", "sunlogin", "rustdesk", "parsec",
	"splashtop", "srstreamer", "vncserver", "remoting_me2me_host", "jump desktop",
}

// matchScreenSharing picks the known screen-sharing programs from `ps -axo
// comm=` output (full paths), each name once, sorted.
func matchScreenSharing(ps string) []string {
	seen := map[string]bool{}
	for _, line := range strings.Split(ps, "\n") {
		name := filepath.Base(strings.TrimSpace(line))
		low := strings.ToLower(name)
		for _, k := range screenSharingNames {
			if strings.HasPrefix(low, k) {
				seen[name] = true
				break
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// screenSharingRunning lists the screen-sharing programs running now.
func screenSharingRunning() ([]string, error) {
	out, err := exec.Command("ps", "-axwwo", "comm=").Output()
	if err != nil {
		return nil, err
	}
	return matchScreenSharing(string(out)), nil
}

func (g *groups) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.pids)
}
