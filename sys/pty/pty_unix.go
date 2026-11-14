//go:build (darwin && !ios) || (linux && !android)

package pty

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// sysPTY is the controlling side of a /dev/ptmx pair. The descriptor is
// non-blocking and registered with the runtime poller, so closing it
// unblocks a pending Read or Write.
type sysPTY struct {
	master *os.File
}

// Start starts the program described by o on a new pseudo-terminal.
func Start(o Options) (*PTY, error) {
	o, err := o.normalize()
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: "/dev/ptmx", Err: err}
	}
	name, err := unlock(fd)
	if err == nil {
		err = unix.SetNonblock(fd, true)
	}
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	master := os.NewFile(uintptr(fd), "/dev/ptmx")

	sfd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		master.Close()
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	// The program gets its own copies; ours must go, or Read never sees
	// the program hang up.
	tty := os.NewFile(uintptr(sfd), name)
	defer tty.Close()
	// macOS only accepts TIOCSWINSZ once the device is open.
	if err := setSize(sfd, o.Cols, o.Rows, 0, 0); err != nil {
		master.Close()
		return nil, err
	}

	cmd := &exec.Cmd{
		Path:   o.Path,
		Args:   o.Args,
		Env:    o.Env,
		Dir:    o.Dir,
		Stdin:  tty,
		Stdout: tty,
		Stderr: tty,
		// A new session whose controlling terminal is the child's fd 0.
		SysProcAttr: &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0},
	}
	if err := cmd.Start(); err != nil {
		master.Close()
		return nil, err
	}
	p := &PTY{
		sys:  sysPTY{master: master},
		pid:  cmd.Process.Pid,
		done: make(chan struct{}),
	}
	go p.reap(cmd)
	return p, nil
}

// reap waits for the program so that it never lingers as a zombie, and
// records its exit status for Wait.
func (p *PTY) reap(cmd *exec.Cmd) {
	err := cmd.Wait()
	p.code = cmd.ProcessState.ExitCode()
	var ee *exec.ExitError
	if errors.As(err, &ee) && p.code >= 0 {
		err = nil // a plain non-zero exit is reported by the code alone
	}
	p.waitErr = err
	close(p.done)
}

func (s *sysPTY) read(b []byte) (int, error) {
	n, err := s.master.Read(b)
	// Linux fails reads with EIO once the other side is closed by every
	// process; that is the end of the output.
	if err != nil && errors.Is(err, syscall.EIO) {
		err = io.EOF
	}
	return n, err
}

func (s *sysPTY) write(b []byte) (int, error) { return s.master.Write(b) }

func (s *sysPTY) resize(cols, rows, wpx, hpx int) error {
	// SyscallConn, not Fd: Fd would switch the descriptor to blocking
	// mode, after which Close could no longer interrupt Read.
	rc, err := s.master.SyscallConn()
	if err != nil {
		return err
	}
	var ierr error
	if err := rc.Control(func(fd uintptr) { ierr = setSize(int(fd), cols, rows, wpx, hpx) }); err != nil {
		return err
	}
	return ierr
}

func (s *sysPTY) close() error { return s.master.Close() }

// setSize sets the window size of the terminal fd belongs to; the
// kernel sends SIGWINCH to its foreground process group.
func setSize(fd, cols, rows, wpx, hpx int) error {
	ws := &unix.Winsize{Row: uint16(rows), Col: uint16(cols), Xpixel: uint16(wpx), Ypixel: uint16(hpx)}
	if err := unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, ws); err != nil {
		return os.NewSyscallError("ioctl TIOCSWINSZ", err)
	}
	return nil
}

// DefaultShell returns the user's shell, $SHELL, or /bin/sh when it is
// not set, with the arguments to pass as Options.Args.
func DefaultShell() (path string, args []string) {
	path = os.Getenv("SHELL")
	if path == "" {
		path = "/bin/sh"
	}
	return path, []string{path}
}
