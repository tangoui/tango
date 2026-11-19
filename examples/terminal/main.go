// Command terminal opens the Tango terminal component (ui/terminal) in a
// window, connected to a shell on a local pseudo-terminal (sys/pty).
//
//	go run ./examples/terminal                          # $SHELL (or the platform default)
//	go run ./examples/terminal -shell "/bin/zsh -f"     # any program with arguments
//	go run ./examples/terminal -renderer cpu -font-size 15 -scheme dracula
//	go run ./examples/terminal -echo                    # no PTY: input is echoed back
//	go run ./examples/terminal -replay out.vt           # no PTY: plays recorded output
//
// Where there is no local pseudo-terminal (iOS, Android, js/wasm) the
// example falls back to the echo connection, so the same program shows
// the view, the soft keyboard and input methods on a phone. The echo and
// replay connections live in conns.go; they are small examples of
// implementing terminal.Conn.
//
// On a phone the view is kept inside the safe area and above the soft
// keyboard.
//
// The window title follows the title the program sets (OSC 0/2) and is
// "Tango Terminal" otherwise. The application quits when the program
// exits.
//
// -auto presses 100 keys once the shell is up (x and Backspace in turn,
// so the line never wraps), one at a time, each as soon as the previous
// one has been echoed, and logs the key-to-echo
// latency (event posted until a frame shows the echo, including the PTY
// round trip) and the frame intervals; it needs a shell or -echo. Keys
// are posted right after a frame is built, so the latency is that of a
// key pressed just after a frame: the worst point of the display cycle.
// -bench-cat FILE runs "cat FILE" in the shell and logs the time until
// the output has been read and drawn (it waits for a line printed after
// cat); it needs a shell.
// -quit-after quits after the given duration (for scripted screenshots).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/pty"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/terminal"
	uitheme "github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

const defaultTitle = "Tango Terminal"

var (
	app      *tango.App
	runner   *ui.Runner
	surface  *tango.Surface
	term     *terminal.Terminal
	counter  *countConn
	fontSize float32
	scheme   *terminal.Scheme
	focused  bool
	title    = defaultTitle
	auto     *autoRun
	bench    *benchCat
)

var schemes = map[string]*terminal.Scheme{
	"":                nil, // the theme's terminal token
	"token":           nil,
	"dark":            &terminal.SchemeDark,
	"light":           &terminal.SchemeLight,
	"solarized-dark":  &terminal.SchemeSolarizedDark,
	"solarized-light": &terminal.SchemeSolarizedLight,
	"dracula":         &terminal.SchemeDracula,
}

func page(c *ui.Context) {
	v := term.Lock()
	t := v.Title()
	cx, cy, _ := v.Cursor()
	term.Unlock()
	if t == "" {
		t = defaultTitle
	}
	if t != title && surface != nil {
		title = t
		surface.SetTitle(t)
	}

	// On a phone, keep the terminal clear of the status bar, the home
	// indicator and the soft keyboard, so the cursor line stays visible
	// (all zero on desktops).
	in := c.SafeArea()
	in.Bottom = max(in.Bottom, c.KeyboardInset().Bottom)
	var vw *terminal.ViewW
	ui.Column(c, func() {
		vw = terminal.View(c, term).FontSize(fontSize).Layout(layout.Grow(1))
		if scheme != nil {
			vw.Scheme(*scheme)
		}
	}, layout.Fill(), layout.Padding(in))
	if !focused {
		c.SetFocus(vw.Box().ID())
		focused = true
	}
	if auto != nil {
		auto.frame(c, cx, cy)
	}
	if bench != nil {
		bench.frame(c)
	}
}

// countConn counts the bytes the program wrote and, for -bench-cat,
// watches the output for a marker (the read loop is its only reader).
type countConn struct {
	terminal.Conn
	n      atomic.Int64
	marker []byte
	tail   []byte // the last len(marker)-1 bytes read, for a marker split across reads
	seen   atomic.Bool
}

// watch makes Read look for marker in the output.
func (c *countConn) watch(marker string) { c.marker = []byte(marker) }

func (c *countConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.n.Add(int64(n))
	if len(c.marker) > 0 && n > 0 && !c.seen.Load() {
		buf := append(c.tail, b[:n]...)
		if bytes.Contains(buf, c.marker) {
			c.seen.Store(true)
		}
		keep := min(len(buf), len(c.marker)-1)
		c.tail = append(c.tail[:0], buf[len(buf)-keep:]...)
	}
	return n, err
}

// autoRun presses autoType keys from inside the page, x and Backspace in
// turn, each once the previous one is echoed (the cursor moved), and
// records the latency and the frame intervals.
type autoRun struct {
	frameNow time.Time // c.Now of the frame last counted
	ready    time.Time // when the shell first printed something
	last     time.Time
	sent     time.Time // when the pending character was posted
	sentX    int       // cursor position when it was posted
	sentY    int
	pending  bool
	typed    int
	latency  []time.Duration
	interval []time.Duration
	done     bool
}

