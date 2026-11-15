// Command editor-demo opens the Tango code editor view (ui/editor) in a
// real window.
//
//	go run ./examples/editor-demo                       # 10 000 generated TypeScript lines
//	go run ./examples/editor-demo -gen-lines 1000000    # ~100 MB, 1M lines (spec 6.6)
//	go run ./examples/editor-demo -file main.go -theme vs
//	go run ./examples/editor-demo -renderer cpu         # force the CPU renderer
//	go run ./examples/editor-demo -split -wrap          # two views of one model
//	go run ./examples/editor-demo -theme hc-black -language go
//	go run ./examples/editor-demo -diff old.go new.go   # diff view (ui/editor/diffview)
//	go run ./examples/editor-demo -diff -inline -fold old.go new.go
//
// The toolbar opens files, saves them, saves generated content with Save As,
// and opens a find/replace panel. Dropping a file onto the window or opening
// a file through the operating system uses the same document workflow; dirty
// documents ask whether to save before replacement or close.
//
// The window contains a language selector, a Commands()-backed palette,
// one or two editor views, and a status line (caret position, line count,
// language, renderer, highlight progress). Syntax highlighting runs
// in the background goroutine of the State's Highlighter and redraws the
// window as tokens arrive.
//
// With -diff the window shows the diff view of the two files named after
// the flags instead (side by side, or -inline; -fold folds the unchanged
// regions); the toolbar switches the mode and moves between the changes
// (also Alt+F5 / Shift+Alt+F5).
//
// -auto runs a scripted check once the window is up: 240 frames of
// scrolling (two lines per frame), then 100 keystrokes, one per frame, and
// logs the frame intervals the window actually got (paced by the display).
// -quit-after quits after the given duration (for scripted screenshots).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diffview"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	uitheme "github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

var (
	app             *tango.App
	runner          *ui.Runner
	st              *editor.State
	rendName        = "?"
	focused         bool
	hlDone          bool
	auto            *autoRun
	second          *editor.State
	active          *editor.State
	paletteOpen     bool
	paletteTarget   *editor.State
	paletteCommand  string
	pendingCommand  string
	languageOptions []ui.Option
	search          searchPanel
	background      *modelHighlight
	documents       *documentController
	surface         *tango.Surface
	surfaceTitle    string
	diffState       *diffview.State
	diffOrigHL      *modelHighlight
	diffMode        = "sideBySide"
)

func init() {
	editor.RegisterCommand(editor.Command{ID: "editor.action.quickCommand", Label: "Command Palette", Run: func(target *editor.State, _ any) bool {
		paletteTarget, paletteOpen = target, true
		return true
	}})
	for _, lang := range langs.Builtin() {
		languageOptions = append(languageOptions, ui.Option{Label: lang.ID, Value: lang.ID})
	}
}

func setupDocument(name, text string, th *theme.Theme, split, wrap bool) {
	if documents == nil {
		documents = newDocumentController(editor.DefaultRegistry(), th, split, wrap)
	} else {
		documents.theme, documents.split, documents.wrap = th, split, wrap
	}
	documents.install(newDocumentSession(name, text, documents.registry))
}

func syncTitle() {
	if surface == nil || documents == nil {
		return
	}
	title := documents.title()
	if title != surfaceTitle {
		surface.SetTitle(title)
		surfaceTitle = title
	}
}

func showMessage(kind dialog.Kind, title, message string) {
	if app == nil {
		return
	}
	_, _ = dialog.Message(app, dialog.MessageOptions{Parent: surface, Kind: kind, Title: title, Message: message})
}

func saveAsDocument() bool {
	if documents == nil || documents.session == nil {
		return false
	}
	name := documentDisplayName(documents.session.path)
	if name == "Untitled" {
		name = "untitled.txt"
	}
	path, err := dialog.Save(app, dialog.FileOptions{Parent: surface, Title: "Save As", Filename: name})
	if err != nil || path == "" {
		return false
	}
	if err := documents.saveAs(path); err != nil {
		showMessage(dialog.Error, "Save failed", err.Error())
		return false
	}
	syncTitle()
	return true
}

