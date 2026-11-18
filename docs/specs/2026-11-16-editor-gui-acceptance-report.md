# 编辑器真实窗口 GUI 验收报告（2026-11-16 ~ 11-18）

本报告记录 Tango 代码编辑器在 macOS 真实窗口里“从输入到画面”的测量、与 mygo / Monaco 的对比、找到并修复的一帧延迟，以及替其他分支做的配对验收。设计见 [docs/superpowers/specs/2026-11-16-editor-gui-acceptance-design.md](../superpowers/specs/2026-11-16-editor-gui-acceptance-design.md)，计划见 [docs/superpowers/plans/2026-11-16-editor-gui-acceptance.md](../superpowers/plans/2026-11-16-editor-gui-acceptance.md)；它补的是 [M2 报告](code-editor-m2-report.md) 第 4.10 节与第 6 节里“按键到上屏延迟、真实 present、Monaco 对照”这几项欠项。

## 1. 结论

| 项 | 结论 |
|---|---|
| 三编辑器同条件对比（Metal） | 修复前 Tango 比 mygo、Monaco 慢约 1 帧：打字中位 47 ms，对 31 / 30 ms（第 3 节）。 |
| 原因 | 有 CADisplayLink 时，停顿后的输入要等下一拍才开始出帧，“收到按键 → 提交画面”稳定约 17 ms（第 5 节）。提交到上屏的约 2 帧是 macOS 窗口合成固有的，不是三缓冲或 GPU 太慢。 |
| 修复 | `internal/platform/darwin/canvas.go` 新增 `frameAtOnce`（b25960c）：上一拍没画、或显示链接已暂停时，出帧请求立即执行；两拍之间最多立即出一帧，连续变化仍由显示链接控制节奏。 |
| 修复效果 | 修复前后 AB/BA/AB 配对，两种合成状态下都快约 1 帧；快状态下修复后打字 32.1 ms、三光标 32.7 ms、滚动 30.7 ms，与 mygo、Monaco 持平（第 6 节）。 |
| 原生图层（native）默认 | 在编辑器上比 Metal 慢约 2 帧（打字 78.8 对 47.2 ms），3acc4cd 因此回退为 a8f5cd4（第 4 节）。 |
| “3acc4cd..a8f5cd4 回退一帧” | 不是代码回退：同一时段新旧版本交替，二者一起在约 48 ms 和约 61 ms 两种合成状态间切换（第 7 节）。 |
| 键盘升级方案 4b59345 | 通过：默认对 `TANGO_RENDERER=gpu` 打字中位差 0.6 ms、三光标 0.9 ms，标准 ≤ 16.7 ms；首键升级那一帧多 30–90 ms，只发生一次（第 8 节）。已随 1ef979c4 进 main。 |
| 中文输入法 | 只做了功能验证，部分通过；“不点击就不组字”现象未查清（第 9 节）。 |
| 未证 | 显示器级（非隔离窗口）延迟、物理光子、Windows 真实硬件、Monaco 内部 GPU 完成时间（第 10 节）。 |

## 2. 方法

### 2.1 工具（`tools/editor-gui-bench/`，均在 main）

- `gui-probe`（Swift）：ScreenCaptureKit 采集目标窗口，帧时间取 `SCStreamFrameInfoDisplayTime`（mach 时钟）；CGEvent 从 HID 事件流投递按键、滚轮和点击；z 序遮挡与隐私门禁；记录模式（稳定期逐像素取最大值，去掉浅色主题下闪烁的深色光标）、模板匹配，以及测量开始前按记录基线做整体对齐（±4 px，对齐后距离 > 1 判整轮无效）。
- `templates.mjs` / `make-templates.mjs` / `latency.mjs`（Node）：从记录轮生成预期正文模板；`latency.mjs` 用 `analysis.mjs` 的 `latencyBounds` / `summarize` 计算每次输入的上下界，所有投递的输入都计入分母。
- `native-driver.py`（Tango / mygo）、`monaco-driver.mjs`（可见 Edge `--app` 窗口，CDP 只做准备与读状态）、`matrix.py`（3 轮拉丁方）、`present-attrib.py`（按 `TANGO_PRESENT_TRACE` 归因）。
- 夹具：Tango 夹具 `fixture/`（`-tags tango_guiprobe`，本地回环、随机令牌的控制接口）；mygo 夹具 `mygo-fixture/`（独立模块，`GOWORK=off`，mygo v0.9.0 自带编辑器）。三者文档与光标逐字节相同（`fixture-text.mjs` 按 Go 夹具校验和核对）。

