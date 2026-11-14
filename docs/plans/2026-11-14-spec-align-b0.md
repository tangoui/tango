# 规范对齐 第 0 批：参考源码与 shadcn v4 令牌 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 锁定四套规范的参考源码，建立逐组件对照记录的格式，并把 `theme.Shadcn` 换成 shadcn/ui v4 的变量值（oklch、默认圆角 10px、v4 的 base 与 color 主题）。

**Architecture:** 新增 oklch 解析；新增生成器 `internal/cmd/shadcngen`，从 shadcn `themes.ts` 和 Tailwind v4 `theme.css` 生成 `ui/theme/shadcn_data.go`（字符串表）；`Shadcn()` 改为从生成数据取变量。slate、gray 两个旧 base 保留现有 v3 hex 推导路径。

**Tech Stack:** Go 1.27，标准库；参考源码 element-plus@2.14.8、@ant-design/pro-components@3.1.14-2、antd@6.7.0、tailwindcss@4.3.5、shadcn-ui/ui@e62c9435fff729a175f38983dee73b2cb4f6a069。

**Spec:** [docs/specs/2026-11-14-spec-alignment-design.md](../specs/2026-11-14-spec-alignment-design.md)（§3、§4.3、§6 第 0 批、§7、§7.1）

## Global Constraints

- 在 worktree `/Users/admin/min/tango-align`（分支 `spec-align`，从 `origin/main` 开）里做，不碰主工作区。
- 提交作者和提交者固定为 `tango <903300326@qq.com>`，**不加** Co-Authored-By；只在 `spec-align` 上本地提交，不推送、不合并。
- 参考源码放在 `/tmp/tango-ref`，不进仓库；精确值从源码里读，不估值。
- 不破坏稳定层 API：`theme.ShadcnOptions` 是白名单符号，只能**加**字段；`ShadcnBases` 只能追加。
- Ant 主题、Element 主题的任何令牌都不能变（用现有测试和截图对比确认）。
- 默认值和视觉变化直接改，写进 CHANGELOG「变更」（用户定）。
- 新文件和注释用英文（与仓库一致），文档用中文。

## Review Focus

1. `oklch(... / 10%)` 这种带 alpha 的边框和输入框（v4 暗色）：解析后必须保留 alpha，`Border` 等令牌叠到背景上要正确，不能变成不透明白色。→ Task 2 测试 `TestParseOKLCHAlpha`，Task 4 测试 `TestShadcnV4DarkTranslucentBorder`。
2. 色域外的 oklch（彩色主题的高彩度值）：转换要裁剪到 0–255，不能溢出回绕。→ Task 2 `TestParseOKLCHClamp`。
3. 用户传 `Primary` 自定义主色加 `Color` 主题同时出现时：`Primary` 优先，焦点环跟随 `Primary`。→ Task 4 `TestShadcnPrimaryOverridesColor`。
4. 旧代码传 `Base: "slate"` / `"gray"`、或未知 base：前两者颜色与改动前完全一致，未知名回落到 neutral。→ Task 4 `TestShadcnLegacyBases`。
5. 暗色模式文字对比度：v4 改了 muted-foreground 等值，gallery 里不能出现新的低对比度文字。→ Task 5 跑 `TestGalleryTextContrast`。

---

### Task 1: worktree、参考源码与对照记录模板

**Files:**
- Create: `docs/specs/align/README.md`
- Add（从主工作区复制，未跟踪文件）: `docs/design-baseline.md`、`AGENTS.md`、`CLAUDE.md`、`docs/specs/2026-11-14-spec-alignment-design.md`、本计划文件
- Modify: `docs/design-baseline.md`（加 shadcn v4 一行）、`AGENTS.md`（加 shadcn v4 一句）

**Interfaces:**
- Produces: `/tmp/tango-ref/{ep,pro,antd,tw}/package/`、`/tmp/tango-ref/shadcn-themes.ts`、`/tmp/tango-ref/shadcn-ui`（浅克隆，`apps/v4/registry/new-york-v4/ui`）；后续批次的对照记录都按 `docs/specs/align/README.md` 的模板写。

- [ ] **Step 1: 建 worktree**

```bash
cd /Users/admin/min/tango && git fetch origin && git worktree add -b spec-align /Users/admin/min/tango-align origin/main
cd /Users/admin/min/tango-align && git config user.name tango && git config user.email 903300326@qq.com
```