func saveDocument() bool {
	if documents == nil || documents.session == nil {
		return false
	}
	if documents.session.path == "" {
		return saveAsDocument()
	}
	if err := documents.save(); err != nil {
		showMessage(dialog.Error, "Save failed", err.Error())
		return false
	}
	syncTitle()
	return true
}

func confirmDiscard() bool {
	if documents == nil || documents.session == nil || !documents.session.dirty() {
		return true
	}
	choice, err := dialog.Message(app, dialog.MessageOptions{
		Parent:  surface,
		Kind:    dialog.Question,
		Title:   "Unsaved changes",
		Message: "Save changes to " + documentDisplayName(documents.session.path) + " before continuing?",
		Buttons: []string{"Save", "Don't Save", "Cancel"},
	})
	if err != nil {
		return false
	}
	switch discardAction(choice) {
	case discardSave:
		return saveDocument()
	case discardDontSave:
		return resolveDiscard(true, discardDontSave)
	default:
		return resolveDiscard(true, discardCancel)
	}
}

func openDocumentPath(path string) bool {
	if diffState != nil || documents == nil {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if !confirmDiscard() {
		return false
	}
	if err := documents.replace(path); err != nil {
		showMessage(dialog.Error, "Open failed", err.Error())
		return false
	}
	syncTitle()
	return true
}

func openDocumentDialog() {
	if documents == nil || diffState != nil || !confirmDiscard() {
		return
	}
	paths, err := dialog.Open(app, dialog.FileOptions{Parent: surface, Title: "Open File", Multiple: false})
	if err == nil && len(paths) > 0 {
		openDocumentPath(paths[0])
	}
}

func commandOptions() []ui.Option {
	var opts []ui.Option
	for _, cmd := range editor.Commands() {
		opts = append(opts, ui.Option{Label: cmd.Label + " — " + cmd.ID, Value: cmd.ID})
	}
	return opts
}

// setupDiff opens the diff view of the files a and b.
func setupDiff(a, b string, th *theme.Theme, wrap, inline, fold bool) error {
	ta, err := os.ReadFile(a)
	if err != nil {
		return err
	}
	tb, err := os.ReadFile(b)
	if err != nil {
		return err
	}
	reg := editor.DefaultRegistry()
	om, mm := editor.NewModel(a, string(ta), reg), editor.NewModel(b, string(tb), reg)
	diffOrigHL, background = newModelHighlight(om), newModelHighlight(mm)
	diffState = diffview.New(om, mm, th)
	if wrap {
		diffState.Options.DiffWordWrap = editor.WrapOn
	}
	if inline {
		diffMode = "inline"
	}
	diffState.Options.HideUnchangedRegions.Enabled = fold
	st, active, focused = diffState.Modified, diffState.Modified, false
	return nil
}

// diffPage is the page of -diff.
func diffPage(c *ui.Context) {
	d := diffState
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Segmented(c, &diffMode, []ui.Option{{Label: "Side by side", Value: "sideBySide"}, {Label: "Inline", Value: "inline"}})
			fold := d.Options.HideUnchangedRegions.Enabled
			if ui.Checkbox(c, "Fold unchanged regions", &fold).Changed() {
				d.Options.HideUnchangedRegions.Enabled = fold
			}
			if ui.Button(c, "Previous change").Clicked() {
				d.Prev()
			}
			if ui.Button(c, "Next change").Clicked() {
				d.Next()
			}
		}, layout.PadVH(4, 12), layout.Gap(12), layout.Middle())
		d.Options.Mode = diffview.SideBySide
		if diffMode == "inline" {
			d.Options.Mode = diffview.Inline
		}
		ui.Row(c, func() {
			vw := diffview.View(c, d)
			if _, mod := vw.Editors(); !focused && mod != nil {
				c.SetFocus(mod.Box().ID())
				focused = true
			}
		}, layout.Grow(1))
		ui.Row(c, func() { ui.Text(c, diffStatus()) }, layout.PadVH(4, 12))
	}, layout.Fill())
	if auto != nil {
		auto.frame(c)
	}
}

func diffStatus() string {
	r, ok := diffState.Result()
	state := "computing…"
	if ok {
		state = "up to date"
		if r.TimedOut {
			state = "timed out"
		}
	}
	return fmt.Sprintf("%d changes (%s)    original %d lines    modified %d lines    %s",
		len(r.Changes), state, diffState.Original.Snapshot().LineCount(), diffState.Modified.Snapshot().LineCount(), status())
}

