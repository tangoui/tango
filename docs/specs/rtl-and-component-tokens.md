# RTL（从右到左布局）与组件级主题令牌 设计方案

> 状态：已批准（开放问题全部按推荐）；全部实现（2026-11-13）：A0=B0、A1–A4（含 mobile/chart）、B1–B3（54 个组件令牌类型全部就位，组件均读令牌）、Q12 第二期视觉光标、a11y 方向标记（Linux AT-SPI 已实现；Windows UIA / macOS NSAccessibility 不适用，见 docs/accessibility.md）、图标未镜像检查钩子。已有令牌对应的 `Theme` 扁平组件字段标注 Deprecated（仍为派生来源，不删除）
> 范围：`ui/`（module `github.com/tangoui/tango`），不含 `ui/mobile`、`ui/chart`、`ui/editor` 的完整改造（只给出边界）
> 关联：AUTHORING.md 第 4/5/8/14/15/17 节；docs/specs/ui-coverage.md 第 6 节（RTL ❌、组件级令牌 ❌）、第 8 节（两项均为 P2 / 大）

## 0. 摘要（核心决策）

| # | 决策 | 一句话理由 |
|---|---|---|
| D0 | 两项共用一个前置基础：**节点环境捕获（node env）**。每个节点在构建时记下当时作用域的 `{主题, 方向, 语言}`；绘制和测量回调执行期间，Runner 把 `c.Theme` 和方向切到该节点的值 | 现在 paint/measure 在 page 函数返回后才运行，作用域早已结束（`ConfigProvider.Theme` 因此只能整窗生效，Locale 也有同类坑）。这一处改动让 158 个组件**不改代码**就能拿到局部主题和方向 |
| A1 | 方向来源：`ConfigProvider(c).Direction(ui.DirAuto/LTR/RTL)`；`Auto` 按 Locale 推断（ar/he/fa/ur/ps/sd/ug/yi/dv/ckb…，再加脚本子标签 Arab/Hebr/Thaa/Syrc/Nkoo/Adlm）；`c.Direction()`、`c.IsRTL()` 读取 | 与 Ant `direction`、Flutter `Directionality` 同构 |
| A2 | **布局在逻辑坐标求解，最后在 RTL 容器内把子节点的 x 水平翻转**（`X' = span − X − W`）。现有 `Padding/Margin/Abs` 的 Left/Right 在 RTL 下自动成为 start/end。提供 `layout.Physical()`（单节点不翻转）和 `ConfigProvider.Direction(LTR)`（子树不翻转）两个逃生口 | 不用迁移 240 处 `geom.Insets{}`、46 处 `layout.Abs`；文字和图片只移动矩形、内容不翻转 |
| A3 | 组件内部不对称的绘制（`r.X+…` 共 329 处 / 107 个文件，`r.Right()` 共 159 处 / 65 个文件）、指针换算（45 个文件）、方向键（约 30 个文件）由组件改造，基础层提供 `c.inline(r)` 辅助、`icon` 方向性元数据、`rovingStep` 自动换键 | 画布级整体镜像会把字形和钟表一并翻转，不可取 |
| B1 | 组件令牌用**类型安全的 struct**：`theme.Components{Button: theme.ButtonToken{…}}`。用户输入时**零值 = 继承**，"显式为零"用哨兵值（`theme.ZeroPx`、`theme.Transparent`、`theme.NoShadow`） | 编译期检查、零分配；不用 `map[string]any` |
| B2 | 覆盖链：全局令牌 → 组件 Alias 覆盖（`Button.Alias.ColorPrimary` 按该设计语言的算法重派生色板）→ 组件专属令牌默认值（由全局派生）→ 用户组件令牌 → **HighContrast 最后执行**（同样作用于组件令牌）。实例链式修饰器优先级最高，在组件内部判定 | 保证高对比度在任何覆盖下仍然达标 |
| B3 | `ConfigProvider(c).Components(...)`、`.Theme(th)` 在嵌套层**局部生效**；根部 provider 仍设置窗口主题（背景、全局层） | 兼容现状，同时补上局部主题能力 |
| B4 | `Defaults` 管**行为与结构属性**（size、variant、shape、allowClear、placement），组件令牌管**视觉数值**。优先级：实例修饰器 > Defaults > 组件令牌 > Alias > 全局令牌 | 两条线不交叉 |

## 1. 现状（代码证据）

- **布局**：`ui/layout/layout.go`。`Style` 只有物理 `Padding/Margin geom.Insets`、`Pos{Left,Top,Right,Bottom}`；`flex()` 提交阶段 `X = padL + mp`（第 514 行）；`placeAbsolute` 先看 Left 再看 Right（第 266–293 行）；`stack()` 用 `Margin.Left`。没有任何方向概念。`measure` 的结果与水平方向无关（便于 A2 只改 arrange 阶段）。
- **构建到绘制**：`runner.go:407 link()` 遍历节点树 → `:424 layout.Solve` → `:440 paintNode`（把子节点相对矩形累加成绝对矩形，调用 `n.paint(l, r)`）。paint/measure 回调里组件读 `t.c.Theme`（114 个文件共 493 处）。
- **ConfigProvider**：`nav_config.go`。`navConfig{locale,size,props,disabled}` 栈；`.Theme()` 的注释写明"Painting reads the theme after the page function returns, so a theme set here applies to the whole window"；`Body` 里 `c.Theme = th` 不还原。
- **文字**：`ui/text/bidi.go`（min-43）有 `Options.Direction`（`DirAuto/DirLTR/DirRTL`），`Line.RTL`。`text/layout.go:81` 注释："Glyphs of every line still start at x=0 … callers right-align RTL lines themselves"。`ui/text.go` 的 `Label.paint` 用 `alignShift(t.align…)`，`AlignLeft = 0` 是物理左对齐，**RTL 行不会自动右对齐**（ui-coverage.md 第 6 节"RTL 段落右对齐"一句需要更正）。
- **浮层**：`overlay.go`，`BottomStart // below anchor, left aligned`，全部是物理语义；`ScreenRight/ScreenLeft/ScreenTopRight` 是物理的。子菜单 `nav_menu.go:842`、`nav_dropdown.go:421` 写死 `RightStart`；`data_popover.go:146 popPlacement` 把 Ant 字符串 `topLeft→TopStart` 等映射为物理位置。
- **滚动**：`scroll.go`。`frame()` 直接返回 `off.X`；竖向滚动条画在 `r.Right()-2-thick`（第 428 行），命中判断用 `p.X >= r.Right()-scrollBarHit`（第 376、454 行）。
- **键盘**：`a11y_keys.go rovingStep` 中 `KeyLeft=prev`、`KeyRight=next`，写死。
- **图标**：`icon.Draw(l, ic, r, col, angle)` 只支持旋转；方向性图标有 `Left/Right/DoubleLeft/DoubleRight/ArrowRight/MenuFold/MenuUnfold/Logout`。
- **主题**：`theme.Theme` 是扁平结构，混有组件字段（`ButtonPadding*`、`Switch*`、`Checkbox*`、`SliderRailHeight`、`Table*`、`Card*`、`Popup/ModalBorder`、`Segmented*`、`Avatar*`、`Skeleton*`、`Sider*/MenuItemHeight`）。`HighContrast` 用反射只处理**顶层** Color 字段（`remapColors`）加固定名单；`contrastPairs` 是固定列表。`CSSVars()` 只输出全局变量（`--tango-*`）。
- **全局层**：Message/Notification 通过 `renderGlobals` 在 page 之后渲染，已不在任何 ConfigProvider 作用域内。

