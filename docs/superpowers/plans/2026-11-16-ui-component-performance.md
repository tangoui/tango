# UI 组件极限性能 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给根 `ui` 包全部组件入口建立统一基准与跨框架对比，再按实测热点分批优化，直到每个组件同场景下 CPU 不超过 MyGo、NSGo 的一半并满足硬预算（或证明受限于渲染器）。

**Architecture:** 两层。第一层在 `ui` 包内：表驱动夹具 + `BenchmarkComponents` + 确定性检查（静止、局部重绘、覆盖）+ 计时预算棘轮；`internal/cmd/uiperf` 负责跑基准、出热点表、做 A/B。第二层在 `tools/uibench/`：自研进程外 runner，三框架各一个 `-scene` 场景程序，量进程 + WindowServer CPU、帧间隔、内存、启动。之后按热点表分批优化，每批 A/B + 跨框架复测。

**Tech Stack:** Go 1.27；Tango `ui` Driver/Runner；MyGo v0.9.0（`github.com/egoist/mygo/ui` 自绘模式）；NSGo（本地源码 `/Users/admin/min/outputs/tango-nsgo-perf-20261115/sources/nsgo`）；`github.com/ebitengine/purego`（runner 读 `proc_pid_rusage`）。

**Spec:** `docs/superpowers/specs/2026-11-16-ui-component-performance-design.md`

## Global Constraints

- 工作区：`/Users/admin/min/tango/.claude/worktrees/ui-component-perf`，分支 `perf/ui-components-20261116`，基于 origin/main `97f5219`。不碰其他会话的 worktree，不改主仓未提交文件。
- 提交作者与提交者固定 `tango <903300326@qq.com>`，**不加 Co-Authored-By**；只 `git add` 明确路径，不 `git add -A`；不裸用 `git stash`；不推送（推送前问用户）。
- 不复制 MyGo / NSGo 代码（含 NSGo benchmark runner）；只借测量方法，代码自写。对手场景按各框架惯用写法。
- 不改组件公开 API、默认值、视觉；不改 `api/` golden（若 `go run ./internal/cmd/apitxt -check` 报差异，说明误改了公开 API，退回）。
- 非目标区域只记录不改：渲染器后端（`internal/render`、`uihost` 的 GPU/原生路径、`macoslite`）、内存常驻策略、`ui/terminal`、`ui/editor`。可改：文字 shaping/测量缓存、`ui` 核心构建/布局/节点生命周期/指纹/damage。改核心文件前先 `git -C /Users/admin/min/tango log --oneline -5 origin/main -- <file>` 并查看 `.claude/worktrees/all-module-perf` 与 `/Users/admin/min/tango-memory-fix` 是否改了同一函数（`git -C <wt> diff origin/main -- <file>`），有重叠就在账本记录并避开该函数。
- 第一层统一口径：窗口 1440×900、Scale 2、`NoAnimations: true`、主题 antd（`theme.AntDesign()`）、计时用 no-op 渲染器（构建 + 布局 + 绘制命令 + damage 指纹，不光栅化）。
- 硬预算（只收紧不放宽）：首次构建 ≤ 0.5 ms，浮层类 ≤ 2 ms；交互帧 ≤ 2 ms；gallery 整页交互帧 ≤ 4 ms；10 万项交互帧 ≤ 4 ms 且 ≤ 1 千项的 1.5 倍；交互帧脏区面积 ≤ 2 × 包围盒面积；`Settle()` 后 `NeedsFrame()` 为 false（`Animated` 夹具除外）。
- 跨框架"更快"：Tango（进程 CPU + WindowServer CPU）各轮中位数 ≤ 对手中位数 × 0.50，双方各轮四分位区间不重叠，帧率 ≥ 对手，帧间隔 p99 ≤ 对手；对 MyGo、NSGo 分别判定。
- 测量纪律：1 分钟负载（`sysctl -n vm.loadavg` 第一个数）< 6 才采信；A/B 用固定测试二进制、每轮新进程、5 轮交替，取各轮 ns/op 中位数；收益 ≥ 10% 或 allocs/op 下降 ≥ 20% 才保留。
- 每批护栏：`go test -count=1 ./ui/... ./examples/ui-gallery/...` 全过（含 `TestGalleryForcedHintsVerified`）；`GOOS=windows go vet ./ui/...`、`GOOS=linux go vet ./ui/...`、`GOOS=js GOARCH=wasm go vet ./ui/...` 通过；进程内 B/op 与跨框架 footprint 回归 ≤ 5%。

## Review Focus

1. **交互不回到原状态**（滚到底、文字越打越长、浮层只开不关）会让基准测成空帧。期望：偶数步做、奇数步撤销，两步后绘制命令与起点一致。→ Task 1 的 `TestPerfInteractSteady`。
2. **负载高时测出的数字被采信**（本机常有其他会话，负载可到 20+）。期望：`uiperf` 的 A/B 和 runner 在负载 ≥ 6 时等待或标记"受干扰"，不出结论。→ Task 8 `TestLoadGate`、Task 9 runner 的轮次剔除测试。
3. **对手场景做的工作比 Tango 少**（或反过来）。期望：每个场景程序每秒报告工作量，runner 发现三家工作量相差 > 5% 时判场景无效。→ Task 9 `TestWorkMismatch`。
4. **渲染路径不一致**：同一场景 Tango 一会走原生、一会走 Metal。期望：结果里记录每轮的渲染路径，同一场景内不一致则报错。→ Task 9 `TestRendererConsistent`。
5. **优化后其他主题或 RTL 下坏掉**（缓存键漏了主题/方向）。期望：每个夹具在 element、shadcn 主题和 RTL 下也能跑完交互且绘制命令与重建一致。→ Task 1 `TestPerfThemesRTL`。

