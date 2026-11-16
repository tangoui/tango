// Command runner measures the uibench scene programs from outside: each
// round starts every framework's program afresh, in turns, waits for its
// "bench ready", lets it settle, and samples process CPU, WindowServer CPU,
// frame rate, work, frame intervals and memory. Every attempt, disturbed or
// not, goes to the results file; the judge reads only the calm ones.
//
//	runner -bin ../bin -scenes counter,list,grid -frameworks tango,mygo,nsgo \
//		-rounds 5 -settle 3s -seconds 10 -out ../results/<date>.json
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Load gate, as uiperf's: wait for a calm machine before a round, and mark
// an attempt disturbed when the 1-minute load reached the limit during it.
const (
	loadLimit    = 6.0
	loadWaitMax  = 10 * time.Minute
	loadPoll     = 30 * time.Second
	readyTimeout = 30 * time.Second
	maxRetries   = 3 // re-measurements of a disturbed attempt
)

// Round is one attempt of one framework on one scene.
type Round struct {
	Scene     string `json:"scene"`
	Framework string `json:"framework"`
	Round     int    `json:"round"`
	Attempt   int    `json:"attempt"`
	Build     string `json:"build,omitempty"` // how the program was built (CGO)

	Ready     bool    `json:"ready"`
	Renderer  string  `json:"renderer"`
	StartupMs float64 `json:"startup_ms"` // exec to "bench ready"

	Seconds             float64 `json:"seconds"`        // sampled wall time
	CPUSeconds          float64 `json:"cpu_s"`          // process user+system
	WindowServerSeconds float64 `json:"windowserver_s"` // compositor delta
	FPS                 float64 `json:"fps"`            // mean of the per-second reports
	WorkPerSec          float64 `json:"work_per_s"`     // mean of the per-second reports
	P50us               float64 `json:"p50_us"`         // frame interval
	P99us               float64 `json:"p99_us"`         // frame interval
	Intervals           int     `json:"intervals"`      // interval samples
	Footprint           int64   `json:"footprint"`      // phys_footprint at the end
	PeakFootprint       int64   `json:"peak_footprint"` // highest seen while sampling

	LoadBefore float64 `json:"load_before"`
	LoadAfter  float64 `json:"load_after"`
	Disturbed  bool    `json:"disturbed"`
	// ScreenLocked: the session's screen was locked, so windows were
	// occluded and frame-clock-driven frameworks may not draw at all.
	ScreenLocked bool     `json:"screen_locked,omitempty"`
	Error        string   `json:"error,omitempty"`
	Notes        []string `json:"notes,omitempty"`
}

func (r Round) cpu() float64 {
	if r.Seconds <= 0 {
		return 0
	}
	return (r.CPUSeconds + r.WindowServerSeconds) / r.Seconds
}

// workPerFrame is what the judge compares between frameworks: the same
// scene must change the same number of items per frame. Per second would
// confuse work with frame rate, which the judge weighs on its own.
func (r Round) workPerFrame() float64 {
	if r.FPS > 0 {
		return r.WorkPerSec / r.FPS
	}
	return r.WorkPerSec
}
func (r Round) fps() float64       { return r.FPS }
func (r Round) p99() float64       { return r.P99us }
func (r Round) footprint() float64 { return float64(r.Footprint) }

// usable drops the attempts the judge must not read: disturbed, failed or
// measured behind a locked screen.
func usable(rs []Round) []Round {
	var out []Round
	for _, r := range rs {
		if !r.Disturbed && !r.ScreenLocked && r.Error == "" && r.Ready {
			out = append(out, r)
		}
	}
	return out
}

// event is one parsed protocol line.
type event struct {
	kind      string // ready, renderer, frames, work, intervals
	value     float64
	name      string
	intervals []int64
}

// parseLine reads one line of the scene protocol; other output (framework
// logs) is not an event.
func parseLine(line string) (event, bool) {
	rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "bench ")
	if !ok {
		return event{}, false
	}
	key, arg, _ := strings.Cut(rest, " ")
	switch key {
	case "ready":
		return event{kind: "ready"}, arg == ""
	case "renderer":
		return event{kind: "renderer", name: strings.TrimSpace(arg)}, strings.TrimSpace(arg) != ""
	case "frames", "work":
		v, err := strconv.ParseFloat(strings.TrimSpace(arg), 64)
		return event{kind: key, value: v}, err == nil
	case "intervals_us":
		e := event{kind: "intervals"}
		arg = strings.TrimSpace(arg)
		if arg == "" {
			return e, true
		}
		for _, f := range strings.Split(arg, ",") {
			n, err := strconv.ParseInt(f, 10, 64)
			if err != nil {
				return event{}, false
			}
			e.intervals = append(e.intervals, n)
		}
		return e, true
	}
	return event{}, false
}

