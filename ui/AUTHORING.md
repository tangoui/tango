# Tango UI 组件编写指南

> **设计基线**：Ant Design 6.7（`antd@6.7.0`）+ Ant Design Pro v6（Pro Components v3）。组件语义、默认值、令牌和交互状态都以它为准，详见 [docs/design-baseline.md](../docs/design-baseline.md)。

本文面向给 `ui/` 补组件的开发者。读完即可按统一风格新增组件，**不需要修改基础层**（theme / paint / text / layout / input / Context / Runner / Driver）。如果确实缺基础能力，先在 PR 描述里说明，不要在组件里绕开基础层私自实现。

## 1. 文件与命名

- 组件写在 `ui` 包里，一个组件（或一组强相关组件）一个文件：`ui/rate.go`、`ui/transfer.go`。测试写在同名 `_test.go`。
- 入口是包级函数，第一个参数总是 `c *ui.Context`，名字与 Ant Design 组件名一致：`ui.Rate(c, &v)`、`ui.Transfer(c, ...)`。
- 返回一个 `*XxxW` 构建器（`ButtonW`、`TagW`……），修饰方法返回自身以便链式调用；查询方法（`Clicked()`、`Changed()`、`IsOpen()`）返回结果，放在链尾：

  ```go
  if ui.Button(c, "保存").Primary().Small().Clicked() { ... }
  ```
- 修饰方法用动词或形容词，不带 `Set`：`Disabled(true)`、`Placeholder("…")`、`Size(ui.Small)`、`Status("error")`。布尔开关型修饰器若常与变量搭配，就接收 `bool`（`Disabled(on bool)`、`Loading(on bool)`）。
- 值由应用持有，组件接收指针：`ui.Input(c, &name)`、`ui.Switch(c, &on)`；选项统一用 `[]ui.Option`（`ui.Opts("a","b")` 快速构造）。
- 带子内容的组件有两种形态：容器型 `ui.Card(c, "标题").Extra(...).Body(func(){...})`（修饰器在 `Body` 之前）；包裹型 `ui.Tooltip(c, "提示", func(){ trigger })`。
- 尺寸用 `ui.ControlSize`（`ui.Small / ui.Middle / ui.Large`）；状态色用字符串 `"success" | "warning" | "error" | "info" | "processing"`，经 `th.ColorOf(status)` 取色板。

## 2. 每帧发生了什么

```
事件入队 → Runner.Frame:
  processEvents（按上一帧的命中区域算 hover/press/click/focus）
  build：调用页面函数，组件创建节点、读取交互、改写应用状态
         → c.deferred 里的延迟处理（链式修饰器都已生效）
  若本帧处理过事件或有人调用 c.Invalidate()，再 build 一次（最多 3 次）
  flexbox.Solve → paint（显示列表）→ Renderer → Window.Present
```

要点：
- **build 阶段只创建节点、读交互、改状态；绘制推迟到 paint 回调**。所以 paint 回调读取的是链式修饰之后的最终字段。
- 交互判断基于**上一帧**的几何位置，这是即时模式的标准做法。
- 改了会影响本帧前面已构建部分的状态时，调用 `c.Invalidate()`（或在交互处理里设置 `c.invalid = true`），Runner 会立刻再 build 一次。

## 3. 节点、ID 与保留状态

- 新建叶子节点：`n := c.add("kind", label)`。`kind` 是组件种类（参与 ID 计算），`label` 是可见文字（参与 ID 计算，并供 `Driver.Find/ClickLabel` 测试定位）。
- 容器：`b := c.box("kind", layout.Row, opts)` 得到 `*Box`，`b.Body(fn)` 在其中构建子节点；或直接用公开的 `ui.Row/Column/StackBox/Div/Scroll`。
- ID = 作用域 + kind + label + 出现次序。列表里的动态项必须用 `ui.Key(c, key, func(){...})` 包起来，插入/删除兄弟项时状态才不会串。
- 组件私有的保留状态（展开、光标、视图月份……）用 `st := ui.State[myState](c, n.id, initFunc)`。超过约 300 帧未访问会被回收。
- 动画值用 `c.Anim(id, "slot", target, th.DurationMid)`（首次直接返回 target）；进场动画用 `c.AnimFrom(id, "open", 0, 1, dur)`。动画进行中 Runner 会自动请求下一帧，静止时不出帧。计时器用 `c.WakeAt(t)`，时间一律用 `c.Now()`。`c.WakeAtDamaged(t)` 也是在 t 唤醒一帧的定时器（宿主看到的 NextWake 与 WakeAt 合并），区别在重绘：到达 `WakeAt` 定时的那一帧整帧重画，到达 `WakeAtDamaged` 定时的那一帧仍可局部重画——调用者承诺在醒来的那一帧用 `c.Damage` 声明时钟改变的全部绘制（如代码编辑器的光标闪烁）；见第 6 节“重绘提示”。
- 需要等全部链式修饰生效后再处理的逻辑（例如依赖 `Disabled` 的点击切换、要在节点里追加子节点的弹层），用 `z := c.lazy(n, fn)`：`fn` 会在查询方法里或本次 build 结束前执行一次，执行时当前父节点是 `n`。

## 4. 布局

`n.ln` 是 `flexbox.Node`（内部求解器 `ui/internal/flexbox`；公开的只有不透明的 `layout.Option` 及其构造函数），常用字段/选项：

| 需求 | 写法 |
|---|---|
| 叶子尺寸随内容 | `n.ln.Measure = func(maxW, maxH float32) geom.Size {...}`（`maxW` 可能是 +Inf；文字按 maxW 折行） |
| 固定尺寸 | `n.ln.Width/Height`，或 `layout.W/H/Size` |
| 主轴伸展 | `layout.Grow(1)`（默认 Basis 0，等分） |
| 列中不被拉宽（按钮、标签） | `n.ln.NoStretch = true` |
| 交叉轴居中 | `layout.Middle()`；两轴居中 `layout.Center()` |
| 间距 / 内边距 / 外边距 | `layout.Gap(8)`、`layout.PadVH(4, 11)`、`layout.Margin(geom.Insets{...})` |
| 换行 | `layout.WrapItems()` |
| 绝对定位 | `layout.Abs(left, top, right, bottom)`（未用的边传 `layout.Unset`） |
| 滚动 | `ui.Scroll(c, fn, layout.MaxH(300))`（偏移 float64：`Offset()/ScrollTo(x,y)`；长列表键盘移动用 `followRow`/`c.ScrollIntoView(scrollID, id)`/`sb.Reveal(y,h)` 滚到可见；`Always()` 常显滚动条，滚动条可拖动） |
| 非滚动容器的滚轮 | `dx, dy, ok := c.WheelFor(id)`（缩放、轮播等；不挡指针的浮层不吞滚轮） |

## 5. 取主题令牌

