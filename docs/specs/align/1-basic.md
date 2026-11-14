# 第 1 批 · 组 A 基础与布局 对照记录

对照日期：2026-11-14。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：antd@6.7.0（`antd/package/es`）、element-plus@2.14.8（`ep/package`）、shadcn-ui@e62c9435（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）。下文源码路径省略这些前缀，`antd` 指 `antd/package/es`，`ep` 指 `ep/package`，`shadcn` 指 `new-york-v4/ui`。

本组测试：`ui/align_b1a_test.go`（行为）、`ui/theme/align_b1a_test.go`（令牌数值，断言值即源码值）；演示页 `examples/ui-gallery/page_b1a.go`（`ButtonAlign`、`TypographyAlign`、`DividerAlign`、`SpaceAlign`、`SegmentedAlign`、`ToggleAlign`、`RateAlign`）。

## Button（含 IconButton、ButtonGroup）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 类型 / 颜色 / 变体 | antd `type` default/primary/dashed/link/text，`color` 16 种（default/primary/danger + 13 预设），`variant` outlined/dashed/solid/filled/text/link（antd/button/buttonHelpers.d.ts，2026-11-14）；EP `type` primary/success/warning/danger/info，`plain`/`text`/`link`/`dashed`（ep/es/components/button/src/button.d.ts）；shadcn variant default/destructive/outline/secondary/ghost/link（shadcn/button.tsx） | button.go `Primary/Dashed/Text/Link/Secondary/Danger/Color/Appearance/Success/Warning/Info/Plain` | 已对齐 |
| 图标位置 | antd `iconPlacement: 'start' \| 'end'`，`iconPosition` 已弃用（antd/button/Button.d.ts:27） | 只有 `IconRight` | 已补齐（TestButtonIconPlacementEnd、TestButtonIconPlacementDefaults）：`IconPlacement("start"\|"end")`，逻辑方向，Defaults 键 `"iconPlacement"` |
| 加载 | antd `loading: boolean \| {delay, icon}`（Button.d.ts）；EP `loading`、`loading-icon` | `Loading(bool)` | 已补齐（TestButtonLoadingDelay、TestButtonLoadingIconDisabled）：`LoadingIcon(ic)`、`LoadingDelay(d)`；延迟期间不进入加载态、可点击 |
| 形状 | antd `shape` default/circle/round/square（buttonHelpers.d.ts）；EP `round`/`circle` | `Round/Circle`；无文字只给图标即方形 | 已对齐（square = `Button(c, "").Icon(ic)`） |
| 尺寸 | antd size small/middle/large → controlHeightSM/ControlHeight/LG 24/32/40；EP large/default/small 40/32/24；shadcn sm h-8 / default h-9 / lg h-10 / icon size-9 / icon-sm size-8 / icon-lg size-10，另有 `xs` h-6 与 `icon-xs` size-6（shadcn/button.tsx） | `Size(ControlSize)`；三套主题的 ControlHeight* 令牌 | 已对齐（sm/default/lg/icon*）；`xs`、`icon-xs` **暂不做**（缺 `ControlSize` 的 XSmall 档与主题 `ControlHeightXS` 令牌，需主线程在 theme 核心加令牌，见共享文件请求） |
| 内边距与字号令牌 | antd `paddingInline` = paddingContentHorizontal − lineWidth = 15，SM 8 − 1 = 7，LG 15；`fontWeight` 400；`contentFontSize` = fontSize（antd/button/style/token.js prepareComponentToken）；shadcn px-4 / sm px-3 / lg px-6，有图标时 px-3 / px-2.5 / px-4，font-medium | theme/comp_button.go `PaddingInline* / FontWeight / ContentFontSize`：Ant 15/7/15/400，shadcn 16/12/24/500 | 已对齐 |
| 两字中文插空格 | antd `autoInsertSpace ?? contextAutoInsertSpace ?? true`，即默认 true（antd/button/Button.js:127）；EP `auto-insert-space` 默认 false（ep/es/components/button/src/button.d.ts） | 三套主题都默认关 | 已补齐（TestButtonAutoInsertSpaceThemeDefault、TestAlignB1AButtonTokens）：新令牌 `ButtonToken.AutoInsertSpace`（Ant true，Element/shadcn false）作为默认值；**变更**：Ant 主题两字中文按钮默认显示「确 定」 |
| 幽灵 / 块级 / 危险 | antd `ghost`、`block`、`danger` | `Ghost/Block/Danger` | 已对齐 |
| 禁用 | antd `disabled`；shadcn disabled:opacity-50 | `Disabled(bool)`、ConfigProvider.ComponentDisabled；`Clicked()` 禁用/加载时为 false | 已对齐（TestButtonLoadingIconDisabled） |
| 焦点与 a11y | shadcn focus-visible ring-[3px] ring-ring/50；antd outline 令牌 | `drawFocusRing`/`shadFocusRing`；role button，`Loading` 标 Busy | 已对齐 |
| EP `bg` / `dark` / `color` | EP `bg`（文字按钮常显背景）、`dark`、`color`（自定义色，ep button.d.ts） | `Appearance(ButtonFilled)`、`Color("#hex")`；暗色由主题算 | 等价写法：`bg` = `Appearance(ui.ButtonFilled)`；`color` = `Color("#rrggbb")`；`dark` 由 `ConfigProvider.Dark` 决定 |
| `href` / `htmlType` / `nativeType` / `tag` / `autofocus` | antd、EP HTML 属性 | — | 不需要（自绘无原生表单与锚点：`Clicked()` 里打开链接；`autofocus` 用 `c.SetFocus(b.ID())`） |
| classNames / styles 语义化 | antd root/icon/content | `Layout(...)`、组件令牌 | 不需要（Go 用令牌和 `ConfigProvider.Components` 覆盖） |
| ButtonGroup | antd `Button.Group` 已弃用，改用 `Space.Compact`（Button.d.ts）；EP `el-button-group` size/type | button_group.go `ButtonGroup().Size/Variant/Color/Appearance/Vertical` | 已对齐（Space.Compact 归组 B 的 `Compact`） |
| IconButton | 无官方同名（toolbar 关闭按钮类） | button.go `IconButton` | 不需要（Tango 内部辅助，非规范组件） |

