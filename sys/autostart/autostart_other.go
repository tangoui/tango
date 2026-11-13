//go:build !(darwin && !ios) && !(linux && !android) && !windows

package autostart

func enable(Options, []string) error  { return ErrUnsupported }
func disable(Options) error           { return ErrUnsupported }
func isEnabled(Options) (bool, error) { return false, ErrUnsupported }