const (
	autoSettle = 1500 * time.Millisecond // after the first output: prompt drawn, rc files done
	autoType   = 100
)

func (a *autoRun) frame(c *ui.Context, cx, cy int) {
	if a.done || c.Now().Equal(a.frameNow) {
		return
	}
	a.frameNow = c.Now()
	now := time.Now()
	if a.typed > 0 || a.pending {
		a.interval = append(a.interval, now.Sub(a.last))
	}
	a.last = now
	if a.ready.IsZero() {
		if counter.n.Load() > 0 {
			a.ready = now
		}
		keepFraming()
		return
	}
	if now.Sub(a.ready) < autoSettle && a.typed == 0 && !a.pending {
		keepFraming()
		return
	}
	if a.pending {
		if cx == a.sentX && cy == a.sentY {
			keepFraming()
			return // not echoed yet
		}
		a.latency = append(a.latency, now.Sub(a.sent))
		a.pending = false
		a.typed++
	}
	if a.typed == autoType {
		a.done = true
		report("key-to-echo latency", a.latency)
		report("frame interval while typing", a.interval)
		return
	}
	a.pending, a.sentX, a.sentY = true, cx, cy
	ev := []input.Event{input.KeyEvent{Key: input.KeyX, Down: true}, input.TextInput{Text: "x"}, input.KeyEvent{Key: input.KeyX}}
	if a.typed%2 == 1 {
		ev = []input.Event{input.KeyEvent{Key: input.KeyBackspace, Down: true}, input.KeyEvent{Key: input.KeyBackspace}}
	}
	runner.Post(func() { // after this frame, on the window's UI thread
		a.sent = time.Now()
		for _, e := range ev {
			runner.HandleEvent(e)
		}
	})
}

// keepFraming asks for another frame after this one. Called by the page.
func keepFraming() { runner.Post(func() {}) } // Post requests the frame

// benchCat runs cat on a file in the shell and waits for the title the
// command line sets after it.
type benchCat struct {
	file    string
	ready   time.Time
	start   time.Time
	bytes0  int64
	frames  int
	lastNow time.Time
	done    bool
}

const benchMarker = "tango-bench-cat-done"

