package android

import (
	"fmt"
	"os"

	"github.com/tangoui/tango/internal/platform"
)

// Android log priorities (android/log.h).
const (
	logDebug = 3
	logInfo  = 4
	logWarn  = 5
	logError = 6
)

// logWrite sends one line to the log; logcat on Android (log_android.go).
var logWrite = func(prio int, msg string) {
	fmt.Fprintln(os.Stderr, "tango/android:", msg)
}

func logf(prio int, format string, args ...any) {
	logWrite(prio, fmt.Sprintf(format, args...))
}

// lifecycle turns the Activity callbacks the host reports into the core's
// lifecycle events. The Activity starts (and may already be in the
// foreground) before main.main reaches App.Run, so events reported before
// the backend runs are remembered and replayed, in order, when it starts.
// Main thread only.
type lifecycle struct {
	running    bool // Backend.Run started and the replay was delivered
	foreground bool
	terminated bool // Terminating delivered (or reported before running)
}

// report records ev from the host and returns what to deliver now.
func (l *lifecycle) report(ev platform.LifecycleEvent) []platform.LifecycleEvent {
	if l.terminated {
		return nil
	}
	switch ev {
	case platform.LifecycleStarted:
		return nil // delivered once, by start
	case platform.LifecycleForeground:
		if l.foreground {
			return nil
		}
		l.foreground = true
	case platform.LifecycleBackground:
		if !l.foreground {
			return nil
		}
		l.foreground = false
	case platform.LifecycleTerminating:
		l.terminated = true
	default:
		return nil
	}
	if !l.running {
		return nil
	}
	return []platform.LifecycleEvent{ev}
}

// start marks the backend running and returns the replay: Started, then
// Foreground if the Activity is resumed, then Terminating if it already
// went away.
func (l *lifecycle) start() []platform.LifecycleEvent {
	if l.running {
		return nil
	}
	l.running = true
	evs := []platform.LifecycleEvent{platform.LifecycleStarted}
	if l.terminated {
		return append(evs, platform.LifecycleTerminating)
	}
	if l.foreground {
		evs = append(evs, platform.LifecycleForeground)
	}
	return evs
}
