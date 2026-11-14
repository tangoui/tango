# 第 2 批 · 组 G 标签、徽标与状态 对照记录

对照日期：2026-11-14。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：antd@6.7.0（`antd/package/es`）、element-plus@2.14.8（`ep/package`）、shadcn-ui@e62c9435（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）。下文源码路径省略这些前缀，`antd` 指 `antd/package/es`，`ep` 指 `ep/package`，`shadcn` 指 `new-york-v4/ui`。

本组测试：`ui/align_b2g_test.go`（行为）、`ui/theme/align_b2g_test.go`（令牌数值，断言值即源码值）；演示页 `examples/ui-gallery/page_b2g.go`（`TagAlign`、`CheckableTagAlign`、`BadgeAlign`、`AvatarAlign`、`StatisticAlign`、`EmptyAlign`、`SkeletonAlign`、`ProgressAlign`、`TooltipAlign`、`PopoverAlign`）。gallery 没有 Avatar 页，`AvatarAlign` 作为展示组的独立页注册（无 Parent）。

## Tag（含 shadcn Badge）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 变体 variant | antd `variant?: 'filled' \| 'solid' \| 'outlined'`，`bordered` 已弃用改 `variant="filled"`（antd/tag/index.d.ts:31,38）；默认 `filled`（antd/tag/index.js:57-59 `defaultVariant: 'filled'`，hooks/useColor.js） | 只有 `Borderless()` 与 Element 名 `Effect` | 已补齐（TestTagVariantDefault、TestTagVariantOutlinedSolid、TestTagVariantDefaultsKey）：`Variant(v)`、Defaults 键 `"variant"`；`Borderless()` 标 `Deprecated: use Variant("filled")`；**变更**：Ant 默认标签无边框 |
| 默认外观 | antd `defaultBg` = colorFillTertiary 叠在 colorBgContainer 上（#f5f5f5），`.ant-tag-filled` border transparent（antd/tag/style/index.js prepareComponentToken、genBaseStyle） | `FillQuaternary` 底 + `Border` 描边 | 已补齐（TestAlignB2GTagTokens）：令牌 `DefaultBg` 改为 Over(FillTertiary, BgContainer)；**变更**：Ant 默认标签底色 #fafafa → #f5f5f5、去掉描边 |
| 预设色 / 状态色 | antd 预设色 filled = lightColor + textColor，outlined 再加 lightBorderColor，solid = darkColor（antd/tag/style/presetCmp.js）；状态色 success/processing(Info)/error/warning（statusCmp.js） | 状态色已有；预设名（blue、magenta…）落到灰色 | 已补齐（TestTagPresetColor）：预设名走 Ant 色板（1/3/7，solid 用 6） |
| 自定义色 | antd 非预设色：filled/outlined 底色取 HSL 亮度 0.95、文字为该色（useColor.js） | 用 AntPalette 生成（底 1、边 3、字 7） | 暂不做（HSL 算法与色板生成结果相近，改动收益小；需要时可加 `theme.HSL` 工具） |
| solid 无色 | antd `colorBgSolid` = colorTextBase 100%（暗色 95%），文字 solidTextColor（themes/default/colors.js:34、dark/colors.js:36） | 用 TextSecondary | 已补齐（TestTagVariantOutlinedSolid、TestAlignB2GTagTokens）：令牌 `SolidBg`（Ant 黑 / 暗色 95% 白，Element/shadcn 主色） |
| 禁用 | antd 6 `disabled`，colorTextDisabled + colorBgContainerDisabled，outlined 描边 colorBorderDisabled，关闭按钮 tabIndex -1（antd/tag/index.js:63-99,130、style:139-166） | — | 已补齐（TestTagDisabledNoClose、TestTagDisabledA11yConfig）：`Disabled(on)`、Defaults 键 `"disabled"`、`ConfigProvider.ComponentDisabled`；禁用时 `Closed()` 为 false、不可聚焦、a11y Disabled |
| 关闭 | antd `closable`、`closeIcon`、`onClose`，Enter/Space 关闭（index.js:105-125） | `Closable()`、`Closed()`，Enter/Space/Delete/Backspace | 已补齐（TestTagCloseIcon）：`CloseIcon(ic)`（同时开启 closable） |
| 图标 | antd `icon` | `Icon(ic)` | 已对齐 |
| href / target | antd 渲染为 `<a>` | — | 不需要（自绘无锚点；可点击标签用 `Closable` 或外包 `Link`） |
| classNames / styles | antd root/icon/content/close | 组件令牌 | 不需要（用 `ConfigProvider.Components` 覆盖 `TagToken`） |
| 尺寸令牌 | antd tagPaddingHorizontal 8 − lineWidth = 7，行高 lineHeightSM × fontSizeSM = 20 + 边框 = 22（style/index.js prepareToken）；EP `size` large/default/small 32/24/20（ep/theme-chalk/src/tag.scss） | `Size(ControlSize)`，令牌 Height 22/24、PaddingInline 7/9 | 已对齐 |
| EP type / effect | EP `type` primary/success/info/warning/danger、`effect` dark/light/plain，默认 light（ep/es/components/tag/src/tag.d.ts） | `Color(status)`、`Effect` | 已对齐：`Effect("light")` = outlined，`"dark"` = solid，`"plain"` 保留；Element 默认 outlined 由令牌 `TagToken.Outlined` 给出 |
| EP round | EP `round` | — | 已补齐（TestTagRound）：`Round()` |
| EP hit | EP `hit`（描边高亮） | — | 等价写法：`Variant("outlined")` |
| EP disable-transitions | EP | 无过渡动画 | 不需要 |
| shadcn Badge 外观 | `rounded-full border border-transparent px-2 py-0.5 text-xs font-medium`（shadcn/badge.tsx:7） | rounded-md、px-2.5、font-semibold（v3 写法） | 已补齐（TestAlignB2GTagTokens）：shadcn 令牌 Radius 9999（绘制时夹到 h/2）、PaddingInline 7、FontWeight 500 |
| shadcn Badge 变体 | default/secondary/destructive/outline/ghost/link（badge.tsx:11-20） | default/secondary/destructive/outline | 已补齐（TestTagShadcnBadgeVariants）：`Color("ghost")`、`Color("link")`；入口仍是 `Tag`，不新建 Badge |
| shadcn 暗色 destructive | `bg-destructive text-white … dark:bg-destructive/60`（badge.tsx:15）；用户定：与 Button 一致 | 暗色仍用 destructive 实色，白字对比 2.89 | 已补齐（TestAlignB2GShadcnDestructiveDark）：令牌 `TagToken.DestructiveBg` = `ButtonDangerBg`（暗色 Over(Fade(destructive, .6), background)），白字 ≥ 4.5 |
| 明暗主题 | 令牌 | 全部颜色来自 TagToken / 色板 | 已对齐（三套 × 明暗出图检查） |