## Link（Typography.Link / el-link）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 颜色类型 | antd Typography.Link 用 colorLink，`type` secondary/success/warning/danger；EP `type` primary/success/warning/info/danger/default（ep/es/components/link/src/link.d.ts） | text.go `LinkW.Type/Color/Secondary` | 已补齐（TestLinkTypeDefault）：`Type("default")` = EP default（常规文字色，悬停变链接色）；`"primary"`/`"info"` 走 `ColorOf` |
| 下划线 | EP `underline: 'always'\|'hover'\|'never'`，默认 hover（link.vue_vue_type_script_setup_true_lang.mjs:34-36）；antd linkDecoration / linkHoverDecoration 均 none（antd/typography/style）；shadcn 链接 underline | shadcn 固定 underline，Ant/Element 无 | 已补齐（TestLinkUnderlineMode、TestAlignB1ATypographyTokens）：`UnderlineMode(mode)`、Defaults 键 `"underline"`；默认由令牌 `LinkUnderline/LinkHoverUnderline` 给（Ant 0/0，Element 0/1，shadcn 1/1）；**变更**：Element 链接悬停出现下划线 |
| 字重 | EP `--el-link-font-weight` = font-weight-primary 500（ep/theme-chalk/src/common/var.scss `$link`）；shadcn font-medium；antd 继承 400 | shadcn 硬编码 Medium，其它 Regular | 已补齐（TestAlignB1ATypographyTokens）：令牌 `LinkFontWeight`（400/500/500），去掉组件里按 Flavor 分支；**变更**：Element 链接字重 400 → 500 |
| 图标 | EP `icon`，图标后文字 margin-left 5px（ep/theme-chalk/src/link.scss） | — | 已补齐（TestLinkIconDisabled）：`LinkW.Icon(ic)` |
| 默认颜色（EP） | EP 默认 `type="default"` 为 text-color-regular（var.scss `$link`） | Element 主题 `Link` 默认用链接色（= EP type="primary"） | 暂不做（Element 默认链接色改成常规文字色会连带 Anchor、Typography 复制图标、日期面板「今天」等共用 `Typography.LinkColor` 的组件，需主线程决定是否给 Link 单独令牌；当前写法 `Link(c,s).Type("default")`） |
| 禁用 | EP `disabled`，disabled-text-color placeholder，禁用时无悬停下划线；antd `disabled` | `Label.Disabled()`，`Clicked()` 返回 false，a11y Disabled | 已对齐（TestLinkIconDisabled、TestLinkDisabledNoHoverUnderline） |
| href / target | EP、antd 原生属性 | — | 不需要（`Clicked()` 后由应用打开） |
| ellipsis | antd Link `ellipsis: boolean` | `Ellipsis()`（继承 Label） | 已对齐 |

## Icon

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 尺寸 / 颜色 | EP `el-icon` `size`、`color`（ep/es/components/icon/src/icon.d.ts） | text.go `IconW.Size/Color` | 已对齐 |
| spin / rotate | antd `@ant-design/icons` 的 `spin`、`rotate`（依据知识，未核对：图标包不在 antd 包内） | `Spin()`、`Rotate(deg)`；减少动态效果时停转 | 已对齐 |
| twoToneColor / component（自定义 SVG） | antd icons（依据知识，未核对） | 图标集由主题替换（antd/element/lucide） | 不需要（双色图标与自定义 SVG 组件属于图标包；自定义图标走 `ui/icon` 内置集，AUTHORING §6） |
| a11y | 装饰性图标默认隐藏 | 默认装饰；`A11yLabel` 改为 role image | 已对齐 |

