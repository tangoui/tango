package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/tangoui/tango/ipc"
)

// term is the "Term" service: a local shell whose stdin and output are the
// byte frames of the stream (Stream.ReadWriter), so the page reads
// stream.readable and writes stream.writable, and a Go client can use
// io.Copy.
//
// The shell runs on plain os/exec pipes, not a pseudo-terminal: no job
// control, no line editing, no full-screen programs (vi, top), and programs
// may buffer their output since it is not a terminal. Enough to run
// commands; a real terminal would need a pty.
//
// It returns nil when the shell exits (the client then reads io.EOF). When
// the stream ends first (closed, canceled, or the page went away) it kills
// the shell together with the command it runs: without job control that
// command is not stopped when only sh dies, and it would keep the output
// pipe open. Since a pipe has no Ctrl-C, ending the stream is the way to
// stop a runaway command. On Windows cmd.exe prints in the console code
// page, so non-ASCII output may not decode as UTF-8 in the page.
func term(s *ipc.Stream, _ json.RawMessage) error {
	// The stream's context is done once the stream ended; cmd.Cancel, set
	// by shellGroup, then kills the whole process tree.
	cmd := exec.CommandContext(s.Context(), shellPath(), shellArgs()...)
	cmd.Env = append(os.Environ(), "TERM=dumb")
	shellGroup(cmd)
	// Once the shell is gone, stop waiting for output from a process that
	// escaped the kill and still holds the pipe.
	cmd.WaitDelay = 2 * time.Second

	rw := s.ReadWriter()
	// Shell → client: exec copies stdout and stderr (one pipe, since both
	// are the same writer) into the stream; Wait waits for that copy.
	out := &streamOut{w: rw, kill: func() { killShell(cmd) }}
	cmd.Stdout, cmd.Stderr = out, out

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Client → shell, after shellInit. It ends when the client ends its
	// direction (the shell then sees EOF on stdin) or once the stream is
	// closed.
	go func() {
		io.WriteString(stdin, shellInit)
		io.Copy(stdin, rw)
		stdin.Close()
	}()

	cmd.Wait() // an exit status is not a stream error
	return nil
}

// streamOut writes the shell's output to the stream and kills the shell
// once a write fails: nobody reads it any more.
type streamOut struct {
	w    io.Writer
	kill func()
}

func (o *streamOut) Write(p []byte) (int, error) {
	n, err := o.w.Write(p)
	if err != nil {
		o.kill()
	}
	return n, err
}

// shellPath and shellArgs pick the platform's shell: an interactive sh,
// so it prints a prompt even though stdin is not a terminal, or cmd.exe.
func shellPath() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return "/bin/sh"
}

func shellArgs() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	return []string{"-i"}
}
