# 规范对齐 第 1 批：基础和表单输入 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 第 1 批（设计 §6）的组件逐项对照 antd 6.7 / Element Plus 2.14.8 / shadcn v4，写对照记录，补齐缺口（含行为测试、gallery 演示、`ConfigProvider.Defaults`、a11y），默认值不符的直接改并写 CHANGELOG。

**Architecture:** 按文件拆成 A、B、C、D 四组，每组一个实施 agent 并行，独占自己的文件；共享文件（主题核心、gallery 注册与混合页、API 黄金文件、CHANGELOG、AUTHORING 总表）只由主线程合并。各组把要改共享文件的地方写进自己记录文件末尾的「共享文件改动请求」，主线程合并后统一验证、按组提交。

**Tech Stack:** Go 1.27；参考源码 `/tmp/tango-ref`：antd@6.7.0（`antd/package/es/<组件>`，令牌 `es/<组件>/style/token`）、element-plus@2.14.8（`ep/package/es/components/<名>`、`ep/package/theme-chalk/src`）、shadcn-ui/ui@e62c9435fff729a175f38983dee73b2cb4f6a069（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）、tailwindcss@4.3.5（`tw/package`）。

**Spec:** [docs/specs/2026-11-14-spec-alignment-design.md](../specs/2026-11-14-spec-alignment-design.md)（§3、§4、§5、§6 第 1 批与「每一批的步骤」、§7、§7.1）；记录模板 [docs/specs/align/README.md](../specs/align/README.md)；编写约定 [ui/AUTHORING.md](../../ui/AUTHORING.md)；API 规则 [docs/release.md](../release.md)「API 稳定性」R1–R12。

## Global Constraints

- 只在 worktree `/Users/admin/min/tango-align`（分支 `spec-align`）里做；不碰 `/Users/admin/min/tango` 和其它 worktree；不用裸 `git stash`。
- 实施 agent **不提交**；主线程在验证后按组提交。提交作者和提交者固定 `tango <903300326@qq.com>`（`git -c user.name=tango -c user.email=903300326@qq.com commit`），**不加** Co-Authored-By；不推送、不合并。
- 精确值从 `/tmp/tango-ref` 源码读，不估值；记录里写源码路径和日期 2026-11-14。没核对的写「依据知识，未核对」。
- 四套规范冲突时取功能并集，一个组件只有一套 API（§7.1）；默认值按主题不同的，走 `ui/theme/comp_<名>.go` 令牌或 `ConfigProvider.Defaults`（`Prop(c, "<组件>", "<键>", def)`），不在组件里按 `th.Flavor` 分支（结构差异除外，写注释）。
- 默认值或行为不符合规范的直接改，不加兼容开关；每条写进本组记录的「CHANGELOG 草稿」（主线程合并进 `CHANGELOG.md` 的 `## [未发布]` → `### 变更`，不进 `[0.1.0]`）。
- 稳定层 API（`api/stable/ui.txt`、`api/stable/ui_theme.txt` 列出的符号，以及 stable 包 `ui/icon`、`ui/a11y`、`ui/geom`）只能**新增**；需要破坏才能对齐的，结论写「暂不做（需破坏稳定层 API：…）」并在汇报里列出。新符号默认实验层，按 R1–R12 写（链式方法、不加参数、枚举只追加、字符串键是 API）。
- 去重（AUTHORING §13、§14；设计 §7.1）：shadcn Field = `FormItem`；InputGroup = `Compact`/`Addon`/输入框前后缀（`InputGroup` 是已删除旧名，不要恢复）；NativeSelect、Combobox 先映射到 `Select`（含搜索）/`AutoComplete`；Toggle = 已有 `Toggle`。只有确实没有对应物时才新建，文件名 `shad_` 前缀，组件名不带 shad。
- 缺基础能力（paint、text、layout、input、Context/Runner/overlay/driver 层）时不在组件里绕开，结论写「暂不做，缺 X」，记入记录末尾「基础能力缺口」。
- AI 对话组件不在范围（comp-chat 分支）。
- 新代码与注释用英文；文档用中文；界面文案走 Locale（`TestNoHardcodedCJK`）。数据库规则不适用（无数据库）。
- Ant 主题是参考实现：除记录中写明的规范修正外，Ant 主题外观不能变；Element / shadcn 的差异只放在各自令牌里。

