# 更新日志

本文件记录 Tango 各版本的用户可见变化，格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。Go 模块、`cmd/tan` 与 `@tangoui/runtime` 使用同一个版本号，见 [docs/release.md](docs/release.md)。

## [未发布]

### 新增

- **完整图标库**：新增 `ui/icon/element`（Element Plus 293 个）、`ui/icon/antd`（Ant Design 848 个，线框 / 实底 / 双色）、`ui/icon/lucide`（Lucide 2052 个及 300 个旧名别名，shadcn/ui 默认图标），由 `internal/cmd/icongen` 从官方 npm 包生成；图标是静态初始化的变量，程序只链接实际引用到的图标，调用各库的 `Register` 后可用 `icon.ByName("lucide:search")` 按名查找。
- **主题图标**：`theme.Theme.Icons`（`icon.Style`）让组件画出设计体系自己的图标：Ant Design 主题用 @ant-design/icons，Element Plus 主题用 Element Plus 图标，shadcn 主题用 Lucide 并把线宽改为 2；应用里用到的内置图标也会跟着替换。
- `ui/icon`：图标改为由多段 `Part`（填充 / 描边、主色 / 辅色）组成，支持任意 viewBox（`Box`）、自定义线宽、双色图标（`DrawOpts.Secondary`）、绘制时覆盖线宽（`DrawOpts.StrokeWidth`）、位图与单色蒙版图标（`FromImage`）、`Register` / `RegisterAs` 注册自定义图标；内置图标从 85 个增加到 136 个（调试、编辑、Git、窗口按钮等常用图标）。
- `ui/icon/filetype`：按文件名或扩展名解析文件类别并给出图标，三套设计体系各有一组；`filetype.Theme` 按 VS Code 文件图标主题的方式（文件名、扩展名、文件夹名）叠加自定义或插件提供的图标。
- **公开 API 分层与兼容性检查**：Go 包分为稳定与实验性两层（`ui`、`uihost`、`mobile`、`service/updater` 为实验性）；CI 新增 `API` 工作流，用 `scripts/apidiff.sh`（基于 apidiff）检查稳定包的不兼容变化，需要破坏兼容时给 PR 加 `api-break` 标签。见 [docs/release.md](docs/release.md#api-稳定性)。

- **组件补齐**（对照 Ant Design v6、Element Plus、Pro Components 与 shadcn/ui 的"部分"项，见 [docs/specs/ui-coverage.md](docs/specs/ui-coverage.md)）：
  - 新组件 `Menubar`（应用菜单栏）、`AspectRatio`、`Item`/`ItemGroup`、`ButtonGroup`、`MultiDatePicker`、`ColorPickerGradient`/`ColorPickerPanelGradient`、`StatisticTimer`、`ProFieldOf`。
  - Button 颜色 × 样式（`Color`/`Appearance`，13 个预设色）与 Element 的 success/warning/info/plain；Steps inline/panel 与自定义进度点；Segmented 竖向与胶囊形；Timeline 横向。
  - DatePicker `NeedConfirm`、按日期禁用时间（`DisabledTimes`）；DateTimePicker 禁用日期与快捷选项；Calendar `Range`。
  - Switch 任意值（`SwitchValue`）、切换前确认（同步/异步）、内嵌图标、外侧文字、大号与宽度；Checkbox/Radio 边框与按钮样式、尺寸，CheckboxGroup 最少/最多与整组禁用；InputTag 自定义分隔符、拖动排序、失焦保存。
  - AutoComplete/Mentions/Select/Cascader 的 `Variant`（`FieldVariant`）；Select 前缀与后缀图标；AutoComplete 聚焦触发；Mentions 清除与整体删除；Cascader 自定义搜索与显示；TreeSelect 父子不联动与自定义搜索。
  - Upload 图片列表与手动提交；Card 内页签；Countdown `OnChange`；Tour 每步遮罩/类型/按钮与自定义指示器、操作区，切步自动滚到目标；Alert 深色效果与居中；Message 位置、偏移、关闭按钮与朴素样式；MessageBox 区分取消与关闭、居中、可拖动、自定义图标；Pagination 自由排序（`Layout`）与页码数量。
  - ProForm 只读模式；ProField 单独渲染与读写切换；新增 13 种 valueType；SchemaForm `layoutType` 与列表字段；ProLayout 自动面包屑与各渲染钩子、页脚链接、固定侧栏；ProCard 底部操作栏。

### 变更

冻结 API 前的一次性不兼容调整（[docs/specs/api-stability.md](docs/specs/api-stability.md) Phase 3，规则见 [docs/release.md](docs/release.md#api-稳定性)）。

- **结构体不可比较（R4）**：下列结构体的第一个字段改为 `_ [0]func()`，不能再用 `==` 比较，也不能写无键字面量（`T{a, b}` 改成 `T{Field: a, ...}`）。以后给它们加字段不再算破坏兼容。
  - 稳定层的 Options：`tango.SurfaceOptions`、`tango.WebViewOptions`、`asset.Options`、`asset.DevProxyOptions`、`jsapi.Options`、`service/fileserver.Options`、`service/log.Options`、`sys/biometric.Options`、`sys/flyout.Options`、`sys/location.Options`、`sys/media.PickOptions`、`sys/print.Options`（含有 `Options` 字段的 `service/fileserver.Server` 也随之不可比较）。
  - `ui` 的全部 `*W` 构建器与句柄（`Box`、`Context`、`Runner`、`Driver`、`Label`、`ScrollBox`、`AppAPI`、`ProSubmitter`、`TableFilterPanel`、`MessageRef`、`NotificationRef`）、`Locale` 及全部 `*Locale` 子结构，以及 `DriverOptions`、`WindowOptions`、`OverlayOptions`、`ContextMenuOptions`、`AppMessageConfig`、`AppNotificationConfig`、`ProSettings`、`Option`、`TabItem`、`StepItem`、`TransferItem`、`DatePreset`、`CheckCardOption`。
- **`ui` 构造函数返回专属类型（R2）**：
  - `Breadcrumb` 返回 `*BreadcrumbW`（原来返回 `int`）：`i := ui.Breadcrumb(c, ...)` 改成 `i := ui.Breadcrumb(c, ...).Clicked()`。
  - `CheckboxGroup` 返回 `*CheckboxGroupW`，`CheckAll` 返回 `*CheckAllW`（原来返回 `bool`）：`if ui.CheckboxGroup(...)` 改成 `if ui.CheckboxGroup(...).Changed()`。
  - `CardAction` 返回 `*CardActionW`（原来返回 `bool`），改用 `.Clicked()`；`CardMeta` 返回 `*CardMetaW`（原来无返回值）。
  - `Sider` 返回 `*SiderBoxW`，`Kbd`、`BadgeRibbon`、`ListItemMeta`、`ListActions`、`AnchorSection` 分别返回 `*KbdW`、`*BadgeRibbonW`、`*ListItemMetaW`、`*ListActionsW`、`*AnchorSectionW`（原来都返回 `*Box`）。这些类型内嵌 `*Box`，链式调用 `.Bg(...)` 等照常可用；需要 `*Box` 值时取 `.Box` 字段。
- **`layout.Option` 改为不透明类型**：原来是 `func(*layout.Style)`，现在是结构体，只能用 `layout` 包的构造函数（`Grow`、`W`、`H`、`Pad`、`Gap`、`Abs`……）得到，构造函数名称和签名不变。`layout.Style`、`Node`、`Pos`、`Solve`、`SolveAt` 移到内部包 `ui/internal/flexbox`。自己写的 `func(s *layout.Style){...}` 选项改用构造函数；新增 `layout.ScrollX/ScrollY/NoStretch/Stretch`（实验性）覆盖 `ui/mobile` 原来直接改字段的用法。传 `nil` 作为选项的地方改成 `layout.Option{}`。
- **`theme.NumComps` 改为未导出**（R1：计数常量每加一个组件就变值）。`theme.Comp` 的值只追加，文档不再把它和 `Components` 的字段顺序绑定。
- **枚举只追加（R1）**：`ui`、`ui/a11y`、`ui/input`、`ui/layout`、`ui/paint`、`ui/theme` 的枚举类型注释写明"只追加、零值不变"，现有常量值不变。
- **`asset.DevProxy` 恢复单参数（R3）**：`DevProxy(target string) http.Handler`；带选项的用法改为新增的 `DevProxyWith(target string, opts DevProxyOptions) http.Handler`。`DevProxy(url, opts)` 改成 `DevProxyWith(url, opts)`。

### 修复

- `icon.CheckFilled`、`CloseFilled`、`InfoFilled`、`WarningFilled` 原来只画一个实心圆，现在中间的符号是镂空的。

### 安全

- **开发功能改为编译期开关 `tango_dev`**：默认构建（普通 `go build`/`go run`、goreleaser、`tan build` 含 `-debug`）即生产构建，不再响应 `TANGO_DEV_URL`、`TANGO_AUTOMATION*`、`TANGO_DEV_ROLE`、`TANGO_DEV_WINDOW_STATE` 等开发用环境变量，热加载宿主、自动化通道、开发代理不链接进二进制；未处理的 panic 默认弹框退出。`tan dev` 自动加 `-tags tango_dev`；`automation.Launch` 构建被测程序时也加。`tango_release` 标签不再起作用，仅作兼容保留。见 [docs/dev-builds.md](docs/dev-builds.md)。
- `asset.DevProxy` 默认只接受回环目标；要代理到其他主机用 `asset.DevProxyWith(target, asset.DevProxyOptions{AllowRemote: true})`。
- 资源源上 GET/HEAD 以外的跨源请求回 403（`Options.AllowCrossOriginAssets` 关闭）；macOS/iOS 后端用主文档 URL 补上 WebKit 缺失的 `Origin`。
- `tan dev` 热加载控制通道要求会话 token（`TANGO_DEV_TOKEN`），unix socket 权限 0600。

## [0.1.0] - 未发布

首个公开版本。0.x 期间公开 API 可能有不兼容的变化。

**验证状态**：桌面端只在 macOS 实机验证；Windows、Linux 后端已实现，通过交叉编译与 CI 冒烟，尚未在实机上系统验证。iOS、Android 为实验性，只在模拟器上验证。自绘 UI 为实验性，只在 macOS 上实机验证。

### 新增

- **核心运行时**（根包 `tango`）：`App` / `Surface`，主线程队列与 `Post`、`Await`、`AwaitAsync`、`TryAwaitAsync`；生命周期事件、统一的应用事件（深链、打开文件、主题与显示器变化、Dock 重新打开、通知点击）、用户退出拦截、关闭最后一个窗口时的平台默认行为。
- **桌面后端**：macOS（AppKit + WKWebView）、Windows（Win32 + WebView2）、Linux（GTK 3 + WebKitGTK 4.1），均不需要 cgo；资源通过自定义 scheme 提供，不开本地端口。
- **窗口**：位置、尺寸限制、状态、居中、外观（隐藏标题栏 / 无边框、透明、毛玻璃与 Mica、置顶、不透明度）、面板 / 模态 / 表单窗口、无边框拖动区域、文件拖入与拖出、缩放、打印与导出 PDF、截图、页内查找、Cookie 与网站数据、User-Agent 与请求头。
- **WebView 策略**：导航策略、页面权限请求、新窗口请求、下载、HTTP 认证的处理函数。
- **IPC**（`ipc`、`bind`）：调用、双向事件、可取消且带背压的流（JS 打开或 Go 打开），反射绑定；`tan generate bindings` 生成 TypeScript 客户端。
- **前端运行时** `@tangoui/runtime`：`call` / `on` / `emit` / `stream`，以及 `Window`、`App`、`Theme`、`Screens`、`Clipboard`、`Dialog`、`Notify`、`Shell`、`Menu`、`Hotkey`、`Events` 模块；Go 侧 `jsapi` 按能力白名单开放，默认只开窗口自身、右键菜单和只读信息。
- **安全**：只接受资源 origin 与核心导航目标 origin 的消息；`shell` 协议白名单；`service/fileserver` 限制在目录内。
- **系统能力**（`sys/`）：剪贴板（文本、图片、文件）、对话框、菜单（应用、窗口、右键）、托盘、通知、全局与窗口快捷键、显示器、系统主题与页面同步、打开 URL 与文件、应用控制（徽标、进度、请求注意、Dock 图标）、开机启动、运行环境、最近文档、单实例、窗口状态记忆。
- **移动端**（实验性）：iOS（c-archive + 原生宿主）与 Android（c-shared + Java 宿主），用户代码零改动；权限、分享、触感、安全区、屏幕方向、生物识别、安全存储、相册与拍照、定位、远程推送（APNs；FCM 需自行接入）。
- **服务**（`service/`）：服务生命周期、应用目录、键值存储、日志、文件服务；签名的自动更新（`service/updater`：防降级、二进制补丁、启动确认与回滚，实验性）。
- **多语言**（`i18n`）：系统语言、语言匹配（简繁分流）、带具名参数与 CLDR 复数的文案目录；原生菜单标准项和对话框按钮支持 en、zh-CN、zh-TW、ja。
- **自绘 UI**（实验性，`ui`、`uihost`）：即时模式 API、Ant Design 与 Element Plus 两套主题令牌、约 90 个组件、无头 `Driver` 测试、`tan dev` 热加载；macOS 上 WebView 与自绘同窗（`uihost.Attach`）。
- **命令行 `tan`**：`init`（多种模板）、`dev`、`build`（桌面、iOS、Android）、`run`、`package`（dmg、NSIS、MSIX、tar.gz、AppImage）、`sign`（macOS 签名与公证、Windows Authenticode）、`keygen`、`release-manifest`、`sign-update`（更新清单与签名）、`generate bindings`、`doctor`。
- **示例**：`hello`、`runtime-demo`、`showcase`（IPC、流、系统能力、托盘、菜单、通知、主题跟随，带 `-smoke` 自检）等。
- **工程**：CI（lint、三平台测试与真实窗口冒烟、iOS 模拟器与 Android 模拟器冒烟）、文档站（VitePress，中文为主、英文首页与核心页面）。

[未发布]: https://github.com/tangoui/tango/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tangoui/tango/releases/tag/v0.1.0