---

## 文件结构

| 路径 | 职责 |
|---|---|
| `ui/runner.go`（改） | 新增未导出字段 `lastDirty []image.Rectangle`，`Frame()` 在 `collectDamage` 后赋值（nil = 整帧）。唯一的产品代码改动，供测试读取。 |
| `ui/perf_harness_test.go` | 夹具类型、注册表、perf Driver、no-op 渲染器、包围盒与脏区计算、绘制命令指纹。 |
| `ui/perf_bench_test.go` | `BenchmarkComponents`、`BenchmarkComponentsRaster`。 |
| `ui/perf_check_test.go` | 覆盖、静止、局部重绘、交互稳定、主题/RTL、计时预算测试。 |
| `ui/perf_debt_test.go` | 两张棘轮表：`perfDamageDebt`、`perfTimeDebt`（组件/场景 → 当前实测与原因）。 |
| `ui/perf_fx_*_test.go` | 各组件族夹具（Task 1–7）。 |
| `examples/ui-gallery/perf_pages_test.go` | gallery 整页基准与 4 ms 预算。 |
| `internal/cmd/uiperf/` | `bench` / `hot` / `ab` 三个子命令及其解析、统计、报告。 |
| `tools/uibench/` | 进程外对比：`proto/`、`runner/`、`tango/`、`mygo/`、`nsgo/` 各自 go.mod；`scenes.md`；`setup.sh`、`build.sh`。 |
| `docs/performance/ui-components.md` | 方法、逐组件状态表、每批结果、转交清单。 |

---

### Task 1: 第一层基准骨架 + 种子夹具

**Files:**
- Modify: `ui/runner.go`（`Runner` 结构体加字段；`Frame()` 中 `dirty := r.collectDamage(scale)` 之后加 `r.lastDirty = dirty`）
- Create: `ui/perf_harness_test.go`、`ui/perf_bench_test.go`、`ui/perf_check_test.go`、`ui/perf_debt_test.go`、`ui/perf_fx_seed_test.go`

**Interfaces:**
- Produces（后续所有夹具任务使用，名字和类型照抄）：

```go
type perfFixture struct {
	Name     string   // 根 ui 导出入口名，如 "Button"
	Covers   []string // 本夹具顺带覆盖的入口，如 Card 夹具覆盖 "CardMeta","CardAction"
	Skip     string   // 非空 = 不适用，写理由（不渲染任何东西的辅助函数等）
	Overlay  bool     // 浮层类：首次构建预算 2 ms
	Animated string   // 非空 = 静止时合法地持续动画（Spin、Skeleton 闪烁…），写理由
	Build    func(c *Context)                    // 典型实例；浮层类以打开状态构建
	Interact func(r *perfRun, i int)             // 第 i 步交互；i 偶数做、奇数撤销
	Large    func(n int) func(c *Context)        // 仅集合类：n 项数据的页面
	LargeInteract func(r *perfRun, i int)        // nil = 默认滚动（见下）
}
type perfRun struct {
	D      *Driver
	Target geom.Rect // 组件包围盒：根节点第一个子节点的矩形
}
func (r *perfRun) Center() geom.Point
func (r *perfRun) At(fx, fy float32) geom.Point // Target 内按比例取点，0..1
func registerPerf(fx ...perfFixture)            // 名字重复则 panic
func perfFixtures() []perfFixture               // 按 Name 排序
func newPerfDriver(page func(*Context), raster bool) *Driver // 1440×900、Scale 2、NoAnimations；raster=false 时 Renderer 用 perfNopRenderer
func perfPage(build func(c *Context)) func(*Context)         // Column(c, build, layout.Pad(24))
func perfBBox(d *Driver) geom.Rect   // Target ∪ 所有打开浮层根节点矩形（c.overlays[i].root）
func perfDirtyArea(d *Driver) float32 // lastDirty 并集面积（逻辑像素）；nil 记为视口面积
func perfListHash(d *Driver) uint64   // 对 d.r.List().Ops 做 fmt.Fprintf("%#v") 后 FNV-64a
```

- `perfNopRenderer` 实现 `paint.Renderer`（`Render` 直接返回 nil）和 `BackdropBlur() bool { return true }`。
- `newPerfDriver` 在包内手工组装：`win := &driverWindow{...}`、`d := &Driver{w: win}`、`d.r = NewRunner(page, RunnerOptions{Theme: theme.AntDesign(), Clock: ..., NoAnimations: true, Renderer: ...})`、`d.r.Attach(win)`，与 `NewDriver`（`ui/driver.go:81`）同一套时钟起点。
- 默认大数据交互：在 Target 中心 `input.Scroll`，`i%40 < 20` 时 DY=+120，否则 DY=−120。

- [ ] **Step 1: 写检查测试（先失败）**

