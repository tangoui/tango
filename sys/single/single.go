// Package single keeps one instance of an app running per user.
//
// Call Claim from main before App.Run. The first instance becomes the
// primary and listens for later launches; a later instance forwards its
// command-line arguments and working directory to the primary and should
// then exit:
//
//	app := tango.New(tango.Options{ID: "com.example.notes", URLSchemes: []string{"notes"}})
//	primary, err := single.Claim(app, single.Options{
//		OnSecondInstance: func(args []string, cwd string) { log.Println("relaunched with", args) },
//	})
//	if !primary {
//		return // forwarded to the running instance
//	}
//	if err != nil {
//		log.Println("single instance disabled:", err)
//	}
//
// In the primary, a forwarded launch is handled on the main thread: deep
// links and files among its arguments are delivered like launch arguments
// (App.HandleArgs: EventOpenURL, EventOpenFiles), Options.OnSecondInstance
// and the handlers registered with OnSecondInstance run, and the first open
// surface is shown and focused. OnSecondInstance lets other packages
// (jsapi forwards launches to pages) listen without being handed the
// Options:
//
//	single.OnSecondInstance(app, func(args []string, cwd string) { ... })
//
// Instances are told apart by app key and user: macOS and Linux use a lock
// file held with flock(2) plus a Unix-domain socket in a directory only the
// user can access ($XDG_RUNTIME_DIR on Linux, else a 0700 "tango-<uid>"
// directory in $TMPDIR or /tmp); Windows uses a named pipe, named after the
// user's SID, created with FILE_FLAG_FIRST_PIPE_INSTANCE and a DACL that
// admits only that user. Both decide the primary atomically, so two
// instances started together cannot both win. A crashed primary leaves
// nothing that blocks the next launch: the kernel drops the lock with the
// process, and a stale socket file is replaced.
//
// Arguments are only forwarded to a primary run by the same user: a later
// instance checks the owner of the other end (peer credentials, the pipe
// server's process token) before sending anything. If the endpoint is held
// by someone else, or the directory is not private, Claim reports
// primary=true with an error: the app runs on its own and should log it.
//
// On iOS and Android the system already runs one instance; Claim does
// nothing and reports primary.
package single

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
)

// ErrUnsupported is returned (with primary true) where the platform has no
// single-instance mechanism.
var ErrUnsupported = platform.ErrUnsupported

// Options configures Claim.
type Options struct {
	// Key identifies the app: instances with the same Key, run by the same
	// user, are one app. Empty means the app's ID (Options.ID, else Name).
	Key string

	// OnSecondInstance runs on the main thread of the primary when a later
	// launch forwards to it, with that launch's arguments (without the
	// program name) and working directory.
	OnSecondInstance func(args []string, cwd string)

	// NoFocus keeps the primary's main window where it is. By default the
	// first open surface is shown and focused.
	NoFocus bool

	// Args are the arguments a later instance forwards; nil means
	// os.Args[1:].
	Args []string

	// Timeout bounds how long a later instance tries to reach the primary;
	// 0 means 3 seconds.
	Timeout time.Duration
}

