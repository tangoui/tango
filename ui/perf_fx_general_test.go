package ui

import (
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// All mutable fixture values belong to the driver's Context, including when
// the same Build closure is used by several drivers (mount/large scenes).
var perfGeneralID = hashID("perf.general.changed")

func perfGeneralChanged(c *Context) *bool { return State[bool](c, perfGeneralID, nil) }

func perfGeneralLabel(changed bool) string {
	if changed {
		return "已更新 Updated"
	}
	return "默认内容 Default"
}

func perfGeneralToggle(r *perfRun, i int) {
	*perfGeneralChanged(r.D.r.c) = i%2 == 0
	r.D.r.Invalidate()
	r.D.Frame()
}

// Display/layout APIs have no intrinsic input: change their label or style,
// measuring a real rebuild rather than moving over a non-interactive node.
func perfGeneralDisplay(name string, build func(*Context, bool), covers ...string) perfFixture {
	return perfFixture{
		Name: name, Covers: covers,
		Build:    func(c *Context) { build(c, *perfGeneralChanged(c)) },
		Interact: perfGeneralToggle,
	}
}

// Target can be the union of several controls: find the intended control,
// never assume the union's center is interactive.
func perfGeneralHover(label string) func(*perfRun, int) {
	return func(r *perfRun, i int) {
		if i%2 != 0 {
			r.D.Move(perfOutside)
			return
		}
		box, ok := r.D.Find(label)
		if !ok {
			panic("perf: general control not found: " + label)
		}
		r.D.Move(box.Center())
	}
}

func perfGeneralClick(r *perfRun, label string) {
	box, ok := r.D.Find(label)
	if !ok {
		panic("perf: general toggle not found: " + label)
	}
	r.D.Click(box.Center())
	// Restore pointer/focus too, not just the visible expanded state.
	r.D.Click(perfOutside)
}

// Scroll/ScrollH are intentionally not virtualized: n means n laid-out
// children even for Large100k. Masonry separately exercises its virtual API.
func perfGeneralScrollPage(n int, horizontal bool) func(*Context) {
	return func(c *Context) {
		*State[bool](c, hashID("perf.general.scrollRTL"), nil) = c.IsRTL()
		body := func() {
			for i := 0; i < n; i++ {
				Text(c, "项目 Item "+strconv.Itoa(i+1)).Layout(layout.Size(160, 32))
			}
		}
		if horizontal {
			ScrollH(c, body, layout.Size(640, 120)).Always()
		} else {
			Scroll(c, body, layout.Size(640, 320)).Always()
		}
	}
}

func perfGeneralWheel(horizontal bool, large bool) func(*perfRun, int) {
	return func(r *perfRun, i int) {
		delta := float32(120)
		if (!large && i%2 == 1) || (large && i%40 >= 20) {
			delta = -delta
		}
		e := input.Scroll{Pos: r.Center(), DY: delta}
		if horizontal {
			e.DX, e.DY = delta, 0
			// The wheel delta is physical, while ScrollH offsets are logical.
			// Read the direction captured inside the Build's provider scope.
			if *State[bool](r.D.r.c, hashID("perf.general.scrollRTL"), nil) {
				e.DX = -e.DX
			}
		}
		r.D.Inject(e) // perfStep runs the queued frame, without an empty follow-up.
	}
}

func perfGeneralMasonryPage(n int, virtual bool) func(*Context) {
	return func(c *Context) {
		height := func(i int) float32 { return [...]float32{120, 60, 150, 90, 70, 130}[i%6] }
		m := Masonry(c, n, func(i int) {
			Column(c, func() {
				Text(c, "卡片 Card "+strconv.Itoa(i+1)).Strong()
				Text(c, perfGeneralLabel(*perfGeneralChanged(c))).Secondary()
			}, layout.Pad(12), layout.H(height(i))).Bg(c.Theme.FillSecondary).Radius(c.Theme.RadiusLG)
		}).Columns(4).Gutter(16).ItemHeight(height)
		if virtual {
			m.Virtual(360)
		}
		m.Show()
	}
}

func init() {
	registerPerf(
		perfFixture{
			Name: "ButtonGroup",
			Build: func(c *Context) {
				ButtonGroup(c).Body(func() {
					Button(c, "上一页").Icon(icon.Left)
					Button(c, "下一页").IconRight(icon.Right)
				})
			},
			Interact: perfGeneralHover("上一页"),
		},
		perfFixture{
			Name:     "IconButton",
			Build:    func(c *Context) { IconButton(c, icon.Edit, "编辑") },
			Interact: perfGeneralHover("编辑"),
		},
		perfGeneralDisplay("Icon", func(c *Context, changed bool) {
			ic := icon.Star
			if changed {
				ic = icon.Check
			}
			Icon(c, ic).Size(24)
		}),
		perfFixture{
			Name: "Link", Build: func(c *Context) { Link(c, "查看文档 Documentation") },
			Interact: perfGeneralHover("查看文档 Documentation"),
		},
		perfGeneralDisplay("Text", func(c *Context, changed bool) { Text(c, perfGeneralLabel(changed)) }),
		perfGeneralDisplay("Title", func(c *Context, changed bool) { Title(c, perfGeneralLabel(changed), 2) }),
		perfGeneralDisplay("Paragraph", func(c *Context, changed bool) {
			Paragraph(c, perfGeneralLabel(changed)+"：这是一段中英混合的说明文字。A paragraph wraps inside a bounded width, with a second sentence to exercise typography layout.").Layout(layout.W(480))
		}),
		perfGeneralDisplay("Kbd", func(c *Context, changed bool) {
			keys := "Ctrl K"
			if changed {
				keys = "Ctrl P"
			}
			Kbd(c, keys)
		}),
		perfGeneralDisplay("Divider", func(c *Context, changed bool) { Divider(c, perfGeneralLabel(changed)) }),
		perfGeneralDisplay("Space", func(c *Context, changed bool) {
			Space(c).Split(func() { VDivider(c) }).Body(func() {
				Text(c, perfGeneralLabel(changed))
				Text(c, "右侧 Right")
				Text(c, "第三项 Third")
			})
		}, "VDivider"),
		perfGeneralDisplay("Flex", func(c *Context, changed bool) {
			Flex(c).Gap(Middle).Wrap().Style(layout.W(480)).Body(func() {
				Text(c, perfGeneralLabel(changed))
				Button(c, "提交")
				Button(c, "取消")
			})
		}),
		perfGeneralDisplay("Row", func(c *Context, changed bool) {
			Row(c, func() {
				Column(c, func() {
					Text(c, perfGeneralLabel(changed))
					Gap(c, 12)
					Text(c, "辅助说明 Description").Secondary()
				})
				Spacer(c)
				Button(c, "操作")
			}, layout.W(640), layout.Middle())
		}, "Column", "Spacer", "Gap"),
		perfGeneralDisplay("GridRow", func(c *Context, changed bool) {
			GridRow(c).Gutter(16, 16).Body(func() {
				Col(c, 8).Body(func() { Text(c, perfGeneralLabel(changed)) })
				Col(c, 8).Body(func() { Text(c, "第二列 Column 2") })
				Col(c, 8).Body(func() { Text(c, "第三列 Column 3") })
			})
		}, "Col"),
		perfGeneralDisplay("Div", func(c *Context, changed bool) {
			bg := c.Theme.FillSecondary
			if changed {
				bg = c.Theme.Primary.Bg
			}
			Div(c, layout.Size(320, 100), layout.Pad(16)).Bg(bg).Radius(c.Theme.Radius).Body(func() { Text(c, "容器 Box") })
		}),
		perfGeneralDisplay("StackBox", func(c *Context, changed bool) {
			StackBox(c, func() {
				Div(c, layout.Size(320, 120)).Bg(c.Theme.FillSecondary)
				Text(c, perfGeneralLabel(changed)).Layout(layout.Abs(16, 16, -1, -1))
			}, layout.Size(320, 120))
		}),
		perfFixture{
			Name: "Compact",
			Build: func(c *Context) {
				Compact(c).Body(func() { Button(c, "保存"); Button(c, "另存为"); Button(c, "取消") })
			},
			Interact: perfGeneralHover("保存"),
		},
		perfGeneralDisplay("AspectRatio", func(c *Context, changed bool) {
			AspectRatio(c, 16.0/9, func() {
				Div(c, layout.Grow(1), layout.Pad(16)).Bg(c.Theme.FillSecondary).Body(func() { Text(c, perfGeneralLabel(changed)) })
			}, layout.W(400)).Radius(8)
		}),
		perfFixture{
			Name: "Splitter", Covers: []string{"SplitterPanel"},
			Build: func(c *Context) {
				Splitter(c).Style(layout.Size(606, 240)).Body(func() {
					SplitterPanel(c).DefaultSize(200).CollapseToggle("折叠侧栏", "展开侧栏").Body(func() { Text(c, "工程 Files") })
					SplitterPanel(c).Body(func() { Text(c, "编辑区域 Editor") })
				})
			},
			Interact: func(r *perfRun, _ int) { perfGeneralClick(r, "collapse-toggle") },
		},
		perfGeneralDisplay("Layout", func(c *Context, changed bool) {
			Layout(c, func() {
				Header(c, func() { Text(c, "Header").Color(c.Theme.BgContainer) })
				Layout(c, func() {
					Sider(c, 160, false, func() { Text(c, "Sider").Color(c.Theme.BgContainer) })
					Content(c, func() { Text(c, perfGeneralLabel(changed)) }, layout.Pad(24))
				})
				Footer(c, func() { Text(c, "Footer") })
			}, layout.Size(760, 360), layout.Grow(0))
		}, "Header", "Footer", "Content", "Sider"),
		perfFixture{
			Name: "CollapsibleSider",
			Build: func(c *Context) {
				collapsed := State[bool](c, hashID("perf.general.collapsed"), nil)
				Layout(c, func() {
					CollapsibleSider(c, 200, collapsed).Light().Body(func() { Text(c, "导航 Navigation") })
					Content(c, func() { Text(c, "工作区 Workspace") }, layout.Pad(24))
				}, layout.Size(760, 320), layout.Grow(0))
			},
			Interact: func(r *perfRun, i int) {
				label := r.D.r.c.Locale().CollapseMenu
				if i%2 == 1 {
					label = r.D.r.c.Locale().ExpandMenu
				}
				perfGeneralClick(r, label)
			},
		},
		perfFixture{
			Name: "Scroll", Build: perfGeneralScrollPage(50, false), Interact: perfGeneralWheel(false, false),
			Large: func(n int) func(*Context) { return perfGeneralScrollPage(n, false) },
		},
		perfFixture{
			Name: "ScrollH", Build: perfGeneralScrollPage(50, true), Interact: perfGeneralWheel(true, false),
			Large: func(n int) func(*Context) { return perfGeneralScrollPage(n, true) }, LargeInteract: perfGeneralWheel(true, true),
		},
		perfFixture{
			Name: "Masonry", Build: perfGeneralMasonryPage(16, false), Interact: perfGeneralToggle,
			Large: func(n int) func(*Context) { return perfGeneralMasonryPage(n, true) },
		},
		perfGeneralDisplay("Watermark", func(c *Context, changed bool) {
			Watermark(c, "Tango", perfGeneralLabel(changed)).Layout(layout.Size(640, 320)).Body(func() {
				Text(c, "水印下的文档内容 Document content")
			})
		}),
		perfGeneralDisplay("BorderBeam", func(c *Context, changed bool) {
			col := c.Theme.Primary.Base
			if changed {
				col = c.Theme.Error.Base
			}
			BorderBeam(c).Color(col).LineWidth(2).Layout(layout.Size(320, 120), layout.Pad(16)).Body(func() { Text(c, "AI 助手 Assistant") })
		}),
		perfFixture{
			Name: "App",
			Build: func(c *Context) {
				App(c).Layout(layout.Size(400, 180), layout.Grow(0)).Body(func() { Button(c, "应用内操作") })
			},
			Interact: perfGeneralHover("应用内操作"),
		},
		perfGeneralDisplay("ConfigProvider", func(c *Context, changed bool) {
			sz := Middle
			if changed {
				sz = Large
			}
			ConfigProvider(c).Size(sz).Body(func() { Button(c, "配置作用域") })
		}),
		perfGeneralDisplay("Disabled", func(c *Context, changed bool) {
			Disabled(c, !changed, func() { Button(c, "禁用作用域") })
		}),
		perfGeneralDisplay("Custom", func(c *Context, changed bool) {
			bg := c.Theme.FillSecondary
			if changed {
				bg = c.Theme.Primary.Bg
			}
			Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 320, H: 100} }, nil, layout.Pad(16)).Bg(bg).Body(func() { Button(c, "自定义容器") })
		}),
		perfFixture{Name: "UseApp", Skip: "Reads the enclosing App API; emits no visible node."},
		perfFixture{Name: "State", Skip: "Context-local state storage; emits no visible node."},
		perfFixture{Name: "Prop", Skip: "Reads a configured property; emits no visible node."},
		perfFixture{Name: "Key", Skip: "Scopes child identity; emits no visible node of its own."},
		perfFixture{Name: "HoverDelay", Skip: "Sets hover timing policy; emits no visible node."},
		perfFixture{Name: "ScreenBreakpoint", Skip: "Queries the window breakpoint; emits no visible node."},
	)
}

func TestPerfGeneralContextIsolation(t *testing.T) {
	fx := perfRegistry["Text"]
	a := newPerfDriver(perfPage(fx.Build), false)
	b := newPerfDriver(perfPage(fx.Build), false)
	a.Settle()
	b.Settle()
	initial := perfListHash(b)
	perfStep(&perfRun{D: a}, fx.Interact, 0)
	b.Frame()
	if perfListHash(b) != initial || *perfGeneralChanged(b.r.c) {
		t.Fatal("fixture interaction leaked into another driver's Context")
	}
	if perfListHash(a) == initial {
		t.Fatal("fixture interaction did not update its own driver")
	}
}

func TestPerfGeneralScrollCounts(t *testing.T) {
	for _, horizontal := range []bool{false, true} {
		d := newPerfDriver(perfPage(perfGeneralScrollPage(1000, horizontal)), false)
		d.Settle()
		scroll := d.r.c.root.kids[0].kids[0]
		if got := len(scroll.kids); got != 1000 {
			t.Fatalf("horizontal=%v: built %d items, want 1000 (no hidden cap)", horizontal, got)
		}
		if _, ok := d.FindText("项目 Item 1000"); !ok {
			t.Fatal("last requested item was not built")
		}
	}
}