在 `ui/perf_check_test.go`：
- `TestPerfCoverage`：用 `go/parser` 解析 `ui` 目录全部非 `_test.go` 文件，收集导出函数中第一个参数类型是 `*Context` 的函数名（与命令 `grep -E "^func [A-Z][A-Za-z0-9]*(\[[^]]*\])?\(c \*Context"` 同口径，当前 224 个）。每个名字必须出现在某个夹具的 `Name`、`Covers` 中，或作为带 `Skip` 理由的夹具出现。缺失项列表一次性报告。夹具 `Name` 不对应任何入口也报错。在 Task 7 完成前，用 `perfCoveragePending`（`perf_debt_test.go` 中的 `map[string]bool`，初始为全部 224 个名字减去种子已覆盖的）放行；表里有但已覆盖的名字要报错（棘轮）。
- `TestPerfIdle`：每个非 Skip 夹具 `Settle()` 后断言 `!d.NeedsFrame()`；`Animated` 非空的跳过断言但 `t.Log`。
- `TestPerfDamage`：每个有 `Interact` 的夹具，Settle 后执行 `Interact(r,0)` 再 `Frame()`，若 `perfDirtyArea > 2*面积(perfBBox ∪ 交互前的 perfBBox)` 则不达标。不达标且不在 `perfDamageDebt` 中 → 失败；在 `perfDamageDebt` 中但已达标 → 失败并提示删除该项。
- `TestPerfInteractSteady`：Settle 后记 `h0 := perfListHash`；`Interact(r,0)`+Frame 得 `h1`，`Interact(r,1)`+Frame 得 `h2`。断言 `h1 != h0` 且 `h2 == h0`。夹具若确需例外，不允许豁免：改写 Interact。
- `TestPerfThemesRTL`：对每个有 `Interact` 的夹具，在 `theme.Element()`、`theme.Shadcn()` 和 RTL（`ConfigProvider(c).Direction(RTL).Body(func(){ fx.Build(c) })`，见 `ui/nav_config.go:191`）各跑 Settle + Interact(0) + Interact(1)，断言不 panic 且 `h2 == h0`。
- `TestPerfBudgets`：仅当 `TANGO_PERF_BUDGET=1` 时运行，否则 `t.Skip`。对每个夹具每个场景用 `testing.Benchmark` 跑 3 次取 ns/op 中位数，按 Global Constraints 的硬预算判定；不达标且不在 `perfTimeDebt` → 失败；在表中但已达标 → 失败提示删除。另外输出一行 `PERF <Name>/<Scene> <ns> <budget> <ok|over>` 供 uiperf 解析。

`perf_debt_test.go` 中两张表的类型：

```go
type perfDebt struct{ Measured string; Why string } // Measured 如 "3.1ms" 或 "dirty 4.2×"
var perfDamageDebt = map[string]perfDebt{}           // 键 = 夹具名
var perfTimeDebt = map[string]perfDebt{}             // 键 = "Name/Scene"
```

- [ ] **Step 2: 运行，确认失败**

Run: `go test ./ui -run 'TestPerf' -count=1`
Expected: 编译失败（`perfFixture` 等未定义）。

- [ ] **Step 3: 实现骨架**

按 Interfaces 实现 `perf_harness_test.go` 与 `runner.go` 字段。`perf_bench_test.go`：

- `BenchmarkComponents`：`b.Run(fx.Name, ...)` 下分场景：
  - `Mount`：先在计时外 mount 一次预热缓存，然后 `for b.Loop() { d := newPerfDriver(perfPage(fx.Build), false); d.Frame() }`。
  - `MountCold`：同 Mount 但不预热；只在新进程里以 `-benchtime 1x` 运行才有意义（uiperf 负责），只报告不判定。
  - `Unchanged`：`Settle()` 后 `for b.Loop() { d.Frame() }`。
  - `Interact`：`Settle()` 后每步 `d.Advance(16*time.Millisecond); fx.Interact(r, i); d.Frame()`。
  - `Large1k` / `Large10k` / `Large100k`：页面换成 `fx.Large(n)`，交互同上（`LargeInteract` 或默认滚动）。
  - 每个子基准都调用 `b.ReportAllocs()`，并 `b.ReportMetric(dirty/viewport, "dirty/op")`，其中 dirty 是最后一步的 `perfDirtyArea`。
- `BenchmarkComponentsRaster`：只做 `Interact` 场景，`newPerfDriver(..., true)`，仅供参考。

- [ ] **Step 4: 写种子夹具**

`ui/perf_fx_seed_test.go` 中 `func init() { registerPerf(...) }`，覆盖五类路径各一个：
- `Button`：Interact 偶数 `D.Move(r.Center())`，奇数 `D.Move(geom.Point{X: 2, Y: 2})`。
- `Input`：Build 绑定预置 30 字中英混合文本；Interact 偶数先点中输入框再 `D.Type("a")`，奇数 `D.Key(input.KeyBackspace, 0)`。
- `Select`：Interact 偶数点击打开，奇数 `D.Key(input.KeyEscape, 0)`；Large 用 n 个选项、下拉打开，滚动默认。
- `Table`：6 列，Build 50 行；Interact 偶数悬停第 3 行，奇数移出；Large 用 `Virtual()` + n 行（写法参考 `ui/table_bench_test.go:benchVirtualTable`）。
- `Modal`：`Overlay: true`，Build 打开状态；Interact 偶数关闭（Escape），奇数重新打开（置 `open = true`）。
典型属性和文字以 `examples/ui-gallery` 对应页面为准。

- [ ] **Step 5: 运行检查与基准**

Run: `go test ./ui -run 'TestPerf' -count=1`
Expected: PASS。不达标的种子组件已登记进 `perfDamageDebt`，`Measured` 填实测值，`Why` 写 "baseline"。

Run: `go test ./ui -run '^$' -bench 'BenchmarkComponents/(Button|Table)' -benchtime=200x -count=1`
Expected: 输出 `BenchmarkComponents/Button/Mount`、`/Unchanged`、`/Interact` 和 `Table/Large100k` 等行，带 `allocs/op` 与 `dirty/op`。

Run: `go test ./ui -count=1 -short`
Expected: PASS（`lastDirty` 不改变行为）。

- [ ] **Step 6: 提交**

