package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// windowServerCPU is the compositor's cumulative CPU time. WindowServer
// runs as another user, so rusage cannot read it; ps can, to the
// hundredth of a second.
func windowServerCPU() (time.Duration, error) {
	pid, err := exec.Command("pgrep", "-x", "WindowServer").Output()
	if err != nil {
		return 0, fmt.Errorf("pgrep WindowServer: %w", err)
	}
	first := strings.Fields(string(pid))
	if len(first) == 0 {
		return 0, fmt.Errorf("no WindowServer process")
	}
	out, err := exec.Command("ps", "-o", "time=", "-p", first[0]).Output()
	if err != nil {
		return 0, fmt.Errorf("ps WindowServer: %w", err)
	}
	return parsePSTime(string(out))
}

// parsePSTime reads ps's time column: [[dd-]hh:]mm:ss.cc.
func parsePSTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	var days time.Duration
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, fmt.Errorf("ps time %q: %w", s, err)
		}
		days, s = time.Duration(n)*24*time.Hour, rest
	}
	var total float64
	for _, part := range strings.Split(s, ":") {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, fmt.Errorf("ps time %q: %w", s, err)
		}
		total = total*60 + v
	}
	return days + time.Duration(total*float64(time.Second)), nil
}

// bringToFront raises the scene's window, so every framework is measured
// as the active app.
func bringToFront(pid int) error {
	script := fmt.Sprintf(`tell application "System Events" to set frontmost of (first process whose unix id is %d) to true`, pid)
	if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// screenLocked reports whether the console session's screen is locked,
// from the IOConsoleUsers property of the I/O Registry root.
func screenLocked() bool {
	out, err := exec.Command("ioreg", "-n", "Root", "-d1", "-w0").Output()
	return err == nil && strings.Contains(string(out), `"CGSSessionScreenIsLocked"=Yes`)
}

// frontmostPID is the unix id of the frontmost process, from System Events,
// or from lsappinfo when osascript fails.
func frontmostPID() (int, error) {
	script := `tell application "System Events" to get unix id of first process whose frontmost is true`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(out))); perr == nil {
			return pid, nil
		}
	}
	asn, lerr := exec.Command("lsappinfo", "front").Output()
	if lerr != nil {
		return 0, fmt.Errorf("osascript: %v; lsappinfo front: %v", err, lerr)
	}
	info, lerr := exec.Command("lsappinfo", "info", "-only", "pid", strings.TrimSpace(string(asn))).Output()
	if lerr != nil {
		return 0, fmt.Errorf("osascript: %v; lsappinfo info: %v", err, lerr)
	}
	return parseLsappinfoPID(string(info))
}

// parseLsappinfoPID reads `lsappinfo info -only pid`: `"pid"=1234`.
func parseLsappinfoPID(s string) (int, error) {
	_, v, ok := strings.Cut(strings.TrimSpace(s), "=")
	if !ok {
		return 0, fmt.Errorf("lsappinfo pid %q", s)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("lsappinfo pid %q", s)
	}
	return pid, nil
}
