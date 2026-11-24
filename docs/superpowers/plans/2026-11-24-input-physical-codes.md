# 输入事件物理键码与逐事件指针 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让应用拿到每个按键的 USB HID 物理键码和平台原始键码，以及一帧内的每一个指针事件（带系统时间戳），用于远程桌面这类要原样转发键盘鼠标的应用。

**Architecture:** `input.KeyEvent` 新增 `HID`、`Native` 两个字段，`input.Pointer` 新增 `Time` 字段，零值表示未知，现有代码不受影响。macOS、Windows、Linux 三个桌面后端在各自的 `canvas_keys.go` 里加「平台键码 → HID」映射表并在构造事件时填写。`ui.Context` 新增 `PointerEvents()`，返回本帧按顺序到达的全部指针事件。

**Tech Stack:** Go；Tango `ui/input`、`ui`、`internal/platform/{darwin,windows,linux}`。

**Spec:** `/Users/admin/min/mmdesk/docs/superpowers/specs/2026-11-24-mmdesk-spike-design.md` 第 6.1 节。

## Global Constraints

- 只新增字段和函数，不改现有字段语义；零值 = 未知；`api/go/ui_input.txt`、`api/go/ui.txt` 用 `go run ./internal/cmd/apitxt -w` 更新，`-check` 通过；ui/input 是 split 包，新增导出项同时加进 `api/stable/ui_input.txt`。
- HID 用法码取 USB HID Usage Tables 的 Keyboard/Keypad 页（0x07），只存用法码本身（`uint16`，如 A = 0x04，Enter = 0x28），不含页号。
- `Native`：macOS 为 NSEvent `keyCode`；Windows 为扫描码，扩展键加 0xE000 前缀（右 Ctrl = 0xE01D），Pause 为 0xE11D；Linux 为 XKB 键码（evdev + 8）。
- `Pointer.Time`：系统事件时间，单调时钟，原点任意（只用于同一来源事件之间求差）；macOS 取 NSEvent `timestamp`，Windows 取 `GetMessageTime`（毫秒），Linux 取 GDK 事件 `time`（毫秒）；拿不到时为 0。
- 〔执行中决定〕`Key` 没有名字但有 HID 用法码的键（F13–F24、ISO/JIS 额外键）改为以 `KeyUnknown` 上报，按下和松开按 HID 成对；没有 HID 的键仍丢弃。原因：三个后端此前都丢弃 `Key` 未知的键，只填字段达不到目的。代价：现有应用会多收到 `Key` 为未知的事件，按 `Key` 匹配的代码不受影响。
- 不关闭系统自身的鼠标合并（AppKit、Windows、GTK 的默认行为保持）；不捕获系统快捷键。
- 映射表事实可参考公开的键码对照资料（如 USB HID Usage Tables、各平台 SDK 头文件），实现按 Tango 风格自写。
- 推送前检查：`gofmt -l`（含带构建标签的文件）、`go vet ./...`、`GOOS=linux go vet ./...`、`GOOS=windows go vet ./...`、`go run ./internal/cmd/apitxt -check`、相关包测试。编译测试用 `nice -n 19`（本机有其他会话在做性能测量）。
- 提交作者 `tango <903300326@qq.com>`，不加 Co-Authored-By；不推送、不合并，除非用户要求。

## Review Focus

1. 按住修饰键再松开（左右 Shift/Ctrl/Alt/Cmd 分别按）时，按下和松开事件的 `HID` 必须相同且区分左右。macOS 的 `flagsChanged` 和 Windows 的右 Alt（AltGr 会伴随一个假的左 Ctrl）最容易错——Windows 的假左 Ctrl 不应带右 Ctrl 的码。
2. Windows 的 NumLock（扫描码 0x45 带扩展位）与 Pause（VK_PAUSE，扫描码 0x45 不带扩展位）、PrintScreen（0xE037）、小键盘 Enter（0xE01C）与主 Enter（0x1C）必须得到不同 HID。
3. ISO 键盘的额外键（macOS `kVK_ISO_Section` 0x0A、Windows 扫描码 0x56、Linux evdev 86）和 JIS 的英数/かな、¥、_ 键要有码。
4. `Runner` 把 `Key` 折叠成 `Base()`、在 `Code` 为 0 时补成 `Key`，这些处理不能丢掉 `HID`、`Native`。
5. 一帧内多个指针移动时，`PointerEvents()` 要按到达顺序全部返回、坐标已换算成逻辑像素（与 `Pointer()` 一致），下一帧清空；触摸模拟出的指针事件也在其中并带 `Source`。