```bash
git add ui/runner.go ui/perf_harness_test.go ui/perf_bench_test.go ui/perf_check_test.go ui/perf_debt_test.go ui/perf_fx_seed_test.go docs/superpowers/specs/2026-11-16-ui-component-performance-design.md docs/superpowers/plans/2026-11-16-ui-component-performance.md
git commit -m "test(ui): component performance harness with seed fixtures"
```

---

### Task 2–7: 各组件族夹具

每个任务一个文件，格式与 Task 1 相同。每个任务的步骤：

- [ ] **Step 1:** 在 `ui/perf_fx_<family>_test.go` 中用 `registerPerf` 登记下表中的全部入口：每个入口作为 `Name`，或进入同族某夹具的 `Covers`，或作为带 `Skip` 理由的夹具。典型实例与交互取自 gallery 对应页面；集合类必须提供 `Large`。
- [ ] **Step 2:** 从 `perfCoveragePending` 删除本任务覆盖的名字。
- [ ] **Step 3:** Run `go test ./ui -run 'TestPerf' -count=1`。Expected：除新增 damage 不达标项外全部 PASS。把不达标项登记进 `perfDamageDebt`（`Why: "baseline"`），再跑一次直到 PASS。
- [ ] **Step 4:** Run `go test ./ui -run '^$' -bench 'BenchmarkComponents/(<本族名字正则>)' -benchtime=50x -count=1`。Expected：每个夹具都有输出，没有 panic。
- [ ] **Step 5:** 提交 `git add ui/perf_fx_<family>_test.go ui/perf_debt_test.go && git commit -m "test(ui): performance fixtures for <family>"`。

**Skip 判定规则：** 只有不产生可见节点的函数可以 Skip，例如 `State`、`Prop`、`Key`、`UseApp`、`CloseContextMenu`、`ContextMenuOpen`、`SetHoverOpen`、`MessageDestroy`、`NotificationDestroy`、`HoverDelay`、`ScreenBreakpoint`、`RouteContext`。命令式弹出（`Message`、`Notification`、`Confirm`、`MessageBox`、`Prompt`、`OpenDialogs`）**不能** Skip：它们的夹具在 Build 中按打开状态调用，Interact 一开一关。包装器（`ConfigProvider`、`ProSettingsProvider`、`Disabled`、`Custom`）用"包一个 Button"作为夹具。

| Task | 文件后缀 | 入口 |
|---|---|---|
| 2 通用与布局 | `general` | Button 以外的：ButtonGroup IconButton Icon Link Text Title Paragraph Kbd Divider VDivider Space Spacer Gap Flex Row Col Column GridRow Div StackBox Compact AspectRatio Splitter SplitterPanel Layout Header Footer Content Sider CollapsibleSider Scroll ScrollH Masonry Watermark BorderBeam App UseApp State Prop Key ConfigProvider Disabled Custom HoverDelay ScreenBreakpoint |
| 3 数据展示与反馈 | `display` | Avatar AvatarGroup Badge BadgeRibbon Tag CheckableTag Empty Image QRCode Statistic StatisticTimer Countdown Progress Spin Loading Skeleton SkeletonAvatar SkeletonButton SkeletonImage SkeletonInput Result Alert Card CardMeta CardAction Descriptions Timeline Tooltip Popover Popconfirm Collapse Carousel Calendar Segmented Rate Markdown RichText CodeBlock JSONView JSONViewSource JSONViewValue |
| 4 数据录入 | `entry` | Input 以外的：InputNumber InputTag TextArea Search OTP AutoComplete Mentions Checkbox CheckboxGroup CheckAll Radio RadioGroup Switch SwitchValue Toggle ToggleGroup Slider SliderRange ColorPicker ColorPickerGradient ColorPickerPanel ColorPickerPanelGradient DatePicker DatePickerPanel RangePicker MultiDatePicker DateTimePicker TimePicker TimePanel TimeRangePicker TimeSelect Upload DropZone Form FieldSet Item ItemGroup Addon HotkeyInput MarkdownEditor |
| 5 导航与浮层 | `nav` | Modal 以外的：Menu Menubar Tabs Breadcrumb BreadcrumbItems Pagination Steps Anchor AnchorSection Affix BackTop FloatButton FloatButtonGroup Dropdown ContextMenuAt ContextMenuAtWith CloseContextMenu ContextMenuOpen SetHoverOpen Drawer Overlay Message MessageDestroy MessageBox Notification NotificationDestroy NotificationCenter Confirm Prompt OpenDialogs Tour Command |
| 6 集合与大数据 | `collection` | Select 以外的：MultiSelect VirtualSelect TreeSelect Cascader CascaderPanel Transfer List ListActions ListItemMeta Listy VirtualList Tree VirtualTree InfiniteScroll LogView（只建基准，`perfTimeDebt` 中 `Why` 写 "owned by perf/memory-logview"）。另给 Task 5 的 Menu、Tabs 夹具补 `Large`（n 个菜单项 / n 个标签） |
| 7 Pro、IDE 与对话 | `pro` | ProLayout PageContainer PageHeader GridContent FooterToolbar DefaultFooter SettingDrawer ProSettingsProvider RouteContext ProTable EditableProTable ProList ProCard ProDescriptions ProForm ProFormFieldSet ProFormGroup ProFormItems ProFieldOf ProSkeleton QueryFilter LightFilter SchemaForm StepsForm ModalForm DrawerForm LoginForm LoginFormPage CheckCard CheckCardGroup StatisticCard StatisticCardGroup Dock StatusBar StatusBarItem TitleBar WindowControls NativeView Bubble BubbleList Conversations Sender Prompts ThoughtChain ToolCall Approval。ProTable、ProList、BubbleList、Conversations 需要 `Large` |

