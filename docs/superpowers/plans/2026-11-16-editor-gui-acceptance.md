# macOS Editor GUI Acceptance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans for inline implementation. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在macOS真实窗口中测OS合成输入到WindowServer正文变化，公平比较Tango/Monaco，验证真实拼音IME，并用Metal呈现反馈归因多光标迟滞。

**Architecture:** 本地Swift采集/输入工具 + Node启动/分析 + Go原生编辑器夹具。先验证纯逻辑和测量器，再做小pilot；Metal呈现诊断默认关闭、与正式两引擎对照分离。每一步以采集有效性为门禁，不把未知反馈推断为成功。

**Tech Stack:** Go1.27，purego/objc，macOS ScreenCaptureKit/CoreGraphics/Carbon，系统Swift，Node内置模块，本机Edge与Monaco0.56；不新增依赖。

**Spec:** `docs/superpowers/specs/2026-11-16-editor-gui-acceptance-design.md`

## Global Constraints

- 使用已有隔离worktree `/Users/admin/min/tango/.claude/worktrees/editor-m2-perf`、分支gui-acceptance-design，基线137bb26。不碰主checkout/index，不合并、不推送。
- Windows排除；macOS原生真实IME路径，不注入Unicode文本/剪贴板冒充组字。
- 时钟：mach_absolute_time→ns，SCK displayTime同基准；PTS/callback时间不能替代。
- 60Hz捕捉仅证明WindowServer可见画面，不证明物理光子/Monaco内部GPU-complete，也不替代原离屏16ms门槛。
- 默认present诊断不开启；有界队列、无Device锁的异步回调，Close不得无限等待。
- 稀疏输入完成必须模型预期+正文识别，不能用光标/状态栏或确认标记替代。
- 输出仅测试ROI、合成fixture、时间线。候选列表不得落盘。恢复输入源/焦点、只关自己进程。
- 固定pilot各10键/30滚动，正式3对（100键/240滚动；多光标30事件），每组最多2次环境重试；最多一轮产品修复和一轮3对复测，不无限循环。
- 正式load<6、解锁、无其他GUI重任务；中途screen变化/采集失效标无效。缺失/超时在总事件分母中，不能只挑成功样本。
- 本次不自行提交；完成后一个整体独立review。

## Review Focus

1. SCK Idle≠丢帧；时间倒退/尺度改变/空白帧必须阻塞PASS（Task1纯逻辑测试）。
2. 正文未变而光标闪烁/角落状态变化不能算输入响应；多光标必须核所有可见位置（Task1匹配测试与Task3pilot）。
3. 原生回调在Close之后到达、重复present和缺失反馈不能泄漏/写已关闭Device（Task2队列/生命周期测试）。
4. IME提交前buffer不变、撤销恢复原文；提交中文但没有Composition轨迹不算真实IME通过（Task4oracle测试）。
5. 正式3对不能算窄总体CI；超时保留、p95区间跨阈值保持未定（Task1分析测试、Task5报告）。

---

### Task 1: 原生采集与可验证的时间/图像分析

**Files:** Create `tools/editor-gui-bench/CaptureCore.swift`, `tools/editor-gui-bench/main.swift`, `tools/editor-gui-bench/analysis.mjs`, `tools/editor-gui-bench/analysis.test.mjs`.

**Interfaces:** CLI `gui-probe --list`, `--self-test`, `--job job.json --out directory`；Job包含targetPID、windowID、displayID、bounds、采样ROI、物理keycode/scroll序列、超时、目标标识。NDJSON事件含type/id、投递host时间上下界、capture displayTime/status/scale、正文匹配结果、超时/丢样。`analysis.mjs`导出 `latencyBounds(event, frames)`、`summarize(events)`，最近秩p95对完整分母计算上下界，缺失上界null/无穷。

- [ ] 写并运行失败测试：clock/negative time、Idle/Blank、尺度改变、正文ROI-only匹配、全部可见位置完成、右删失p95、3pair不算总体CI。
- [ ] 最小实现纯逻辑；Node内置`node --test tools/editor-gui-bench/analysis.test.mjs`、Swift `gui-probe --self-test`通过。
- [ ] 实现SCK显示器crop采集、前台PID核验、同clock CGEvent投递、原始事件NDJSON；`--list`列出仅窗口元信息，不采集其他内容。
- [ ] 构建工具并预检权限；单独校准动画验证displayTime/scale/Complete/Idle与投递时钟。不把校准作为性能验收。

### Task 2: 默认关闭的Metal presented诊断

