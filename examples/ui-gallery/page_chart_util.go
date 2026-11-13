package main

import (
	"math"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/chart"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// chartPage registers one chart demo three times: Ant Design (Name),
// Element Plus / ECharts style (Name-Element) and Ant dark (Name-Dark).
func chartPage(p gallery.Page) {
	p.Group = gallery.Display
	if p.Order == 0 {
		p.Order = 200
	}
	if p.Width == 0 {
		p.Width = 1100
	}
	setup := p.Setup
	gallery.Register(p)
	el := p
	el.Variant = true
	el.Name, el.Title = p.Name+"-Element", p.Title+"（Element Plus / ECharts）"
	el.Setup = func(d *ui.Driver) {
		t := theme.ElementPlus()
		t.BgLayout = t.BgContainer
		d.SetTheme(t)
		d.Settle()
		if setup != nil {
			setup(d)
		}
	}
	gallery.Register(el)
	dk := p
	dk.Variant = true
	dk.Name, dk.Title = p.Name+"-Dark", p.Title+"（暗色）"
	dk.Setup = func(d *ui.Driver) {
		d.SetTheme(theme.AntDesign(theme.Dark()))
		d.Settle()
		if setup != nil {
			setup(d)
		}
	}
	gallery.Register(dk)
}

// chartGrid lays demo cards out two per row.
func chartGrid(c *ui.Context, cards ...func()) {
	for i := 0; i < len(cards); i += 2 {
		ui.Row(c, func() {
			for j := i; j < i+2; j++ {
				ui.Column(c, func() {
					if j < len(cards) {
						cards[j]()
					}
				}, layout.Grow(1), layout.Basis(0))
			}
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
	}
}

// chartCard is a titled demo card holding one chart.
func chartCard(c *ui.Context, title string, fn func()) {
	ui.Card(c, title).Layout(layout.Margin(geom.Insets{Bottom: 16})).Body(fn)
}

// hoverAt moves the pointer to a fraction of the n-th chart's plot box.
func hoverAt(d *ui.Driver, v *chart.View, fx, fy float32) {
	if v == nil || v.Box == nil {
		return
	}
	r := v.Rect()
	d.Move(geom.Pt(r.X+r.W*fx, r.Y+r.H*fy))
	d.Frame()
}

var months = []string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"}

// cityTemps is a two-series monthly dataset.
var cityTemps = func() []chart.Row {
	tokyo := []float64{7.0, 6.9, 9.5, 14.5, 18.4, 21.5, 25.2, 26.5, 23.3, 18.3, 13.9, 9.6}
	london := []float64{3.9, 4.2, 5.7, 8.5, 11.9, 15.2, 17.0, 16.6, 14.2, 10.3, 6.6, 4.8}
	var out []chart.Row
	for i, m := range months {
		out = append(out, chart.Row{"month": m, "city": "东京", "temp": tokyo[i]})
		out = append(out, chart.Row{"month": m, "city": "伦敦", "temp": london[i]})
	}
	return out
}()

// salesByChannel is a monthly dataset with four series.
var salesByChannel = func() []chart.Row {
	chs := []string{"直接访问", "邮件营销", "联盟广告", "搜索引擎"}
	base := []float64{320, 120, 220, 820}
	var out []chart.Row
	for i, m := range months[:7] {
		for k, ch := range chs {
			v := base[k] * (1 + 0.25*math.Sin(float64(i+k)*0.9))
			out = append(out, chart.Row{"month": m, "channel": ch, "value": math.Round(v)})
		}
	}
	return out
}()

// dailyTraffic is a time series over ~5 months.
var dailyTraffic = func() []chart.Row {
	start := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	var out []chart.Row
	v := 3200.0
	for i := 0; i < 150; i++ {
		v += 180*math.Sin(float64(i)/6) + float64((i*7919)%400-190)
		out = append(out, chart.Row{"date": start.AddDate(0, 0, i), "pv": math.Round(v)})
	}
	return out
}()

// bigSeries is a 10 000-point series for the downsampling demo.
var bigSeries = func() []chart.Row {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]chart.Row, 10000)
	v := 50.0
	seed := uint32(7)
	for i := range out {
		seed = seed*1664525 + 1013904223
		v += (float64(seed>>8)/float64(1<<24) - 0.5) * 2.4
		v += 0.02 * (50 - v)
		out[i] = chart.Row{"t": start.Add(time.Duration(i) * 5 * time.Minute), "v": math.Round(v*100) / 100}
	}
	return out
}()