Task 7 完成时 `perfCoveragePending` 必须为空。删掉这个变量，并删掉 `TestPerfCoverage` 中放行它的分支。若源码在执行期间新增了入口，就在发现它的任务里补上夹具。

另外，Task 7 还要新建 `examples/ui-gallery/perf_pages_test.go`：
- `BenchmarkGalleryPages/<page.Name>/{Unchanged,Interact}` 遍历 `gallery.Pages()` 中 `!gallery.IsVariant(p)` 的页面，复用同包已有的 `nopRenderer`、`hintWin`、`newDash()`、`newOverlayState()`（见 `examples/ui-gallery/damage_verify_test.go`），窗口 1440×900、Scale 2。Interact 定义为：在页面中心滚动，DY 按 `i%40<20` 取 +120，否则取 −120。
- `TestGalleryPageBudgets` 仅在 `TANGO_PERF_BUDGET=1` 时运行，判定 Interact 是否 ≤ 4 ms。不达标的页面登记在同文件的 `galleryTimeDebt`，同样按棘轮处理。

---

### Task 8: `internal/cmd/uiperf` 工具

**Files:**
- Create: `internal/cmd/uiperf/main.go`、`parse.go`、`bench.go`、`hot.go`、`ab.go`、`load.go`、`report.go`，以及测试 `parse_test.go`、`hot_test.go`、`ab_test.go`、`load_test.go`

**Interfaces:**
- `uiperf bench [-count 3] [-run regex] -out dir`：
  - 运行 `go test ./ui -run '^$' -bench 'BenchmarkComponents/<regex>' -benchmem -count N` 和 `TANGO_PERF_BUDGET=1 go test ./ui ./examples/ui-gallery -run 'Test(Perf|GalleryPage)Budgets' -v`；
  - 冷启动：对每个夹具在新进程里执行一次 `go test` 编出的测试二进制（`-test.run '^$' -test.bench '^BenchmarkComponents/<Name>/MountCold$' -test.benchtime 1x`），记入 `cold_ns` 列，只报告；
  - 写出 `dir/bench.json` 和 `dir/status.md`：每个组件×场景一行，列为 ns/op 中位数、allocs/op、B/op、dirty/op、预算、是否达标，另有冷启动列。
- `uiperf hot [-top 30] -out dir`：
  - 对 `bench.json` 中超预算的每个"组件/场景"，单独执行 `go test ./ui -run '^$' -bench '^BenchmarkComponents/<Name>/<Scene>$' -benchtime=2s -cpuprofile dir/prof/<Name>-<Scene>.pprof`；
  - 再用 `go tool pprof -top -cum -nodecount=60` 读取文本，解析出 (函数, flat ns, cum ns)；
  - 得分 = Σ_组件 (cum 占比 × 超预算倍数)；
  - 输出 `dir/hot.md`：按得分排序，列出函数、得分、出现在哪些组件的 top 60 里、是否属于非目标区域（包路径前缀 `github.com/tangoui/tango/internal/render`、`/uihost`、`/macoslite`、`/ui/terminal`、`/ui/editor`）。
- `uiperf ab -base <git-rev> -bench <regex> [-rounds 5] [-pkg ./ui]`：
  1. 用 `git worktree add --detach $TMPDIR/uiperf-base-<sha7> <rev>` 建出 base 树，两边各 `go test -c -o` 一个二进制。
  2. 每轮先做负载闸门，然后交替执行 base/head（每轮交换先后），参数 `-test.run '^$' -test.bench <regex> -test.benchmem -test.count 1`。
  3. 输出每个基准两边的 ns/op 与 allocs/op 中位数、变化百分比，以及结论：`keep`（ns/op ≥ 10% 或 allocs ≥ 20% 改善且没有任何项回退 > 5%）、`regress`、`noise`。
  4. 退出时删除临时 worktree。
- 负载闸门 `loadGate(read func() (float64, error), sleep func(time.Duration), max float64, waitMax time.Duration) error`：负载 ≥ 6 时每 30 s 重读一次，超过 `waitMax`（默认 10 min）返回错误，A/B 本轮作废。负载读自 `sysctl -n vm.loadavg`，输出格式为 `{ 11.53 20.60 27.67 }`。

- [ ] **Step 1: 写测试**
  - `parse_test.go`：
    - `TestParseBenchLine`：输入 `BenchmarkComponents/Table/Large100k-10  	    1234	    812345 ns/op	  0.1250 dirty/op	   2048 B/op	      17 allocs/op`，得到 Name=`Table`、Scene=`Large100k`、NsOp=812345、Dirty=0.125、BOp=2048、AllocsOp=17。
    - `TestParseBudgetLine`：解析 `PERF Button/Interact 2500000 2000000 over`。
  - `hot_test.go`：
    - `TestParsePprofTop`：固定一段 `go tool pprof -top -cum` 文本，解析出函数名与 cum 占比。
    - `TestHotScore`：两个组件共享函数 F（cum 50%、超预算 2×；cum 30%、超预算 1.5×），F 的得分为 1.45；并且 `internal/render` 下的函数被标为非目标区域。
  - `ab_test.go`：`TestABVerdict` 覆盖三种结论，每种一个用例，其中包括"ns/op 改善 12%，但另一项回退 6%"的情况，结论应为 `regress`。
  - `load_test.go`：`TestLoadGate`：读数依次为 7.2、6.5、5.1 时等待两次后通过；一直是 8 时超时返回错误；`TestParseLoadavg` 解析 `{ 11.53 20.60 27.67 }` 得 11.53。