## 文件归属

每个文件只有一个主人。不在下表里的文件一律只读；需要改就写「共享文件改动请求」。

| 组 | 组件 | 独占的源文件 | 独占的主题文件 | 独占的 gallery 文件 | 新建 |
|---|---|---|---|---|---|
| A 基础与布局 | Button（含 IconButton、ButtonGroup）、Link、Icon、Typography（Text/Title/Paragraph）、Divider、Space/Flex、Layout（Layout/Header/Sider/Content/Footer，不含 ProLayout 部分）、Grid（GridRow/Col）、Segmented、Toggle/ToggleGroup、Rate | `ui/button.go`、`ui/button_group.go`、`ui/text.go`、`ui/data_typography.go`、`ui/display.go`（只改 Divider 相关代码）、`ui/nav_space.go`、`ui/nav_grid.go`、`ui/prolayout.go`（只改 `Layout/Header/Sider/Content/Footer` 及其私有辅助）、`ui/entry_segmented.go`、`ui/shad_toggle.go`、`ui/entry_rate.go` 及它们同名的 `_test.go` | `ui/theme/comp_button.go`、`comp_typography.go`、`comp_divider.go`、`comp_layout.go`、`comp_segmented.go`、`comp_rate.go` | `page_general.go`、`page_button_group.go`、`page_layout.go`、`page_entry_rate.go`、`page_shad2a_entry.go` | `docs/specs/align/1-basic.md`、`ui/align_b1a_test.go`、`ui/theme/align_b1a_test.go`、`examples/ui-gallery/page_b1a.go` |
| B 输入与表单 | Input（含 Password）、TextArea、Search、InputNumber、OTP、Compact/Addon（shadcn InputGroup）、Form/FormItem（shadcn Field）、Checkbox/CheckboxGroup/CheckAll、Radio/RadioGroup、Switch、Slider/SliderRange、Upload/DropZone | `ui/input.go`、`ui/entry_inputx.go`、`ui/entry_otp.go`、`ui/entry_compact.go`、`ui/form.go`、`ui/choice.go`、`ui/entry_checkall.go`、`ui/p2b_slider.go`、`ui/entry_upload.go`、`ui/p2d_upload.go`、`ui/p2e_drop.go` 及同名 `_test.go` | `comp_input.go`、`comp_inputnumber.go`、`comp_form.go`、`comp_checkbox.go`、`comp_radio.go`、`comp_switch.go`、`comp_slider.go`、`comp_upload.go` | `page_p2d_entry.go`、`page_shad2a_form.go`、`page_p2b_slider.go` | `docs/specs/align/1-input.md`、`ui/align_b1b_test.go`、`ui/theme/align_b1b_test.go`、`examples/ui-gallery/page_b1b.go` |
| C 选择器 | Select、VirtualSelect（SelectV2）、MultiSelect、AutoComplete、Mentions、InputTag、Cascader/CascaderPanel、TreeSelect、Transfer，以及 shadcn NativeSelect、Combobox | `ui/select.go`、`ui/selector.go`、`ui/fill_virtual_select.go`、`ui/entry_multiselect.go`、`ui/entry_autocomplete.go`、`ui/entry_mentions.go`、`ui/entry_inputtag.go`、`ui/entry_cascader.go`、`ui/entry_cascader_panel.go`、`ui/entry_treeselect.go`、`ui/entry_transfer.go` 及同名 `_test.go`；确需新建时 `ui/shad_<名>.go` | `comp_select.go`、`comp_cascader.go`、`comp_transfer.go` | `page_entry_select.go`、`page_fill_virtual_select.go`、`page_p2b_select.go`、`page_p2b_transfer.go`、`page_p2b_tree.go`、`page_shad2a_tree.go` | `docs/specs/align/1-select.md`、`ui/align_b1c_test.go`、`ui/theme/align_b1c_test.go`、`examples/ui-gallery/page_b1c.go` |
| D 日期时间与颜色 | DatePicker、DatePickerPanel、RangePicker、DateTimePicker、TimePicker/TimePanel、TimeRangePicker、TimeSelect、ColorPicker/ColorPickerPanel | `ui/date.go`、`ui/date_util.go`、`ui/date_multi.go`、`ui/fill_date_panel.go`、`ui/p2a_date.go`、`ui/p2a_time_range.go`、`ui/p2a_range_time.go`、`ui/entry_daterange.go`、`ui/entry_datetime.go`、`ui/entry_time.go`、`ui/more_timeselect.go`、`ui/entry_color.go`、`ui/fill_color_panel.go` 及同名 `_test.go` | `comp_datepicker.go`、`comp_colorpicker.go` | `page_entry_date.go`、`page_date_more.go`、`page_p2a_date.go`、`page_fill_panels.go`、`page_more_timeselect.go`、`page_shad2a_date.go` | `docs/specs/align/1-datetime.md`、`ui/align_b1d_test.go`、`ui/theme/align_b1d_test.go`、`examples/ui-gallery/page_b1d.go` |