## CheckableTag / CheckableTagGroup

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 受控选中 | antd `checked`、`onChange`、`icon`（5.27+）、`disabled`（antd/tag/CheckableTag.d.ts） | more_checktag.go `CheckableTag(c, label, &v)`、`Icon`、`Disabled`、`Changed/Checked` | 已对齐 |
| 悬停 | antd 未选中悬停：colorPrimary 字 + colorFillSecondary 底（antd/tag/style/index.js `&-checkable`） | 只换字色 | 已补齐（TestCheckableTagHoverFill） |
| 按下 / 选中 / 禁用颜色 | antd active colorPrimaryActive、checked colorPrimary（hover colorPrimaryHover）、禁用 colorTextDisabled / colorBgContainerDisabled | 同 | 已对齐 |
| CheckableTagGroup | antd 6 `Tag.CheckableTagGroup`：`options`、`multiple`、`value/defaultValue`、`onChange`（单选取消为 null；`selected` 为 nil 时全部未选、点击不写，TestCheckableTagGroupNilSelection）、`disabled`，`gap: paddingXS`（antd/tag/CheckableTagGroup.d.ts，style `&-group`） | — | 已补齐（TestCheckableTagGroupSingle、TestCheckableTagGroupMultiple、TestCheckableTagGroupDisabled）：`CheckableTagGroup(c, &sel, opts)`、`Multiple(on)`、`Disabled(on)`、`Changed()`，Defaults 键 `"multiple"`；换行、间距 8；选项 `Option.Disabled` 生效 |
| EP el-check-tag | EP `checked`、`type`、`disabled`（ep/es/components/check-tag） | `Type(status)` | 已对齐 |
| a11y | 切换按钮 | role button + Toggleable/Pressed；组 role group | 已对齐 |

## Badge / Badge.Ribbon

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| count / overflowCount / showZero / dot | antd `count`、`overflowCount` 99、`showZero` false、`dot`（antd/badge/Badge.js:25-35） | `Count/Max/ShowZero/Dot`，Defaults 键 `overflowCount/showZero/dot` | 已对齐 |
| 负数 | antd 只在 0 且未 showZero 时隐藏（Badge.js:63），负数原样显示；EP 同（max < value 才折叠） | `<= 0` 一律隐藏 | 已补齐（TestBadgeOutOfRangeCount）；**变更**：负数计数改为显示 |
| status / text / color | antd `status`、`text`、`color`（预设色） | `Status(status, label)`、`Color` | 已对齐 |
| size | antd `size: small \| medium`（index.d.ts） | `Small()`、Defaults 键 `size` | 已对齐 |
| offset | antd `offset [x, y]`；EP `offset` | `Offset(x, y)` | 已对齐 |
| title | antd `title`（悬停/读屏文字） | — | 已补齐（TestBadgeHiddenTitle）：`Title(s)` 作为读屏文字（自绘无原生 title 提示） |
| 令牌 | antd indicatorHeight 20、SM 14、dotSize 6、multiple-words paddingInline = paddingXS 8（antd/badge/style/index.js:297-316,129-131）；EP size 18、padding 6（var.scss `$badge`） | 多位数内边距 Ant 6 / SM 4 | 已补齐（TestAlignB2GBadgeTokens）；**变更**：Ant 多位数气泡左右内边距 6 → 8（小号 4 → 8） |
| EP hidden | EP `hidden` | — | 已补齐（TestBadgeHiddenTitle）：`Hidden(on)` |
| EP show-zero 默认 true | EP `showZero` 默认 true（ep/es/components/badge/src/badge.mjs） | 默认 false | 等价写法：`ShowZero()` 或 `Defaults("Badge", Props{"showZero": true})`；行为以 antd 为准，不按主题分 |
| EP type / color / badge-style / badge-class | EP | `Color(status / hex)` | 等价写法：`Color("success")`、`Color("#hex")`；style/class 用组件令牌 |
| EP content 插槽 | EP | `Text(s)` | 暂不做（自定义节点内容；字符串已由 `Text` 覆盖） |
| Ribbon | antd `text`、`color`、`placement: start \| end`（antd/badge/Ribbon.d.ts） | `BadgeRibbon(c, label, color, placement, fn)`，逻辑方向 | 已对齐 |