- [ ] **Step 2:** Run `go test ./internal/cmd/uiperf -count=1`。Expected：编译失败。
- [ ] **Step 3:** 实现。只用标准库加 `os/exec`。
- [ ] **Step 4:** Run `go test ./internal/cmd/uiperf -count=1`。Expected：PASS。
- [ ] **Step 5:** 冒烟：
  - Run `go run ./internal/cmd/uiperf bench -count 1 -run 'Button|Table' -out /tmp/uiperf-smoke`。Expected：生成 `status.md`，其中有 Button 和 Table 的各场景行。
  - Run `go run ./internal/cmd/uiperf ab -base HEAD -bench 'BenchmarkComponents/Button/Interact' -rounds 2`。Expected：结论为 `noise`，临时 worktree 已删除（`git worktree list` 中没有 `uiperf-base`）。
- [ ] **Step 6:** 提交 `git add internal/cmd/uiperf && git commit -m "tools(uiperf): component benchmark runner, hotspot ranking and A/B"`。

---

### Task 9: 第二层：协议、runner 与三个基础场景

**Files:**
- Create:
  - `tools/uibench/README.md`、`setup.sh`、`build.sh`、`.gitignore`（忽略 `third_party/`、`bin/`、`results/`）
  - `tools/uibench/proto/go.mod`（module `tango.local/uibench/proto`）、`proto/proto.go`
  - `tools/uibench/runner/go.mod`（module `tango.local/uibench/runner`）、`main.go`、`proc_darwin.go`、`windowserver_darwin.go`、`stats.go`、`judge.go`，以及测试 `stats_test.go`、`judge_test.go`、`runner_test.go`
  - `tools/uibench/tango/go.mod`（require tango，`replace github.com/tangoui/tango => ../../..`）、`main.go`、`scene_base.go`
  - `tools/uibench/mygo/go.mod`（require `github.com/egoist/mygo v0.9.0`）、`main.go`、`scene_base.go`
  - `tools/uibench/nsgo/go.mod`（`replace github.com/nsgo-dev/nsgo => ../third_party/nsgo`）、`main.go`、`scene_base.go`
  - `tools/uibench/scenes.md`

**Interfaces:**
- 协议（`proto` 包，三家都用）：`proto.Frame(work int)` 每做完一帧调用一次，`work` 是本帧实际改动的项数。第一次调用打印 `bench ready`。之后每秒打印三行：`bench frames <fps>`、`bench work <每秒累计 work>`、`bench intervals_us <逗号分隔>`。函数 `proto.Renderer(name string)` 打印 `bench renderer <name>`，Tango 传入 `uihost.Renderer(r)` 的返回值。
- 场景程序：`<bin> -scene <name>`，窗口尺寸由场景决定，未知场景退出码为 2。
- runner：`runner -bin ../bin -scenes counter,list,grid -frameworks tango,mygo,nsgo -rounds 5 -settle 3s -seconds 10 -out ../results/<date>.json`。
  - 每轮按轮转顺序启动每个框架的程序，等 `bench ready`（超时 30 s），把窗口置前（`osascript -e 'tell application "System Events" to set frontmost of (first process whose unix id is <pid>) to true'`）。
  - 静置 settle 时长后采样 seconds 秒，指标包括：
    - 进程 CPU（`proc_pid_rusage` flavor 2 的 user+system，乘以 `mach_timebase_info` 换算）；
    - WindowServer CPU（`ps -o time= -p <pid of WindowServer>` 采样前后之差）；
    - fps、work/s、帧间隔 p50/p99、phys_footprint（rusage 第 9 个 uint64）；
    - 启动时间（exec 到 ready）、渲染路径。
  - 每轮前后做负载闸门（逻辑与 uiperf 相同，代码各写一份），轮内负载 ≥ 6 的数据标记 `disturbed`，最多补测 3 次。
  - 结果文件保留全部原始轮次。
- `judge.go`：`func judge(t, o []Round) Verdict`，按 Global Constraints 的"更快"口径判定，对 disturbed 轮次剔除后计算。返回 `Faster`、`NotFaster` 或 `Invalid`。工作量差 > 5% 或渲染路径不一致时返回 `Invalid`。

**基础场景（尺寸与每帧工作量必须三家一致）：**

| 场景 | 窗口 | 内容 | 每帧 |
|---|---|---|---|
| counter | 360×260 | 一个数字和三个按钮 | ready 后不再变化（静止 CPU 与内存），work=0 |
| list | 800×600 | 10 万行虚拟列表，每行三段文字 | 滚动 8 逻辑像素，work=1 |
| grid | 800×600 | 600 个格子，各自背景色，数字每帧变化 | 600 个数字全部更新，work=600 |

counter 场景的判定只比较 CPU 和内存，不比较帧率。

- [ ] **Step 1: 写测试**
  - `stats_test.go`：`TestMedianIQR`（奇数个、偶数个各一例）；`TestP99`（1000 个间隔样本）。
  - `judge_test.go`：
    - `TestJudgeFaster`：Tango CPU 中位数是对手的 0.45，区间不重叠，fps 和 p99 不差 → `Faster`。
    - `TestJudgeHalfNotEnough`：0.55 → `NotFaster`。
    - `TestJudgeOverlap`：0.48 但区间重叠 → `NotFaster`。
    - `TestWorkMismatch`：工作量差 6% → `Invalid`。
    - `TestRendererConsistent`：同场景内 Tango 一轮走 native、一轮走 metal → `Invalid`。
  - `runner_test.go`：`TestDisturbedRoundsDropped`：5 轮中 2 轮 disturbed，判定只用另外 3 轮。`TestParseProtocol`：解析上述四种输出行。
