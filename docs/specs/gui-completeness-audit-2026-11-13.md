# Tango 自绘 GUI 功能复核

日期：2026-11-13。基线提交：`6d75dcc`。

范围：`ui`、`ui/chart`、`ui/editor`、`ui/mobile`、`uihost` 与平台 Canvas 接入。以当前代码和仓库规格为依据，核查已有覆盖报告中的遗漏；没有重新逐项对照外部组件库的全部 API。本轮未修改产品代码。

## 结论

基础桌面控件、三套主题、RTL、表单和表格已经有较完整的实现。剩余工作集中在平台能力闭环、异步交互、部分高级模式和编辑器后续里程碑。旧的组件覆盖报告有大量已过时条目，不能继续用其中的完整率、Defaults 接线率或缺失数量描述当前版本。

下文 P1 表示影响相应应用场景的重要工作，P2 表示增强或优化；不把所有可选组件变体当成发布阻塞项。

## 1. 平台与宿主

### P1：Linux GPU 呈现尚未实现

- `uihost/gpu_other.go` 明确让非 macOS / Windows 平台返回 `errNoGPUPresenter`。
- 目前已有 Metal、D3D11 和 HLSL；没有 Linux GL 呈现器或 GLSL 输出后端。
- Linux 桌面自绘窗口可走 CPU，不能据此声称 Linux 没有 GUI；缺的是 GPU 路径。
- 证据：[gpu_other.go](../../uihost/gpu_other.go)、[gpu_windows.go](../../uihost/gpu_windows.go)、[shader](../../ui/shader)。

### P1：系统高对比度偏好没有自动接入

- 已有 `theme.HighContrast`、对比度审计以及手动设置主题的路径。
- 没有 `ContrastPreference` / `IncreaseContrast` / `AppIncreaseContrastChanged` 的平台实现，也没有 uihost 跟随逻辑。
- `ConfigProvider` 没有对应的便捷开关；应用目前可以显式传入高对比度主题。
- 明暗主题、强调色和减少动态效果的现有支持不能与这一缺口混淆。
- 证据：[accessibility.md](../accessibility.md)、[nav_config.go](../../ui/nav_config.go)、[uihost/motion.go](../../uihost/motion.go)。

### P1：桌面原生多点触摸尚未进入 UI 事件通道

- `ui/input.TouchEvent`、手势识别、指针接管和安全区都有 UI 层实现与测试。
- Windows Canvas 没有 WM_TOUCH / WM_POINTER 的触摸转发，Linux Canvas 也没有 GTK touch 事件转发。
- 触摸板滚轮和系统模拟鼠标不等于多指触摸支持。普通点击是否可用需要在实际触屏设备上验证。
- 证据：[touch-input.md](touch-input.md)、[Windows 输入](../../internal/platform/windows/canvas_input_windows.go)、[Linux 输入](../../internal/platform/linux/canvas_input_linux.go)。

### P1：热加载窗口没有无障碍桥接

- 正常 `uihost.Open` / Attach 窗口已有平台语义树桥接。
- `remoteWin` 没有 `AccessibilityHost` 实现，devhost 协议也没有传输语义树与辅助技术动作的消息。
- 因而 `tan dev` 的热加载模式不能代表发布窗口的无障碍行为；需要独立运行来验收。
- 证据：[remote_dev.go](../../uihost/remote_dev.go)、[a11y.go](../../uihost/a11y.go)、[a11y-bridge.md](a11y-bridge.md)。

### P2：CPU 脏矩形能力没有落到真实宿主

- Runner 能收集 Damage，也定义了 `DamagePresenter`，但真实的 uihost / 平台 Canvas 没有 `PresentDamage` 实现。
- CPU 渲染仍整帧光栅化、整帧上屏。这是性能优化缺口，不是所有平台都没有局部重绘：Metal / D3D11 的 GPU 路径另有局部帧支持。
- 证据：[window.go](../../ui/window.go)、[runner.go](../../ui/runner.go)、[uihost](../../uihost)。

## 2. 控件与业务交互

### P1：远程搜索组件的接口不统一

- `Select` / `MultiSelect` 已有 `Remote`、`Debounce`、`Loading`。
- `AutoComplete.Source` 仍是在构建时同步调用的 `func(string) []Option`，没有同等的异步结果、加载状态和防抖接口。直接在这里访问网络会阻塞当前 UI 帧；应用需自行在后台取数并更新状态。
- `Mentions` 的查询词解析是私有方法，没有公开的查询词回调。远程联想需要应用额外解析输入。
- 普通 `Select` / `MultiSelect` 还没有 `VirtualSelect.FilterMethod` 对应的自定义过滤方法。
- 证据：[entry_autocomplete.go](../../ui/entry_autocomplete.go)、[entry_mentions.go](../../ui/entry_mentions.go)、[p2b_select.go](../../ui/p2b_select.go)、[fill_virtual_select.go](../../ui/fill_virtual_select.go)。

