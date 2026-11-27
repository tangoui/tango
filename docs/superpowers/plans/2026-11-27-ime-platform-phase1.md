# 输入法框架补齐 · 第 1 阶段（公开 API 与 macOS）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 敲定 `sys/ime` 新增的公开 API（修饰键事件、Key 新字段、模式、菜单、候选列表），并在 macOS 上完整实现；示例做到单击 Shift 切换中英文、显示模式、菜单里有"关于"。

**Architecture:** `internal/platform` 增加可选接口（`InputMethodModifiers`、`InputMethodModes`、`InputMethodMenus`、`InputMethodCandidates`），backend 没实现的就由 `sys/ime` 返回 ErrUnsupported 或按文档忽略。darwin 在 IMK 控制器上增加 `recognizedEvents:`、flagsChanged 处理、`menu`、模式切换。tan 新增顶层 `inputMethod` 配置并生成 `ComponentInputModeDict`。

**Tech Stack:** Go、purego/objc、InputMethodKit。

**Spec:** `docs/superpowers/specs/2026-11-27-ime-platform-design.md`（全文必读，尤其"实现者审查后补充的决定""用户回答""自审补充"三节）

## Global Constraints

- 只用 purego，不开 cgo、不加 .m 文件。
- 现有 `sys/ime` API 只增不改，现有测试和示例行为保持通过；`sys/ime` 仍是 experimental。
- 修饰键事件默认放行，只有 Handler 返回 true 才吞掉；Cmd/Ctrl 组合快捷键不受影响。
- 模式是全局一份，在 tan.json 预先声明；当前模式由 `ime.SetMode` 设置，系统侧切换时通过 `Handler.ModeChanged` 回调。
- 本阶段 Windows/Linux 不实现新能力：调用新 API 返回 ErrUnsupported（或按文档被忽略），按键、组字、上屏照旧可用。
- 提交作者 `tango <903300326@qq.com>`，不加 Co-Authored-By。每次提交前：`gofmt -l`（含带构建标签的文件）无输出；`GOOS=windows`、`GOOS=linux` 交叉 vet 改动的包；`go run ./internal/cmd/apitxt -check`；改了 cmd/tan 就用 GOWORK=off 单独编译和测试。
- 只在 worktree `/Users/admin/min/.tango-ime-platform` 工作，不推送、不合并。

## Review Focus

1. 组字中按 Shift+字母（打大写）不能被误判成"单击 Shift"：按下 Shift 后有别的键按下，松开时就不算单击。→ Task 4 的 `TestShiftTap*`。
2. 中文模式下按 Cmd+C、Ctrl+A：修饰键的按下和松开都要放行，快捷键照常生效。→ Task 3 的 flagsChanged 测试，加上 Task 4。
3. 打开大写锁定时，英文字母应原样上屏，不进入组字（示例的约定）。→ Task 4。
4. 用户在系统输入法菜单里直接选"英"模式：应用收到 ModeChanged，状态同步，不会和 `SetMode` 互相触发形成死循环。→ Task 3 和 Task 4。
5. 菜单项回调里 panic：宿主不崩，后续菜单照常工作。→ Task 1 的 `TestMenuPanic`。

---

### Task 1: 公开 API 与平台接口

**Files:** Modify `sys/ime/ime.go`（必要时拆出 `sys/ime/modes.go`、`sys/ime/menu.go`），`internal/platform/ext.go`；Test `sys/ime/ime_test.go`（扩展现有的假 backend）；更新 `api/go/sys_ime.txt`。

**Interfaces（Produces，后续任务使用；名称可微调但须在汇报中列出）：**

```go
// sys/ime
type Key struct { Code, Text string; Shift, Ctrl, Alt, Meta bool; CapsLock, Repeat bool }
type ModifierKey struct { Code string /* "ShiftLeft","ShiftRight","ControlLeft",…,"CapsLock" */; Down bool; CapsLock bool }
type Mode struct { ID, Label, Short string } // 来自 tan.json，运行时只读
type MenuItem struct { ID, Title string; Checked, Disabled, Separator bool }
type Candidate struct { Text, Comment string }
type Handler struct { /* 现有字段 */
	Modifier    func(s *Session, m ModifierKey) bool // 返回 true 吞掉
	ModeChanged func(mode string)                    // 系统侧切换模式时
	Menu        func() []MenuItem                    // 每次打开菜单时调用
	MenuClicked func(id string)
}
func SetMode(a *tango.App, id string) error   // ErrUnsupported / 未声明的 id 报错
func CurrentMode(a *tango.App) string
func (s *Session) SetCandidates(list []Candidate, selected int) // macOS/Windows：无操作
func SystemDrawsCandidates() bool                              // Linux 为 true，其余 false
```

