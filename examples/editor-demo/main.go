// Command editor-demo opens the Tango code editor view (ui/editor) in a
// real window.
//
//	go run ./examples/editor-demo                       # 10 000 generated TypeScript lines
//	go run ./examples/editor-demo -gen-lines 1000000    # ~100 MB, 1M lines (spec 6.6)
//	go run ./examples/editor-demo -file main.go -theme vs
//	go run ./examples/editor-demo -renderer cpu         # force the CPU renderer
//
// The whole window is the editor plus a status line (caret position, line
// count, language, renderer, highlight progress). Syntax highlighting runs
// in the background goroutine of the State's Highlighter and redraws the
// window as tokens arrive.
//
// -auto runs a scripted check once the window is up: 240 frames of
// scrolling (two lines per frame), then 100 keystrokes, one per frame, and
// logs the frame intervals the window actually got (paced by the display).
// -quit-after quits after the given duration (for scripted screenshots).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	uitheme "github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

var (
	app      *tango.App
	runner   *ui.Runner
	st       *editor.State
	rendName = "?"
	focused  bool
	hlDone   bool
	auto     *autoRun
)

func page(c *ui.Context) {
	ui.Column(c, func() {
		vw := editor.View(c, st)
		if !focused {
			c.SetFocus(vw.Box().ID())
			focused = true
		}
		ui.Row(c, func() {
			ui.Text(c, status())
		}, layout.PadVH(4, 12))
	}, layout.Fill())
	if auto != nil {
		auto.frame(c)
	}
}

func status() string {
	snap := st.Snapshot()
	p := snap.PosOf(st.Primary().Head)
	hl := "plain text"
	if h := st.Highlighter(); h != nil {
		hl = "highlighting…"
		if hlDone {
			hl = "highlighted"
		}
	}
	return fmt.Sprintf("Ln %d, Col %d    %d lines    %.1f MB    %s (%s)    renderer: %s",
		p.Line+1, snap.UTF16Col(p.Line, p.Col)+1, snap.LineCount(), float64(snap.Len())/1e6, st.Model.Language(), hl, rendName)
}

// tsTemplates generate the sample document (the same lines as the
// benchmarks in ui/editor/perf_test.go: about 100 bytes per line).
var tsTemplates = []string{
	"export interface Row%[1]d { id: number; label: string; tags: string[]; nested?: { ok: boolean } }",
	"export function compute%[1]d(input: number[], scale = %[1]d): number { return input.length * scale; }",
	"  // line %[1]d: a comment explaining the next statement, with a few more words to fill it up nicely.",
	"  const message%[1]d = `row ${index + %[1]d} of the generated document: ${name.toUpperCase()}`;",
	"  if (value%[1]d !== null && typeof value%[1]d === \"object\") { counter += Object.keys(value).length; }",
	"class Widget%[1]d extends Base<string> implements Disposable { private readonly name = \"w-%[1]d\"; }",
	"  for (let i = 0; i < %[1]d; i++) { buffer.push({ index: i, hex: 0x%[1]x, ratio: i / 3.14159 }); }",
	"  return await fetch(`/api/items/%[1]d`, { method: \"POST\", body: JSON.stringify({ id: %[1]d }) });",
	"}  // end of block %[1]d: closes the class, function or loop that was opened above",
	"/* block %[1]d */ export const enumValue%[1]d = Direction.Up | Direction.Left; // trailing comment goes here",
}

