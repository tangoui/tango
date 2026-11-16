package ui

import (
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"strconv"
	"testing"
)

var perfProID = hashID("perf.pro")

func perfProToggle(r *perfRun, i int) {
	if i%2 == 0 {
		r.D.Click(r.Center())
	} else {
		r.D.Key(input.KeyEscape, 0)
		r.D.Click(perfOutside)
	}
}
func perfProRows(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
func perfProFields() []ProField {
	return []ProField{{Key: "name", Label: "名称"}, {Key: "status", Label: "状态", ValueType: ValueSelect}}
}
func perfProCols() []ProColumn[int] {
	return []ProColumn[int]{{Key: "id", Title: "编号", Value: func(i int) any { return i + 1 }}, {Key: "name", Title: "名称", Value: func(i int) any { return "项目 " + strconv.Itoa(i+1) }}}
}
func init() {
	registerPerf(
		perfFixture{Name: "ProLayout", Build: func(c *Context) { ProLayout(c, ProLayoutConfig{}, func() { Button(c, "工作台") }) }},
		perfFixture{Name: "PageContainer", Build: func(c *Context) { PageContainer(c, "页面").Body(func() { Button(c, "内容") }) }},
		perfFixture{Name: "PageHeader", Build: func(c *Context) { PageHeader(c, "页面标题") }},
		perfFixture{Name: "GridContent", Build: func(c *Context) { GridContent(c).Body(func() { Button(c, "内容") }) }},
		perfFixture{Name: "FooterToolbar", Build: func(c *Context) { FooterToolbar(c).Body(func() { Button(c, "保存") }) }},
		perfFixture{Name: "DefaultFooter", Build: func(c *Context) { DefaultFooter(c) }},
		perfFixture{Name: "SettingDrawer", Overlay: true, Build: func(c *Context) {
			s := &ProSettings{}
			open := State(c, hashID("perf.pro.open"), func() bool { return true })
			if Button(c, "打开页面设置").Clicked() {
				*open = true
			}
			SettingDrawer(c, s).Open(open).NoTrigger().Show()
		}},
		perfFixture{Name: "ProSettingsProvider", Build: func(c *Context) { ProSettingsProvider(c, &ProSettings{}, func() { Button(c, "设置") }) }},
		perfFixture{Name: "RouteContext", Build: func(c *Context) { _ = RouteContext(c); Button(c, "路由") }},
		perfFixture{Name: "ProTable", Build: func(c *Context) {
			ProTable(c, perfProCols()).Request(func(req ProRequest, done func(ProResult[int])) {
				done(ProResult[int]{Rows: perfProRows(20), Total: 20})
			}).Show()
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				Scroll(c, func() {
					ProTable(c, perfProCols()).PageSize(n).Request(func(req ProRequest, done func(ProResult[int])) { done(ProResult[int]{Rows: perfProRows(n), Total: n}) }).Show()
				}, layout.Size(640, 300))
			}
		}},
		perfFixture{Name: "EditableProTable", Build: func(c *Context) {
			rows := State(c, hashID("editable"), func() []int { return perfProRows(5) })
			EditableProTable(c, perfProCols(), rows).Show()
		}},
		perfFixture{Name: "ProList", Build: func(c *Context) { ProList[int](c, perfProRows(5), ProListMetas[int]{}).Show() }, Large: func(n int) func(*Context) {
			return func(c *Context) {
				Scroll(c, func() {
					ProList[int](c, perfProRows(n), ProListMetas[int]{Title: func(i int) string { return perfCollItem(i) }}).Show()
				}, layout.Size(640, 300))
			}
		}},
		perfFixture{Name: "ProCard", Build: func(c *Context) { ProCard(c, "卡片").Body(func() { Text(c, "内容") }) }},
		perfFixture{Name: "ProDescriptions", Build: func(c *Context) {
			ProDescriptions(c, []ProDescItem{{Label: "名称", Render: func(c *Context, v any, all ProValues) { Text(c, "Tango") }}}).Show()
		}},
		perfFixture{Name: "ProForm", Build: func(c *Context) { ProForm(c, perfProFields()).Show() }},
		perfFixture{Name: "ProFormFieldSet", Build: func(c *Context) { _ = ProFormFieldSet(c, ProFieldSet{}); Button(c, "字段") }},
		perfFixture{Name: "ProFormGroup", Build: func(c *Context) { ProFormGroup(c, "分组").Body(func() { Button(c, "字段") }) }},
		perfFixture{Name: "ProFormItems", Build: func(c *Context) { f := Form(c); ProFormItems(c, f, perfProFields(), ProValues{}); Button(c, "表单") }},
		perfFixture{Name: "ProFieldOf", Build: func(c *Context) { v := any("值"); ProFieldOf(c, perfProFields()[0], &v) }},
		perfFixture{Name: "ProSkeleton", Build: func(c *Context) { ProSkeleton(c, "list") }},
		perfFixture{Name: "QueryFilter", Build: func(c *Context) { QueryFilter(c, perfProFields()).Show() }},
		perfFixture{Name: "LightFilter", Build: func(c *Context) { LightFilter(c, perfProFields()).Show() }},
		perfFixture{Name: "SchemaForm", Build: func(c *Context) { SchemaForm(c, perfProFields()).Show() }},
		perfFixture{Name: "StepsForm", Build: func(c *Context) { StepsForm(c, nil).Show() }},
		perfFixture{Name: "ModalForm", Overlay: true, Build: func(c *Context) {
			o := State(c, hashID("modalform"), func() bool { return true })
			if Button(c, "打开表单").Clicked() {
				*o = true
			}
			ModalForm(c, o, "表单", perfProFields()).Show()
		}},
		perfFixture{Name: "DrawerForm", Overlay: true, Build: func(c *Context) {
			o := State(c, hashID("drawerform"), func() bool { return true })
			if Button(c, "打开抽屉").Clicked() {
				*o = true
			}
			DrawerForm(c, o, "表单", perfProFields()).Show()
		}},
		perfFixture{Name: "LoginForm", Build: func(c *Context) { LoginForm(c).Show() }},
		perfFixture{Name: "LoginFormPage", Build: func(c *Context) { LoginFormPage(c).Show() }},
		perfFixture{Name: "CheckCard", Build: func(c *Context) {
			v := State(c, hashID("check"), func() bool { return false })
			CheckCard(c, "选项", v).Show()
		}},
		perfFixture{Name: "CheckCardGroup", Build: func(c *Context) { CheckCardGroup(c, nil).Show() }},
		perfFixture{Name: "StatisticCard", Build: func(c *Context) { StatisticCard(c, "统计").Show() }},
		perfFixture{Name: "StatisticCardGroup", Build: func(c *Context) { StatisticCardGroup(c).Body(func() { Text(c, "统计") }) }},
		perfFixture{Name: "Dock", Build: func(c *Context) { Dock(c, &DockLayout{}).Show() }},
		perfFixture{Name: "StatusBar", Build: func(c *Context) { StatusBar(c).Left(func() { StatusBarItem(c, "main") }) }},
		perfFixture{Name: "StatusBarItem", Build: func(c *Context) { StatusBarItem(c, "main") }},
		perfFixture{Name: "TitleBar", Build: func(c *Context) { TitleBar(c).Title("Tango") }},
		perfFixture{Name: "WindowControls", Build: func(c *Context) { WindowControls(c) }},
		perfFixture{Name: "NativeView", Build: func(c *Context) { NativeView(c, "native", layout.Size(320, 120)) }},
		perfFixture{Name: "Bubble", Build: func(c *Context) { Bubble(c, ChatAssistant, "你好").Actions(BubbleCopy) }},
		perfFixture{Name: "BubbleList", Build: func(c *Context) {
			BubbleList(c, 5, func(i int) string { return strconv.Itoa(i) }, func(i int) { Bubble(c, ChatAssistant, "消息") }, layout.Size(480, 320))
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				Scroll(c, func() {
					BubbleList(c, n, func(i int) string { return strconv.Itoa(i) }, func(i int) { Bubble(c, ChatAssistant, perfCollItem(i)) }, layout.Size(480, 760))
				}, layout.Size(520, 300))
			}
		}},
		perfFixture{Name: "Conversations", Build: func(c *Context) {
			a := State(c, hashID("conv"), func() string { return "1" })
			Conversations(c, []Conversation{{Key: "1", Title: "会话一"}, {Key: "2", Title: "会话二"}}, a)
		}, Large: func(n int) func(*Context) {
			return func(c *Context) {
				a := State(c, hashID("conv.large"), func() string { return "0" })
				xs := make([]Conversation, n)
				for i := range xs {
					xs[i] = Conversation{Key: strconv.Itoa(i), Title: perfCollItem(i)}
				}
				Scroll(c, func() { Conversations(c, xs, a) }, layout.Size(520, 300))
			}
		}},
		perfFixture{Name: "Sender", Build: func(c *Context) {
			v := State(c, hashID("sender"), func() string { return "" })
			Sender(c, v).Placeholder("输入")
		}},
		perfFixture{Name: "Prompts", Build: func(c *Context) { Prompts(c, []PromptItem{{Key: "a", Label: "建议"}}) }},
		perfFixture{Name: "ThoughtChain", Build: func(c *Context) {
			ThoughtChain(c, []ChainItem{{Title: "思考", Description: "内容", Detail: "详细步骤"}}).Title("思考")
		}},
		perfFixture{Name: "ToolCall", Build: func(c *Context) { ToolCall(c, "工具").Summary("展开工具").Args("参数").Result("结果") }},
		perfFixture{Name: "Approval", Build: func(c *Context) {
			Approval(c, "需要批准").Description("确认操作").Choices(ApproveOnce, ApproveDeny)
		}},
	)
}