## Typography（Text / Title / Paragraph）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 修饰 | antd `code/delete/underline/mark/strong/italic/disabled/type`（antd/typography/Base/index.d.ts） | text.go `Code/Delete/Underline/Mark/Strong/Italic/Disabled/Type` | 已对齐 |
| keyboard | antd `keyboard`：margin 0 0.2em，padding 0.15em 0.4em 0.1em，font-size 90%，背景 rgba(150,150,150,.06)，边框 rgba(100,100,100,.2)、底边 2px，圆角 3（antd/typography/style/mixins.js:79-88） | shad_kbd.go `Kbd`（AUTHORING §14 已有入口） | 等价写法：`Kbd(c, "Ctrl")`（一个组件一个入口，不给 Text 加同义修饰）；Ant/Element 键帽数值落到 `Kbd` 组件令牌见共享文件请求 |
| 类型色 | antd `type` secondary/success/warning/danger；EP `el-text` `type` primary/success/info/warning/danger（ep/es/components/text/src/text.d.ts） | `Type(status)`（ColorOf 覆盖 primary/info） | 已对齐 |
| 尺寸 | EP `el-text` `size` large/default/small | `Size(px)` | 等价写法：`Size(th.FontSizeLG)` / 默认 / `Size(th.FontSizeSM)` |
| 截断 | antd `ellipsis`（rows、expandable、suffix、tooltip）；EP `truncated`、`line-clamp` | `Ellipsis()`、`Lines(n)`、data_typography.go `Expandable` | 已对齐（`truncated` = `Ellipsis()`，`line-clamp` = `Lines(n)`） |
| 复制 / 编辑 | antd `copyable`（text、icon、tooltips、onCopy）、`editable`（maxLength、triggerType、onChange…）（Base/index.d.ts） | data_typography.go `Copyable/CopyText/Copied/Editable/Editing` | 已对齐（`onCopy` = `Copied()`；`format: text/html` 不需要，剪贴板只放纯文本） |
| Title | antd `level` 1–5，字号 38/30/24/20/16（fontSizeHeading1–5，依据知识，未核对源码） | `Title(c, s, level)`，role heading + Level | 已对齐 |
| Paragraph | antd 段落 margin-bottom 1em | `Paragraph` 底边距 14 | 已对齐 |
| EP `tag` / antd `component` | 渲染的 HTML 元素 | — | 不需要（自绘无 DOM 元素；语义由 a11y 角色给出） |

## Divider

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 方向 | antd v6 `orientation: 'horizontal'\|'vertical'`、`vertical`，`type` 已弃用（antd/divider/index.d.ts）；EP `direction` | `Divider` 只有横向；`VDivider` 单独函数；`Orientation(s)` 被当作标题位置 | 已补齐（TestDividerVertical、TestDividerOrientationTitlePlacement）：`Vertical()`；`Orientation("horizontal"\|"vertical")` 设方向，旧值 left/right/center/start/end 仍设标题位置（同 antd 兼容逻辑 index.js:52-54），方法注释说明旧值仍接受、新代码用 TitlePlacement（不标 Deprecated：方向用法正是 v6 推荐写法） |
| 标题位置 | antd `titlePlacement` left/right/center/start/end，left/right 按 RTL 换成 end/start（antd/divider/index.js:53-62）；EP `content-position` left/center/right | 只有 `Orientation`，left 当逻辑起点 | 已补齐（TestDividerOrientationTitlePlacement）：`TitlePlacement(s)`、Defaults 键 `"titlePlacement"`；**变更**：`"left"`/`"right"` 改为物理方向（RTL 下 left 在左） |
| 线型 | antd `variant: 'dashed'\|'dotted'\|'solid'`、`dashed`（index.d.ts）；EP `border-style` | `Dashed()` | 已补齐（TestDividerSizeVariant）：`Variant(v)`，含 dotted；EP 的其它 CSS 线型（double 等）不需要 |
| 尺寸 | antd `size` small → marginBlock marginXS 8，middle → margin 16（antd/divider/style/index.js genSizeDividerStyle）；不设 size 时 24 / 带标题 16 | — | 已补齐（TestDividerSizeVariant、TestDividerDefaultsVerticalSizeString、TestAlignB1ADividerTokens）：`Size(ControlSize)`、Defaults 键 `"size"`（ControlSize 或 "small"/"middle"/"large"）与 `"vertical"`，ConfigProvider 尺寸生效；令牌 `MarginBlockSM/MD`（Element、shadcn 无 size，取 MarginBlock） |
| 标题字号 | antd with-text fontSize fontSizeLG (16)、fontWeight 500，plain 为 fontSize、400（style/index.js:62-70,135-139）；EP `__text` 14px、500、padding 0 20px（ep/theme-chalk/src/divider.scss） | 一律 14px，标题高 22 | 已补齐（TestDividerTitleFont、TestAlignB1ADividerTokens）：令牌 `TextFontSize`（Ant 16，Element 14，shadcn 14）；**变更**：Ant 带标题分割线 14px → 16px、高 22 → 24（行高取 Tango 行高表 16→24，antd 为 16×1.5714 = 25.14，差 1.1px，与全库文字行高一致） |
| 竖线高度 | antd vertical height 0.9em、marginInline marginXS (8)（style/index.js:44-54）；EP 1em、margin 0 8px；shadcn `data-[orientation=vertical]:h-full w-px`（shadcn/separator.tsx） | 三套主题都 14px | 已补齐（TestVDividerShadcnFullHeight、TestAlignB1ADividerTokens）：`VerticalHeight` Ant 12.6、Element 14、shadcn 0（撑满所在行）；**变更**：Ant 竖线 14 → 12.6px，shadcn 竖线随行高 |
| 颜色 / 边距令牌 | antd colorSplit、textPaddingInline 1em、orientationMargin 0.05、横线 marginLG 24、带标题 margin 16 | comp_divider.go | 已对齐 |
| orientationMargin（数值） | antd 已弃用，改 `styles.content.margin` | 令牌 `OrientationMargin` | 等价写法：`ConfigProvider.Components(theme.Components{Divider: …})` |
| plain | antd `plain` | `Plain()` | 已对齐 |
| a11y | role separator | role separator + Horizontal/Vertical | 已对齐 |

