package tango

// Panic handling at the places where the framework calls application code
// (docs/panics.md). Every such call goes through App.guard (or guardCall),
// which recovers a panic and hands it to App.reportPanic: Options.OnPanic
// when set, otherwise log, keep running in development builds, and show an
// error dialog and exit with status 2 in production builds (the default,
// without -tags tango_dev).

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// Panic is a panic recovered where the framework called application code.
type Panic struct {
	Value any    // the value passed to panic
	Stack []byte // the panicking goroutine's stack trace
	// Where tells what was running: "main-thread callback" (Post, Await,
	// AwaitAsync), "OnReady", "event handler" (OnXxx handlers),
	// "canvas callback", "ui frame", "goroutine" (App.Go), or
	// "binding Service.Method" / "stream Service.Method" (ipc).
	Where string
}

// Where values used by the core.
const (
	whereMain   = "main-thread callback"
	whereReady  = "OnReady"
	whereEvent  = "event handler"
	whereCanvas = "canvas callback"
	whereGo     = "goroutine"
)

// panicExitCode is the process exit status after a fatal panic.
const panicExitCode = 2

var (
	// exitProcess ends the process after a fatal panic; replaced in tests.
	exitProcess = os.Exit
	// releaseBuild is set by panic_release.go in production builds; a
	// variable so tests can take the release path.
	releaseBuild = false
	// fatalDialogWait bounds how long a panicking goroutine waits for the
	// main thread to start showing the error dialog.
	fatalDialogWait = 5 * time.Second
)

func init() {
	corehook.Guard = func(app any, where string, fn func()) bool {
		a, _ := app.(*App)
		if a == nil { // no app to report to: leave the panic to the caller's caller
			fn()
			return true
		}
		return a.guard(where, fn)
	}
	corehook.ReportPanic = func(conn any, where string, value any, stack []byte) bool {
		var a *App
		switch c := conn.(type) {
		case *Surface:
			a = c.app
		case *App: // sys/background: the task already failed
			a = c
		}
		if a == nil {
			return false
		}
		// Bindings run on their own goroutines and their panic already
		// became an error for the caller: report only, never exit.
		a.reportPanic(Panic{Value: value, Stack: stack, Where: where}, false)
		return true
	}
}

// Go runs fn on a new goroutine. A panic in fn is handled like one in a
// callback (Options.OnPanic, Where "goroutine") instead of crashing the
// process. Use it instead of a go statement for application goroutines: a
// panic on a goroutine started with a plain go statement cannot be
// intercepted by the framework and terminates the program at once.
func (a *App) Go(fn func()) {
	go a.guard(whereGo, fn)
}

// guard runs fn, reporting false and handing a panic in it to reportPanic.
// After a panic the caller carries on (with the next handler, the next
// queued function, ...) unless the panic was fatal.
func (a *App) guard(where string, fn func()) (ok bool) {
	defer func() {
		if !ok {
			a.recovered(where, recover())
		}
	}()
	fn()
	return true
}

// guardCall is guard for a callback with a result: def when fn panicked.
func guardCall[T any](a *App, where string, def T, fn func() T) (v T) {
	if !a.guard(where, func() { v = fn() }) {
		return def
	}
	return v
}

// recovered handles the value guard's deferred function recovered; nil
// means fn called runtime.Goexit, which is left alone.
func (a *App) recovered(where string, v any) {
	if v == nil {
		return
	}
	a.reportPanic(Panic{Value: v, Stack: debug.Stack(), Where: where}, true)
}

// reportPanic hands p to Options.OnPanic or the default handling. mayExit
// false (ipc bindings) only reports: keepRunning is ignored.
func (a *App) reportPanic(p Panic, mayExit bool) {
	keep := !releaseBuild
	fn := a.opts.OnPanic
	if fn != nil {
		keep = callOnPanic(fn, p)
	} else {
		logPanic(p)
	}
	if keep || !mayExit {
		return
	}
	if a.dying.Swap(true) {
		return // another panic is already ending the process
	}
	if fn == nil {
		a.showFatal(p)
	}
	exitProcess(panicExitCode)
}

// callOnPanic calls the application's handler. If the handler panics
// itself, both panics are logged and the default decision applies.
func callOnPanic(fn func(Panic) bool, p Panic) (keep bool) {
	defer func() {
		if r := recover(); r != nil {
			logPanic(p)
			logPanic(Panic{Value: r, Stack: debug.Stack(), Where: "OnPanic"})
			keep = !releaseBuild
		}
	}()
	return fn(p)
}

func logPanic(p Panic) {
	slog.Error("tango: panic recovered", "where", p.Where, "panic", fmt.Sprint(p.Value), "stack", string(p.Stack))
}

// showFatal shows "<app> has a problem and needs to close" with a short
// error and waits until the user dismisses it. Without a running app or a
// dialog capability it shows nothing.
func (a *App) showFatal(p Panic) {
	d, ok := a.b.(platform.Dialogs)
	if !ok || !a.ran.Load() || a.state.Load() == stateClosed {
		return
	}
	name := a.opts.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	}
	msg := fmt.Sprint(p.Value)
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300]) + "…"
	}
	text := name + " has encountered a problem and needs to close."
	if a.zhLocale() {
		text = name + " 遇到问题，需要关闭。"
	}
	opts := platform.MessageOptions{Kind: platform.MessageError, Title: name, Message: text + "\n\n" + msg}

	if a.isMain() {
		_, _ = TryAwaitAsync(a, func(done func(struct{})) {
			d.MessageDialog(0, opts, func(int, error) { done(struct{}{}) })
		})
		return
	}
	// Off the main thread: if the main thread does not get to the dialog
	// soon (it may be stuck), exit without it.
	started, closed := make(chan struct{}), make(chan struct{})
	if !a.loop.post(func() {
		close(started)
		d.MessageDialog(0, opts, func(int, error) { close(closed) })
	}) {
		return
	}
	select {
	case <-started:
	case <-a.closedCh:
		return
	case <-time.After(fatalDialogWait):
		return
	}
	select {
	case <-closed:
	case <-a.closedCh:
	}
}

// zhLocale reports whether the user's preferred language is Chinese.
func (a *App) zhLocale() bool {
	var ls []string
	if l, ok := a.b.(platform.Locales); ok {
		ls = l.PreferredLocales()
	} else if src := platform.LocaleSource(); src != nil {
		ls = src()
	} else {
		ls = platform.EnvLocales()
	}
	return len(ls) > 0 && strings.HasPrefix(strings.ToLower(ls[0]), "zh")
}
