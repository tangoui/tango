# 默认更新界面 `service/updater/updateui` 设计

日期：2026-11-14。来源：Tango 与 Wails v3 / MyGo / Viva 的功能对比，第一批第 1 项。

## 意图

Wails v3 自带可换主题、可替换的默认更新界面，MyGo 自带 Sparkle 风格的更新窗口。Tango 的 `service/updater` 已经能检查、下载（补丁优先）、校验、安装、回滚，但界面要应用自己从 `OnEvent` 事件拼。目标：**应用开发者一行代码接上一个现成的更新窗口**，WebView 应用和 Go 自绘应用都能用。

## 目标

- 一行接入：`updateui.Start(app, u, updateui.Options{})` 之后，后台定时检查，发现新版本就弹出更新窗口。
- 手动检查：`updateui.CheckNow(app, ...)` 给"检查更新…"菜单项用，这时"已是最新"也要显示。
- 界面跟随应用：主题（Ant / Element / shadcn）、深色模式、语言都和应用的其他自绘窗口一致。
- 状态可记忆："跳过此版本"和上次检查时间跨重启保存。

## 非目标

- 不改 `service/updater` 的现有 API 与行为（它是稳定层）。
- 不替代 jsapi：WebView 应用想要和自己前端完全一致的更新页面，继续用 `tango:updater.*`。
- 不做移动端（`Apply` 在移动端本来就返回 `ErrUnsupported`；应用商店负责更新）。
- 不做"强制更新 / 阻止使用"的模式。

## 反目标（技术上能跑也算失败）

- 后台自动检查时，没有新版本也弹窗打扰用户。
- 稳定包 `service/updater` 依赖了实验性的 `ui` / `uihost`（违反 API 分层，CI 会报错）。
- 用户点了"跳过此版本"后，同一版本又被自动弹出来。
- 下载完成后不经用户同意就退出应用。

## 约束

- API 分层：`ui`、`uihost` 是 split / 实验性层，`service/...` 是稳定层。新包放在 `service/updater/updateui`，在 `api/tiers.txt` 里单独标为 `experimental`（精确匹配优先于 `service/...`）。
- `Apply` 的语义：成功后应用必须尽快退出，辅助进程等应用退出再替换安装并启动新版本。
- 窗口用 `uihost.Open` 打开（在 `tan dev` 下进入开发宿主，其余情况开真实窗口）。
- 平台：只在 `Apply` 支持的桌面平台上显示安装相关按钮；`Check` 返回 `ErrUnsupported` 时界面显示原因。

## 做法（已选 A）

| 方案 | 说明 | 取舍 |
|---|---|---|
| **A. 新包 `updateui` + `uihost.Open` 自绘窗口**（选定） | 单独的小窗口，WebView 应用和自绘应用都能用；跟随应用主题与语言 | 新包依赖实验性 `ui`，自身标为实验性；`service/updater` 不动 |
| B. 默认界面放进 `service/updater` | 只需引入一个包 | 稳定包依赖实验性 `ui`，违反分层，排除 |
| C. 内置 HTML 页面 | 和 WebView 应用风格易统一 | 要打包网页资源和 WebView，纯自绘应用用不了；jsapi 已覆盖"自写网页"，排除 |

选 A 的理由：唯一同时守住分层、又对两类应用都适用的方案。

## 已定下的行为

### 流程（参照 Sparkle）

1. **发现新版本**：窗口显示新版本号、当前版本号、发布日期、更新说明（`Release.Notes`），按钮：**跳过此版本**、**稍后提醒**、**下载并安装**（主按钮）。
2. **下载中**：进度条（`EventProgress` 的 Done/Total；Total 未知时显示不确定进度），注明"增量更新"或"完整安装包"；按钮：**取消**（取消下载的 context）。补丁失败自动改下完整包时，界面上把说明改成完整包，不当错误处理（`EventPatchFailed`）。
3. **下载完成**：按钮：**立即重启**（`Apply` 后 `app.Quit()`）、**下次启动时安装**。后者**不在退出时安装**（用户决定）：只把已下载、已校验的 Payload 记到状态文件；下次 `Start` 发现它且版本仍比当前新时，立即弹出"更新已下载，现在重启安装吗？"（按钮：**立即重启**、**稍后**）。原因：所有退出路径上唯一可靠的钩子是 `OnLifecycle(Terminating)`（`tango.go:94`），它在进程退出前一刻于主线程执行，`Apply`（暂存、签名检查、启动辅助进程，`apply.go:49`）可能来不及做完。Payload 文件不在了或校验失败时，丢弃记录，回到"发现新版本"。
4. **出错**：显示错误原因（`EventError` / 返回的 error），按钮：**重试**、**关闭**。`ErrDowngrade`、签名校验失败这类安全相关错误要原样说明，不提供"忽略"。
5. **已是最新**：只在 `CheckNow` 时显示（"当前已是最新版本 x.y.z"），后台检查不弹。

### 默认值（我的决定）

