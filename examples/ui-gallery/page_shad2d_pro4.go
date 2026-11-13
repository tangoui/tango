package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Pages of the fourth Pro batch (SettingDrawer, ProFormGroup,
// RouteContext / GridContent, App, ProTable params / polling / columns
// state / alert render) and Ant dark variants of the pro2 pages.

var shad2dDemo struct {
	settings    ui.ProSettings
	drawerOpen  bool
	sel         string
	collapsed   bool
	groupFolded bool
	vals        ui.ProValues
	status      string
	ptSel       []string
	cols        ui.TableColumnState
	store       map[string]ui.TableColumnState
	changes     int
	appClicks   int
}

func shad2dReset() {
	d := &shad2dDemo
	d.settings = ui.DefaultProSettings()
	d.settings.NavTheme = "light"
	d.drawerOpen = true
	d.sel = "analysis"
	d.collapsed = false
	d.groupFolded = false
	d.vals = ui.ProValues{"name": "Tango", "phone": "13800000000"}
	d.status = ""
	d.ptSel = []string{"rule-1", "rule-4"}
	d.cols = ui.TableColumnState{}
	d.store = map[string]ui.TableColumnState{}
	d.changes = 0
	d.appClicks = 0
}

func shad2dMenu() []ui.MenuItem {
	return []ui.MenuItem{
		{Key: "dashboard", Label: "仪表盘", Icon: icon.Dashboard, Children: []ui.MenuItem{
			{Key: "analysis", Label: "分析页"}, {Key: "monitor", Label: "监控页"}}},
		{Key: "form", Label: "表单页", Icon: icon.Form},
		{Key: "list", Label: "列表页", Icon: icon.Table},
		{Key: "profile", Label: "详情页", Icon: icon.Profile},
	}
}

func shad2dSettingPage(c *ui.Context) {
	s := &shad2dDemo
	gallery.Demo(c, "页面设置抽屉", "SettingDrawer 编辑 ProSettings：整体风格、主题色、导航模式、内容宽度、固定顶栏、色弱与紧凑模式；ProSettingsProvider 与 Apply 让改动立即作用到 ProLayout。", func() {
		ui.Column(c, func() {
			ui.ProSettingsProvider(c, &s.settings, func() {
				cfg := ui.ProLayoutConfig{Title: "Tango Pro", Menu: shad2dMenu(), Selected: &s.sel, Collapsed: &s.collapsed,
					Breadcrumb: []string{"首页", "仪表盘", "分析页"}, UserName: "曲丽丽"}
				s.settings.Apply(&cfg)
				ui.ProLayout(c, cfg, func() {
					ui.ProCard(c, "当前设置").Body(func() {
						st := s.settings
						ui.Text(c, fmt.Sprintf("layout=%s  navTheme=%s  contentWidth=%s  fixedHeader=%v  colorWeak=%v", st.Layout, st.NavTheme, st.ContentWidth, st.FixedHeader, st.ColorWeak))
					})
				})
				ui.SettingDrawer(c, &s.settings).Open(&s.drawerOpen).Show()
			})
		}, layout.H(560)).Border(1, c.Theme.BorderSecondary).Clip()
	})
}

func shad2dGroupPage(c *ui.Context) {
	s := &shad2dDemo
	gallery.Demo(c, "按 Group 分组", "ProField.Group 相同的连续字段由 ProForm 自动包进带标题的 ProFormGroup。", func() {
		fields := []ui.ProField{
			{Key: "name", Label: "姓名", Group: "基本信息", Required: true},
			{Key: "phone", Label: "手机号", Group: "基本信息"},
			{Key: "city", Label: "城市", Group: "地址", ValueType: ui.ValueSelect, ValueEnum: []ui.ProEnum{{Value: "bj", Label: "北京"}, {Value: "sh", Label: "上海"}, {Value: "sz", Label: "深圳"}}},
			{Key: "street", Label: "街道", Group: "地址"},
		}
		ui.ProForm(c, fields).Values(&s.vals).Columns(2).Show()
	})
	gallery.Demo(c, "可折叠与栅格", "ProFormGroup 手动使用：Collapsible、Columns(3)、Extra；ProFormItems 构建字段。", func() {
		fields := []ui.ProField{{Key: "a", Label: "开户行"}, {Key: "b", Label: "账号"}, {Key: "c", Label: "户名"}}
		ui.ProForm(c, nil).Values(&s.vals).NoSubmitter().Children(func(f *ui.FormW) {
			ui.ProFormGroup(c, "结算账户").Description("用于每月结算打款").Collapsed(&s.groupFolded).Columns(3).
				Extra(func() { ui.Button(c, "校验").Small() }).
				Body(func() { ui.ProFormItems(c, f, fields, s.vals) })
		}).Show()
	})
}

func shad2dGridPage(c *ui.Context) {
	s := &shad2dDemo
	gallery.Demo(c, "定宽内容与路由上下文", "ContentWidth: \"Fixed\" 时内容区在 1200px 内居中（GridContent）；RouteContext 读出外层 ProLayout 的状态。", func() {
		ui.Column(c, func() {
			cfg := ui.ProLayoutConfig{Title: "Tango Pro", Menu: shad2dMenu(), Selected: &s.sel, Layout: "top", ContentWidth: "Fixed", UserName: "曲丽丽"}
			ui.ProLayout(c, cfg, func() {
				rc := ui.RouteContext(c)
				ui.ProCard(c, "RouteContext").Body(func() {
					ui.Text(c, fmt.Sprintf("inLayout=%v layout=%s contentWidth=%s selected=%s fixedHeader=%v", rc.InLayout, rc.Layout, rc.ContentWidth, rc.Selected, rc.FixedHeader))
				})
			})
		}, layout.H(320)).Border(1, c.Theme.BorderSecondary).Clip()
	})
	gallery.Demo(c, "GridContent 单独使用", "Fluid 撑满，Fixed 以 MaxWidth 居中。", func() {
		ui.GridContent(c).ContentWidth("Fixed").MaxWidth(480).Body(func() {
			ui.ProCard(c, "Fixed 480px").Bordered().Body(func() { ui.Text(c, "居中的内容") })
		})
	})
}

func shad2dAppPage(c *ui.Context) {
	s := &shad2dDemo
	gallery.Demo(c, "App 统一配置", "App 把 message / notification 配置下发给子树；UseApp 返回的句柄按配置打开（含 MaxCount）。", func() {
		ui.App(c).Message(ui.AppMessageConfig{Duration: 10 * time.Second, MaxCount: 2}).
			Notification(ui.AppNotificationConfig{Placement: ui.NotifyBottomRight, PlacementSet: true}).
			Body(func() {
				app := ui.UseApp(c)
				ui.Row(c, func() {
					if ui.Button(c, "打开消息").Primary().Clicked() {
						s.appClicks++
						app.Message("success", fmt.Sprintf("第 %d 条消息（最多同时 2 条）", s.appClicks))
					}
					if ui.Button(c, "打开通知").Clicked() {
						app.Notification("info", "来自 App 的通知").Description("位置由 App 配置为右下角")
					}
					ui.Text(c, fmt.Sprintf("InApp=%v", app.InApp())).Tertiary()
				}, layout.Gap(8), layout.Middle())
			})
	})
}

type shad2dStore map[string]ui.TableColumnState

func (m shad2dStore) LoadColumnsState(k string) (ui.TableColumnState, bool) {
	s, ok := m[k]
	return s, ok
}
func (m shad2dStore) SaveColumnsState(k string, s ui.TableColumnState) { m[k] = s }

func shad2dTablePage(c *ui.Context) {
	s := &shad2dDemo
	all := tbRules(37)
	enum := []ui.ProEnum{{Value: "running", Label: "运行中", Status: "processing"}, {Value: "online", Label: "已上线", Status: "success"},
		{Value: "closed", Label: "关闭", Status: "default"}, {Value: "error", Label: "异常", Status: "error"}}
	cols := []ui.ProColumn[tbRule]{
		{Key: "name", Title: "规则名称", Width: 140, Value: func(r tbRule) any { return r.name }},
		{Key: "owner", Title: "负责人", Width: 100, Value: func(r tbRule) any { return r.owner }},
		{Key: "status", Title: "状态", Width: 110, ValueType: ui.ValueStatus, ValueEnum: enum, HideInSearch: true, Value: func(r tbRule) any { return r.status }},
		{Key: "calls", Title: "调用次数", Width: 116, ValueType: ui.ValueDigit, HideInSearch: true, Value: func(r tbRule) any { return r.calls }},
		{Key: "updated", Title: "更新时间", ValueType: ui.ValueDateTime, HideInSearch: true, Value: func(r tbRule) any { return r.updated }},
	}
	gallery.Demo(c, "外部参数、轮询、列状态与批量操作栏", "Params 变化自动重新请求；Polling 每 5 秒刷新（可暂停）；ColumnsState 受控并经 PersistColumnsState 存到应用提供的存储；TableAlertRender / TableAlertOptionRender 自定义批量操作栏。", func() {
		ui.Row(c, func() {
			ui.Segmented(c, &s.status, []ui.Option{{Label: "全部", Value: ""}, {Label: "运行中", Value: "running"}, {Label: "异常", Value: "error"}})
			ui.Text(c, fmt.Sprintf("列状态变更 %d 次", s.changes)).Tertiary()
		}, layout.Gap(16), layout.Middle(), layout.Margin(geom.Insets{Bottom: 12}))
		var pt *ui.ProTableW[tbRule]
		pt = ui.ProTable(c, cols).Title("规则列表").NoSearch().
			RowKey(func(r tbRule) string { return r.key }).
			Selection(&s.ptSel).
			Params(map[string]string{"status": s.status}).
			Polling(5*time.Second).
			ColumnsState(&s.cols).
			OnColumnsStateChange(func(ui.TableColumnState) { s.changes++ }).
			PersistColumnsState("rules", shad2dStore(s.store)).
			TableAlertRender(func(keys []string, clear func()) {
				ui.Text(c, "已选 "+strings.Join(keys, "、"))
			}).
			TableAlertOptionRender(func(keys []string, clear func()) {
				ui.Button(c, "批量删除").Small().Danger()
				if ui.Button(c, "清空").Small().Clicked() {
					clear()
				}
			}).
			ToolBar(func() {
				label := "暂停轮询"
				if pt != nil && pt.PollingPaused() {
					label = "恢复轮询"
				}
				if ui.Button(c, label).Clicked() && pt != nil {
					if pt.PollingPaused() {
						pt.ResumePolling()
					} else {
						pt.PausePolling()
					}
				}
			}).
			Request(func(req ui.ProRequest, done func(ui.ProResult[tbRule])) {
				rows, total, err := tbRuleServer(all, req)
				done(ui.ProResult[tbRule]{Rows: rows, Total: total, Err: err})
			})
		pt.Show()
	})
}

func init() {
	shad2dReset()
	pro2Page(gallery.Page{Name: "SettingDrawer", Title: "SettingDrawer 页面设置", Group: gallery.Layout, Order: 42, Width: 1200, Render: shad2dSettingPage}, shad2dReset)
	pro2Page(gallery.Page{Name: "GridContent", Title: "RouteContext / GridContent 路由上下文与内容容器", Group: gallery.Layout, Order: 43, Width: 1200, Render: shad2dGridPage}, shad2dReset)
	pro2Page(gallery.Page{Name: "ProFormGroup", Title: "ProFormGroup 表单分组", Group: gallery.Form, Order: 39, Width: 1080, Render: shad2dGroupPage}, shad2dReset)
	pro2Page(gallery.Page{Name: "App", Title: "App 应用容器", Group: gallery.General, Order: 90, Width: 960, Render: shad2dAppPage,
		Setup: func(d *ui.Driver) {
			for i := 0; i < 3; i++ {
				d.ClickLabel("打开消息")
				d.Settle()
			}
		}}, shad2dReset)
	pro2Page(gallery.Page{Name: "ProTableExtras", Title: "ProTable 外部参数 / 轮询 / 列状态 / 批量操作", Group: gallery.Display, Order: 110, Width: 1100, Parent: "ProTable", Render: shad2dTablePage}, shad2dReset)

	// Ant dark variants of the pro2 pages and of the pages above.
	for _, name := range []string{"CheckCard", "FooterToolbar", "PageContainer", "ProCard", "ProSkeleton", "StatisticCard",
		"SettingDrawer", "GridContent", "ProFormGroup", "App", "ProTableExtras"} {
		shad2dDarkVariant(name)
	}
}

// shad2dDarkVariant registers Name-Dark (Ant dark algorithm) for a page.
func shad2dDarkVariant(name string) {
	for _, p := range gallery.Pages() {
		if p.Name != name || p.Variant {
			continue
		}
		dk := p
		dk.Variant = true
		dk.Name, dk.Title = p.Name+"-Dark", p.Title+"（暗色）"
		setup := p.Setup
		dk.Setup = func(d *ui.Driver) {
			d.SetTheme(theme.AntDesign(theme.Dark()))
			d.Settle()
			if setup != nil {
				setup(d)
			}
		}
		dk.Interact = nil
		gallery.Register(dk)
		return
	}
}