**主线程独占（共享文件）：** `ui/theme/theme.go`、`ui/theme/components.go`、`ui/theme/shadcn*.go`、`ui/theme/cssvars*.go`、`ui/theme/components_*test.go`、其它 `ui/theme/*.go`；`ui/context.go`、`runner.go`、`overlay.go`、`driver.go`、`edit*.go`、`hover.go`、`layers.go`、`reveal.go`、`nav_config.go`；`api/**`；`CHANGELOG.md`；`ui/AUTHORING.md`；`examples/ui-gallery/{main.go,audit*.go,gallery/**}` 和混合页 `page_form.go`、`page_entry_input.go`、`page_entry_misc.go`、`page_fill_misc.go`、`page_display.go`、`page_nav_layout.go`、`page_shadcn.go`、`page_p2c_display.go`、`page_data_misc2.go`；`ui/golden_test.go`、`ui/ui_test.go`、`ui/testdata/**`。

**唯一例外 —— Locale：** 组件自己的 `XxxLocale` 结构体（`ui/locale.go`）和各语言包里该组件那一段（`ui/locale_packs.go` 等）由该组件的组可以改，**只用 Edit 工具做小块插入**（不整文件重写、不 `sed -i`、不 `gofmt -w` 这些文件），并在「共享文件改动请求」里列出已改的字段。新增顶层 `Locale` 字段仍走请求。

**并行纪律：** 四组同时改 `ui` 包。每次保存都要让 `go build ./ui/...` 能过（先加完整的小块再测试）；构建失败的位置在别组文件里时不要动它，过一会儿再跑；只跑自己的测试（`go test ./ui -run '<自己的测试名>'`）。共享辅助函数（如 `input.go` 的 `paintFrame`、`date_util.go`、`selector.go`、`choice.go` 的勾选框外壳）被别组组件调用：改它们时别组组件的外观必须不变，除非规范要求，并在记录里写明影响面。

---

## Review Focus

1. **Ant 主题外观意外变化**：只有记录里写明的规范修正才能改 Ant 主题外观；其它组件截图逐像素不变。→ Task 5 Step 3 对比 `origin/main` 与本分支 `-theme antd` 截图，差异只出现在 CHANGELOG「变更」列出的组件上。
2. **禁用和只读**：每个新增的交互（新按钮、新键盘路径、新清除图标）在 `Disabled(true)` 或 `ConfigProvider.ComponentDisabled` 下不响应，查询方法返回 false。→ 每组测试文件里为每个新增交互加一条 `Test…Disabled…`。
3. **受控值越界或为空**：外部传入的值不在选项里、超出 min/max、空切片、nil 指针（允许 nil 的 API）时不 panic，显示与 antd 一致（例如 Select 显示原值、InputNumber 失焦时夹到范围内）。→ 每组为改过的受控组件加一条 `Test…OutOfRange…` 或 `Test…UnknownValue…`。
4. **键盘与无障碍**：新增的可交互部分可 Tab 到达、有焦点环、有 a11y 角色和名称（`Driver.A11yFind`），名称来自 Locale。→ 每组至少一条 `Test…A11y…`，并跑 gallery `-audit`（Task 5）。
5. **暗色与三套主题**：新绘制只用令牌，不判断 `th.Dark`；三套主题 × 明暗都能渲染，文字对比度过 `TestGalleryTextContrast`。→ 每组 gallery 新页在 Task 5 跑 `-theme all` 与对比度测试。