### 2.2 口径

- **数值含义**：表中“中位 / p95”都是**上界**：投递前时间到第一帧“所有模板位置都显示预期字形”的 displayTime。60 Hz 采集，分辨率约 16.7 ms。下界只用于严格区间判定；Tango 内容不变时只有 idle 帧，下界偏弱。
- **采集范围**：隔离窗口模式，即 WindowServer 合成出的目标窗口画面，不是显示器上实际扫出的画面。因为用户其他窗口一直在屏幕上，显示器裁剪模式会被隐私门禁拒绝。
- **对齐条件**：三个编辑器正文都是 1920×944@2x（显示器逻辑分辨率 1920×1080，减去菜单栏和 Edge 标题栏），字号 14、行高 21 px、浅色主题。Monaco 关闭了自动补全、悬停、相同词和选中内容高亮（Tango 没有这些）。
- **场景**：打字（第 20 行 `// qq` 之后连续打 `x`）、三光标（第 20、24、28 行同时打字）、滚动（每次 40 px 像素滚轮）。Monaco 每 40 px 实际滚 50 px，比较时按各自实际位移判定。
- **公平性差异**：Tango 要点一下窗口才拿到焦点，点击发生在计时之前，然后重新设置文档和光标；mygo 用 AutoFocus、Monaco 用 `ed.focus()`，都不点击。
- **比较规则**：比较版本必须在同一时段按 AB/BA 交替。原因见第 7 节：窗口服务器的合成状态能维持几分钟，会让同一份代码整体快或慢一帧。

## 3. 三编辑器正式矩阵（修复前）

2026-11-18 06:46–07:12，基线 c19cc8d（origin/main a4a5a2b 加工具提交），Metal，load 2.4–4.3。3 轮拉丁方（tango/mygo/monaco、mygo/monaco/tango、monaco/tango/mygo），每个引擎每轮打字 100 键、三光标 30 次、滚动 240 次；第 1 轮录模板，后两轮复用。27 个场景全部完成，最终文本全部精确；只有 mygo 第 2 轮滚动 239/240。

| 场景 | Tango 中位 / p95 | mygo 中位 / p95 | Monaco 中位 / p95 |
|---|---|---|---|
| 打字 | 47.2 / 64.2 | 31.1 / 46.5 | 30.1 / 40.1 |
| 三光标 | 47.2 / 64.3 | 30.4 / 47.1 | 29.2 / 52.8 |
| 滚动 | 47.7 / 65.1 | 14.2 / 32.5 | 29.3 / 33.4 |

各轮的 p95 上界很稳定：Tango 62–67，mygo 44–48（滚动 31–38），Monaco 35–42（三光标 46–56，滚动 33–34）。按严格区间规则（一方 p95 上界 < 另一方 p95 下界）仍判“无法区分”，但三个场景、三轮的上界方向一致：Tango 多约 1 帧。

## 4. 原生图层（native）对 Metal

3acc4cd 曾把大窗口默认渲染器改成原生图层。基线 3acc4cd，同一时段 AB/BA/AB：

