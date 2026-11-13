package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// Mirror check of every gallery page (go run ./examples/ui-gallery
// -mirror-check): each non-variant page is rendered left to right and right
// to left in the Ant Design theme with ui.Driver.MirrorCheck, and nodes
// whose RTL rectangle is not the mirror of the LTR one are listed, then
// directional icons drawn unmirrored in the RTL frame (icon check). Pages
// are checked without their Setup interaction, so both passes start from
// the same application state.

// mirrorPageResult is the check of one page.
type mirrorPageResult struct {
	name string
	rep  ui.MirrorReport
}

// mirrorResult sums up a gallery mirror check.
type mirrorResult struct {
	pages []mirrorPageResult
}

func mirrorGallery(only string) mirrorResult {
	var res mirrorResult
	for _, p := range gallery.Pages() {
		if p.Variant || p.Render == nil || (only != "" && !want(p.Name)) {
			continue
		}
		w, h := p.Width, p.Height
		if w == 0 {
			w = 960
		}
		if h == 0 {
			h = 1400
		}
		p := p
		page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
		if p.Bare {
			page = p.Render
		}
		dash, ov = newDash(), newOverlayState()
		d := ui.NewDriver(page, ui.DriverOptions{Width: w, Height: h, Theme: white(theme.AntDesign())})
		// A first check warms up page state kept in package variables
		// (one-time validation, lazily filled data), so the reported pass
		// renders LTR and RTL from the same state.
		d.MirrorCheck(page)
		res.pages = append(res.pages, mirrorPageResult{name: p.Name, rep: d.MirrorCheck(page)})
	}
	return res
}

// write prints totals, then the pages with violations (most first) with
// up to five violations each, then every unmirrored directional icon.
func (r mirrorResult) write(w io.Writer) {
	checked, skipped, bad, total, icons, iconPages, iconSkipped := 0, 0, 0, 0, 0, 0, 0
	for _, p := range r.pages {
		checked += p.rep.Checked
		skipped += p.rep.Skipped
		if n := len(p.rep.Violations); n > 0 {
			bad++
			total += n
		}
		if n := len(p.rep.Icons); n > 0 {
			iconPages++
			icons += n
		}
		iconSkipped += p.rep.IconsSkipped
	}
	fmt.Fprintf(w, "mirror check: %d pages, %d nodes checked (%d exempt), %d pages with violations, %d violations\n", len(r.pages), checked, skipped, bad, total)
	ps := append([]mirrorPageResult(nil), r.pages...)
	sort.SliceStable(ps, func(i, j int) bool { return len(ps[i].rep.Violations) > len(ps[j].rep.Violations) })
	for _, p := range ps {
		if len(p.rep.Violations) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s: %d of %d nodes\n", p.name, len(p.rep.Violations), p.rep.Checked)
		for i, v := range p.rep.Violations {
			if i == 5 {
				fmt.Fprintf(w, "  … %d more\n", len(p.rep.Violations)-5)
				break
			}
			fmt.Fprintf(w, "  %s\n", v)
		}
	}
	fmt.Fprintf(w, "icon check: %d unmirrored directional icons on %d pages (%d exempt under NoMirror)\n", icons, iconPages, iconSkipped)
	for _, p := range r.pages {
		for _, v := range p.rep.Icons {
			fmt.Fprintf(w, "  %s: %s\n", p.name, v)
		}
	}
}