- [ ] **Step 2: 准备参考源码**（已有的跳过）。`antd@6.7.0` 用 `npm pack` 解到 `/tmp/tango-ref/antd`；shadcn 仓库 `git clone --filter=blob:none --sparse https://github.com/shadcn-ui/ui /tmp/tango-ref/shadcn-ui && git -C /tmp/tango-ref/shadcn-ui sparse-checkout set apps/v4/registry && git -C /tmp/tango-ref/shadcn-ui checkout e62c9435fff729a175f38983dee73b2cb4f6a069`。

  验证：`ls /tmp/tango-ref/antd/package/es/button /tmp/tango-ref/shadcn-ui/apps/v4/registry/new-york-v4/ui | head` 有输出；`grep -c 'name: "' /tmp/tango-ref/shadcn-ui/apps/v4/registry/themes.ts` 为 24。

- [ ] **Step 3: 复制未跟踪文档进 worktree**，在 `docs/design-baseline.md` 的版本表追加一行：`| shadcn 主题（只用于 theme.Shadcn） | **shadcn/ui v4**（registry new-york-v4，仓库 commit e62c943，CLI shadcn@4.23.0；颜色取自 apps/v4/registry/themes.ts，灰阶取自 tailwindcss@4.3.5 theme.css） | 2026-11-14 核对 |`；`AGENTS.md` 的列表加 `- shadcn 主题对照 shadcn/ui v4（new-york-v4）。`

- [ ] **Step 4: 写 `docs/specs/align/README.md`**，内容：参考源码版本与路径（Global Constraints 里那 5 个）、每批记录文件命名 `docs/specs/align/<批号>-<组名>.md`、每个组件一张表，列为「规范项 | 官方定义（来源 文件或URL，日期） | Tango 现状（文件:符号） | 结论」，结论只能取 `已对齐 / 已补齐（测试名） / 等价写法（说明） / 不需要（说明） / 暂不做（缺什么）`；以及「基础能力缺口」汇总表格式。

- [ ] **Step 5: 提交**

```bash
git add AGENTS.md CLAUDE.md docs/design-baseline.md docs/specs/2026-11-14-spec-alignment-design.md docs/specs/align/README.md docs/plans/2026-11-14-spec-align-b0.md
git commit -m "docs: design baseline (antd 6.7, Pro v6, Element Plus 2.14.8, shadcn v4) and alignment records"
```

---

### Task 2: oklch 颜色解析

**Files:**
- Modify: `ui/theme/color.go`（新增函数；`ParseColor` 加 `oklch(` 分支）
- Test: `ui/theme/color_oklch_test.go`

**Interfaces:**
- Produces: `func ParseOKLCH(s string) (Color, error)`、`func OKLCH(l, c, h, alpha float64) Color`（l 为 0–1，c 原值，h 角度；超出 sRGB 的分量裁剪），`ParseColor` 接受 oklch 写法。

- [ ] **Step 1: 写失败的测试** `TestParseOKLCH`（表驱动，`HexString` 比较）：

```go
{"oklch(1 0 0)", "#ffffff"}, {"oklch(0.145 0 0)", "#0a0a0a"}, {"oklch(0.205 0 0)", "#171717"},
{"oklch(0.985 0 0)", "#fafafa"}, {"oklch(0.922 0 0)", "#e5e5e5"}, {"oklch(0.708 0 0)", "#a1a1a1"},
{"oklch(0.577 0.245 27.325)", "#e7000b"}, {"oklch(0.488 0.243 264.376)", "#1447e6"},
{"oklch(0.555 0.163 48.998)", "#bb4d00"},
{"oklch(98.5% 0 none)", "#fafafa"}, // Tailwind v4 theme.css 写法
```

  `TestParseOKLCHAlpha`：`oklch(1 0 0 / 10%)` → RGB `#ffffff`、`A == 26`；`oklch(1 0 0 / 0.15)` → `A == 38`。
  `TestParseOKLCHClamp`：`oklch(0.7 0.4 145)` 不报错，每个分量在 0–255（不回绕；比较 `OKLCH(0.7,0.4,145,1)` 的 G 分量为 255）。
  `TestParseOKLCHErrors`：`"oklch(1 0)"`、`"oklch(a b c)"`、`"oklch(1 0 0"` 返回错误。
  `TestParseColorOKLCH`：`ParseColor("oklch(0.205 0 0)")` 等于 `Hex("#171717")`。