---

### Task 1: 公共字段与 Context 读取接口

**Files:**
- Modify: `ui/input/input.go`（`Pointer` 约第 74 行、`KeyEvent` 约第 503 行）
- Modify: `ui/runner.go`（`processEvents` 约第 404 行起，帧结束清理处约第 400 行）
- Modify: `ui/context.go`（`Pointer()` 旁约第 570 行）
- Test: `ui/input_codes_test.go`（新建）
- Modify: `api/go/ui_input.txt`、`api/go/ui.txt`、`api/stable/ui_input.txt`（apitxt 生成/追加）

**Interfaces:**
- Produces:
  - `input.KeyEvent.HID uint16`、`input.KeyEvent.Native uint32`
  - `input.Pointer.Time time.Duration`
  - `func (c *Context) PointerEvents() []input.Pointer` —— 本帧全部指针事件（逻辑像素），按到达顺序；切片只在本帧有效。

- [ ] **Step 1: 写失败测试** `ui/input_codes_test.go`

```go
func TestKeyEventKeepsPhysicalCodes(t *testing.T) {
	d := NewDriver(/* 与现有 driver 测试相同的最小页面：一个获得焦点的 NativeView "nv" */)
	d.Inject(input.KeyEvent{Key: input.KeyNumpadEnter, Down: true, HID: 0x58, Native: 0xE01C})
	// 读 nv 的 EventsFor，断言 Key == input.KeyEnter（被 Base 折叠）、
	// Code == input.KeyNumpadEnter、HID == 0x58、Native == 0xE01C
}

func TestPointerEventsReturnsEveryMoveInOrder(t *testing.T) {
	// 一帧内注入三个 Move：(10,10) Time=1ms、(20,10) Time=2ms、(30,10) Time=3ms
	// 断言 PointerEvents() 长度 3、顺序与 Pos、Time 一致；Pointer() == (30,10)
	// 再跑一帧不注入，断言 PointerEvents() 为空
}
```

Driver 的构造和读取焦点控件事件的写法照 `ui/driver.go` 与已有 `ui/*_test.go` 中使用 `EventsFor` 的测试。

- [ ] **Step 2: 运行，确认失败**

Run: `nice -n 19 go test ./ui -run 'TestKeyEventKeepsPhysicalCodes|TestPointerEventsReturnsEveryMoveInOrder' -count=1`
Expected: 编译失败（`HID`、`Native`、`Time`、`PointerEvents` 未定义）。

- [ ] **Step 3: 实现**

- `input.go`：加三个字段，各写一行注释说明含义、单位与零值（照 Global Constraints）。
- `runner.go`：`processEvents` 处理 `input.Pointer` 时把（已换算为逻辑坐标的）事件追加到 `c.framePointers`；帧开始清理处把 `c.framePointers = c.framePointers[:0]`。键事件路径只改 `Key`/`Code`，确认结构体其余字段原样保留。
- `context.go`：`PointerEvents()` 返回 `c.framePointers`；注释写明「包括 Move、Press、Release、Leave；坐标同 Pointer()；切片在下一帧前有效」。

- [ ] **Step 4: 运行，确认通过**

Run: `nice -n 19 go test ./ui ./ui/input -count=1`
Expected: PASS。

- [ ] **Step 5: 更新 API 文件**

Run: `go run ./internal/cmd/apitxt -w && git diff --stat api/`
Expected: `api/go/ui_input.txt`、`api/go/ui.txt` 出现新增行；把同样的新增项加进 `api/stable/ui_input.txt`；`go run ./internal/cmd/apitxt -check` 通过。

- [ ] **Step 6: 提交**

```bash
git add ui/input/input.go ui/runner.go ui/context.go ui/input_codes_test.go api/
git -c user.name=tango -c user.email=903300326@qq.com commit -m "feat(input): physical HID and native key codes, pointer timestamps, PointerEvents"
```

### Task 2: macOS 后端

**Files:**
- Modify: `internal/platform/darwin/canvas_keys.go`（新增 `macHIDKeys` 表与 `hidFromMac`）
- Modify: `internal/platform/darwin/canvas_view.go`（`keyDown` 约 249、`keyUp` 约 301、`flagsChanged` 约 332、`pointerHandler` 约 187；`rawKeys` 记录按下时的码供松开使用）
- Test: `internal/platform/darwin/canvas_keys_test.go`

