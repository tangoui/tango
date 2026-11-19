package ui

import (
	"github.com/tangoui/tango/ui/geom"
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
		}, LargeVirtual: func(n int) func(*Context) {
			// The same page in the table's own 300px body (ScrollY +
			// Virtual); the rows are made once.
			return func(c *Context) {
				rows := State(c, hashID("protable.virtual", n), func() []int { return perfProRows(n) })
				Column(c, func() {
					ProTable(c, perfProCols()).PageSize(n).ScrollY(300).Virtual().
						Request(func(req ProRequest, done func(ProResult[int])) { done(ProResult[int]{Rows: *rows, Total: n}) }).Show()
				}, layout.W(640))
			}
		}, NA: map[string]string{"Large100k": perfNANonVirtual}},
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
		}, LargeVirtual: func(n int) func(*Context) {
			// The rows are made once: the timed steps scroll the list.
			return func(c *Context) {
				rows := State(c, hashID("prolist.virtual", n), func() []int { return perfProRows(n) })
				Column(c, func() {
					ProList[int](c, *rows, ProListMetas[int]{Title: func(i int) string { return perfCollItem(i) }}).Height(300).Virtual().Show()
				}, layout.W(640))
			}
		}, NA: map[string]string{"Large100k": perfNANonVirtual}},
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
		}, LargeVirtual: func(n int) func(*Context) {
			// The conversations are made once: the timed steps scroll the list.
			return func(c *Context) {
				a := State(c, hashID("conv.virtual"), func() string { return "0" })
				xs := State(c, hashID("conv.virtual", n), func() []Conversation {
					xs := make([]Conversation, n)
					for i := range xs {
						xs[i] = Conversation{Key: strconv.Itoa(i), Title: perfCollItem(i)}
					}
					return xs
				})
				Column(c, func() { Conversations(c, *xs, a).Height(300).Virtual() }, layout.W(520))
			}
		}, NA: map[string]string{"Large100k": perfNANonVirtual}},
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
	for _, name := range []string{"ProTable", "ProList", "BubbleList", "Conversations", "CellEditorTable", "RowEditorTable"} {
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

// Post-merge catch-up (task M1): Pro entries added upstream.

// perfProEditRow is a row of the CellEditorTable / RowEditorTable
// fixtures: the ID column is read only, name and note are editable.
type perfProEditRow struct{ ID, Name, Note string }

func perfProEditRows(n int, name func(int) string) []perfProEditRow {
	out := make([]perfProEditRow, n)
	for i := range out {
		out[i] = perfProEditRow{ID: strconv.Itoa(i + 1), Name: name(i), Note: "备注 " + strconv.Itoa(i+1)}
	}
	return out
}

func perfProEditName(i int) string { return "项目 " + strconv.Itoa(i+1) }

func perfProEditCols() []ProColumn[perfProEditRow] {
	return []ProColumn[perfProEditRow]{
		{Key: "id", Title: "编号", Width: 80, Value: func(r perfProEditRow) any { return r.ID }},
		{Key: "name", Title: "名称", Width: 220, Value: func(r perfProEditRow) any { return r.Name },
			Editor: &ProField{Required: true}, Set: func(r *perfProEditRow, v any) { r.Name, _ = v.(string) }},
		{Key: "note", Title: "备注", Width: 220, Value: func(r perfProEditRow) any { return r.Note },
			Editor: &ProField{}, Set: func(r *perfProEditRow, v any) { r.Note, _ = v.(string) }},
	}
}

// perfProDoubleClick double-clicks p (the editor tables start editing on
// a double click).
func perfProDoubleClick(d *Driver, p geom.Point) {
	d.Inject(input.Pointer{Kind: input.Move, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p, Clicks: 1})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p})
	d.Inject(input.Pointer{Kind: input.Press, Pos: p, Clicks: 2})
	d.Inject(input.Pointer{Kind: input.Release, Pos: p})
	d.Settle()
}