## Space / Flex

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 方向 | antd v6 `orientation`、`vertical`，`direction` 已弃用（antd/space/index.d.ts）；EP `direction` | `Vertical()` | 已补齐（TestSpaceOrientation）：`Orientation(o)`、Defaults 键 `"orientation"`；竖排时子项保持自然宽度、靠起始边（antd 竖排是 inline-flex column，未设 align 时 space-item 包裹层拉伸而其中的控件不拉伸，antd/space/style/index.js；Tango 用 AlignStart 得到同样外观） |
| 分隔符 | antd v6 `separator`，`split` 已弃用；EP `spacer` | `Split(fn)` | 已补齐（TestSpaceSeparator）：`Separator(fn)`；`Split` 加 `// Deprecated: use Separator`（ui/date_test.go、ui/nav_test.go、examples/ui-gallery/page_display.go 仍用旧名，归属外未改） |
| 间距 | antd size small/middle/large = paddingXS/padding/paddingLG 8/16/24，数字、[h, v]（antd/space/style）；EP small/default/large = 8/12/16（ep/es/components/space/src/use-space.mjs:5-8），默认 small | `Size(ControlSize)`（8/16/24）、`SizePx(h, v)`，默认 8 | 已对齐（Ant）；Element 的 default/large 为 12/16 **暂不做**（Space 没有组件令牌；需主线程在 theme 核心新增 `CompSpace`/`SpaceToken`，见共享文件请求；默认 small 两家相同） |
| 对齐 | antd `align` start/end/center/baseline/stretch，横向默认 center；EP `alignment` 默认 center | `Align(layout.Align)` | 已对齐 |
| 换行 | antd `wrap`；EP `wrap` | `Wrap()` | 已对齐 |
| EP fill / fill-ratio | EP `fill`、`fillRatio` 默认 100 | — | 等价写法：子项 `Layout(layout.Grow(1))` / 指定宽度 |
| Flex 方向 | antd Flex `vertical`、`orientation`（antd/flex/interface.d.ts） | `Vertical()` | 已补齐（TestSpaceOrientation）：`FlexW.Orientation(o)` |
| Flex gap | antd small/middle/large = flexGapSM/flexGap/flexGapLG，默认无间距（antd/flex/style/index.js） | `Gap(ControlSize)`、`GapPx`，默认 0 | 已对齐 |
| Flex justify / align / wrap / flex | antd `justify`、`align`、`wrap`、`flex`；竖排且未设 align 时 stretch（antd/flex/utils.js:16） | `Justify/Align/Wrap/Style` | 已对齐（`flex` = `Style(layout.Grow(n))`） |
| `component` | antd 渲染的元素 | — | 不需要 |