- [ ] **Step 2:** Run `cd tools/uibench/runner && GOWORK=off go test ./... -count=1`。Expected：编译失败。
- [ ] **Step 3:** 实现 runner 和 proto。`setup.sh <nsgo源码目录>`：创建符号链接 `third_party/nsgo`，并把该目录的 `CHANGELOG.md` 首个版本号和目录树哈希（`find <dir> -type f -name '*.go' | sort | xargs shasum | shasum`）写入 `third_party/nsgo.version`。`build.sh`：对三个应用模块执行 `GOWORK=off CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ../bin/<fw>`。若某框架在 `CGO_ENABLED=0` 下构建失败，就对该框架改用 `CGO_ENABLED=1`，并在 README 和结果里注明。
- [ ] **Step 4:** 实现三家的基础场景。
  - Tango：驱动方式为每帧调用 `c.WakeAt(c.Now())`，按 `c.Now()` 变化判断新帧，窗口用 `uihost.Open`。
  - MyGo：`mygo.NewWindow(mygo.WindowOptions{Content: ui.View(view)})`，view 里每帧 `c.Invalidate()`。
  - NSGo：`cx.RequestAnimationFrame` 链式驱动。
  - 列表行内容三家一致，都是 `"Row <i>"` 加 `i*7%1000`。
- [ ] **Step 5:** Run `cd tools/uibench/runner && GOWORK=off go test ./... -count=1`。Expected：PASS。然后执行 `./setup.sh /Users/admin/min/outputs/tango-nsgo-perf-20261115/sources/nsgo && ./build.sh`，再跑 `go run ./runner -scenes counter,list,grid -rounds 1 -seconds 3 -out results/smoke.json`。Expected：三家都 ready，结果里有 9 组数据，Tango 每个场景都有 `renderer` 字段。
- [ ] **Step 6:** 提交 `git add tools/uibench && git commit -m "tools(uibench): out-of-process cross-framework runner with base scenes"`（`bin/`、`third_party/`、`results/` 已被 `.gitignore` 排除，提交前用 `git status --porcelain tools/uibench` 确认）。

---

### Task 10: 直接可比的组件场景

**Files:**
- Modify: `tools/uibench/{tango,mygo,nsgo}/main.go`（场景表）、`tools/uibench/scenes.md`
- Create: `tools/uibench/{tango,mygo,nsgo}/scene_components.go`

- [ ] **Step 1: 核对交集。** 读 MyGo `ui/`（`~/go/pkg/mod/github.com/egoist/mygo@v0.9.0/ui`）和 NSGo `ui/` 的源码文件与导出函数，在 `scenes.md` 写出"组件 → MyGo 实现 → NSGo 实现"表。下表中的场景，只有三家都有对应组件才保留；任何一家缺失的，移到 Task 11。
- [ ] **Step 2: 实现场景。** 每个场景三家一份，都放在 800×600 窗口里：

| 场景 | 内容 | 每帧工作（work） |
|---|---|---|
| buttons | 60 个按钮，6 列 | 第 `f%60` 个按钮的文字改成 `"Clicked <f>"`（1） |
| inputs | 20 个单行输入框，预置 20 字 | 第 `f%20` 个追加一个字符，超过 40 字后重置（1） |
| textarea | 一个多行输入，2000 字 | 末尾追加一个字符，到 4000 字重置（1） |
| checkboxes | 100 个复选框 | 切换第 `f%100` 个（1） |
| radios | 20 组，每组 5 个 | 第 `f%20` 组选中下一个（1） |
| switches | 100 个开关 | 切换第 `f%100` 个（1） |
| sliders | 20 个滑块 | 第 `f%20` 个的值 +1，超过 100 回到 0（1） |
| progress | 50 个进度条 | 全部 +1（50） |
| select | 一个选择框，1000 个选项，下拉打开 | 下拉列表滚动 1 行（1） |
| tabs | 10 个标签，各有 20 行文字内容 | 每 10 帧切换一次标签（帧内 work=1 或 0） |
| menu | 竖向菜单，50 项 | 选中第 `f%50` 项（1） |
| tooltip | 一个按钮带提示 | 每 30 帧切换一次显示/隐藏 |
| dialog | 模态框，内含 3 个输入和 2 个按钮 | 每 30 帧切换一次打开/关闭 |
| toast | 每 30 帧弹出一条消息，1 秒后消失 | 弹出当帧 work=1 |
| table | 1 万行 × 6 列 | 滚动 1 行（1） |
| tree | 1 万个节点，全部展开 | 滚动 1 行（1） |
| datepicker | 日期面板打开 | 选中日期 +1 天（1） |
| colorpicker | 颜色面板打开 | 色相 +1（1） |
| pagination | 共 1000 页 | 当前页 +1，到底回到 1（1） |

  状态变化都由程序在帧回调里直接修改，不模拟指针输入。

- [ ] **Step 3: 冒烟。** 先 `./build.sh`，再运行 `go run ./runner -scenes <全部直接场景> -rounds 1 -seconds 3 -out results/smoke-components.json`。Expected：每个场景三家都 ready，`judge` 不返回 `Invalid`。若返回 `Invalid`，修正工作量或渲染路径后重跑。
- [ ] **Step 4: 提交** `git add tools/uibench && git commit -m "tools(uibench): directly comparable component scenes"`。

---

### Task 11: 就近对手场景与映射表

**Files:**
- Modify: `tools/uibench/scenes.md`，以及三家的 `scene_components.go` 或新建 `scene_nearest.go`

