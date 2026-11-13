package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// Only used by the native screenshot helper declared in the reused gallery
// source. Browser previews never execute screenshot Setup hooks.
var outDir = new(string)

type livePage struct {
	Name    string  `json:"name"`
	Title   string  `json:"title"`
	Group   string  `json:"group"`
	Parent  string  `json:"parent,omitempty"`
	Variant bool    `json:"variant,omitempty"`
	Source  string  `json:"source"`
	Line    int     `json:"line"`
	Width   float32 `json:"width,omitempty"`
	Height  float32 `json:"height,omitempty"`
}

func livePages() []livePage {
	out := make([]livePage, 0)
	for _, p := range gallery.Pages() {
		if p.Render == nil {
			continue
		}
		file, line := runtime.FuncForPC(reflect.ValueOf(p.Render).Pointer()).FileLine(0)
		if i := strings.LastIndex(file, "gallery_page_"); i >= 0 {
			file = "examples/ui-gallery/" + file[i+len("gallery_"):]
		}
		if i := strings.LastIndex(file, "/examples/ui-gallery/"); i >= 0 {
			file = file[i+1:]
		}
		out = append(out, livePage{Name: p.Name, Width: p.Width, Height: p.Height, Title: p.Title, Group: p.Group, Parent: p.Parent, Variant: gallery.IsVariant(p), Source: file, Line: line})
	}
	return out
}

// Screenshot-only gallery callbacks are never invoked by a live page. Keep
// their shared symbol resolvable without bundling the native screenshot CLI.
func shotOnce(_ string, _, _ float32, _ *theme.Theme, _ func(*ui.Context), _ func(*ui.Driver)) error {
	return fmt.Errorf("screenshot export is available in the native ui-gallery command")
}
