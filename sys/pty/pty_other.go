//go:build !(darwin && !ios) && !(linux && !android) && !windows

package pty

// sysPTY is never created on platforms without local terminals.
type sysPTY struct{}

// utf8Ctype is empty: nothing is started here.
const utf8Ctype = ""

// Start returns ErrUnsupported: this platform has no local
// pseudo-terminals.
func Start(o Options) (*PTY, error) { return nil, ErrUnsupported }

// DefaultShell returns "", nil: this platform has no local shell.
func DefaultShell() (path string, args []string) { return "", nil }

func (sysPTY) read([]byte) (int, error)    { return 0, ErrUnsupported }
func (sysPTY) write([]byte) (int, error)   { return 0, ErrUnsupported }
func (sysPTY) resize(_, _, _, _ int) error { return ErrUnsupported }
func (sysPTY) close() error                { return ErrUnsupported }
