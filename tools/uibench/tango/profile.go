package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime/pprof"
	"sync"
	"syscall"
)

// profileEnv names the opt-in CPU profile: UIBENCH_CPUPROFILE=<path> makes
// the program write a pprof CPU profile from ready until it exits. It is a
// manual diagnostic aid: the runner clears it from measurement runs and
// sets it only with -profile-dir (README, "Profiling the Tango program").
const profileEnv = "UIBENCH_CPUPROFILE"

// cpuProfile is the profile being written, if any.
type cpuProfile struct {
	path string

	mu      sync.Mutex
	file    *os.File
	started bool // start ran (even if it failed)
	stopped bool
}

// newCPUProfile is the profile UIBENCH_CPUPROFILE asks for, or nil. When it
// asks for one, SIGTERM and SIGINT stop the profile and exit (the runner
// ends a scene program with SIGTERM), so the file is complete.
func newCPUProfile(getenv func(string) string) *cpuProfile {
	path := getenv(profileEnv)
	if path == "" {
		return nil
	}
	return &cpuProfile{path: path}
}

// handleSignals stops the profile on SIGTERM or SIGINT, then exits 0.
func (p *cpuProfile) handleSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		p.stop()
		os.Exit(0)
	}()
}

// start begins profiling, once: at the first frame reported, which is
// ready. A profile that cannot be written is reported on standard error
// and never stops the scene.
func (p *cpuProfile) start() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.stopped {
		return
	}
	p.started = true
	f, err := os.Create(p.path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "uibench tango: cpu profile:", err)
		return
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		fmt.Fprintln(os.Stderr, "uibench tango: cpu profile:", err)
		f.Close()
		return
	}
	p.file = f
}

// stop ends the profile and closes its file; later calls do nothing.
func (p *cpuProfile) stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.stopped = true
	if p.file == nil {
		return
	}
	pprof.StopCPUProfile()
	if err := p.file.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "uibench tango: cpu profile:", err)
	}
	p.file = nil
}
