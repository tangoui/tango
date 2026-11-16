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
		// Nearest-counterpart scenes (scene_nearest.go).
		"select":       {800, 600, selectRoot},
		"tooltip":      {800, 600, tooltipRoot},
		"table":        {800, 600, tableRoot},
		"pagination":   {800, 600, paginationRoot},
		"cascader":     {800, 600, cascaderRoot},
		"transfer":     {800, 600, transferRoot},
		"treeselect":   {800, 600, treeselectRoot},
		"calendar":     {800, 600, calendarRoot},
		"steps":        {800, 600, stepsRoot},
		"timeline":     {800, 600, timelineRoot},
		"descriptions": {800, 600, descriptionsRoot},
		"form":         {800, 600, formRoot},
		"upload":       {800, 600, uploadRoot},
		"carousel":     {800, 600, carouselRoot},
		"collapse":     {800, 600, collapseRoot},
		"drawer":       {800, 600, drawerRoot},
		"tour":         {800, 600, tourRoot},
		"protable":     {800, 600, protableRoot},
		"segmented":    {800, 600, segmentedRoot},
		"rate":         {800, 600, rateRoot},
		"tags":         {800, 600, tagsRoot},
		"badges":       {800, 600, badgesRoot},
		"cards":        {800, 600, cardsRoot},
		"statistics":   {800, 600, statisticsRoot},
	}
}

var reported bool

// report counts a frame that did the scene's work; the first names the
// renderer, which for NSGo on macOS is always its native views.
// reportHook, set by the tests only, sees every frame report() counts.
var reportHook func(work int)

func report(work int) {
	if !reported {
		reported = true
		proto.Renderer("appkit")
	}
	if reportHook != nil {
		reportHook(work)
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
	// The components' built-in texts in English, as in all three programs,
	// whatever the system's language (Scope.SetLocale, what --nsgo-lang
	// does). NSGo pins a forced language in the app's defaults for later
	// launches only for a bundled app (internal/platform/apple PinLanguage
	// needs a bundle identifier); this program runs unbundled, so nothing
	// is kept.
	root := func(cx *core.Scope) core.Node {
		node := s.root(cx)
		cx.SetLocale("en")
		return node
	}
	nsgo.Run(root, nsgo.WindowSize(s.width, s.height))
}