**Interfaces:**
- Consumes: Task 1 的字段。
- Produces: `func hidFromMac(keyCode uint16) uint16`（未知返回 0）。

- [ ] **Step 1: 写失败测试** `TestHIDFromMac`，表驱动：

| keyCode | 键 | HID |
|---|---|---|
| 0x00 | A | 0x04 |
| 0x24 | Return | 0x28 |
| 0x4C | 小键盘 Enter | 0x58 |
| 0x38 / 0x3C | 左 / 右 Shift | 0xE1 / 0xE5 |
| 0x37 / 0x36 | 左 / 右 Command | 0xE3 / 0xE7 |
| 0x3A / 0x3D | 左 / 右 Option | 0xE2 / 0xE6 |
| 0x69 | F13 | 0x68 |
| 0x0A | ISO Section | 0x64 |
| 0x66 / 0x68 | JIS 英数 / かな | 0x91 / 0x90 |
| 0x5D / 0x5E | JIS ¥ / _ | 0x89 / 0x87 |
| 0x7F（不存在） | — | 0 |

再加一条：遍历 `macKeyCodes` 和 `macPhysicalKeys` 里出现的每个 keyCode，`hidFromMac` 都不为 0。

- [ ] **Step 2: 运行确认失败**：`nice -n 19 go test ./internal/platform/darwin -run TestHIDFromMac -count=1` → 未定义。
- [ ] **Step 3: 实现** `macHIDKeys`（覆盖 macOS 全部 kVK_* 虚拟键码，含 F13–F20、Help/Insert、小键盘、ISO/JIS 键）与 `hidFromMac`；`keyDown`/`keyUp`/`flagsChanged`/Caps Lock 构造 `KeyEvent` 时填 `HID: hidFromMac(code), Native: uint32(code)`；`pointerHandler` 填 `Time: time.Duration(msgGetFloat(ev, sel("timestamp")) * float64(time.Second))`（写法照 `canvas_attach.go:261`）。
- [ ] **Step 4: 运行确认通过**：`nice -n 19 go test ./internal/platform/darwin -count=1`。
- [ ] **Step 5: 实机核对**：用 `examples/ui-window` 或临时测试程序打印 `EventsFor` 的 `HID`，后台方式按 A、右 Shift、F13（如无此键跳过）；记录结果。按「GUI 测试不抢前台」用 cua-driver 后台发键。
- [ ] **Step 6: 提交** `feat(darwin): fill HID/native key codes and pointer time`

### Task 3: Windows 后端

**Files:**
- Modify: `internal/platform/windows/canvas_keys.go`（新增 `winScanHID` 表与 `hidFromWinScan`）
- Modify: `internal/platform/windows/canvas_input_windows.go`（`c.key` 约 548–572 构造处；`mouseMove` 约 376 及按下/松开/离开构造处）
- Test: `internal/platform/windows/canvas_keys_test.go`

**Interfaces:**
- Produces: `func hidFromWinScan(vk uint16, scan uint16, extended bool) (hid uint16, native uint32)`；`native` = `scan`，扩展键为 `0xE000|scan`，`vk == VK_PAUSE` 时为 `0xE11D` 且 HID 0x48。

- [ ] **Step 1: 写失败测试** `TestHIDFromWinScan`（此文件无构建标签，Mac 上可运行）：

| vk | scan | ext | HID | native |
|---|---|---|---|---|
| 'A' | 0x1E | no | 0x04 | 0x1E |
| VK_RETURN | 0x1C | no | 0x28 | 0x1C |
| VK_RETURN | 0x1C | yes | 0x58 | 0xE01C |
| VK_CONTROL | 0x1D | no / yes | 0xE0 / 0xE4 | 0x1D / 0xE01D |
| VK_MENU | 0x38 | yes | 0xE6 | 0xE038 |
| VK_NUMLOCK | 0x45 | yes | 0x53 | 0xE045 |
| VK_PAUSE | 0x45 | no | 0x48 | 0xE11D |
| VK_SNAPSHOT | 0x37 | yes | 0x46 | 0xE037 |
| VK_LWIN | 0x5B | yes | 0xE3 | 0xE05B |
| VK_OEM_102 | 0x56 | no | 0x64 | 0x56 |
| VK_F13 | 0x64 | no | 0x68 | 0x64 |
| 0 | 0x00 | no | 0 | 0 |