**Files:** Create `internal/render/gpu/metal/presenttrace.go`, `presenttrace_test.go`, `presenttrace_darwin.go`; modify `presenter_darwin.go` narrowly. Existing timeline/frametrace语义仍是CB完成；不混入同回调。

**Interfaces:** `TANGO_PRESENT_TRACE=1` opt-in；每个presenter会话一个独立序号，有界record包含session/present/renderFrame/drawableID、host提交/回调时间、presentedTime、有效/零时间/超时/溢出/late状态。后台输出不在callback或Device锁中进行，默认不开启不分配block。复用objc.NewBlock，捕获独立recorder而非Device。

- [ ] 先写测试证明关闭、重复、乱序、容量溢出、Close后到达行为；旧实现测试应缺少类型/方法失败。
- [ ] 实现有界recorder和presented handler，释放本地block引用、原生持有期间不释放Go捕获；callback仅入队。
- [ ] 运行record测试、native Metal像素/Wait/retained/Close定向测试；diagnostic关闭路径不改变计数/像素。

### Task 3: Tango/Monaco真实窗口夹具与pilot

**Files:** Create `tools/editor-gui-bench/fixture/main.go`, `fixture/main_test.go`, `driver.mjs`, `monaco.html`, `README.md`；复用`tools/monaco-bench/server.mjs`资源服务。Create `internal/guiprobe/input.go`+tag-off文件；narrow observer call at `uihost/host.CanvasInput` for explicit `tango_guiprobe` build only.

**Interfaces:** 夹具通过仅loopback、随机token保护的HTTP `/state`、`/setup`、`/quit` 控制，UI访问用App.Post，计时关键路径无HTTP等待。state含文本版本/校验摘要、选区、viewport、正文bounds、输入Composition/TextInput轨迹（只fixture），私有ready JSON写OUT。Tango真实UI renderer=gpu、Monaco可见Edge正常vsync；均同1920×1080@2x窗口、字体/行高/主题/fixture。

- [ ] 写state/oracle测试（主线程调度、token认证、假版本拒绝、ROI只限正文）；写监听器默认关闭与事件轨迹测试。
- [ ] 实现夹具和Node驱动：私有profile与仅本轮进程清理；CDP仅setup与核对状态，性能输入必须CGEvent。
- [ ] 各跑一次10键/30滚动pilot；记录全部事件与预期正文签名。最多两次适配修正，仍无法可靠关联则停止该项并记录阻塞。
- [ ] 冻结源码/工具/参数hash，写入环境元数据和采集校准结果，不混后续变更数据。

### Task 4: 真实系统拼音IME案例

**Files:** Extend `main.swift`, `driver.mjs`；create `ime-cases.mjs`, `ime-cases.test.mjs`。

**Interfaces:** TIS获取/保存/选择已启用ABC/简体拼音；退出/异常恢复。真实合成KeyDown/KeyUp触发原生setMarkedText/insertText；只记录本轮输入。oracle规定nihao→你好、取消保持原文、undo/redo精确预期及3光标所有提交。

- [ ] 写失败测试：中文提交无Composition不通过、marked阶段model变化不通过、undo文本不符、候选内容不输出、恢复状态。
- [ ] 实现8类案例：提交、Escape、Backspace、候选选择、移动后再组字、undo/redo、3光标、失焦；记录输入源真实ID。
- [ ] 运行真实案例；不直接调用setMarkedText。候选不确定/权限不足时标阻塞，截图遮蔽候选正文，否则不保存候选窗。

### Task 5: 固定预算真实GUI矩阵、回归与报告

**Files:** Create `docs/specs/2026-11-16-editor-gui-acceptance-report.md`; update `docs/specs/code-editor-m2-report.md`对应遗留项；证据仅ignored工作目录。

- [ ] 与其他会话明确交接GUI资源，检查screen/load，单实例lock。正式3对轮转，present诊断关闭；逐对原始分位区间/丢样/超时/正确性完整报告。
- [ ] 6000/30与100000/100多光标场景各30输入/导航；若有异常再开启present诊断短归因≤30事件，记录线程CPU/wall/采集关联，不改变生产默认时序。
- [ ] 若证明产品热点，只做一次TDD最小修复与一次3对重测；无法归因保留未证，不放宽测试。
- [ ] `go test ./internal/render/gpu/metal ./uihost ./ui/editor/...`、tag-on夹具/observer测试、`go vet`、`go build ./...`及Node/Swift自测；适用平台交叉构建，Windows不做实机。
- [ ] 报告按功能/观测有效性/原目标判定三列更新；保留M1旧PASS与所有未证，不把新可见响应等同于物理光子/Monaco内部GPU。
- [ ] 一次整体独立review；修Critical/Important，不自行合并或推送。
