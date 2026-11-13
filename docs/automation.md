# 整机自动化测试

`ui.Driver` 在进程内无头地测试组件；整机自动化则驱动一个**真实运行中的 Tango 应用**：独立进程、真实窗口，自绘 UI 和 WebView 都能操作，类似 Playwright 之于浏览器。测试用 `go test` 写，客户端是 `github.com/tangoui/tango/automation` 包。

```go
func TestE2EAddRecord(t *testing.T) {
	app := automation.Launch(t, ".", "-data", filepath.Join(t.TempDir(), "ledger.json"))
	win := app.Window("记账本")

	win.Find(a11y.RoleButton, "记一笔").Click()
	win.Find(a11y.RoleSpinButton, "金额").SetValue("88.8")
	win.Find(a11y.RoleCombobox, "分类").Expand()
	win.Find(a11y.RoleOption, "交通").Click()
	win.Find(a11y.RoleTextbox, "备注").Type("打车回家")
	win.Find(a11y.RoleButton, "保存").Click()

	win.Find(a11y.RoleDialog, "记一笔").WaitGone()
	win.FindContains(a11y.RoleRow, "打车回家").Wait()
	win.SaveScreenshot(filepath.Join(t.TempDir(), "ledger.png"))
}
```

完整示例：`examples/ledger/e2e_test.go`（自绘）、`examples/hello/e2e_test.go`（WebView）。

## 运行

```sh
TANGO_E2E=1 go test -tags tango_dev ./examples/ledger -run E2E -v
TANGO_E2E=1 TANGO_E2E_VERBOSE=1 go test -tags tango_dev ./examples/hello -run E2E -v   # 同时把被测程序输出打到终端
```

被测程序由 Launch 自己用 `-tags tango_dev` 构建，所以测试命令本身加不加 `-tags tango_dev` 都可以。`automation.Launch` 在没有设置 `TANGO_E2E` 或带 `-short` 时 `t.Skip`，所以 `go test ./...` 默认不会弹窗口，e2e 测试照常参与编译和 `go vet`。

`Launch(t, target, args...)`：

1. `target` 是 Go 包（`"."`、`"./examples/ledger"`、导入路径）时先 `go build -tags tango_dev` 到临时目录（开发构建；`Options.BuildFlags` 里已有 `-tags` 时合并进去）；是可执行文件路径时直接运行，该文件必须用 `-tags tango_dev` 构建，否则 Launch 直接报错 "has no automation channel: build it with -tags tango_dev"（程序启动后未开启通道时的超时/退出错误也会提示这一点）。
2. 以 `TANGO_AUTOMATION=127.0.0.1:0`、`TANGO_AUTOMATION_TOKEN_FILE=<临时目录>/automation.json` 启动，等应用写出 token 文件后连接。
3. `t.Cleanup` 里请求应用退出，5 秒不退就杀进程；测试失败时把被测程序的输出（最后 256 KiB）写进测试日志。

已经在运行的应用用 `automation.Connect(t, addr, token)` 连接（地址与 token 见下文）。

## 应用侧：启用条件与安全

控制通道只有**同时满足**以下条件才会开启：