- 一律通过 `th := c.Theme` 读，不写死颜色。常用：`th.Primary/Success/Warning/Error/Info`（`Palette`：`Base/Hover/Active/Bg/BgHover/Border/BorderHover/Shades[10]`）、`th.Text/TextSecondary/TextTertiary/TextQuaternary/TextDisabled`、`th.Border/BorderSecondary/Split`、`th.BgContainer/BgElevated/BgLayout/BgSpotlight/BgMask`、`th.Fill*`、`th.Radius/RadiusSM/RadiusLG`、`th.ControlHeight*`、`th.FontSize*`、`th.Shadow/ShadowSecondary/ShadowTertiary`、`th.Duration*`。
- 三套设计语言：`theme.AntDesign()`、`theme.ElementPlus()`、`theme.Shadcn(...)`（`th.Flavor` 分别为 `theme.Ant/StyleAnt`、`theme.Element/StyleElement`、`theme.StyleShadcn`）。差异**优先用令牌/配方**表达：焦点环 `FocusOutline/FocusRingWidth/FocusRingOffset`（统一用 `drawFocusRing(l, th, r, rad)`）、输入框聚焦 `ControlFocusBorder/ControlOutline*`（`paintFrame` 已处理）、按钮 `ButtonFontWeight/ButtonPadding*/SecondaryBg`、选择控件 `CheckboxSize/CheckboxRadius/SwitchHeight/SwitchWidth/SwitchOffBg/SwitchHandle/SliderRailHeight/LabelFontWeight`、浮层 `PopupBorder/ModalBorder/CardBorder/CardShadow/TooltipShadow/InputShadow`、表格 `TableHeaderText/TableHeaderWeight/TableSorterIdle`、提示文字 `TextOnSpotlight`、彩色实底上的字 `TextOnSolid`、行高 `LineHeights`（shadcn 14px → 20px）。只有结构差异（shadcn 的分段 Tabs、Sonner 堆叠 Toast、Accordion）才按 `th.Flavor` 分支；新分支先想想能不能加令牌。暗色由令牌自动处理，**不要**判断 `th.Dark` 去换颜色（骨架屏、头像、插画、Addon、mark 等原来的暗色分支已改为 `SkeletonBg/AvatarBg/Illustration/AddonBg/MarkText…` 令牌），缺了就加令牌并给三套主题各设一值。
- shadcn 只有一套侧栏外观：`Menu.Dark()`（及 Defaults `theme: "dark"`）在 shadcn 下被忽略，侧栏颜色来自 `--sidebar-*` 令牌并随亮/暗主题变化（暗色侧栏即 `theme.Shadcn(theme.Dark())` 下的侧栏），不提供"亮色页面里的暗色侧栏"。
- 颜色工具在 theme：`theme.HSVA/ToHSVA/FromHSVA`、`ParseHex/ParseColor/ParseRGB`、`HexString/RGBString`、`Luma`；Segmented 用 `th.SegmentedBg/SegmentedSelectedBg`。确需按 `th.Dark` 分支的（如二维码固定白底）写注释说明原因。
- 尺寸：控件构造时把 `size` 初始化为 `c.defaultSize()`（来自 `ConfigProvider` 的全局组件尺寸，未设置为 Middle），显式 `.Size()/.Small()` 优先。
- 文字样式：`c.style(size, text.Regular)` 会带上主题行高（14px → 22px）。控件字号 `c.controlFont(size)`、高度 `c.controlHeight(size)`、圆角 `c.controlRadius(size)`。
- WebView 页面复用令牌：`th.CSSVars()` 返回 `--tango-color-primary`、`--tango-color-primary-hover`、`--tango-radius`、`--tango-control-height`、`--tango-shadow`、`--tango-motion-duration-mid` 等 CSS 变量（完整列表见 `ui/theme/cssvars.go` 的注释），`th.CSS()` / `theme.CSSRule(sel, vars)` 生成按名字排序的 `:root { ... }` 文本。新增令牌时如果页面也可能用到，同步加进 `CSSVars` 和它的测试。

- **局部主题与组件令牌**（docs/specs/rtl-and-component-tokens.md）：节点构建时记下作用域的主题、语言和方向，paint/measure 回调里的 `c.Theme`、`c.Locale()`、`c.IsRTL()` 都是该节点作用域的值，所以嵌套 `ConfigProvider.Theme/Dark/PrimaryColor/Components` 只影响子树。新组件优先读组件令牌：构造时 `th := c.ThemeFor(theme.CompButton)`、`tk := &th.Components.Button`，paint 里用 `th`/`tk`，不要在 paint 里另取全局主题。新增组件专属视觉数值一律加进 `theme.Components` 对应的 `XToken`（`ui/theme/comp_<name>.go`，默认值在 `deriveX` 里由全局令牌派生），**不再往 `Theme` 加扁平组件字段**；行为/结构属性（size、variant、placement…）仍走 `Defaults`。

## 6. 绘制（paint）

paint 回调签名 `func(l *paint.List, r geom.Rect)`，`r` 是节点的绝对矩形（逻辑像素）。

```go
l.Shadows(r, geom.Rad(8), th.Shadow)          // 多层阴影
l.Fill(r, geom.Rad(th.Radius), th.BgContainer) // 圆角填充
l.Stroke(r, geom.Rad(th.Radius), 1, th.Border) // 内描边；虚线 StrokeDashed
lay := text.Shape(s, c.style(14, text.Regular), text.Options{NoWrap: true})
l.Glyphs(geom.Pt(r.X, r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.Text)
icon.Draw(l, icon.Close, geom.Rect{...}, col, 0) // 或 drawIcon(l, ic, x, y, size, col)
l.Path(paint.NewPath().Circle(cx, cy, 4), col) // 任意路径；描边 l.StrokePath
l.Clip(r, rad) ... l.Pop()                     // 裁剪
l.Opacity(0.5) ... l.Pop()                     // 透明度组
l.Transform(geom.RotateAround(a, p)) ... l.Pop() // 仿射变换组（缩放/旋转/平移）
l.LinearStops(...) / l.Gradient(paint.Gradient{...}) // 多段线性/径向/锥形渐变
l.Checker(r, rad, cell, a, b)                  // 透明背景棋盘格
```
- 节点整体变换（动画）用 `(*Box).Transform(m)` / `(*Box).Scale(k)`：只影响绘制，命中按变换后包围盒。斜体文字：`text.Style{Italic: true}` / `Label.Italic()`（有斜体字面用真斜体，否则合成倾斜）。图片工具在 paint：`DecodeImage/ResizeBilinear/Resample/Orient/ToRGBA`。
- 复选框外观统一用 choice.go 的 `drawCheckbox`；带标签的选择器外壳（多选标签、折叠 +N、搜索输入、清除、箭头）用 selector.go 的 `newSelector`，不要再自画一套。
- 绘制在子节点之下用 `n.paint`，之上（焦点框、覆盖层）用 `n.over`；`Box` 上对应 `Paint/PaintOver`。
- 状态图标（实心圆 + 白色符号）用 `drawStatusIcon`；加载转圈用 `drawSpinner` 或 `IconW.Spin()`。
- 组件需要的图标加到内置集 `ui/icon/builtin.go`：24×24 网格、1.8 线宽、自己绘制的 SVG 路径，不得手抄第三方图标数据；不要在组件文件里 `icon.New` 私建图标，也不要用 `paint.NewPath` 手画图标。主题会按设计体系替换内置图标（`theme.Theme.Icons`：Ant 主题换成 `ui/icon/antd`，Element 换成 `ui/icon/element`，shadcn 换成 `ui/icon/lucide` 并把线宽改为 2），新增内置图标时在 `ui/theme/icons.go` 三张映射表里补上对应项。
- 三套第三方图标库（`ui/icon/element`、`antd`、`lucide`）由 `internal/cmd/icongen` 从 npm 包生成，不要手改 `icons.go`；升级版本时重新生成并更新 THIRD_PARTY_NOTICES。文件类型图标用 `ui/icon/filetype`。
- **重绘提示（可选，纯性能提示）**：组件可以声明本帧变化区域，渲染器据此只重画脏区。只能在 UI 线程调用。`c.Damage(r)`：页面构建期间或 paint/over 回调里都可以调用；r 用窗口逻辑坐标（与 `RectOf`/`Box.Rect` 同一空间）。`c.ScrollShift(r, d)`：只能在构建期间、被滚动内容的 `Body` 内调用（当前父节点记为"滚动内容"；在 `Scroll` 的回调里调用时，滚动内容是它的子节点），表示 r 内内容像素平移了 d。`(*Box).ScrollShift(r, d)`：给自己绘制滚动内容的组件（代码编辑器的行）用，声明 b 自己绘制的内容连同子树在 r 内平移了 d；可以在构建期间或 b 的 paint 回调里调用（paint 时才知道滚到了哪），规则与 `c.ScrollShift` 相同。两种平移都要求 r 内祖先背景处处一致；Runner 会把 r 内不随内容移动的节点（吸顶表头、合计行、Affix、浮层）原位与平移后的位置、露出的条带、滚动条槽并入 Damage，节点跨 r 边界、内容未绘制、祖先在 r 上有 `over` 绘制或变换时撤销平移、整块 r 重画。Runner 自动并入：`Anim/AnimFrom` 值变化的 ID 的新旧矩形（找不到该 ID 的节点就整帧）、浮层打开/关闭/移动、焦点环显示/隐藏/移动、hover/press 变化；主题、尺寸、scale、ReduceMotion 变化、首帧、上一帧 Render/Present 出错，以及时钟越过上一帧 `WakeAt` 定时或上一帧在动画中（依赖 `c.Now()` 的绘制，如输入框光标闪烁、骨架屏闪光、自动消失计时）一律整帧；`WakeAtDamaged` 定时不触发整帧。**未声明的变化有自动兜底**：只要本帧带提示（局部重画），Runner 就比较每个节点自己的绘制指纹（它的 paint/over 回调产生的绘制命令——坐标相对节点原点——加上节点矩形、裁剪、继承的透明度与变换；子节点各算各的）与上一帧，变了的节点把新旧绘制范围并入 Damage，新出现的并入新范围，消失的并入旧范围；随 ScrollShift 内容恰好平移 d 且绘制不变的节点交给拷贝，不会把整块区域标脏。所以别的组件声明了 Damage、而某个 `ui.Text` 状态栏改了文字没声明时，也不会残留旧像素。代价：带提示的帧对整棵绘制树做一次哈希（约 1250 个节点 ~20µs），不带提示的帧只记录每个节点的命令区间（~10ns/节点）。声明仍然有用：在 paint/over 回调里自己声明过（`c.Damage`、`Box.ScrollShift`、`l.Damage`/`l.Scroll`）的节点被信任，不再按指纹整块标脏——变化只占节点一小块（编辑器的光标、某几行）时，声明比指纹精细得多。注意：指纹看不见原地修改的数据——图片像素原地改了要调渲染器的 `InvalidateImage` 并声明 Damage（或换一个新的图片值），已记录的 `*paint.Path`、字形切片不要原地改。paint 回调里也可以直接 `l.Damage(r)` / `l.Scroll(r, d)`，此时坐标随变换栈。

