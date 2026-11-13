package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// fpPage registers an advanced-form demo in three themes (Ant Design,
// Element Plus, Ant Design dark). mk builds a fresh page for every variant.
// Output: components/<Name>.png, <Name>-element.png, <Name>-dark.png.
func fpPage(name, title string, order int, width, height float32, mk func() (func(c *ui.Context), func(d *ui.Driver))) {
	variants := []struct {
		suffix string
		th     func() *theme.Theme
	}{
		{"", nil},
		{"-element", func() *theme.Theme { return theme.ElementPlus() }},
		{"-dark", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
	}
	for _, v := range variants {
		render, setup := mk()
		p := gallery.Page{Name: name + v.suffix, Title: title, Group: gallery.Form, Order: order, Width: width, Height: height, Render: render}
		p.Variant = v.th != nil
		p.Interact = setup
		if name != "ProForm" {
			p.Parent = "ProForm"
		}
		thf := v.th
		p.Setup = func(d *ui.Driver) {
			if thf != nil {
				t := thf()
				t.BgLayout = t.BgContainer
				d.SetTheme(t)
				d.Settle()
			}
			if setup != nil {
				setup(d)
			}
		}
		gallery.Register(p)
	}
}

var fpCities = map[string][]ui.ProEnum{
	"zj": {{Value: "hz", Label: "杭州"}, {Value: "nb", Label: "宁波"}, {Value: "wz", Label: "温州"}},
	"js": {{Value: "nj", Label: "南京"}, {Value: "sz", Label: "苏州"}, {Value: "wx", Label: "无锡"}},
	"gd": {{Value: "gz", Label: "广州"}, {Value: "sz2", Label: "深圳"}},
}

var fpProvinces = []ui.ProEnum{{Value: "zj", Label: "浙江"}, {Value: "js", Label: "江苏"}, {Value: "gd", Label: "广东"}}