## 2. 共享前置：节点环境捕获（A0 = B0）

```go
// ui 内部
type env struct {
    th  *theme.Theme // 作用域主题（含组件令牌）
    rtl bool         // 已解析的方向
    loc *Locale      // 可选：顺带修复 paint 期 c.Locale() 退回全局的问题
}
// node 增加一个指针字段：env *env
```

1. `Context` 持有 `c.env`（当前作用域）与 `c.rootEnv`。`ConfigProvider.Body` 用该 frame 的 env 入栈，返回时出栈。env 对象缓存在 `State` 里，避免每帧分配；没有覆盖项时复用父 env 指针。
2. `c.add / c.box / Overlay` 创建节点时执行 `n.env = c.env`，并写 `n.ln.RTL = c.env.rtl`（见 3.3）。
3. `paintNode`：`if n.env != c.env { 保存; c.env, c.Theme = n.env, n.env.th }`，`n.paint`、`n.over` 执行完后还原。子节点各自处理。开销只是一次指针比较。
4. measure：`link()` 已在 Solve 前遍历全树。对 `n.env != c.rootEnv && n.ln.Measure != nil` 的节点包一层闭包（切换 env 后调用原 Measure）。只有处在非根作用域的节点付出一个闭包的代价。
5. 全局层：`Message/Notification` 调用时把 `c.env` 存进 note 配置，渲染时恢复。
6. 根部 provider（外层没有 provider，并且 `c.cur == c.root`）设置的主题继续写窗口主题（`r.list.Background`、全局层默认），以兼容现有用法。

验收：现有黄金图逐像素不变；新增测试——嵌套 `ConfigProvider.Dark(true)` 只让内层按钮变暗；内层 `Locale(EnUS)` 在 paint 期取到 EnUS。

工作量：中（2–3 天），需要改 `context.go/runner.go/nav_config.go`，按 AUTHORING 第 13 节须先提出。

## 3. 方案 A：RTL

### 3.1 方向的来源与传递

```go
type Direction uint8 // 包 ui；与 layout.Direction（Row/Column）、text.Direction 不冲突
const ( DirAuto Direction = iota; LTR; RTL )

ui.ConfigProvider(c).Direction(ui.RTL).Body(page)
c.Direction() Direction // 已解析：只返回 LTR / RTL
c.IsRTL() bool
ui.SetDefaultDirection(d) // 窗口级默认（全局层、无 provider 时）；初始 DirAuto
```

- `DirAuto` 解析顺序：最近一层 provider 的显式值 → 最近一层的 Locale（`Locale.Dir` 字段非零时优先，否则由 `Code` 推断）→ `SetDefaultDirection` → 默认 Locale 推断。
- 推断表 `localeRTL(code)`：语言子标签 ∈ {ar, arc, ckb, dv, fa, he, iw, ks, ps, sd, ug, ur, yi, syr, nqo}，或脚本子标签 ∈ {Arab, Hebr, Thaa, Syrc, Nkoo, Adlm, Rohg, Mand}，即为 RTL；显式 `-Latn/-Cyrl` 强制 LTR（例如 `ku-Latn`、`az-Arab` 分别为 LTR、RTL）。表放在 `locale.go`，只有十几行，不引入新依赖。
- 宿主跟随系统布局方向（macOS `userInterfaceLayoutDirection`、Android `layoutDirection`）属于后续工作，经 `SetDefaultDirection` 注入。

### 3.2 与 ui/text 衔接

- **段落基方向**：新增 `c.textOpts(o text.Options) text.Options`。界面文字（标签、按钮、菜单、表头）在 RTL 上下文中设为 `DirRTL`，在 LTR 上下文中保持 `DirAuto`（LTR 行为与现在完全一致）。
- **用户内容**（Input/TextArea/表格单元格的数据文字）默认 `DirAuto`，相当于 HTML `dir="auto"`。组件提供 `.TextDir(text.DirXxx)` 覆盖。
- **对齐**：`TextAlign` 改为 `AlignStart`（零值，沿用原 `AlignLeft` 的数值 0）、`AlignCenter`、`AlignEnd`，另加**物理**的 `AlignLeft`、`AlignRight`（新数值）。`AlignStart` 按**行**解析：`ln.RTL ? 右对齐 : 左对齐`，正好补上 text 层注释里"由调用者右对齐"的空缺。现有 13 个文件 33 处 `AlignLeft/AlignRight` 逐一审核：表示"起点/终点"的改成 Start/End，真正要求物理方向的保留。
- **编辑器**：`paintEditor/processEdit` 对 RTL 行加上 `alignShift`；`Caret/IndexAt/SelectionRects` 的结果按同一偏移平移。空输入框在 RTL 下光标与占位符都靠右。方向键见 3.6。

### 3.3 布局引擎镜像

**做法比较**