### P2：部分高级交互仍然缺失

| 控件 | 确认的缺口 | 已有能力与边界 |
|---|---|---|
| Drawer | 用户拖动边缘调整宽高 | 有 `Size`，也有嵌套推开效果；不能再把 push 列为缺失 |
| Splitter | lazy 模式、独立的拖拽开始/结束事件 | 当前是拖动过程中持续修改比例，只有统一的 `Changed` |
| Carousel | 拖动/滑动切换、关闭循环的配置 | 有键盘切换、指示器、箭头、自动播放与 Card；本轮键盘审计没有失败 |
| SliderRange | 双柄交叉、pushable；范围/竖向输入框联动 | `limits` 将双柄互相夹住；`WithInput` 只在单值且横向时构建输入框 |
| ColorPicker | 渐变值、HSB/HSV 文本格式、多组预设 | 控件绑定单个颜色；`Format` 只接收 hex/rgb |
| Cascader | 独立的内嵌 CascaderPanel 入口 | 已有级联弹出选择器和懒加载，不是整个 Cascader 缺失 |

证据：[feedback.go](../../ui/feedback.go)、[nav_splitter.go](../../ui/nav_splitter.go)、[data_carousel.go](../../ui/data_carousel.go)、[p2b_slider.go](../../ui/p2b_slider.go)、[entry_color.go](../../ui/entry_color.go)、[entry_cascader.go](../../ui/entry_cascader.go)。这些是能力增强，不代表每个应用都需要实现。

### P2：货币格式仍有不一致

- `ProFormat(ValueMoney, ...)` 固定使用 `¥`，而 ProTable 等组件按当前 Locale 或显式 Currency 选择符号。
- 格式仍是“符号 + 千分位 + 两位小数”，没有按 ISO 货币代码决定精度、符号位置和地区格式。
- 这是多语言业务应用需要注意的数据展示缺口。
- 证据：[table_pro.go](../../ui/table_pro.go) 的 `ProFormat` / `proFormatMoney`、[p2e_currency.go](../../ui/p2e_currency.go)。

## 3. 代码编辑器：尚不能视为 Monaco 完整替代

目前已有文本模型、语法高亮、IME、多光标、行操作、缩进、注释、命令、快捷键、when 表达式、模型池和视图状态。

规格中的后续能力仍缺少相应实现：查找替换面板、折叠、小地图、自动换行、相对行号、语言 provider 与补全/悬停/诊断展示、JSON Schema、Diff、Vim、调试和通用装饰。`Options` 当前只覆盖字号、Tab、空格缩进、只读、绝对行号开关和注释设置。

这一组对普通 GUI 不是统一的 P1，但如果目标是接替 MinCode 的 Monaco，就是独立的关键项目。不能把已经完成的多光标、模型池和命令系统再次报成未实现。

证据：[编辑器规格](code-editor.md)、[options.go](../../ui/editor/options.go)、[editor 目录](../../ui/editor)。

## 4. 测试与验收缺口

### P1：无障碍审计的失败门槛不完整

- `TestGalleryAccessibility` 只断言交互节点缺名及对比度失败，没有断言 `AuditNotKeyboard` 或 `AuditNoRole`。
- `foreignPage` 仍按过去的工作流归属把 Pro2 / Pro3 / fill-in 页面分组；这些页面的缺名问题只记录，不进入 `missingNames` 的失败计数。
- 当前审计为零不表示存在正在复现的控件错误；这里的缺口是以后回归时测试可能仍然通过。
- 证据：[audit_test.go](../../examples/ui-gallery/audit_test.go)、[audit.go](../../examples/ui-gallery/audit.go)。

### 实机验收仍需补齐

- Windows D3D11 已有平台胶水和实现，不能再标记“缺 Windows GPU”。但仓库报告仍有 Windows 实机待验清单；本轮只交叉编译，没有在 Windows 上运行。
- Linux、跨屏 DPI、系统输入法、触屏、真实辅助技术等行为不能靠无头审计或交叉编译完全确认。
- 证据：[D3D11 报告](gpu-renderer-m3-report.md)、[Canvas 规格](canvas-surface.md)。

## 5. 旧报告中应关闭的遗漏

