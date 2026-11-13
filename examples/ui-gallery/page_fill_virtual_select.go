package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

const fillVSTotal = 100000

var (
	fillVSOpts   []ui.Option
	fillVSGroups []ui.VirtualOptionGroup
	fillVSDemo   struct {
		single, created string
		multi           []string
		group           string
		remoteVal       string
		remoteOpts      []ui.Option
		remoteQuery     string
		remoteAt        time.Time
		remoteBusy      bool
		tags            []string
		custom          string
	}
	fillVSCities = []string{"北京", "上海", "广州", "深圳", "杭州", "南京", "成都", "重庆", "武汉", "西安", "苏州", "天津"}
)

func fillVSReset() {
	if fillVSOpts == nil {
		fillVSOpts = make([]ui.Option, fillVSTotal)
		for i := range fillVSOpts {
			fillVSOpts[i] = ui.Option{Label: fmt.Sprintf("选项 %06d", i+1), Value: fmt.Sprint(i + 1), Disabled: i%97 == 5}
		}
		for g := 0; g < 1000; g++ {
			grp := ui.VirtualOptionGroup{Label: fmt.Sprintf("分组 %d", g+1)}
			for k := 0; k < 10; k++ {
				v := fmt.Sprintf("g%d-%d", g+1, k+1)
				grp.Options = append(grp.Options, ui.Option{Label: fmt.Sprintf("分组 %d · 选项 %d", g+1, k+1), Value: v})
			}
			fillVSGroups = append(fillVSGroups, grp)
		}
	}
	d := &fillVSDemo
	d.single, d.created, d.group, d.remoteVal, d.custom = "", "", "", "", "3"
	d.multi = []string{"2", "8"}
	d.tags = nil
	d.remoteOpts, d.remoteQuery, d.remoteBusy = nil, "", false
}

func init() {
	fillVSReset()
	morePage(gallery.Page{Name: "VirtualSelect", Title: "VirtualSelect 虚拟化选择器", Group: gallery.Form, Order: 15, Height: 760,
		Render: func(c *ui.Context) {
			d := &fillVSDemo
			gallery.Demo(c, "十万选项", "下拉只构建可见行；支持搜索过滤、键盘上下选择与高亮跟随滚动、清空。", func() {
				moreRow(c, func() {
					ui.VirtualSelect(c, &d.single, fillVSOpts).Placeholder("单选（10 万项）").Filterable().AllowClear().Width(240)
					ui.VirtualSelect(c, nil, fillVSOpts).Multiple(&d.multi).Placeholder("多选，最多 3 项").MaxCount(3).MaxTagCount(2).Filterable().Width(320)
				})
			})
			gallery.Demo(c, "分组、远程搜索、创建条目、自定义选项", "", func() {
				moreRow(c, func() {
					ui.VirtualSelect(c, &d.group, nil).Groups(fillVSGroups).Placeholder("分组（1000 组）").Width(220)
					rs := ui.VirtualSelect(c, &d.remoteVal, d.remoteOpts).Placeholder("远程搜索城市").Remote(func(q string) {
						d.remoteQuery, d.remoteAt, d.remoteBusy = q, c.Now(), true
					}).Loading(d.remoteBusy).Width(200)
					_ = rs
					if d.remoteBusy {
						if c.Now().Sub(d.remoteAt) >= 600*time.Millisecond {
							d.remoteBusy = false
							d.remoteOpts = nil
							for _, s := range fillVSCities {
								if d.remoteQuery != "" && strings.Contains(s, d.remoteQuery) {
									d.remoteOpts = append(d.remoteOpts, ui.Option{Label: s, Value: s})
								}
							}
							c.Invalidate()
						} else {
							c.WakeAt(d.remoteAt.Add(600 * time.Millisecond))
						}
					}
					tw := ui.VirtualSelect(c, nil, ui.Opts("Go", "Rust", "Zig")).Multiple(&d.tags).AllowCreate().Placeholder("输入后回车创建").Width(220)
					if v, ok := tw.Created(); ok {
						d.created = v
					}
					ui.VirtualSelect(c, &d.custom, fillVSOpts[:2000]).OptionRender(func(o ui.Option, s ui.VirtualOptionState) {
						ui.Text(c, o.Label).Layout(layout.Grow(1))
						ui.Text(c, "#"+o.Value).Tertiary()
					}).Width(220)
				})
				if d.created != "" {
					ui.Text(c, "新建："+d.created).Tertiary()
				}
			})
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("单选（10 万项）")
			d.Type("0001")
			d.Settle()
		},
	}, fillVSReset)
}
