//go:build !windows

package main

import (
	"errors"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// ttyGuard keeps tan's terminal usable while it runs apps. A program can
// give the terminal's foreground to a process group of its own (an
// interactive shell started with job control, such as `zsh -i` probing
// PATH) and exit without handing it back. The terminal then sends Ctrl-C
// to a group that no longer exists and tan never sees it. check takes the
// foreground back once its holder is gone.
type ttyGuard struct {
	on   bool // stdin is a terminal and tan started in its foreground
	pgrp int
}

func newTTYGuard() *ttyGuard {
	fg, err := unix.IoctlGetInt(0, unix.TIOCGPGRP)
	pg := unix.Getpgrp()
	return &ttyGuard{on: err == nil && fg == pg, pgrp: pg}
}

// check reclaims the foreground if it belongs to a group with no process
// left. A live foreground group (one the user is typing to) is left alone.
func (g *ttyGuard) check() {
	if !g.on {
		return
	}
	fg, err := unix.IoctlGetInt(0, unix.TIOCGPGRP)
	if err != nil || fg == g.pgrp || fg <= 0 {
		return
	}
	if err := unix.Kill(-fg, 0); !errors.Is(err, unix.ESRCH) {
		return
	}
	// Setting the foreground from a background group raises SIGTTOU.
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	if err := unix.IoctlSetPointerInt(0, unix.TIOCSPGRP, g.pgrp); err == nil {
		warnf("An app took over the terminal and exited; Ctrl-C works again")
	}
}