| | 做法 1：新增逻辑字段（`PaddingStart/End`、`MarginStart/End`、`Pos.Start/End`，等同 Flutter `EdgeInsetsDirectional`） | 做法 2：镜像规则（在 RTL 容器内，Left 视为 start、Right 视为 end；求解后统一翻转 x） |
|---|---|---|
| 语义清晰度 | 高 | 名字与语义不一致，需要写进文档 |
| 迁移量 | 240 处 Insets、45 处 `Padding.Left` 一类、46 处 Abs 逐个判断；漏改的在 RTL 下错位 | **零迁移**，所有现有布局自动正确 |
| 物理需求 | 天然支持 | 需要逃生口 |
| 实现位置 | flex/stack/placeAbsolute 全部要区分 | `arrange` 末尾一个循环 |

**推荐做法 2**，在 API 层补文档与别名：`geom.Insets` 的 Left/Right 在文档里称为"起始边/结束边（LTR 下即左/右）"；新增 `layout.PadSE(v, start, end)` 一类的可读别名（内部仍写 Left/Right）。逃生口有两个：
- `layout.Physical()` → `Style.NoMirror = true`：本节点在 RTL 父容器中保持物理位置（例如固定在物理右侧的系统按钮）。
- `ConfigProvider(c).Direction(ui.LTR)`：整个子树不镜像（代码编辑器、图表、电话号码输入、媒体控制条）。

**迁移策略**：不做批量迁移，只在 A2/A3 阶段由镜像检查器（3.9）找出"不该翻转"的个案，再加 `Physical()`。

**算法**（只改 arrange 提交阶段，measure 与缓存不受影响）：

```go
// Style 增加：RTL bool（由 ui 按 env 写入）、NoMirror bool
func arrange(n *Node, w, h float32) {
    ... // 现有逻辑：flex/stack 提交与 placeAbsolute 全部按逻辑坐标（LTR）求解
    if n.RTL {
        span := w
        if n.ScrollX { span = max(w, n.Content.W) } // 在滚动内容宽度内翻转
        for _, ch := range n.Children {
            if !ch.NoMirror { ch.Rect.X = span - ch.Rect.X - ch.Rect.W }
        }
    }
}
```

- 子节点在被翻转前已经完成自身的 arrange，它的坐标相对自己，因此翻转不影响其内部；内部由它自己的 `RTL` 标志处理。
- 自动获得的效果：Row 主轴起点在右；`JustifyStart/End` 变为逻辑方向（SpaceBetween/Around/Evenly 本来就对称）；`Gap` 不变；Wrap 时每行从右开始、行仍从上往下；Column 的 `AlignStart` 靠右；Stack 的 `AlignStart` 在右上；`Padding.Left` 与 `Margin.Left` 成为起始侧；`Abs(left=8)` 变成距右 8；同时设置 Left 和 Right 时宽度不变。
- 复杂度 O(子节点数)；LTR 下 `n.RTL == false`，零开销、零差异。

**滚动**（scroll.go）：
- `scrollState` 增加 `rtl`（每帧从节点写入）。偏移保存为**逻辑值**：`off.X = 0` 表示起点（RTL 下即最右端）。`frame()` 中 RTL 时返回 `sx = maxX − off.X`。`ScrollTo/Offset/ScrollX` 初始偏移都按逻辑值，所以 `ScrollH` 一类组件初始自然停在右端。
- 滚轮和触摸拖动的 dx 是物理量，RTL 下换算到逻辑值时取反。`revealX(x,w)` 先把物理内容坐标换成逻辑坐标：`x' = span − x − w`。`c.ScrollShift` 的位移在 RTL 下取反（提供 `c.scrollShiftLogical` 辅助）。
- **竖向滚动条放到左侧**（与浏览器 `dir=rtl` 一致）：`paintScrollbars`、`scrollbarAt`、`dragScrollbars` 改用 `edgeEnd(r)` 辅助，不再写死 `r.Right()`。横向滚动条的拇指位置按 `maxX − off` 计算。

**浮层根**：Overlay 根节点同样带 `RTL`，内部布局正常镜像；放置规则见 3.5。

**被否决的方案**："画布级镜像"，即对节点的 paint 做 `scale(-1,1)` 变换，再对字形和图片做反向变换。这需要 paint 列表区分每种 op，`icon` 里的钟表、加载转圈也会被错误翻转，命中区域还要逆变换，调试成本高，因此不采用。

### 3.4 组件绘制辅助（基础层提供）

```go
type inlineSpan struct{ r geom.Rect; rtl bool }
func (c *Context) inline(r geom.Rect) inlineSpan // paint 期由 env 得到方向
func (s inlineSpan) X(off, w float32) float32    // 距起点 off、宽 w 的元素左边 x
func (s inlineSpan) Rect(off, y, w, h float32) geom.Rect
func (s inlineSpan) Start() / End() float32
func (s inlineSpan) Frac(p geom.Point) float32   // 指针在 r 中的逻辑比例（滑块、进度）
func mirrorRect(r, box geom.Rect) geom.Rect
```

改法示例：Tag 的关闭按钮从 `r.Right()-18` 改为 `s.X(r.W-18, 14)`，命中判断从 `pointer.X > r.Right()-18` 改为 `s.Frac(p)`/`s.Rect`。

### 3.5 需要组件参与的部分（逐类改法与工作量）

工作量：小 ≤ 0.5 天/类，中 1–2 天，大 ≥ 3 天。

