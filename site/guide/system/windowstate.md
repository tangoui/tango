# windowstate：记住窗口的位置和大小

`windowstate` 记住窗口上次在哪：位置、大小、是否最大化。窗口变化时自动保存，下次启动时恢复。

默认所有窗口共用一个 JSON 文件 `<UserConfigDir>/<App.ID>/windows.json`，每个窗口用一个 key 区分；也可以实现 `Store` 接口存到别处。

```go
import "github.com/tangoui/tango/sys/windowstate"
```

## 用法

在显示窗口**之前**调用 `Track`，避免窗口跳动：

```go
s, _ := app.NewSurface(tango.SurfaceOptions{Hidden: true, Resizable: true})
if err := windowstate.Track(s, "main", nil); err != nil {
	log.Println("windowstate:", err)
}
s.Show()
```

多个窗口用不同 key；指定文件路径：

```go
store := windowstate.NewFileStore(filepath.Join(dir, "windows.json"))
_ = windowstate.Track(editor, "editor", store)
_ = windowstate.Track(inspector, "inspector", store)
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Track(s, key, store) error` | 从 `store` 恢复 `s`，之后在移动、缩放、最大化 / 还原、关闭和应用退出时保存；`store` 为 nil 时用默认文件 |
| `State` | `X`、`Y`、`Width`、`Height`（非最大化时的边界，同 `Surface.Bounds`）、`Maximized` |
| `Store` | 接口：`Load(key) (State, bool)`、`Save(key, State) error`，实现须并发安全 |
| `NewFileStore(path)` | 基于单个 JSON 文件的 Store；同一路径的 Store 共享，不会互相覆盖 |
| `(*FileStore).Path()` | 文件路径 |
| `DefaultPath(appID)` | `<os.UserConfigDir>/<appID>/windows.json`；appID 为空时用可执行文件名 |
| `SaveDelay` | 最后一次变化后等待多久再写盘，默认 400ms |

完整签名见 [API 参考](/reference/api/sys-windowstate)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ | ✓ | ✓ | 不适用 | 不适用 |

> Windows、Linux 尚未实机验证。窗口不能自由摆放的后端上 `Track` 返回 `tango.ErrUnsupported`。

## 在页面里调用

目前没有 jsapi 对应，也通常不需要：在 Go 侧创建窗口时调用 `Track` 即可。如果要让页面提供「重置窗口布局」，可以绑定一个方法：

```go
type Layout struct{ s *tango.Surface }

func (l Layout) Reset() error {
	l.s.SetSize(1024, 768)
	return l.s.Center()
}

bind.Register(r, "Layout", Layout{win})
```

## 注意

- 保存的位置如果已不在任何显示器上（例如拔掉了外接屏），不会恢复位置，窗口保持保存的大小并居中。
- `FileStore` 原子地重写整个文件。
- 默认路径按 `App.ID()` 区分应用（为空时用可执行文件名），请为应用设置稳定的 `ID`。窗口相关概念见 [窗口](/guide/window)。
