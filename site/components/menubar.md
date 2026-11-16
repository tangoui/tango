---
title: Menubar 菜单栏
---
# Menubar 菜单栏

窗口内的应用菜单栏（对标 shadcn/ui Menubar 和桌面应用的菜单栏）：一排顶层菜单，点击后展开下拉面板，支持子菜单、勾选项、单选组、分隔线、快捷键提示，以及 Alt / F10 键盘进入。

## 何时使用

- 桌面风格的应用需要“文件 / 编辑 / 视图 / 帮助”这类分层命令菜单，并且要完整的键盘操作。
- 无边框窗口里，把菜单放进自绘标题栏（[TitleBar 标题栏](/components/titlebar) 的 `Menu` 插槽）。
- 导航到页面或视图用 [Menu 导航菜单](/components/menu)；某个按钮或右键弹出的一组操作用 [Dropdown 下拉菜单](/components/dropdown)。

## 在线体验

<GalleryPreview component="Menubar" />

## 基本用法

`ui.Menubar` 接收顶层菜单列表，`Clicked()` 返回这一帧被选中条目的 `Key`（没有则为空串）。菜单列表可以每帧构建，勾选项和单选项通过指针绑定到包级变量。

```go
import "github.com/tangoui/tango/ui"

var lastCommand string

func page(c *ui.Context) {
    mb := ui.Menubar(c, []ui.MenubarMenu{
        {Label: "文件", Items: []ui.MenubarItem{
            {Key: "new", Label: "新建", Shortcut: "Ctrl+N"},
            {Key: "open", Label: "打开…", Shortcut: "Ctrl+O"},
        }},
        {Label: "帮助", Items: []ui.MenubarItem{
            {Key: "about", Label: "关于"},
        }},
    }).A11yLabel("应用菜单")

    if k := mb.Clicked(); k != "" {
        lastCommand = k
    }
    ui.Text(c, "最近执行："+lastCommand)
}
```

`MenubarMenu.Key` 标识顶层菜单，留空时用 `Label`；`MenubarItem.Key` 同理，留空时用 `Label`。`Clicked()` 报告的就是这个 key，用 `switch` 分发命令即可。

## 常见用法

### 分隔线、图标、禁用与危险项

条目可以带 `Icon`，`Divider: true` 的条目是分隔线（不需要 Key），`Disabled` 置灰，`Danger` 用危险色。整个顶层菜单也可以 `Disabled`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func fileMenu() []ui.MenubarMenu {
    return []ui.MenubarMenu{
        {Key: "file", Label: "文件", Items: []ui.MenubarItem{
            {Key: "new", Label: "新建窗口", Icon: icon.Plus, Shortcut: "Ctrl+N"},
            {Key: "open", Label: "打开…", Icon: icon.Folder, Shortcut: "Ctrl+O"},
            {Divider: true},
            {Key: "print", Label: "打印…", Shortcut: "Ctrl+P", Disabled: true},
            {Key: "delete", Label: "删除工程", Danger: true},
        }},
        {Key: "window", Label: "窗口", Disabled: true},
    }
}

func menubarIcons(c *ui.Context) {
    ui.Menubar(c, fileMenu())
}
```

### 子菜单

`Children` 让条目打开子菜单，可以多层嵌套。点击带子菜单的条目只是展开，`Clicked()` 只报告叶子条目的 key。

```go
import "github.com/tangoui/tango/ui"

func shareMenu() []ui.MenubarMenu {
    return []ui.MenubarMenu{{Key: "file", Label: "文件", Items: []ui.MenubarItem{
        {Key: "share", Label: "共享", Children: []ui.MenubarItem{
            {Key: "mail", Label: "邮件链接"},
            {Key: "msg", Label: "消息"},
        }},
    }}}
}

func menubarSub(c *ui.Context) {
    ui.Menubar(c, shareMenu())
}
```

### 勾选项与单选组

`Kind: ui.MenubarItemCheckbox` 配合 `Checked *bool`：点击时直接翻转你的变量。单选项用 `Kind: ui.MenubarItemRadio`，多个条目共用同一个 `Radio *string`，各自的 `Value` 是选中时写入的值（`Value` 留空时用 key）；`Group: true` 的条目是带标题的分组，`Children` 在分组下内联显示。`Changed()` 在勾选项或单选项改变值的那一帧返回 true，这时变量已经更新。

```go
import "github.com/tangoui/tango/ui"

var (
    showStatus = true
    showRuler  bool
    zoom       = "100"
)

func viewMenu() []ui.MenubarMenu {
    return []ui.MenubarMenu{{Key: "view", Label: "视图", Items: []ui.MenubarItem{
        {Key: "status", Label: "状态栏", Kind: ui.MenubarItemCheckbox, Checked: &showStatus},
        {Key: "ruler", Label: "标尺", Kind: ui.MenubarItemCheckbox, Checked: &showRuler},
        {Label: "缩放", Group: true, Children: []ui.MenubarItem{
            {Key: "z75", Label: "75%", Kind: ui.MenubarItemRadio, Radio: &zoom, Value: "75"},
            {Key: "z100", Label: "100%", Kind: ui.MenubarItemRadio, Radio: &zoom, Value: "100"},
            {Key: "z150", Label: "150%", Kind: ui.MenubarItemRadio, Radio: &zoom, Value: "150"},
        }},
    }}}
}