| 类别 | 改法 | 工作量 |
|---|---|---|
| **方向性图标** | `icon.Icon` 增加 `Directional bool`（内置白名单在 `def` 处标注；`icon.New(...).Directional()` 供应用使用）。`Draw` 增加 `DrawOpts{Mirror bool}`，或由 `drawIcon` 读 `c.IsRTL()`：**先旋转后镜像**，所以"右箭头旋转 90° 表示展开（向下）"在镜像后仍向下，组件不用改角度。缓存键加 mirror 位（或用 `l.Transform` 实现，GPU 路径缓存照样命中） | 小（基础）+ 涉及的 18 个文件逐个确认 |
| 图标白名单（翻转） | Left、Right、DoubleLeft、DoubleRight、ArrowRight、MenuFold、MenuUnfold、Logout、Message（气泡尾巴）、Profile/Form/FileText（左对齐的文本行）、Rise/Fall（可选，见开放问题） | — |
| 不翻转的例外 | Up/Down/Caret*、Check/Close/Plus/Minus、Search、Clock、Loading/Spin（顺时针）、RotateLeft/RotateRight（旋转方向是物理语义）、Reload、播放/媒体控制（应用自定义图标默认不翻转）、Link、Paperclip、Like、Star、Question/Info/Warning、Upload/Download、Swap/SwapVertical（本身对称）、Eye、Fullscreen | — |
| **浮层 Placement** | `BottomStart/BottomEnd/TopStart/TopEnd` **改为逻辑语义**（RTL 下对齐锚点右边缘）。逐个核查现有用法（下拉对齐触发器起点）都符合预期。新增 `InlineEnd/InlineEndTop/InlineEndBottom/InlineStart/InlineStartTop/InlineStartBottom` 与 `ScreenEnd/ScreenStart/ScreenTopEnd/ScreenTopStart`，`placeOverlay` 开头按 `o.rtl` 解析成物理位置；`Left/Right/RightStart…/ScreenRight/ScreenTopRight` 保持物理。`popPlacement` 把 Ant 字符串映射成逻辑位置（见开放问题 Q2） | 中 |
| Menu / Dropdown / Cascader 子菜单 | `RightStart → InlineEndTop`（nav_menu.go:842、nav_dropdown.go:421）；子菜单箭头用 Right（属于白名单）；Cascader 多列由 Row 镜像自动向左展开 | 小 |
| Drawer | 新增 `DrawerStart/DrawerEnd`；**未显式设置时默认 End**（RTL 下从左侧滑出），显式 `DrawerRight/Left` 保持物理；进出场 `ShiftX` 符号跟随实际边 | 小 |
| Notification | 新增 `NotifyTopEnd/TopStart/BottomEnd/BottomStart`，默认 TopEnd；Sonner 堆叠的横向滑出方向跟随实际边；全局层方向取自 note 捕获的 env | 小 |
| Message / Modal / Tooltip / Popover | 居中类不变；Popover 箭头位置随解析后的 placement 走 | 小 |
| **输入框前后缀** | Input/InputNumber/Select/DatePicker 的 prefix、suffix、清除、密码眼睛、步进箭头、下拉箭头：用 `inline(r)` 放到逻辑起点/终点；`paintFrame` 与 `selector.go`（共享外壳）各改一次，多数选择类组件一并受益；文字与占位符按 3.2 对齐 | 中 |
| Tag / Badge / Avatar 组 | 关闭按钮及其命中；Badge 计数放在 top-end 角；AvatarGroup 的叠放顺序由 Row 镜像自动得到，阴影边按 inline 处理 | 小 |
| **Slider / SliderRange** | 横向时最小值在右（Ant RTL 行为）：值与指针换算改用 `Frac`；刻度标签、轨道填充、Tooltip 跟随；方向键见 3.6；竖向不变 | 中 |
| Progress / Steps / Timeline | 进度条从起点填充；Steps 横向由 Row 镜像得到，连接线（paint 里的 `r.X+`）改用 inline；Timeline 的 `left/right/alternate` 改为 start/end 语义 | 中 |
| Switch / Segmented / Rate | Switch 打开时拇指在 end（RTL 下在左）；Segmented 滑块动画 x 用逻辑值；Rate 星星从右填充，半星裁剪反向 | 小 |
| Tabs / Breadcrumb / Pagination | 标签顺序由布局镜像；墨水条 x、溢出滚动起点（逻辑偏移）、"更多"按钮位置；Breadcrumb 的 chevron 分隔符翻转（文字 "/" 不翻转）；Pagination 箭头翻转、顺序镜像 | 中 |
| **表格** | 列顺序：table_body/header 若自行计算列 x，就在行内按 `span − x − w` 镜像；`FixedLeft/FixedRight` **按逻辑 start/end 解释**（与 Ant RTL 一致），粘性偏移、固定列阴影的方向、表头排序与筛选图标位置、树形缩进从右开始、展开图标、列宽拖拽的增量符号、横向滚动起点都要处理；`table_keys` 的左右键语义 | 大 |
| Tree / VirtualTree / TreeSelect | 缩进从右开始；切换箭头（白名单）；Left/Right 展开折叠语义互换（WAI-ARIA APG） | 中 |
| Layout / ProLayout / Sider / Menu inline | Sider 由 Row 镜像自动到右侧；折叠触发图标；Menu inline 缩进；`CollapsibleSider` 拖拽方向 | 中 |
| 日期组件（DatePicker/RangePicker/DateTime/Calendar/DatePickerPanel） | 星期网格由 Row 镜像（与阿拉伯语日历习惯一致）；前后月、年箭头翻转；RangePicker 的起始输入在右；网格方向键按视觉方向（3.6）。时间列（时:分:秒）**保持 LTR**（见 Q5） | 大 |
| Carousel / Image 预览 | 幻灯片顺序、滑动手势方向、"下一张"的箭头与自动播放方向；预览工具栏镜像，旋转图标不翻转 | 中 |
| Transfer / Splitter / Affix / FloatButton / BackTop | Transfer 源列表在右、穿梭箭头翻转；Splitter 拖拽增量与方向键；FloatButton/BackTop 的 `Abs(right=24)` 自动变为距左 24 | 小–中 |
| 图表 / 代码编辑器 / QRCode / Watermark / 图片内容 | 默认整块 `Direction(LTR)`（图表可选择镜像，见 Q5） | 小 |

### 3.6 键盘方向键语义

规则：**左右键是视觉方向**。凡是表达"前一个/后一个"或"增/减"的地方，RTL 下左右互换；Home/End 表示逻辑起点和终点；上下键不变。