func genTS(n int) string {
	var sb strings.Builder
	sb.Grow(n * 101)
	for i := range n {
		fmt.Fprintf(&sb, tsTemplates[i%len(tsTemplates)], i)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// autoRun scrolls and types from inside the page, one event per frame,
// and records the interval between frames.
type autoRun struct {
	start    time.Time
	last     time.Time
	frameNow time.Time // c.Now of the frame last counted
	n        int
	scroll   []time.Duration
	typing   []time.Duration
	done     bool
}

const (
	autoWarm   = 30 // frames before measuring (window shown, first highlight)
	autoScroll = 240
	autoType   = 100
)

func (a *autoRun) frame(c *ui.Context) {
	// The runner builds the page up to three times in a frame with input
	// (ui.Runner.Frame); c.Now is the same in each pass: count the frame
	// and post its event once.
	if a.done || c.Now().Equal(a.frameNow) {
		return
	}
	a.frameNow = c.Now()
	now := time.Now()
	if a.n > autoWarm {
		d := now.Sub(a.last)
		if a.n <= autoWarm+autoScroll {
			a.scroll = append(a.scroll, d)
		} else {
			a.typing = append(a.typing, d)
		}
	}
	a.last = now
	a.n++
	var ev []input.Event
	switch {
	case a.n <= autoWarm:
		// Nothing; keep frames coming.
	case a.n <= autoWarm+autoScroll:
		ev = []input.Event{input.Scroll{Pos: geom.Pt(c.Size().W/2, c.Size().H/2), DY: 2 * st.Viewport.LineHeight}}
	case a.n <= autoWarm+autoScroll+autoType:
		ev = []input.Event{input.KeyEvent{Key: input.KeyX, Down: true}, input.TextInput{Text: "x"}, input.KeyEvent{Key: input.KeyX}}
	default:
		a.done = true
		report("scroll", a.scroll)
		report("typing", a.typing)
		return
	}
	app.Post(func() {
		for _, e := range ev {
			runner.HandleEvent(e)
		}
		runner.Invalidate()
	})
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
	log.Printf("editor-demo: auto %s: %d frames, mean interval %.2f ms (%.0f fps), p50 %.2f, p95 %.2f, p99 %.2f, max %.2f ms",
		name, len(s), ms(mean), 1000/ms(mean), ms(p(0.5)), ms(p(0.95)), ms(p(0.99)), ms(p(1)))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func main() {
	file := flag.String("file", "", "file to open (default: a generated TypeScript document)")
	genLines := flag.Int("gen-lines", 10000, "lines of the generated TypeScript document when -file is not given")
	renderer := flag.String("renderer", "", `force the renderer: "cpu" or "gpu" (sets TANGO_RENDERER)`)
	themeName := flag.String("theme", "vs-dark", `editor theme: "vs" or "vs-dark"`)
	autoFlag := flag.Bool("auto", false, "scripted scroll and typing check; logs the frame intervals")
	quitAfter := flag.Duration("quit-after", 0, "quit after this long (0: run until the window closes)")
	flag.Parse()
	switch *renderer {
	case "":
	case "cpu", "gpu":
		os.Setenv("TANGO_RENDERER", *renderer)
	default:
		log.Fatalf("editor-demo: -renderer %q: want cpu or gpu", *renderer)
	}
	if *themeName != "vs" && *themeName != "vs-dark" {
		log.Fatalf("editor-demo: -theme %q: want vs or vs-dark", *themeName)
	}

	start := time.Now()
	var text, name string
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			log.Fatal(err)
		}
		text, name = string(b), *file
	} else {
		text, name = genTS(*genLines), "generated.ts"
	}
	st = editor.NewView(editor.NewModel(name, text, editor.DefaultRegistry()), theme.Base(*themeName))
	text = ""
	log.Printf("editor-demo: %s: %d lines, %.1f MB, language %s, state in %v",
		filepath.Base(name), st.Snapshot().LineCount(), float64(st.Snapshot().Len())/1e6, st.Model.Language(), time.Since(start).Round(time.Millisecond))
	if *autoFlag {
		auto = &autoRun{}
	}

	ropts := ui.RunnerOptions{Theme: uitheme.AntDesign()}
	if *themeName == "vs-dark" {
		ropts.Theme = uitheme.AntDesign(uitheme.Dark())
	}
	quit := true
	app = tango.New(tango.Options{Name: "Tango Editor", ID: "dev.tango.editordemo", QuitOnLastClose: &quit})
	app.OnReady(func() {
		r, _, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "Tango Editor — " + filepath.Base(name), Width: 1200, Height: 800, MinWidth: 480, MinHeight: 320,
			Resizable: true, Centered: true,
		}}, page, ropts)
		if err != nil {
			log.Fatal(err)
		}
		runner = r
		rendName = uihost.Renderer(r)
		log.Printf("editor-demo: window open %v after start, renderer %s", time.Since(start).Round(time.Millisecond), rendName)
		if hl := st.Highlighter(); hl != nil {
			ctx := context.Background()
			ch := make(chan buffer.Snapshot, 1)
			ch <- st.Snapshot()
			hlStart := time.Now()
			hl.Start(ctx, ch, func() {
				app.Post(func() {
					if !hlDone {
						if hl.Done(st.Snapshot()) {
							hlDone = true
							log.Printf("editor-demo: document highlighted in %v", time.Since(hlStart).Round(time.Millisecond))
						}
					}
					r.Invalidate()
				})
			})
		}
		if *quitAfter > 0 {
			time.AfterFunc(*quitAfter, func() { app.Post(app.Quit) })
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