// perfProEdit is the editor tables' interaction: even steps double-click
// the name cell of row 2, append "X" and save with Enter; odd steps edit
// it back (Backspace, Enter). A click on the page padding then blurs the
// table so the settled screen has no focus ring.
func perfProEdit(r *perfRun, i int) {
	label, typed := "项目 2", "X"
	if i%2 == 1 {
		label, typed = "项目 2X", ""
	}
	box, ok := r.D.FindText(label)
	if !ok {
		panic("perf: editor table cell not found: " + label)
	}
	perfProDoubleClick(r.D, box.Center())
	r.D.Key(input.KeyEnd, 0)
	if typed != "" {
		r.D.Type(typed)
	} else {
		r.D.Key(input.KeyBackspace, 0)
	}
	r.D.Key(input.KeyEnter, 0)
	r.D.Click(perfOutside)
}

// perfProEditLarge is a page with n editable rows in a bounded scroll
// region (the editor tables' default, non-virtual form: every row is
// built).
func perfProEditLarge(id string, show func(c *Context, rows *[]perfProEditRow)) func(n int) func(*Context) {
	return func(n int) func(*Context) {
		return func(c *Context) {
			rows := State(c, hashID(id), func() []perfProEditRow { return perfProEditRows(n, perfCollItem) })
			Scroll(c, func() { show(c, rows) }, layout.Size(640, 300))
		}
	}
}

// perfProEditLargeVirtual is perfProEditLarge in the table's own 300px
// body: show gets the rows and adds ScrollY + Virtual.
func perfProEditLargeVirtual(id string, show func(c *Context, rows *[]perfProEditRow)) func(n int) func(*Context) {
	return func(n int) func(*Context) {
		return func(c *Context) {
			rows := State(c, hashID(id, n), func() []perfProEditRow { return perfProEditRows(n, perfCollItem) })
			Column(c, func() { show(c, rows) }, layout.W(640))
		}
	}
}

func perfProListToolBarMenu() []TabItem {
	return []TabItem{{Key: "all", Label: "全部项目"}, {Key: "doing", Label: "进行中"}, {Key: "done", Label: "已完成"}}
}

func init() {
	cellShow := func(c *Context, rows *[]perfProEditRow) {
		CellEditorTable(c, perfProEditCols(), rows).RowKey(func(r perfProEditRow) string { return r.ID }).Show()
	}
	rowShow := func(c *Context, rows *[]perfProEditRow) {
		RowEditorTable(c, perfProEditCols(), rows).RowKey(func(r perfProEditRow) string { return r.ID }).Show()
	}
	cellShowVirtual := func(c *Context, rows *[]perfProEditRow) {
		CellEditorTable(c, perfProEditCols(), rows).RowKey(func(r perfProEditRow) string { return r.ID }).ScrollY(300).Virtual().Show()
	}
	rowShowVirtual := func(c *Context, rows *[]perfProEditRow) {
		RowEditorTable(c, perfProEditCols(), rows).RowKey(func(r perfProEditRow) string { return r.ID }).ScrollY(300).Virtual().Show()
	}
	registerPerf(
		perfFixture{
			Name: "CellEditorTable",
			Build: func(c *Context) {
				cellShow(c, State(c, hashID("perf.pro.celledit"), func() []perfProEditRow { return perfProEditRows(5, perfProEditName) }))
			},
			Interact:     perfProEdit,
			Large:        perfProEditLarge("perf.pro.celledit.large", cellShow),
			LargeVirtual: perfProEditLargeVirtual("perf.pro.celledit.virtual", cellShowVirtual),
			NA:           map[string]string{"Large100k": perfNANonVirtual},
		},
		perfFixture{
			Name: "RowEditorTable",
			Build: func(c *Context) {
				rowShow(c, State(c, hashID("perf.pro.rowedit"), func() []perfProEditRow { return perfProEditRows(5, perfProEditName) }))
			},
			Interact:     perfProEdit,
			Large:        perfProEditLarge("perf.pro.rowedit.large", rowShow),
			LargeVirtual: perfProEditLargeVirtual("perf.pro.rowedit.virtual", rowShowVirtual),
			NA:           map[string]string{"Large100k": perfNANonVirtual},
		},
		perfFixture{
			// The toolbar's header menu (Pro ListToolBar menu type "tab")
			// switches the list view; search and actions are built too.
			// It wraps no list, so it has no Large.
			Name: "ListToolBar",
			Build: func(c *Context) {
				active := State(c, hashID("perf.pro.listtoolbar.menu"), func() string { return "all" })
				search := State(c, hashID("perf.pro.listtoolbar.search"), func() string { return "" })
				ListToolBar(c).Menu("tab", active, perfProListToolBarMenu()).Search(search).
					Actions(func() { Button(c, "新建").Primary() }).Show()
			},
			Interact: perfNavSwitch("进行中", "全部项目"),
		},
		// PageLoading, ProStatistic, StatisticCardDivider and
		// StatisticCardOperation are non-interactive visual helpers:
		// build-only (idle, damage-free first frame and Mount timing).
		perfFixture{Name: "PageLoading", Build: func(c *Context) { PageLoading(c).Description("页面加载中").Show() }},
		perfFixture{Name: "ProStatistic", Build: func(c *Context) {
			ProStatistic(c, "今日 UV").Value(1328, 0).Trend("up").Description(func() { Text(c, "较昨日 +12%") }).Show()
		}},
		perfFixture{Name: "StatisticCardDivider", Build: func(c *Context) {
			StatisticCardGroup(c).Body(func() {
				Row(c, func() {
					StatisticCard(c, "总流量").Value(79, 0).Show()
					StatisticCardDivider(c, "")
					StatisticCard(c, "付费流量").Value(112, 0).Show()
				})
			})
		}},
		perfFixture{Name: "StatisticCardOperation", Build: func(c *Context) {
			Row(c, func() {
				ProStatistic(c, "服务网格数").Value(500, 0).StatisticLayout(StatisticVertical).Show()
				StatisticCardOperation(c, "=")
				ProStatistic(c, "未发布").Value(234, 0).StatisticLayout(StatisticVertical).Show()
				StatisticCardOperation(c, "+")
				ProStatistic(c, "已发布").Value(266, 0).StatisticLayout(StatisticVertical).Show()
			}, layout.Gap(16))
		}},
	)
}