| 场景 | native 中位 / p95（三对中位） | Metal 中位 / p95（三对中位） |
|---|---|---|
| 滚动 240（08:51–09:11，default 对 gpu） | 78.1 / 96.6（64.1* / 81.0 / 79.5） | 46.1 / 88.1（46.0 / 46.1 / 46.3） |
| 打字 100（09:54–09:59，`TANGO_RENDERER=native` 对 gpu） | 78.8 / 99.7（69.2 / 79.9 / 79.8） | 47.2 / 63.9（47.4 / 47.0 / 47.1） |
| 三光标 30（同上） | 87.6 / 101.4（81.7 / 89.0 / 91.0） | 47.2 / 66.5（46.1 / 47.4 / 47.6） |

\* 这一次 default 在运行中途升级到了 Metal。

结论：native 比 Metal 慢约 2 帧（32–40 ms）。3acc4cd 随后回退为 a8f5cd4，最终方案是“普通窗口收到键盘输入就切到 Metal”（第 8 节）。测量期间 UU 远程在推流，绝对值偏高，但两边交替运行，相对差可信。

## 5. 归因：多出的那一帧在哪里

10:09，基线 a8f5cd4 加工具，`TANGO_RENDERER=gpu TANGO_PRESENT_TRACE=1`，打字 30 键，全部完成。夹具的输入观察器、probe、Metal 呈现追踪用同一个 mach 时钟。每键取“收到按键后第一个提交的 drawable”：

| 阶段 | 中位 | p90 |
|---|---|---|
| 投递 → 夹具收到 TextInput | 11.6 | 16.7 |
| 收到 → drawable 提交 | 17.2 | 17.8 |
| 提交 → presentedTime | 32.3 | 32.4 |
| presentedTime → SCK 匹配帧 | −2.5 | 4.2 |
| 合计 | 61.1 | 68.6 |

同时开 `TANGO_FRAME_TRACE`：GPU 每帧 p50 2.19 ms、p95 3.41 ms；主线程每帧 0.75 ms；`nextDrawable` 阻塞 0.02 ms。

- “提交 → 上屏”固定 2 个刷新，但 GPU 3 ms 内已画完；而且每次按键只提交 2 个 drawable，提交时队列是空的。所以原因不是 GPU 赶不上，也不是三缓冲堆积。mygo 的 `docs/architecture.md` 同样记录了这一点：裸 CAMetalLayer 窗口也要约 2 个刷新，减少 drawable 数量或随事务呈现都没有稳定收益。
- “收到 → 提交”稳定约 1 帧：按键到达时刚错过当前这一拍，Tango 只恢复显示链接，要等下一拍才出帧。mygo 在停顿后的输入会立即出帧。Tango 没有显示链接时用的 16 ms 定时器退路本来就会立即出帧，只有显示链接这条主路径缺了这一步。
- SCK 的 displayTime 比 Core Animation 的 presentedTime 早 2–5 ms，所以最后一段可能是负数。

## 6. 修复与配对验证

修复：b25960c `fix(darwin): draw a frame asked for after a pause at once`。规则是 `frameAtOnce(idleTicks, linkRunning, inFrame) = !inFrame && (!linkRunning || idleTicks > 0)`；立即出帧后 `idleTicks` 清零，所以两拍之间最多立即出一帧。测试 `TestFrameAtOnce`（先红后绿）；darwin、uihost、`ui/...` 测试通过。

修复前后 AB/BA/AB，Metal，三个场景都用相同模板：

| 场景 | 约 61 ms 状态（10:40，修复前 → 后） | 约 48 ms 状态（12:13，修复前 → 后） |
|---|---|---|
| 打字 100 | 61.5 → 44.8（三对 61.5/61.4/61.6 → 45.1/45.0/43.8） | 47.5 → 32.1（47.6/47.0/47.8 → 31.9/32.0/32.2） |
| 三光标 30 | 61.7 → 43.5 | 54.0 → 32.7 |
| 滚动 240 | 62.9 → 46.8 | 48.6 → 30.7 |

