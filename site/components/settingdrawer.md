---
title: SettingDrawer 页面设置
---
# SettingDrawer 页面设置

提供 Pro 风格的页面设置抽屉，集中编辑导航主题、主题色、布局、内容宽度、固定区域、色弱和紧凑模式。

## 何时使用

- 后台应用需要让用户即时切换整体布局和主题偏好时使用。
- 需要把设置同时应用到 `ProLayout` 和主题 Provider 时，用 `ProSettingsProvider` 与 `ProSettings.Apply` 配合。
- 产品不允许终端用户调整外壳时，不应显示 SettingDrawer；直接固定 `ProLayoutConfig` 和主题即可。
- 只需要编辑业务字段的抽屉使用 [Drawer 抽屉](/components/drawer)，不要把 SettingDrawer 当作通用表单抽屉。

## 在线体验

gallery 的 SettingDrawer 场景把设置抽屉接入 ProLayout，可实时切换导航主题、主题色、side / top / mix 布局、内容宽度、固定顶栏与侧栏、色弱和紧凑模式。

<GalleryPreview component="SettingDrawer" />

## 基本用法

`ProSettings` 保存设置；`ProSettingsProvider` 把主题相关字段作用于子树，`Apply` 把布局字段复制到 `ProLayoutConfig`，最后调用 `SettingDrawer.Show` 构建浮动入口和抽屉。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var (
    settingState    = ui.DefaultProSettings()
    settingSelected = "home"
    settingCollapsed bool
)

var settingMenu = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "settings", Label: "设置", Icon: icon.Setting},
}

func settingDrawerPage(c *ui.Context) {
    ui.ProSettingsProvider(c, &settingState, func() {
        cfg := ui.ProLayoutConfig{
            Title: "TanGo Admin", Menu: settingMenu,
            Selected: &settingSelected, Collapsed: &settingCollapsed,
        }
        settingState.Apply(&cfg)
        ui.ProLayout(c, cfg, func() {
            ui.Text(c, "页面内容")
        })
        ui.SettingDrawer(c, &settingState).Show()
    })
}
```

默认会显示右下角的设置浮动按钮；点击后打开右侧 320px 抽屉。

## 常见用法

### 绑定外部打开状态

`Open(&open)` 将抽屉的打开状态交给页面变量；配合 `NoTrigger` 可隐藏默认浮动按钮，从顶栏或其他入口打开。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var (
    settingOpen  bool
    settingPrefs = ui.DefaultProSettings()
)

func settingExternalTriggerPage(c *ui.Context) {
    if ui.Button(c, "页面设置").Icon(icon.Setting).Clicked() {
        settingOpen = true
    }
    drawer := ui.SettingDrawer(c, &settingPrefs).Open(&settingOpen).NoTrigger()
    if drawer.Show().Changed() {
        ui.Message(c, "success", "设置已更新")
    }
}
```

### 应用到 ProLayout

每帧创建配置并调用 `Apply`，设置变更会在下一帧作用到布局；`ProSettingsProvider` 应放在窗口或页面根部，确保主题覆盖整个外壳。

```go
import "github.com/tangoui/tango/ui"

var (
    appliedSettings = ui.DefaultProSettings()
    appliedSelected = "dashboard"
)

func settingApplyPage(c *ui.Context) {
    ui.ProSettingsProvider(c, &appliedSettings, func() {
        cfg := ui.ProLayoutConfig{
            Title: "TanGo Pro", Selected: &appliedSelected,
            Menu: []ui.MenuItem{{Key: "dashboard", Label: "仪表盘"}},
        }
        appliedSettings.Apply(&cfg)
        ui.ProLayout(c, cfg, func() {
            ui.Text(c, "layout="+appliedSettings.Layout+" width="+appliedSettings.ContentWidth)
        })
        ui.SettingDrawer(c, &appliedSettings).Show()
    })
}
```

### 自定义主题色

`Colors` 替换抽屉中的预设色板。颜色值使用 `theme.Hex` 或当前主题颜色；设置本身仍由 `ProSettings.PrimaryColor` 保存。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var customColorSettings = ui.DefaultProSettings()