func menubarChecks(c *ui.Context) {
    mb := ui.Menubar(c, viewMenu())
    if mb.Changed() {
        // 布局、缩放等设置变了，showStatus / showRuler / zoom 已是新值
    }
    if showStatus {
        ui.Text(c, "状态栏已显示，缩放 "+zoom+"%")
    }
}
```

### 快捷键提示

`Shortcut` 只用于显示（在条目右侧按平台习惯排版），**不会注册快捷键**。真正的热键需要应用自己处理，并和菜单点击走同一个命令函数，避免两处逻辑分叉。

```go
import "github.com/tangoui/tango/ui"

var commandLog []string

func runCommand(key string) {
    commandLog = append(commandLog, key)
}

func menubarCommands(c *ui.Context) {
    mb := ui.Menubar(c, []ui.MenubarMenu{{Label: "编辑", Items: []ui.MenubarItem{
        {Key: "undo", Label: "撤销", Shortcut: "Ctrl+Z"},
        {Key: "redo", Label: "重做", Shortcut: "Ctrl+Shift+Z"},
    }}})
    if k := mb.Clicked(); k != "" {
        runCommand(k)
    }
}
```

### 键盘操作

- 单独按下并松开 **Alt**，或按 **F10**，把焦点移到菜单栏；再按一次（或 Esc）回到原来的位置并关闭菜单。
- 菜单栏上：左 / 右切换顶层菜单（RTL 下镜像），Home / End 跳到首尾，下 / 回车 / 空格打开菜单，上键打开并定位到最后一项，字母键跳到下一个以该字母开头的菜单。
- 菜单打开后：上 / 下移动，右 / 左展开和收起子菜单，到边缘时切换到相邻的顶层菜单，回车执行，Esc 关闭，字母键跳到下一个以该字母开头的条目。
- 鼠标：点击顶层项打开，打开后把指针移到相邻的顶层项会直接切换。

### 读取当前展开的菜单

`OpenMenu()` 返回当前展开的顶层菜单 key，全部收起时为空串，可以用来在菜单展开时暂停某些后台刷新。

```go
import "github.com/tangoui/tango/ui"

func menubarOpen(c *ui.Context) {
    mb := ui.Menubar(c, []ui.MenubarMenu{
        {Key: "file", Label: "文件", Items: []ui.MenubarItem{{Key: "new", Label: "新建"}}},
    })
    if mb.OpenMenu() == "file" {
        ui.Text(c, "文件菜单已展开").Tertiary()
    }
}
```

## 主题与样式

菜单栏和触发器的外观来自 `theme.Components.Menubar`（`theme.MenubarToken`）：条/触发器的高度与内边距（`Height`、`TriggerHeight`、`TriggerPaddingInline`）、背景 `Bg`、外框 `BorderColor`、下分隔线 `DividerColor`、圆角与阴影、触发器的悬停 / 展开颜色，以及勾选项指示器 `IndicatorColor` / `IndicatorSize` / `IndicatorWidth`。下拉面板复用 [Dropdown](/components/dropdown) 的令牌。可以用 `ui.ConfigProvider(c).Components(theme.Components{Menubar: ...})` 局部覆盖。

- **Ant Design**：通栏的条，背景为容器色，底部一条分隔线。
- **Element Plus**：更高、更平的条，展开和悬停的触发器带主色浅底与主色文字。
- **shadcn**：带边框、圆角和轻阴影的行内小条（只占内容宽度），触发器字重 500，勾选指示器更大。

Menubar 没有 `ConfigProvider.Defaults` 键。放进 [TitleBar](/components/titlebar) 的 `Menu` 插槽时，标题栏会自动去掉菜单栏自己的底色、边框、分隔线和阴影，并让它与标题栏等高。

## 注意事项

- **状态放在哪**：勾选项、单选项绑定的 `*bool` / `*string` 必须是长期存在的变量（包级或应用状态里），不要指向页面函数里的局部变量，否则每帧都会被重置。菜单的展开状态、键盘焦点由 Menubar 内部按自身标识保留；同一位置会被条件分支替换时，用 `ui.Key(c, "app-menubar", func() { ... })` 包住。
- **Key 要唯一**：同一个顶层菜单内（包含子菜单）条目的 key 不能重复，`Clicked()`、勾选、单选都按 key 找条目。分隔线不需要 key。
- **`Clicked()` 与 `Changed()`**：只在选中发生的那一帧有值；勾选 / 单选条目既会更新绑定的变量，也会出现在 `Clicked()` 里。
- **无障碍**：菜单栏是水平的 menubar，顶层触发器报告 popup / 展开状态；多个菜单栏同时存在时用 `A11yLabel` 区分。条目的快捷键会作为快捷键提示暴露给读屏软件。
- **RTL**：左右方向键、触发器顺序和面板展开方向都会按阅读方向镜像。

## 示例代码

<!--@include: ./_gen/menubar-examples.md-->

## API

<!--@include: ./_gen/menubar-api.md-->

## 相关链接

- [源码：menubar.go](https://github.com/tangoui/tango/blob/main/ui/menubar.go)
- [ui Go API](/reference/api/ui)
- [Dropdown 下拉菜单](/components/dropdown)
- [Menu 导航菜单](/components/menu)
- [TitleBar 标题栏](/components/titlebar)