func perfProText(r *perfRun, text string) {
	if box, ok := r.D.FindText(text); ok {
		r.D.Click(box.Center())
	} else {
		r.D.Click(r.Center())
	}
}
func init() {
	for _, name := range []string{"ThoughtChain"} {
		f := perfRegistry[name]
		f.Interact = func(r *perfRun, i int) {
			r.D.Click(r.Center())
			r.D.Move(perfOutside)
		}
		perfRegistry[name] = f
	}
	f := perfRegistry["Sender"]
	f.Interact = func(r *perfRun, i int) {
		r.D.Click(r.Center())
		if i%2 == 0 {
			r.D.Type("hello")
		} else {
			for range 5 {
				r.D.Key(input.KeyBackspace, 0)
			}
			r.D.Click(perfOutside)
		}
	}
	perfRegistry["Sender"] = f
	_ = perfRegistry["Approval"]

}

func TestPerfProOverlayOpenCloseReopen(t *testing.T) {
	tests := []struct {
		name, initial, trigger string
	}{
		{"SettingDrawer", "整体风格", "打开页面设置"},
		{"ModalForm", "名称", "打开表单"},
		{"DrawerForm", "名称", "打开抽屉"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := perfRegistry[tc.name]
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			if _, ok := d.FindText(tc.initial); !ok {
				t.Fatalf("%s identifying content is not visible initially", tc.name)
			}
			d.Click(perfOutside)
			d.Settle()
			if d.NeedsFrame() {
				t.Fatal("overlay close did not settle")
			}
			if _, ok := d.FindText(tc.initial); ok {
				t.Fatalf("%s identifying content remained visible after close", tc.name)
			}
			if err := d.ClickLabel(tc.trigger); err != nil {
				t.Fatalf("reopen %s: %v", tc.name, err)
			}
			d.Settle()
			if _, ok := d.FindText(tc.initial); !ok {
				t.Fatalf("%s identifying content did not return after reopen", tc.name)
			}
		})
	}
}