---

### Task 0（主线程）：参考源码与计划

**Files:** Create `docs/plans/2026-11-14-spec-align-b1.md`（本文件）

- [ ] **Step 1:** 按 [docs/specs/align/README.md](../specs/align/README.md) 的表把 antd、element-plus、tailwindcss 用 `npm pack` 解到 `/tmp/tango-ref/{antd,ep,tw}/package`，shadcn 稀疏克隆并 checkout `e62c9435fff729a175f38983dee73b2cb4f6a069`（本批不需要 pro）。验证：`ls /tmp/tango-ref/antd/package/es/button /tmp/tango-ref/ep/package/es/components/input-number /tmp/tango-ref/shadcn-ui/apps/v4/registry/new-york-v4/ui/field.tsx` 有输出。
- [ ] **Step 2:** 派发 Task 1–4 的实施 agent（并行），每组完成后派审阅 agent。

---

### Task 1–4（每组一个，结构相同）：对照、补齐、测试、演示

各组按「文件归属」表操作，下面的步骤每组都走一遍。**每组的已知缺口**写在步骤后面，它们是起点，不是全部；对照中发现的其它缺口同样处理。

**Interfaces（各组共同）：**
- Consumes：`ui.Prop[T](c, comp, key, def)`、`c.ThemeFor(theme.CompX)`、`drawFocusRing`、`paintFrame`、`c.itemStatus`、`Locale()`、`ui.Driver`（见 AUTHORING §5、§7、§9、§12）。
- Produces：本组记录文件、实现、测试、gallery 页；记录末尾三个汇总节：「基础能力缺口」（README 格式）、「CHANGELOG 草稿」（分「新增」「变更」两段，一条一行，变更写旧值 → 新值和依据）、「共享文件改动请求」（每条写文件、改什么、为什么；已改的 Locale 段也列在这里；AUTHORING §14 需要更新的行也写在这里）。

- [ ] **Step 1: 对照记录。** 在本组记录文件里每个组件一张表（README 模板），规范项至少覆盖：功能（API 表逐项）、默认值、受控和非受控、交互和键盘、禁用和加载、尺寸和令牌（antd `style/token.ts` 的组件令牌与 EP `theme-chalk` 的 CSS 变量，读源码给出数值）、明暗主题、shadcn v4 外观（读 `new-york-v4/ui/<名>.tsx` 的 class：高度、圆角、阴影、焦点 ring、`aria-invalid` 外观）。Tango 现状写 `文件:符号`。结论五选一。JS 特有的写法（ref、render prop、classNames/styles 语义化、`props` 字段映射、`name`/`id`/`validate-event`）写「等价写法」或「不需要」并说明。

- [ ] **Step 2: 先写失败的测试。** 每个「已补齐」项至少一条行为测试，写在 `ui/align_b1<组>_test.go`；令牌数值与源码比对的测试写在 `ui/theme/align_b1<组>_test.go`（断言值即源码值，注释写源码路径）。运行 `go test ./ui -run '<测试名>' -v` 确认失败（编译失败或断言失败）。

- [ ] **Step 3: 实现。** 新方法按 AUTHORING：链式修饰、查询方法放链尾、值由应用持有；结构/行为默认值用 `Prop(c, "<组件>", "<键>", def)` 接入 `ConfigProvider.Defaults`（键名用 antd 的 prop 名，如 `"variant"`、`"iconPlacement"`、`"mode"`）；视觉数值加进本组的 `comp_<名>.go` 令牌并给三套主题赋值；a11y 用组件已有的 `semantics` 写法补角色、名称、状态。v6 改名的属性（如 `iconPosition` → `iconPlacement`、Space `direction` → `orientation`、`split` → `separator`）：Tango 已有旧名方法的，新增新名方法，旧方法加 `// Deprecated: use X.`（不删除）。

- [ ] **Step 4: 运行测试。** `go test ./ui -run '<本组测试名前缀>|<被改组件已有测试>' ` 与 `go test ./ui/theme -run '<本组>'` 通过；`go vet ./ui` 无新告警。已有测试因规范修正而失效的，改期望值并在 CHANGELOG 草稿里对应一条，不删用例。