func page(c *ui.Context) {
	syncTitle()
	if diffState != nil {
		diffPage(c)
		return
	}
	ui.Column(c, func() {
		ui.Row(c, func() {
			if documents != nil {
				if ui.Button(c, "Open").Clicked() {
					openDocumentDialog()
				}
				if ui.Button(c, "Save").Disabled(!documents.session.dirty()).Clicked() {
					saveDocument()
				}
				if ui.Button(c, "Save As").Clicked() {
					saveAsDocument()
				}
				if ui.Button(c, "Find").Clicked() {
					search.open = true
				}
				if ui.Button(c, "Replace").Clicked() {
					search.open = true
				}
			}
			language := st.Model.Language()
			if ui.Select(c, &language, languageOptions).Searchable().Width(210).A11yLabel("Document language").Changed() {
				st.Model.SetLanguage(language)
				hlDone = false
			}
			if ui.Button(c, "Command Palette").Clicked() {
				paletteTarget, paletteOpen = active, true
			}
			ui.Text(c, "Right-click: editor menu · commands target the last focused pane")
		}, layout.PadVH(4, 12))
		ui.Row(c, func() {
			show := func(target *editor.State) {
				vw := editor.View(c, target)
				if !focused {
					c.SetFocus(vw.Box().ID())
					focused = true
				}
				if vw.Focused() {
					active = target
				}
				if pendingCommand != "" && target == paletteTarget {
					c.SetFocus(vw.Box().ID())
					// Execute only after a build observes the restored focus, so
					// editorTextFocus preconditions and clipboard commands work.
					if vw.Focused() {
						id := pendingCommand
						pendingCommand = ""
						if !target.Execute(id, nil) {
							log.Printf("editor-demo: command %s refused or made no change", id)
						}
					} else {
						ui.Invalidate(c.Window())
					}
				}
			}
			show(st)
			if second != nil {
				show(second)
			}
		}, layout.Grow(1))
		ui.Row(c, func() { ui.Text(c, status()) }, layout.PadVH(4, 12))
	}, layout.Fill())
	ui.Modal(c, &paletteOpen, "Editor commands").Width(720).NoFooter().Body(func() {
		ui.Select(c, &paletteCommand, commandOptions()).Searchable().Width(650).A11yLabel("Registered editor commands")
		if ui.Button(c, "Execute").Disabled(paletteCommand == "").Clicked() {
			pendingCommand, paletteOpen = paletteCommand, false
		}
		ui.Text(c, "Commands without arguments only; preconditions/read-only rules still apply.")
	})
	if documents != nil {
		search.render(c, active)
	}
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
	return fmt.Sprintf("Ln %d, Col %d    %s    %d lines    %.1f MB    %s (%s)    renderer: %s",
		p.Line+1, snap.UTF16Col(p.Line, p.Col)+1, indentStatus(st.Model), snap.LineCount(), float64(snap.Len())/1e6, st.Model.Language(), hl, rendName)
}