func settingColorsPage(c *ui.Context) {
    ui.ProSettingsProvider(c, &customColorSettings, func() {
        ui.Text(c, "自定义主题色板")
        ui.SettingDrawer(c, &customColorSettings).Colors([]theme.Color{
            theme.Hex("#1677ff"),
            theme.Hex("#13c2c2"),
            theme.Hex("#722ed1"),
        }).Show()
    })
}
```

### 隐藏设置分区

可以通过 `ConfigProvider.Defaults("SettingDrawer", ...)` 隐藏颜色、布局或其他设置分区。适合产品只开放部分偏好的场景。

```go
import "github.com/tangoui/tango/ui"

var limitedSettings = ui.DefaultProSettings()

func settingSectionsPage(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("SettingDrawer", ui.Props{
        "hideColors": true,
        "hideOther":  true,
    }).Body(func() {
        ui.SettingDrawer(c, &limitedSettings).Show()
    })
}
```

支持的键是 `hideColors`、`hideLayout` 和 `hideOther`，值为 bool；单个 SettingDrawer 没有对应的链式隐藏方法。

### 持久化变更

`Changed()` 只报告当前帧是否有设置被修改；要跨窗口或跨会话保存，应在变更时把 `ProSettings` 序列化到应用自己的存储，并在页面启动时恢复字段。

```go
import "github.com/tangoui/tango/ui"

var persistedSettings = ui.DefaultProSettings()

func settingChangePage(c *ui.Context) {
    drawer := ui.SettingDrawer(c, &persistedSettings).Show()
    if drawer.Changed() {
        // 在这里调用应用自己的持久化服务。
        ui.Text(c, "设置已改变").Secondary()
    }
}
```

## 主题与样式

- Ant Design Pro 默认是深色菜单、side 布局、Fluid 内容、固定顶栏和固定侧栏；`NavTheme` 可选 `"light"`、`"dark"`、`"realDark"`。
- Element Plus 使用对应主题令牌和亮色布局；shadcn 侧栏使用其 dashboard 风格，`Light()` 等经典深色侧栏选项不改变 shadcn 的固定外观。
- `ProSettingsProvider` 会把 `realDark` 转成暗色主题、`PrimaryColor` 转成主色、`Compact` 转成紧凑尺寸、`ColorWeak` 转成高对比主题；TanGo 没有 CSS 滤镜，色弱模式使用 `theme.HighContrast`。
- SettingDrawer 自身的默认隐藏项来自 `ConfigProvider.Defaults("SettingDrawer", ...)`：`hideColors`、`hideLayout`、`hideOther`。

## 注意事项

- **状态放在哪**：`ProSettings` 和外部 `open` 状态必须放在包级变量或页面状态结构中，不能在页面函数里每帧重置，否则用户选择会立即丢失。
- **作用域**：`ProSettingsProvider` 要包住 ProLayout 和 SettingDrawer；如果只包住抽屉，主题变更不会覆盖外壳。
- **布局字段**：`Apply` 只复制布局相关字段；主题相关字段由 Provider 处理。修改 `ProSettings` 后仍应每帧重新构建 `ProLayoutConfig`。
- **无障碍**：默认触发器有本地化的设置标签；用 `NoTrigger` 自定义入口时要为图标按钮设置 `A11yLabel`，并保留清晰的键盘操作路径。
- **RTL**：抽屉和浮动入口会遵循当前方向；不要用固定的屏幕左 / 右文案描述逻辑边。

## 示例代码

<!--@include: ./_gen/settingdrawer-examples.md-->

## API

<!--@include: ./_gen/settingdrawer-api.md-->

## 相关链接

- 源码：[ui/pro4_setting.go](https://github.com/tangoui/tango/blob/main/ui/pro4_setting.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[ProLayout 高级布局](/components/prolayout)、[Drawer 抽屉](/components/drawer)、[ConfigProvider 全局配置](/components/configprovider)