## Layout（Layout / Header / Sider / Content / Footer）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 结构 | antd Layout、`hasSider` 自动判断（antd/layout）；EP el-container `direction`，含 el-aside 时横排 | prolayout.go `Layout` 直接含 Sider 时横排 | 已对齐（`hasSider`、`direction` 自动判断） |
| Header 令牌 | antd headerHeight controlHeight×2 (64)、headerPadding 0 controlHeightLG×1.25 (50)、headerBg #001529（antd/layout/style/index.js:85-95）；EP el-header 60px、padding 0 20px | comp_layout.go | 已对齐（TestAlignB1ALayoutTokens） |
| Footer | antd footerPadding controlHeightSM 50（24 50）、footerBg colorBgLayout、无 text-align（style/index.js:58-64,96-97）；EP el-footer 60px、padding 0 20px | 内容水平居中 | 已补齐（TestLayoutFooterStart）：**变更**：Footer 内容改为从起始边排起（三套主题）；居中用 `Footer(c, fn, layout.Center())`（gallery Layout 页已显式加上，外观不变） |
| Sider 宽度 | antd `width` 默认 200、`collapsedWidth` 80（antd/layout/Sider.js:37-38）；EP el-aside 默认 300px | `Sider(c, width, …)` 宽度必填；`SiderCollapsedWidth` Ant 64 | Sider 宽度：不需要（Go 函数必填参数）；collapsedWidth 80 **暂不做**（ProLayout 调用同一 Sider，Pro 收起宽度是 64，需主线程同时让 ProLayout 显式 `CollapsedWidth(64)`——prolayout.go ProLayout 部分归属外，见共享文件请求） |
| 收起触发器 | antd triggerBg #002140、triggerHeight 48、`reverseArrow`、`trigger={null}`、`breakpoint`、`onBreakpoint`、`onCollapse`、`theme` light/dark（Sider.js、style/index.js:99-104） | `CollapsibleSider().Breakpoint/NoTrigger/ReverseArrow/Light/Changed/Broken`；TriggerBg 用的是菜单子菜单底色 #000c17 | 已补齐（TestAlignB1ALayoutTokens）：**变更**：Ant 亮色主题（深色侧栏）触发器底色 #000c17 → #002140；Ant 暗色主题不变（antd 两种算法都是字面量 #002140，但 Tango 的 Ant 暗色侧栏本身不是 antd 的 #001529，触发器仍随 SiderSubmenuBg 与侧栏配套）；`onBreakpoint` = `Broken()`，`onCollapse` = `Changed()` |
| zeroWidthTrigger | antd collapsedWidth=0 时显示贴在侧栏外的特殊触发器（zeroTriggerWidth/Height 40） | collapsedWidth 0 时不显示触发器 | 暂不做（触发器要画到被 `Clip()` 的 Sider 之外，需改成 Overlay 锚定实现；本批未做） |
| defaultCollapsed（非受控） | antd `defaultCollapsed` | `CollapsibleSider(c, w, nil)` 时用临时变量，不保留 | 等价写法：应用持有 `collapsed` 变量（AUTHORING §1「值由应用持有」） |
| Content | antd flex auto、minHeight 0；EP el-main padding 20px | Content grow、Element pad 20 | 已对齐 |
| headerColor | antd headerColor = colorText | 令牌 `HeaderColor` 未被 Header 使用 | 不需要（Header 内容自带颜色，与 antd 一样只是继承色） |
| a11y | — | Sider role navigation（名称来自 Locale） | 已对齐 |

## Grid（GridRow / Col）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 24 栅格 span / offset / push / pull / order / flex | antd ColProps（antd/grid/col.d.ts）；EP el-col span/offset/push/pull（ep/es/components/col/src/col.d.ts） | nav_grid.go `Col(c, span).Offset/Push/Pull/Order/Flex` | 已对齐（antd `flex` 字符串写法如 "100px" = `Style(layout.W(100))`；EP span 默认 24 = `Col(c, 24)`） |
| 响应式 | antd xs/sm/md/lg/xl/xxl，v6 新增 xxxl ≥ 1920（antd/_util/responsiveObserver.js:4，theme/util/alias.js:33）；EP xs–xl | `XS…XXL`、`BreakpointMins` | xs–xxl 已对齐；**xxxl 暂不做**（`Breakpoint` 枚举可追加 `XXXL`，但 ui/data_util.go、ui/fill_masonry.go 用 `[6]` 数组按断点下标取值，追加会越界，归属外；见共享文件请求） |
| gutter | antd `gutter` number / object / [h, v] 均可响应式（antd/grid/row.d.ts）；EP 只有 number | `Gutter(h, v)`、`GutterAt(bp, px)`（横向） | 已补齐（TestGridVGutterAt）：`VGutterAt(bp, px)` 竖向响应式 |
| justify / align / wrap | antd justify start/end/center/space-around/space-between/space-evenly、align top/middle/bottom/stretch、`wrap`；v5 起支持响应式对象 | `Justify/Align/NoWrap` | 已对齐（响应式 justify/align 等价写法：按 `ScreenBreakpoint(c)` 选值） |
| EP `tag` | 渲染元素 | — | 不需要 |

## Segmented

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 方向 | antd v6 `vertical`、`orientation`（antd/segmented/index.d.ts）；EP `direction`（ep/es/components/segmented/src/segmented.d.ts） | entry_segmented.go `Vertical()` | 已补齐（TestSegmentedUnknownValue、TestSegmentedOrientationHorizontalResets）：`Orientation(o)` |
| 自定义每项内容 | EP 默认插槽；antd `options[].label: ReactNode` | 只有 `Icons` | 已补齐（TestSegmentedItemRender、TestSegmentedItemRenderDisabled）：`ItemRender(func(o Option, selected bool))`，选项 Label 仍是无障碍名称 |
| 选项提示 | antd `options[].tooltip`（index.d.ts:25） | — | 已补齐（TestSegmentedTooltips）：`Tooltips(tips...)`（悬停该项显示，复用 Typography 的提示气泡） |
| 选项图标 / 禁用 | antd `icon`、`disabled`；EP `props.disabled` | `Icons`、`Option.Disabled` | 已对齐 |
| block / size / shape | antd `block`、`size`、`shape: 'default'\|'round'`；EP `block`、`size` | `Block/Size/Shape` | 已对齐 |
| 受控、越界值 | antd `value`/`onChange` | `*string` + `Changed()`；不在选项里的值不选中任何项，方向键从第一项开始 | 已对齐（TestSegmentedUnknownValue） |
| 键盘 / a11y | radiogroup、方向键 | 轨道持焦点，方向键换项，role radiogroup / radio | 已对齐 |
| 令牌 | antd trackPadding 2，item 内边距 controlPaddingHorizontal − lineWidth = 11，SM controlPaddingHorizontalSM − lineWidth = 7（antd/segmented/style/index.js:240-241）；shadcn TabsList p-[3px] | comp_segmented.go | 已对齐 |
| EP `props` / `name` / `id` / `validate-event` | EP | — | 不需要（Go 的 Option 有类型；表单校验由 FormItem 处理） |

