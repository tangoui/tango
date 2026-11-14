// Package pty runs programs on local pseudo-terminals, the
// connection a terminal emulator such as ui/terminal needs to run a
// shell.
//
// A [PTY] satisfies the ui/terminal Conn interface: Read returns the
// program's output, Write sends it input, Resize changes the terminal
// size it sees and Close hangs it up.
//
// On macOS and Linux the terminal is a /dev/ptmx pair: the program runs
// in a new session with the subsidiary device as its controlling
// terminal and standard input, output and error. On Windows it is a
// ConPTY pseudo console, which needs Windows 10 1809 or later. Other
// platforms (iOS, Android, js/wasm…) have no local terminals: [Start]
// returns [ErrUnsupported].
//
// The program's environment defaults to the current process's, and
// TERM=xterm-256color and COLORTERM=truecolor are always set.
//
// Experimental: this package's API may change in any 0.x release (see docs/release.md, "API 稳定性").
package pty

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ErrUnsupported is returned by [Start] on platforms without local
// pseudo-terminals.
var ErrUnsupported = errors.New("pty: not supported on this platform")

// Options configures the program [Start] runs.
type Options struct {
	// Path is the program to run. A name without a path separator is
	// looked up in the current process's PATH, as exec.Command does; a
	// relative path with a separator is relative to Dir.
	Path string
	// Args are the program's arguments, starting with the program name
	// (argv[0]). Empty means {Path}. On Windows they are joined into one
	// command line quoted by the MSVCRT rules, which cmd.exe and
	// PowerShell do not follow: never embed untrusted strings in a
	// "cmd /c" or "pwsh -Command" line.
	Args []string
	// Dir is the working directory; empty means the current one.
	Dir string
	// Env is the environment, as "key=value" strings; nil means
	// os.Environ(). TERM and COLORTERM are replaced either way. On
	// Windows SYSTEMROOT is added from the current environment if Env
	// lacks it, as os/exec does.
	Env []string
	// Cols and Rows are the initial terminal size in cells; 0 means
	// 80×24.
	Cols, Rows int
}

// Default terminal size.
const defaultCols, defaultRows = 80, 24

// Variables Start always sets in the program's environment.
var termEnv = []string{"TERM=xterm-256color", "COLORTERM=truecolor"}

// PTY is a program running on a pseudo-terminal. Its methods are safe
// for concurrent use; Read and Write may each be pending in one
// goroutine while another calls Resize or Close.
type PTY struct {
	sys sysPTY
	pid int

	done    chan struct{} // closed once the program has been reaped
	code    int           // exit code, set before done is closed
	waitErr error         // set before done is closed

	closeOnce sync.Once
	closeErr  error
}

// Read reads the program's output. After Close it returns an error
// wrapping os.ErrClosed.
//
// On Unix Read returns io.EOF once the program and every other process
// using the terminal (such as a job it left in the background) have
// exited and their output has been read. On Windows it returns io.EOF
// once the started program has exited: the pseudo console is then
// closed, after its output has been quiet for 100 ms (at most 1 s
// after the exit), and closing it ends any other process still
// attached to it.
func (p *PTY) Read(b []byte) (int, error) { return p.sys.read(b) }

// Write sends input to the program, as typed keys. After Close it
// returns an error wrapping os.ErrClosed.
func (p *PTY) Write(b []byte) (int, error) { return p.sys.write(b) }

// Resize sets the terminal size the program sees, in cells and pixels
// (pixels may be 0). The program is told by SIGWINCH on Unix. Resize
// does not block; it returns an error after Close.
func (p *PTY) Resize(cols, rows, widthPx, heightPx int) error {
	return p.sys.resize(clamp(cols), clamp(rows), clamp(widthPx), clamp(heightPx))
}

