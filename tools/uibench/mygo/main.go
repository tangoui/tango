// Command mygo is uibench's MyGo program: `mygo -scene <name>` opens the
// scene in MyGo's self-drawn native UI (ui.View, drawn on the GPU), not a
// web view, and reports frames on standard output
// (tango.local/uibench/proto).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"tango.local/uibench/proto"
)

// scene is a window size and the view drawn in it.
type scene struct {
	width, height int
	view          func(*ui.Context)
}

func scenes() map[string]scene {
	return map[string]scene{
		"counter": {360, 260, (&counter{}).view},
		"list":    {800, 600, (&scrollingList{}).view},
		"grid":    {800, 600, (&dashboard{}).view},
		// Component scenes (scene_components.go).
		"buttons":     {800, 600, newButtons().view},
		"inputs":      {800, 600, newInputs().view},
		"textarea":    {800, 600, newTextarea().view},
		"checkboxes":  {800, 600, newCheckboxes().view},
		"radios":      {800, 600, (&radios{}).view},
		"switches":    {800, 600, newSwitches().view},
		"sliders":     {800, 600, newSliders().view},
		"progress":    {800, 600, newProgress().view},
		"tabs":        {800, 600, newTabs().view},
		"menu":        {800, 600, (&menu{}).view},
		"dialog":      {800, 600, newDialog().view},
		"toast":       {800, 600, (&toast{}).view},
		"tree":        {800, 600, newTree().view},
		"datepicker":  {800, 600, (&datepicker{}).view},
		"colorpicker": {800, 600, (&colorpicker{}).view},
		// Nearest-counterpart scenes (scene_nearest.go).
		"select":       {800, 600, newSelect().view},
		"tooltip":      {800, 600, newTooltip().view},
		"table":        {800, 600, newTable().view},
		"pagination":   {800, 600, newPagination().view},
		"cascader":     {800, 600, newCascader().view},
		"transfer":     {800, 600, newTransfer().view},
		"treeselect":   {800, 600, newTreeselect().view},
		"calendar":     {800, 600, newCalendar().view},
		"steps":        {800, 600, newSteps().view},
		"timeline":     {800, 600, newTimeline().view},
		"descriptions": {800, 600, newDescriptions().view},
		"form":         {800, 600, newForm().view},
		"upload":       {800, 600, newUpload().view},
		"carousel":     {800, 600, newCarousel().view},
		"collapse":     {800, 600, newCollapse().view},
		"drawer":       {800, 600, newDrawer().view},
		"tour":         {800, 600, newTour().view},
		"protable":     {800, 600, newProtable().view},
		"segmented":    {800, 600, newSegmented().view},
		"rate":         {800, 600, newRate().view},
		"tags":         {800, 600, newTags().view},
		"badges":       {800, 600, newBadges().view},
		"cards":        {800, 600, newCards().view},
		"statistics":   {800, 600, newStatistics().view},
	}
}

// clock tells frames apart: a frame can build the view more than once,
// with the same c.Now(), and only its first pass advances the scene.
type clock struct {
	prev   time.Time
	frames int // frames seen, the first included
}

func (k *clock) next(c *ui.Context) bool {
	if c.Now().Equal(k.prev) {
		return false
	}
	k.prev = c.Now()
	k.frames++
	return true
}

// The renderer is what MyGo's painter says of the first frame: drawn on
// the GPU (Metal on macOS) or in software.
var (
	probed, accelerated bool
)

// probe watches the first painted frame of root for the renderer.
func probe(root ui.Element) ui.Element {
	if probed {
		return root
	}
	return root.Draw(func(p *ui.Painter, _ ui.Rect) { probed, accelerated = true, p.Accelerated() })
}

// report counts a frame that did the scene's work and gives proto the
// renderer every frame, once the first painted frame told it (proto
// reports it at ready, with every per-second report and on change).
// reportHook, set by the tests only, sees every frame report() counts.
var reportHook func(work int)

func report(work int) {
	if probed {
		name := "software"
		if accelerated {
			name = "metal"
		}
		proto.Renderer(name)
	}
	if reportHook != nil {
		reportHook(work)
	}
	proto.Frame(work)
}

func main() {
	name := flag.String("scene", "counter", "scene: counter, list, grid or a component scene (../scenes.md)")
	flag.Parse()
	s, ok := scenes()[*name]
	if !ok {
		fmt.Fprintf(os.Stderr, "uibench mygo: unknown scene %q\n", *name)
		os.Exit(2)
	}
	// The components' built-in texts in English, as in all three programs,
	// whatever the system's language. MYGO_LOCALE (what `mygo dev -lang`
	// sets) chooses the language for this run and saves nothing, where
	// App.SetLocale would keep it for later launches (mygo locale.go).
	os.Setenv("MYGO_LOCALE", "en")
	proto.Scene(*name, s.width, s.height)
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "uibench mygo " + *name, Width: s.width, Height: s.height,
			UseContentSize: true,
			Content:        ui.View(s.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