## 7. 交互、键盘、焦点、IME

```go
ia := c.interact(n, focusable, input.CursorPointer)
// ia.Hovered ia.Pressed ia.PressedNow ia.Clicked ia.Clicks ia.Focused ia.FocusVisible
```
- 可聚焦组件自动进入 Tab 顺序；聚焦状态下 Enter/Space 视为点击，除非 `n.ownKeys = true`（自己处理按键的组件，如 Select）。
- 键盘：`for _, k := range c.KeysFor(id) {...}`（消费焦点组件的按键，Tab/Escape 不在其中）。字母/数字键为 `input.KeyA…KeyZ / Key0…Key9`，快捷键修饰用 `k.Mods.Shortcut()`。Escape：浮层用 `c.EscapeAt(panelID)`（只交给上一帧最上层的监听浮层，逐层关闭）；聚焦控件用 `c.Escape()`（按调用者所在层判定）。右键：`c.ContextMenu()` / `c.ContextMenuFor(id)`。
- 只想点击获得焦点、Tab 不经过的组件（可选中文本）设 `n.noTab = true`。
- 焦点环：`ia.FocusVisible` 时调用 `drawFocusRing(l, th, r, rad)`（Ant 4px 外发光、Element 2px 描边、shadcn 2px ring + 2px offset，全部来自令牌）。
- 拖拽：`c.Dragging(id)` + `c.Pointer()`。
- 指针接管（拖动冒泡）：滚动/滑动容器声明 `box.TakePointer(ui.PanY)`（`PanX/PanY/PanXY`，相当于 CSS `touch-action`），拖动起点落在其中的普通控件（输入框、按钮）上、移动超过 `ui.DragSlop` 且方向匹配时，最内层匹配的容器接管指针：`c.Dragging(box.ID())` 变真、`c.PointerTaken(id)` 报告接管与按下点；原控件 `c.PointerCancelled(id)` 为真一帧（系统取消、窗口失焦也会报告），抬起不产生点击，按下给它的焦点退回。自己处理同向拖动的控件在持有指针期间调用 `c.KeepPointer(id)` 拒绝接管；持有指针的本身就是声明了轴的容器时不再被外层接管（由它自己仲裁）。不声明则行为不变。
- 触摸：宿主送 `input.TouchEvent`（多指，见 `docs/specs/touch-input.md`），第一根手指自动合成指针事件，普通组件无需处理。需要多指的盒子每帧调用 `box.Touches()` 取本帧绑定到它的触摸（按上一帧几何路由，按下后一直绑定到抬起）；`box.RawTouch()` 让落在其中的手指不再合成指针。`c.TouchCount()` 为屏幕上的手指数。
- 安全区：贴边组件（导航栏、标签栏、底部弹层）用 `c.SafeArea()`（逻辑像素 `geom.Insets`）留白；变化时整帧重画，组件不用声明 Damage。
- 文本输入不要自己写：复用 `editState` + `c.processEdit(...)` + `c.paintEditor(...)`，它已经处理光标、选区、剪贴板、词跳转、IME 组合串（`input.Composition`）与 `c.WantTextInput(caret)`。参考 `InputW`。
- 禁用：`n.disabled`（`ui.Disabled(c, true, fn)` 会传递给子树）；禁用时 `n.focus=false`、`n.cursor=input.CursorNotAllowed`，查询方法返回 false。

## 8. 浮层（下拉、弹出、对话框）

```go
ov := ui.Overlay(c, ui.OverlayOptions{Anchor: n.id, Place: ui.BottomStart, Offset: 4, MatchWidth: true, Flip: true}, func() {
    // 浮层内容，正常构建
}, layout.Pad(4))
ov.Bg(th.BgElevated).Radius(th.RadiusLG).Shadow(th.Shadow).Opacity(c.AnimFrom(n.id, "open", 0, 1, th.DurationMid))
st.panel = ov.n.id // 记住浮层 ID
if st.open && c.Dismissed(st.panel, n.id) { st.open = false } // Esc 或外部点击；点在更高层浮层上不算外部
```
- 浮层按创建顺序叠放，后建的在上；浮层会挡住下层的指针。全屏遮罩用 `Place: ui.ScreenFill`；锚点两侧有 `Left/Right`（垂直居中）、`LeftStart/RightStart`、`LeftEnd/RightEnd`，均在布局阶段按本帧尺寸放置，不要用上一帧面板尺寸自己算偏移；居中/顶部/右侧/抽屉有 `ScreenCenter/ScreenTop/ScreenTopRight/ScreenRight/ScreenLeft/ScreenTopCenter`；进出场位移用 `ShiftX/ShiftY`。
- 打开状态保存在 `ui.State` 里；外部点击判断要用**浮层 ID 查上一帧矩形**，不要缓存矩形本身。
- 对话框类自己管理 Esc：`if c.EscapeAt(panel.ID()) {...}`（每帧调用即登记，只有上一帧最上层的监听浮层拿到 Esc）；只想判断外部点击用 `c.PressedOutside(ids...)`；右键菜单用 `c.ContextMenuFor(id)`。
- 没有触发器控件的右键菜单（编辑器、画布）：在 `c.ContextMenuFor(id)` 报告右键时调用一次 `ui.ContextMenuAt(c, ownerID, pos, items, onSelect)`，菜单在全局层里显示到选中 / Esc / 外部按下为止，焦点回到 owner；`CloseContextMenu`、`ContextMenuOpen` 查询或关闭。代码编辑器式不预选：`ui.ContextMenuAtWith(c, ownerID, pos, items, onSelect, ui.ContextMenuOptions{NoAutoSelect: true})`，打开时焦点落在菜单面板本身（role menu、名称取 `Locale().A11y.Menu`、不是 Tab 停靠点），无高亮项，↓/Home 到首个可用项、↑/End 到末项、字母键 typeahead，未高亮时 Enter/Space 不执行。菜单项的 `MenuItem.Shortcut` 在 Dropdown、Menu 弹出子菜单和右键菜单里显示为末端的快捷键列（令牌 `Dropdown.Shortcut*` / `Menu.PopupShortcut*`）。
- 全局提示类（Message/Notification）用 `globalLayers` 注册渲染函数，见 `nav_notify.go`；全局层里组件注册的 deferred 同样会被执行。
- 其它 goroutine 改了状态后唤醒一帧：`ui.Invalidate(win)` 或预先取 `wake := c.Invalidator()` 再在 goroutine 里调用 `wake()`。

## 9. 表单接入

控件创建时读取 `c.itemStatus`（FormItem 校验失败时为 `"error"`）作为默认状态：

```go
w := &MyW{c: c, status: c.itemStatus}
```
这样放进 `form.Item("名称").Rules(&v, ui.Required("必填")).Body(...)` 就会自动显示错误态。

## 10. 范例：Tag（完整实现，见 `display.go`）

```go
type TagW struct {
	c        *Context
	n        *node
	label    string
	color    string
	closable bool
	ia       Interaction
}

func Tag(c *Context, label string) *TagW {
	t := &TagW{c: c, label: label}
	t.n = c.add("tag", label)      // 1. 建节点（label 参与 ID，也供测试定位）
	t.n.ln.NoStretch = true         // 2. 布局：列里不拉伸
	t.n.ln.Measure = t.measure      // 3. 尺寸随文字
	t.n.paint = t.paint             // 4. 推迟到 paint 阶段绘制
	return t
}

func (t *TagW) Color(col string) *TagW { t.color = col; return t } // 链式修饰器
func (t *TagW) Closable() *TagW {
	t.closable = true
	t.ia = t.c.interact(t.n, false, input.CursorDefault) // 需要交互时才注册命中
	return t
}
func (t *TagW) Closed() bool { // 查询方法
	r := t.c.rects[t.n.id]
	return t.closable && t.ia.Clicked && t.c.pointer.X > r.Right()-18
}

func (t *TagW) measure(_, _ float32) geom.Size {
	w := text.Width(t.label, t.c.style(t.c.Theme.FontSizeSM, text.Regular))
	return geom.Size{W: w + 16, H: 22}
}

func (t *TagW) paint(l *paint.List, r geom.Rect) {
	th := t.c.Theme
	p := th.ColorOf(t.color)                         // 令牌，而不是写死颜色
	l.Fill(r, geom.Rad(th.RadiusSM), p.Bg)
	l.Stroke(r, geom.Rad(th.RadiusSM), 1, p.Border)
	lay := text.Shape(t.label, t.c.style(th.FontSizeSM, text.Regular), text.Options{NoWrap: true})
	l.Glyphs(geom.Pt(r.X+8, r.Y+(r.H-lay.Height)/2), lay.Glyphs, p.Base)
}
```