## Avatar / AvatarGroup

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| size | antd `size` 数字 / small / medium / large / 响应式（antd/avatar/Avatar.d.ts） | `Size(px)`，Defaults 键 `size`（px 或 ControlSize） | 已补齐（TestAvatarSizeShape）：`Small()`、`Large()`；响应式尺寸暂不做（缺断点查询以外的尺寸映射，可用 `Size(px)` 按 `c.Size()` 自算） |
| shape | antd `shape: circle \| square` | `Square()`、Defaults 键 `shape` | 已补齐（TestAvatarSizeShape）：`Shape(s)` |
| gap | antd `gap` 4：文字按 (size − 2 × gap) / 文字宽 缩小（Avatar.js:31-56） | 固定字号 | 已补齐：`Gap(px)`、Defaults 键 `gap`；绘制时缩放（演示页 `AvatarAlign` 的 “Tango”）。el-avatar 不缩放文字：令牌 `AvatarToken.TextAutoScale`（Ant、shadcn 开，Element 关） |
| 字号 / 图标 / 圆角令牌 | antd textFontSize = fontSize 各尺寸相同；iconFontSize 18 / LG 24 / SM 14；方形圆角 borderRadius / LG / SM（antd/avatar/style/index.js:102-128） | Ant 文字 14/12/18、图标 = size × 0.56、圆角统一 Radius | 已补齐（TestAlignB2GAvatarTokens）：令牌 `IconFontSize*`、`Radius*`；**变更**：Ant 小号文字 12 → 14、大号 18 → 14（长文字按 gap 缩小）；图标 17.9/22.4/13.4 → 18/24/14；小 / 大方形圆角 6 → 4 / 8 |
| EP 尺寸 | EP 24/40/56、text-size 14、icon-size 18、border-radius-base（var.scss `$avatar`、`$avatar-size`） | 已有尺寸 | 已对齐（图标统一 18） |
| shadcn | `size-8`（sm size-6、lg size-10），AvatarFallback `bg-muted text-sm text-muted-foreground`（sm text-xs）（shadcn/avatar.tsx） | 文字 foreground | 已补齐（TestAlignB2GAvatarTokens）：shadcn `Color` = muted-foreground，小号 12 |
| src / srcSet / alt / crossOrigin / onError | antd | `Image(img)`、`A11yLabel` | 等价写法：图片由应用解码后传 `image.Image`；`alt` = `A11yLabel`；srcSet / crossOrigin / onError 不需要 |
| draggable | antd | — | 不需要 |
| Group max | antd `max {count, style, popover}`（`maxCount` 等已弃用，AvatarGroup.d.ts）；“+N” 用普通头像样式，悬停 Popover 列出被折叠头像（AvatarGroup.js:67-90） | `Max(n)`，“+N” 写死 Ant 文档示例的桃色 #fde3cf / #f56a00，无弹层 | 已补齐（TestAvatarGroupMaxPopover）：`MaxPopover(on, placement, trigger)`（默认开、top、hover）、`MoreBg` = max.style；**变更**：Ant/Element “+N” 头像改用默认头像颜色 |
| Group size / shape | antd 组上 `size`、`shape` 作用到全部头像 | — | 等价写法：`ConfigProvider(c).Defaults("Avatar", Props{"size": ui.Large, "shape": "square"}).Body(...)` 包住组 |
| EP collapse-avatars | EP `collapse-avatars`、`max-collapse-avatars`、`collapse-avatars-tooltip`（ep/es/components/avatar/src/avatar-group-props.mjs） | — | 等价写法：`Max(n)`；tooltip = `MaxPopover(true, "top", "hover")`（行为以 antd 为准默认开） |
| shadcn AvatarBadge / AvatarGroupCount | shadcn/avatar.tsx | — | 等价写法：`Badge(c).Dot().Body(avatar)`；GroupCount = “+N” 头像 |

