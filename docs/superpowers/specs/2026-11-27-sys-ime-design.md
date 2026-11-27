# sys/ime：用 Tango 写系统输入法（第一期：macOS 宿主）

日期：2026-11-27 · 分支 `feat/sys-ime`

## 目的

让 Tango 应用可以直接当作系统输入法运行。系统把用户在任意应用里的按键交给 Tango 应用，应用用 Go 决定显示什么组字（未上屏文字）、提交什么文字。拼音引擎、候选窗、设置界面都由应用自己写；Tango 只负责对接操作系统的输入法框架，以及打出符合要求的安装包。

用户的长期目标是用 Tango 写一个中文输入法。第一期先在 macOS 上把"系统 ↔ Go"这条链路做成公开、可复用的 API。

## 目标

- 新增公开包 `github.com/tangoui/tango/sys/ime`，在 `api/tiers.txt` 中登记为 `experimental`。
- macOS 通过 InputMethodKit 实现：在运行时注册 `IMKInputController` 子类，启动 `IMKServer`，每个输入会话对应一个 `*ime.Session`。
- `tan build` 根据 tan.json 的 `macos.inputMethod` 打出输入法包：往 Info.plist 写入 IMK 所需的键和 `LSBackgroundOnly`，并校验 id 中含 `.inputmethod.`。
- Tango 启动时不再强制把带 `LSBackgroundOnly` / `LSUIElement` 的应用设成普通 Dock 应用。
- 提供示例 `examples/inputmethod`（玩具拼音引擎），在真实系统里验证整条链路：文本编辑中输入 `nihao` 出现组字，空格上屏"你好"，Esc 取消，退格删字母，失焦时提交。

## 非目标（后续期）

- Windows TSF、Linux fcitx5/ibus、Android/iOS 键盘：其他平台的 `Serve` 返回 `ErrUnsupported`。
- 候选窗。本期提供 `Session.Caret()` 给出光标位置；"不激活、不抢焦点"的浮窗能力另开一期补到 Tango 窗口里。
- 多输入模式（`ComponentInputModeDict`）、真正的拼音引擎和词库。
- `tan dev` 热重载。输入法必须由系统从 `~/Library/Input Methods` 启动。

## 反目标

- 不能让宿主应用卡死或崩溃：Go 回调里的 panic 必须用 Tango 现有的 guard 接住，按键当作"未处理"放行。
- 不改变现有非输入法应用的任何行为，包括激活策略和 Info.plist。

## 约束

- darwin 后端全部基于 purego，不加 .m 文件，不要求 cgo。
- IMK 的回调都在主线程上。Session 的方法在主线程直接执行，在其他 goroutine 调用时转到主线程。
- 包 id 必须含 `.inputmethod.`，否则系统不加载。Info.plist 里 `InputMethodServerControllerClass` 必须跟运行时注册的类名一致：固定为 `TangoInputController`。
- 默认连接名为 `<id>_Connection`。tan 写入的值和 `ime.Serve` 默认用的值要一致；两边都可以覆盖，但必须相同。

## 思路比较

1. **公开包 `sys/ime` + 平台接口 `platform.InputMethods`（选用）**：跟 hotkey、notify 等现有能力的结构一致，之后接 Windows、Linux 只需要再实现同一个接口。缺点是公开 API 需要谨慎设计，所以先标为 experimental。
2. 放在应用侧的独立 Go 包里，直接用 purego：起步最快，但用户明确要求做成 Tango 公开 API；而且应用侧改不了 Tango 的激活策略，也改不了 tan 的打包。
3. 应用侧注入一段 Objective-C（cgo）：违背 Tango"纯 purego、免 C 工具链"的约束。

## 公开 API（第一期）

```go
package ime

var ErrUnsupported = platform.ErrUnsupported
var ErrServing     = errors.New("ime: already serving")

// Key 是一次按下。Code 用 KeyboardEvent.code 的命名（"KeyA"、"Digit1"、
// "Space"、"Enter"、"Backspace"、"Escape"、"Tab"、"ArrowLeft"……），
// 认不出时为 ""；Text 是这个键打出的字符（已计入 Shift），非字符键为 ""。
type Key struct {
	Code string
	Text string
	Shift, Ctrl, Alt, Meta bool
}

type Handler struct {
	Connection        string                       // 默认 "<app id>_Connection"
	Key               func(s *Session, k Key) bool // 必填；返回 true 表示吞掉这个键
	Activate          func(s *Session)             // 某个输入框开始使用本输入法
	Deactivate        func(s *Session)             // 失焦或切走：在这里提交或丢弃组字
	CommitComposition func(s *Session)             // 系统要求立即结束组字
}

func Serve(a *tango.App, h Handler) error // 在 OnReady 中调用

type Session struct{ /* … */ }
func (s *Session) ID() uint64
func (s *Session) ClientApp() string                 // 客户端应用的 bundle id
func (s *Session) SetComposition(text string, cursor int) // 组字；cursor 按 rune 计，"" 清除
func (s *Session) Commit(text string)                // 上屏，并清掉组字
func (s *Session) Caret() tango.Rect                 // 光标所在矩形（桌面坐标、逻辑像素、原点在左上），未知时为零值
```

不给 `Handler` 设 `CommitComposition` 时，不做任何事：组字状态由应用自己管理，Tango 不知道还有没有组字。

## tan.json

```json
"macos": {
  "inputMethod": {
    "connection": "可选，默认 <id>_Connection",
    "language": "zh-Hans",
    "icon": "可选，菜单栏图标 PNG；省略时用应用图标"
  }
}
```

有这一段时，`tan build` 额外写入：`InputMethodConnectionName`、`InputMethodServerControllerClass=TangoInputController`、`LSBackgroundOnly=true`、`TISIntendedLanguage`、`tsInputMethodCharacterRepertoireKey`（zh-Hans→Hans，zh-Hant→Hant，ja→Jpan，其他→Latn）、`tsInputMethodIconFileKey`。id 中没有 `.inputmethod.` 时报错。

安装方法写进文档：复制到 `~/Library/Input Methods/`，`killall <exe>` 让系统重新拉起，然后在"系统设置 → 键盘 → 输入法"中添加。

## 留给实现者

- 键码到 `Code` 的映射表：可以复用 darwin 现有的按键表，没有的话新写一份。
- 坐标翻转用后端现有的屏幕坐标工具。
- 验收用的冒烟脚本：用 TIS（Carbon）注册、启用、选中输入源，用 cua-driver 在后台往文本编辑发键，再读回文本内容。