1. **开发构建**（`-tags tango_dev`，`tan dev` 与 `automation.Launch` 自动加）。默认构建就是生产构建——普通 `go build`、`go run`、goreleaser、`tan build`（含 `-debug`）都是——整套代码编译掉：`internal/devfeat/release.go` 与 `uihost/automation_release.go` 是空实现，`internal/automation` 不会被链接进二进制（`automation` 包的 `TestReleaseBuildHasNoChannel` 用 `go list -deps` 检查；`internal/devfeat` 的 `TestAutomationReleaseOff` 在默认构建下验证设置了变量也不监听）。与应用同用户的进程可以随意设置环境变量，所以这里不能靠运行时检查。见 [开发功能与生产构建](dev-builds.md)。
2. **设置了 `TANGO_AUTOMATION`**，值为本机回环地址：`127.0.0.1:端口`、`localhost:端口`、`[::1]:端口`（端口 0 自动选择），或 unix socket `unix:/path/app.sock`。`0.0.0.0`、局域网地址、域名一律拒绝（打印一行日志，不开启）。unix socket 创建后权限设为 0600。
3. **token**。每次启动生成 256 位随机 token。设置了 `TANGO_AUTOMATION_TOKEN_FILE` 时，地址与 token 以 JSON（`{"addr","token","pid","version"}`）原子地写入该文件（0600），退出时删除；否则打印到 stderr：`tango: automation listening on tcp:127.0.0.1:54321 token <hex>`。每个连接的第一条请求必须是带正确 token 的 `hello`（常量时间比较），否则回一条 `auth` 错误并断开；5 秒内不发完整的 `hello` 也断开。鉴权前单帧上限 4 KiB（`MaxHelloFrame`），帧头声明更大就直接断开、不分配缓冲；同时最多保持 4 个连接（`MaxConns`，含未鉴权的），超出的新连接立即关闭。

其他：

- 读取后两个环境变量会从进程环境里删除，被测应用启动的子进程（包括其他 Tango 应用）不会继承。
- 自动化处理中的 panic 走统一的 panic 处理（`Panic.Where = "automation"`，见 [panics.md](panics.md)），开发构建默认记日志后继续运行，对应请求返回 `internal` 错误，连接不断。被测应用自己设置的 `Options.OnPanic` 返回 false 时照常退出。
- 所有对 runner、surface 的操作都在主线程执行（`App.Post`/`Await`）。

## 架构与协议

```
go test 进程                          被测应用（开发构建）
automation（客户端）  ── TCP/unix ──  internal/automation.Server（连接 goroutine：鉴权、等待/轮询）
                       长度前缀 JSON      │ Host（internal/devfeat/automation.go）：窗口列表、Eval、WebView 截图、退出
                                          │ Canvas（uihost/automation_dev.go，每个自绘窗口一个）：
                                          │   语义树（a11y.go 的 tap + force）、PerformAction、输入、截图
```

- 帧格式：4 字节大端长度 + JSON，鉴权后单帧上限 64 MiB（鉴权前 4 KiB）。客户端发 `Request{id, op, ...}`，服务端按顺序逐条回 `Response{id, error, code, ...}`。一个连接上请求串行处理；可以同时有多个连接（最多 4 个）。
- 操作：`hello`、`ping`、`windows`、`tree`、`find`、`count`、`wait`、`action`、`input`、`eval`、`screenshot`、`quit`。错误码：`auth`、`bad_request`、`no_window`、`timeout`、`hot_reload`、`unsupported`、`action`、`script`、`quit`、`internal`。定义见 `internal/automation/proto.go`。
- 根包在 `New` 时（`automationAttach`）读取变量，`OnReady` 时开始监听，应用退出时关闭；`uihost.Open` 为每个窗口注册自绘适配器，窗口关闭时注销。

### 自绘窗口：语义树

语义树来自窗口的无障碍桥（`uihost/a11y.go`，[specs/a11y-bridge.md](specs/a11y-bridge.md)）：自动化第一次访问某个窗口时订阅（`tap`），并以引用计数强制建树（`force`），因为没有真正的辅助技术来请求它；平台桥接在没有辅助技术时不会因此收到任何调用。窗口关闭时释放。动作在 UI 线程调用 `Runner.PerformAction`，与屏幕阅读器走同一条路，下一帧由组件处理。

**节点查找**（`Query`，服务端匹配，可单测）：

| 字段 | 含义 |
|---|---|
| `Role` | `a11y.Role` 名称（`button`、`textbox`…）；`RoleNone` / 空 = 任意 |
| `Name` + `Match` | 精确（默认）、`contains`（不区分大小写的子串）、`regexp`（Go 正则，不锚定） |
| `Nth` | 按阅读顺序（从根深度优先）的第 n 个匹配；负数从末尾数，-1 为最后一个 |
| `States` | 必须具备的状态名（`checked`），`!` 前缀表示必须不具备（`!disabled`） |
| `Value` | `Node.Value` 必须相等 |