## Statistic / Countdown / StatisticTimer

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 分隔符 | antd `decimalSeparator`、`groupSeparator`（antd/statistic/Number.js）；EP 同（ep/es/components/statistic/src/statistic.mjs） | 固定 `,` 和 `.` | 已补齐（TestStatisticSeparators）：`GroupSeparator(s)`、`DecimalSeparator(s)`，Defaults 键同名 |
| precision | antd / EP：小数 `padEnd(precision,'0').slice(0, precision)`，截断不四舍五入（antd/statistic/Number.js；ep statistic.vue_vue_type_script_setup_true_lang.mjs:18） | `FormatNumber` 四舍五入 | 已补齐（TestStatisticSeparators）；**变更**：Statistic 改为截断（1.239 精度 2 → 1.23）；负精度 = antd 未设 precision（原样小数）。`FormatNumber` 本身不变（表格、Pro 组件在用） |
| NaN / Inf | antd 非法数字原样显示 | strconv 输出 "NaN"/"+Inf" | 已补齐（TestStatisticOutOfRangeNaN）：显示 "NaN" / "Infinity" / "-Infinity" |
| formatter | antd / EP `formatter` | `Value(s)` | 已补齐：`Formatter(fn)` |
| prefix / suffix / title / loading | antd | `Prefix/Suffix/PrefixIcon`、`Loading(on)` | 已对齐 |
| valueStyle / valueRender | antd；EP `value-style` | `Color`、`ValueSize`、`Value` | 等价写法 |
| 令牌 | antd titleFontSize = fontSize 14、contentFontSize = fontSizeHeading3 24（antd/statistic/style/index.js:49-58） | comp_statistic.go 14 / 24 | 已对齐 |
| Statistic.Timer | antd `type: countdown \| countup`、`format`、`onFinish`、`onChange`（antd/statistic/Timer.d.ts） | data_skeleton.go `StatisticTimer`、`Countdown`、`Format/OnChange/Finished` | 已对齐 |
| EP el-countdown | EP `format` 默认 HH:mm:ss、`finish`、`change`、`value-style` | `Format` 默认 "HH:mm:ss"，Defaults 键 `format` | 已对齐 |

## Empty（含 shadcn Empty）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| image / description / children | antd `image`（默认 / PRESENTED_IMAGE_SIMPLE）、`description`、children（antd/empty/index.d.ts） | data_result.go `Image(fn)`、`Simple()`、`Description`、`Body(actions)` | 已对齐 |
| 尺寸与间距 | antd 图高 controlHeightLG × 2.5 = 100、simple 40；marginInline 8、图下 8、footer 16（antd/empty/style/index.js）；EP padding 40px 0、image-width 160、description-margin-top 20、bottom-margin-top 20（var.scss `$empty`） | 写在组件里按 Flavor 分支 | 已补齐（TestAlignB2GEmptyTokens）：令牌 `PaddingBlock/PaddingInline/DescriptionMarginTop/FooterMarginTop`，去掉 Flavor 分支；Ant / Element 外观不变 |
| Simple 外边距 | antd simple 图（`-normal`）marginBlock 32；Select 等的 `-small` marginBlock 8、图高 35（config-provider/defaultRenderEmpty.js） | Simple 统一 8 | 暂不做（改 Simple 会改 Select/Table/List 等别组组件的空态，需统一决定是否拆 normal/small 两档） |
| styles.image / imageStyle | antd（imageStyle 已弃用） | `ImageSize(px)` | 等价写法 |
| EP image / image-size | EP | `Image(fn)`、`ImageSize(px)` | 已对齐 |
| shadcn EmptyTitle | `text-lg font-medium tracking-tight`（shadcn/empty.tsx:64） | — | 已补齐（TestEmptyTitle）：`Title(s)`（a11y heading），令牌 `TitleFontSize/TitleFontWeight/TitleColor` |
| shadcn Empty 布局 | `gap-6 rounded-lg border-dashed p-6 md:p-12`；EmptyMedia icon `size-10 rounded-lg bg-muted`；header `gap-2`（empty.tsx:9,22,36） | 图标块已有；间距同 Ant | 已补齐（TestAlignB2GEmptyTokens）：shadcn 内边距 24、标题上 16、footer 24；`border-dashed` 无 `border` 宽度 → 不画边框；`md:p-12` 断点暂不做（缺按容器宽切换内边距） |
| shadcn EmptyMedia variant default | `bg-transparent`（自定义图） | `Image(fn)` | 等价写法 |

## Skeleton（含 SkeletonAvatar/Button/Input/Image）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| active / loading / round | antd（antd/skeleton/Skeleton.d.ts） | `Active/Loading/Round`，Defaults 键 `active/round` | 已对齐 |
| 默认行数 | antd 无头像且有标题 3 行，否则 2 行（Skeleton.js:49-60 getParagraphBasicProps） | 一律 3 行 | 已补齐（TestSkeletonDefaultRows）；**变更**：带头像或无标题时默认 2 行 |
| 宽度 | antd 标题 38%（有头像 50%，无段落 100%）；末行 61%（有头像且有标题时仅 ≥3 行由 CSS 给出）（Skeleton.js:36-55、style:262-264） | 标题 38/50%，末行总是 61% | 已补齐（TestSkeletonDefaultRows、演示页）：`TitleWidth(w)`、`ParagraphWidth(w...)`（≤1 为比例，>1 为 px）；默认宽度规则按 antd |
| avatar | antd `avatar {size, shape}`，默认 size large（controlHeightLG 40），有标题无段落时方形（Skeleton.js:23-34） | 32px；方形参数传错导致总是圆形 | 已补齐（TestSkeletonDefaultRows、TestSkeletonSquareAvatar、TestAlignB2GSkeletonProgressTokens）：令牌 `AvatarSize` 40、`AvatarSize(px)`；修正 `Avatar(true)` 方形；**变更**：头像 32 → 40。自动方形：等价写法 `Avatar(true)`（Go 的 bool 参数无法区分“未设置”） |
| 间距令牌 | antd 标题 + 段落 controlHeightSM 24；有头像 paragraphMarginTop 28、标题上 marginSM 12；行间 controlHeightXS 16（style/index.js:237-279, 318-334） | 24 与 28 写反；有头像标题上 4 | 已补齐（TestAlignB2GSkeletonProgressTokens）：令牌 `ParagraphMarginTop` 24、`ParagraphMarginTopAvatar` 28、新 `TitleMarginTopAvatar` 12；**变更**：Ant 骨架屏间距 |
| 无头像时标题上边距 | antd h3 未设 margin（依据源码 style，浏览器默认值未核对） | 4 | 暂不做（依据知识，未核对） |
| EP rows / animated / loading | EP `rows` 3、`animated`、`loading`（ep/es/components/skeleton/src/skeleton.d.ts） | `Rows/Active/Loading` | 已对齐 |
| EP count | EP `count` 1 | — | 已补齐（TestSkeletonDefaultRows）：`Count(n)` |
| EP throttle | EP `throttle`（显示延迟） | — | 暂不做（需要按 loading 变化计时的保留状态，下批与 Spin delay 一起做） |
| Skeleton.Node | antd `Skeleton.Node`（自定义内容的占位块） | — | 暂不做（`SkeletonImage` 覆盖常见用法；自定义内容块价值低） |
| Button / Input / Avatar / Image | antd 元素宽 controlHeight × 2、× 5、头像 controlHeight 各档 | `SkeletonButton/Input/Avatar/Image` | 已对齐 |
| shadcn | `animate-pulse rounded-md bg-accent`（shadcn/skeleton.tsx） | shadcn 走脉冲、bg = accent | 已对齐 |
| a11y | 装饰 | 组 role group + Busy；各条 aHidden | 已对齐 |

