# Defaults 与 FooterToolbar 验收实施计划

> 历史实施计划，实现及主要回归代码已合入 main。下方原步骤用于追溯，不是当前待办，不应重复执行产品修改或重放历史提交。

**归档复核：** 2026-11-16，源码基线 `116b9e63`，集成提交 `9877d9a`。

| 原任务 | 已核实的落地内容 | 提交依据 |
| --- | --- | --- |
| Task 1 | Defaults 审计矩阵和声明边界已记录 | `00e4c24` |
| Task 2 | Defaults 覆盖测试已加入；该批未新增公开 Defaults key | `24fac6a`，仅修改 `ui/defaults_more_test.go` |
| Task 3 | 不支持 blur/高对比度的 opaque fallback 及状态回归代码已落地 | `40c1ba4`、`bd57ada`、`4632cf3` |
| Task 4 | 主题、RTL、damage、示例和高对比度白名单代码已落地 | `a7db7dc`、`5718b3d` |
| Task 5 | AUTHORING、覆盖记录和变更说明已更新 | `eb1e2ad` |

**验收记录边界：** 本次只核对已提交源码和提交关系，没有重跑 Go/API 检查、截图、GPU strict verify 或跨平台构建，也未复核所有历史执行日志。以下流程不是这些检查通过的证明；历史红灯只针对修复前版本，当前已修复基线不要求再次失败。

**Goal:** 以现有 `ui-coverage.md` 的根 `ui` Defaults 清单为权威，验证已声明的 ConfigProvider.Defaults 契约，并完成 FooterToolbar 在三套主题、状态切换、无 blur renderer、RTL 和局部重绘下的可执行验收。

**Architecture:** 不引入反射式 Defaults 系统，也不为未声明的 Chat、Dock、Graph、Viewer、Shell、DiffView 新增公开 Defaults key。Defaults 工作沿用现有 `Prop` / `ConfigProp` / 组件 helper；FooterToolbar 继续使用 `FooterToolbarToken`、Runner 的 `noBlur` 和 `paint.BackdropBlur`，只在 no-blur/HighContrast 路径改为 `theme.BgContainer` 的 alpha 255 不透明背景，并用现有 display-list、damage presenter 和 gallery strict verify 验证。

**Tech Stack:** Go 1.27、Tango `ui`/`ui/theme`/`paint`、CPU/GPU paint list、`ui-gallery` Driver/a11y/mirror/damage 验收、`apitxt` API golden。

**Spec:** `docs/superpowers/specs/2026-11-15-defaults-footer-toolbar-design.md`

## Global Constraints

- Defaults 审计权威是 `docs/specs/ui-coverage.md` §6.1 的根 `ui` 177 个公开入口。
- 本轮只处理已声明 Defaults 契约；未声明的 CodeBlock、JSONView、LogView、Dock、StatusBar、TitleBar、NotificationCenter、HotkeyInput、Chat、Graph、DiffView 记录为“未声明”，不新增公开 key。
- Chat 若未来纳入，按 `Bubble`、`Sender` 等独立组件命名，不建立 `Defaults("Chat", ...)`。
- `ui/graph`、`ui/editor/diffview`、`ui/mobile`、`ui/chart`、`ui/editor` 不纳入本轮根 `ui` 分母。
- FooterToolbar no-blur renderer 与 HighContrast 使用 `theme.BgContainer` 的 alpha 255 不透明背景。
- FooterToolbar RTL 使用逻辑布局：Extra 在 inline-start，actions 在 inline-end，RTL 时物理左右交换。
- 不使用 `git add -A`；提交只包含本计划涉及的文件。
- 不宣称编辑器 M2 自动化验收通过；若性能测试受负载影响，报告单测和独立复测结果。

## Review Focus

- 已声明 Defaults 的显式 modifier 必须覆盖 provider 默认，尤其是 `size`、`format`、`placeholder`、`allowClear` 和复用组件 key；由 Task 2 的表驱动覆盖测试固定。
- 嵌套 ConfigProvider 的内层默认必须覆盖外层，内部 panel 明确不继承的边界不能泄漏；由 Task 2 的嵌套/边界测试固定。
- FooterToolbar 首帧尚未 affixed 时不能错误发 blur，滚动到 affixed、脱离 affixed、Target 容器和 resize 后必须正确切换；由 Task 3 的帧序列测试固定。
- no-blur renderer 和 HighContrast 不能只“不发 BackdropBlur”，最终 Fill alpha 必须是 255 且来自 BgContainer；由 Task 3 的 display-list 断言固定。
- blur reach 内外的内容变化不能都退化成整帧，旧/新 bar 区域切换必须被 damage 覆盖；由 Task 4 的 damage presenter / strict verify 测试固定。