## Toggle / ToggleGroup（shadcn）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| variant | shadcn toggle `variant: default\|outline`（shadcn/toggle.tsx） | shad_toggle.go `Outline()`，Defaults `"variant"` | 已补齐：`Variant("default"\|"outline")`（Toggle 与 ToggleGroup） |
| size 与内边距 | shadcn default h-9 min-w-9 px-2；sm h-8 min-w-8 px-1.5；lg h-10 min-w-10 px-2.5 | sm 也是 px-2 | 已补齐（TestToggleSizePadding）：**变更**：Small 内边距 8 → 6 |
| 组间距 spacing | shadcn toggle-group `spacing` 默认 0：项相连、`rounded-none` 只圆首尾、`shadow-none`、outline 时 `border-l-0`；>0 时 gap = spacing×4px（shadcn/toggle-group.tsx） | 默认（非 outline）组有 4px 间隔，各项全圆角 | 已补齐（TestToggleGroupSpacing）：`Spacing(n)`、Defaults 键 `"spacing"`；**变更**：默认组改为相连（间距 0、只圆外角），相连时 outline 项不再各自带阴影 |
| 组内项内边距 | shadcn 组内项 `px-3 min-w-0` | 同单个 Toggle | 已补齐（TestToggleSizePadding）：组内项 12px、无最小宽度；**变更** |
| type single / multiple | radix ToggleGroup `type` | `Single()`；默认多选 | 已对齐 |
| 禁用 | disabled:opacity-50 | `Disabled`、`Option.Disabled` | 已对齐（TestToggleGroupA11yDisabled） |
| 键盘 / a11y | radix：单选为 radiogroup、多选为 toolbar 内按钮（aria-pressed）、漫游焦点 | role radiogroup/toolbar，项 radio/button（Pressed），`rovingTabStop` | 已对齐（TestToggleGroupA11yDisabled） |
| orientation | radix ToggleGroup `orientation`（依据知识，未核对：radix 源码不在参考包里） | 只有横向 | 暂不做（shadcn v4 的 toggle-group.tsx 不暴露竖排样式；需要时再加） |

## Rate

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 数量 / 半星 / 清除 | antd `count` 5、`allowHalf`、`allowClear` 默认 true；EP `max` 5、`allow-half`、`clearable` 默认 false（ep/es/components/rate/src/rate.mjs:152） | entry_rate.go `Count/AllowHalf/AllowClear`（默认 true） | 已对齐（按 §7.1 行为以 antd 为准；Element 应用要 EP 默认用 `Defaults("Rate", Props{"allowClear": false})`） |
| 尺寸 | antd v6 `size`：starSize controlHeight×0.625 = 20，SM 15，LG 25（antd/rate/style/index.js:97-100）；EP large/default/small 高 40/32/24、图标 18/18/14（ep/theme-chalk/src/rate.scss:7-22） | 只有 `Size(px)` | 已补齐（TestRateSizes、TestAlignB1ARateTokens）：`Small()/Large()`、Defaults 键 `"size"`，令牌 `StarSizeSM/StarSizeLG/HeightSM/HeightLG` |
| 键盘 | antd `keyboard` 默认 true | 方向键 / Home / End 总是开启 | 已补齐（TestRateKeyboardOff）：`Keyboard(bool)`、Defaults 键 `"keyboard"` |
| 提示 | antd `tooltips` 每颗星一条 | — | 已补齐（TestRateTooltipsCharacterFunc）：`Tooltips(tips...)` |
| 自定义字符 | antd `character` 可为节点或 `(props) => node`（按 index） | `Character(string)` | 已补齐（TestRateTooltipsCharacterFunc）：`CharacterFunc(func(i int) string)` |
| EP 分段颜色 | EP `colors`（3 色）、`low-threshold` 2、`high-threshold` 4（rate.mjs:31-60）；colorMap 中 high 为 excluded，`v < high` 取第二色，v = high 起取第三色（rate.vue_vue_type_script_setup_true_lang.mjs:31-38,66-72） | 只有 `Color` | 已补齐（TestRateColorsThresholds）：`Colors(...)`、`Thresholds(low, high)` |
| EP 空星颜色 / 图标 | EP `void-color`、`disabled-void-color`、`icons`（按阈值 3 个）、`void-icon` | 令牌 `StarBg/StarBgDisabled` | 已补齐（TestRateColorsThresholds）：`VoidColor(col)`、`Icons(on, void)`；按阈值切换的 3 个图标：等价写法——按 `*val` 与阈值在应用里选 `Icons(...)` 的参数 |
| EP 文字 | EP `show-text`、`texts`、`show-score`、`score-template` "{value}"、`text-color` | `Texts/ShowScore`；令牌 `TextColor/ScoreColor` | 已补齐（TestRateScoreFormatOutOfRange）：`ScoreFormat(fn)`；`text-color` 等价写法：`ConfigProvider.Components(theme.Components{Rate: theme.RateToken{TextColor: …}})` |
| 受控 / 越界 | antd `value`；值大于 count 时全亮 | `*float64`；越界值照常绘制，按键时夹到 0..count | 已对齐（TestRateScoreFormatOutOfRange） |
| 禁用 | antd `disabled`、EP `disabled` | `Disabled`；只读，a11y ReadOnly | 已对齐（TestRateKeyboardOff） |
| 悬停回调 | antd `onHoverChange` | `Hovered()` | 已对齐 |
| 令牌 | antd starColor yellow6、starBg colorFillContent、starHoverScale 1.1、星间距 marginXS 8；EP 18px、间距 6 | comp_rate.go | 已对齐 |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 超小控件档 | Button（shadcn `xs`、`icon-xs` h-6） | `ControlSize` 追加 XSmall 档，主题核心新增 `ControlHeightXS`（及 `controlHeightOf` 等辅助） | 暂不做，等主线程决定 |
| xxxl 断点 | Grid、Masonry、数据组件按断点取列数 | `Breakpoint` 追加 `XXXL`（Ant 1920），`BreakpointMins` 与 data_util.go / fill_masonry.go 的 `[6]` 数组扩成 7 | 暂不做，归属外 |
| Space 组件令牌 | Space（Element small/default/large = 8/12/16） | theme 核心新增 `CompSpace` + `SpaceToken{GapSM, Gap, GapLG}` | 暂不做，等主线程 |
| 布尔组件令牌的 CSS 变量 | Button `AutoInsertSpace` | `TestComponentCSSVars` 跳过非 CSS 字段（主线程已定） | 合并前该测试失败（少 1 个变量） |