- [ ] **Step 5: gallery。** 新能力在本组可改的页或 `examples/ui-gallery/page_b1<组>.go` 里演示（新页用 `Parent: "<组件>"` 并到父页面下，Name 形如 `<组件>Align`）。运行 `go run ./examples/ui-gallery -only '<页名>' -theme all -out /tmp/spec-align-b1/<组>` 三套主题和明暗都能出图；用 Read 打开 antd 与 shadcn 暗色两张图看一眼，确认没有明显错位或低对比度。

- [ ] **Step 6: 自查并交回。** 列出本组改动和新建的文件（`git status --short -- <本组文件>`），确认没有改到归属表以外的文件（Locale 段除外）。不提交。

#### Task 1 — 组 A 已知缺口（源码已核对，2026-11-14）

- Button `iconPlacement: 'start' | 'end'`（antd `es/button/Button.d.ts:27`，`iconPosition` 已弃用）：新增 `(*ButtonW).IconPlacement(p string) *ButtonW` 与 Defaults 键 `"iconPlacement"`；测试 `TestButtonIconPlacementEnd`：`"end"` 时图标的 x 大于文字的 x，RTL 下相反。
- Space `orientation`、`separator`（`es/space/index.d.ts:31,35`，`direction`、`split` 已弃用）：对照现有 `SpaceW` 方法，缺的按新名补；测试 `TestSpaceSeparator`：三个子项之间出现两个分隔节点，首尾没有。
- Divider v6 `orientation`/`titlePlacement`、`vertical`、`plain`、`variant`（`es/divider/index.d.ts`）逐项对照 `display.go` 现有方法。
- Typography、Link、Icon、Grid、Layout、Segmented、Toggle、Rate：按 Step 1 逐项对照（EP `el-link` 的 `underline` 取值、`el-text` 的 `truncated/line-clamp`、`el-row` 的 `justify/align`、`el-segmented` 的 `direction`、shadcn `toggle.tsx` 的 `variant: default|outline` 与 `size: default|sm|lg`）。

#### Task 2 — 组 B 已知缺口

- InputNumber `mode: 'input' | 'spinner'`（antd 6 新增，`es/input-number/index.d.ts`）：新增 `(*InputNumberW).Mode(m string) *InputNumberW` 与 Defaults 键 `"mode"`；`"spinner"` 时两侧为 −/+ 按钮；测试 `TestInputNumberSpinnerMode`：点 + 两次值从 1 到 3，到 `Max` 后 + 禁用。EP `controls-position="right"` 已有 `ControlsPosition`，对照即可。
- OTP `separator`（`es/input/OTP/index.d.ts:36`）：新增 `(*OTPW).Separator(s string) *OTPW`（Go 里用字符串；antd 的按索引函数写「等价写法」：新增 `SeparatorFunc(func(i int) string)` 视需要）；测试 `TestOTPSeparator`：长度 6、分隔符 "-" 时有 5 个分隔文字。
- Input `variant` 含 `underlined`（antd 6）：对照 `input.go` 现有 variant 取值，缺的补；测试 `TestInputVariantUnderlined`。
- shadcn Field（field.tsx）对照 `FormItem`：描述、错误、横向布局、`FieldSet/FieldLegend` → 写等价写法或补到 `FormItem`；InputGroup（input-group.tsx）对照 `Compact/Addon` 与输入框前后缀，缺的能力（例如组内按钮、文本、带 textarea 的组）补到已有入口。
- Checkbox、Radio、Switch、Slider、Upload、Form：按 Step 1 逐项对照（EP `el-switch` 的 `inline-prompt/active-action-icon/before-change`、`el-slider` 的 `show-input/marks/vertical`、`el-upload` 的 `limit/on-exceed/list-type=picture-card`）。

#### Task 3 — 组 C 已知缺口

