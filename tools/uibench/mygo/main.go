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
	probed, accelerated, reported bool
)

// probe watches the first painted frame of root for the renderer.
func probe(root ui.Element) ui.Element {
	if probed {
		return root
	}
	return root.Draw(func(p *ui.Painter, _ ui.Rect) { probed, accelerated = true, p.Accelerated() })
}

// report counts a frame that did the scene's work; the first names the
// renderer.
func report(work int) {
	if !reported {
		reported = true
		name := "software"
		switch {
		case !probed:
			name = "unknown"
		case accelerated:
			name = "metal"
		}
		proto.Renderer(name)
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