func TestPerfProLargeRenderedAndScrolled(t *testing.T) {
	const n = 100000
	for _, name := range []string{"ProTable", "ProList", "BubbleList", "Conversations"} {
		t.Run(name, func(t *testing.T) {
			fx := perfRegistry[name]
			d := newPerfDriver(perfPage(fx.Large(n)), false)
			d.Settle()
			first := perfCollItem(0)
			if name == "ProTable" {
				first = "项目 1"
			}
			firstRect, ok := d.FindText(first)
			if !ok {
				t.Fatalf("%s first item not rendered", name)
			}
			if perfCollExtent(d, false) < float64(n*16) {
				t.Fatalf("large %s extent is too short", name)
			}
			before := perfCollScrollOffsets(d)
			d.Inject(input.Scroll{Pos: firstRect.Center(), DY: 240})
			d.Settle()
			down := perfCollScrollOffsets(d)
			downDY := perfCollScrollDY(before, down)
			if downDY == 0 {
				t.Fatalf("%s wheel did not move the retained viewport", name)
			}
			if name != "BubbleList" {
				viewport := perfTarget(d)
				d.Inject(input.Scroll{Pos: viewport.Center(), DY: -240})
				d.Settle()
				up := perfCollScrollOffsets(d)
				upDY := perfCollScrollDY(down, up)
				if upDY == 0 || downDY*upDY >= 0 {
					t.Fatalf("%s reverse wheel did not move the retained viewport in the opposite direction (down %.0f, up %.0f)", name, downDY, upDY)
				}
			}
		})
	}
}