## Progress

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| type / steps / success / format / status | antd（antd/progress/progress.d.ts） | `Type/Circle/Dashboard/Steps/Success/Format/Status` | 已对齐 |
| percent 越界 | antd validProgress：非数或 <0 → 0，>100 → 100（utils.js:3-11） | NaN 未处理 | 已补齐（TestProgressOutOfRange） |
| gapPlacement | antd 6 `gapPlacement: top \| bottom \| start \| end`，`gapPosition` 已弃用；dashboard 默认 bottom，start/end 随方向（Circle.js:59-70） | 只有 dashboard 底部缺口 | 已补齐（TestProgressGapPlacement）：`GapPlacement(s)`；circle 设 `GapDegree` 后同样有缺口。Tango 无旧名 `GapPosition`，不新增 |
| percentPosition | antd 6 `percentPosition {align: start\|center\|end, type: inner\|outer}`，默认 end/outer；center+outer 文字在条下（gap 4），inner 白字 paddingInline 4（progress.js:43-45、Line.js、style:126-150） | 只有 `TextInside` | 已补齐（TestProgressPercentPosition）：`PercentPosition(align, typ)` |
| railColor | antd `trailColor` 已弃用改 `railColor`（progress.d.ts） | `TrailColor` | 已补齐（TestProgressPercentPosition）：`RailColor(col)`；`TrailColor` 标 Deprecated |
| size | antd 数字 / [w,h] / small / medium；circle small 60（utils.js getSize） | `Small()`、`Width(px)`、`StrokeWidth`；circle small 80 | 已补齐（TestAlignB2GSkeletonProgressTokens）；**变更**：小号环形 80 → 60 |
| steps {count, gap} | antd 6 | `Steps(n)`，gap 令牌 2 | 已补齐：`StepsGap(px)`（演示页） |
| strokeColor 渐变 / 数组 | antd 字符串、数组（steps 逐段）、渐变对象 | `Color`、`Gradient(a, b)` | 已对齐（两色渐变）；多段渐变对象与 steps 逐段色暂不做（缺多 stop 线性渐变 API 设计，可用 `ColorFunc` 近似） |
| strokeLinecap | antd round/butt/square | `Square()`、Defaults 键 `strokeLinecap` | 已对齐 |
| rounding | antd `rounding` | `Format(fn)` | 等价写法 |
| EP stroke-width / text-inside / indeterminate / duration / striped / striped-flow | EP（ep/es/components/progress） | `StrokeWidth/TextInside/Indeterminate/Striped(flow)` | 已对齐；`duration` 暂不做（动画周期固定 3s/1s） |
| EP color 函数 / 数组 | EP `color: string \| fn \| {color, percentage}[]` | `Color` | 已补齐（TestProgressPercentPosition）：`ColorFunc(fn)`；数组 = 等价写法（ColorFunc 内按阈值返回） |
| 令牌 | antd defaultColor colorInfo、remainingColor colorFillSecondary、line 8/6（style:271-279、utils.js）；EP stroke-width 6、width 126；shadcn `h-2 rounded-full bg-primary/20`（shadcn/progress.tsx） | comp_progress.go | 已对齐 |