// monitor collects one program's protocol lines.
type monitor struct {
	ready chan struct{}
	done  chan struct{} // stdout closed: the program exited

	mu        sync.Mutex
	isReady   bool
	readyAt   time.Time
	renderer  string
	sampling  bool
	frames    []float64
	work      []float64
	intervals []float64
}

func newMonitor() *monitor {
	return &monitor{ready: make(chan struct{}), done: make(chan struct{})}
}

func (m *monitor) read(r io.Reader) {
	defer close(m.done)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20) // a second of intervals at a high rate
	for sc.Scan() {
		e, ok := parseLine(sc.Text())
		if !ok {
			continue
		}
		m.mu.Lock()
		switch e.kind {
		case "ready":
			if !m.isReady {
				m.isReady, m.readyAt = true, time.Now()
				close(m.ready)
			}
		case "renderer":
			m.renderer = e.name
		case "frames":
			if m.sampling {
				m.frames = append(m.frames, e.value)
			}
		case "work":
			if m.sampling {
				m.work = append(m.work, e.value)
			}
		case "intervals":
			if m.sampling {
				for _, us := range e.intervals {
					m.intervals = append(m.intervals, float64(us))
				}
			}
		}
		m.mu.Unlock()
	}
}

func (m *monitor) setSampling(on bool) {
	m.mu.Lock()
	m.sampling = on
	m.mu.Unlock()
}

// tail keeps the last bytes of a program's standard error for the report.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2048 {
		t.buf = t.buf[len(t.buf)-2048:]
	}
	t.mu.Unlock()
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

type config struct {
	bin      string
	settle   time.Duration
	seconds  time.Duration
	builds   map[string]string
	loadRead func() (float64, error)
}