`Locator` 是惰性的：每次使用都在当前树里重新查找，并等待最多超时时间（默认 10 秒，`app.SetTimeout` / `Locator.Timeout` 调整），所以不必关心重渲染和动画。动作（`Click`、`SetValue`…）和坐标输入返回前会等到处理它的那一帧跑完（树有变化，最多 400ms），紧接着读状态不会读到旧树；异步结果用 `Wait*` 等。

坐标是窗口内容区左上角起的逻辑像素，与 `Node.Bounds` 相同。坐标输入经 uihost 的 host 送进 runner（窗口快捷键先处理，和平台输入一样），不经过操作系统的事件队列。

截图把最后一帧的显示列表在 CPU 上重新绘制成 PNG（物理像素），CPU/GPU 渲染路径结果一致，窗口被遮挡或最小化也能截。

### WebView 窗口

`Eval(expr)` 把表达式嵌进一段包装脚本，经 `Surface.Eval` 执行，结果（Promise 会被 await，值 `JSON.stringify`）通过内部消息通道 `\x00tango-int:automation:` 回传。表达式以源码形式嵌入而非交给 `eval()`，页面 CSP 不含 `unsafe-eval` 也能用。页面还没加载完（`OnLoaded` 之前）时先等加载完成。要求页面有 Tango 桥（应用自己的资源页，或核心导航过且受信任的源）；语法错误表现为超时，错误信息会提示。

选择器辅助都是在页面里跑的 JS：`WaitSelector`、`WaitJS`、`ClickSelector`（等待、滚动到可见、`click()`）、`Fill`（用原生 setter 设值并派发 `input`/`change`，React/Vue 能感知）、`TextOf`。截图用 `Surface.Screenshot`。

## API

```go
func Launch(t testing.TB, target string, args ...string) *App
func LaunchOptions(t testing.TB, target string, opts Options) *App
func Connect(t testing.TB, addr, token string) *App

type Options struct {
	Args, Env, BuildFlags []string
	Dir                   string
	Timeout, StartTimeout time.Duration
	Addr                  string    // 默认 "127.0.0.1:0"
	Output                io.Writer // 另外接收被测程序输出
}

func (a *App) Window(title string) *Window
func (a *App) WindowMatching(what string, match func(WindowInfo) bool) *Window
func (a *App) Windows() []WindowInfo
func (a *App) SetTimeout(d time.Duration)
func (a *App) Quit()
func (a *App) Output() string
func (a *App) Exited() bool

// 自绘窗口：语义树
func (w *Window) Find(role a11y.Role, name string) *Locator
func (w *Window) FindContains(role a11y.Role, sub string) *Locator
func (w *Window) FindRegexp(role a11y.Role, pattern string) *Locator
func (w *Window) Locate(q Query) *Locator
func (w *Window) WaitFor(l *Locator) a11y.Node
func (w *Window) WaitGone(l *Locator)
func (w *Window) Nodes() []a11y.Node
func (w *Window) Dump() string
// 自绘窗口：坐标输入
func (w *Window) Move(x, y float32)
func (w *Window) Click(x, y float32)
func (w *Window) DoubleClick(x, y float32)
func (w *Window) RightClick(x, y float32)
func (w *Window) Drag(x1, y1, x2, y2 float32)
func (w *Window) Scroll(x, y, dx, dy float32)
func (w *Window) Press(keys string) // "Enter"、"Cmd+N"、"CmdOrCtrl+Shift+Z"
func (w *Window) Type(text string)
// 两类窗口
func (w *Window) Screenshot() []byte
func (w *Window) SaveScreenshot(path string)
// WebView 窗口
func (w *Window) Eval(expr string) json.RawMessage
func (w *Window) EvalInto(expr string, v any)
func (w *Window) EvalString(expr string) string
func (w *Window) WaitJS(expr string)
func (w *Window) WaitSelector(sel string)
func (w *Window) ClickSelector(sel string)
func (w *Window) Fill(sel, text string)
func (w *Window) TextOf(sel string) string

// Locator
func (l *Locator) Nth(i int) *Locator      // First()、Last()
func (l *Locator) WithState(s a11y.State) *Locator
func (l *Locator) WithoutState(s a11y.State) *Locator
func (l *Locator) WithValue(v string) *Locator
func (l *Locator) Timeout(d time.Duration) *Locator
func (l *Locator) Node() a11y.Node         // Wait() 同义
func (l *Locator) WaitGone()
func (l *Locator) WaitState(s a11y.State) a11y.Node
func (l *Locator) WaitValue(v string) a11y.Node
func (l *Locator) Exists() bool            // 不等待
func (l *Locator) Count() int              // 不等待
func (l *Locator) All() []a11y.Node
func (l *Locator) Bounds() a11y.Rect       // Name()、Value()、Has(state)
func (l *Locator) Click()                  // 支持 Press 时走 a11y 动作，否则在中心点指针点击
func (l *Locator) ClickPointer()           // 强制指针点击（真实命中测试、悬停/按下态）
func (l *Locator) Perform(act a11y.Action, arg string) a11y.Node
func (l *Locator) Focus()
func (l *Locator) SetValue(v string)
func (l *Locator) Type(text string)        // 聚焦后输入文本
func (l *Locator) Expand()                 // Collapse、Increment、Decrement、Dismiss、ScrollIntoView
```