## CHANGELOG 草稿

### 新增

- `ui`：Button `IconPlacement("start"|"end")`（antd v6 iconPlacement，Defaults 键 `iconPlacement`）、`LoadingIcon(ic)`、`LoadingDelay(d)`。
- `ui`：Link `UnderlineMode("always"|"hover"|"never")`（Defaults 键 `underline`）、`Icon(ic)`、`Type("default")`。
- `ui`：Divider `Vertical()`、`TitlePlacement(s)`、`Variant("solid"|"dashed"|"dotted")`、`Size(ControlSize)`；Defaults 键 `titlePlacement`、`variant`、`size`、`vertical`、`orientation`（horizontal/vertical）。
- `ui`：Space `Orientation(o)`、`Separator(fn)`（`Split` 弃用）；Flex `Orientation(o)`；Defaults 键 Space `orientation`。
- `ui`：GridRow `VGutterAt(bp, px)`。
- `ui`：Segmented `ItemRender(fn)`（Element 默认插槽）、`Tooltips(...)`、`Orientation(o)`。
- `ui`：Toggle / ToggleGroup `Variant("default"|"outline")`；ToggleGroup `Spacing(n)`（Defaults 键 `spacing`）。
- `ui`：Rate `Small()/Large()`（Defaults 键 `size`）、`Keyboard(bool)`（Defaults 键 `keyboard`）、`Tooltips(...)`、`CharacterFunc(fn)`、`Colors(...)`、`Thresholds(low, high)`、`VoidColor(col)`、`Icons(on, void)`、`ScoreFormat(fn)`。
- `ui/theme`：`ButtonToken.AutoInsertSpace`；`DividerToken.TextFontSize/MarginBlockSM/MarginBlockMD`；`TypographyToken.LinkFontWeight/LinkUnderline/LinkHoverUnderline`；`RateToken.StarSizeSM/StarSizeLG/HeightSM/HeightLG`。

### 变更

- Button：Ant 主题两字中文按钮默认插入空格（「确定」→「确 定」），新令牌 `Button.AutoInsertSpace` Ant true / Element、shadcn false；依据 antd Button.js:127 默认 true、EP `auto-insert-space` 默认 false。关闭用 `Defaults("Button", Props{"autoInsertSpace": false})` 或 `AutoInsertSpace(false)`（布尔令牌不能经 Components 覆盖为 false）；优先级：方法 > Defaults > 令牌（TestButtonAutoInsertSpacePrecedence）。
- Divider：`Orientation` 的含义改为方向（horizontal/vertical，antd v6），旧的标题位置值仍可用；`"left"`/`"right"` 标题位置从逻辑方向改为物理方向（RTL 下 left 在左，antd v6 index.js:53-62）。
- Divider：Ant 带标题分割线字号 14 → 16（fontSizeLG）、高 22 → 24；Ant 竖线高度 14 → 12.6（0.9em）；shadcn 竖线 14 → 撑满所在行（separator.tsx h-full）；`VDivider` 同步。
- Link：Element 主题链接字重 400 → 500、悬停显示下划线（`--el-link-font-weight`、underline 默认 hover）；shadcn 下划线与字重改由令牌给出（外观不变）。
- Layout：Footer 内容不再居中，从起始边排起（antd/EP 均无 text-align）；Ant 亮色主题深色 Sider 的触发器底色 #000c17 → #002140（antd triggerBg）；Ant 暗色主题不变。
- Rate：Ant 星星尺寸改由 ControlHeight×0.625 推导（默认仍 20，紧凑主题变小）；shadcn 保持 20/15/25。
- Toggle：Small 内边距 8 → 6（shadcn px-1.5）。ToggleGroup：默认间距 4 → 0，项相连只圆外角、相连时不再各带阴影；组内项内边距 12、无最小宽度（shadcn toggle-group spacing=0、px-3）。