---

### Task 1: 建立 Defaults 审计矩阵

**Files:**
- Modify: `docs/specs/ui-coverage.md:351-405`（现有 §6.1 Defaults 清单）
- Test/Inspect: `ui/defaults_more_test.go`、各组件已有 Defaults helper、`ui/nav_config.go`

**Interfaces:**
- Consumes: 根 `ui` 177 入口的现有清单、`ConfigProvider.Defaults`、`Context.ConfigProp`、已有 `Prop`/helper。
- Produces: §6.1 中可逐行复核的审计结论，区分 `已声明且已接线`、`已声明但未接线`、`未声明，不是 bug`、`复用其他组件 key`、`内部 panel 明确不继承`、`显式覆盖不适用`。

- **原步骤 1: 写审计脚本/记录草稿，不改构造器**
  - 从 `ui-coverage.md §6.1` 的 177 入口逐项核对 `ConfigProp`、`Prop(c, ...)`、helper 和复用入口。
  - 将 `CodeBlock` 等未声明的新组件明确标记“未声明，不是 bug”，不要放入已声明分母。
  - 将 `TimeRangePicker` 不继承 `TimePicker` 的现有测试列为正式边界。

- **原步骤 2: 验证既有 Defaults 回归测试**
  - 运行：`go test -count=1 ./ui -run 'TestDefaultsWiring|TestButtonAutoInsertSpaceRules|TestMessageMaxCount|TestFieldSetDefaults|TestTimeRangePanelsIgnoreTimePickerDefaults'`
  - 预期：PASS；若矩阵发现已声明但没有测试的项，记录到 Task 2 的测试清单，不先扩公开 API。

- **原步骤 3: 更新 `ui-coverage.md` 的矩阵说明**
  - 写清审计分母、结论分类、复用 key 和 panel 继承边界。
  - 不新建第二份 Defaults 权威文档，不改未声明组件的 API。

- **原步骤 4: 独立检查文档一致性**
  - 运行：`git diff --check`。
  - 预期：只有 Defaults 审计文字变化；若发现真实漏接，转入 Task 2，不在本任务混合实现。

- **原步骤 5: Commit**
  ```bash
  git add docs/specs/ui-coverage.md
  git commit -m "docs(ui): define the ConfigProvider defaults audit matrix"
  ```

### Task 2: 补齐已声明 Defaults 的行为测试和真实漏接

**Files:**
- Modify: 由 Task 1 确认的具体构造器/helper 文件（只列真实漏接）
- Modify: `ui/defaults_more_test.go`
- Test: `ui/defaults_more_test.go`、必要时对应组件测试
- Modify: `api/go/ui.txt` only if a real new Defaults key is explicitly required by an existing declared contract

**Interfaces:**
- Consumes: Task 1 的审计矩阵；现有 `ConfigProvider(c).Defaults(component, props)`。
- Produces: 已声明 Defaults 的默认生效、显式覆盖、嵌套 provider 和错误输入回归测试；不产生未批准的新组件 Defaults 契约。

- **原步骤 1: 为审计发现的覆盖缺口写失败测试**
  - 每个测试至少包含：默认生效、显式 modifier 覆盖、错误类型回退。
  - 对有嵌套边界的组件增加：outer default → inner override → constructor value。
  - 未声明 key 和未知 key 必须保持不 panic 且不改变内置默认。

- **原步骤 2: 运行相关测试确认失败**
  - 运行：`go test -count=1 ./ui -run 'TestDefaultsWiring|TestDefaultsNested|TestDefaultsInvalidType|Test.*Panel.*Defaults'`（按实际测试名调整）。
  - 历史红灯要求：仅适用于对应修复前版本；本次未重验当时的执行顺序，当前回归不要求再次失败。

- **原步骤 3: 以现有显式 helper 接线**
  - 构造器创建状态后调用对应 helper；沿用 `sizeSet`、`formatSet`、`phSet` 等标志。
  - 不使用 reflection；不把复杂回调对象放进 Props。
  - 如果没有真实漏接，保留纯测试和矩阵，不修改产品代码。

- **原步骤 4: 运行 Defaults 全集**
  - 运行：`go test -count=1 ./ui -run 'TestDefaults|TestButtonAutoInsertSpaceRules|TestMessageMaxCount|TestFieldSetDefaults|TestTimeRangePanelsIgnoreTimePickerDefaults'`。
  - 预期：全部 PASS，显式 modifier 优先级和内部 panel 边界固定。