- [ ] **Step 1: 写映射表。** 对每个非 Skip 的 Tango 夹具（以 Task 7 结束时 `perfFixtures()` 的结果为准），在 `scenes.md` 的映射表里写一行，内容为：Tango 组件、对比场景、每家用什么组合实现，以及为什么认为等价。规则：
  - 独立成场景（三家都实现）的组件：Cascader（对手用 combobox 加多列 list）、Transfer（两个 list 加按钮）、TreeSelect、Calendar、Steps、Timeline、Descriptions、Form（10 个字段）、Upload 列表、Carousel、Collapse、Drawer、Tour（popover 加遮罩）、ProTable（table 加查询表单）、Segmented、Rate、Tag 列表、Badge 列表、Card 网格、Statistic 网格。每帧工作量写法同 Task 10。
  - 其余组件归到最接近的已有场景（例如 Kbd、Link、Text 归 buttons；InputNumber、OTP、Search 归 inputs），并写明归类理由。
  - 对手完全无法组合出近似物的组件，标"无对手，只看硬预算"，并写明原因。
- [ ] **Step 2: 实现** Step 1 中独立成场景的项。
- [ ] **Step 3: 冒烟。** 同 Task 10 Step 3。
- [ ] **Step 4: 提交** `git commit -m "tools(uibench): nearest-counterpart scenes and component mapping"`。

---

### Task 12: 基线测量与状态表

**Files:**
- Create: `docs/performance/ui-components.md`

- [ ] **Step 1:** 负载 < 6 时运行 `go run ./internal/cmd/uiperf bench -count 3 -out /tmp/uiperf-baseline` 和 `go run ./internal/cmd/uiperf hot -out /tmp/uiperf-baseline`。
- [ ] **Step 2:** 负载 < 6 时运行 runner，覆盖全部场景，`-rounds 5`。
- [ ] **Step 3:** 写 `docs/performance/ui-components.md`：
  - 方法：两层口径和硬预算，引用设计稿；机器、负载、Tango commit、MyGo 版本、NSGo 版本与哈希。
  - 逐组件状态表，列为：组件、场景、Tango 进程内各项、对应的跨框架场景、对 MyGo 和对 NSGo 的判定、硬预算、状态。状态取值为：完成 / 待优化 / 受限于渲染器（转交）/ 无对手只看预算。
  - 热点表：`hot.md` 前 30 项。
  - 转交清单：热点属于非目标区域的，写明证据和应转交的会话。
  - 把 `perfTimeDebt` 填成基线实测值（`TANGO_PERF_BUDGET=1` 时 `TestPerfBudgets` 必须 PASS）。
- [ ] **Step 4:** Run `TANGO_PERF_BUDGET=1 go test ./ui ./examples/ui-gallery -run 'Test(Perf|GalleryPage)Budgets' -count=1`。Expected：PASS。
- [ ] **Step 5:** 提交 `git add docs/performance/ui-components.md ui/perf_debt_test.go examples/ui-gallery/perf_pages_test.go && git commit -m "docs(perf): ui component baseline and hotspot ranking"`。

---

### Task 13+: 优化批次（重复执行，直到达到停止条件）

每一批是一个任务，编号依次为 Task 13、14、……，由控制者在账本里登记"批次 K：目标热点"后派发。每批都按以下步骤执行。

- [ ] **Step 1: 选目标。**
  - 从最新的 `hot.md` 里取得分最高、且不属于非目标区域的 1–3 个函数，优先选被多个组件共享的。
  - 若其中涉及核心文件，按 Global Constraints 检查其他会话的改动是否重叠。
  - 在账本写明目标、预期收益和受影响的夹具。
- [ ] **Step 2: 先锁定行为。** 对要改的函数，补充能证明行为不变的测试，例如绘制命令哈希不变、damage 结果不变、布局矩形不变。测试必须在改动前就能通过。
- [ ] **Step 3: 改。** 一次只做一个优化点，例如减少分配、加缓存、跳过不必要的重建或重测量，或者在组件内补 `c.Damage` 声明，让交互帧走局部重绘。
- [ ] **Step 4: A/B。** 运行 `go run ./internal/cmd/uiperf ab -base <本批起点 commit> -bench 'BenchmarkComponents/(<受影响的夹具>)/' -rounds 5`。结论不是 `keep` 就回滚这个优化点，并在账本里记录原因。
- [ ] **Step 5: 护栏。** 跑 Global Constraints 的每批护栏；跑 `go test ./ui -run 'TestPerf' -count=1`，按棘轮删掉已达标的 debt 项；跑 `TANGO_PERF_BUDGET=1 ... Budgets`，把已改善的 `perfTimeDebt` 实测值更新为新数字。
- [ ] **Step 6: 提交。** 每个保留下来的优化点单独提交，提交信息写明改动和 A/B 数字，例如 `perf(ui): reuse text measure cache across frames (Table/Interact -23%, allocs -41%)`。
- [ ] **Step 7: 跨框架复测。** 每批结束后，对受影响的场景跑 runner（`-rounds 5`），更新 `docs/performance/ui-components.md` 的状态表与批次记录，然后重新生成 `hot.md`。
- [ ] **Step 8: 判停。**
  - 每个组件同时满足以下条件，即标为完成：所有场景满足硬预算，`perfTimeDebt` 和 `perfDamageDebt` 中都没有它的条目；有对手的场景判定为 `Faster`。
  - 剩余差距如果有证据表明来自渲染器（WindowServer CPU 或渲染提交占大头，而构建、布局、绘制命令部分已经低于对手的一半），标为"受限于渲染器（转交）"，附上 profile。
  - 全部组件都是"完成"或"受限于渲染器"时，项目结束，转入 superpowers:finishing-a-development-branch。
  - 否则开下一批。