## Tooltip

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| placement 12 向 | antd；EP `placement` | `Placement(s)`，left/right 为逻辑方向 | 已对齐 |
| 延迟 | antd mouseEnterDelay / mouseLeaveDelay 默认 0.1 s（antd/tooltip/index.js:76-77）；EP show-after 0、hide-after 200（ep/es/hooks/use-delayed-toggle/index.mjs:33-34）；shadcn delayDuration 0（shadcn/tooltip.tsx:8） | 默认 0 / 0 | 已补齐（TestTooltipDelayFromTheme、TestAlignB2GTooltipPopoverTokens）：令牌 `MouseEnterDelay/MouseLeaveDelay`（Ant 100/100 ms，Element 0/200 ms，shadcn 0/0）作默认，方法与 Defaults 键优先；**变更**：Ant 悬停提示延迟 0.1 s 出现 / 0.1 s 消失，Element 消失延迟 0.2 s。关闭动画（Driver 默认）时不等待 |
| open / onOpenChange | antd 受控 `open` | — | 已补齐（TestTooltipOpenControlled、TestTooltipControlledClickEsc）：`Open(&v)`、`OpenChanged()`；click 触发时以 `*v` 为准（应用关掉后一次点击即可再开）；Esc 也能关掉应用打开的提示 |
| EP disabled | EP `disabled` | — | 已补齐（TestTooltipOpenControlled、TestTooltipDisabledNoWrite）：`Disabled(on)`，禁用时不改写受控值、`OpenChanged()` 为 false |
| offset | EP `offset` 12；antd 箭头距离 sizePopupArrow / 2 = 8 | 固定 8 | 已补齐（TestAlignB2GTooltipPopoverTokens）：令牌 `Offset`（Ant 8、Element 12、shadcn 8）；**变更**：Element 提示与触发器距离 8 → 12 |
| arrow / pointAtCenter | antd `arrow {pointAtCenter}` 默认 false | `Arrow(on)`，箭头总指向锚点中心 | 等价写法（Tango 箭头始终 pointAtCenter，窄锚点下与 antd 一致） |
| autoAdjustOverflow | antd 默认 true | Overlay Flip | 已对齐 |
| color / effect | antd `color`；EP `effect: dark \| light` | `Color`、`Effect` | 已对齐 |
| trigger | antd hover/focus/click/contextMenu | hover/focus/click | 暂不做 contextMenu（右键打开的提示少见；需要时按 Dropdown 的右键路径补） |
| enterable | EP `enterable` 默认 true；antd 弹层可移入 | 气泡不接收指针（hit=false），移入即消失 | 暂不做（需决定提示气泡是否挡住下层点击，影响全部 Tooltip） |
| destroyOnHidden / fresh / persistent / teleported | antd / EP | 即时模式，不保留 | 不需要 |
| shadcn 外观 | `bg-foreground text-background px-3 py-1.5 text-xs rounded-md`、sideOffset 0、箭头 size-2.5（tooltip.tsx:43-50） | 令牌已有；`Offset` 8 | 已对齐（差异：shadcn sideOffset 0，但 Radix 箭头 size-2.5 旋转 45° 后约伸出 5px，气泡实际离触发器约 5px；Tango 箭头高 6，按 Ant 的 8 保留，避免箭头压住触发器） |
| a11y | role tooltip，触发器 describedBy | 同 | 已对齐 |

## Popover

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| title / content / placement / trigger / open | antd；EP | `Title/Content/Text/Placement/Trigger/Open/OpenChanged` | 已对齐 |
| 延迟 | antd mouseEnterDelay / mouseLeaveDelay 0.1 s（antd/popover/index.js:46-47）；EP show-after 0 / hide-after 200 | 进入 0、离开固定 100 ms | 已补齐（TestPopoverDelayAndWidth、TestAlignB2GTooltipPopoverTokens）：`MouseEnterDelay/MouseLeaveDelay`、Defaults 键同名、令牌默认（Ant 100/100，Element 0/200，shadcn 0/100）；**变更**：Ant 悬停弹出延迟 0.1 s，Element 离开延迟 0.2 s。离开延迟至少 100 ms，关闭动画时固定 100 ms（指针能越过箭头间隙进入面板，TestPopoverHoverGrace） |
| 宽度 | EP `width` 150（style width，ep/es/components/popover/src/popover.mjs:77）；shadcn `w-72`（popover.tsx:33） | 只有 Element 标题最小宽 150 | 已补齐（TestPopoverDelayAndWidth）：令牌 `Width`（Ant 0 自适应、Element 150、shadcn 288）；`Width(px)`、`NoPadding` 与 AvatarGroup 的 “+N” 弹层（自适应宽，TestAvatarGroupPopoverAutoWidth）不受影响；**变更**：Element 弹出框默认宽 150、shadcn 288 |
| arrow | antd `arrow`；EP `show-arrow` | `NoArrow()`、Defaults 键 `arrow` | 已补齐：`Arrow(on)` |
| shadcn | `rounded-md border p-4 shadow-md`、sideOffset 4、无箭头 | 令牌 + 无箭头距离 4 | 已对齐 |
| shadcn HoverCard 延迟 | Radix HoverCard openDelay 700 / closeDelay 300 | Popover hover | 暂不做（第 3 批 HoverCard 一起定） |
| a11y | hover = tooltip、click = dialog | 同 | 已对齐 |

## 第 2 批复审修正与已知偏离