- [ ] **Step 2: 运行确认失败** — `cd /Users/admin/min/tango-align && go test ./ui/theme -run 'OKLCH' -v`，预期编译失败（未定义）。

- [ ] **Step 3: 实现**。算法：OKLCH→OKLab（a=c·cos h，b=c·sin h），OKLab→LMS'（系数 0.3963377774、0.2158037573 / −0.1055613458、−0.0638541728 / −0.0894841775、−1.2914855480），立方，LMS→线性 sRGB（4.0767416621、−3.3077115913、0.2309699292 / −1.2684380046、2.6097574011、−0.3413193965 / −0.0041960863、−0.7034186147、1.7076147010），裁剪到 [0,1] 后做 sRGB 伽马（≤0.0031308 乘 12.92，否则 1.055·x^(1/2.4)−0.055），四舍五入到 uint8；alpha `round(a*255)`。L 支持 `0.5` 与 `50%`，h 支持 `none`（=0），alpha 支持小数与百分比。

- [ ] **Step 4: 运行确认通过** — 同上命令，预期全部 PASS；再跑 `go test ./ui/theme` 整包 PASS。

- [ ] **Step 5: 提交** — `git add ui/theme/color.go ui/theme/color_oklch_test.go && git commit -m "feat(theme): parse oklch colors"`

---

### Task 3: 生成器 shadcngen 与生成数据

**Files:**
- Create: `internal/cmd/shadcngen/main.go`、`internal/cmd/shadcngen/main_test.go`、`internal/cmd/shadcngen/testdata/themes.ts`、`internal/cmd/shadcngen/testdata/theme.css`
- Create（生成）: `ui/theme/shadcn_data.go`

**Interfaces:**
- Consumes: 无（只做文本解析，不依赖 Task 2）
- Produces（`ui/theme/shadcn_data.go`，未导出，文件头 `// Code generated by internal/cmd/shadcngen; DO NOT EDIT.`，注明两个来源的版本与 commit）：

```go
// shadcnThemeVars["neutral"].light["primary"] == "oklch(0.205 0 0)"
var shadcnThemeVars = map[string]struct{ light, dark map[string]string }{...}
// base 顺序与 themes.ts 一致：neutral stone zinc mauve olive mist taupe
var shadcnBaseNames = []string{...}
// color 主题：amber blue cyan emerald fuchsia green indigo lime orange pink purple red rose sky teal violet yellow
var shadcnColorNames = []string{...}
// tailwindV4Scales["mauve"][0] 是 50 号色，[10] 是 950 号色，值为 oklch 字符串
var tailwindV4Scales = map[string][11]string{...}
```

  判断 base / color：变量里含 `background` 的是 base，否则是 color 主题。`radius` 不进表（统一默认 10px）。

- [ ] **Step 1: 写失败的测试** `TestGenerate`：用 testdata 里裁剪的 `themes.ts`（neutral + amber 两项，各取 3 个变量，含一个 `"oklch(1 0 0 / 10%)"`）和 `theme.css`（neutral 与 mauve 的 50…950 共 22 行），调用 `generate(themesTS, themeCSS []byte, source string) ([]byte, error)`，断言输出能被 `go/format.Source` 接受，并包含 `"oklch(1 0 0 / 10%)"`、`shadcnBaseNames = []string{"neutral"}`、`shadcnColorNames = []string{"amber"}`、`"mauve": {"oklch(98.5% 0 none)"`。另测缺少 `--color-neutral-950` 时返回错误。

- [ ] **Step 2: 运行确认失败** — `go test ./internal/cmd/shadcngen -v`。

- [ ] **Step 3: 实现** `main.go`：参数 `-themes /tmp/tango-ref/shadcn-ui/apps/v4/registry/themes.ts -css /tmp/tango-ref/tw/package/theme.css -o ui/theme/shadcn_data.go -source "shadcn-ui/ui@e62c943, tailwindcss@4.3.5"`。逐行解析：`name: "x"` 开始一项，`light: {` / `dark: {` 切换，`"key": "value",` 或 `key: "value",` 记入；css 里 `--color-<family>-<step>: <value>;` 只收 neutral stone zinc mauve olive mist taupe slate gray 九个灰阶家族，每家必须 11 级齐全。输出按名字排序以保证稳定。

