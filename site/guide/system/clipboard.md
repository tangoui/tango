# clipboard：读写系统剪贴板

`clipboard` 读写系统剪贴板，支持三种内容：纯文本、图片（统一用 PNG 字节交换）和文件列表（与在文件管理器里复制文件相同）。每次写入都会替换剪贴板的全部内容。

```go
import "github.com/tangoui/tango/sys/clipboard"
```

## 用法

```go
_ = clipboard.WriteText(app, "hello")
text, err := clipboard.ReadText(app) // 没有文本时为 ""
```

图片和文件：

```go
png, err := clipboard.ReadImage(app) // 没有图片时 (nil, nil)
if err == nil && png != nil {
	os.WriteFile("paste.png", png, 0o644)
}

err = clipboard.WriteImage(app, pngBytes) // 不是 PNG 返回 ErrNotPNG

err = clipboard.WriteFiles(app, []string{"/Users/me/report.pdf"}) // 必须是绝对路径
paths, err := clipboard.ReadFiles(app)                           // 没有文件时为 nil
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `ReadText(a)` / `WriteText(a, text)` | 文本 |
| `ReadImage(a)` / `WriteImage(a, png)` | 图片；读取时把平台原生格式转成 PNG，写入时同时提供原生格式供其他应用粘贴 |
| `ReadFiles(a)` / `WriteFiles(a, paths)` | 文件列表；写空列表会清空剪贴板 |
| `ErrNotPNG` | `WriteImage` 收到的不是 PNG |
| `ErrNotAbsolute` | `WriteFiles` 收到相对路径 |
| `ErrUnsupported` | 后端没有剪贴板（或没有该类内容的支持） |

完整签名见 [API 参考](/reference/api/sys-clipboard)。

## 平台支持

| 能力 | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| 文本 | ✓ | ✓ | ✓ | ✓ | ✓ |
| 图片 | ✓ | ✓ | ✓ | — | — |
| 文件列表 | ✓ | ✓ | ✓ | — | — |

> Windows、Linux 实现尚未实机验证；移动端只实现了文本，仅在模拟器上验证，属实验性。

## 在页面里调用

对应 `@tangoui/runtime` 的 `Clipboard` 模块。读和写是两个独立的开关，默认都关：

```go
jsapi.Install(app, r, jsapi.Options{Allow: jsapi.ClipboardRead | jsapi.ClipboardWrite})
```

```ts
import { Clipboard } from "@tangoui/runtime";

await Clipboard.writeText("hi");
const text = await Clipboard.readText();
const png = await Clipboard.readImage();     // base64 PNG 或 null
await Clipboard.writeImage(png!);            // 也接受 data URL；非 PNG 报 bad_args
await Clipboard.writeFiles(["/tmp/a.txt"]);  // 相对路径报 bad_args
```

注入脚本的写法是 `tango.Clipboard.writeText("hi")`。读剪贴板默认关闭是有意的：剪贴板里常有密码等敏感内容。见 [JS API](/reference/js-api) 与 [安全](/guide/concepts/security)。

## 注意

- 应用运行后可在任意 goroutine 调用；应用退出后返回 `tango.ErrNotRunning`。
- 剪贴板里没有对应内容时返回零值且不报错（`""`、`nil`），错误只表示读取或转换失败。
- 移动端调用图片、文件相关函数返回 `ErrUnsupported`。
