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
		// Nearest-counterpart scenes (scene_nearest.go).
		"select":       {800, 600, selectPage()},
		"tooltip":      {800, 600, tooltipPage()},
		"table":        {800, 600, tablePage()},
		"pagination":   {800, 600, paginationPage()},
		"cascader":     {800, 600, cascaderPage()},
		"transfer":     {800, 600, transferPage()},
		"treeselect":   {800, 600, treeselectPage()},
		"calendar":     {800, 600, calendarPage()},
		"steps":        {800, 600, stepsPage()},
		"timeline":     {800, 600, timelinePage()},
		"descriptions": {800, 600, descriptionsPage()},
		"form":         {800, 600, formPage()},
		"upload":       {800, 600, uploadPage()},
		"carousel":     {800, 600, carouselPage()},
		"collapse":     {800, 600, collapsePage()},
		"drawer":       {800, 600, drawerPage()},
		"tour":         {800, 600, tourPage()},
		"protable":     {800, 600, protablePage()},
		"segmented":    {800, 600, segmentedPage()},
		"rate":         {800, 600, ratePage()},
		"tags":         {800, 600, tagsPage()},
		"badges":       {800, 600, badgesPage()},
		"cards":        {800, 600, cardsPage()},
		"statistics":   {800, 600, statisticsPage()},
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

// report counts a frame that did the scene's work. Every frame it polls
// the renderer uihost uses for the window, on the UI thread; proto reports
// it at ready, with every per-second report and as soon as it changes
// (spec §6.3: the default policy may move between native, gpu and cpu).
// reportHook, set by the tests only, sees every frame report() counts.
var reportHook func(work int)

func report(work int) {
	if runner != nil { // nil only before uihost.Open returned, and in the headless tests
		proto.Renderer(uihost.Renderer(runner))
	}
	if reportHook != nil {
		reportHook(work)
	}
	proto.Frame(work)
}

// useEnglish shows the components' built-in texts (buttons, month and
// weekday names, counts) in English, as all three programs do, whatever
// the system's language: ui's English pack, kept by uihost.LocaleFixed.
func useEnglish() {
	if l, ok := ui.LocaleByCode("en"); ok {
		ui.SetDefaultLocale(l)
	}
}

func main() {
	name := flag.String("scene", "counter", "scene: counter, list, grid or a component scene (../scenes.md)")
	flag.Parse()
	s, ok := scenes()[*name]
	if !ok {
		fmt.Fprintf(os.Stderr, "uibench tango: unknown scene %q\n", *name)
		os.Exit(2)
	}
	useEnglish()
	proto.Scene(*name, s.width, s.height)
	quit := true
	app := tango.New(tango.Options{Name: "uibench tango", ID: "local.tango.uibench", QuitOnLastClose: &quit})
	app.OnReady(func() {
		r, surface, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "uibench tango " + *name, Width: s.width, Height: s.height, Centered: true,
		}, Locale: uihost.LocaleFixed}, s.page, ui.RunnerOptions{Theme: theme.AntDesign()})
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
