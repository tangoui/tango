package main

import (
	"path/filepath"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// runHighContrast renders the dashboard and the Form, Switch and Checkbox
// pages of every style, light and dark, once with the stock theme and once with
// theme.HighContrast, as hc/<page>-<style>[-dark].png and the same name
// with -hc, for side-by-side comparison.
func runHighContrast() error {
	styles := []struct {
		name string
		th   func(dark bool) *theme.Theme
	}{
		{"antd", func(d bool) *theme.Theme { return theme.AntDesign(darkOpt(d)...) }},
		{"element", func(d bool) *theme.Theme { return theme.ElementPlus(darkOpt(d)...) }},
		{"shadcn", func(d bool) *theme.Theme {
			if d {
				return theme.Shadcn(theme.Dark())
			}
			return theme.Shadcn()
		}},
	}
	for _, st := range styles {
		for _, dark := range []bool{false, true} {
			suffix := "-" + st.name
			if dark {
				suffix += "-dark"
			}
			for _, hc := range []bool{false, true} {
				th := func() *theme.Theme {
					t := st.th(dark)
					if hc {
						t = theme.HighContrast(t)
					}
					return t
				}
				s := suffix
				if hc {
					s += "-hc"
				}
				if want("dashboard" + s) {
					if err := shot(filepath.Join("hc", "dashboard"+s), 1440, 900, th(), dashboard, nil); err != nil {
						return err
					}
				}
				for _, p := range gallery.Pages() {
					if (p.Name != "Form" && p.Name != "Switch" && p.Name != "Checkbox") || !want(p.Name+s) {
						continue
					}
					p := p
					page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
					if err := shot(filepath.Join("hc", p.Name+s), 720, 0, white(th()), page, nil); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func darkOpt(dark bool) []theme.Option {
	if dark {
		return []theme.Option{theme.Dark()}
	}
	return nil
}