// indentStatus is VS Code's indentation item of the status bar: the
// Model's tab size and insertSpaces, as detected from the text when it was
// opened (editor.action.detectIndentation re-detects them).
func indentStatus(m *editor.Model) string {
	if m.InsertSpaces() {
		return fmt.Sprintf("Spaces: %d", m.TabSize())
	}
	return fmt.Sprintf("Tab Size: %d", m.TabSize())
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
	themeName := flag.String("theme", "vs-dark", `editor theme: vs, vs-dark, hc-black, hc-light`)
	split := flag.Bool("split", false, "two independent editor views sharing one text model")
	wrap := flag.Bool("wrap", false, "word wrap (large-file optimizations may disable it)")
	language := flag.String("language", "", "override document language ID (also switchable in the toolbar)")
	autoFlag := flag.Bool("auto", false, "scripted scroll and typing check; logs the frame intervals")
	quitAfter := flag.Duration("quit-after", 0, "quit after this long (0: run until the window closes)")
	diffFlag := flag.Bool("diff", false, "show the diff view of the two files given as arguments")
	inline := flag.Bool("inline", false, "with -diff: start in inline mode")
	fold := flag.Bool("fold", false, "with -diff: fold the unchanged regions")
	flag.Parse()
	if *diffFlag && flag.NArg() != 2 {
		log.Fatal("editor-demo: -diff wants two files: -diff [flags] a b")
	}
	switch *renderer {
	case "":
	case "cpu", "gpu":
		os.Setenv("TANGO_RENDERER", *renderer)
	default:
		log.Fatalf("editor-demo: -renderer %q: want cpu or gpu", *renderer)
	}
	if !slices.Contains([]string{"vs", "vs-dark", "hc-black", "hc-light"}, *themeName) {
		log.Fatalf("editor-demo: -theme %q: want vs, vs-dark, hc-black or hc-light", *themeName)
	}
	if *language != "" && langs.ByID(*language) == nil {
		log.Fatalf("editor-demo: unknown language %q", *language)
	}

	start := time.Now()
	var text, name string
	if *diffFlag {
		if err := setupDiff(flag.Arg(0), flag.Arg(1), theme.Base(*themeName), *wrap, *inline, *fold); err != nil {
			log.Fatal(err)
		}
		defer diffState.Close()
		defer diffOrigHL.Close()
		defer background.Close()
		name = flag.Arg(1)
	} else if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			log.Fatal(err)
		}
		text, name = string(b), *file
	} else {
		text, name = genTS(*genLines), "generated.ts"
	}
	if !*diffFlag {
		setupDocument(name, text, theme.Base(*themeName), *split, *wrap)
		if *file == "" {
			documents.session.path = ""
		}
		defer func() {
			if background != nil {
				background.Close()
			}
			if st != nil {
				st.Close()
			}
			if second != nil {
				second.Close()
			}
		}()
	}
	if *language != "" {
		st.Model.SetLanguage(*language)
	}
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
	app.OnOpenFiles(func(paths []string) {
		if len(paths) > 0 {
			openDocumentPath(paths[0])
		}
	})
	app.OnReady(func() {
		r, s, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "Tango Editor — " + filepath.Base(name), Width: 1200, Height: 800, MinWidth: 480, MinHeight: 320,
			Resizable: true, Centered: true,
		}}, page, ropts)
		if err != nil {
			log.Fatal(err)
		}
		runner = r
		surface = s
		surface.OnClose(func() bool { return confirmDiscard() })
		surface.OnFilesDropped(func(paths []string, _, _ int) {
			if len(paths) > 0 {
				openDocumentPath(paths[0])
			}
		})
		for _, shortcut := range []struct {
			accelerator string
			fn          func()
		}{
			{"CmdOrCtrl+O", openDocumentDialog},
			{"CmdOrCtrl+S", func() { saveDocument() }},
			{"CmdOrCtrl+F", func() { search.open = true }},
			{"CmdOrCtrl+H", func() { search.open = true }},
		} {
			if _, err := uihost.Shortcut(surface, shortcut.accelerator, shortcut.fn); err != nil {
				log.Printf("editor-demo: shortcut %s: %v", shortcut.accelerator, err)
			}
		}
		syncTitle()
		rendName = uihost.Renderer(r)
		log.Printf("editor-demo: window open %v after start, renderer %s", time.Since(start).Round(time.Millisecond), rendName)
		if diffOrigHL != nil {
			orig := diffOrigHL
			orig.SetNotify(func() {
				app.Post(func() {
					if !orig.isClosed() {
						r.Invalidate()
					}
				})
			})
		}
		life := background
		hlStart := time.Now()
		life.SetNotify(func() {
			// ready runs off-thread (or is replayed during attachment).
			// Never read Model/State until this UI-thread callback runs.
			app.Post(func() {
				if life.isClosed() {
					return
				}
				if hl := st.Highlighter(); hl != nil && !hlDone && hl.Done(st.Snapshot()) {
					hlDone = true
					log.Printf("editor-demo: document highlighted in %v", time.Since(hlStart).Round(time.Millisecond))
				}
				r.Invalidate()
			})
		})
		if *quitAfter > 0 {
			time.AfterFunc(*quitAfter, func() { app.Post(app.Quit) })
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
