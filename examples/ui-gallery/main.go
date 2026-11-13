// Command ui-gallery renders the Tango self-drawn component gallery to PNG
// files with the headless ui.Driver:
//
//	go run ./examples/ui-gallery            # writes build/ui-gallery/*.png
//	go run ./examples/ui-gallery -only Button
//	go run ./examples/ui-gallery -theme shadcn   # <Name>-shadcn.png, <Name>-shadcn-dark.png
//	go run ./examples/ui-gallery -theme hc       # hc/dashboard-antd.png next to hc/dashboard-antd-hc.png, Form, Switch, Checkbox …
//	go run ./examples/ui-gallery -dir rtl        # <Name>-rtl.png: every shot right to left (combines with -theme)
//	go run ./examples/ui-gallery -mirror-check   # LTR/RTL geometry check of every page (prints a summary)
//
// Outputs: index.png (the gallery home: every component by group), overview.png (general + form controls), dashboard-antd.png,
// dashboard-element.png, dashboard-dark.png, overlays.png and one PNG per
// registered component page under components/.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

var (
	outDir = flag.String("out", "build/ui-gallery", "output directory")
	scale  = flag.Float64("scale", 2, "device pixel ratio")
	only   = flag.String("only", "", "render only pages whose name contains this")
	audit  = flag.Bool("audit", false, "run the accessibility audit (every page in the antd, element and shadcn styles) instead of rendering PNGs")
	dirs   = flag.String("dir", "ltr", "direction passes: ltr, rtl (each shot under ConfigProvider.Direction(RTL), written as <name>-rtl.png), both")
	mirror = flag.Bool("mirror-check", false, "render every page left to right and right to left and report nodes that do not mirror (ui.Driver.MirrorCheck) instead of rendering PNGs")
	themes = flag.String("theme", "antd", "theme passes: antd (Ant Design + per-page Element/dark variants), shadcn (every page as <Name>-shadcn and <Name>-shadcn-dark), hc (dashboard, Form, Switch and Checkbox pages of every style, default and HighContrast, under hc/), all")
)

// themePass is one extra rendering of every page with a whole theme.
type themePass struct {
	suffix string
	th     func() *theme.Theme
}

var shadcnPasses = []themePass{
	{"-shadcn", func() *theme.Theme { return theme.Shadcn() }},
	{"-shadcn-dark", func() *theme.Theme { return theme.Shadcn(theme.Dark()) }},
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// dirPass is one direction the shots are rendered in.
type dirPass struct {
	suffix string
	rtl    bool
}

func dirPasses() ([]dirPass, error) {
	switch *dirs {
	case "ltr":
		return []dirPass{{"", false}}, nil
	case "rtl":
		return []dirPass{{"-rtl", true}}, nil
	case "both":
		return []dirPass{{"", false}, {"-rtl", true}}, nil
	}
	return nil, fmt.Errorf("unknown -dir %q (ltr, rtl, both)", *dirs)
}

// rtlPage lays page out right to left (English text: the layout and
// component mirroring is what the pass checks).
func rtlPage(page func(*ui.Context)) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.ConfigProvider(c).Direction(ui.RTL).Body(func() { page(c) })
	}
}

func shot(name string, w, h float32, th *theme.Theme, page func(*ui.Context), setup func(*ui.Driver)) error {
	ds, err := dirPasses()
	if err != nil {
		return err
	}
	for _, dp := range ds {
		dash, ov = newDash(), newOverlayState()
		pg := page
		if dp.rtl {
			pg = rtlPage(page)
		}
		if err := shotOnce(name+dp.suffix, w, h, th, pg, setup); err != nil {
			return err
		}
	}
	return nil
}

func shotOnce(name string, w, h float32, th *theme.Theme, page func(*ui.Context), setup func(*ui.Driver)) error {
	fit := h == 0
	if fit {
		h = 3000
	}
	d := ui.NewDriver(page, ui.DriverOptions{Width: w, Height: h, Scale: float32(*scale), Theme: th})
	d.Settle()
	if setup != nil {
		setup(d)
		d.Settle()
	}
	if fit {
		cs := d.ContentSize()
		d.Resize(w, max(200, min(cs.H, 3000)))
		d.Settle()
	}
	p := filepath.Join(*outDir, name+".png")
	if err := d.SavePNG(p); err != nil {
		return err
	}
	fmt.Println("wrote", p)
	return nil
}