func (b *benchCat) frame(c *ui.Context) {
	if b.done || c.Now().Equal(b.lastNow) {
		return
	}
	b.lastNow = c.Now()
	now := time.Now()
	switch {
	case b.ready.IsZero():
		if counter.n.Load() > 0 {
			b.ready = now
		}
		keepFraming()
	case b.start.IsZero():
		if now.Sub(b.ready) < autoSettle {
			keepFraming()
			return
		}
		b.start, b.bytes0 = now, counter.n.Load()
		// The marker is split in the command line, so only printf's
		// output contains it.
		h := len(benchMarker) / 2
		term.Send(fmt.Appendf(nil, "cat %s; printf '%%s%%s\\n' '%s' '%s'\r", shellQuote(b.file), benchMarker[:h], benchMarker[h:]))
	case counter.seen.Load():
		b.done = true
		d := now.Sub(b.start)
		n := counter.n.Load() - b.bytes0
		log.Printf("terminal: bench-cat %s: %.1f MB in %v (%.1f MB/s), %d frames (%.1f fps)",
			b.file, float64(n)/1e6, d.Round(time.Millisecond), float64(n)/1e6/d.Seconds(), b.frames, float64(b.frames)/d.Seconds())
	default:
		b.frames++
	}
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// runFlags are the flags that choose the connection and the measurements.
type runFlags struct {
	shell, replay, bench string
	echo, auto           bool
}

// checkFlags rejects combinations that cannot work.
func checkFlags(o runFlags) error {
	switch {
	case o.shell != "" && strings.TrimSpace(o.shell) == "":
		return errors.New("-shell is blank")
	case o.echo && o.replay != "":
		return errors.New("-echo and -replay: choose one")
	case o.auto && o.bench != "":
		return errors.New("-auto and -bench-cat: run one at a time")
	case o.auto && o.replay != "":
		return errors.New("-auto needs input that comes back: a shell or -echo, not -replay")
	case o.bench != "" && (o.echo || o.replay != ""):
		return errors.New("-bench-cat runs cat in a shell: not with -echo or -replay")
	}
	return nil
}

func report(name string, d []time.Duration) {
	if len(d) == 0 {
		return
	}
	s := slices.Clone(d)
	slices.Sort(s)
	var sum time.Duration
	for _, v := range s {
		sum += v
	}
	mean := sum / time.Duration(len(s))
	p := func(q float64) time.Duration { return s[int(float64(len(s)-1)*q)] }
	log.Printf("terminal: auto %s: n=%d, mean %.2f ms, p50 %.2f, p95 %.2f, p99 %.2f, max %.2f ms",
		name, len(s), ms(mean), ms(p(0.5)), ms(p(0.95)), ms(p(0.99)), ms(p(1)))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// connect returns the connection the flags ask for: a replay, the echo
// connection, or a shell on a pseudo-terminal (echo where there are
// none).
func connect(shell, replay string, echo bool) (terminal.Conn, string, error) {
	switch {
	case replay != "":
		b, err := os.ReadFile(replay)
		if err != nil {
			return nil, "", err
		}
		return newReplayConn(b), "replay " + replay, nil
	case echo:
		return newEchoConn(), "echo", nil
	}
	path, args := pty.DefaultShell()
	if shell != "" {
		f := strings.Fields(shell)
		path, args = f[0], f
	}
	p, err := pty.Start(pty.Options{Path: path, Args: args, Cols: 80, Rows: 24})
	if errors.Is(err, pty.ErrUnsupported) {
		log.Printf("terminal: no local pseudo-terminal on this platform; using the echo connection")
		return newEchoConn(), "echo", nil
	}
	if err != nil {
		return nil, "", err
	}
	return p, fmt.Sprintf("pty %s (pid %d)", strings.Join(args, " "), p.Pid()), nil
}

func main() {
	renderer := flag.String("renderer", "", `force the renderer: "cpu" or "gpu" (sets TANGO_RENDERER)`)
	size := flag.Float64("font-size", 0, "cell font size in logical pixels (0: the theme's terminal token, 13)")
	schemeName := flag.String("scheme", "", "color scheme: token (default), dark, light, solarized-dark, solarized-light, dracula")
	light := flag.Bool("light", false, "light UI theme (the token scheme follows it)")
	shell := flag.String("shell", "", "program to run, with arguments separated by spaces (default: the user's shell)")
	replay := flag.String("replay", "", "play this file's bytes instead of running a shell")
	echo := flag.Bool("echo", false, "echo input back instead of running a shell")
	autoFlag := flag.Bool("auto", false, "type 100 characters once the shell is up; log key-to-echo latency and frame intervals")
	benchFile := flag.String("bench-cat", "", "run cat on this file in the shell; log the time until the shell is back")
	quitAfter := flag.Duration("quit-after", 0, "quit after this long (0: run until the program exits or the window closes)")
	flag.Parse()
	switch *renderer {
	case "":
	case "cpu", "gpu":
		os.Setenv("TANGO_RENDERER", *renderer)
	default:
		log.Fatalf("terminal: -renderer %q: want cpu or gpu", *renderer)
	}
	sch, ok := schemes[*schemeName]
	if !ok {
		log.Fatalf("terminal: -scheme %q: want token, dark, light, solarized-dark, solarized-light or dracula", *schemeName)
	}
	scheme, fontSize = sch, float32(*size)

	if err := checkFlags(runFlags{shell: *shell, replay: *replay, bench: *benchFile, echo: *echo, auto: *autoFlag}); err != nil {
		log.Fatalf("terminal: %v", err)
	}
	conn, what, err := connect(*shell, *replay, *echo)
	if err != nil {
		log.Fatalf("terminal: %v", err)
	}
	if *benchFile != "" && what == "echo" {
		log.Fatalf("terminal: -bench-cat needs a shell; this platform has no pseudo-terminal")
	}
	if *autoFlag {
		auto = &autoRun{}
	}
	if *benchFile != "" {
		bench = &benchCat{file: *benchFile}
	}
	quit := true
	app = tango.New(tango.Options{Name: defaultTitle, ID: "dev.tango.terminal", QuitOnLastClose: &quit})
	counter = &countConn{Conn: conn}
	if bench != nil {
		counter.watch(benchMarker)
	}
	term = terminal.New(terminal.Options{Conn: counter, OnExit: func(err error) {
		if err != nil {
			log.Printf("terminal: connection ended: %v", err)
		}
		app.Post(app.Quit)
	}})
	defer term.Close()

	ropts := ui.RunnerOptions{Theme: uitheme.AntDesign(uitheme.Dark())}
	if *light {
		ropts.Theme = uitheme.AntDesign()
	}
	start := time.Now()
	app.OnReady(func() {
		r, s, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: defaultTitle, Width: 900, Height: 600, MinWidth: 320, MinHeight: 200,
			Resizable: true, Centered: true,
		}}, page, ropts)
		if err != nil {
			log.Fatalf("terminal: uihost.Open: %v", err)
		}
		runner, surface = r, s
		log.Printf("terminal: %s, window open %v after start, renderer %s",
			what, time.Since(start).Round(time.Millisecond), uihost.Renderer(r))
		if *quitAfter > 0 {
			time.AfterFunc(*quitAfter, func() { app.Post(app.Quit) })
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
