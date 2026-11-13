# recent：最近打开的文档列表

`recent` 维护应用自己的「最近打开」列表，最新的在前，最多保留 `Max`（10）条。列表保存在应用配置目录下的 `recent.json`；在 macOS 上每次 `Add` 还会同步给系统，Dock 菜单里的最近项目会跟着更新。

适合在「文件 → 打开最近」子菜单、欢迎页里展示。

```go
import "github.com/tangoui/tango/sys/recent"
```

## 用法

记录系统要求应用打开的文件：

```go
app.OnOpenFiles(func(paths []string) {
	for _, p := range paths {
		open(p)
		_ = recent.Add(app, p)
	}
})
```

生成「打开最近」子菜单：

```go
// 在主线程调用（例如菜单点击回调里，或用 App.Post）
func rebuildRecent(sub *menu.Menu) {
	sub.Items = nil
	list, _ := recent.List(app)
	for _, p := range list {
		p := p
		sub.Add(filepath.Base(p), func() { open(p) })
	}
	sub.Separator()
	sub.Add("清除列表", func() { _ = recent.Clear(app) })
	_ = sub.Update()
}
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Add(a, path)` | 放到列表顶部（转为绝对路径，重复项上移），任意 goroutine 安全 |
| `List(a) ([]string, error)` | 最新在前，跳过已不存在的文件 |
| `Remove(a, path)` | 从列表删除一项 |
| `Clear(a)` | 清空列表（macOS 上同时清空系统列表） |
| `Max` | 常量 10 |

完整签名见 [API 参考](/reference/api/sys-recent)。

## 平台支持

| | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| 应用内列表（`recent.json`） | ✓ | ✓ | ✓ | 未验证 | 未验证 |
| 同步到系统最近文档 | ✓ `NSDocumentController` | — | — | — | — |

> 列表本身由 Go 读写 JSON 文件，不依赖平台后端；Windows、Linux 尚未实机验证。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Recent struct{ app *tango.App }

func (rc Recent) List() ([]string, error) { return recent.List(rc.app) }

bind.Register(r, "Recent", Recent{app})
```

页面里 `await tango.call("Recent.List")`。

## 注意

- 应用必须设置 `tango.Options` 的 `ID` 或 `Name`，否则返回错误（用于确定配置目录）。
- macOS 的 Dock 只列出应用在 Info.plist 里声明过的文档类型。
- `Remove` 不会改动 macOS 的系统列表：AppKit 没有删除单项的接口。