// TestPerfProInteractOutcome checks the component-specific result of the
// Pro interactions at rest, after step 0 and after step 1.
func TestPerfProInteractOutcome(t *testing.T) {
	name := func(id string) func(*Driver) string {
		return func(d *Driver) string {
			rows := *State[[]perfProEditRow](d.r.c, hashID(id), nil)
			return rows[1].Name + "," + strconv.Itoa(len(perfProEditors(d)))
		}
	}
	tests := []struct {
		name  string
		probe func(*Driver) string
		want  [3]string
	}{
		{"CellEditorTable", name("perf.pro.celledit"), [3]string{"项目 2,0", "项目 2X,0", "项目 2,0"}},
		{"RowEditorTable", name("perf.pro.rowedit"), [3]string{"项目 2,0", "项目 2X,0", "项目 2,0"}},
		{"ListToolBar", func(d *Driver) string {
			return *State[string](d.r.c, hashID("perf.pro.listtoolbar.menu"), nil)
		}, [3]string{"all", "doing", "all"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fx := perfRegistry[tc.name]
			d := newPerfDriver(perfPage(fx.Build), false)
			d.Settle()
			r := &perfRun{D: d}
			for i, want := range tc.want {
				if i > 0 {
					perfStep(r, fx.Interact, i-1)
				}
				if got := tc.probe(d); got != want {
					t.Fatalf("after %d steps: got %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestPerfProEditorMode shows the editor tables' own modes on the
// fixtures' double click: CellEditorTable opens one editor (the cell),
// RowEditorTable opens one per editable column of the row (name, note).
func TestPerfProEditorMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		want int
	}{{"CellEditorTable", 1}, {"RowEditorTable", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			d := newPerfDriver(perfPage(perfRegistry[tc.name].Build), false)
			d.Settle()
			box, ok := d.FindText("项目 2")
			if !ok {
				t.Fatal("cell not found")
			}
			perfProDoubleClick(d, box.Center())
			if got := len(perfProEditors(d)); got != tc.want {
				t.Fatalf("%d editors open, want %d", got, tc.want)
			}
		})
	}
}

// perfProEditors are the text editors shown (pro3TextEditors: the
// editor tables' fixtures have no other text input).
func perfProEditors(d *Driver) []geom.Rect { return pro3TextEditors(d) }