## 11. Gallery 页面

在 `examples/ui-gallery/` 新建 `page_<组件>.go`，只注册，不改其它文件：

```go
func init() {
	gallery.Register(gallery.Page{
		Name: "Rate", Title: "Rate 评分", Group: gallery.Form, Order: 20,
		Render: func(c *ui.Context) { ui.Rate(c, &rateVal) },
		// 可选：Height 固定截图高度（默认按内容），Setup 在截图前驱动交互：
		Setup: func(d *ui.Driver) { d.ClickLabel("…") },
	})
}
```
运行 `go run ./examples/ui-gallery -only Rate`，输出 `build/ui-gallery/components/Rate.png`。**必须用图片查看器亲自对照 Ant Design / Element Plus 官网检查间距、圆角、字号、颜色、对齐。** `General` 与 `Form` 分组会自动进入 `overview.png`。

- 主题维度：`go run ./examples/ui-gallery -theme shadcn` 把每个非变体页面再以 shadcn 亮/暗渲染成 `<Name>-shadcn.png`、`<Name>-shadcn-dark.png`（`-theme all` 两种都出）。变体页（`-Element/-Dark`）设 `Page.Variant = true`，页面自身的交互（不切主题）放 `Page.Interact`，主题轮次会跳过变体并调用 Interact。`gallery.Home` 的目录页头可切换 Ant Design / Element Plus / shadcn/ui 与暗色。
- 每个组件只有一个页面（Name 与公开组件名一致）。同一组件的主题变体截图用 `Name-Dark/Name-Element` 这类带 `-` 的名字；同一组件的额外演示段（如 Table 的各功能、Image 预览）设 `Parent: "Table"`，首页会把它们并到父页面下显示。
- Gallery 首页：`index.png` 按分组列出全部组件；`gallery.Home(c, &sel)` 是可交互的分组浏览页（侧边菜单 + 目录卡片），宿主程序可直接挂到窗口里。

## 12. 测试

- 交互用 `ui.NewDriver(page, ui.DriverOptions{...})`：`d.Frame()`、`d.Click(p)`、`d.ClickLabel("文字")`、`d.FindText("文字")`（含非交互节点）、`d.Drag(from, to)`、`d.Move`、`d.Type`、`d.Compose`（IME）、`d.Key(input.KeyEnter, mods)`、`d.Scroll`、触摸 `d.Tap`/`d.TouchDrag`/`d.Touch(id, phase, p)`/`d.Touches(evs...)`/`d.Pinch`/`d.TwoFingerPan`、`d.SetSafeArea(in)`、`d.Advance(dur)`+`d.Settle()`。默认关闭动画，时间是模拟的，结果确定。
- 组件状态机测试写在 `ui/<组件>_test.go`，参考 `ui_test.go`（点击、hover、焦点、编辑、IME、浮层关闭）。
- 黄金图：把组件加进 `golden_test.go` 的 `goldenPage` 或新增用例，`go test ./ui -run Golden -update` 生成 `ui/testdata/*.png`；黄金图与字体列表 `testdata/golden_fonts.txt` 绑定，字体不同的机器会自动跳过。
- 提交前：`go vet ./ui/... && go test ./ui/...`，并确认 `GOOS=windows`、`GOOS=linux`、`GOOS=js GOARCH=wasm` 下 `go build ./ui/...` 通过。

## 13. 不要做的事

- 不要在组件里直接 `time.Now()`、开 goroutine 或持有 `*Context` 跨帧。
- 不要复制 Viva、mygo、Wails 或其他项目的代码、常量表、图标数据；设计令牌和色值按 Ant Design / Element Plus 官方文档。
- 不要改 `ui/theme`、`ui/paint`、`ui/text`、`ui/layout`、`ui/input` 与 `context.go/runner.go/overlay.go/driver.go` 来满足单个组件；需要扩展先提出。
- 不要在组件文件里私建通用工具（颜色换算、图片缩放、日期网格、图标、复选框/选择器外壳、悬停延迟、子树命中判断、滚到可见）：先查基础层（theme/paint/icon、date_util.go、choice.go、selector.go、hover.go、layers.go、reveal.go），没有就补到基础层。
- 新组件先查第 14 节总表，已有同类组件就扩展它，不要再加一个同义的新名字（shadcn 的 Sheet = `Drawer`、Accordion = `Collapse`、HoverCard = `Popover.Trigger("hover")`、Separator = `Divider`、Sonner = `Message/Notification`、Badge = `Tag`，只做外观）。shadcn 特有组件文件以 `shad_` 开头。
- 不要在组件文件里写死界面文案（中文、英文都不行），见第 15 节；`TestNoHardcodedCJK` 会拦住 CJK 字面量。

## 14. 组件总表

去重整合后每个组件只有一个公开入口（2026-11 定稿，后续多语言接线以此为准；2026-11-13 补入第二轮 Pro 组件与面板/虚拟化组件）。「对标」列写 Ant Design / Element Plus 的对应组件；子部件、辅助函数写在同一行。基础容器与工具（`Row/Column/Div/StackBox/Custom/Spacer/Gap/Key/Disabled/State/Prop/Overlay/HoverDelay/SetHoverOpen/ScreenBreakpoint`）不计入。

