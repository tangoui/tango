// Command nsgo is uibench's NSGo program: `nsgo -scene <name>` runs the
// scene with NSGo's default drawing, the platform's own views (AppKit on
// macOS), and reports frames on standard output
// (tango.local/uibench/proto). Frames are driven by chaining
// Scope.RequestAnimationFrame, NSGo's frame clock.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"tango.local/uibench/proto"
)

// scene is a window size and the root component drawn in it.
type scene struct {
	width, height core.Points
	root          func(cx *core.Scope) core.Node
}

func scenes() map[string]scene {
	return map[string]scene{
		"counter": {360, 260, counterRoot},
		"list":    {800, 600, listRoot},
		"grid":    {800, 600, gridRoot},
		// Component scenes (scene_components.go).
		"buttons":     {800, 600, buttonsRoot},
		"inputs":      {800, 600, inputsRoot},
		"textarea":    {800, 600, textareaRoot},
		"checkboxes":  {800, 600, checkboxesRoot},
		"radios":      {800, 600, radiosRoot},
		"switches":    {800, 600, switchesRoot},
		"sliders":     {800, 600, slidersRoot},
		"progress":    {800, 600, progressRoot},
		"tabs":        {800, 600, tabsRoot},
		"menu":        {800, 600, menuRoot},
		"dialog":      {800, 600, dialogRoot},
		"toast":       {800, 600, toastRoot},
		"tree":        {800, 600, treeRoot},
		"datepicker":  {800, 600, datepickerRoot},
		"colorpicker": {800, 600, colorpickerRoot},
	}
}

var reported bool

// report counts a frame that did the scene's work; the first names the
// renderer, which for NSGo on macOS is always its native views.
func report(work int) {
	if !reported {
		reported = true
		proto.Renderer("appkit")
	}
	proto.Frame(work)
}

func main() {
	// NSGo reads its own flags from os.Args; take -scene off first.
	fs := flag.NewFlagSet("nsgo", flag.ContinueOnError)
	name := fs.String("scene", "counter", "scene: counter, list, grid or a component scene (../scenes.md)")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	s, ok := scenes()[*name]
	if !ok {
		fmt.Fprintf(os.Stderr, "uibench nsgo: unknown scene %q\n", *name)
		os.Exit(2)
	}
	os.Args = append(os.Args[:1], fs.Args()...)
	nsgo.Run(s.root, nsgo.WindowSize(s.width, s.height))
}