- Tag plain：新增令牌 `TagToken.PlainColor` / `PlainBorderColor`，无色 `Effect("plain")` 标签读它们（Ant Text / Border，Element Primary.Base / Primary.Border，shadcn Text / Border）。去掉 c04a878 的对比度判断：那段把 Element 的 plain 状态色（#409eff 2.78、success 2.24 等）和 Ant success / warning、gold-7 / lime-7 / yellow-7 预设色的 plain 标签都改成了正文色。现在 Element 状态色 plain 恢复为 c04a878 之前的类型色 + light-5 边框（TestAlignB2FixTagPlainColors）。
- Tag `Color("<preset>-inverse")`：去掉后缀并强制 solid；优先级与 antd useColor.js:15,21-23 一致：`Variant` 方法 > inverse > `Defaults` 的 variant（TestAlignB2FixTagInverse）。
- Tag plain：无色 plain 标签不再读 `DefaultColor` / `BorderColor`，对它们的覆盖不再作用于 plain 标签（实验层令牌含义变化，已写 CHANGELOG）。
- Progress：shadcn 默认隐藏 line 百分比原先是 `data_progress.go` 里的 Flavor 分支，现改读令牌 `ProgressToken.ShowInfo`（shadcn false，其余 true）（TestAlignB2FixProgressShowInfoToken）。
- Tooltip offset：antd 是 `halfArrowWidth + marginXXS` = 16/2 + 4 = 12（tooltip/index.js:115-117、_util/placements.js:113,137、seed.js:56），不是 sizePopupArrow / 2 = 8。Ant `TooltipToken.Offset` 8 → 12。气泡与触发器的距离一致（12）。小偏离：Tango 箭头高 6（display.go tipArrow），antd 可见三角高约 8（roundedArrow.js unitWidth = sizePopupArrow/2），所以箭头尖到触发器 Tango 是 6、antd 约 4。shadcn 保持 8，未逐项核对 Radix。Popover 的偏移和箭头不在本次修正范围内，由主线程处理。
- 已知偏离：下列 Flavor 分支只决定外观（结构、绘制细节），不决定功能是否可用或 API 默认值，本批不做令牌化：`ui/display.go:430/513/559/579`（Tag 字号 / 尺寸、Element 状态色描边、shadcn Badge 变体、Element plain 描边）；`ui/data_badge.go:195,221`；`ui/more_checktag.go:91,94,119,166,169`。行号是复审时 c04a878 中的行号。

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| HSL 颜色工具 | Tag 自定义色（antd 非预设色 HSL 亮度 0.95） | `ui/theme` 的 RGB↔HSL 转换 | 暂不做，等用户决定 |
| 多 stop 线性渐变的组件 API | Progress 渐变对象 / steps 逐段色 | paint 已有 `LinearStops`，缺统一的渐变描述类型（如 `theme.Gradient`） | 暂不做 |
| 容器宽度断点 | shadcn Empty `md:p-12`、Avatar 响应式 size | 按容器 / 窗口宽切换数值的通用写法 | 暂不做 |
| 提示气泡可移入 | Tooltip enterable | overlay 层决定气泡是否参与命中且不挡下层点击 | 暂不做，等用户决定 |

## CHANGELOG 草稿

### 新增

- `ui`：Tag `Variant("filled"|"solid"|"outlined")`（Defaults 键 `variant`；`Borderless` 弃用）、`Disabled(on)`（Defaults 键 `disabled`）、`CloseIcon(ic)`、`Round()`；Ant 预设色名；shadcn Badge 变体 `ghost`、`link`。
- `ui`：`CheckableTagGroup(c, &sel, opts)`（antd 6 Tag.CheckableTagGroup）：`Multiple`、`Disabled`、`Changed`、`A11yLabel`，Defaults 键 `multiple`。
- `ui`：Badge `Hidden(on)`、`Title(s)`。
- `ui`：Avatar `Small()`、`Large()`、`Shape(s)`、`Gap(px)`（Defaults 键 `gap`）；AvatarGroup `MaxPopover(on, placement, trigger)`。
- `ui`：Statistic `GroupSeparator`、`DecimalSeparator`（同名 Defaults 键）、`Formatter(fn)`。
- `ui`：Empty `Title(s)`。
- `ui`：Skeleton `AvatarSize(px)`、`TitleWidth(w)`、`ParagraphWidth(w...)`、`Count(n)`。
- `ui`：Progress `GapPlacement(s)`、`PercentPosition(align, typ)`、`RailColor(col)`（`TrailColor` 弃用）、`ColorFunc(fn)`、`StepsGap(px)`。
- `ui`：Tooltip `Open(&v)`、`OpenChanged()`、`Disabled(on)`；Popover `MouseEnterDelay`、`MouseLeaveDelay`（同名 Defaults 键）、`Arrow(on)`。
- `ui/theme`：`TagToken.Outlined/DisabledBg/DisabledColor/DisabledBorderColor/SolidBg/DestructiveBg`；`AvatarToken.IconFontSize/IconFontSizeSM/IconFontSizeLG/Radius/RadiusSM/RadiusLG/TextAutoScale`；`EmptyToken.TitleFontSize/TitleFontWeight/TitleColor/PaddingBlock/PaddingInline/DescriptionMarginTop/FooterMarginTop`；`SkeletonToken.TitleMarginTopAvatar/AvatarSize`；`TooltipToken.Offset/MouseEnterDelay/MouseLeaveDelay`；`PopoverToken.Width/MouseEnterDelay/MouseLeaveDelay`。

### 变更