| 分组 | 组件 | 对标 Ant / Element | 文件 |
|---|---|---|---|
| 通用 | `Button`、`IconButton` | Button / el-button | button.go |
| 通用 | `Icon` | Icon / el-icon | text.go（图标数据 ui/icon） |
| 通用 | `Text`、`Title`、`Paragraph`、`Link` | Typography / el-text、el-link | text.go、data_typography.go |
| 通用 | `Divider`、`VDivider` | Divider / el-divider | display.go |
| 通用 | `FloatButton`、`FloatButtonGroup`、`BackTop` | FloatButton、BackTop / el-backtop | nav_float.go |
| 通用 | `ConfigProvider` | ConfigProvider / el-config-provider | nav_config.go |
| 通用 | `App`、`UseApp` | App（App.useApp） | pro4_app.go |
| 通用 | `BorderBeam` | Magic UI Border Beam（流光描边装饰） | fill_border_beam.go |
| 布局 | `Layout`、`Header`、`Sider`、`Content`、`Footer` | Layout / el-container | prolayout.go |
| 布局 | `ProLayout` | ProLayout | prolayout.go |
| 布局 | `CollapsibleSider` | Layout.Sider collapsible、breakpoint / el-aside + 折叠 | prolayout.go |
| 布局 | `PageContainer`、`FooterToolbar` | PageContainer、FooterToolbar | pro2_page.go |
| 布局 | `SettingDrawer`、`ProSettingsProvider`、`DefaultProSettings`（`ProSettings.Apply`） | ProLayout SettingDrawer | pro4_setting.go |
| 布局 | `RouteContext`、`GridContent` | ProLayout RouteContext、GridContent | pro4_route.go |
| 布局 | `Masonry` | Masonry（瀑布流） | fill_masonry.go |
| 布局 | `GridRow`、`Col` | Grid（Row/Col） / el-row、el-col | nav_grid.go |
| 布局 | `Flex` | Flex | nav_space.go |
| 布局 | `Space` | Space / el-space | nav_space.go |
| 布局 | `Compact`、`Addon` | Space.Compact、Input.Group / el-input-group | entry_compact.go |
| 布局 | `Splitter`、`SplitterPanel` | Splitter / el-splitter | nav_splitter.go |
| 布局 | `Scroll`、`ScrollH` | （滚动容器）/ el-scrollbar | scroll.go、reveal.go |
| 导航 | `Menu` | Menu / el-menu | nav_menu.go |
| 导航 | `Tabs` | Tabs / el-tabs | nav.go |
| 导航 | `Breadcrumb`、`BreadcrumbItems`（items 写法、下拉、分隔符） | Breadcrumb / el-breadcrumb | nav.go |
| 导航 | `Pagination` | Pagination / el-pagination | nav.go |
| 导航 | `Dropdown` | Dropdown、Dropdown.Button / el-dropdown；`ContextMenuAt` 无触发器右键菜单 | nav_dropdown.go、nav_contextmenu.go |
| 导航 | `Steps` | Steps / el-steps | nav_steps.go |
| 导航 | `Anchor`、`AnchorSection` | Anchor / el-anchor | nav_affix.go |
| 导航 | `Affix` | Affix / el-affix | nav_affix.go |
| 导航 | `PageHeader` | PageHeader / el-page-header | nav_pageheader.go |
| 导航 | `Command`（`CommandItem`：分组、图标、快捷键、关键词、禁用） | shadcn Command | shad_command.go |
| 通用 | `Toggle`、`ToggleGroup` | shadcn Toggle / ToggleGroup | shad_toggle.go |
| 通用 | `Kbd` | shadcn Kbd / `<kbd>` | shad_kbd.go |
| 导航 | `Menubar`（`MenubarMenu`、`MenubarItem`：子菜单、勾选项、单选组、分隔线、快捷键提示、禁用；Alt/F10 进入；下拉面板复用 Dropdown 的 `navMenuPopup`） | shadcn Menubar | menubar.go |
| 布局 | `AspectRatio` | shadcn AspectRatio / CSS aspect-ratio | aspect_ratio.go |
| 数据展示 | `Item`、`ItemGroup`（default/outline/muted 变体、default/sm 尺寸、可作链接或按钮、分隔） | shadcn Item | item.go |
| 数据录入 | `Input` | Input / el-input | input.go |
| 数据录入 | `TextArea` | Input.TextArea / el-input type=textarea | entry_inputx.go |
| 数据录入 | `Search` | Input.Search | entry_inputx.go |
| 数据录入 | `OTP` | Input.OTP / el-input-otp | entry_otp.go |
| 数据录入 | `InputNumber` | InputNumber / el-input-number | input.go |
| 数据录入 | `Select` | Select / el-select | select.go |
| 数据录入 | `VirtualSelect` | Select virtual / el-select-v2 | fill_virtual_select.go |
| 数据录入 | `MultiSelect` | Select mode=multiple / el-select multiple | entry_multiselect.go、selector.go |
| 数据录入 | `AutoComplete` | AutoComplete / el-autocomplete | entry_autocomplete.go |
| 数据录入 | `Mentions` | Mentions / el-mention | entry_mentions.go |
| 数据录入 | `Cascader`、`CascaderPanel` | Cascader（Cascader.Panel） / el-cascader、el-cascader-panel | entry_cascader.go、entry_cascader_panel.go |
| 数据录入 | `TreeSelect` | TreeSelect / el-tree-select | entry_treeselect.go |
| 数据录入 | `InputTag` | Select mode=tags / el-input-tag | entry_inputtag.go |
| 数据录入 | `Checkbox`、`CheckboxGroup` | Checkbox / el-checkbox | choice.go |
| 数据录入 | `CheckAll` | Checkbox indeterminate 全选 | entry_checkall.go |
| 数据录入 | `Radio`、`RadioGroup` | Radio / el-radio | choice.go |
| 数据录入 | `Switch` | Switch / el-switch | choice.go |
| 数据录入 | `Slider`、`SliderRange` | Slider（含 range） / el-slider range | choice.go、p2b_slider.go |
| 数据录入 | `Rate` | Rate / el-rate | entry_rate.go |
| 数据录入 | `Segmented` | Segmented / el-segmented | entry_segmented.go |
| 数据录入 | `DatePicker` | DatePicker / el-date-picker | date.go、date_util.go |
| 数据录入 | `DatePickerPanel` | DatePicker 内嵌面板（日/周/月/季/年，含范围） / el-date-picker-panel | fill_date_panel.go、p2a_date.go |
| 数据录入 | `RangePicker` | DatePicker.RangePicker / el-date-picker type=daterange | entry_daterange.go |
| 数据录入 | `DateTimePicker` | DatePicker showTime / el-date-picker type=datetime | entry_datetime.go |
| 数据录入 | `TimePicker`、`TimePanel` | TimePicker / el-time-picker | entry_time.go |
| 数据录入 | `TimeRangePicker` | TimePicker.RangePicker / el-time-picker is-range | p2a_time_range.go、p2a_range_time.go |
| 数据录入 | `TimeSelect` | — / el-time-select | more_timeselect.go |
| 数据录入 | `ColorPicker` | ColorPicker / el-color-picker | entry_color.go |
| 数据录入 | `ColorPickerPanel` | ColorPicker 内嵌面板 / el-color-picker-panel | fill_color_panel.go |
| 数据录入 | `Transfer` | Transfer / el-transfer | entry_transfer.go |
| 数据录入 | `Upload` | Upload / el-upload | entry_upload.go、p2d_upload.go |
| 数据录入 | `DropZone` | Upload.Dragger 的拖放悬停区（独立包裹） | p2e_drop.go |
| 数据录入 | `CheckCard`、`CheckCardGroup` | CheckCard | pro2_checkcard.go |
| 数据录入 | `Form`（`FormItem` 与 `Required/Email/MinLen/MaxLen/Pattern/Match/Check/AsyncCheck` 规则；`Name/Dependencies/OnDepsChange/Hidden/Disabled` 联动；`Submit/Finished/ScrollToFirstError`） | Form / el-form | form.go |
| 数据录入 | `FormList`（`FormListRow`） | Form.List、ProFormList | form_pro_list.go |
| 数据录入 | `ProForm`（`ProField`、`ProValues`，值类型与 ProTable 共用） | ProForm | form_pro.go、form_pro_field.go |
| 数据录入 | `StepsForm`（`StepsFormStep`） | StepsForm | form_pro_steps.go |
| 数据录入 | `ModalForm`、`DrawerForm` | ModalForm、DrawerForm | form_pro_modal.go |
| 数据录入 | `QueryFilter` | QueryFilter（ProTable 查询表单复用） | form_pro_query.go |
| 数据录入 | `LightFilter` | LightFilter | pro3_lightfilter.go |
| 数据录入 | `ProFormFieldSet`、`FieldSet` | ProFormFieldSet | pro3_fieldset.go |
| 数据录入 | `ProFormGroup`、`ProFormItems`（`ProField.Group` 自动分组） | ProFormGroup | pro4_group.go |
| 数据录入 | `LoginForm`、`LoginFormPage` | LoginForm、LoginFormPage | pro3_login.go |
| 数据录入 | `editor.View`、`editor.Model`、`editor.State`（代码编辑、语法高亮、撤销重做、只读与行号） | Go 自绘代码编辑器 | editor/view.go、editor/model.go、editor/state.go |
| 数据录入 | `MarkdownEditor` | — | more_mdeditor.go |
| 数据展示 | `Table`（`Wrap` 可变行高、树形勾选联动/`CheckStrictly`、`Sticky/StickySummary` 吸顶吸底、拖拽排序边缘自动滚动） | Table / el-table | table*.go |
| 数据展示 | `ProTable`（列设置含固定到左/右） | ProTable | table_pro.go |
| 数据展示 | `EditableProTable` | EditableProTable | p2e_editable.go |
| 数据展示 | `VirtualList` | List virtual / el-table-v2 思路 | more_virtual.go |
| 数据展示 | `InfiniteScroll` | — / v-infinite-scroll | more_infinite.go |
| 数据展示 | `List`、`ListItemMeta`、`ListActions` | List | data_list.go |
| 数据展示 | `Listy`（虚拟、分组、吸顶分组头） | List virtual / el-table-v2 思路 | fill_listy.go |
| 数据展示 | `ProList` | ProList | pro3_list.go |
| 数据展示 | `Tree` | Tree / el-tree | data_tree.go |
| 数据展示 | `VirtualTree` | Tree virtual / el-tree-v2 | fill_virtual_tree.go |
| 数据展示 | `Tag` | Tag / el-tag | display.go |
| 数据展示 | `CheckableTag` | Tag.CheckableTag / el-check-tag | more_checktag.go |
| 数据展示 | `Badge`、`BadgeRibbon` | Badge、Badge.Ribbon / el-badge | data_badge.go |
| 数据展示 | `Avatar`、`AvatarGroup` | Avatar / el-avatar | display.go、data_badge.go |
| 数据展示 | `Card`、`CardMeta`、`CardAction` | Card / el-card | data_card.go |
| 数据展示 | `ProCard` | ProCard | pro2_card.go |
| 数据展示 | `Descriptions` | Descriptions / el-descriptions | data_card.go |
| 数据展示 | `ProDescriptions` | ProDescriptions | pro3_descriptions.go |
| 数据展示 | `Statistic`、`Countdown` | Statistic / el-statistic、el-countdown | display.go、data_skeleton.go |
| 数据展示 | `StatisticCard`、`StatisticCardGroup` | StatisticCard、StatisticCard.Group | pro2_statistic.go |
| 数据展示 | `Progress` | Progress / el-progress | data_progress.go |
| 数据展示 | `Empty` | Empty / el-empty | data_result.go |
| 数据展示 | `Result` | Result / el-result | data_result.go |
| 数据展示 | `Skeleton`、`SkeletonAvatar/Button/Input/Image` | Skeleton / el-skeleton | data_skeleton.go |
| 数据展示 | `ProSkeleton` | ProSkeleton | pro2_skeleton.go |
| 数据展示 | `Tooltip` | Tooltip / el-tooltip | display.go |
| 数据展示 | `Popover` | Popover / el-popover | data_popover.go |
| 数据展示 | `Tour` | Tour / el-tour | data_popover.go |
| 数据展示 | `Collapse` | Collapse / el-collapse | data_collapse.go |
| 数据展示 | `Timeline` | Timeline / el-timeline | data_timeline.go |
| 数据展示 | `Calendar` | Calendar / el-calendar | data_calendar.go |
| 数据展示 | `Carousel` | Carousel / el-carousel | data_carousel.go |
| 数据展示 | `Image` | Image（含预览）/ el-image | data_image.go |
| 数据展示 | `QRCode` | QRCode | data_qrcode.go |
| 数据展示 | `Watermark` | Watermark / el-watermark | data_watermark.go |
| 数据展示 | `Markdown` | — | more_markdown.go |
| 数据展示 | `RichText` | — | more_richtext.go |
| 数据展示 | `CodeBlock`（只读代码、行号、复制、横向滚动/换行、选择复制；高亮经 `SetCodeHighlighter` 钩子，`ui/codehl` 用编辑器语法实现，Markdown 代码块共用） | — | codeblock.go、code_highlight.go、viewer_text.go |
| 数据展示 | `LogView`、`LogBuffer`（百万行虚拟滚动、ANSI SGR `ParseANSI`、级别过滤、搜索、跟随底部、换行） | — | logview.go、logview_buffer.go、ansi.go |
| 数据展示 | `JSONView`、`JSONViewValue`、`JSONViewSource`（`ValueSource` 懒加载值树、100 项分组、复制值/路径、搜索） | — | jsonview.go、jsonview_source.go |
| 反馈 | `Alert` | Alert / el-alert | feedback.go |
| 反馈 | `Modal` | Modal / el-dialog | feedback.go |
| 反馈 | `Drawer` | Drawer / el-drawer | feedback.go |
| 反馈 | `Confirm` | Modal.confirm / ElMessageBox.confirm | more_confirm.go |
| 反馈 | `MessageBox` | Modal.info/success… / ElMessageBox.alert | more_confirm.go |
| 反馈 | `Prompt`、`OpenDialogs`（打开中的 Confirm/MessageBox/Prompt 数） | — / ElMessageBox.prompt | more_confirm.go |
| 反馈 | `Popconfirm` | Popconfirm / el-popconfirm | feedback.go |
| 反馈 | `Message`、`MessageDestroy` | message / ElMessage | nav_notify.go |
| 反馈 | `Notification`、`NotificationDestroy` | notification / ElNotification | nav_notify.go |
| 反馈 | `Spin` | Spin | feedback.go |
| 反馈 | `Loading` | Spin 包裹 / v-loading | feedback.go |
| 图表 | `chart.Line`、`chart.Area`（平滑、阶梯、面积、堆叠、对数轴、LTTB 降采样） | Line、Area / ECharts line | chart/line.go |
| 图表 | `chart.Column`、`chart.Bar`（分组、堆叠、百分比堆叠） | Column、Bar / ECharts bar | chart/column.go |
| 图表 | `chart.Pie`、`chart.Donut`（引导线标签、内部标签、中心统计） | Pie / ECharts pie | chart/pie.go |
| 图表 | `chart.Scatter`、`chart.Bubble` | Scatter / ECharts scatter | chart/scatter.go |
| 图表 | `chart.Radar` | Radar / ECharts radar | chart/radar.go |
| 图表 | `chart.Gauge` | Gauge / ECharts gauge | chart/gauge.go |
| 图表 | `chart.Funnel` | Funnel / ECharts funnel | chart/funnel.go |
| 图表 | `chart.Heatmap` | Heatmap / ECharts heatmap + visualMap | chart/heatmap.go |
| 图表 | `chart.DualAxes` | DualAxes（柱 + 线）/ ECharts 双 yAxis | chart/dual.go |
| 图表 | `chart.TinyLine`、`chart.TinyArea`、`chart.TinyColumn`、`chart.TinyProgress` | Tiny 系列 / ECharts sparkline 写法 | chart/tiny.go |
| 图表 | `chart.Liquid` | Liquid / echarts-liquidfill | chart/liquid.go |
| 图形 | `graph.Diagram`（节点模板/自定义绘制、直线/正交/贝塞尔连线、端口、平移缩放、框选、拖动吸附、小地图）、`graph.Layout`（分层布局） | React Flow / X6 / G6 思路 | graph/diagram.go、graph/layered.go |
| 图形 | `graph.ER`（表节点、字段行外键连线） | — | graph/er.go |
| 图形 | `graph.GitGraph`、`graph.LayoutCommits`（泳道布局、虚拟列表） | — | graph/gitgraph.go、graph/gitlayout.go |
| 移动端 | `mobile.Provider`、`mobile.ThemeOf`、`mobile.NewTheme`（Ant Design Mobile / Vant 两套令牌，暗色随 ui 主题；ui 主题为 `theme.HighContrast` 时文字抬到 4.5:1、箭头/分隔线/开关轨道抬到 3:1） | ConfigProvider / van-config-provider | mobile/theme.go |
| 移动端 | `mobile.Area`、`mobile.Within`、`mobile.Touchable`（手势仲裁：点按、长按、滑动、拖拽；双指捏合来自 `input.TouchEvent`；起点在普通控件上的拖动经 `TakePointer` 交给手势区） | — / @vant/use useTouch | mobile/arena.go、mobile/recognizer.go |
| 移动端 | `mobile.Scroller`、`mobile.Decay`、`mobile.Spring`、`mobile.Rubber`（惯性、回弹、吸附） | — | mobile/physics.go |
| 移动端 | `mobile.ScrollView`、`mobile.PullToRefresh`、`mobile.InfiniteList`、`mobile.RefreshMachine`；长列表用 `ScrollViewW.Rows(n, rowH, row)`（定高行，只构建可见行） | PullToRefresh、InfiniteScroll / van-pull-refresh、van-list | mobile/scrollview.go |
| 移动端 | `mobile.NavBar`、`mobile.TabBar` | NavBar、TabBar / van-nav-bar、van-tabbar | mobile/navbar.go |
| 移动端 | `mobile.List`、`mobile.Cell`、`mobile.SwipeCell` | List、SwipeAction / van-cell、van-swipe-cell | mobile/list.go |
| 移动端 | `mobile.Popup`、`mobile.ActionSheet`、`mobile.Dialog`、`mobile.Toast`、`mobile.ShowToast` | Popup、ActionSheet、Dialog、Toast / van-popup、van-action-sheet、van-dialog、van-toast | mobile/overlay.go |
| 移动端 | `mobile.PickerView`、`mobile.CascadePickerView`、`mobile.DatePickerView`、`mobile.Picker` | Picker、CascadePicker、DatePicker / van-picker、van-date-picker | mobile/picker.go |
| 移动端 | `mobile.Form`、`mobile.FormItem`、`mobile.Field`、`mobile.SearchBar`、`mobile.Stepper` | Form、Input、SearchBar、Stepper / van-form、van-field、van-search、van-stepper | mobile/form.go |
| 移动端 | `mobile.Tabs`、`mobile.Swiper`（指示点颜色 `.IndicatorColor` / `.IndicatorWhite`） | Tabs、Swiper / van-tabs、van-swipe | mobile/tabs.go |
| 移动端 | `mobile.Button`、`mobile.Badge`、`mobile.Tag`、`mobile.Grid`、`mobile.NoticeBar`、`mobile.Skeleton`、`mobile.Empty`、`mobile.Result` | 同名组件 / van-button、van-badge、van-tag、van-grid、van-notice-bar、van-skeleton、van-empty | mobile/display.go |
| 移动端 | `mobile.IndexBar`、`mobile.FloatingBubble`、`mobile.SafeArea`（读 `c.SafeArea()`） | IndexBar、FloatingBubble、SafeArea / van-index-bar、van-floating-bubble | mobile/indexbar.go、mobile/theme.go |