- [ ] **Step 4: 运行确认通过并生成** — `go test ./internal/cmd/shadcngen -v` PASS；执行上面的生成命令，`go vet ./ui/theme` 通过；`grep -c oklch ui/theme/shadcn_data.go` 大于 1000。

- [ ] **Step 5: 提交** — `git add internal/cmd/shadcngen ui/theme/shadcn_data.go && git commit -m "feat(theme): generate shadcn/ui v4 theme data"`

---

### Task 4: `Shadcn()` 改为 v4

**Files:**
- Modify: `ui/theme/shadcn.go`
- Modify（更新期望值）: `ui/theme/shadcn_test.go`、`ui/theme/shadcn_tokens_test.go`、`ui/shad_test.go`、`ui/mobile/shad_test.go`（只改因 v4 数值变化而失效的断言，不删用例）
- Test: `ui/theme/shadcn_v4_test.go`

**Interfaces:**
- Consumes: Task 2 `ParseOKLCH`，Task 3 的 `shadcnThemeVars / shadcnBaseNames / shadcnColorNames / tailwindV4Scales`
- Produces:
  - `ShadcnOptions` 新增字段 `Color string`（v4 color 主题名，空为不叠加；未知名忽略）。`Base` 注释改为列出 v4 base 加旧名 slate、gray。
  - `ShadcnBases` 变为 `neutral, zinc, slate, stone, gray, mauve, olive, mist, taupe`（只追加，原顺序不动）；新增 `var ShadcnColors = []string{...}`（等于 shadcnColorNames）。
  - 默认 `Radius` 由 8 改为 10。

- [ ] **Step 1: 写失败的测试** `ui/theme/shadcn_v4_test.go`：
  - `TestShadcnV4Neutral`：`Shadcn()` 的 `Shadcn.Background/Foreground/Primary/Border/Ring` 依次等于 `Hex("#ffffff")/#0a0a0a/#171717/#e5e5e5/#a1a1a1`，`Destructive == Hex("#e7000b")`，`RadiusLG == 10`、`Radius == 8`、`RadiusSM == 6`、`CardRadius == 14`。
  - `TestShadcnV4DarkTranslucentBorder`：`Shadcn(Dark())` 的 `Shadcn.Border` 为白色且 `A == 26`、`Shadcn.Input.A == 38`；`th.Border`（不透明令牌）等于 `Over(Shadcn.Input, Shadcn.Background)`；`Shadcn.SidebarPrimary == Hex("#1447e6")`。
  - `TestShadcnV4Bases`：对 `stone zinc mauve olive mist taupe` 各取 light `primary`，与 `ParseOKLCH(shadcnThemeVars[b].light["primary"])` 相等。
  - `TestShadcnV4Color`：`Shadcn(ShadcnOptions{Color: "amber"}).Shadcn.Primary == Hex("#bb4d00")`，背景仍是 neutral 的白色；暗色下 `Primary` 等于 amber dark `primary` 的解析值。
  - `TestShadcnPrimaryOverridesColor`：`ShadcnOptions{Color: "amber", Primary: Hex("#2563eb")}` 的 `Shadcn.Primary == Hex("#2563eb")`、`Shadcn.Ring == Hex("#2563eb")`。
  - `TestShadcnLegacyBases`：在改动前先用当前代码算出 `Shadcn(ShadcnOptions{Base: "slate", Radius: 8})` 和 `gray` 明暗两套的 `Background/Foreground/Primary/Border/Muted/MutedForeground` 的 hex，写进测试作为期望值；改动后保持一致。`Base: "nope"` 的 `Options.Base == "neutral"`。
  - `TestShadcnCSSVarsV4`：`Shadcn().CSSVars()["--radius"] == "0.625rem"`。

- [ ] **Step 2: 运行确认失败** — `go test ./ui/theme -run 'ShadcnV4|ShadcnLegacy|ShadcnPrimaryOverrides|ShadcnCSSVarsV4' -v`（`TestShadcnLegacyBases` 应通过，其余失败）。