- 两种状态下都快约 1 帧（15–21 ms），连续滚动也受益，因为滚轮事件之间也有空拍。
- 关掉呈现追踪复测：修复前 61.5、修复后 45.8，说明追踪本身不增加延迟。
- 快状态下的配对是先探测、碰到快状态后再开跑的：前 12 次探测都在 61–63 ms，第 13 次 49.9 ms 后开跑，结束后复探 45.7 ms。
- 快状态下修复后的 Tango（31–33 ms）与第 3 节的 mygo（约 31 ms）、Monaco（约 30 ms）持平。mygo 和 Monaco 在慢状态下的数据没有测。

## 7. 合成状态：不是代码回退

10:40 修复前的基线是 61.5 ms，比早上同配置的 47 ms 慢一帧。先二分（11:22，修复后的代码，分别撤掉 b47d0cc、撤掉数字快速路径 43d3061+b6ed525、加入 9f0d478）：四个变体都在 42–45 ms，差别 1–2 ms。再在同一时段让 3acc4cd 与 a8f5cd4（都不含修复）交替：

| 轮次 | 3acc4cd | a8f5cd4 |
|---|---|---|
| 11:39–11:41（O N N O O N N O） | 48.8*、47.5、50.9、48.2 | 60.8、57.8、47.8、51.6 |
| 11:42–11:44（O N 逐次交替） | 61.6、59.8、60.3、61.9、61.9、61.7 | 51.0、59.3、61.2、60.9、61.4、61.6 |

\* 被 Qoder 抢前台，88/100。

两个版本会一起在约 48 ms 和约 61 ms 之间切换，相差 1 帧，状态能维持几分钟，与代码版本无关；mygo 的文档也描述过这种现象。修复后在多个时段都是 43–46 ms（慢状态）或 31–33 ms（快状态），不再卡在显示链接的节拍上。

## 8. 键盘升级分支验收（替 min-84，用户同意）

min-84 的 `feat/native-keyboard-promotion` 4b59345（基于 2f1c4f5，已含工具和 `frameAtOnce`）：所有窗口默认 native，普通窗口收到键盘输入后下一帧切到 Metal，`ui.ContentData` 窗口按键不切换。15:13–15:15，default 对 `TANGO_RENDERER=gpu`，AB/BA/AB，每次运行都是新进程（default 从 native 起步），12/12 文本精确：

| 场景 | default（第 2 键起中位，三对） | gpu | 差 |
|---|---|---|---|
| 打字 100 | 32.5（32.6/31.9/33.0），p95 48.3 | 31.9（32.4/31.7/31.7），p95 48.9 | 0.6 |
| 三光标 30 | 32.7（34.8/32.0/32.7），p95 45.5 | 31.8（30.6/33.1/31.6），p95 45.2 | 0.9 |

- 首键（触发升级）：打字 default 133.6 / 74.9 / 90.9，gpu 45.7 / 41.4 / 45.2；三光标 default 77.5 / 91.8 / 85.3，gpu 38.2 / 45.6 / 61.9。升级那一帧多 30–90 ms，只发生一次。
- 渲染器：夹具 stderr 不打印 renderer 行，改用 `/state` 的 `uihost.Renderer`：default 三次都是按键前 native、测完 gpu；gpu 三次始终是 gpu。
- 首键升级会让整行重绘，原来的模板规则因此拒收第 1 步。已修工具（e22c4be9）：至少两步能直接定位时，按列宽推算其余步骤，再做同样的校验。
- 该方案已由 min-84 推到 main（1ef979c4），1ef979c4 的 CI、API 和文档构建都通过。

## 9. 中文输入法（功能验证，非延迟）

输入法用百度拼音（`com.baidu.inputmethod.BaiduIM.pinyin`；系统简体拼音在本机的 TIS 列表里取不到），真实按键 `n i h a o` 后按空格或回车提交，不直接调用 `setMarkedText`，候选列表不落盘。