已合并删除的旧名（不要再引入）：`StepBar`、`DropdownMenu`、`DropdownButton`、`NavMenu`、`Dialog`、`DrawerPanel`、`Spinner`、`LoadingMask`、`SpaceBox`、`SpaceCompact`、`InputGroup`、`MessageOpen`、`MessageLoading`、`Notify`、`NotificationClose`、`ProgressX`、`CardX`、`DescriptionsX`、`BadgeX`、`BadgeDot`、`StatusDot`、`EmptyX`、`Textarea`、`CalendarView`、`Scrollbar`、`AppLayout`、`HLayout`、`Container*`、`CheckAllLabel`；`DecodeImage`/`ResizeBilinear` 已移到 `ui/paint`。

## 15. 文案与多语言

组件里所有给用户看的文字——按钮、占位符、空状态、提示气泡、无障碍名（`c.add(kind, label)`、`IconButton(c, ic, label)` 的 label）、日期面板标题与星期——**一律从 `c.Locale()` 取**，不写字面量。

```go
loc := c.Locale()
Button(c, loc.OK)                                  // 通用字段
Empty(c).Simple().Description(loc.NoData)
IconButton(c, icon.Close, loc.Tour.Close)          // 组件分组字段
Text(c, loc.Total(total))                          // 带数字的文案用辅助方法，不要自己 Sprintf
Text(c, loc.YearMonth(y, m))                       // "2026年11月" / "Nov 2026"
for _, d := range loc.WeekdayNames() { ... }       // 已按 loc.WeekStart 旋转
label := loc.WithColon(it.Label)                   // 表单/描述列表的冒号："：" / ":"
```