// runOnce starts one scene program, measures it and stops it.
func runOnce(cfg config, fw, scene string) Round {
	r := Round{Scene: scene, Framework: fw, Build: cfg.builds[fw], ScreenLocked: screenLocked()}
	cmd := exec.Command(filepath.Join(cfg.bin, fw), "-scene", scene)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr := &tail{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	m := newMonitor()
	start := time.Now()
	if err := cmd.Start(); err != nil {
		r.Error = err.Error()
		return r
	}
	go m.read(stdout)
	defer stop(cmd, m)
	pid := cmd.Process.Pid

	select {
	case <-m.ready:
	case <-m.done:
		r.Error = "exited before ready: " + stderr.String()
		return r
	case <-time.After(readyTimeout):
		r.Error = "no bench ready within " + readyTimeout.String() + ": " + stderr.String()
		return r
	}
	m.mu.Lock()
	r.Ready, r.StartupMs = true, float64(m.readyAt.Sub(start).Microseconds())/1000
	m.mu.Unlock()
	if err := bringToFront(pid); err != nil {
		r.Notes = append(r.Notes, "frontmost: "+err.Error())
	}
	time.Sleep(cfg.settle)

	r.LoadBefore, _ = cfg.loadRead()
	c0, err1 := procCPU(pid)
	w0, err2 := windowServerCPU()
	m.setSampling(true)
	t0 := time.Now()
	tick := time.NewTicker(250 * time.Millisecond)
	for deadline := t0.Add(cfg.seconds); time.Now().Before(deadline); {
		select {
		case <-tick.C:
			if f, err := procFootprint(pid); err == nil {
				r.PeakFootprint = max(r.PeakFootprint, f)
			}
		case <-m.done:
			tick.Stop()
			r.Error = "exited while sampling: " + stderr.String()
			return r
		}
	}
	tick.Stop()
	c1, err3 := procCPU(pid)
	w1, err4 := windowServerCPU()
	r.Seconds = time.Since(t0).Seconds()
	m.setSampling(false)
	r.Footprint, _ = procFootprint(pid)
	r.PeakFootprint = max(r.PeakFootprint, r.Footprint)
	r.LoadAfter, _ = cfg.loadRead()
	r.ScreenLocked = r.ScreenLocked || screenLocked()
	if err := errors.Join(err1, err2, err3, err4); err != nil {
		r.Error = err.Error()
	}

	r.CPUSeconds = (c1 - c0).Seconds()
	r.WindowServerSeconds = (w1 - w0).Seconds()
	m.mu.Lock()
	r.Renderer = m.renderer
	r.FPS = mean(m.frames)
	r.WorkPerSec = mean(m.work)
	r.Intervals = len(m.intervals)
	r.P50us = percentile(m.intervals, 50)
	r.P99us = percentile(m.intervals, 99)
	m.mu.Unlock()
	r.Disturbed = r.LoadBefore >= loadLimit || r.LoadAfter >= loadLimit
	return r
}

// stop ends the program and its process group, then reaps it.
func stop(cmd *exec.Cmd, m *monitor) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	exited := make(chan struct{})
	go func() { <-m.done; _ = cmd.Wait(); close(exited) }()
	select {
	case <-exited:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-exited
	}
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// rotate is round's turn order: each framework starts each position once
// every len(fws) rounds.
func rotate(fws []string, round int) []string {
	out := make([]string, len(fws))
	for i := range fws {
		out[i] = fws[(i+round)%len(fws)]
	}
	return out
}

// Results is the file the runner writes, with every attempt.
type Results struct {
	Meta     Meta          `json:"meta"`
	Rounds   []Round       `json:"rounds"`
	Verdicts []VerdictLine `json:"verdicts,omitempty"`
}

type Meta struct {
	Started   string            `json:"started"`
	Finished  string            `json:"finished,omitempty"`
	Machine   string            `json:"machine"`
	MemBytes  int64             `json:"mem_bytes"`
	OS        string            `json:"os"`
	Runner    string            `json:"runner_go"`
	Args      []string          `json:"args"`
	Settle    string            `json:"settle"`
	Seconds   string            `json:"seconds"`
	Builds    map[string]string `json:"builds"` // framework -> build line of bin/build-info.txt
	NSGo      string            `json:"nsgo,omitempty"`
	LoadStart float64           `json:"load_start"`
	// ScreenLocked at the start: the run cannot produce usable rounds.
	ScreenLocked bool `json:"screen_locked"`
}

type VerdictLine struct {
	Scene    string  `json:"scene"`
	Opponent string  `json:"opponent"`
	Verdict  Verdict `json:"verdict"`
	Why      string  `json:"why"`
	Rounds   [2]int  `json:"usable_rounds"` // tango, opponent
}

func main() {
	bin := flag.String("bin", "../bin", "directory of the built scene programs")
	scenesFlag := flag.String("scenes", "counter,list,grid", "scenes, comma-separated")
	fwFlag := flag.String("frameworks", "tango,mygo,nsgo", "frameworks, comma-separated; tango is judged against the others")
	rounds := flag.Int("rounds", 5, "rounds per scene")
	settle := flag.Duration("settle", 3*time.Second, "wait after ready before sampling")
	seconds := flag.Int("seconds", 10, "sampling seconds")
	out := flag.String("out", "../results/"+time.Now().Format("2006-01-02-150405")+".json", "results file")
	flag.Parse()

	scenes, fws := split(*scenesFlag), split(*fwFlag)
	if len(scenes) == 0 || len(fws) == 0 || *rounds < 1 || *seconds < 1 {
		fmt.Fprintln(os.Stderr, "runner: need scenes, frameworks, rounds ≥ 1 and seconds ≥ 1")
		os.Exit(2)
	}
	for _, fw := range fws {
		if _, err := os.Stat(filepath.Join(*bin, fw)); err != nil {
			fmt.Fprintf(os.Stderr, "runner: %v (run build.sh)\n", err)
			os.Exit(1)
		}
	}
	cfg := config{bin: *bin, settle: *settle, seconds: time.Duration(*seconds) * time.Second,
		builds: readBuilds(*bin), loadRead: systemLoad}
	res := &Results{Meta: newMeta(cfg)}
	if res.Meta.ScreenLocked {
		fmt.Fprintln(os.Stderr, "runner: the screen is locked: windows are occluded and no round will be usable")
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal(err)
	}

	for _, scene := range scenes {
		for round := range *rounds {
			if err := loadGate(systemLoad, time.Sleep, loadLimit, loadWaitMax); err != nil {
				fmt.Fprintln(os.Stderr, "runner: load gate:", err, "(attempts will be marked disturbed)")
			}
			for _, fw := range rotate(fws, round) {
				for attempt := 0; attempt <= maxRetries; attempt++ {
					r := runOnce(cfg, fw, scene)
					r.Round, r.Attempt = round, attempt
					res.Rounds = append(res.Rounds, r)
					printRound(r)
					if err := write(*out, res); err != nil {
						fatal(err)
					}
					if !r.Disturbed {
						break
					}
				}
			}
		}
	}

	if contains(fws, "tango") {
		for _, scene := range scenes {
			t := pick(res.Rounds, scene, "tango")
			for _, fw := range fws {
				if fw == "tango" {
					continue
				}
				o := pick(res.Rounds, scene, fw)
				v, why := judgeWhy(t, o)
				res.Verdicts = append(res.Verdicts, VerdictLine{Scene: scene, Opponent: fw, Verdict: v, Why: why,
					Rounds: [2]int{len(usable(t)), len(usable(o))}})
				fmt.Printf("verdict %-8s tango vs %-5s %-9s %s\n", scene, fw, v, why)
			}
		}
	}
	res.Meta.Finished = time.Now().Format(time.RFC3339)
	if err := write(*out, res); err != nil {
		fatal(err)
	}
	fmt.Println("results:", *out)
}

func printRound(r Round) {
	status := "ready"
	if !r.Ready {
		status = "NOT-READY"
	}
	if r.Error != "" {
		status += " error=" + r.Error
	}
	if r.Disturbed {
		status += " disturbed"
	}
	if r.ScreenLocked {
		status += " screen-locked"
	}
	fmt.Printf("%-8s %-6s round %d.%d %s renderer=%s startup=%.0fms fps=%.1f work/s=%.0f cpu=%.3f ws=%.3f cores p50=%.0fus p99=%.0fus footprint=%.1fMiB load=%.2f/%.2f\n",
		r.Scene, r.Framework, r.Round, r.Attempt, status, r.Renderer, r.StartupMs, r.FPS, r.WorkPerSec,
		safeDiv(r.CPUSeconds, r.Seconds), safeDiv(r.WindowServerSeconds, r.Seconds),
		r.P50us, r.P99us, float64(r.Footprint)/(1<<20), r.LoadBefore, r.LoadAfter)
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func pick(rs []Round, scene, fw string) []Round {
	var out []Round
	for _, r := range rs {
		if r.Scene == scene && r.Framework == fw {
			out = append(out, r)
		}
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func split(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func write(path string, res *Results) error {
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "runner:", err)
	os.Exit(1)
}

// readBuilds reads bin/build-info.txt, which build.sh writes as
// "<framework> <how it was built>" lines.
func readBuilds(bin string) map[string]string {
	builds := map[string]string{}
	b, err := os.ReadFile(filepath.Join(bin, "build-info.txt"))
	if err != nil {
		return builds
	}
	for _, line := range strings.Split(string(b), "\n") {
		if fw, how, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			builds[fw] = strings.TrimSpace(how)
		}
	}
	return builds
}

func newMeta(cfg config) Meta {
	m := Meta{
		Started: time.Now().Format(time.RFC3339),
		Machine: sysctl("machdep.cpu.brand_string"),
		OS:      runtime.GOOS + "/" + runtime.GOARCH,
		Runner:  runtime.Version(),
		Args:    os.Args[1:],
		Settle:  cfg.settle.String(),
		Seconds: cfg.seconds.String(),
		Builds:  cfg.builds,
	}
	m.MemBytes, _ = strconv.ParseInt(sysctl("hw.memsize"), 10, 64)
	if v, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
		m.OS += " macOS " + strings.TrimSpace(string(v))
	}
	if b, err := os.ReadFile(filepath.Join(cfg.bin, "..", "third_party", "nsgo.version")); err == nil {
		m.NSGo = strings.TrimSpace(string(b))
	}
	m.LoadStart, _ = systemLoad()
	m.ScreenLocked = screenLocked()
	return m
}

func sysctl(name string) string {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseLoadavg reads `sysctl -n vm.loadavg`: "{ 1.40 1.52 1.61 }".
func parseLoadavg(s string) (float64, error) {
	f := strings.Fields(strings.Trim(strings.TrimSpace(s), "{}"))
	if len(f) == 0 {
		return 0, fmt.Errorf("invalid loadavg %q", s)
	}
	return strconv.ParseFloat(f[0], 64)
}

func systemLoad() (float64, error) {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0, err
	}
	return parseLoadavg(string(out))
}

// loadGate waits, polling, until the 1-minute load is below max, for at
// most waitMax.
func loadGate(read func() (float64, error), sleep func(time.Duration), max float64, waitMax time.Duration) error {
	var waited time.Duration
	for {
		v, err := read()
		if err != nil {
			return err
		}
		if v < max {
			return nil
		}
		if waited >= waitMax {
			return fmt.Errorf("load %.2f remains at or above %.2f after %s", v, max, waitMax)
		}
		sleep(loadPoll)
		waited += loadPoll
	}
}