| 用例 | 结果 |
|---|---|
| 组字 → 提交“你好”；组字期间模型不变；撤销、重做精确 | 通过 |
| 退格后重新组字 | 3/3 通过 |
| Esc 取消（文本不变、不提交） | 3 轮 2 次通过；失败那次的原因没有留下记录 |
| 三光标提交（三处都插入“你好”、一次提交、一个撤销步） | 4 轮 3 次通过（第一次失败时插入次数还没记录） |

- “三光标只插入一处”最初是驱动的问题：用来聚焦的点击会把夹具预设的三个选区合并成一个。现在改为点击后重新设置光标。
- 未定现象：先点击聚焦、在另一次 probe 里再打字时，百度拼音完全不组字。Tango 一侧的状态（textOn、当前输入上下文、上下文选中的输入法）在点击和不点击两种情况下完全一致；原生 `NSTextView` 对照同样不组字，不能作为判据。所以这不认定为 Tango 缺陷，宿主没有改动。
- 所有用例都在隔离窗口模式下运行，计划中的“候选选择、移动后再组字、失焦”还没测。

## 10. 局限与未证

1. **显示器级延迟**：全部是隔离窗口（WindowServer 合成出的窗口画面），不是显示器扫出的画面，也不是物理光子。显示器裁剪模式需要屏幕上没有其他窗口遮挡目标，本次一直达不到。
2. **60 Hz 采集**：上界的分辨率约 16.7 ms；严格区间判定在第 3 节仍是“无法区分”，结论依靠多轮、多场景方向一致与同时段配对。
3. **环境**：多个会话共用这台 Mac，UU 远程、企业微信等会抢前台或推流；被打断的运行都已丢弃，只用同一时段交替的数据比较。
4. **合成状态**：同一份代码在约 48 和约 61 ms 两种状态间切换，绝对值必须注明测量时段，比较必须同时段配对。
5. **Windows 真实硬件**、Monaco 内部 GPU 完成时间、`CAMetalDisplayLink`、显示器级 present：均未测。
6. **切换前台时的 P99**：min-0f 发现切换前台时 Tango 主线程本身被堵 34–99 ms，显示链接晚到是被它连带的；其补帧修复改善有限，后台线程出帧原型在验证中。这超出本报告范围。

## 11. 提交与复现

- 工具：8c25419、a5a845d、85cdb16、867603c、098d29c、c26f75d、e22c4be9；修复：b25960c；min-5a 的后续修复：140cad3（`tools/...` 归为非 API）、2f1c4f5（`canvas_pacing_test.go` 补 `//go:build darwin && !ios`）。main 在 2f1c4f5 与 1ef979c4 上 CI 全绿。
- 证据（未入库，被 git 忽略）：editor-m2-perf worktree 的 `.superpowers/sdd/2026-11-16-editor-gui-acceptance/`，台账 `progress.md`；正式矩阵 `matrix-formal/`，配对 `renderer-pairs/`、`key-pairs/`、`atonce-pairs/`、`fast-pairs/`、`old-vs-new*/`、`bisect/`、`kbd-pairs/`，归因 `present-attrib/`、`frame-trace/`。
- 复现：构建 `go build -tags tango_guiprobe -o <bin>/tango-fixture ./tools/editor-gui-bench/fixture`、`swiftc -swift-version 5 -parse-as-library CaptureCore.swift main.swift -o <bin>/gui-probe`、`(cd tools/editor-gui-bench/mygo-fixture && GOWORK=off go build -o <bin>/mygo-fixture .)`；矩阵 `python3 tools/editor-gui-bench/matrix.py --bin <bin> --out <dir>`；单引擎 `native-driver.py --engine tango|mygo`、`monaco-driver.mjs --templates`；归因 `TANGO_RENDERER=gpu TANGO_PRESENT_TRACE=1 native-driver.py … --keys 30` 后运行 `present-attrib.py <dir>/type`。运行前确认没有其他前台或计时测量，1 分钟负载 < 6，并且屏幕没有人在操作。
