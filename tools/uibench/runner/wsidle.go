package main

import (
	"fmt"
	"io"
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
// round's launches (each after -settle, so a closing window's animation
// and the focus change are not counted as idle); each attempt's idle rate
// is interpolated between the two at its sample midpoint, and the judge
// reads process CPU + max(0, raw - idle).
const (
	idleWSLimit = 0.60 // cores: a busier idle compositor disturbs the round
	// Drift: before and after differ by more than max(idleDriftFloor,
	// idleDriftShare × the larger) cores. The floor is ps's 10 ms
	// resolution and ordinary jitter on a quiet compositor.
	idleDriftShare = 0.10
	idleDriftFloor = 0.02
	// busyIdleCores: a scene whose median idle compositor is above this is
	// measured on a busy compositor; its verdicts say so and the results are
	// not adoptable.
	busyIdleCores = 0.10
)

// WSBaseline is a scene round's idle WindowServer; Cores is the attempt's.
type WSBaseline struct {
	BeforeCores float64 `json:"before_cores"`
	AfterCores  float64 `json:"after_cores"`
	BeforeMidNs int64   `json:"before_mid_ns,omitempty"` // midpoints of the idle samples
	AfterMidNs  int64   `json:"after_mid_ns,omitempty"`
	MeanCores   float64 `json:"mean_cores"` // mean of before and after (the one that worked)
	// Cores is this attempt's idle rate: before and after interpolated at
	// its sample midpoint (MeanCores without one).
	Cores float64 `json:"cores"`
	Drift float64 `json:"drift"` // |before - after| / the larger
	// Disturbed says why the background compositor disturbed the round.
	Disturbed string `json:"disturbed,omitempty"`
	ok        [2]bool
}

// idleSample is one idle measurement, in cores, and its midpoint.
type idleSample struct {
	cores float64
	midNs int64
	err   error
}

// roundDeps is what a scene round needs from the machine; tests fake it.
type roundDeps struct {
	seconds  time.Duration
	settle   time.Duration                 // waited before each idle sample
	gate     func()                        // load gate before each attempt of the round (nil: none)
	wsClock  func() (time.Duration, error) // WindowServer's cumulative CPU
	now      func() time.Time
	sleep    func(time.Duration)
	children func() int                   // scene programs running
	measure  func(fw, scene string) Round // one attempt (runOnce)
}

// wsNet is the compositor CPU a round added over the idle compositor.
func wsNet(raw, idle float64) float64 { return math.Max(0, raw-idle) }

// sampleIdle waits d.settle, then measures WindowServer for d.seconds; it
// refuses while a scene program runs.
func sampleIdle(d roundDeps) idleSample {
	d.sleep(d.settle)
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
	t1 := d.now()
	el := t1.Sub(t0)
	if el <= 0 {
		return idleSample{err: fmt.Errorf("idle WindowServer sample: no time elapsed")}
	}
	return idleSample{cores: (w1 - w0).Seconds() / el.Seconds(), midNs: t0.Add(el / 2).UnixNano()}
}

// combineBaseline is the round's baseline from its before and after samples.
func combineBaseline(before, after idleSample) WSBaseline {
	b := WSBaseline{BeforeCores: before.cores, AfterCores: after.cores, BeforeMidNs: before.midNs, AfterMidNs: after.midNs,
		ok: [2]bool{before.err == nil, after.err == nil}}
	var why []string
	if before.err != nil {
		why = append(why, "before: "+before.err.Error())
	}
	if after.err != nil {
		why = append(why, "after: "+after.err.Error())
	}
	switch {
	case b.ok[0] && b.ok[1]:
		b.MeanCores = (before.cores + after.cores) / 2
		b.Drift = relDiff(before.cores, after.cores)
		larger := math.Max(before.cores, after.cores)
		if lim := math.Max(idleDriftFloor, idleDriftShare*larger); math.Abs(before.cores-after.cores) > lim {
			why = append(why, fmt.Sprintf("idle drift %.3f → %.3f cores (limit %.3f)", before.cores, after.cores, lim))
		}
	case b.ok[0]:
		b.MeanCores = before.cores
	case b.ok[1]:
		b.MeanCores = after.cores
	}
	if m := math.Max(before.cores, after.cores); m > idleWSLimit {
		why = append(why, fmt.Sprintf("idle %.3f cores above %.2f", m, idleWSLimit))
	}
	b.Cores = b.MeanCores
	b.Disturbed = strings.Join(why, "; ")
	return b
}

// idleAt is the idle rate at midNs: before and after interpolated (clamped
// to the two), or the mean when either sample or a time is missing.
func (b WSBaseline) idleAt(midNs int64) float64 {
	if !b.ok[0] || !b.ok[1] || midNs <= 0 || b.AfterMidNs <= b.BeforeMidNs {
		return b.MeanCores
	}
	f := float64(midNs-b.BeforeMidNs) / float64(b.AfterMidNs-b.BeforeMidNs)
	f = math.Min(1, math.Max(0, f))
	return b.BeforeCores + f*(b.AfterCores-b.BeforeCores)
}

// applyBaseline records the idle and net WindowServer CPU of r (the raw
// delta stays) and marks r disturbed when the background compositor was.
func applyBaseline(r *Round, b WSBaseline) {
	var mid int64
	if r.SampleStartNs > 0 && r.SampleEndNs > r.SampleStartNs {
		mid = r.SampleStartNs + (r.SampleEndNs-r.SampleStartNs)/2
	}
	b.Cores = b.idleAt(mid)
	r.WSBaseline = &b
	r.WindowServerIdleSeconds = b.Cores * r.Seconds
	r.WindowServerNetSeconds = wsNet(r.WindowServerSeconds, r.WindowServerIdleSeconds)
	if b.Disturbed != "" {
		r.Disturbed = true
		r.Notes = append(r.Notes, "background WindowServer: "+b.Disturbed)
	}
}

// runSceneRound measures one round of a scene: the load gate, an idle
// WindowServer sample, each framework in turn (a load-disturbed attempt is
// measured again, up to maxRetries more times), another idle sample. A
// round the background compositor disturbed is measured again as a whole,
// load gate included, up to maxRetries more times. emit receives every
// attempt, once the round's second idle sample is in.
func runSceneRound(d roundDeps, scene string, round int, fws []string, emit func(Round) error) error {
	next := map[string]int{} // each framework's next attempt number
	for try := 0; try <= maxRetries; try++ {
		if d.gate != nil {
			d.gate()
		}
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

// sceneIdle is the median idle compositor (cores) of a scene's usable
// rounds (ready, not disturbed: a disturbed round's baseline may be a
// failed 0), and how many rounds it covers.
func sceneIdle(rs []Round) (float64, int) {
	var idle []float64
	for _, r := range usable(rs) {
		if r.WSBaseline != nil {
			idle = append(idle, r.WSBaseline.MeanCores)
		}
	}
	if len(idle) == 0 {
		return 0, 0
	}
	return median(idle), len(idle)
}

// assessCompositor judges a scene's compositor (R46) and returns what its
// verdict lines append. Adoptability follows the measured idle compositor
// only: a median idle above busyIdleCores marks the run not adoptable
// ("compositor busy"). A screen-share program running (a daemon may stay
// with no viewer) is a note, as is an unknown screen-share state (ps
// failed); the notes also go to m.CompositorNotes.
func assessCompositor(m *Meta, scene string, rs []Round) string {
	idle, n := sceneIdle(rs)
	var parts []string
	if n > 0 && idle > busyIdleCores {
		parts = append(parts, fmt.Sprintf("compositor busy: idle WindowServer %.3f cores > %.2f", idle, busyIdleCores))
		m.markNotAdoptable("compositor busy")
	}
	var names []string
	for _, x := range append(append([]string{}, m.ScreenSharing...), m.ScreenSharingFinish...) {
		if !contains(names, x) {
			names = append(names, x)
		}
	}
	var notes []string
	if len(names) > 0 {
		notes = append(notes, fmt.Sprintf("screen-share app running: %s, idle %.3f cores", strings.Join(names, ", "), idle))
	}
	if m.ScreenSharingError != "" || m.ScreenSharingFinishError != "" {
		notes = append(notes, "screen-share state unknown")
	}
	for _, note := range notes {
		m.addCompositorNote(scene + ": " + note)
	}
	parts = append(parts, notes...)
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

// checkIdle is -check-idle: one settle wait and one idle WindowServer
// sample, no window and no program; it prints the idle load, the
// threshold, pass or fail and the screen-share programs found, and passes
// when the sample worked and the idle load is at most busyIdleCores.
func checkIdle(d roundDeps, procs []string, procErr error, w io.Writer) bool {
	s := sampleIdle(d)
	switch {
	case procErr != nil:
		fmt.Fprintf(w, "screen-share processes: unknown (%v)\n", procErr)
	case len(procs) == 0:
		fmt.Fprintln(w, "screen-share processes: none")
	default:
		fmt.Fprintf(w, "screen-share processes: %s (left alone)\n", strings.Join(procs, ", "))
	}
	if s.err != nil {
		fmt.Fprintf(w, "idle WindowServer: sample failed: %v\nFAIL\n", s.err)
		return false
	}
	pass := s.cores <= busyIdleCores
	verdict := "PASS"
	if !pass {
		verdict = "FAIL"
	}
	fmt.Fprintf(w, "idle WindowServer: %.3f cores over %s (threshold %.2f)\n%s\n", s.cores, d.seconds, busyIdleCores, verdict)
	return pass
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
