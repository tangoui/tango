// Package background runs work while the app is in the background:
// periodic content refresh and longer maintenance tasks, scheduled with
// the system's task scheduler (BGTaskScheduler on iOS, JobScheduler on
// Android) and run when the system decides.
//
//	func main() {
//	    app := tango.New(...)
//	    background.OnTask(app, func(ctx context.Context, t background.Task) error {
//	        return syncInbox(ctx) // stop when ctx is done
//	    })
//	    app.OnReady(func() {
//	        go background.Schedule(app, background.Request{Kind: background.Refresh, Earliest: 15 * time.Minute})
//	        ...
//	    })
//	    app.Run()
//	}
//
// The system may launch the app just to run a task: main and App.Run run
// as usual (OnReady included), but no page is on screen. Register OnTask
// in main, before Run, so tasks of such a launch are not missed. The task
// function runs on its own goroutine; its context is canceled when the
// system wants the task to stop (iOS gives a refresh about 30 seconds,
// Android up to 10 minutes) or at the deadline. Return nil for success.
// Requests are one-shot: schedule the next one from the task.
//
// The app must declare the "background" capability in tan.json
// "permissions" (iOS: UIBackgroundModes fetch/processing and the task
// identifiers <id>.tango.refresh / <id>.tango.processing in Info.plist;
// Android: the host's JobService and RECEIVE_BOOT_COMPLETED). Without it
// Schedule returns ErrNotConfigured on iOS.
//
// Testing on iOS: the simulator never runs BGTasks by itself. Pause the app
// in the debugger and run
//
//	e -l objc -- (void)[[BGTaskScheduler sharedScheduler] _simulateLaunchForTaskWithIdentifier:@"<id>.tango.refresh"]
//
// On Android: adb shell cmd jobscheduler run -f <applicationId> <job id>
// (1 refresh, 2 processing).
//
// Schedule and Cancel return ErrUnsupported on desktop and
// tango.ErrNotRunning after the app quit.
package background

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported   = platform.ErrUnsupported
	ErrNotConfigured = platform.ErrNotConfigured
)

// Kind is the kind of a task. Each kind has at most one pending request.
type Kind = platform.BackgroundKind

const (
	Refresh    = platform.BackgroundRefresh    // short, frequent (content refresh)
	Processing = platform.BackgroundProcessing // longer, typically while idle
)

// Request asks the system to run a task of Kind later.
type Request = platform.BackgroundRequest

// Task describes a running task.
type Task struct {
	Kind     Kind
	Deadline time.Time // when the system stops it at the latest (zero: unknown)
}

// defaultBudget bounds a task whose deadline the backend does not know.
var defaultBudget = map[Kind]time.Duration{Refresh: 25 * time.Second, Processing: 9 * time.Minute}

// Schedule asks the system to run a task of r.Kind, replacing a pending
// request of that kind.
func Schedule(a *tango.App, r Request) error {
	bt, err := capcall.Backend[platform.BackgroundTasks](a)
	if err != nil {
		return err
	}
	_, err = capcall.Sync(a, func() (struct{}, error) { return struct{}{}, bt.ScheduleBackground(r) })
	return err
}

// Cancel withdraws the pending request of kind k.
func Cancel(a *tango.App, k Kind) error {
	bt, err := capcall.Backend[platform.BackgroundTasks](a)
	if err != nil {
		return err
	}
	_, err = capcall.Sync(a, func() (struct{}, error) { return struct{}{}, bt.CancelBackground(k) })
	return err
}

type state struct {
	mu      sync.Mutex
	fn      func(context.Context, Task) error
	running map[int64]context.CancelFunc
}

var states sync.Map // *tango.App -> *state

// OnTask sets the function that runs tasks (replacing an earlier one). It
// does nothing where the backend has no background tasks.
func OnTask(a *tango.App, fn func(ctx context.Context, t Task) error) {
	bt, ok := a.Backend().(platform.BackgroundTasks)
	if !ok {
		return
	}
	v, loaded := states.LoadOrStore(a, &state{running: map[int64]context.CancelFunc{}})
	st := v.(*state)
	st.mu.Lock()
	st.fn = fn
	st.mu.Unlock()
	if loaded {
		return
	}
	// The listener runs on the main thread, where the backend may be used.
	drain := func() {
		for _, t := range bt.TakeBackgroundTasks() {
			st.handle(a, bt, t)
		}
	}
	a.OnEventKind(platform.AppBackgroundTask, func(tango.AppEvent) { drain() })
	// Tasks whose event went by before this registration (the core keeps
	// unclaimed events only while the app launches) are still queued in
	// the backend.
	a.Post(drain)
}

// handle runs on the main thread.
func (st *state) handle(a *tango.App, bt platform.BackgroundTasks, t platform.BackgroundTask) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if t.Expired {
		if cancel := st.running[t.ID]; cancel != nil {
			cancel()
		}
		return
	}
	if _, dup := st.running[t.ID]; dup {
		return
	}
	deadline := t.Deadline
	if deadline.IsZero() {
		deadline = time.Now().Add(defaultBudget[t.Kind])
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	st.running[t.ID] = cancel
	fn := st.fn
	go func() {
		err := run(a, ctx, fn, Task{Kind: t.Kind, Deadline: deadline})
		cancel()
		st.mu.Lock()
		delete(st.running, t.ID)
		st.mu.Unlock()
		tango.Await(a, func() struct{} {
			bt.FinishBackgroundTask(t.ID, err == nil)
			return struct{}{}
		})
	}()
}

type panicked struct{ v any }

func (p panicked) Error() string { return "background: task panicked" }

// run calls fn; a panic in it fails the task and is reported to a's panic
// handling (Where "background task") without ending the process: the
// system must still hear that the task finished.
func run(a *tango.App, ctx context.Context, fn func(context.Context, Task) error, t Task) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = panicked{v}
			if corehook.ReportPanic != nil {
				corehook.ReportPanic(a, "background task", v, debug.Stack())
			}
		}
	}()
	if fn == nil {
		return context.Canceled
	}
	return fn(ctx, t)
}
