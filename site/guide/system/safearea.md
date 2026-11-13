# safearea：系统界面遮挡的安全区域

`safearea` 报告页面被系统界面遮住的部分（状态栏、刘海 / 灵动岛、Home 指示条、导航栏）并跟踪变化（例如旋转）。数值是相对页面 WebView 的 CSS 像素。

`SyncPage` 把安全区同步到每个页面的 CSS 变量，适合 `env(safe-area-inset-*)` 不可靠的场景（Android WebView 那里报 0、旧引擎、需要在 JS 里拿数值）。

```go
import "github.com/tangoui/tango/sys/safearea"
```

## 用法

在创建 Surface 之前开启同步：

```go
_ = safearea.SyncPage(app) // 桌面返回 ErrUnsupported，可忽略
```

页面 CSS 用 `env()` 作后备：

```css
header {
  padding-top: var(--tango-safe-top, env(safe-area-inset-top, 0px));
}
```

在 Go 里读取：

```go
in, err := safearea.Get(app)
if err == nil {
	log.Println(in.Top, in.Bottom, in.Left, in.Right)
}
safearea.OnChange(app, func(in safearea.Insets) { relayout(in) })
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Get(a) (Insets, error)` | 当前安全区 |
| `OnChange(a, fn)` | 变化时在主线程调用；没有安全区的后端上什么也不做 |
| `SyncPage(a) error` | 保持每个页面的 `--tango-safe-top/bottom/left/right` 为最新值；页面也可以监听 `window` 上的 `tangosafearea` 事件，`detail` 为 `{top, bottom, left, right}` |
| `Insets` | `Top`、`Bottom`、`Left`、`Right`（float64，CSS 像素） |

完整签名见 [API 参考](/reference/api/sys-safearea)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| — | — | — | ✓ 窗口 `safeAreaInsets` | ✓ WebView 被系统栏 / 刘海 / 输入法遮住的部分（宿主已留边，通常为 0） |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应；一般用 `SyncPage` 的 CSS 变量即可，JS 里需要数值时监听事件：

```js
window.addEventListener("tangosafearea", (e) => {
  const { top, bottom } = e.detail;
  layout(top, bottom);
});
```

## 注意

- **iOS**：页面默认铺满屏幕，但只有在页面声明 `viewport-fit=cover` 后这些数值才有意义；否则 WKWebView 自己会给内容留边，再按安全区加 padding 会重复。
- **Android**：宿主让页面避开系统栏，数值通常为 0。
- `SyncPage` 会添加一个桥接脚本，必须在创建 Surface 之前调用；不支持的后端上返回 `ErrUnsupported` 且什么也不安装。
- 不需要在 `tan.json` 的 `permissions` 里声明。`Get` 在桌面返回 `ErrUnsupported`，应用退出后返回 `tango.ErrNotRunning`。更多见 [移动端](/guide/mobile)。