- 基础层：`c.inlineKey(k) (prev, next bool)`；`rovingStep` 内部改用它，RadioGroup、Tabs、ToggleGroup、Segmented、Toolbar 等一次全部生效。
- 滑块类（Slider、Rate、Splitter、ColorPicker 色相条）：朝最大值所在一侧的方向键增加数值。RTL 下 Left 增加、Right 减少；Up/PageUp 始终增加（与 Chrome `input[type=range]` 在 `dir=rtl` 下的行为一致）。a11y `Increment/Decrement` 不受影响。
- 树形和级联：RTL 下 Left 展开或进入子级、Right 折叠或回到父级（APG）。
- 网格（日期、表格单元格）：按视觉方向移动。
- 文本光标：第一期在 RTL 段落中把 Left/Right 映射为逻辑的下一个/上一个字符；混排文本的"视觉光标移动"留作后续（见 Q12）。

需要修改的文件（grep `KeyLeft|KeyRight`）：nav_menu、entry_color、entry_cascader、data_calendar、p2a_time_range、entry_treeselect、table_keys、table_header、p2b_slider、nav_splitter、nav_dropdown、fill_virtual_tree、fill_date_panel、entry_time、entry_segmented、entry_rate、entry_otp、date_util、data_tree、data_image、data_carousel、entry_datepanel、editor.go。约 25 个文件，每个文件工作量都小。

### 3.7 无障碍

- 语义树坐标来自 `c.rects`，即镜像后的物理矩形，**自动正确**。
- 读屏顺序等于构建顺序（逻辑顺序），RTL 下正好是正确的阅读顺序，**不要按 x 排序**。
- `a11y.Tree` 增加 `RTL bool`（或节点级 `TextDirection`），由平台桥接映射：UIA `FlowDirection`、AT-SPI 文本属性 `direction`、NSAccessibility 布局方向。只影响平台的空间导航，内容不变。
- 表格的 `ColIndex` 保持逻辑序号；Slider 仍标 `Horizontal`，值文本不变。
- `Audit` 增加一项检查：RTL 下 `Directional` 图标未镜像时报警告（Driver 记录图标绘制）。

### 3.8 测试策略

1. **单元**：layout 的 RTL 表驱动测试（Row/Column/Stack × Justify × Align × Wrap × Padding/Margin × Abs × ScrollX × NoMirror），断言 `X_rtl = span − X_ltr − W`。scroll 的逻辑偏移、滚动条命中在左侧。`localeRTL` 表测试。`rovingStep` 在 RTL 下的换键测试。
2. **镜像检查器**（`Driver.MirrorCheck(page)`）：
   - **几何启发式**（主判据，结果确定）：同尺寸各渲染一帧 LTR 和 RTL，对两帧都有的节点 ID 断言 `|X_rtl − (W − X_ltr − W_n)| ≤ 1`，并且 Y/W/H 相同；节点或其祖先带 `NoMirror` 或 LTR 作用域时跳过；输出违例列表。
   - **像素启发式**（发现组件内部绘制的不对称）：把 RTL 截图水平翻转后与 LTR 截图比较，先用 paint 列表里的 Glyphs/Image op 包围盒和非方向性图标框做掩膜，再按节点区域统计差异，超过阈值就报出该节点。
   - **图标检查**：Driver 记录 `(图标名, mirrored)`，Directional 图标在 RTL 下未镜像即报错。
3. **黄金图**：`golden_test.go` 增加 RTL 用例（Button、Input 带前后缀、Select 打开、Tabs、Pagination、Steps、Slider、Table 固定列、Menu 子菜单、DatePicker 面板、Drawer、Notification）。需要在 `testdata/golden_fonts.txt` 加入阿拉伯文/希伯来文字体（例如 Noto Sans Arabic/Hebrew），否则黄金图会被跳过。
4. **Gallery**：增加 `-dir rtl` 维度（输出 `<Name>-rtl.png`，`-theme all -dir rtl` 可组合），页面使用 `LocaleCode("ar")`（需要先有阿拉伯语包，或用 EnUS + `Direction(RTL)` 先行）；`-mirror-check` 对全部页面运行检查器并汇总；`-audit` 增加 RTL 轮次。
5. **i18n**：`i18n_wiring_test` 增加 RTL 下的渲染，断言文案仍从 Locale 取得。

### 3.9 分阶段计划（每阶段可独立合入；LTR 下黄金图逐像素不变是每阶段的合入门槛）

| 阶段 | 内容 | 工作量 |
|---|---|---|
| A0 | 节点环境捕获（第 2 节，与 B0 共用） | 2–3 天 |
| A1 基础层 | `Direction` API、Locale 推断、`layout.Style.RTL/NoMirror` + arrange 镜像、scroll 逻辑偏移与左侧滚动条、`c.textOpts` 与 `TextAlign` Start/End、编辑器 RTL 行对齐、`icon.Directional` 与镜像绘制、Placement 逻辑化、`inlineKey/rovingStep`、`inline(r)` 辅助、Driver 镜像检查器、gallery `-dir rtl` | 约 1.5 周 |
| A2 通用组件 | Button/IconButton、Text 系列、Divider、Tag/Badge/Avatar、Input 系列（paintFrame）、选择器外壳（selector.go）与 Select 家族、Checkbox/Radio/Switch、Segmented、Rate、Breadcrumb、Pagination、Tabs、Steps、Progress、Alert、Message/Notification、Drawer/Modal、Tooltip/Popover、Card/List/Descriptions、Empty/Result、FloatButton/BackTop，共约 45 个入口 | 1.5–2 周（两人并行约 1 周） |
| A3 复杂组件 | Table/ProTable/EditableProTable、日期全家、Tree/VirtualTree/TreeSelect/Cascader、Menu/Dropdown/ProLayout/Sider、Slider/SliderRange、Carousel/Image、Transfer、Splitter、Timeline、Tour、Upload、Form 标签对齐（label 在 start 侧）、Pro 系列 | 2–3 周 |
| A4 子包 | mobile（NavBar 返回箭头、SwipeCell 方向、Tabs）、chart 可选镜像 | 另行评估 |

## 4. 方案 B：组件级主题令牌

### 4.1 三层覆盖与优先级

```
全局令牌（theme.AntDesign/ElementPlus/Shadcn + 选项）
  └─ 组件 Alias 覆盖（Components.Button.Alias.ColorPrimary/BorderRadius/ControlHeight/FontSize…）
       └─ 组件专属令牌：默认值由（Alias 后的）全局令牌派生，再合并用户值
            └─ Defaults（行为属性，ConfigProvider.Defaults）
                 └─ 实例链式修饰器（.Primary()、.Round()、.Color(...)）
最后：HighContrast（如开启）作用于以上全部令牌
```