平台侧可选接口（`internal/platform`）：`InputMethodModifiers`（sink 增加 `IMEModifier(session, IMEModifierKey) bool`）、`InputMethodModes{ IMESetMode(id string) error; IMECurrentMode() string }`（sink 增加 `IMEModeChanged(id)`）、`InputMethodMenus`（sink 增加 `IMEMenu() []IMEMenuItem` 和 `IMEMenuClicked(id)`）、`InputMethodCandidates{ IMESetCandidates(session, []IMECandidate, selected) }`。

- [ ] **Step 1:** 写测试（使用假 backend）。`TestModifierRouting`：sink.IMEModifier 到达 Handler.Modifier，返回值透传；没设 Modifier 时返回 false。`TestKeyNewFields`：CapsLock、Repeat 原样透传。`TestSetModeUnsupported`：backend 没实现 Modes 时 SetMode 返回 ErrUnsupported。`TestSetModeUnknown`：未声明的 id 报错。`TestModeChangedNoLoop`：sink.IMEModeChanged("en") 触发一次 ModeChanged，CurrentMode 为 "en"；在回调里调用 SetMode("en") 不会再次触发。`TestMenu`：IMEMenu 返回 Handler.Menu 的结果，IMEMenuClicked 到达 MenuClicked。`TestMenuPanic`：Menu 或 MenuClicked 中 panic 时不崩，返回空菜单，并报告 OnPanic。`TestSetCandidatesNoBackend`：backend 不支持候选时调用无副作用。
- [ ] **Step 2:** 运行 `go test ./sys/ime`，应为 FAIL。
- [ ] **Step 3:** 实现；回调同样走 `corehook.GuardNonFatal`，并补上对应的 `Panic.Where` 取值和 docs/panics.md。
- [ ] **Step 4:** 测试通过；apitxt、交叉 vet 通过。
- [ ] **Step 5:** 提交 `sys/ime: modifier keys, modes, menus and candidate lists (API)`。

### Task 2: tan.json 顶层 inputMethod 与模式表

**Files:** Modify `cmd/tan/config.go`、`cmd/tan/build.go`、`cmd/tan/tan.schema.json`、`docs/packaging.md`；Test `cmd/tan/build_inputmethod_test.go`。

**Interfaces:** Produces `type InputMethod struct { Connection, Language, Icon string; Modes []InputMode; Windows *IMEWindows; Linux *IMELinux }` 和 `type InputMode struct { ID, Label, Short, Icon string }`。旧的 `MacOS.InputMethod` 作为别名保留。运行时 Go 侧需要读到模式表：macOS 从 Info.plist 读取（不新增嵌入机制）。

- [ ] **Step 1:** 写测试。`TestInputMethodTopLevel`：顶层配置生成的 Info.plist 与旧写法的 IMK 键一致。`TestInputMethodAliasConflict`：两种写法同时出现时 validate 报错。`TestInputModesPlist`：modes 为 zh/en 时生成 `ComponentInputModeDict`，其中 `tsInputModeListKey` 下有 `<id>.zh` 和 `<id>.en`，每项含 `TISInputSourceID`、`TISIntendedLanguage`、`tsInputModeScriptKey`、`tsInputModeMenuIconFileKey`，`tsVisibleInputModeOrderedArrayKey` 的顺序与配置一致，并能通过 plutil -lint；同时写入自定义键 `TangoInputModes`（id、label、short 数组），供运行时读取。`TestInputModesIcons`：图标被拷进 Resources，缺少图标文件时报错。不配置 modes 时与现状一致。
- [ ] **Step 2:** 运行 `cd cmd/tan && GOWORK=off go test -run 'InputMethod|InputMode' .`，应为 FAIL。
- [ ] **Step 3:** 实现；schema 和 docs 同步。
- [ ] **Step 4:** 测试通过，现有 tan 测试全部通过。
- [ ] **Step 5:** 提交 `tan: top-level inputMethod with declared modes`。

### Task 3: macOS 实现

**Files:** Modify `internal/platform/darwin/inputmethod.go`、`inputmethod_keys.go`，可新建 `inputmethod_menu.go`、`inputmethod_modes.go`；Test `internal/platform/darwin/inputmethod_test.go`，以及在 smokeext 或一个原生 IMK 夹具测试里验证 menu 和模式（照之前 sys/ime 原生探针的做法，不切换系统输入源）。

做法：在 `TangoInputController` 上覆盖 `recognizedEvents:`，返回 KeyDown|FlagsChanged（1<<10 | 1<<12）。handleEvent 处理 type 12（flagsChanged）时，用现有的 `modifierKeyFromMac(code, flags)` 判断是哪个键、按下还是松开，再转成 KeyboardEvent.code 风格的名字。keyDown 时 Key.CapsLock 来自 flags 的 1<<16，Repeat 来自 `isARepeat`。覆盖 `menu` 方法，每次调用时从 sink 取菜单项，构造 NSMenu；菜单项的 target 是控制器，action 是新注册的 `tangoMenuItem:`，用 representedObject 或 tag 携带 id。模式：用 Info.plist 的 `TangoInputModes` 建立模式表；SetMode 时对当前活跃会话的 client 调用 `selectInputMode:`（参数为 TISInputSourceID）；系统侧切换时通过 `setValue:forTag:client:`（tag 为 kTSMDocumentInputModePropertyTag）得到新的模式 id，再回调 IMEModeChanged，自己发起的切换要去重。