- Tag：`ConfigProvider.ComponentDisabled` 下不可关闭的标签也标 a11y Disabled。
- Tag：Ant 默认变体改为 filled（无边框），默认底色 #fafafa → #f5f5f5（antd 6.7 defaultBg = colorFillTertiary，useColor 默认 filled）；状态色 / 预设色标签同样去掉描边；`Effect("plain"|"outlined")` 的 outlined 保留浅色底（antd outlined），plain 仍为容器底（无色 plain 的文字和边框读 `TagToken.PlainColor/PlainBorderColor`）；无色 solid 标签 TextSecondary → colorBgSolid（亮色黑、暗色 95% 白）。Element 外观不变（令牌 `Outlined`）。
- Tag（shadcn）：Badge 改为 v4 外观 rounded-md → rounded-full、font-semibold → font-medium、左右内边距 9 → 7；暗色 destructive 背景 destructive → destructive/60（badge.tsx:15，用户定，与 Button 一致；白字对比 2.89 → ≥ 4.5）。
- CheckableTag：Ant 未选中悬停加 colorFillSecondary 底（antd `&-checkable`）。
- Badge：负数计数改为显示（原先 ≤ 0 一律隐藏；antd Badge.js:63、EP 同）；Ant 多位数气泡左右内边距 6 → 8，小号 4 → 8（antd paddingInline = paddingXS）；Element 6/4、shadcn 6/4 不变（shadcn 无对应组件）。
- Avatar：Ant 小号文字 12 → 14、大号 18 → 14，超宽文字按 gap 4 缩小；图标 size × 0.56 → 18/24/14；方形圆角小号 6 → 4、大号 6 → 8（antd prepareComponentToken）；shadcn 文字 foreground → muted-foreground、大号文字 18 → 14（text-sm）、小号 12 不变，图标 size × 0.56 → 18/24/14；Element 图标 size × 0.56 → 18（icon-size），文字不缩放。AvatarGroup：“+N” 头像不再用桃色 #fde3cf/#f56a00（那是 antd 文档示例的 maxStyle），改用默认头像颜色；悬停弹出被折叠的头像。
- Statistic：precision 改为截断（1.239 → "1.23"，antd / EP 均截断）；NaN / ±Inf 显示 "NaN" / "Infinity"。
- Empty：shadcn 内边距 0/8 → 24/24、描述上 8 → 16、操作区上 16 → 24（empty.tsx p-6 gap-6）；Ant / Element 不变；`Simple()` 的操作区上边距三套主题仍为 16。
- Skeleton：带头像或无标题时默认 2 行（原 3 行）；头像 32 → 40；Ant 标题到段落 28 → 24、带头像 24 → 28，带头像时标题上边距 4 → 12；带头像且有标题、少于 3 行时末行不再缩到 61%；`Avatar(true)` 的方形修正（原先总是圆形）；`NoTitle()` 时第一行段落上边距 16 → 0（antd 无标题时段落无上边距）。
- Progress：小号环形 80 → 60（antd getSize）；percent 为 NaN 时按 0。
- Tooltip：默认悬停延迟按主题（Ant 0.1 s / 0.1 s，Element 0 / 0.2 s，shadcn 0 / 0；原先 0 / 0）；Element 气泡距离 8 → 12。
- Popover：默认悬停延迟按主题（Ant 进入 0 → 0.1 s，Element 离开 0.1 → 0.2 s）；Element 默认宽 150、shadcn 默认宽 288（原先自适应）。

## 共享文件改动请求

1. `ui/testdata/basic_antd.png`、`basic_dark.png`（黄金图）：本组的 Tag 规范修正改了像素——`Tag(c, "默认")`、`Tag(c, "成功").Color("success")` 去掉描边、默认底色 #fafafa → #f5f5f5（暗色同理）。当前差异在 `TestGolden` 容差内（测试仍通过），建议主线程在合并后 `go test ./ui -run 'TestGolden$' -update` 重录 Ant 两张；`basic_element.png` 应不变（Element 标签仍为 outlined）。golden 页里的 Badge（单个数字 5）、方形 Avatar（中号，圆角仍 6）、Progress 60 不受影响。
2. `ui/AUTHORING.md` §14 总表：Tag 一行补 `Variant/Disabled/CloseIcon/Round`，注明 shadcn Badge 变体 `ghost/link`；CheckableTag 一行补 `CheckableTagGroup`（antd 6 Tag.CheckableTagGroup）；Empty 一行补 `Title`（shadcn EmptyTitle）；Avatar 一行补 `Small/Large/Shape/Gap` 与 AvatarGroup `MaxPopover`。§10 的 Tag 范例仍可用（字段名有增无改），可选地把 `Borderless` 换成 `Variant`。
3. 第 3 批（HoverCard）：shadcn Popover 的 hover 延迟暂保持 0 / 100 ms，HoverCard（Radix openDelay 700 / closeDelay 300）对齐时改 `PopoverToken` 的 shadcn 值。
4. `ui/theme/highcontrast.go`（如需）：新令牌 `TagToken.DestructiveBg/SolidBg`、`AvatarToken.Color` 未在高对比度主题里单独处理；`go test ./ui/theme` 现已通过，仅供复核。
5. Locale：本组未改 `ui/locale.go` / `ui/locale_packs.go`。
6. 归属外的已有调用：无 `Tag.Borderless` / `Progress.TrailColor` 调用需要迁移。
