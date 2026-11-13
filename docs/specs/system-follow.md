# 自绘 UI 跟随系统（主题 / 语言）与主题统一

状态：已实现，2026-11-12。代码：`uihost/follow.go`、`sys/theme/uivars`、runtime-js `Theme.vars` / `Theme.onVarsChange`。

## 1. uihost 跟随系统

`ui.RunnerOptions` 与 `ui/` 均未改动；新增内容都在 `uihost`：

```go
type Options struct {
	Surface tango.SurfaceOptions

	Theme            ThemeMode // ThemeSystem（零值）| ThemeLight | ThemeDark | ThemeFixed
	Flavor           func(dark bool, accent *theme.Color) *theme.Theme // nil：AntDesign(+Dark/+WithPrimary)
	AccentFromSystem bool       // 系统强调色作为主色
	Customize        func(*theme.Theme) // 每次生成后改令牌
	Contrast         ContrastMode // ContrastSystem（零值）| ContrastStandard | ContrastHigh
	Locale           LocaleMode // LocaleSystem（零值）| LocaleFixed
}

func (o Options) ThemeFor(sys systheme.Theme) *theme.Theme // ThemeFixed 返回 nil；不读系统对比度
func (o Options) ThemeForContrast(sys systheme.Theme, highContrast bool) *theme.Theme
func SystemHighContrast(app *tango.App) (on, ok bool)      // 主线程
func FollowSystemTheme(r *ui.Runner, on bool)              // 主线程
func FollowingSystemTheme(r *ui.Runner) bool               // 主线程
func AttachWith(s *tango.Surface, layer Layer, opts Options, page func(*ui.Context), ropts ui.RunnerOptions) (*ui.Runner, *Attached, error)
```

### 主题

- `ThemeSystem`：打开时用 `sys/theme.System(app)` 生成主题作为 `RunnerOptions.Theme`；`sys/theme.OnChange`（主线程）触发时 `runner.SetTheme(opts.ThemeFor(新主题))`。
- `ThemeLight` / `ThemeDark`：固定深浅，仍用 Flavor/Customize；开了 `AccentFromSystem` 时继续跟随强调色。
- `ThemeFixed`：完全使用 `RunnerOptions.Theme`。**兼容**：`Theme` 为零值但调用方显式传了 `RunnerOptions.Theme`，按 `ThemeFixed` 处理（旧行为）。
- 强调色 `0xRRGGBBAA` 转为不透明的 `theme.Color`；系统没有强调色（0）时 `accent` 为 nil。
- 生成顺序：`Flavor(dark, accent)`（nil 或返回 nil 时用默认 `theme.AntDesign`）→ `Customize` → 需要高对比度时 `theme.HighContrast`。
- 高对比度（`uihost/contrast.go`，详见 [accessibility.md](../accessibility.md) 6.1 节）：`ContrastSystem` 读取 `platform.ContrastPreference`，并订阅 `AppContrastChanged`（与主题监听同时注册，每个 app 一次）；变化时只对仍在跟随、且主题未被应用改动的 `ContrastSystem` 窗口，按上次的系统外观重新生成主题。`ContrastStandard` / `ContrastHigh` 固定对比度，不受系统影响。

### 手动 SetTheme 后不再跟随

uihost 无法拦截 `Runner.SetTheme`，采用两层办法：

1. **自动识别**：uihost 记住自己最后一次设置的 `*theme.Theme`。系统主题变化时，如果 `runner.Context().Theme` 已不是这个指针（应用调用了 `SetTheme`，或带主题设置的 `ConfigProvider` 替换了主题），就认定应用接管了主题，此后不再覆盖该窗口。
2. **显式开关**：`FollowSystemTheme(r, false)` 关闭；`FollowSystemTheme(r, true)` 重新跟随并立即应用当前系统外观（`ThemeFixed` 打开的窗口也可以借此改为跟随，按 `ThemeSystem` 生成）。文档建议"手动设主题前先关跟随"，使意图明确。

已知边界：`ConfigProvider` 的 `Dark/Flavor/Primary/Compact` 会派生新主题替换 `c.Theme`，同样被视为接管。

### 语言

- `LocaleSystem`：打开时 `ui.SetDefaultLocale(ui.LocaleByCode(i18n.Locale()))`；没有对应组件语言包时用英文。`i18n.OnChange`（可能在任意 goroutine）通过 `app.Post` 回到主线程后更新默认语言并 Invalidate 该应用所有 uihost 窗口。
- 默认语言是进程全局的；若发现当前默认语言既不是 uihost 上次设的、也不是新语言（应用自己调了 `ui.SetDefaultLocale`），该应用的窗口停止跟随语言。
- `LocaleFixed`：不碰全局默认语言。