- **原步骤 5: API 检查并提交**
  - 若未新增 Defaults key：运行 `go run ./internal/cmd/apitxt -check`，确认无 golden 变化。
  - 若确有已声明契约缺口需要新 key：先检查 `api/go/ui.txt` diff，运行 `go run ./internal/cmd/apitxt -w` 后再 `-check`。
  - 历史提交 `24fac6a` 仅包含以下测试文件；有其他真实变更时须逐文件列出，没有 golden 变更时不暂存 `api/go/ui.txt`。
  ```bash
  git add ui/defaults_more_test.go
  git commit -m "test(ui): cover declared ConfigProvider defaults"
  ```

### Task 3: 修正 FooterToolbar 的 no-blur/HighContrast 回退并覆盖状态矩阵

**Files:**
- Modify: `ui/pro2_page.go:125-140`（surface fallback）
- Test: `ui/blur_test.go`
- Test: `ui/pro2_test.go` only when reusing existing semantic assertions

**Interfaces:**
- Consumes: `theme.FooterToolbarToken`、`Runner.c.noBlur`、`paint.BackdropBlur`、`Affix.Affixed()`、`FooterToolbar.Target`。
- Produces: no-blur/HighContrast 的 opaque `theme.BgContainer` Fill；flow/sticky/Target/resize 的严格行为测试。

- **原步骤 1: 写 no-blur 和状态转换失败测试**
  - 对每个 Ant/Element/shadcn 的 light/dark/HighContrast light/dark 构造 driver。
  - 按帧序列验证：首帧未 affixed → 滚到 affixed → 继续滚动 → 滚回脱离 → resize。
  - 重复默认 viewport 和 `Target(scrollBox.ID())`。
  - no-blur / HighContrast 断言：无 `paint.BackdropBlur`，且 FooterToolbar 区域的 Fill alpha 为 255、颜色为 `theme.BgContainer`。

- **原步骤 2: 运行测试确认 no-blur 断言失败**
  - 回归复查命令：`go test -count=1 ./ui -run 'TestFooterToolbarFrosted|TestFooterToolbarNoBlurRenderer|TestFooterToolbarOpaqueFallbacks'`。
  - 历史红灯要求仅适用于修复前版本。基线已包含 opaque fallback 修复，当前回归应通过；本次没有重新执行该命令或验证历史红灯记录。

- **原步骤 3: 修正 `FooterToolbarW.surface`**
  - 当 `HighContrast` 或 `c.noBlur` 且 bar affixed 时，调用 `b.Bg(th.BgContainer)`，不设置 `b.blur`。
  - flow 继续使用 token `tk.Bg`；支持 blur 且 affixed 继续使用 `tk.FrostedBg` + `tk.BlurRadius`。
  - 不改 FooterToolbar 公开 API。

- **原步骤 4: 运行完整 FooterToolbar 状态矩阵**
  - 运行：`go test -count=1 ./ui -run 'TestFooterToolbar|TestPro2Semantics'`。
  - 预期：PASS；BackdropBlur 的 radius/rect、opaque fallback、语义角色和按钮键盘可达性均满足。

- **原步骤 5: Commit**
  ```bash
  git add ui/pro2_page.go ui/blur_test.go ui/pro2_test.go
  git commit -m "fix(ui): use an opaque footer fallback without backdrop blur"
  ```

### Task 4: FooterToolbar damage、RTL、对比度和画廊验收

**Files:**
- Modify: `ui/blur_test.go`（damage presenter/帧序列/RTL 断言）
- Modify: `examples/ui-gallery/page_pro2_page.go`（扩展现有 FooterToolbar 展示）
- Modify: `examples/ui-gallery/highcontrast.go`（将 FooterToolbar 页面纳入 HC 白名单）
- Modify: `docs/specs/ui-coverage.md:321`（修正 shadcn FooterToolbar 旧矛盾记录）
- Test: `examples/ui-gallery/damage_verify_test.go`、现有 contrast/a11y/mirror 测试

**Interfaces:**
- Consumes: Task 3 的 FooterToolbar surface 行为、`runner_damage_test.go` 的 `damagePresenterWin`/`PresentDamage`、gallery 的 `-audit`/`-mirror-check`/strict verify。
- Produces: 可观察的 damage 验收矩阵和三套主题明暗/高对比度 gallery 页面。

- **原步骤 1: 写 damage 和 RTL 失败/回归测试**
  - 定义 full-frame：dirty rect 并集覆盖 viewport。
  - blur reach 内滚动必须包含 bar rect；reach 外变化不能额外扩大为 full-frame；flow↔sticky 必须覆盖旧/新 bar 区域；无事件不持续 full-frame。
  - RTL 断言 Extra 在 inline-start、actions 在 inline-end，物理左右按方向交换；bar rect 与 blur rect 一致。

