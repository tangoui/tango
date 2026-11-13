# menu：原生菜单（菜单栏、窗口菜单、右键菜单）

`menu` 在 Go 里描述原生菜单，并把点击路由到 Go 回调。同一个 `*menu.Menu` 可以同时用在多个地方：应用菜单栏（`SetAppMenu`）、窗口菜单（`SetSurfaceMenu`）、托盘图标（[tray](/guide/system/tray)）和右键菜单（`Popup`、`RegisterContext`）。

本页只列 API；完整教程（菜单结构、角色、动态更新、页面右键菜单）见 [菜单与托盘](/guide/menu-tray)。

```go
import "github.com/tangoui/tango/sys/menu"
```

## 用法

```go
m := menu.New()
appMenu := m.Submenu("App")
appMenu.Role(menu.About)
appMenu.Separator()
appMenu.Role(menu.Quit)

file := m.Submenu("File")
file.Add("New", newDoc).Accelerator = "CmdOrCtrl+N"
var wrap *menu.Item
wrap = file.Check("Word Wrap", true, func() { setWrap(wrap.IsChecked()) })

m.Submenu("Edit").Role(menu.EditMenu)
_ = menu.SetAppMenu(app, m)
```

改动菜单后调用 `Update`：

```go
item.SetLabel("Close Project")
item.SetDisabled(false)
_ = m.Update()
```

页面元素的右键菜单（`data-tango-menu="row"`）：

```go
_ = menu.RegisterContextFunc(app, "row", func(c menu.Context) *menu.Menu {
	cm := menu.New()
	cm.Add("删除 "+c.Data, func() { deleteRow(c.Data) })
	return cm
})
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `New()` | 空菜单 |
| `(*Menu).Add` / `Check` / `Radio` / `Role` / `Separator` / `Submenu` | 追加普通项、勾选项、单选项、标准项、分隔线、子菜单 |
| `(*Menu).Update()` | 把当前状态推送到所有显示位置 |
| `Item` 字段 | `Label`、`Accelerator`、`Role`、`Disabled`、`Checked`、`Checkable`、`RadioGroup`、`Icon`、`Submenu`、`OnClick` |
| `(*Item).SetLabel` / `SetChecked` / `SetDisabled` / `SetIcon` / `IsChecked` | 任意 goroutine 安全的读写 |
| `SetAppMenu(a, m)` | 应用菜单：macOS 菜单栏，Windows/Linux 上所有现有和以后的窗口 |
| `SetSurfaceMenu(a, s, m)` | 单个窗口的菜单（Windows/Linux；macOS 由后端决定，通常就是共享菜单栏） |
| `Popup(s, x, y, m)` | 在窗口内容坐标处弹出右键菜单，阻塞到菜单关闭 |
| `RegisterContext(a, name, m)` / `RegisterContextFunc(a, name, fn)` | 声明式页面右键菜单 |
| `Role` 常量 | `Quit` `About` `Hide` `Copy` `Paste` `Cut` `SelectAll` `Undo` `Redo` `Minimize` `Close` `AppMenu` `EditMenu` `WindowMenu` |

完整签名见 [API 参考](/reference/api/sys-menu)。

## 平台支持

| | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| 应用 / 窗口菜单 | ✓ | ✓ | ✓ | — | — |
| `Popup` / 页面右键菜单 | ✓ | ✓ | ✓ | — | — |
| 菜单项图标 | ✓ | ✓ | 忽略 | — | — |

> Windows、Linux 实现尚未实机验证。

## 在页面里调用

对应 `@tangoui/runtime` 的 `Menu` 模块，`Menu` 能力默认开启。页面只能在自己的窗口里弹出菜单，`role` 只允许编辑类（`copy`、`paste`、`cut`、`selectAll`、`undo`、`redo`）：

```ts
import { Menu } from "@tangoui/runtime";

Menu.bindContextMenu(document.querySelector("#list")!, (ev) => [
  { label: "重命名", onClick: () => rename(ev.target) },
  { type: "separator" },
  { label: "复制", role: "copy" },
]);
```

见 [JS API](/reference/js-api)。

## 注意

- 点击回调在主线程运行。菜单首次设置前可以直接写 `Item` 字段；之后请用 setter，或在主线程里写（点击回调里、`App.Post`）。
- 勾选项和单选项被点击时会自动切换状态并刷新原生菜单，然后才调用 `OnClick`；其他改动要调用 `Update`。
- 右键菜单在弹出时才构建，不需要 `Update`；`RegisterContext` 请在创建 Surface 之前调用，已存在的 Surface 只对当前页面生效。