所有方法出错或等待超时都 `t.Fatal`，消息里带窗口、查询条件，找不到节点时附上当前语义树（`a11y.Tree.Dump` 格式），便于对照名称和角色。需在测试 goroutine 里调用。

写测试前想知道节点叫什么：`t.Log(win.Dump())`，或者先用 `ui.Driver` 的 `EnableAccessibility()` + `AccessibilityTree().Dump()` 在单测里看。

## 限制

- **热加载（`tan dev`）不支持自绘窗口**：窗口画在 dev host 进程里，worker 侧没有无障碍桥（`a11yFor` 为 nil），所有自绘操作返回 `hot_reload` 错误：“热加载模式（tan dev）不支持自动化自绘窗口，请用 go run 或 tan build 的开发构建”。dev host 进程本身不开控制通道。以后 devhost 协议转发语义树后再支持（见 a11y-bridge 规范）。
- `uihost.AttachWith` 叠在 WebView 上的自绘层目前不单独暴露；该窗口按 WebView 窗口处理。
- 坐标输入是合成到 runner 的事件，不经过系统输入法与系统事件队列；需要验证原生输入路径时用平台自身的 UI 自动化工具。
- 一个进程里只有第一个创建的 `App` 会读取环境变量（读取后即删除）。
- WebView 截图依赖平台的 `WebSnapshot` 能力，不支持的平台返回 `unsupported`。

## CI

- macOS：需要图形会话（已登录用户的桌面）；锁屏时 WebView 与 Metal 相关的项目可能失败，先用 `ioreg -n Root -d1 -a | grep CGSSessionScreenIsLocked` 确认。
- Linux：在 xvfb 下运行，例如 `TANGO_E2E=1 xvfb-run -a go test ./examples/ledger ./examples/hello -run E2E`（需要 GTK3 与 WebKitGTK 运行库）。
- Windows：交互式桌面会话中运行（服务账户的 session 0 没有可见桌面）。
- 慢机器调大等待：`app.SetTimeout(30*time.Second)`、`Options.StartTimeout`。首次 `go build` 不计入启动超时。
- 截图可以作为失败产物上传：失败时 `win.SaveScreenshot(filepath.Join(os.Getenv("ARTIFACTS"), t.Name()+".png"))`。
