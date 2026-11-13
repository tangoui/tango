# dialog：原生消息框与文件对话框

`dialog` 弹出系统原生的消息框、打开对话框和保存对话框。每个函数都会阻塞调用方直到用户作答；在主线程（例如某个回调里）调用时，它会在等待期间继续运转应用的事件循环，界面不会卡死。

传入 `Parent` 时对话框挂在该窗口上（macOS 上是 sheet，其他平台是 owner 窗口）。

```go
import "github.com/tangoui/tango/sys/dialog"
```

## 用法

```go
btn, err := dialog.Message(app, dialog.MessageOptions{
	Parent:  win,
	Kind:    dialog.Question,
	Title:   "未保存的修改",
	Message: "关闭前要保存吗？",
	Buttons: []string{"保存", "不保存", "取消"},
})
if errors.Is(err, dialog.ErrCanceled) || btn == 2 {
	return
}
```

打开与保存：

```go
paths, err := dialog.Open(app, dialog.FileOptions{
	Parent:   win,
	Title:    "选择图片",
	Filters:  []dialog.FileFilter{{Name: "Images", Patterns: []string{"*.png", "*.jpg"}}},
	Multiple: true,
})

path, err := dialog.Save(app, dialog.FileOptions{Parent: win, Filename: "untitled.txt"})
if errors.Is(err, dialog.ErrCanceled) {
	// 用户关掉了对话框
}
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Message(a, MessageOptions) (int, error)` | 消息框，返回所选按钮的下标（默认单个 "OK" 为 0） |
| `Open(a, FileOptions) ([]string, error)` | 打开对话框；`Multiple` 多选，`PickFolders` 选目录 |
| `Save(a, FileOptions) (string, error)` | 保存对话框；`Filename` 为默认文件名 |
| `MessageOptions` | `Parent`、`Kind`、`Title`、`Message`、`Buttons` |
| `FileOptions` | `Parent`、`Title`、`Directory`、`Filename`、`Filters`、`Multiple`、`PickFolders`、`CanCreateDirs`、`ShowHidden` |
| `Info` `Warning` `Error` `Question` | 消息框图标 / 级别 |
| `ErrCanceled` / `ErrUnsupported` | 用户取消 / 后端没有对话框 |

完整签名见 [API 参考](/reference/api/sys-dialog)。

## 平台支持

| 函数 | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| `Message` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `Open` | ✓ | ✓ | ✓ | ✓ 文档选择器（支持选目录） | ✓（不支持 `PickFolders`） |
| `Save` | ✓ | ✓ | ✓ | — | — |

> Windows、Linux 实现尚未实机验证；移动端能力仅在模拟器上验证，属实验性。

## 在页面里调用

对应 `@tangoui/runtime` 的 `Dialog` 模块，需要开启 `Dialogs`（默认关）。对话框挂在发起调用的窗口上：

```go
jsapi.Install(app, r, jsapi.Options{Allow: jsapi.Dialogs})
```

```ts
import { Dialog, TangoError } from "@tangoui/runtime";

const i = await Dialog.message({ kind: "question", title: "删除", message: "确定？", buttons: ["删除", "取消"] });
try {
  const files = await Dialog.open({ multiple: true, filters: [{ name: "Text", patterns: ["*.txt"] }] });
  const out = await Dialog.save({ filename: "notes.md" });
} catch (e) {
  if (e instanceof TangoError && e.code === "canceled") { /* 用户取消 */ }
}
```

见 [JS API](/reference/js-api)。

## 注意

- 用户按 Esc 或关闭按钮时，`Message` 返回 `(-1, ErrCanceled)`；只有一个按钮的消息框例外，返回 `(0, nil)`。文件对话框被关掉返回 `ErrCanceled`。
- 应用退出（包括对话框开着时退出）返回 `tango.ErrNotRunning`。
- `CanCreateDirs` 只在平台对话框允许应用选择时生效（macOS、GTK）；Windows 总是提供「新建文件夹」。
- 移动端的 `Save` 返回 `ErrUnsupported`：iOS 没有「选择保存位置」面板，Android 的 `ACTION_CREATE_DOCUMENT` 只给 content URI 而不是路径。请写到应用沙盒里，再用 [share](/guide/system/share) 分享出去。Android 上请在 goroutine 里调用（见 [移动端](/guide/mobile)）。