### 4.2 API

```go
// 构造主题时
th := theme.AntDesign(theme.WithComponents(theme.Components{
    Button: theme.ButtonToken{
        Alias:         theme.Alias{ColorPrimary: theme.Hex("#00b96b"), BorderRadius: theme.ZeroPx},
        PaddingInline: 20,
        PrimaryShadow: theme.NoShadow,
    },
    Table: theme.TableToken{HeaderBg: theme.Hex("#f0f5ff"), CellPaddingBlock: 12},
}))

// 局部（B0 之后生效）
ui.ConfigProvider(c).Components(theme.Components{Card: theme.CardToken{HeaderBg: ...}}).Body(fn)
ui.ConfigProvider(c).Theme(th).Body(fn)   // 嵌套层：局部；根部：同时设窗口主题

// 组件读取的统一入口
func (c *Context) ThemeFor(k theme.Comp) *theme.Theme // 应用了该组件 Alias 的主题（无 Alias 时返回 c.Theme 本身）
func (c *Context) Token() *theme.Components           // = &c.Theme.Components（paint 期为节点作用域）
```

组件内的写法（构造时捕获，paint 和 measure 直接使用，不再读 `b.c.Theme`）：

```go
b.th = c.ThemeFor(theme.CompButton)
b.tk = &b.th.Components.Button
// paint: l.Fill(r, geom.Rad(b.tk.BorderRadius()), b.tk.DefaultBg) ...
```

### 4.3 令牌结构：类型安全 + 零值继承

```go
package theme
type Alias struct { // 每个组件令牌都可嵌入的全局令牌子集（与 Ant 的 component token 可覆盖 alias token 对应）
    ColorPrimary, ColorError, ColorText, ColorBorder, ColorBgContainer Color
    BorderRadius, BorderRadiusSM, BorderRadiusLG               float32
    ControlHeight, ControlHeightSM, ControlHeightLG            float32
    FontSize, FontSizeSM, FontSizeLG, LineWidth                float32
}
type ButtonToken struct {
    Alias
    FontWeight                                     int
    PaddingInline, PaddingInlineSM, PaddingInlineLG float32
    DefaultBg, DefaultColor, DefaultBorderColor     Color `contrast:"text:DefaultBg"`
    DefaultHoverBg, DefaultHoverColor, DefaultHoverBorderColor Color
    SecondaryBg, SecondaryColor, SecondaryHoverBg  Color
    PrimaryColor                                   Color `contrast:"text:@Primary"`
    DangerColor                                    Color
    TextHoverBg, LinkHoverBg                       Color
    DefaultShadow, PrimaryShadow, DangerShadow     []paint.Shadow
}
type Components struct {
    Button ButtonToken; Input InputToken; Select SelectToken; Checkbox CheckboxToken
    Switch SwitchToken; Slider SliderToken; Table TableToken; Card CardToken; Menu MenuToken
    Tabs TabsToken; Tag TagToken; Modal ModalToken; Drawer DrawerToken; Tooltip TooltipToken
    Popover PopoverToken; Notification NotificationToken; Message MessageToken
    Pagination PaginationToken; Segmented SegmentedToken; Avatar AvatarToken; Skeleton SkeletonToken
    Layout LayoutToken; DatePicker DatePickerToken; Tree TreeToken; Steps StepsToken; /* … */
}
```

- 字段名沿用 Ant 的 component token 名（camelCase 转成 PascalCase），方便对照文档；Element/shadcn 的差异由各自的派生函数给出默认值。
- 用户输入**零值 = 继承**。合并只在主题构建时用反射执行一次（不是每帧），之后组件读到的是已解析的完整值。
- "显式为零"的哨兵：
  - 长度：`theme.ZeroPx`（`float32` 负零，合并时用 `math.Signbit` 识别，绘制效果与 0 相同）
  - 颜色：`theme.Transparent = Color{R:255,G:255,B:255,A:0}`（与零值不同，但同样不可见）
  - 阴影：`theme.NoShadow = []paint.Shadow{}`（非 nil 的空切片）
  - 时长：`theme.Instant = time.Nanosecond`
  - 令牌里不用 bool，需要开关时用"0 = 继承"的枚举
- 已解析的值沿用扁平 Theme 的约定（`zero: none`），用 struct tag `zero:"none"` 标注。

### 4.4 派生规则（每个组件一个文件 `theme/comp_<name>.go`）

```go
func (t *Theme) deriveComponents(user Components) // 在 AntDesign/ElementPlus/Shadcn 构造末尾调用
// 对每个组件 X：
//  1. tX := t；若 user.X.Alias 非空 → tX = t.withAlias(user.X.Alias)
//     （复制 Theme，按 t.Flavor 的算法重派生色板：AntPalette/ElementPalette/shadcnPalette；几何直接替换）
//  2. def := deriveX(tX)        // 默认值来自全局令牌，现有扁平字段（ButtonPaddingH 等）作为来源
//  3. tX.Components.X = merge(def, user.X)
//  4. 有 Alias 时 t.comp[X] = tX（供 ThemeFor），否则不存
```

- Flavor 差异只在 `deriveX` 内按 `t.Flavor` 分支（theme 包内部允许），组件代码不再分支。
- 扁平组件字段（Button*、Switch* …）保留为派生来源，B3 完成后再标 Deprecated（见 Q10）。

**与 HighContrast 的关系（明确规则）**：组件令牌**在 HighContrast 之前**合并；`HighContrast(th)` 是最后一步：
1. 先按现有逻辑处理全局字段。
2. 遍历 `Components` 的 Color 字段：带 `contrast:"text:<背景字段|@Primary|@Page>"` 的按 4.5:1 调用 `reach`，带 `contrast:"nontext:…"` 的按 3:1；色板搬移（`paletteMoves`）同样应用到组件字段（推广 `remapColors`，递归处理嵌套 struct）。
3. 对每个 `t.comp[X]` 的 Alias 主题递归执行 HighContrast。

