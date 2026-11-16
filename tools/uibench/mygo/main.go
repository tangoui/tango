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
	name := flag.String("scene", "counter", "scene: counter, list or grid")
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
