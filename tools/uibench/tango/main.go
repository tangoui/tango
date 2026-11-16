// Command tango is uibench's Tango program: `tango -scene <name>` opens
// the scene's window through uihost, in the drawing path Tango picks by
// default for that window size, and reports frames on standard output
// (tango.local/uibench/proto).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
	"tango.local/uibench/proto"
)

// scene is a window size and the page drawn in it.
type scene struct {
	width, height int
	page          func(*ui.Context)
}

func scenes() map[string]scene {
	return map[string]scene{
		"counter": {360, 260, counterPage()},
		"list":    {800, 600, listPage()},
		"grid":    {800, 600, gridPage()},
		// Component scenes (scene_components.go).
		"buttons":     {800, 600, buttonsPage()},
		"inputs":      {800, 600, inputsPage()},
		"textarea":    {800, 600, textareaPage()},
		"checkboxes":  {800, 600, checkboxesPage()},
		"radios":      {800, 600, radiosPage()},
		"switches":    {800, 600, switchesPage()},
		"sliders":     {800, 600, slidersPage()},
		"progress":    {800, 600, progressPage()},
		"tabs":        {800, 600, tabsPage()},
		"menu":        {800, 600, menuPage()},
		"dialog":      {800, 600, dialogPage()},
		"toast":       {800, 600, toastPage()},
		"tree":        {800, 600, treePage()},
		"datepicker":  {800, 600, datepickerPage()},
		"colorpicker": {800, 600, colorpickerPage()},
	}
}

// runner is the window's runner, set once uihost opened it.
var runner *ui.Runner

// clock tells frames apart: the page can run several build passes in one
// frame, all with the same c.Now(), and only the first pass of a frame
// advances the scene.
type clock struct {
	prev   time.Time
	frames int // frames seen, the first included
}

// next reports whether this pass starts a new frame.
func (k *clock) next(c *ui.Context) bool {
	if c.Now().Equal(k.prev) {
		return false
	}
	k.prev = c.Now()
	k.frames++
	return true
}

var reported bool

// report counts a frame that did the scene's work. The first one names the
// renderer, which uihost settled by then.
func report(work int) {
	if !reported {
		reported = true
		proto.Renderer(uihost.Renderer(runner))
	}
	proto.Frame(work)
}

func main() {
	name := flag.String("scene", "counter", "scene: counter, list, grid or a component scene (../scenes.md)")
	flag.Parse()
	s, ok := scenes()[*name]
	if !ok {
		fmt.Fprintf(os.Stderr, "uibench tango: unknown scene %q\n", *name)
		os.Exit(2)
	}
	quit := true
	app := tango.New(tango.Options{Name: "uibench tango", ID: "local.tango.uibench", QuitOnLastClose: &quit})
	app.OnReady(func() {
		r, surface, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "uibench tango " + *name, Width: s.width, Height: s.height, Centered: true,
		}}, s.page, ui.RunnerOptions{Theme: theme.AntDesign()})
		if err != nil {
			log.Fatal(err)
		}
		runner = r
		surface.Focus()
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