// Claim makes this process the app's primary instance, or forwards to the
// running primary. It reports primary=false only when the arguments were
// forwarded (exit then) or the primary exists but could not be reached
// (err says why; exiting is still the safe choice). With primary=true and
// a non-nil err, single-instance mode could not be set up and the app runs
// as if it were not used.
//
// Call it once, before App.Run. The primary stops listening when the app
// terminates.
func Claim(a *tango.App, o Options) (primary bool, err error) {
	if mobile {
		return true, nil
	}
	key := o.Key
	if key == "" {
		key = a.ID()
	}
	if key == "" {
		return true, errors.New("single: no key: set Options.Key or the app's ID or Name")
	}
	args := o.Args
	if args == nil && len(os.Args) > 1 {
		args = os.Args[1:]
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	deliver := func(m message) {
		a.Post(func() {
			if !o.NoFocus {
				if ss := a.Surfaces(); len(ss) > 0 {
					ss[0].Show()
					ss[0].Focus()
				}
			}
			a.HandleArgs(m.Args, m.Cwd)
			// Post guards the whole function; guard each handler on its
			// own too so a panic in one does not skip the rest.
			if o.OnSecondInstance != nil {
				corehook.Guard(a, "event handler", func() { o.OnSecondInstance(m.Args, m.Cwd) })
			}
			for _, fn := range handlers(a) {
				corehook.Guard(a, "event handler", func() { fn(m.Args, m.Cwd) })
			}
		})
	}
	srv, fwd, err := claim(name(key), args, deliver, timeout)
	if err != nil && srv == nil && !fwd && !errors.Is(err, errReach) {
		return true, err // could not set up; run unguarded
	}
	if srv == nil {
		return false, err
	}
	a.OnLifecycle(func(l tango.Lifecycle) {
		if l == tango.Terminating {
			srv.Close()
		}
	})
	return true, nil
}

var (
	hmu  sync.Mutex
	hset = map[*tango.App][]func(args []string, cwd string){}
)

// OnSecondInstance registers fn to run on the main thread of the primary
// instance each time a later launch forwards to it, after
// Options.OnSecondInstance. It may be called before or after Claim (from
// any goroutine) and has no effect unless Claim made this process the
// primary.
func OnSecondInstance(a *tango.App, fn func(args []string, cwd string)) {
	hmu.Lock()
	hset[a] = append(hset[a], fn)
	hmu.Unlock()
}

func handlers(a *tango.App) []func([]string, string) {
	hmu.Lock()
	defer hmu.Unlock()
	return slices.Clone(hset[a])
}

// errReach: another instance holds the key but did not take our message.
var errReach = errors.New("single: another instance is running but did not respond")

// message is what a later instance sends, as one JSON line. The primary
// answers "ok\n" once it has queued it.
type message struct {
	Args []string `json:"args"`
	Cwd  string   `json:"cwd"`
}

const maxMessage = 1 << 20

// server is a listening primary.
type server interface{ Close() error }

// claim becomes the primary for name (srv != nil) or forwards args to it
// (fwd). It retries until timeout while the primary is starting up or
// shutting down.
func claim(name string, args []string, deliver func(message), timeout time.Duration) (srv server, fwd bool, err error) {
	cwd, _ := os.Getwd()
	msg, err := json.Marshal(message{Args: args, Cwd: cwd})
	if err != nil {
		return nil, false, err
	}
	deadline := time.Now().Add(timeout)
	for {
		srv, err := listen(name, deliver)
		if err == nil {
			return srv, false, nil
		}
		if !errors.Is(err, errBusy) {
			return nil, false, err
		}
		err = send(name, msg, time.Until(deadline))
		if err == nil {
			return nil, true, nil
		}
		if errors.Is(err, errUntrusted) {
			// Held by another user (or we cannot tell): nothing was sent;
			// run unguarded rather than exit on their behalf.
			return nil, false, err
		}
		if time.Now().After(deadline) {
			return nil, false, errors.Join(errReach, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// errBusy: the key's endpoint exists, so another process may be the
// primary. Only send, which checks who owns that process, confirms it.
var errBusy = errors.New("single: key held by another instance")

// errUntrusted: the endpoint for the key is not owned by this user, or its
// owner cannot be verified. Nothing is forwarded to it.
var errUntrusted = errors.New("single: instance endpoint is not owned by the current user")

// serve handles one connection on the primary: read a message, queue it,
// acknowledge.
func serve(rw io.ReadWriter, deliver func(message)) {
	line, err := bufio.NewReaderSize(io.LimitReader(rw, maxMessage), 4096).ReadBytes('\n')
	if err != nil && (err != io.EOF || len(line) == 0) {
		return
	}
	var m message
	if json.Unmarshal(line, &m) != nil {
		return
	}
	deliver(m)
	io.WriteString(rw, "ok\n")
}

// exchange sends msg on a connected stream and waits for the ack.
func exchange(rw io.ReadWriter, msg []byte) error {
	if _, err := rw.Write(append(msg, '\n')); err != nil {
		return err
	}
	ack, err := bufio.NewReader(rw).ReadString('\n')
	if err != nil {
		return err
	}
	if ack != "ok\n" {
		return errors.New("single: unexpected reply from primary")
	}
	return nil
}

// name derives a short, filesystem- and pipe-safe name from key and user.
func name(key string) string {
	sum := sha256.Sum256([]byte(key + "\x00" + userTag()))
	return "tango-" + hex.EncodeToString(sum[:10])
}

// closeOnce makes Close idempotent.
type closeOnce struct {
	once sync.Once
	err  error
}

func (c *closeOnce) do(fn func() error) error {
	c.once.Do(func() { c.err = fn() })
	return c.err
}
