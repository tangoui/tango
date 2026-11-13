package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// tablePage registers a table demo in three themes: Ant Design (light),
// Element Plus (light) and Ant Design dark (plus Element dark when
// elementDark is set). mk builds a fresh page (render + setup) for every
// variant so interactions in one screenshot never leak into another.
// Output: components/<Name>.png, <Name>-element.png, <Name>-dark.png.
func tbPage(name, title string, order int, width, height float32, elementDark bool, mk func() (func(c *ui.Context), func(d *ui.Driver))) {
	variants := []struct {
		suffix string
		th     func() *theme.Theme
	}{
		{"", nil},
		{"-element", func() *theme.Theme { return theme.ElementPlus() }},
		{"-dark", func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
	}
	if elementDark {
		variants = append(variants, struct {
			suffix string
			th     func() *theme.Theme
		}{"-element-dark", func() *theme.Theme { return theme.ElementPlus(theme.Dark()) }})
	}
	for _, v := range variants {
		render, setup := mk()
		p := gallery.Page{Name: name + v.suffix, Title: title, Group: gallery.Display, Order: order, Width: width, Height: height, Render: render}
		if name != "ProTable" {
			p.Parent = "Table"
		}
		p.Variant = v.th != nil
		p.Interact = setup
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

// demo is a titled block.
func tbDemo(c *ui.Context, title, desc string, fn func()) {
	gallery.Demo(c, title, desc, fn)
}

// tbPerson is the sample record of the table demos.
type tbPerson struct {
	id      int
	name    string
	age     int
	city    string
	addr    string
	tags    []string
	status  string
	amount  float64
	created time.Time
	score   float64
	dept    string
	math    int
	chinese int
	english int
}

var (
	tbFamily   = []string{"王", "李", "张", "刘", "陈", "杨", "赵", "黄", "周", "吴", "徐", "孙", "胡", "朱", "高", "林"}
	tbGiven    = []string{"伟", "芳", "娜", "秀英", "敏", "静", "丽", "强", "磊", "军", "洋", "勇", "艳", "杰", "娟", "涛", "明", "超", "秀兰", "霞"}
	tbCities   = []string{"杭州", "上海", "北京", "深圳", "成都", "南京", "武汉", "西安"}
	tbStreets  = []string{"西湖区文三路", "滨江区网商路", "朝阳区望京街", "南山区科技园", "高新区天府大道", "鼓楼区中山路", "江汉区解放大道", "雁塔区科技路"}
	tbTagPool  = []string{"nice", "developer", "cool", "teacher", "designer", "loser", "master"}
	tbStatuses = []string{"running", "online", "closed", "error"}
	tbDepts    = []string{"研发部", "设计部", "市场部", "运营部"}
)

// people returns n deterministic sample people.
func tbPeople(n int) []tbPerson {
	out := make([]tbPerson, n)
	seed := uint32(7)
	rnd := func(k int) int {
		seed = seed*1664525 + 1013904223
		return int(seed>>8) % k
	}
	base := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	for i := range out {
		ci := rnd(len(tbCities))
		p := tbPerson{
			id:      i + 1,
			name:    tbFamily[rnd(len(tbFamily))] + tbGiven[rnd(len(tbGiven))],
			age:     22 + rnd(36),
			city:    tbCities[ci],
			addr:    tbCities[ci] + tbStreets[ci] + strconv.Itoa(1+rnd(900)) + "号",
			status:  tbStatuses[rnd(len(tbStatuses))],
			amount:  float64(rnd(2000000)) / 100,
			created: base.Add(time.Duration(rnd(240*24)) * time.Hour),
			score:   float64(rnd(101)),
			dept:    tbDepts[rnd(len(tbDepts))],
			math:    50 + rnd(51),
			chinese: 50 + rnd(51),
			english: 50 + rnd(51),
		}
		for k := 0; k < 1+rnd(2); k++ {
			t := tbTagPool[rnd(len(tbTagPool))]
			if len(p.tags) == 0 || p.tags[0] != t {
				p.tags = append(p.tags, t)
			}
		}
		out[i] = p
	}
	return out
}

var tbStatusText = map[string]string{"running": "运行中", "online": "已上线", "closed": "关闭", "error": "异常"}
var tbStatusBadge = map[string]string{"running": "processing", "online": "success", "closed": "default", "error": "error"}

func tbTagColor(t string) string {
	switch t {
	case "loser":
		return "error"
	case "developer", "designer":
		return "success"
	case "master":
		return "warning"
	}
	return "processing"
}

func tbCmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func tbGap(c *ui.Context, h float32) { ui.Div(c, layout.H(h)) }

func tbMoney(v float64) string { return "¥" + ui.FormatNumber(v, 2) }

var _ = fmt.Sprint
var _ = geom.Pt