- [ ] **Step 1:** 写纯函数测试。`TestIMEModifierFromFlags`：0x38 加 Shift 位得到 {ShiftLeft, Down:true}；0x38 无 Shift 位得到 {ShiftLeft, Down:false}；0x3C 得到 ShiftRight；0x39 带 CapsLock 位得到 {CapsLock, Down:true, CapsLock:true}；0x37 得到 MetaLeft。`TestRecognizedEventsMask`：等于 (1<<10)|(1<<12)。`TestModeIDMapping`：bundle "com.x.inputmethod.p" 加模式 "en" 得到 "com.x.inputmethod.p.en"，反向也能解析，非本输入法的 id 返回空。`TestKeyCapsRepeat`：flags 含 1<<16 时 CapsLock 为 true。原生夹具测试：menu 返回的 NSMenu 条目与 Handler.Menu 一致，触发菜单项后 MenuClicked 收到对应 id。
- [ ] **Step 2:** 运行，应为 FAIL。
- [ ] **Step 3:** 实现。
- [ ] **Step 4:** 测试通过；Windows/Linux 交叉 vet 通过（新接口在那两端不实现）。
- [ ] **Step 5:** 提交 `darwin: IME modifier keys, menus and input modes`。

### Task 4: 示例：单击 Shift 切换中英文、模式、菜单

**Files:** Modify `examples/inputmethod/main.go`、`examples/inputmethod/tan.json`（顶层 inputMethod，modes 为 zh "中文"/"中" 和 en "英文"/"英"，带图标），新建 `examples/inputmethod/shift.go` 和 `examples/inputmethod/shift_test.go`，必要时修改 candidates.go（有系统候选面板时不画自己的候选窗）。

**Interfaces:** Produces `type shiftTap struct{…}`，方法为 `Down(code string, now time.Time)`、`Key()`（有别的键按下时调用）、`Up(code string, now time.Time) bool`（返回是否构成一次单击）。阈值 500ms。

- [ ] **Step 1:** 写测试。`TestShiftTapSimple`：ShiftLeft 按下后 100ms 松开为 true。`TestShiftTapWithKey`：按下、中间有 Key()、再松开为 false。`TestShiftTapSlow`：600ms 后松开为 false。`TestShiftTapOtherModifier`：ShiftLeft 按下期间 ControlLeft 按下，松开为 false。`TestEnglishModePassesLetters`：en 模式下 handle 收到字母键，consumed 为 false。`TestCapsLockPassesLetters`：Key.CapsLock 为 true 时字母不进入组字。`TestToggleFlushes`：组字 "nihao" 时单击 Shift，原拼音 "nihao" 上屏并切到 en（搜狗的行为）。`TestModeChangedSync`：ModeChanged("en") 后示例状态为 en。菜单：Menu 返回 [切换中/英（Checked 表示当前模式）、分隔线、关于]，点击"关于"后有日志或回调记录（测试替身）。
- [ ] **Step 2:** 运行 `go test ./examples/inputmethod/...`，应为 FAIL。
- [ ] **Step 3:** 实现；按 `ime.SystemDrawsCandidates()` 决定用系统候选还是自绘候选窗。
- [ ] **Step 4:** 测试通过；`tan build` 产物的 Info.plist 含 ComponentInputModeDict。
- [ ] **Step 5:** 提交 `examples: shift toggles Chinese and English, modes and menu`。

### Task 5: macOS 真机验收（必须先问用户）

开始前停下来，征得用户同意，并请用户在测试期间不要打字；不同意就标为"待验收"。不要用 cua-driver 的 type_text（它绕过输入法），必须让文本编辑到前台、发真实按键。

- [ ] **Step 1:** 安装测试输入法，并启用 zh 模式（模式的输入源 id 为 `<bundle>.zh`；register 之后 select 可能返回 -50，需要重试）。在文本编辑里：输入 `ni`，单击 Shift，"ni" 上屏且菜单栏图标变为"英"；输入 `abc` 原样上屏；再单击 Shift 回到"中"；按 Shift+A 得到 "A" 而且不切换模式；Cmd+A 照常全选。打开菜单栏的输入法菜单，看到"切换中/英""关于"，点"关于"后应用收到回调（看日志）。在系统输入法菜单里直接选"英"，示例的状态跟着变。
- [ ] **Step 2:** 证据存到 `/Users/admin/min/outputs/tango-ime-platform/phase1/`。
- [ ] **Step 3:** 收尾：切回原输入源，停用并删除测试包，关闭文档（不保存），核对原输入源已恢复。