// Close closes the terminal and unblocks a pending Read and Write. On
// Unix the kernel then sends SIGHUP to the program's session; on
// Windows the pseudo console is closed, which ends the programs
// attached to it. Close does not wait for the program to exit; see
// [PTY.Wait]. It is safe to call more than once, and every call returns
// the first call's result.
func (p *PTY) Close() error {
	p.closeOnce.Do(func() { p.closeErr = p.sys.close() })
	return p.closeErr
}

// Wait waits for the program to exit and returns its exit code. The
// program is reaped in the background whether or not Wait is called,
// and Wait may be called more than once. A program killed by a signal
// has exit code -1 and err describes the signal; err is also set if
// the exit status could not be obtained. Wait waits only for the
// started program; on Windows its exit also closes the pseudo console,
// ending any other process attached to it (see [PTY.Read]).
func (p *PTY) Wait() (exitCode int, err error) {
	<-p.done
	return p.code, p.waitErr
}

// Pid returns the program's process ID.
func (p *PTY) Pid() int { return p.pid }

// normalize validates o and fills in the defaults.
func (o Options) normalize() (Options, error) {
	if o.Path == "" {
		return o, errors.New("pty: Options.Path is empty")
	}
	if filepath.Base(o.Path) == o.Path {
		lp, err := exec.LookPath(o.Path)
		if err != nil {
			return o, err
		}
		o.Path = lp
	}
	if len(o.Args) == 0 {
		o.Args = []string{o.Path}
	}
	if o.Env == nil {
		o.Env = os.Environ()
	}
	o.Env = environ(o.Env)
	if o.Cols <= 0 {
		o.Cols = defaultCols
	}
	if o.Rows <= 0 {
		o.Rows = defaultRows
	}
	o.Cols, o.Rows = clamp(o.Cols), clamp(o.Rows)
	return o, nil
}

// environ returns env without its TERM and COLORTERM entries, followed
// by termEnv. Windows keys are case-insensitive.
func environ(env []string) []string {
	out := make([]string, 0, len(env)+len(termEnv))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if !isTermKey(key) {
			out = append(out, kv)
		}
	}
	return append(out, termEnv...)
}

func isTermKey(key string) bool {
	for _, kv := range termEnv {
		k, _, _ := strings.Cut(kv, "=")
		if key == k || runtime.GOOS == "windows" && strings.EqualFold(key, k) {
			return true
		}
	}
	return false
}

// clamp limits a size to the 16-bit range the platforms take.
func clamp(n int) int { return max(0, min(n, 0x7fff)) }

// resolvePath returns path relative to dir when it is a relative path
// with a separator, as the child's chdir before exec makes it on Unix.
// Windows resolves the program before the child starts, so it needs
// this explicitly. Rooted paths (/x, \x, C:x, C:\x) are left alone.
func resolvePath(path, dir string) string {
	if dir == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" ||
		strings.HasPrefix(path, "/") || strings.HasPrefix(path, string(filepath.Separator)) {
		return path
	}
	return filepath.Join(dir, path)
}

// addSystemRoot returns env with SYSTEMROOT=root appended if env has no
// SYSTEMROOT (keys compared case-insensitively) and root is not empty.
// Many Windows DLLs fail to load without it.
func addSystemRoot(env []string, root string) []string {
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(k, "SYSTEMROOT") {
			return env
		}
	}
	if root == "" {
		return env
	}
	return append(env, "SYSTEMROOT="+root)
}

// How long the Windows pseudo console is kept open after the program
// exits: until its output has been quiet for quietTime, at most
// maxLinger. conhost renders on a timer, so closing it right at the
// exit can lose the last frame.
const (
	quietTime = 100 * time.Millisecond
	maxLinger = time.Second
)

// lingerDelay returns how much longer to wait, after the program exited
// at exit and the last output was read at last (zero if none), before
// closing the console at now; 0 or less means close now.
func lingerDelay(exit, last, now time.Time) time.Duration {
	if last.Before(exit) {
		last = exit
	}
	until := last.Add(quietTime)
	if limit := exit.Add(maxLinger); until.After(limit) {
		until = limit
	}
	return until.Sub(now)
}