## 共享文件改动请求

1. `ui/theme/components_test.go` `TestComponentCSSVars`（主线程已定此方案）：计数时只算 `cssValue` 能表示的字段。具体改法：在计数循环里对每个字段 `ft.Field(j)` 用零值试探 `if _, ok := cssValue(reflect.New(ft.Field(j).Type).Elem()); !ok && ft.Field(j).Type.Kind() != reflect.Array { n-- }`（Alias 字段不受影响）。原因：`ButtonToken.AutoInsertSpace bool` 不是 CSS 变量；当前报 2395/2396 类的差 1。
2. `ui/defaults_more_test.go`：
   - `TestButtonAutoInsertSpaceRules`（208 行起）：`plain = Button(c, "提交")` 改为 `plain = Button(c, "提交").AutoInsertSpace(false)`；want 列表最后一项对应的 `Button(c, "默认").shownLabel()` 改为 `Button(c, "默认").AutoInsertSpace(false).shownLabel()`（期望仍为 "默认"）。或者整测试加 `Theme: theme.ElementPlus()` 到 DriverOptions。
   - `TestDefaultsWiring` 的 `Button autoInsertSpace` 用例：Ant 下内置默认已是「确 定」。改为设 `Props{"autoInsertSpace": false}`、期望「确定」，或该用例换 Element 主题。优先级已由 `TestButtonAutoInsertSpacePrecedence`（ui/align_b1a_test.go）覆盖。
3. `ui/testdata/basic_antd.png`、`basic_dark.png`：`go test ./ui -run Golden -update` 重新生成（`basic_element.png` 不变）。golden 页还含组 B 的 Input/Checkbox/Switch，请先确认差异只来自两字按钮（默认 / 虚线 / 危险 / 禁用）。
4. `ui/shad_kbd.go`（主线程独占外的其它文件，非本组）与 `ui/theme/components.go`：给 `Kbd` 加组件令牌（新 `CompKbd` + `KbdToken{Bg, BorderColor, BorderBottomWidth, Radius, FontScale, PaddingInline, PaddingBlock, MarginInline}`），去掉 `shad_kbd.go` 里的 Flavor 分支。Ant 取 antd `es/typography/style/mixins.js:79-88`：margin 0 0.2em、paddingInline 0.4em、paddingBlock 0.15em 0.1em、fontSize 90%、fontFamily code、背景 rgba(150,150,150,0.06)、边框 1px rgba(100,100,100,0.2)、底边 2px、圆角 3；shadcn 取 `new-york-v4/ui/kbd.tsx`（bg-muted、h-5、rounded-sm、px-1、text-xs font-medium）；Element 无 kbd，建议同 Ant。这样 antd `<Text keyboard>` 的等价写法 `Kbd(c, …)` 在 Ant 主题下外观对齐。
5. `ui/theme/theme.go:307` `SiderSubmenuBg` 注释：「remains the source of the derived default」对 `Components.Layout.TriggerBg` 现在只在 Element、shadcn 与 Ant 暗色成立；Ant 亮色用 antd 字面量 #002140。建议注释改为「…the derived default of Components.Menu.DarkSubMenuItemBg (and of Layout.TriggerBg except the light Ant theme, which uses antd's #002140)」。
6. `ui/theme/theme.go`（或 components.go）：如要做 shadcn `xs` 按钮，新增 `ControlHeightXS` 等令牌（见基础能力缺口）。
7. `ui/theme/components.go`：如要做 Element 的 Space 间距，新增 `CompSpace` 与 `SpaceToken`（GapSM 8 / Gap 16 / GapLG 24；Element 8 / 12 / 16），之后 Space 改读令牌（nav_space.go 归组 A，可下一批做）。
8. `ui/prolayout.go` ProLayout 部分（830、985 行附近，归属外）：如接受 Ant `Layout.SiderCollapsedWidth` 64 → 80（antd Sider collapsedWidth 80），ProLayout 需显式 `CollapsedWidth(64)`；本组未改该令牌。
9. `ui/data_util.go`、`ui/fill_masonry.go`、`ui/nav_grid.go`：如要加 antd v6 `xxxl` 断点，三处断点数组从 6 扩到 7。
10. 旧名 `SpaceW.Split`（现标 Deprecated）的调用点：`ui/date_test.go:77`、`ui/nav_test.go:271`、`examples/ui-gallery/page_display.go:94`，可改为 `Separator`（不改也能编译）。
11. AUTHORING §14：Divider 一行可补「`Divider().Vertical()` 等同 VDivider」；Toggle 一行可补 `Spacing`；Kbd 一行可补「antd Typography keyboard」。非必须。
12. Locale：本组未改 `ui/locale.go` / `ui/locale_packs.go`。