- [ ] **Step 3: 实现**。v4 base：把 `shadcnThemeVars[base]` 的 light/dark 逐项解析进 `ShadcnVars`；有 `Color` 时用 color 主题的同名变量覆盖（primary、primary-foreground、secondary、secondary-foreground、chart-1…5、sidebar-primary、sidebar-primary-foreground）；`DestructiveForeground` 设为 `Hex("#ffffff")`（v4 删除了该变量，按钮用 text-white）。派生令牌里原来用 `s.at(n)` 的地方，v4 base 改用 `tailwindV4Scales[base]` 解析出的 scale（同一套 `scale` 类型）。含 alpha 的 `Border/Input/SidebarBorder` 保留在 `ShadcnVars`；写进 `Theme` 的不透明令牌（`t.Border`、`t.BorderSecondary`、`t.Split`、`t.CardBorder`、`t.PopupBorder`、`t.ModalBorder`、`t.SwitchOffBg`）先 `Over(..., v.Background)`。slate、gray 继续走现有 `tailwindScales` 的 v3 路径（把旧代码收进 `shadcnLegacyVars(s scale, dark bool) *ShadcnVars`）。`Primary` 覆盖逻辑放在 Color 叠加之后。

- [ ] **Step 4: 运行确认通过** — `go test ./ui/theme -v -run Shadcn` 全 PASS；然后 `go test ./ui/theme ./ui ./ui/mobile`，对因数值变化失败的旧断言逐个改成 v4 值（每处在提交说明里列出），Ant/Element 相关测试不得改动。

- [ ] **Step 5: 提交** — `git add ui/theme ui/shad_test.go ui/mobile/shad_test.go && git commit -m "feat(theme): shadcn/ui v4 colors, bases, color themes and 10px radius"`

---

### Task 5: 全量验证、API 文件与 CHANGELOG

**Files:**
- Modify: `api/go/ui_theme.txt`（`go run ./internal/cmd/apitxt -w` 生成）、`api/stable/ui_theme.txt`（不加新符号：`Color` 字段随 `ShadcnOptions` 已稳定；`ShadcnColors` 保持实验性）、`CHANGELOG.md`
- 可能修改：因对比度回归需要调整的 `ui/theme/shadcn.go` 派生令牌或 gallery 演示

- [ ] **Step 1: API 检查** — `go run ./internal/cmd/apitxt -w && go run ./internal/cmd/apitxt -check`，预期退出 0；`bash scripts/apidiff.sh` 不报稳定层破坏。

- [ ] **Step 2: 包测试** — `go vet ./ui/... ./uihost/... ./sys/theme/...` 与 `go test ./ui/... ./uihost/... ./sys/theme/... ./internal/cmd/shadcngen` 全部通过。

- [ ] **Step 3: gallery 对比度与审计** — `go test ./examples/ui-gallery -run 'TestGalleryTextContrast' -timeout 30m` 通过；`go run ./examples/ui-gallery -audit` 与改动前（在 `origin/main` 上跑一次存档）比较，没有新增的 shadcn 问题。有回归就调派生令牌（例如 `TextSecondary`），不改 v4 原始变量值；仍无法解决的写进汇报。

- [ ] **Step 4: 截图** — `go run ./examples/ui-gallery -theme shadcn -out /tmp/spec-align-b0/shadcn`，抽看 Button、Input、Card、Table、Menu、Dashboard 的明暗截图，确认圆角变大、暗色边框半透明、整体为 v4 外观；同时 `-theme antd -only Button` 与 `origin/main` 的截图逐像素相同（Ant 不受影响）。

- [ ] **Step 5: CHANGELOG** — 在「未发布」下：
  - 新增：`theme.ParseOKLCH`/`OKLCH`，`ParseColor` 接受 oklch；`ShadcnOptions.Color` 与 `ShadcnColors`（v4 的 17 个彩色主题）；`ShadcnBases` 增加 mauve、olive、mist、taupe。
  - 变更：shadcn 主题改为 shadcn/ui v4 的变量值（oklch，来源 commit e62c943）；默认圆角 8px → 10px；暗色边框和输入框改为半透明白；暗色侧栏主色为蓝色；slate、gray 为旧名，保留 v3 颜色。

- [ ] **Step 6: 提交并汇报** — `git add -A && git commit -m "chore(theme): shadcn v4 API files, changelog"`；向用户汇报：改了什么、截图位置、对比度和审计结果、未验证项（Windows/Linux 只编译）。之后开始写第 1 批计划。
