# theme：系统深浅色与强调色

`theme` 读取系统当前是浅色还是深色外观、强调色是什么，并在用户切换时通知应用。

WebView 本身已经把 `prefers-color-scheme` 应用到页面上，纯 CSS 的深浅色适配不需要这个包。需要强调色、需要一个可挂样式的属性，或 Go 侧要跟着变化（例如换托盘图标）时再用它。

```go
import "github.com/tangoui/tango/sys/theme"
```

## 用法

```go
_ = theme.SyncPage(app) // 在创建 Surface 之前调用

theme.OnChange(app, func(t theme.Theme) {
	log.Printf("dark=%v accent=%s", t.Dark, theme.AccentCSS(t.Accent))
})

app.OnReady(func() {
	if t, err := theme.System(app); err == nil && t.Dark {
		// ...
	}
})
```

`SyncPage` 之后页面根元素形如 `<html data-tango-theme="dark" style="--tango-accent: #0a84ffff">`：

```css
button.primary { background: var(--tango-accent, #0a84ff); }
html[data-tango-theme="dark"] .card { border-color: #333; }
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `System(a) (Theme, error)` | 当前系统主题 |
| `OnChange(a, fn)` | 深浅色或强调色变化时在主线程调用 |
| `SyncPage(a) error` | 让每个页面的 `data-tango-theme`（`"dark"` / `"light"`）和 `--tango-accent` 保持最新；页面也可监听 `window` 上的 `tangotheme` 事件，`detail` 为 `{dark, accent}` |
| `AccentCSS(c) string` | 把 `0xRRGGBBAA` 格式化为 `#rrggbbaa`，0 返回 `""` |
| `Theme` | `Dark bool`、`Accent uint32`（`0xRRGGBBAA`，未知为 0） |

完整签名见 [API 参考](/reference/api/sys-theme)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ | ✓ | ✓ | — | — |

> Windows、Linux 实现尚未实机验证。移动端页面仍可用 `prefers-color-scheme`。

## 在页面里调用

对应 `@tangoui/runtime` 的 `Theme` 模块，属于默认开启的 `Info` 能力：

```ts
import { Theme } from "@tangoui/runtime";

const { dark, accent } = await Theme.get(); // accent 为 "#rrggbbaa"，未知时为 ""
const off = Theme.onChange((t) => document.body.classList.toggle("dark", t.dark));
```

见 [JS API](/reference/js-api)。

## 与自绘 UI 共享令牌

子包 `sys/theme/uivars` 把自绘 UI（`ui/theme`）的设计令牌作为 CSS 变量注入每个页面，让 WebView 页面和 `uihost` 自绘窗口视觉一致。它单独成包，只用 WebView 的应用引入 `sys/theme` 不会链接 `ui` 包。

```go
import "github.com/tangoui/tango/sys/theme/uivars"

_ = uivars.SyncPage(app, nil) // 在创建 Surface 之前调用；nil：Ant Design，系统深色时用暗色

// 与自绘窗口用同一套主题（Flavor、AccentFromSystem、Customize 等）
opts := uihost.Options{AccentFromSystem: true}
_ = uivars.SyncPage(app, opts.ThemeFor)
```

注入的是 `(*theme.Theme).CSSVars()` 的全部输出，写成 `:root { … }` 放在 `<style id="tango-ui-vars">` 里（文档最前，页面样式可覆盖）。文档开始时就按 `prefers-color-scheme` 套用浅色或深色变体，页面加载完成后和每次系统深浅色、强调色变化时换成精确值。生成函数返回 nil 时移除变量；它依赖应用自身状态时，状态变化后调用 `uivars.Refresh(app)`。

```css
body { background: var(--tango-color-bg-layout); color: var(--tango-color-text); }
.btn-primary { background: var(--tango-color-primary); border-radius: var(--tango-radius); }
```

页面脚本里：

```ts
import { Theme } from "@tangoui/runtime";

const vars = Theme.vars(); // {"--tango-color-primary": "#1677ff", ...}；未注入时为 {}
const off = Theme.onVarsChange((v) => chart.setOption({ color: [v["--tango-color-primary"]] }));
```

不用 runtime 时可直接读 `window.__tangoUIVarsState`，或监听 `window` 上的 `tangouivars` 事件（`detail` 为变量表）。Vue / React 组件里直接在样式中写 `var(--tango-…)` 即可，变量变化时浏览器自动重绘，无需重新渲染组件。

| 名字 | 说明 |
|---|---|
| `uivars.SyncPage(a, gen) error` | 注入并跟随；`gen func(theme.Theme) *uitheme.Theme`，nil 用 `Default`；再次调用只替换 `gen` |
| `uivars.Refresh(a)` | 用 `gen` 重新生成并推送到所有页面，任意 goroutine |
| `uivars.Default(sys)` | 默认生成函数 |
| `uivars.Vars(th)` | 注入的变量表（`th.CSSVars()`） |

## 注意

- `SyncPage` 会添加桥接脚本，须在创建 Surface 之前调用。已存在的页面在每次变化时更新；之后加载的页面（新窗口或导航后）在加载完成时拿到当前主题，在此之前深浅色跟随 `prefers-color-scheme`。
- 后端读不到主题时 `System` 和 `SyncPage` 返回 `ErrUnsupported`，此时 `data-tango-theme` 仍跟随 `prefers-color-scheme`。
- 应用运行后可在任意 goroutine 调用；应用退出后返回 `tango.ErrNotRunning`。
- 自绘窗口（`uihost.Open`）默认已经跟随系统深浅色，不需要自己监听 `OnChange` 再 `SetTheme`，见 [自绘 UI](/guide/ui#跟随系统深浅色与语言)。