- **原步骤 2: 扩展现有 gallery 页面和 HC 白名单**
  - 不新建重复 FooterToolbar 页面；在 `page_pro2_page.go` 增加普通流、sticky、Target、暗色/高对比度可观察状态。
  - 在 `highcontrast.go` 白名单加入 FooterToolbar 页面。
  - 更新 `ui-coverage.md`，明确 Element/shadcn blur 是 Tango 适配，不代表官方组件默认值。

- **原步骤 3: 运行 gallery 视觉与无障碍验收**
  - 运行：
    ```bash
    go run ./examples/ui-gallery -only FooterToolbar -theme all -scale 1 -out /tmp/footer-toolbar-shots
    go run ./examples/ui-gallery -only FooterToolbar -dir both -theme all -scale 1 -out /tmp/footer-toolbar-rtl
    go run ./examples/ui-gallery -only FooterToolbar -theme hc -scale 1 -out /tmp/footer-toolbar-hc
    go run ./examples/ui-gallery -only FooterToolbar -mirror-check
    go run ./examples/ui-gallery -only FooterToolbar -audit
    ```
  - 预期：页面能生成；缺名、缺角色、键盘不可达、镜像专用断言均为 0；人工查看三套主题明暗和 HC 截图。

- **原步骤 4: 运行 strict damage 和相关包测试**
  - 运行：`go test -count=1 ./ui/... ./examples/ui-gallery/...`。
  - 运行 gallery 的 GPU strict verify；若已知 blink/负载间歇失败，记录基线对照，不放宽断言。

- **原步骤 5: Commit**
  ```bash
  git add ui/blur_test.go examples/ui-gallery/page_pro2_page.go examples/ui-gallery/highcontrast.go docs/specs/ui-coverage.md
  git commit -m "test(ui): verify FooterToolbar themes and damage"
  ```

### Task 5: 全局验证、文档和交付

**Files:**
- Modify: `ui/AUTHORING.md`（Defaults 新增规则）
- Modify: `CHANGELOG.md` only if Task 2/3 修复了用户可见行为
- Modify: `docs/specs/ui-coverage.md` if Task 1/4 changed audit or visual records

**Interfaces:**
- Consumes: Tasks 1–4 的测试、矩阵和 FooterToolbar 行为。
- Produces: 可审查的 Defaults 结论、FooterToolbar 验收证据和最终提交。

- **原步骤 1: 更新开发规范**
  - 在 `ui/AUTHORING.md` 写明：只有明确声明 Defaults 的组件才进入矩阵；构造器读取、显式 modifier 覆盖、嵌套 panel 边界必须有测试；未声明组件不自动视为缺陷。

- **原步骤 2: 运行格式、静态检查和跨平台构建**
  ```bash
  gofmt -l <新增或修改的 Go 文件>
  go vet ./ui/...
  go test -count=1 ./ui/...
  go test -count=1 ./examples/ui-gallery/...
  GOOS=windows go build ./ui/...
  GOOS=linux go build ./ui/...
  GOOS=js GOARCH=wasm go build ./ui/...
  ```
  预期：gofmt 无输出，相关测试通过，三平台构建成功。

- **原步骤 3: API golden 检查**
  - 由于本轮不新增 Defaults key，先运行 `go run ./internal/cmd/apitxt -check`。
  - 若 Task 2 证实既有契约需要新 key，才运行 `go run ./internal/cmd/apitxt -w`，检查生成 diff，再运行 `-check`。

- **原步骤 4: 最终审查**
  - `git diff --check`。
  - 确认没有把未声明组件误写成 Defaults 缺陷，没有改变 M2 的验收结论，没有提交无关文件。
  - 复核 FooterToolbar 的 opaque fallback、Target 帧序列、RTL、HC、damage 和截图路径。

- **原步骤 5: Commit**
  ```bash
  git add ui/AUTHORING.md CHANGELOG.md docs/specs/ui-coverage.md
  git commit -m "docs(ui): document defaults and footer toolbar verification"
  ```

## 自检覆盖

- Defaults 权威范围、未声明组件边界、Chat 命名、子包排除：Tasks 1–2。
- FooterToolbar no-blur opaque fallback：Task 3。
- flow/sticky/Target/resize/HighContrast/主题/RTL：Tasks 3–4。
- damage 可观测 full-frame 定义与 GPU strict verify：Task 4。
- Ant Design 6.7 / Pro v6 约束：Task 1 的矩阵来源和 Task 4 的视觉记录；不引入新设计值。
- API、跨平台、gofmt/vet/test：Task 5。