- [ ] **Step 2: 运行确认失败**：`nice -n 19 go test ./internal/platform/windows -run TestHIDFromWinScan -count=1`。
- [ ] **Step 3: 实现** 表（Set 1 扫描码 → HID，覆盖主键区、F1–F24、小键盘、导航区、JIS 0x70/0x73/0x79/0x7B/0x7D、韩文 0xF1/0xF2）；`c.key` 用 `canvasKeyLParam(lp)` 取扫描码与扩展位（扫描码为 0 时沿用现有 `MapVirtualKeyW` 兜底），调用 `hidFromWinScan` 填两个字段。AltGr 产生的假左 Ctrl 沿用现有处理，不额外改。鼠标事件填 `Time: time.Duration(getMessageTime()) * time.Millisecond`（新增 `GetMessageTime` 的 syscall 绑定，照该包已有 user32 绑定写法）。
- [ ] **Step 4: 运行确认通过**：Mac 上 `nice -n 19 go test ./internal/platform/windows -count=1`（只跑无标签文件的测试）；`GOOS=windows nice -n 19 go vet ./internal/platform/windows/... ./uihost/...`；`GOOS=windows nice -n 19 go test -c -o /dev/null ./internal/platform/windows`。
- [ ] **Step 5: 提交** `feat(windows): fill HID/native key codes and pointer time`

### Task 4: Linux 后端

**Files:**
- Modify: `internal/platform/linux/canvas_keys.go`（新增 `evdevHID` 表与 `hidFromXKB`）
- Modify: `internal/platform/linux/canvas_input_linux.go`（`onKey` 约 162；`onMotion` 约 106 及按键事件构造处；新增运动事件 time 偏移常量，照 `canvasEvButtonTime` 写法）
- Test: `internal/platform/linux/canvas_keys_test.go`

**Interfaces:**
- Produces: `func hidFromXKB(hw uint16) uint16`（evdev = hw − 8 后查表）。

- [ ] **Step 1: 写失败测试** `TestHIDFromXKB`：hw 38(A)→0x04、36(Return)→0x28、104(KPEnter)→0x58、37/105(左/右 Ctrl)→0xE0/0xE4、133(左 Super)→0xE3、94(ISO 102nd)→0x64、191(F13)→0x68、127(Pause)→0x48、107(Print)→0x46、0→0；另加：`canvasHWKeys` 里每个键码 `hidFromXKB` 不为 0。
- [ ] **Step 2: 运行确认失败**：`nice -n 19 go test ./internal/platform/linux -run TestHIDFromXKB -count=1`。
- [ ] **Step 3: 实现** 表与函数；`onKey` 填 `HID: hidFromXKB(hw), Native: uint32(hw)`；`onMotion` 和按键（press/release）构造处填 `Time`（GdkEventMotion / GdkEventButton 的 `time` 字段，毫秒；按 64 位布局取偏移，与现有常量同处注明）。
- [ ] **Step 4: 运行确认通过**：`nice -n 19 go test ./internal/platform/linux -count=1`；`GOOS=linux nice -n 19 go vet ./internal/platform/linux/...`。
- [ ] **Step 5: 提交** `feat(linux): fill HID/native key codes and pointer time`

### Task 5: 收尾检查与文档

**Files:**
- Modify: `CHANGELOG.md`（未发布段落加一条）
- Modify: 输入相关文档（若 `docs/` 或 `ui/AUTHORING.md` 有描述 `KeyEvent` 字段的段落，补上 `HID`/`Native`/`Time` 与 `PointerEvents`）

- [ ] **Step 1: 全量检查**

```bash
gofmt -l $(git ls-files '*.go')
nice -n 19 go vet ./...
GOOS=linux nice -n 19 go vet ./...
GOOS=windows nice -n 19 go vet ./...
go run ./internal/cmd/apitxt -check
nice -n 19 go test ./ui/... ./internal/platform/... ./uihost/... -count=1
```
Expected: gofmt 无输出，其余全部通过；有失败先确认是否在 origin/main 上同样失败，是则在报告中注明。

- [ ] **Step 2: 文档与 CHANGELOG**，提交 `docs: physical key codes and PointerEvents`。
- [ ] **Step 3: 汇报**：列出提交、测试结果、实机核对结果；询问用户是否合入 main 并推送。