`contrastPairs` 由 tag 自动生成组件条目，`ContrastIssues` 在普通模式下也能发现用户覆盖造成的低对比度（报警告），HC 模式要求 0 失败。实例级自定义颜色（如 `Tag.Color("#…")`）不在 HC 保证范围内，由 `Audit` 报告。

### 4.5 CSSVars（WebView 共享）

- `CSSVars()` 保持不变（全局变量，兼容现有调用）。
- 新增 `t.CSSVarsWith(theme.CSSOptions{Components: true, OnlyChanged: true})`：变量名为 `--tango-<comp>-<token>`，由字段名转 kebab-case（可用 tag `css:"…"` 覆盖），例如 `--tango-button-padding-inline`、`--tango-button-color-primary`（Alias）、`--tango-table-header-bg`。`OnlyChanged` 只输出与"无覆盖派生值"不同的变量，避免一次输出近千个变量。
- 可选（P3）：`Ref: true` 时，对恒等派生的令牌输出 `var(--tango-color-primary)` 引用，形成 Element Plus 那样的变量层级，页面改一个全局变量即可级联生效。
- 测试：变量名唯一、全部字段都有对应变量、kebab 规则、`OnlyChanged` 在空覆盖时为空。

### 4.6 局部主题

依赖 B0：`ConfigProvider.Body` 计算作用域主题（缓存键为外层主题指针加覆盖项哈希，存于 `State`），设为 `c.env.th`，退出时还原。`Theme/Flavor/Dark/PrimaryColor/Compact/Components` 全部局部生效；根部 provider 额外写窗口主题（第 2 节第 6 条）。

### 4.7 现有组件改造

- 改造模式：构造时 `th := c.ThemeFor(k)`，把 paint/measure 中的 `b.c.Theme.X` 换成 `b.th`/`b.tk`；有对应组件令牌的全局读取改读令牌。没有 Alias 也没有专属令牌的组件不必改，借 B0 已能局部换主题。
- 共享外壳优先：`paintFrame`（输入框类）、`selector.go`（选择器类）、`drawCheckbox`、`drawFocusRing`、`controlHeight/controlRadius/controlFont`（改为接受 `*Theme`）。改这几处就覆盖了约 30 个入口。
- 分批：
  - **B2**（约 25 个入口，1.5–2 周）：Button、Input 家族、Select 家族、Checkbox/Radio/Switch、Slider、Tag、Tabs、Menu、Table、Card、Modal/Drawer、Message/Notification、Tooltip/Popover、Pagination、Segmented、Avatar、Skeleton、Layout/Sider。
  - **B3**（其余约 130 个入口，按分组 4 路并行约 1–1.5 周，串行约 3–4 周）：日期家族、Tree 系列、Steps/Timeline、Descriptions/List、Badge、Progress、Upload、Transfer、Pro 系列……其中 Pro 组件多数复用基础组件令牌，只需少量专属令牌。
- 守护测试：仿照 `TestNoHardcodedCJK`，对已迁移的组件文件 grep，禁止直接读取已有组件令牌对应的全局字段。

### 4.8 与 ConfigProvider.Defaults 的边界

- Defaults（`map[string]Props`，键名与 Ant prop 名一致）只放**能用实例修饰器设置**的行为或结构属性：size、variant、shape/round、allowClear、showSearch、placement、duration、maxCount。
- 组件令牌只放**不暴露为修饰器**的视觉数值：颜色、内边距、圆角、阴影、字重、高度像素值。
- 交叉情形：`Defaults("Button", {"shape":"round"})` 胜过 `BorderRadius` 令牌；`size` 选档位，令牌给出该档位的像素值（ControlHeightSM）。
- 约定：**不得**往 Defaults 加颜色或尺寸数值键；也不得在令牌里加行为开关。
- 协作：另一个会话正在给全部组件接 Defaults，改的是构造函数；令牌迁移改的是 paint/measure，同一文件会冲突。某个组件的令牌迁移应排在它的 Defaults 接线合入**之后**。

### 4.9 测试

1. **零值继承**：`deriveComponents(Components{})` 后的主题与改造前 `reflect.DeepEqual`（扁平字段一致）；全部现有黄金图逐像素不变。
2. **哨兵**：`ZeroPx/Transparent/NoShadow` 合并后生效，并与零值区分开。
3. **覆盖黄金图**：每个迁移的组件一张"令牌覆盖"图（目标像素改变、相邻其他组件不变）；作用域测试（内层 `Components` 只影响内层）。
4. **Alias 重派生**：`Button.Alias.ColorPrimary` 改变后 hover/active 按设计语言算法变化，三套主题各测一次。
5. **高对比度回归**：用一组刻意低对比的组件覆盖，`ContrastFailures(HighContrast(th)) == 0`；gallery `-theme hc` 增加"带组件覆盖"轮次。
6. CSSVars 测试（4.5）；性能：`ThemeFor` 在无 Alias 时零分配（基准测试）。

### 4.10 分阶段计划

| 阶段 | 内容 | 工作量 |
|---|---|---|
| B0 | 节点环境捕获与局部主题（与 A0 共用） | 2–3 天 |
| B1 | theme 包：`Alias/Components/各 Token`（先建 B2 的约 20 个类型）、merge 与哨兵、`deriveComponents`、`withAlias`、`ThemeFor`、HighContrast 与 contrastPairs 扩展、`CSSVarsWith`、`ConfigProvider.Components`、测试 | 约 1 周 |
| B2 | 高频组件与共享外壳 | 1.5–2 周 |
| B3 | 其余组件；扁平组件字段标 Deprecated；AUTHORING 第 5 节改写为"组件令牌优先" | 3–4 周（可并行） |

## 5. 依赖关系与并行

```
A0=B0（节点环境捕获）
 ├─ A1（RTL 基础层）──┬─ A2 ── A3
 └─ B1（theme 包）────┴─ B2 ── B3
```