- 后台检查间隔：**每天一次**（`Options.Interval`，默认 24h）；应用启动后延迟一小段时间（默认 30 秒）做第一次检查，避免拖慢启动。
- 发现新版本时**直接弹窗**（Sparkle 的默认做法），不先发系统通知。
- "跳过此版本"、"上次检查时间"、"已下载待安装的 Payload"存到 `updateui.json`，放在更新器的状态目录里。`stateDir()` 未导出且不改 updater 的 API，所以 updateui 照搬同一套规则：显式 `StateDir`，否则 `<用户配置目录>/tango-updater/<AppID，没有则可执行文件名（去扩展名）>`（`state.go:70-84`，含 `safeName`）。文件名不与 updater 自己的状态文件冲突。
- "稍后提醒"：本次运行不再自动弹出该版本，下次检查周期到了照常提醒。
- 窗口：固定大小的普通窗口（约 480×360，更新说明可滚动），标题"软件更新"/"Software Update"；同一时间只开一个，再次触发时把已有窗口拉到前面。
- 文字：内置 en、zh-CN、zh-TW、ja，与 `i18n.Builtin`（`i18n/i18n.go:27`）一致，按 `i18n.Locale()` 匹配，其余语言用英文；系统语言变化时经 `i18n.OnChange`（`i18n.go:199`）实时更新；应用可以通过 `Options.Text` 覆盖全部文字。
- 主题：默认跟随应用（`uihost.Options` 的系统跟随行为）；`Options` 可传入 `uihost.Options` 的主题相关字段来指定。

### API 形状（草案）

```go
package updateui

type Options struct {
    Interval   time.Duration // 后台检查间隔，0 = 24h；<0 关闭后台检查（只手动）
    FirstDelay time.Duration // 首次检查延迟，0 = 30s
    Window     uihost.Options // 主题、风格、语言等；Surface 的标题和尺寸由 updateui 设定
    Text       *Strings       // 覆盖界面文字，nil 用内置四种语言
}

// Start 先调用 u.Confirm()（新版本启动确认，见下），处理已下载待安装的
// 更新，然后开始后台定时检查，发现新版本时弹窗。配置缺失
// （ManifestURL、Version、PublicKey/PublicKeys）时返回错误，不启动。
// 返回 stop：停止后台检查并还原 u.OnEvent。
func Start(app *tango.App, u *updater.Updater, opts Options) (stop func(), err error)

// CheckNow 立即检查并显示结果（包括"已是最新"和错误）。
func CheckNow(app *tango.App, u *updater.Updater, opts Options)
```

`Updater.OnEvent` 若应用已设置，`updateui` 包一层：先调应用原来的回调，再更新界面；不覆盖应用的设置。每个 `*Updater` 只包一层（重复 `Start` 或单独调用 `CheckNow` 不会叠加），`stop()` 还原。

### 已决定的细节（审阅后补充）

- **`Confirm()` 由 `Start` 自动调用**（用户决定）：放在 `Start` 开头，新版本启动后不会因为应用忘了确认而被回滚。`Confirm` 报告 `StatusRolledBack` 时，界面提示"上次更新未能启动，已恢复到 x.y.z"。文档注明：需要"自检通过后再确认"的应用不要用 `Start`，或在 `Start` 前自己调用（重复确认无害，以实现时核对 `Confirm` 的幂等性为准）。
- **macOS "检查更新…"菜单**（用户决定）：`updateui` 不改应用菜单，只在文档里给出用 `sys/menu` 接 `CheckNow` 的示例。
- **并发**（我的决定）：`*Updater` 不是并发安全的（`updater.go:70`）。`updateui` 对每个 `*Updater` 串行化：有检查或下载在进行时，后台检查直接跳过；`CheckNow` 不再发起新检查，而是打开窗口显示正在进行的状态。与 jsapi 的 `busy`（`jsapi/updater.go:51`）思路一致。同一个 `*Updater` 同时交给 jsapi `InstallUpdater` 和 `updateui` 不支持，文档注明。
- **线程**（我的决定，代码已有现成做法）：界面状态放在加锁的结构里，`OnEvent`（下载 goroutine）更新后调 `Runner.Invalidate`（`ui/runner.go:134-142`，任意 goroutine 可调）；打开窗口、聚焦已有窗口经 `app.Post`（`loop.go:174`）切到主线程，`uihost.Open` 要求主线程（`uihost.go:74-77`）。同一 `App` 只有一个更新窗口，已存在时 `Surface.Show`/`Focus`（`surface.go:202-204`）。
- **配置缺失 / 错误**（我的决定）：`Start` 校验配置，缺失时返回错误。运行中，后台检查出错只记录、不弹窗；`CheckNow` 时显示错误。
- **安装按钮的平台限制**（我的决定）：`Apply` 是否支持只有调用时才知道（`apply.go:46-48`）。只在 `GOOS` 为 darwin / windows / linux 时显示"下载并安装"；在这些平台上 `Apply` 返回 `ErrUnsupported`（例如不是从 .app / AppImage / 安装目录运行）时，界面说明原因并提供"打开下载页面"（若 `Release` 有可用链接；否则只关闭）。
- **窗口与应用退出**：`QuitOnLastClose` 下，用户关掉主窗口后应用可能退出，更新窗口随之关闭（`lifecycle.go:188`）。这是可接受的：更新窗口不阻止应用退出。

## 留给实现者

- `Check` / `Download` 的 context 与超时（`Check` 阻塞并接受 ctx，`updater.go:198`）。
- 下载取消后临时文件的清理（`Download` 的 `dir` 为空时会建临时目录，`updater.go:303`）。
- Payload 记录的序列化格式（`Payload` 字段可直接序列化，`updater.go:178`）。
- 测试：用 `ui.Driver` 无头测试各个界面状态；用本地 HTTP 服务器 + 测试密钥测完整流程（参照 `service/updater` 现有测试与 `internal/e2e` 演练）。
- 文档：`docs/updates.md` 增加"默认更新界面"一节；`examples` 里找一个接入示例（`examples/hello` 目前没接 updater）。