- 语言包：类型与注册在 `ui/locale.go`，内置 `ui.ZhCN`（默认）、`ui.EnUS`、`ui.ZhTW`、`ui.Ja` 在 `ui/locale_packs.go`。`ConfigProvider(c).Locale(ui.EnUS)` 局部切换；`ui.SetDefaultLocale(l)` 改全局默认；`ui.LocaleByCode("zh-HK")` 按标签查（简繁按书写系统分流）。
- **新文案的加法**：通用词放 `Locale` 顶层字段；只属于一个组件的放该组件的分组结构（`l.Image.ZoomIn`、`l.ProTable.Density`，没有就新建 `XxxLocale` 分组）；**四个内置包都要补齐**（`TestLocalePacksComplete` 检查空字段）。应用自己注册的语言包缺的字段由 `RegisterLocale` 从 EnUS 补。第三方组件不改 `Locale` 时用 `l.T("MyWidget.key", fallback)` 读 `Extra`；包 ui 自己的组件**不用** `T()` 兜底（Pro 组件的文案在 `DatePicker`、`TimePicker`、`Tabs`、`Pagination`、`Slider`、`DropZone`、`EditableTable`、`PageContainer`、`ProCard`、`StatisticCard`、`ProField`、`ProDescriptions`、`LightFilter`、`LoginForm` 等分组里，货币符号是顶层 `Currency`）。
- 带数字/年月的格式串用 fmt 动词，辅助方法会传齐参数，格式串可以只用一部分或用显式下标（`MonthFmt` 参数是月份数字与 `Months[m-1]`，英文写 `"%[2]s"`）。
- **在 build 阶段取 locale**：paint/measure 回调运行时 ConfigProvider 作用域已经结束，`c.Locale()` 会退回全局默认。需要在绘制里画文字的组件，在构造时把 `c.Locale()`（或算好的字符串）存进构建器字段（参考 `ImageW.loc`、`QRCodeW.loc`、`UploadW.loc`）。`c.lazy` 已在延迟执行时恢复创建时的 ConfigProvider 作用域，lazy 里的 build 直接调用 `c.Locale()` 即可。
- 星期起始日、年月写法随语言走（`WeekStart`、`YearMonthFmt`），不要再按 `th.Flavor` 分支。
- 不是界面文案的 CJK 字面量（用户输入的分隔符 `"，"`、断行标点表、测字形用的探针字）在同一行加 `// i18n:ok <原因>`。
- 测试：组件带新文案时把它加进 `ui/i18n_wiring_test.go` 的用例表（四种语言下渲染，断言本语言文案出现、其他语言文案不出现；EnUS 下不得出现任何 CJK 标签）；只在 paint 里画的文字加到 `TestI18nPaintedTexts`。

## 16. 图表（ui/chart）

图表在子包 `github.com/tangoui/tango/ui/chart`，只用 ui 的公开 API 搭建（`ui.Div/Custom/Overlay/State`、`c.Anim*`、`paint`、`text`），写法与普通组件一致：链式修饰，以 `Show()` 结尾。

```go
rows := []chart.Row{{"month": "1月", "city": "东京", "temp": 7.0}, ...} // chart.Row = map[string]any
chart.Line(c, rows).X("month").Y("temp").Series("city").Smooth().Show()
chart.Column(c, rows).X("month").Y("sales").Series("shop").Stack().Label().Show()
chart.Pie(c, rows).Angle("value").Color("type").Inner(0.64).Center("总计", "3,267").Show()
chart.DualAxes(c, rows).X("month").Column("sales").Line("rate").LineFormat(chart.Percent(0)).Show()
chart.TinyArea(c, []float64{3, 5, 2, 8}).Smooth().Show() // 统计卡片
v := chart.Heatmap(c, rows).X("hour").Y("day").Value("count").Show()
if tip, ok := v.Tooltip(); ok { ... } // View：*ui.Box + 本帧悬停内容 + v.Hidden(name)
```

- **数据**：`[]chart.Row`，字段值可以是任意 Go 数值、字符串（类目）或 `time.Time`；`chart.RowsOf(items, fn)` 把结构体切片转成行。X 字段类型自动识别：字符串 → 类目轴，`time.Time` → 时间轴（按跨度自动选秒/分/时/日/周/月/年刻度，跨年处显示年份），数字 → 线性轴；柱状图、热力图的 X 总按类目处理。缺失值用 nil，折线会跳过该点。2000 行以上的数据会缓存解析结果（按切片地址 + 抽样指纹），大数据请整体替换切片而不是原地修改。
- **通用修饰**（所有图表）：`Height`、`Colors`、`Legend(chart.LegendTop/Bottom/Left/Right/None)`、`NoTooltip`、`Loading(bool)`、`Animate(bool)`、`Label`（数据标签）、`Format(chart.Thousands(0) / chart.Percent(1) / chart.Unit("°C", 1) / chart.Compact(c.Locale().Code))`、`Layout(...)`、`Key(k)`、`Style(func(*chart.Style))`。笛卡尔图另有 `X/Y/Series/YMin/YMax/NoZero`。
- **交互**：图例项可点击切换系列显隐（不会全部隐藏）；悬停出十字准线（柱状为整列背景高亮）和多系列合并 Tooltip，Tooltip 是 `ui.Overlay` 浮层，跟随指针、不被相邻卡片遮挡。测试里用 `ui.Driver` 的 `Move` 注入悬停，读 `View.Tooltip()`。
- **动画**：入场（折线左→右展开、柱/饼/雷达从 0 生长）和数据更新过渡（值从上次显示值缓动到新值）都走 `c.AnimFrom`，Driver 默认关闭动画时直接到终态。
- **样式与扩展**：`chart.StyleOf(th)` 由主题推导色板、轴线、网格、图例、Tooltip 令牌：Ant 用 G2 v5 分类色板与 G2 轴样式，Element 用 ECharts 5 色板与轴样式，暗色各自调整。新增设计语言用 `chart.RegisterStyle(match, fn)`，后注册者优先，不要在图表里按主题名分支；shadcn 已按此注册（`chart.ShadcnStyle`：`--chart-1..5` 色板、只画水平网格、无轴线、底部方块图例、圆角柱）。
- **空/加载**：无数据时显示 `ui.Empty(c).Simple()`（文案 `Locale.NoData`）；`Loading(true)` 在图上覆盖半透明遮罩和 `ui.Spin`。内置文案还用到 `Locale.T("Chart.total", "总计")`、`Locale.T("Chart.conversion", "转化率")`。
- **基础件**（自绘新图表时复用）：比例尺 `Linear/Log/TimeScale/Band/Point`，刻度 `NiceDomain/NiceTicks/TimeTicks`，格式化 `FormatNumber/FormatCompact`，堆叠 `Stack`，降采样 `LTTB`，坐标 `Cartesian/Polar`。
- Gallery：`examples/ui-gallery/page_chart_*.go`，每种图一页，`ChartDashboard` 是组合的分析仪表盘，均输出 Ant、`-Element`、`-Dark` 三套截图。

- **RTL**：图表默认在 RTL 作用域里绘图区仍从左到右（坐标轴、时间轴按惯例读向不变），图例、Tooltip、类目轴与饼图标签等外围元素跟随方向，数值保持 LTR。需要整个绘图区镜像时用 `Mirror()`（x 轴从右往左、y 轴在右、极坐标起始角镜像并逆时针扫掠），字形不镜像。

## 17. 无障碍（语义、键盘、动效）

宿主（`ui.AccessibilityHost`）或 `Driver.EnableAccessibility()` 请求时，Runner 每帧在布局后由节点树生成 `a11y.Tree`（`ui/a11y`），与上一帧比较后只上报变化；未请求时一律不构建，组件里的标注只是几次字段赋值。平台映射与上报协议见 `docs/accessibility.md`。