### 订阅共享

每个 App 一份：`App.OnEvent` 主题监听只注册一次（tango 不提供注销，窗口全关后监听器保留但空转）；`i18n.OnChange` 在第一个窗口打开时订阅、最后一个窗口关闭（`Surface.OnClosed` / `Attached.Detach`）时取消，再开窗口重新订阅。

### Attach 与热加载

- `Attach` 等价于 `AttachWith(..., Options{}, ...)`：叠层默认跟随系统，与下方页面的 `prefers-color-scheme` 一致；需要定制时用 `AttachWith`（`opts.Surface` 被忽略）。
- `tan dev` 热加载：worker 是带真实平台后端的普通进程（隐藏），`sys/theme` 在其中可用，主题在 worker 里生成与跟随，宿主不参与；关闭宿主窗口会关闭 worker 中的替身 Surface，从而注销跟随。

## 2. WebView 与自绘 UI 主题统一

`ui/theme` 已有 `(*Theme).CSSVars()` / `CSS()` / `CSSRule`（由主题包维护，输出什么就注入什么，不在本功能里枚举变量名；shadcn 主题会额外输出 `--background`、`--primary`、`--radius` 等同名变量）。

### 包归属

注入放在新包 **`sys/theme/uivars`**，而不是给 `sys/theme.SyncPage` 加选项：`ui/theme` 依赖 `ui/paint`（连带 `golang.org/x/image/draw`、`vector` 等），放进 `sys/theme` 会让只用 WebView 的应用也链接这些包。现有 `sys/theme.SyncPage` 行为不变，两者可以同时使用。

```go
type Gen func(sys systheme.Theme) *uitheme.Theme // 返回 nil：移除变量
func SyncPage(a *tango.App, gen Gen) error        // gen 为 nil 用 Default；在创建 Surface 之前调用
func Refresh(a *tango.App)                        // 应用自身状态变化后重新生成并推送；任意 goroutine
func Default(sys systheme.Theme) *uitheme.Theme   // AntDesign，系统深色时 Dark()
func Vars(th *uitheme.Theme) map[string]string    // th.CSSVars()，nil 为空表
```

与自绘窗口保持一致：`uivars.SyncPage(app, opts.ThemeFor)`（`opts` 即传给 `uihost.Open` 的 `Options`）。

### 页面侧

- 桥接脚本在文档开始时注入 `<style id="tango-ui-vars">:root { ... }</style>`（放在文档最前，页面自己的样式可以覆盖），按 `prefers-color-scheme` 选用 `SyncPage` 调用时生成的浅色/深色变体，避免加载时闪烁。
- 页面加载完成后以及每次系统主题变化时，Go 侧推送当前精确变量（含强调色）：`window.__tangoUIVars({...})`。
- 当前变量在 `window.__tangoUIVarsState`；每次更新在 `window` 上派发 `tangouivars` 事件，`detail` 为变量表。
- runtime-js：`Theme.vars()` 返回当前变量表，`Theme.onVarsChange(fn)` 订阅变化（返回取消函数）。

```css
.card { background: var(--tango-color-bg-container); border-radius: var(--tango-radius); }
```

```ts
import { Theme } from "@tangoui/runtime";
const v = Theme.vars();                       // {"--tango-color-primary": "#1677ff", ...}
const off = Theme.onVarsChange((v) => chart.setColor(v["--tango-color-primary"]));
```

后端读不到系统主题时 `SyncPage` 返回 `ErrUnsupported`，页面保留文档开始时的变量，仍按 `prefers-color-scheme` 切换。

## 测试

- `uihost/follow_test.go`（fake 后端 `fake.Backend.SetTheme` 模拟系统变化）：默认跟随、ThemeFixed/隐式固定、ThemeDark、手动 SetTheme 后不覆盖、FollowSystemTheme 开关、AccentFromSystem、Flavor/Customize 调用次数、语言跟随与手动语言、LocaleFixed、多窗口共享订阅且全关后取消、Attach/AttachWith/Detach。
- `uihost/follow_dev_test.go`：热加载 worker 中跟随。
- `sys/theme/uivars/uivars_test.go`：注入内容等于 `CSSVars()` 输出、变化时更新、新窗口拿到当前值、Refresh、nil 移除、不支持时返回错误。
- runtime-js `test/api.test.mjs`：`Theme.vars` / `onVarsChange`。

## 以后再说

- 从 JSON 加载主题、运行时主题编辑器。
- 更多设计语言（macOS 原生、Fluent、Material）。
- 系统语言变化的自动监听（目前 i18n 需应用调用 `i18n.Refresh`）。
