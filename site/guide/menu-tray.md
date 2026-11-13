# 菜单与托盘

菜单用 Go 描述（`sys/menu`），同一个 `*menu.Menu` 可以放到应用菜单栏、窗口菜单、托盘图标和右键菜单里；点击回调在主线程上运行。托盘图标是 `sys/tray`。

::: tip 平台状态
macOS 已实机验证；Windows、Linux 的菜单与托盘已实现但尚未实机验证。移动端没有菜单和托盘（返回 `ErrUnsupported`）。
:::

## 应用菜单

```go
m := menu.New()
m.Role(menu.AppMenu)                 // macOS 应用菜单（关于、隐藏、退出……）

file := m.Submenu("文件")
file.Add("新建", newDoc).Accelerator = "CmdOrCtrl+N"
file.Add("打开…", openDoc).Accelerator = "CmdOrCtrl+O"
file.Separator()
wrap := file.Check("自动换行", true, func() { setWrap(wrap.IsChecked()) })

view := m.Submenu("视图")
view.Radio("size", "小", false, func() { setSize(12) })
view.Radio("size", "大", true, func() { setSize(16) })

m.Role(menu.EditMenu)                // 撤销、重做、剪切、复制、粘贴、全选
m.Role(menu.WindowMenu)

if err := menu.SetAppMenu(app, m); err != nil { ... }
```

- **标准项（Role）**：`Quit`、`About`、`Hide`、`Copy`、`Paste`、`Cut`、`SelectAll`、`Undo`、`Redo`、`Minimize`、`Close`，以及整组的 `AppMenu`、`EditMenu`、`WindowMenu`。标签和动作由平台提供，会跟随系统语言（见 [多语言](/guide/i18n)）。
- **加速键写法**：`CmdOrCtrl+Shift+N`，`CmdOrCtrl` 在 macOS 是 Command、其他平台是 Ctrl。
- `menu.SetSurfaceMenu(app, s, m)` 设置某个窗口的菜单栏（Windows / Linux；macOS 通常是共享菜单栏）。

### 修改菜单

`Menu` 是一份描述。设置之后用 setter 修改（任意 goroutine），再对菜单或任一子菜单调用 `Update()` 推送到所有显示它的地方：

```go
item := file.Add("同步", syncNow)
// ……之后
item.SetLabel("同步中…")
item.SetDisabled(true)
file.Update()
```

可勾选项和单选项例外：点击时自动切换 `Checked` 并刷新，然后才调用 `OnClick`。

## 右键菜单

**Go 描述、页面声明位置**：

```go
menu.RegisterContext(app, "row", rowMenu)          // 在创建窗口前注册
menu.RegisterContextFunc(app, "file", func(c menu.Context) *menu.Menu {
    return buildFileMenu(c.Data)                    // c.Data 来自 data-tango-menu-data
})
```

```html
<tr data-tango-menu="file" data-tango-menu-data="42">…</tr>
```

**直接弹出**：`menu.Popup(s, x, y, m)`。

**页面描述、原生弹出**（jsapi 的 `Menu`，默认开启，只允许编辑类 role）：

```js
import { Menu } from "@tangoui/runtime";
Menu.bindContextMenu(document.body, () => [
  { label: "复制", role: "copy" },
  { type: "separator" },
  { label: "置顶", type: "checkbox", onClick: (on) => Window.setAlwaysOnTop(on) },
]);
```

## 托盘图标

```go
t, err := tray.New(app, tray.Options{
    Icon:     iconPNG,      // 32×32 或 44×44（2x）PNG
    Template: true,         // macOS：单色模板图，自动适配菜单栏深浅色
    Tooltip:  "Notes",
    MenuOn:   tray.MenuOnSecondary, // 右键开菜单、左键触发 OnClick（macOS、Windows）
})

m := menu.New()
m.Add("显示窗口", showMain)
m.Separator()
m.Role(menu.Quit)
t.SetMenu(m)
t.OnClick(showMain)
```

点击行为：

| 设置 | macOS | Windows | Linux（AppIndicator） |
|---|---|---|---|
| 有菜单、默认 | 左键开菜单 | 右键开菜单，左键 `OnClick` | 任意点击开菜单 |
| `MenuOnSecondary` | 右键开菜单，左键 `OnClick` | 同左 | 不区分左右键 |
| `MenuOnPrimary` | 左键开菜单 | 左键开菜单 | 任意点击开菜单 |
| 无菜单 | 所有点击进 `OnClick` / `OnRightClick` / `OnDoubleClick` | 同左 | 收不到点击，请设置菜单 |

`SetIcon`、`SetTitle`（macOS 图标旁的文字）、`SetTooltip`、`Bounds`（图标位置，用于在旁边弹窗口；Linux 拿不到）、`Remove`。

### 常驻托盘的应用

```go
keep := false
app := tango.New(tango.Options{Name: "Notes", QuitOnLastClose: &keep})
// 可选：macOS 上隐藏 Dock 图标，只留托盘
appctl.SetDockVisible(app, false)
```

完整示例见 `examples/showcase`（托盘菜单里的「发送通知」「暂停行情流」通过事件回传给页面）。

## 相关

- [menu](/guide/system/menu)、[tray](/guide/system/tray) 包页面
- [hotkey](/guide/system/hotkey)：全局快捷键与窗口内快捷键
