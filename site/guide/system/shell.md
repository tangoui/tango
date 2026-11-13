# shell：交给系统打开链接和文件

`shell` 把 URL 和文件交给操作系统处理：用默认浏览器打开链接、用默认程序打开文件、在 Finder / 资源管理器 / 文件管理器里选中显示某个文件。

出于安全考虑，`OpenURL` 只接受白名单内的协议（默认 `http`、`https`、`mailto`）。页面通常通过绑定的方法间接调用这些函数，不能让页面只凭一个字符串就启动任意协议处理器或本地程序。

```go
import "github.com/tangoui/tango/sys/shell"
```

## 用法

```go
if err := shell.OpenURL(app, "https://example.com"); errors.Is(err, shell.ErrSchemeNotAllowed) {
	// 协议不在白名单
}

_ = shell.OpenPath(app, "/Users/me/Documents/report.pdf") // 默认程序打开；目录则在文件管理器中打开
_ = shell.RevealPath(app, "/Users/me/Documents/report.pdf") // 在文件夹里选中
```

扩展白名单（整个进程生效）：

```go
shell.AllowSchemes("http", "https", "mailto", "tel")
ok := shell.Allowed("tel:10086") // true
shell.AllowSchemes()             // 恢复 DefaultSchemes
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `OpenURL(a, url)` | 用该协议的系统处理程序打开 |
| `OpenPath(a, path)` | 用默认程序打开文件，或在文件管理器中打开目录 |
| `RevealPath(a, path)` | 在所在文件夹中选中；文件管理器不能选中时打开父目录 |
| `AllowSchemes(s...)` | 替换允许的协议集合（不区分大小写，不带冒号）；无参数恢复默认 |
| `Allowed(url) bool` | `OpenURL` 是否会接受 |
| `DefaultSchemes` | `["http", "https", "mailto"]` |
| `ErrSchemeNotAllowed` / `ErrUnsupported` | 协议不允许 / 后端没有该能力 |

完整签名见 [API 参考](/reference/api/sys-shell)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ | ✓ | ✓ | — | — |

> Windows、Linux 实现尚未实机验证。

## 在页面里调用

对应 `@tangoui/runtime` 的 `Shell` 模块，打开链接和打开路径是两个独立开关，默认都关：

```go
jsapi.Install(app, r, jsapi.Options{Allow: jsapi.OpenURL | jsapi.OpenPath})
```

```ts
import { Shell } from "@tangoui/runtime";

await Shell.openURL("https://example.com"); // 协议不在白名单时 reject "forbidden"
await Shell.openPath("/tmp/out.txt");
await Shell.revealPath("/tmp/out.txt");
```

见 [JS API](/reference/js-api) 与 [安全](/guide/concepts/security)。

## 注意

- `file` 协议永远不允许，打开本地文件请用 `OpenPath`。
- 路径会先检查是否存在。
- `OpenPath` 会用默认程序打开任意文件，给页面开 `jsapi.OpenPath` 前请确认页面内容可信。
- 应用运行后可在任意 goroutine 调用；应用退出后返回 `tango.ErrNotRunning`。