- **硬依赖**：A1、B1 都依赖 A0=B0（paint 期需要作用域方向和作用域主题）。
- **可并行**：A1 与 B1 改的文件基本不交叉（layout/scroll/text/icon/overlay 与 theme）。
- **组件阶段**：A2/A3 与 B2/B3 改的是**同一批组件文件的 paint 代码**（前者改 x 坐标，后者改令牌读取）。建议按组件批次合并成一个 PR 一起改，或者固定顺序为"先 B 后 A"：令牌迁移后 paint 代码收敛到 `b.th/b.tk`，再做 inline 改造时冲突更少。
- **与 shadcn 补全的衔接**：shadcn 会话在 `theme/theme.go`、`theme/shadcn.go` 追加扁平令牌。B1 只新增 `theme/components*.go`、`theme/comp_*.go`，在 `Theme` 里加一个 `Components` 字段和 `comp` 缓存，在三个构造函数末尾各加一行 `t.deriveComponents(o.components)`，冲突面很小。约定：**B1 合入后，新增的组件专属令牌一律进 `Components`**；shadcn 已加的扁平字段作为派生来源，B3 再收拢。建议 B1 在 shadcn 这一轮合入后再开始，或先对齐字段清单。
- **与 Defaults 接线**：见 4.8，按组件串行。

## 6. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| 组件内部不对称绘制遗漏（329 + 159 处 x 坐标、45 个指针换算文件） | RTL 下局部错位 | 像素与几何镜像检查器、gallery `-dir rtl` 全量截图、按组件逐个验收 |
| `TextAlign` 与 `BottomStart/End` 改变语义 | API 行为变化（LTR 下不变，RTL 下才有差异） | LTR 黄金图不变作为门槛；CHANGELOG 说明；逐处审核 33 处 Align 用法 |
| Drawer/Notification 默认值改为逻辑 End | RTL 应用的观感变化 | 显式物理常量保留；列入开放问题 |
| measure 包装闭包、作用域主题复制（Theme 是大结构体） | 帧时间与内存 | 只对非根作用域节点包装；主题与 Alias 主题按键缓存；加基准测试 |
| HighContrast 反射扩展到嵌套 struct | 漏改或误改颜色 | tag 驱动 + 测试：所有带 contrast tag 的字段在 HC 下都达标 |
| 双向混排文本的光标移动 | 编辑体验不理想 | 第一期逻辑换键，视觉移动列入后续 |
| 黄金图字体缺少阿拉伯文/希伯来文 | RTL 黄金图被跳过 | 在 testdata 字体列表加入 Noto 系列，并在 CI 镜像中安装 |
| 滚动偏移改为逻辑值后 `ScrollShift`/Damage 符号出错 | 残留像素 | RTL 下默认整帧重画，直到专项测试通过 |
| 与并行会话（Defaults、shadcn、组件改造）冲突 | 合并成本 | 按第 5 节排序；B1 只新增文件 |

## 7. 需要人决策的开放问题（附推荐）

| # | 问题 | 选项 | 推荐 |
|---|---|---|---|
| Q1 | 逻辑边距的实现方式 | a) 新增 Start/End 字段并迁移；b) 镜像规则（Left 即 start）+ `Physical()` 逃生口 | **b**，零迁移，与已有代码量匹配 |
| Q2 | Ant 风格 placement 字符串（topLeft/left…）在 RTL 下的含义 | a) 物理；b) 逻辑 | **b**（与 Ant Dropdown 在 RTL 下翻转 bottomLeft 的行为一致，实施前用 Ant 官网 RTL 示例核实）；Go 常量 `Left/Right` 保持物理 |
| Q3 | Drawer/Notification 未设置时的默认边 | a) 保持物理右；b) 逻辑 End | **b** |
| Q4 | 用户输入内容的段落方向 | a) 跟随上下文；b) DirAuto | **界面文字跟随上下文，可编辑和数据文字用 DirAuto**，并提供 `.TextDir()` |
| Q5 | 图表、时间列、代码编辑器、数字类输入是否镜像 | — | 编辑器、时间列、数字输入**固定 LTR**；图表默认 LTR，提供 `chart.Mirror()` 选项 |
| Q6 | 零值继承的"显式为零"表达 | a) 哨兵值；b) `Opt[T]` 泛型字段；c) 指针字段 | **a**，字面量最简洁；`Opt[T]` 作为备选 |
| Q7 | `ConfigProvider.Theme` 局部还是整窗 | a) 一律局部；b) 根部整窗、嵌套局部 | **b**，兼容现有用法 |
| Q8 | HC 是否覆盖用户的组件颜色 | a) 是；b) 提供豁免 | **a**，无障碍优先，不提供豁免 |
| Q9 | 组件 CSS 变量默认是否输出 | a) 默认输出；b) 显式开启 | **b**，并默认 `OnlyChanged` |
| Q10 | 扁平组件字段（ButtonPaddingH…）的去留 | — | B3 前保留为派生来源，B3 后标 Deprecated，下一个大版本删除 |
| Q11 | `DirAuto` 的依据 | a) Locale；b) 系统布局方向 | **a**；系统方向由宿主经 `SetDefaultDirection` 注入 |
| Q12 | 文本光标左右键 | a) 逻辑换键；b) 视觉移动 | 第一期 **a**，后续再做 b |
| Q13 | `Locale` 是否新增 `Dir` 字段 | — | 新增可选字段，零值时由 Code 推断；同时内置一个阿拉伯语包用于验证（翻译需要人工校对） |
| Q14 | 实施顺序 | — | A0 → (A1 ∥ B1) → 按组件批次合做 B 与 A；整体排在 ui-coverage 的 P0/P1 之后 |

## 8. 关键文件

- `ui/layout/layout.go`、`ui/layout/options.go`：镜像算法、`RTL/NoMirror`
- `ui/runner.go`（`link`、`paintNode`）、`ui/context.go`、`ui/nav_config.go`：节点环境捕获、Direction、局部主题与 Components
- `ui/scroll.go`、`ui/overlay.go`、`ui/a11y_keys.go`、`ui/text.go`、`ui/icon/icon.go`：滚动、浮层放置、方向键、对齐、图标镜像
- `ui/theme/theme.go`、`ui/theme/highcontrast.go`、`ui/theme/contrast.go`、`ui/theme/cssvars.go`，新增 `ui/theme/components.go`、`ui/theme/comp_*.go`
- `ui/text/bidi.go`、`ui/text/layout.go`：只读衔接（`Options.Direction`、`Line.RTL`）
