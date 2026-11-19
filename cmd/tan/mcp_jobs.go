package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// Job output limits: what a job keeps, and what status returns.
const (
	jobOutputKeep     = 256 << 10
	jobStatusLines    = 200
	jobStatusMaxBytes = 64 << 10
	jobStopGrace      = 5 * time.Second
)

// selfExe is the tan executable jobs run; tests replace it.
var selfExe = os.Executable

// selfEnv is added to every job's environment. It is nil in production;
// tests that run the test binary as tan set TAN_TEST_MAIN=1 here.
var selfEnv []string

// jobInfo describes a job for the MCP tools.
type jobInfo struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Args      []string  `json:"args"`
	Dir       string    `json:"dir"`
	State     string    `json:"state"` // running, exited, stopped
	ExitCode  *int      `json:"exitCode,omitempty"`
	Started   time.Time `json:"started"`
	Output    string    `json:"output,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}

// jobManager runs tan subcommands (build, package, dev, ...) as background
// child processes, each in its own process group.
type jobManager struct {
	mu     sync.Mutex
	next   int
	jobs   map[string]*job
	order  []*job
	closed bool
}

type job struct {
	id, kind, dir string
	args          []string
	started       time.Time
	cmd           *exec.Cmd
	group         *jobGroup
	out           *tailBuffer
	done          chan struct{} // closed when the process was reaped
	outDone       chan struct{} // closed when every writer closed the output pipe

	mu       sync.Mutex
	stopping bool // stop was asked: the exit is a stop
	state    string
	exitCode *int
}

func newJobManager() *jobManager { return &jobManager{jobs: map[string]*job{}} }

// start runs "tan args..." in dir with env added to the environment.
func (m *jobManager) start(kind, dir string, args []string, env []string) (jobInfo, error) {
	exe, err := selfExe()
	if err != nil {
		return jobInfo{}, fmt.Errorf("finding the tan executable: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return jobInfo{}, errShuttingDown
	}

	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	// os/exec keeps the last of duplicate variables: env cannot turn off
	// the background launch.
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "TANGO_LAUNCH_BACKGROUND=1")
	cmd.Env = append(cmd.Env, selfEnv...)
	// Our own pipe, not an io.Writer: Wait then returns when tan exits,
	// not when the last process that inherited the output closes it.
	pr, pw, err := os.Pipe()
	if err != nil {
		return jobInfo{}, err
	}
	cmd.Stdout, cmd.Stderr = pw, pw
	prepareJobGroup(cmd)
	err = cmd.Start()
	pw.Close()
	if err != nil {
		pr.Close()
		return jobInfo{}, err
	}

	m.next++
	j := &job{
		id: "job-" + strconv.Itoa(m.next), kind: kind, dir: dir, args: slices.Clone(args),
		started: time.Now(), cmd: cmd, group: attachJobGroup(cmd), out: newTailBuffer(jobOutputKeep),
		done: make(chan struct{}), outDone: make(chan struct{}), state: "running",
	}
	go func() {
		defer close(j.outDone)
		defer pr.Close()
		_, _ = io.Copy(j.out, pr)
	}()
	go func() {
		_ = cmd.Wait()
		code := cmd.ProcessState.ExitCode() // -1: ended by a signal
		j.mu.Lock()
		j.exitCode = &code
		j.state = "exited"
		if j.stopping {
			j.state = "stopped"
		}
		j.mu.Unlock()
		close(j.done)
	}()
	m.jobs[j.id] = j
	m.order = append(m.order, j)
	return j.info(), nil
}

func (m *jobManager) get(id string) (*job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobs[id]; j != nil {
		return j, nil
	}
	return nil, fmt.Errorf("no job %q", id)
}

// status returns the job with the last lines of its output (0: 200), at
// most 64 KiB of it.
func (m *jobManager) status(id string, lines int) (jobInfo, error) {
	j, err := m.get(id)
	if err != nil {
		return jobInfo{}, err
	}
	if lines <= 0 {
		lines = jobStatusLines
	}
	info := j.info()
	info.Output, info.Truncated = j.out.Lines(lines, jobStatusMaxBytes)
	return info, nil
}

// list returns every job, oldest first, without output.
func (m *jobManager) list() []jobInfo {
	m.mu.Lock()
	jobs := slices.Clone(m.order)
	m.mu.Unlock()
	infos := make([]jobInfo, 0, len(jobs))
	for _, j := range jobs {
		infos = append(infos, j.info())
	}
	return infos
}

// stop ends the job's process group: a polite signal first, then a kill
// after 5 s. Stopping a job that already ended only kills what it left
// running in its group. Processes the job put in groups of their own (tan
// build and tan package run the tan.json front-end commands so) are not
// signalled here: tan ends them itself when the polite signal cancels its
// context (signalContext in build.go).
func (m *jobManager) stop(id string) (jobInfo, error) {
	j, err := m.get(id)
	if err != nil {
		return jobInfo{}, err
	}
	j.stop()
	return j.info(), nil
}

// stopAll stops every job, in parallel, and refuses new ones. Processes an
// ended job left behind are killed too: on Unix only while they still hold
// its output pipe; on Windows through the job object, which is closed.
func (m *jobManager) stopAll() {
	m.mu.Lock()
	m.closed = true
	jobs := slices.Clone(m.order)
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Go(func() {
			j.stop()
			j.group.release()
		})
	}
	wg.Wait()
}

// killAll kills every job's process group at once and refuses new jobs
// (a second signal while stopAll runs).
func (m *jobManager) killAll() {
	m.mu.Lock()
	m.closed = true
	jobs := slices.Clone(m.order)
	m.mu.Unlock()
	for _, j := range jobs {
		j.group.kill()
		j.group.release()
	}
}

func (j *job) stop() {
	select {
	case <-j.done:
		select {
		case <-j.outDone:
		default:
			j.group.kill() // the leader is gone; something still holds the output
		}
		return
	default:
	}
	j.mu.Lock()
	j.stopping = true
	j.mu.Unlock()
	if j.group.terminate() == nil {
		// Wait for the leader and for everything holding its output.
		expired := make(chan struct{})
		t := time.AfterFunc(jobStopGrace, func() { close(expired) })
		defer t.Stop()
		select {
		case <-j.done:
		case <-expired:
		}
		select {
		case <-j.outDone:
		case <-expired:
		}
	}
	j.group.kill() // whatever is left
	<-j.done
}

func (j *job) info() jobInfo {
	j.mu.Lock()
	defer j.mu.Unlock()
	info := jobInfo{
		ID: j.id, Kind: j.kind, Args: slices.Clone(j.args), Dir: j.dir,
		State: j.state, Started: j.started,
	}
	if j.args == nil {
		info.Args = []string{}
	}
	if j.exitCode != nil {
		c := *j.exitCode
		info.ExitCode = &c
	}
	return info
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu      sync.Mutex
	max     int
	b       []byte // the kept bytes are the last max of b
	dropped bool   // Write moved the kept bytes to the front, dropping some
}

func newTailBuffer(max int) *tailBuffer { return &tailBuffer{max: max} }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	// Let b grow to twice max before moving the kept bytes to the front.
	if len(t.b) > 2*t.max {
		t.b = append(t.b[:0], t.b[len(t.b)-t.max:]...)
		t.dropped = true
	}
	return len(p), nil
}

// kept returns the kept bytes and whether anything before them is gone.
// The caller holds t.mu.
func (t *tailBuffer) kept() ([]byte, bool) {
	if len(t.b) > t.max {
		return t.b[len(t.b)-t.max:], true
	}
	return t.b, t.dropped
}

// Tail returns the last maxBytes bytes (all of them when maxBytes <= 0),
// starting on a rune boundary; truncated says output before it is left out.
func (t *tailBuffer) Tail(maxBytes int) (s string, truncated bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, truncated := t.kept()
	if maxBytes > 0 && len(b) > maxBytes {
		b, truncated = b[len(b)-maxBytes:], true
	}
	if truncated {
		b = runeStart(b)
	}
	return string(b), truncated
}

// Lines returns the last n lines, cut to their last maxBytes bytes (no
// limit when maxBytes <= 0) at a line boundary when there is one; truncated
// says output before it is left out. A line cut by the buffer is dropped.
func (t *tailBuffer) Lines(n, maxBytes int) (s string, truncated bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, truncated := t.kept()
	if truncated {
		b = afterLineStart(b, true)
	}
	// The nth newline from the end, not counting a final one, starts the lines.
	end := len(b)
	if end > 0 && b[end-1] == '\n' {
		end--
	}
	for i := 0; i < n; i++ {
		k := bytes.LastIndexByte(b[:end], '\n')
		if k < 0 {
			end = -1
			break
		}
		end = k
	}
	if end >= 0 {
		b, truncated = b[end+1:], true
	}
	if maxBytes > 0 && len(b) > maxBytes {
		partial := b[len(b)-maxBytes-1] != '\n'
		b, truncated = b[len(b)-maxBytes:], true
		b = afterLineStart(b, partial)
	}
	return string(b), truncated
}

// afterLineStart drops a partial first line from b (when partial), unless
// b is that one line; what is left starts on a rune boundary.
func afterLineStart(b []byte, partial bool) []byte {
	if partial {
		if k := bytes.IndexByte(b, '\n'); k >= 0 && k < len(b)-1 {
			return b[k+1:]
		}
	}
	return runeStart(b)
}

// runeStart skips the continuation bytes of a rune cut at the start of b.
func runeStart(b []byte) []byte {
	for i := 0; i < len(b) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(b[i]) {
			return b[i:]
		}
	}
	return b
}