func run() error {
	if *audit {
		res := auditGallery(*only)
		res.write(os.Stdout)
		if n := res.missingNames(); n > 0 {
			return fmt.Errorf("%d interactive nodes without an accessible name", n)
		}
		if n := res.count(ui.AuditNoRole); n > 0 {
			return fmt.Errorf("%d nodes without a role", n)
		}
		if n := res.count(ui.AuditNotKeyboard); n > 0 {
			return fmt.Errorf("%d controls not reachable by keyboard", n)
		}
		if def, hc := res.contrastFailures(); def > 0 || hc > 0 {
			return fmt.Errorf("contrast: %d required pairs failing (default themes), %d (high contrast)", def, hc)
		}
		return nil
	}
	if *mirror {
		res := mirrorGallery(*only)
		res.write(os.Stdout)
		return nil // informational: violations do not fail the run
	}
	if _, err := dirPasses(); err != nil {
		return err
	}
	switch *themes {
	case "antd":
		return runAnt()
	case "shadcn":
		return runPasses(shadcnPasses)
	case "hc":
		return runHighContrast()
	case "all":
		if err := runAnt(); err != nil {
			return err
		}
		return runPasses(shadcnPasses)
	}
	return fmt.Errorf("unknown -theme %q (antd, shadcn, hc, all)", *themes)
}

func want(n string) bool {
	return *only == "" || strings.Contains(strings.ToLower(n), strings.ToLower(*only))
}

func white(t *theme.Theme) *theme.Theme { t.BgLayout = t.BgContainer; return t }

// runPasses renders every non-variant page, the overview and the
// dashboards once per pass, adding the pass suffix to the file name.
func runPasses(passes []themePass) error {
	for _, ps := range passes {
		if want("index" + ps.suffix) {
			sel := "home"
			home := func(c *ui.Context) { gallery.Home(c, &sel) }
			if err := shot("index"+ps.suffix, 1440, 0, ps.th(), home, nil); err != nil {
				return err
			}
		}
		if want("overview" + ps.suffix) {
			if err := shot("overview"+ps.suffix, 1200, 0, white(ps.th()), overview, nil); err != nil {
				return err
			}
		}
		if want("dashboard" + ps.suffix) {
			if err := shot("dashboard"+ps.suffix, 1440, 900, ps.th(), dashboard, nil); err != nil {
				return err
			}
		}
		if want("overlays" + ps.suffix) {
			if err := shot("overlays"+ps.suffix, 1200, 760, ps.th(), overlays, overlaysSetup); err != nil {
				return err
			}
		}
		for _, p := range gallery.Pages() {
			if p.Variant || !want(p.Name+ps.suffix) {
				continue
			}
			w := p.Width
			if w == 0 {
				w = 960
			}
			p := p
			setup := p.Interact
			if setup == nil {
				setup = p.Setup
			}
			page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
			if p.Bare {
				page = p.Render
			}
			if err := shot(filepath.Join("components", p.Name+ps.suffix), w, p.Height, white(ps.th()), page, setup); err != nil {
				return err
			}
		}
	}
	return nil
}

func runAnt() error {
	if want("index") {
		sel := "home"
		home := func(c *ui.Context) { gallery.Home(c, &sel) }
		if err := shot("index", 1440, 0, theme.AntDesign(), home, nil); err != nil {
			return err
		}
	}
	if want("overview") {
		if err := shot("overview", 1200, 0, white(theme.AntDesign()), overview, nil); err != nil {
			return err
		}
	}
	if want("dashboard") {
		if err := shot("dashboard-antd", 1440, 900, theme.AntDesign(), dashboard, nil); err != nil {
			return err
		}
		if err := shot("dashboard-element", 1440, 900, theme.ElementPlus(), dashboard, nil); err != nil {
			return err
		}
		if err := shot("dashboard-dark", 1440, 900, theme.AntDesign(theme.Dark()), dashboard, nil); err != nil {
			return err
		}
	}
	if want("overlays") {
		if err := shot("overlays", 1200, 760, theme.AntDesign(), overlays, overlaysSetup); err != nil {
			return err
		}
		if err := shot("overlays-element", 1200, 760, theme.ElementPlus(), overlays, overlaysSetup); err != nil {
			return err
		}
	}
	for _, p := range gallery.Pages() {
		if !want(p.Name) {
			continue
		}
		w := p.Width
		if w == 0 {
			w = 960
		}
		p := p
		page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
		if p.Bare {
			page = p.Render
		}
		if err := shot(filepath.Join("components", p.Name), w, p.Height, white(theme.AntDesign()), page, p.Setup); err != nil {
			return err
		}
	}
	return nil
}