- Select `showSearch`、`mode`、`maxTagCount`、`variant`（含 `underlined`）、`allowClear`、`labelRender/optionRender`、`placement`、`open/onOpenChange`：逐项对照 `select.go`、`entry_multiselect.go`、`selector.go`；EP `el-select` 的 `filterable/remote/allow-create/collapse-tags/collapse-tags-tooltip/reserve-keyword`、`el-select-v2` 对照 `VirtualSelect`。
- shadcn NativeSelect（native-select.tsx）：结论应为「等价写法：`Select`」（自绘无原生下拉）；若有外观差异（尺寸 sm/default）写入 Select 令牌。Combobox（combobox.tsx）：对照 `Select` 搜索 + `AutoComplete`，缺的能力补到已有入口，不新建同义组件，除非确实没有对应物。
- AutoComplete、Mentions、InputTag、Cascader、TreeSelect、Transfer：按 Step 1 逐项对照（EP `el-input-tag` 的 `trigger/draggable/max`、`el-cascader` 的 `props.checkStrictly/lazy` 在 Go 里是方法、`el-transfer` 的 `filterable/target-order`）。

#### Task 4 — 组 D 已知缺口

- DatePicker/RangePicker/TimePicker 的 `variant`、`placement`、`needConfirm`、`order`、`minDate/maxDate`、`multiple`、`presets`、`showNow`、`changeOnScroll`（antd `es/date-picker/generatePicker/interface.d.ts`、rc-picker 类型）：逐项对照。
- EP `el-date-picker` 的 `type` 全集（year/years/month/months/dates/week/datetime/datetimerange/daterange/monthrange/yearrange）、`unlink-panels`、`default-time`、`shortcuts`；`el-time-select` 的 `start/end/step/min-time/max-time/include-end-time`；`el-color-picker` 的 `show-alpha/predefine/color-format`、`el-color-picker-panel`。
- ColorPicker antd `mode: 'single' | 'gradient'`、`disabledAlpha`、`disabledFormat`、`format`、`showText`、`trigger`、`presets`：逐项对照 `entry_color.go`。

---

### Task 1R–4R：审阅（每组一个审阅 agent）

- [ ] **Step 1:** 审阅 agent 读本计划和该组记录文件，用 `git diff -- <该组文件>` 与 `git status` 看改动；抽查至少 5 条「已对齐/已补齐」结论与 `/tmp/tango-ref` 源码是否一致（数值、默认值）；检查 Global Constraints（稳定层只增、无 Flavor 分支、无硬编码文案、无归属外改动、Disabled 生效、a11y 名称来自 Locale）。
- [ ] **Step 2:** 运行该组测试 `go test ./ui -run '<该组>'`、`go test ./ui/theme`。
- [ ] **Step 3:** 输出问题清单（严重 / 一般 / 建议），实施 agent 按清单修复后由主线程确认。

---

### Task 5（主线程）：合并共享文件、验证、提交

**Files:** Modify `CHANGELOG.md`、`api/go/*.txt`（生成）、`ui/AUTHORING.md`（§14 表）、按请求修改的共享文件；Create 无。

- [ ] **Step 1: 合并请求。** 逐组读「共享文件改动请求」，合并进共享文件；「CHANGELOG 草稿」合并进 `## [未发布]` 的 `### 新增`/`### 变更`（一组一条总条目加子项）。
- [ ] **Step 2: rebase 与构建。** `git fetch origin && git rebase origin/main`（先把各组改动按组提交，再 rebase）；`go build ./...`；`go vet ./ui/... ./uihost/...`；`go test ./ui/... ./uihost/...`（`ui/editor` 的计时/性能用例失败时单独重跑确认是否是负载导致）。
- [ ] **Step 3: gallery。** `go test -timeout 60m ./examples/ui-gallery/...`；`go test ./examples/ui-gallery -run TestGalleryTextContrast -timeout 30m`；`go run ./examples/ui-gallery -audit` 与 `origin/main` 上同一命令的输出比较，没有新增 FAIL。Ant 截图对比见 Review Focus 1。
- [ ] **Step 4: API。** `go run ./internal/cmd/apitxt -w && go run ./internal/cmd/apitxt -check` 退出 0；`bash scripts/apidiff.sh` 没有稳定层破坏。
- [ ] **Step 5: 提交并汇报。** 按组提交（`feat(ui): align <组> with antd 6.7 / Element Plus 2.14.8 / shadcn v4`）加一条 `chore: batch 1 API files, changelog`；汇报每组补了什么、改了什么默认值、暂不做清单、基础能力缺口、需要用户决定的事、没验证的项。