**每个可交互组件、以及有意义的展示组件都要标注**：

```go
n := c.add("rate", "")
n.setRole(a11y.RoleSlider).setState(a11y.Horizontal) // 角色 + 构建期已知的状态
n.setName(loc.A11y.Rating)                           // 可访问名：默认取 n.label；label 是内部键（"drag:"+key、日期串）时必须 setName
n.stateIf(sel, a11y.Selected)                        // Checked / Selected / Expandable|Expanded / Pressed / Current …
n.fill(w)                                            // 构建器实现 semantics(s *a11y.Node)：值、范围、描述、关系，在树构建时（链式修饰之后）调用
func (w *RateW) semantics(s *a11y.Node) { rangeSem(s, *w.val, 0, float64(w.count), 1); s.Value = w.loc.Rating(*w.val, w.count) }
```

- 构建器自身实现 `semantics` 不产生分配；逐行代码里要用闭包时写 `if c.a11yOn { n.fill(semFunc(func(s *a11y.Node){...})) }`。
- **没有可见文字的控件**（图标按钮、开关、滑块、颜色块、头像）提供 `.A11yLabel(s)` 修饰器（`n.setName`），文案走 `Locale`（通用无障碍文案在 `loc.A11y`）。放在 `FormItem` 里的输入类控件会自动取表单项标签为名、必填为 `Required`、校验错误为 `Invalid` + 描述，组件无需处理。
- 名称计算：显式名 > `n.label` > 表单项标签 > 占位符（输入类在 `semantics` 里兜底）> 子孙文字（按钮、选项、单元格等「由内容命名」的角色）。按钮/选项类角色的纯文字子节点会并入名称，不单独暴露。
- 纯装饰或被父节点语义覆盖的子树设 `n.aHidden = true`（组合框里的搜索编辑器、数字框的步进箭头、重复文字）；焦点落在其中时报告在父节点上。
- 角色与 WAI-ARIA 一致：容器 `listbox/menu/tablist/tree/radiogroup/table|grid/toolbar/navigation`，条目 `option/menuitem/tab/treeitem/radio/row/cell/columnheader`；集合条目的 `PosInSet/SetSize` 由树构建自动编号（虚拟列表要在 `semantics` 里填真实位置与 `RowCount`）。表头 `Sort`、树节点 `Level`、表格 `RowIndex/ColIndex`。
- 浮层：下拉列表的容器标 `listbox`/`menu`/`tree`/`grid`，触发器标 `combobox`（`Expandable|HasPopup`，`semantics` 里 `Expanded`、`Controls`、`ActiveDescendant`）。对话框类：面板 `RoleDialog` + `Modal`，每帧 `c.TrapFocus(panelID)`（Tab 限制在面板内、打开时焦点移入、关闭后归还触发元素、下层从语义树隐藏），`semantics` 里加 `a11y.Dismiss` 动作。
- 滚动：`ui.Scroll` 里的节点自动带 `ScrollIntoView` 动作，完全被裁剪/滚出的节点自动标 `Offscreen`。自己用 scroller 移动内容的容器（移动端 `ScrollView`）在外框上调 `.Clip().A11yScroller()`，构建时 `if t, ok := c.A11yScrollRequest(id); ok { … }` 用 `c.RectOf(t)` 把它滚进可见部分（扣掉安全区、键盘）。
- 消息/通知类：`RoleStatus`（polite）或 `RoleAlert`（assertive，错误/警告）；一次性播报用 `c.Announce(text, a11y.LivePolite)`。
- 动作：`Press/Focus` 映射为点击/聚焦，`Increment/Decrement` 映射为方向键，`Expand/Collapse` 映射为点击（展开箭头是另一个节点时设 `n.aTarget = arrow.id`），`SetValue` 由编辑器（`processEdit`）、`c.takeSetNumber(id)`/`c.takeSetValue(id)` 消费，`Dismiss` 映射为 Esc。
- 自定义组件（应用层）用公开 API：`box.Semantics(a11y.RoleButton, loc.X).A11yState(a11y.Pressed).A11y(func(s *a11y.Node){...})`，或 `c.Semantics(id, role, name, states...)`。

**键盘**：所有可交互组件都要 Tab 可达（`interact(n, true, …)`）并能用键盘操作。组合控件用「漫游 Tab 停靠」：`rovingTabStop(nodes, cur)` 只留当前项为 Tab 停靠点，`rovingStep(c, ids, enabled, axis)` 处理方向键 / Home / End，再 `navFocus(c, id)`（RadioGroup、Tabs 为范例）。由触发器持有焦点、自己处理按键的弹层（Select、DatePicker）里的条目算作可达；只能指针操作的辅助命中区（拖拽柄、滚动条）若有等价键盘操作，设 `n.aKbd = true`。浮层关闭时，若焦点在浮层里，会自动回到 `OverlayOptions.Anchor`。

**动效**：`RunnerOptions.ReduceMotion`（`Runner.SetReduceMotion`）开启时 `Anim/AnimFrom` 直接到终值；组件自己驱动的装饰性动效（闪烁、脉冲、轮播自动播放、跑马灯）用 `c.still()`（= 无动画或减少动态效果）判断后停住。

**测试**：`d.EnableAccessibility()` 后用 `d.A11yFind(role, name)`、`d.PerformAction(id, act, arg)`、`d.A11yFocused()` 断言；`d.Audit()` 报告缺名、无角色、不可键盘到达与主题对比度问题。`go run ./examples/ui-gallery -audit` 审计全部 gallery 页面（Ant、Element、shadcn 三种风格），可交互节点缺名必须为 0。

## 18. RTL（从右到左）

方向来自 `ConfigProvider(c).Direction(ui.RTL)`，或按 Locale 推断（ar/he/fa/ur…），读取用 `c.IsRTL()` / `c.Direction()`。设计与取舍见 docs/specs/rtl-and-component-tokens.md。

- **布局自动镜像**：RTL 容器在逻辑（LTR）求解后把子节点水平翻转，`Padding/Margin/Abs` 的 Left 即起始边、Right 即结束边，现有布局不用改。确需物理位置的节点加 `layout.Physical()`；整块必须保持 LTR 的子树（代码编辑器、时间列、数字输入、图表）包一层 `ConfigProvider(c).Direction(ui.LTR)`。
- **paint 里的不对称绘制要组件自己处理**：凡是 `r.X + 偏移`、`r.Right() - 偏移` 表示"起点/终点"的，改用 `s := c.inline(r)`：`s.X(off, w)`、`s.Rect(off, y, w, h)`、`s.Start()/End()`；指针换算比例用 `s.Frac(p)`（滑块、进度）。
- **图标**：方向性图标（左右箭头、折叠、退出…，见 `icon.Icon.Directional`）用 `c.paintIcon(l, ic, r, col, angle)` 或 `c.drawIconAt` 画，RTL 下自动镜像（先旋转再镜像，所以"右箭头转 90° 表示展开"不用改角度）。直接调 `icon.Draw` 不会镜像。
- **文字对齐**：`AlignStart/AlignEnd` 按行方向解析（`AlignLeft/AlignRight` 目前是它们的别名）；必须物理左右时用 `AlignPhysicalLeft/AlignPhysicalRight`。界面文字段落方向用 `c.textOpts`，用户输入内容保持 `DirAuto`，可用 `.TextDir()` 覆盖。
- **浮层**：`BottomStart/TopEnd…` 和新增的 `InlineStart*/InlineEnd*`、`ScreenStart/ScreenEnd…` 是逻辑位置；`Left/Right/RightStart/ScreenRight…` 是物理位置。子菜单用 `InlineEndTop`。
- **键盘**：表达"前一个/后一个"的左右键用 `c.inlineKey(k)`（`rovingStep` 已内置）；滑块类朝最大值一侧的方向键增加数值；树形 RTL 下 Left 展开、Right 折叠。
- **滚动**：横向偏移是逻辑值（0 = 起点，RTL 下在最右），竖向滚动条在 RTL 下画在左侧，`scroll.go` 已处理。
- **图表与手机端**：图表见 §16 的 RTL 一条；检查工具 `Driver.MirrorCheck` / gallery `-mirror-check` 同时报告布局违例和未镜像的方向性图标（`Driver.Audit` 的 `unmirrored-icon`）。
- **包外组件**（ui/mobile、ui/chart、第三方）：用导出的 `c.Inline(r)`（`InlineSpan`）、`c.InlineKey(k)`、`c.TextOpts(o)`、`c.PaintIcon(...)`，与内置组件用的 `inline/inlineKey/textOpts/paintIcon` 等价。