| 旧说法 | 当前代码证据 |
|---|---|
| Windows GPU 未实现 | `ui/gpu/d3d11`、`uihost/gpu_windows.go`、`windows/canvas_gpu_windows.go` |
| 没有 App、SettingDrawer、ProFormGroup、RouteContext/GridContent | `ui/pro4_app.go`、`pro4_setting.go`、`pro4_group.go`、`pro4_route.go` |
| Form 没有按字段校验、清除或整体 Disabled | `ui/p2d_form.go` 的 `ValidateFields`、`ClearValidate`、`Disabled` |
| Tree 没有自身虚拟视口 | `Tree.Height` / `Built`，见 `ui/data_tree.go` |
| TreeSelect 没有受控展开、勾选策略不能写入绑定值 | `ExpandedKeys` / `StrategyValue`；Cascader 同样已有 `StrategyValue` |
| RangePicker 不支持周/月/季/年 | 已有 `RangePicker.Picker`；日期控件还补有输入字段路径 |
| 响应式描述列表和列表网格都未接线 | `Descriptions.ColumnAt` / `Columns`、`List.GridAt` 等 |
| ProTable 没有外部参数、轮询和持久化列状态 | `Params`、`Polling`、`ColumnsState`、`PersistColumnsState` |
| Upload 没有自定义请求 | `Upload.CustomRequest` |
| Dropdown 不处理分组 | `nav_dropdown.go` 的 `Group` 分支与 `group` 渲染 |
| CarouselCard 有 12 处键盘不可达 | 本轮实际审计 not-keyboard 为 0 |
| Defaults 只有 33/158 接线 | Button 等已新增 Defaults，旧统计不能沿用；本轮没有重新计算完整覆盖率 |

## 6. 本轮验证记录

- `go test ./ui/... ./uihost/...`：通过。
- `go test -tags tango_dev ./uihost/... ./internal/devhost/...`：通过。
- `go vet ./ui/... ./uihost/...`：通过。
- Windows amd64、Linux amd64：`CGO_ENABLED=0 go build ./ui/... ./uihost/...` 通过。
- `go run ./examples/ui-gallery -audit`：645 次页面渲染，missing-name / no-role / not-keyboard 均为 0。
- 六个内置主题：0 fail、58 design-default、24 exempt；HighContrast 为 0 fail。DesignDefault 是审计规则允许保留、但低于相应 AA 门槛的默认设计颜色组合；不能把“0 fail”等同于所有默认颜色都达标。
- 这些是代码、无头渲染与编译检查，没有新增真实窗口截图或跨平台实机验收。

## 7. 建议顺序

1. 补齐无障碍测试失败门槛、系统高对比度跟随和远程搜索接口。
2. 完成 Windows 实机验收；按 Linux 使用场景投入 GPU 后端。
3. 补 Drawer / Splitter / Carousel 等实际应用需要的高级交互、统一货币格式。
4. 单独推进代码编辑器后续里程碑。

移动端自绘 Canvas 宿主作为独立的可选平台项目，不计入本次桌面 GUI 的必补项。（更正，2026-11-13 晚：本文写成时说它"仍未实现"，与代码不符。iOS / Android 自绘页已按 [mobile-canvas.md](mobile-canvas.md) 实现 M1–M5，即上屏、触摸、安全区、输入法、无障碍、Android GPU 与 WebView 叠放；Android 的 M2 在模拟器上跑过，其余里程碑没有实测记录，两端都缺真机验证。）

## 8. 处理状态（2026-11-13 同日跟进）

| 条目 | 状态 |
|---|---|
| 无障碍审计门槛 | 已修：`TestGalleryAccessibility` 与 `-audit` 同时以 no-role / not-keyboard 为失败条件；取消“其他工作流页面只记录”的豁免 |
| 系统高对比度跟随 | 已实现：`platform.ContrastPreference` + `AppContrastChanged`（macOS / Windows / Linux portal），`uihost.Options.Contrast`，`ConfigProvider.HighContrast`；Windows/Linux 仅编译验证 |
| 远程搜索接口 | 已实现：AutoComplete `Remote/Debounce/Loading`，Mentions `Remote/Debounce/Loading/Query`，Select / MultiSelect `FilterMethod`，共享 `ui/remote_search.go` |
| 热加载无障碍 | 已实现：devhost 协议 v2 传语义树（整树 + 增量）与 AT 动作；自动化接口在热加载下仍不支持 |
| CPU 脏矩形 | 已实现：`platform.CanvasDamagePresenter`，macOS 瓦片层、Windows 局部 BitBlt、Linux queue_draw_area（普通帧改在 frame clock tick 渲染）；Windows/Linux 待实机 |
| 桌面多点触摸 | 已实现：Windows WM_POINTER(PT_TOUCH)、Linux GTK touch-event；待实机 |
| Drawer / Splitter / Carousel / SliderRange / ColorPicker / CascaderPanel | 已实现；ColorPicker 渐变值未做；SliderRange 拖动交叉原本已有，现加 `AllowCross` 开关 |
| 货币格式 | 已修：`FormatMoney` / `ProFormatLocale`，ISO 4217 代码决定符号与精度，地区决定分隔符与符号位置；ja 语言包改为 JPY |
| Linux GPU | 已实现（提交 `25ba786`）：`ui/gpu/gl` OpenGL 3.3 设备、`GtkGLArea` 呈现器、`uihost/gpu_linux.go`，Linux 窗口默认走 GL；只在容器里的 llvmpipe/Xvfb 上验证，真实 GPU 驱动、Wayland、HiDPI 待测（见 [gpu-renderer-m4-report.md](gpu-renderer-m4-report.md)） |
| 编辑器后续里程碑、Windows 实机验收 | 未处理 |
