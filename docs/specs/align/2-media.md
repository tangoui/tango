# 第 2 批 · 组 H：标签页、时间线与媒体

核对日期：2026-11-14。参考源码（`/tmp/tango-ref`）：

- antd@6.7.0：`antd/package/es/{tabs,timeline,steps/style,image,carousel,calendar,qr-code,watermark,masonry}`（`index.d.ts` / `*.d.ts`、`index.js`、`style/index.js` 的 `prepareComponentToken`）。
- antd 依赖：`@rc-component/tabs@2.0.0`（`lib/Tabs.js`、`lib/interface.d.ts`、`lib/hooks/useIndicator.js`、`lib/TabNavList/index.js`）、`@rc-component/image@1.10.0`（`lib/Preview/{index,Footer,PrevNext}.js`、`lib/hooks/useMouseEvent.js`、`lib/previewConfig.js`）、`@rc-component/qrcode@2.0.0`（`lib/interface.d.ts`），分别 `npm pack` 到 `/tmp/tango-ref/rc-{tabs,image,qrcode}/package`（版本按 antd `package.json` 的 `~2.0.0` / `~1.10.0` / `~2.0.0` 解析）。
- element-plus@2.14.8：`ep/package/es/components/{tabs,timeline,image-viewer,carousel,calendar}`、`ep/package/theme-chalk/src/{image-viewer,carousel}.scss`。
- shadcn/ui v4（new-york-v4）：`registry/new-york-v4/ui/{tabs,carousel,calendar}.tsx`（shadcn 没有 Timeline、Image、QRCode、Watermark、Masonry）。

下文“antd”指 antd@6.7.0（含上面的 rc 包），“EP”指 element-plus@2.14.8。Tango 现状写 `文件:符号`；测试在 `ui/align_b2h_test.go`、`ui/theme/align_b2h_test.go`，演示在 `examples/ui-gallery/page_b2h.go`（`TabsAlign`、`TimelineAlign`、`ImageAlign`、`CarouselAlign`、`CalendarAlign`、`QRCodeAlign`、`WatermarkAlign`、`MasonryAlign`）。

## Tabs

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 类型 type | antd `line \| card \| editable-card`；EP `"" \| card \| border-card` | nav.go:`Card`、`Closable`、`BorderCard`，Defaults `"type"` | 已对齐 |
| tabPlacement | antd `tabPlacement: top\|end\|bottom\|start`（index.d.ts，`tabPosition` 弃用）；EP `tab-position` | `Placement(Placement)`（逻辑方向） | 已补齐：`TabPlacement(string)`，Defaults `"tabPlacement"`（旧键 `"placement"`/`"tabPosition"` 仍认）（TestTabsTabPlacementStart）。`Placement` 本身就是 tabPlacement 的 Go 写法，不弃用 |
| indicator {size, align} | antd `indicator`（`indicatorSize` 弃用）；rc-tabs useIndicator align 默认 center | — | 已补齐：`Indicator(size, align)`（TestTabsIndicator）；函数形式 `size: (origin)=>n` 暂不做（先无需求） |
| destroyOnHidden / forceRender | antd `destroyOnHidden`、`items[].forceRender` | `Panel` 只构建当前项，其它项的 `State` 约 300 帧后回收 | 等价写法：即时模式下“隐藏面板”就是不构建；需要保留输入时把状态放在应用里 |
| size | antd `size`；`cardHeightSM/LG`、`horizontalItemPaddingSM/LG`、`cardPaddingSM` | 大尺寸只改字号，卡片高度固定 40 | 已补齐（规范修正）：令牌 `CardHeightSM 32 / LG 48`、`HorizontalItemPaddingSM 8 / LG 16`、`CardPaddingH 16 / SM 8`（TestTabsSizes、TestAlignB2HTabsTokens） |
| centered / tabBarExtraContent / tabBarStyle | antd | `Centered`、`Extra`/`ExtraLeft`、`TabBarStyle` | 已对齐 |
| tabBarGutter | antd `tabBarGutter`（TabNavList marginInlineStart / marginTop） | 只有主题值 | 已补齐：`TabBarGutter(px)`，Defaults `"tabBarGutter"`（TestTabsStretchGutter） |
| addIcon / removeIcon / more.icon | antd | 固定图标 | 已补齐：`AddIcon`、`RemoveIcon`、`MoreIcon` |
| hideAdd / onEdit | antd editable-card 默认显示 + | `OnAdd` 才显示 +；`Closed`/`Added` | 等价写法：不调 `OnAdd` = hideAdd |
| more.trigger / popupRender | antd MoreProps | 点击下拉 | 暂不做（先无需求） |
| items[].icon/disabled/closable | antd Tab | `TabItem.Icon/Disabled/NoClose` | 已对齐（`NoClose` 是 closable 的反义） |
| onTabClick / tab-click | antd / EP | 只有 `Changed` | 已补齐：`TabClicked()`（点已选中项也报告）（TestTabsBeforeLeave） |
| activeKey 不在 items | rc-tabs Tabs.js：退到上次索引（夹到范围） | 没有项被选中 | 已补齐：回落到上次索引、跳过禁用项，不报 `Changed`；`active` 可传 nil；空 items 不 panic（TestTabsEmptyUnknownKey） |
| EP stretch | EP `stretch` | — | 已补齐：`Stretch()`，Defaults `"stretch"`（TestTabsStretchGutter） |
| EP before-leave | EP `beforeLeave(new, old)` | — | 已补齐：`BeforeLeave(fn)`，点击、方向键、更多菜单、拖拽激活都经过它（TestTabsBeforeLeave、TestTabsDragBeforeLeave）；复审修正：拖拽原先直接写 active 绕过钩子 |
| EP addable / editable / closable | EP | `OnAdd`、`Closable` | 已对齐 |
| 键盘 | rc-tabs onKeyDown：←/→/↑/↓、Home、End、Delete/Backspace 关闭 | `rovingStep`、Delete 关闭 | 已对齐（TestTabsA11yKeys） |
| 禁用 | antd 无整体 disabled；ComponentDisabled 作用域 | 作用域下节点禁用 | 已对齐：标签、关闭、新增均不响应（TestTabsDisabledScope） |
| a11y | tab / tablist / tabpanel | `tabSem`、`Panel` | 已对齐（TestTabsA11yKeys） |
| 令牌 | antd prepareComponentToken：cardHeight 40、titleFontSize 14/16、inkBarColor primary、horizontalItemGutter 32、horizontalItemPadding 12、cardGutter 2、verticalItemPadding 8 24、verticalItemMargin 16 | theme/comp_tabs.go | 已对齐（新增尺寸令牌见上） |
| shadcn TabsList variant | tabs.tsx `variant: default \| line`；line：`bg-transparent gap-1 rounded-none`，选中项 `after:h-0.5 bg-foreground bottom-[-5px]`（竖向 `-right-1 w-0.5`） | 只有分段外观 | 已补齐：`Variant("line")`，Defaults `"variant"`；Ant / Element 本来就是线条（TestTabsShadcnLine） |
| shadcn 暗色选中项 | tabs.tsx `dark:data-[state=active]:border-input dark:bg-input/30` | 用 `SegmentedSelectedBg` | 已补齐（规范修正）：令牌 `ItemSelectedBg`、`ItemSelectedBorder`；暗色 = `Over(Fade(input,.3), muted)` + `input` 描边（TestAlignB2HTabsTokens） |
| shadcn orientation / 尺寸 | `h-9 rounded-lg p-[3px] bg-muted`，trigger `h-[calc(100%-1px)] px-2 py-1 font-medium` | 已有 | 已对齐 |
| 拖拽排序 / 撕出、Dock | Tango 扩展 | nav_tabs_drag.go；Dock 改用私有构造 nav.go:`dockTabs`（dock.go 只改这一处） | 已对齐：Dock 标签条不读 `Defaults("Tabs", …)`、不做未知 key 回落，卡片保持 CardHeight + 16px 内边距与旧的 shadcn 选中底色——与 HEAD 截图逐字节相同（Dock、DockDrop × 三套主题 × 明暗）（TestTabsDockLegacy） |
| classNames / styles / renderTabBar / getPopupContainer | antd | — | 不需要（自绘，外观走令牌；`TabBarStyle` 覆盖布局） |

## Timeline

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| mode | antd `start \| alternate \| end`（left/right 弃用，Timeline.js mergedMode 默认 start）；EP 另有 `alternate-reverse` | data_timeline.go:`Mode`，默认 `"left"` | 已补齐：默认值改为 `"start"`（行为不变），新增 `"alternate-reverse"`（TestTimelineModeStartEnd、TestTimelineAlternateReverse） |
| orientation | antd `orientation: vertical \| horizontal` | `Horizontal()` | 已补齐：`Orientation(string)`，Defaults `"orientation"`（TestTimelineLoadingOrientation） |
| variant | antd `variant: filled \| outlined`，默认 outlined（useVariant） | Ant 空心圈，Element/shadcn 实心 | 已补齐：`Variant(string)`，Defaults `"variant"`；空串 = 主题默认，由令牌 `TimelineToken.DotOutlined` 决定（Ant true），组件里不再按 Flavor 判断 |
| titleSpan | antd `titleSpan`（非 alternate 时生效，默认 12/24） | 标题列与内容列等宽 | 已补齐：`TitleSpan(span)`（TestTimelineTitleSpan）；百分比字符串写法等价（传 24 分之几） |
| items[].title / content | antd v6（label / children 弃用） | `Label` / `Content` | 已补齐：`TimelineItem.Title`，`Label` 标 Deprecated（TestTimelineTitleSpan） |
| items[].placement | antd v6（position 弃用） | — | 已补齐：`TimelineItem.Placement`（TestTimelineModeStartEnd） |
| items[].loading | antd v6：LoadingOutlined 图标、status process，其 rail 为 dotted | — | 已补齐：`TimelineItem.Loading`（Busy 状态；进入该项的一段轴线为点线，`Reverse` 时为离开该项的一段，与 antd 一致）（TestTimelineLoadingOrientation、TestTimelinePendingTailDashed） |
| items[].icon / color | antd `icon`、`color: blue\|red\|green\|gray`（gray = colorTextDisabled） | `DotIcon`/`Dot`、`Color`（gray 用 TextQuaternary） | 已对齐（gray 改读 `TextDisabled`，Ant 下数值相同） |
| pending / reverse | antd（pending 弃用但保留）、EP `reverse` | `Pending`、`Reverse` | 已对齐；按 rc-steps（Step.js:147 `railFollowPrevStatus ? status : nextStatus`，Timeline.js:104 `railFollowPrevStatus: reverse`），不 reverse 时进入忙碌项（Pending / Loading）的一段为点线，reverse 时离开忙碌项的一段为点线；横向同理。Element 不画点线。按主题由令牌 `TimelineToken.BusyTailDashed` 决定（TestTimelinePendingTailDashed） |
| EP timestamp / placement / hide-timestamp | EP timeline-item | `Time`、`TimeTop`；不设 `Time` 即隐藏 | 已对齐 / 等价写法 |
| EP center | EP `center` | — | 已补齐：`TimelineItem.Center`；轴线改为每项各画上下两段，居中点也能连上 |
| EP type / color / size / icon / hollow | EP | `Color`、`Large`、`DotIcon`、`Hollow` | 已对齐 |
| 令牌 | antd prepareComponentToken：tailWidth / dotBorderWidth = lineWidthBold 2、itemPaddingBottom 20；Steps dotCurrentSize 10 | `DotBorderWidth 3` | 已补齐（规范修正）：Ant `DotBorderWidth 3 → 2`（TestAlignB2HTimelineTokens） |
| 空数据 | — | — | 已对齐：nil items 不 panic（TestTimelineLoadingOrientation） |
| classNames / styles | antd | — | 不需要 |

## Image

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| preview 开关 / PreviewGroup | antd `preview`、`Image.PreviewGroup {items, current}` | `Preview`、`PreviewGroup(imgs, index)` | 已对齐；越界索引夹到范围（TestImagePreviewOutOfRangeInfinite） |
| preview.open | antd 受控 open | 只有查询 `PreviewOpen` | 已补齐：`OpenPreview()`（TestImagePreviewDisabled） |
| preview.src | antd | — | 已补齐：`PreviewSrc(img)` |
| minScale / maxScale / scaleStep | rc-image：1 / 50 / 0.5；EP min-scale 0.2 / max-scale 7 / zoom-rate 1.2 | 固定 0.25 / 8，步长 ×1.5 | 已补齐（规范修正）：令牌 `PreviewMinScale/MaxScale/ScaleStep`（Ant 1/50/0.5，Element 0.2/7/0.2）+ `MinScale`/`MaxScale`/`ScaleStep` 方法（TestImagePreviewScaleBounds） |
| 双击 | rc-image：scale 为 1 时放大一步，否则复位；EP image-viewer 没有双击动作 | 放大到 2 | 已补齐（规范修正）：Ant 放大到 1+step；Element 双击无动作（令牌 `PreviewDblClickZoom`）（TestImagePreviewElementDblClickMode） |
| EP 模式切换 | EP toggleMode：contain（适应）↔ original（原始尺寸），并复位变换；Space 也切换，↑/↓ 缩放 | “复位”按钮只复位变换 | 已补齐：Element 的该按钮切换适应 / 原始尺寸（TestImagePreviewElementDblClickMode）；按钮图标不随模式换（缺 ScaleToOriginal 内置图标），Space / ↑ / ↓ 键暂不做 |
| shadcn | shadcn v4 没有图片预览 | — | 等价写法：shadcn 跟随 Ant 预览（颜色 0.65/0.85、42px 按钮、缩放 1–50、到头即停） |
| movable | antd 默认 true | 总能拖动 | 已补齐：`Movable(bool)` |
| mask.closable / hide-on-click-modal | antd 默认 true；EP 默认 false | 按 Flavor 分支 | 已补齐：令牌 `PreviewMaskClosable` + `MaskClosable(bool)`（TestImagePreviewOutOfRangeInfinite） |
| mask.blur | antd `mask: {blur}` | — | 暂不做，缺背景模糊（见基础能力缺口） |
| 到头即停 / infinite | rc-image PrevNext：首尾禁用，方向键不越界；EP `infinite` 默认 true | 按钮按 Flavor 分支，方向键总是循环 | 已补齐（规范修正）：令牌 `PreviewInfinite` + `Infinite(bool)`；Ant 方向键不再循环（TestImagePreviewOutOfRangeInfinite） |
| close-on-press-escape | EP 默认 true；antd 总是 Esc 关闭 | 总是关闭 | 已补齐：`CloseOnEscape(bool)` |
| show-progress / countRender | antd 组内总显示 “1 / 3”；EP 默认 false | 只有 Ant 显示 | 已补齐：令牌 `PreviewShowProgress` + `ShowProgress(bool)`；countRender 暂不做 |
| actionsRender / toolbarRender / imageRender / icons | antd | 固定工具栏，`Rotate` 可程序调用 | 等价写法：`Rotate`、`PreviewScale`/`PreviewAngle` 查询；自定义工具栏暂不做 |
| fallback / placeholder | antd | `Fallback`、`Loading`、`Failed` | 已对齐 / 等价写法；`placeholder.progress` 暂不做（缺加载进度来源） |
| EP fit / lazy / initial-index / preview-src-list | EP | `Fit`、`PreviewGroup` | 已对齐；lazy 不需要（图像是 Go 值） |
| 键盘 | rc-image ←/→；Esc 关闭 | 有，但焦点在工具栏按钮上时按键被按钮吃掉 | 已补齐（修复）：方向键在按钮之前读取（TestImagePreviewOutOfRangeInfinite） |
| 禁用 | ComponentDisabled | 禁用作用域里仍能打开 | 已补齐：禁用时点击不打开（TestImagePreviewDisabled） |
| 令牌 | antd style：previewOperationColor / Hover / Disabled = 白 0.65 / 0.85 / 0.25；按钮 18+2×12 = 42、关闭按钮距角 marginSM 12、动作间距 12、工具栏内边距 0 24、计数在工具栏上方 16；EP 关闭 40、计数 bottom 90 | 40px 按钮、关闭距角 32、工具栏按钮 44、颜色 .85/1 | 已补齐（规范修正）：新增 `PreviewSwitchColor/HoverColor`、`PreviewCloseOffset`、`PreviewSwitchOffset`、`PreviewActionSize/Gap`、`PreviewActionsPadding`、`PreviewProgressBottom/Color`；禁用的切换按钮无底色（TestAlignB2HImageTokens） |

## Carousel

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| dotPlacement | antd `top\|bottom\|start\|end`（dotPosition 弃用），start/end 默认竖向 | `DotPosition`（含 outside） | 已补齐：`DotPlacement`、Defaults `"dotPlacement"`、`DotOutside()`；`DotPosition` 标 Deprecated（TestCarouselDotPlacement） |
| autoplay / autoplaySpeed | antd `autoplay` 默认 false、3000ms | `Autoplay(time.Duration)` | 等价写法 |
| autoplay.dotDuration | antd：当前指示点随间隔填满 | — | 已补齐：`DotDuration(bool)` |
| speed / effect / arrows / dots / draggable / infinite | antd 500 / scrollx / false / true / false / true | `Fade`、`Arrows`、`Dots`、`Draggable`、`Loop` | 已对齐 |
| waitForAnimate / adaptiveHeight | antd | — | 不需要（goTo 随时改目标）/ 暂不做（先无需求） |
| goTo / next / prev / initialSlide | antd ref | `Current(&i)` | 等价写法；越界值夹到范围，首帧先夹值不播一次错误动画（TestCarouselOutOfRange、TestCarouselOutOfRangeNoAnim）；复审修正 |
| EP trigger | EP `trigger: hover\|click`，默认 hover | 只能点击 | 已补齐：`Trigger(string)`、Defaults `"trigger"`、令牌 `DotHoverTrigger`（Element 默认 hover）（TestCarouselHoverTriggerPause） |
| EP autoplay 默认 true | EP `autoplay: true`、`interval: 3000` | 默认不播放 | 暂不做（所有 Element 轮播默认自动翻页属行为变更，需用户定） |
| EP pause-on-hover | EP / react-slick 默认 true | 总是暂停 | 已补齐：`PauseOnHover(bool)`、Defaults `"pauseOnHover"`（TestCarouselHoverTriggerPause） |
| EP indicator-position / arrow / type card / direction / loop / height | EP | `DotOutside`、`Arrows`、`Card`、`Vertical`、`Loop`、`Height` | 已对齐 |
| EP cards-scale | EP 0.83 | 常量 0.83 | 已补齐：`CardScale(k)` |
| EP motion-blur | EP | — | 暂不做，缺运动模糊 |
| 箭头 | antd：16px、距边 marginXS 8、白 0.4（悬停 1）、到头 opacity 0；EP 36px 圆钮距边 16；shadcn size-8 outline 在 `-left-12` | Ant 28px、0.6、距边 16、到头 0.35；箭头在裁剪框内 | 已补齐（规范修正）：令牌 `ArrowSize 16`、`ArrowColor` 0.4、`ArrowDisabledOpacity 0`、新增 `ArrowOffset`（Ant 8、Element 16、shadcn 16）与 `ArrowIconInset`；箭头移到外层盒，负的 `ArrowOffset` 可把箭头放到框外；shadcn 禁用透明度 0.5（Button disabled:opacity-50）（TestAlignB2HCarouselTokens） |
| shadcn 箭头在框外 | carousel.tsx `-left-12` / `-right-12` | 内侧 16 | 暂不做（偏离）：框外需要每个轮播旁有 48px 空地，否则在 Card / Scroll 里被裁掉或盖住相邻组件（gallery Carousel 页即如此）；默认保留内侧 16，需要时用 `ConfigProvider.Components` 把 `Carousel.ArrowOffset` 设为 −48 |
| DotDuration 重绘 | — | — | 已补齐：只在自动播放计时中、填充未满时请求帧；悬停暂停时填充为 0、`Loop(false)` 停在末张时为满，均不再请求帧（TestCarouselDotDurationIdle） |
| 指示点 | antd：dotWidth 16、dotHeight 3、dotActiveWidth 24、每侧 marginXXS 4、按钮 colorBgContainer 透明度 0.2（悬停 0.75） | 0.3 | 已补齐（规范修正）：Ant `DotColor` 0.3 → 0.2；暗色下 antd 用 colorBgContainer（深色），Tango 保留白色以便在图片上可见——暂不做（需用户定） |
| 键盘 / a11y | ←/→；指示点 tablist | 已有 | 已对齐（TestCarouselDotPlacement） |
| 禁用 | ComponentDisabled | 节点禁用 | 已对齐（TestCarouselDisabledArrows） |
| shadcn orientation | carousel.tsx `orientation` | `Vertical` | 已对齐 |

## Calendar

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| fullscreen / mode / value / onChange / onPanelChange | antd | `Card`、`Mode(&m)`、`Changed`、`PanelChanged` | 已对齐；nil 值不 panic（TestCalendarShowWeekSelect） |
| validRange | antd `validRange` | — | 已补齐：`ValidRange(start, end)`：范围外日期禁用、年下拉只列范围内年份、月份项禁用、翻页与键盘停在范围内（TestCalendarValidRange） |
| disabledDate | antd | `DisabledDate` | 已对齐 |
| showWeek | antd `showWeek` | — | 已补齐：`ShowWeek(bool)`、Defaults `"showWeek"`，ISO 周数列（rowheader，名称走 `DatePicker.WeekFmt`）；令牌 `WeekColumnWidth`（= controlHeightSM×1.5 = 36）、`WeekNumberColor`（取 TextTertiary：TextQuaternary 过不了对比度）（TestCalendarShowWeekSelect） |
| cellRender / fullCellRender / headerRender | antd | `CellRender`、`MonthCellRender`、`NoHeader` | 等价写法：`NoHeader` + 应用自建头部（持有 `Mode`/值指针）；`fullCellRender` 暂不做（单元格整体自定义钩子待设计） |
| yearControlWidth / monthControlWidth | antd minWidth 80 / 70 | 固定 100 / 80（卡片 92 / 68） | 等价写法：Tango Select 不随内容伸缩，固定宽度取中文“2026年”所需宽度，不小于 antd 的 minWidth |
| EP range | EP `range` | `Range` | 已对齐 |
| EP controller-type | EP `button \| select` | 只有按钮 | 已补齐：`ControllerType("select")`、Defaults `"controllerType"`，头部补 Element 小号 `Today` 按钮并使用已有 Locale（TestCalendarShowWeekSelect、TestCalendarSelectToday） |
| EP formatter | EP select 标签格式化 | — | 已补齐：`Formatter(fn(v, kind))` |
| shadcn | calendar.tsx：`--cell-size` 2rem、今天 `bg-accent`、选中 `bg-primary`、外月 muted | 令牌既有 | 已对齐（未改） |
| 键盘 | 方向键、Home/End、PageUp/Down | `calDayKeys` | 已对齐 |

## QRCode

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| value / size / errorLevel / color / bgColor / bordered / icon / iconSize | antd 默认 160、M、colorText、transparent、true、40 | `QRCode`、`Size`、`Level`、`Color`、`Bg`、`Bordered`、`Icon`、`IconSize` | 已对齐 |
| 空 value | antd `if (!value) return null` | 照样编码 | 已补齐：空值不画、`Matrix()` 为 nil（TestQRCodeBoostEmpty） |
| boostLevel | rc-qrcode 默认 true：同版本内提升纠错级别 | 不提升 | 已补齐（规范修正）：`BoostLevel(bool)`、Defaults `"boostLevel"`（TestQRCodeBoostEmpty） |
| marginSize | antd 默认 0 | — | 已补齐：`MarginSize(n)`、Defaults `"marginSize"` |
| status / onRefresh | antd active/expired/loading/scanned | `Status`、`Refreshed` | 已对齐 |
| statusRender | antd | — | 等价写法：`StackBox` 叠放自定义内容 + `Status("active")` |
| iconSize {width,height} / type canvas\|svg | antd | 正方形 | 暂不做（先无需求）/ 不需要 |
| 边框与内边距 | antd style：`border lineWidth colorSplit`、`padding paddingSM`；borderless 去掉内边距和圆角 | 边框 BorderSecondary；无边框仍留 12 内边距 | 已补齐（规范修正）：边框改 `Split`，无边框时内边距 0、圆角 0 |
| 暗色白底 | 扫码可读性 | 暗色固定白底黑码（有注释） | 已对齐（保留既有决定） |

## Watermark

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| content / rotate / gap / offset | antd 默认 -22、[100,100]、gap/2 | `Watermark(lines...)`、`Rotate`、`Gap`、`Offset` | 已对齐 |
| 排布 | antd useClips：标记绕中心旋转，取旋转后包围盒 + gap 为一格，每隔一列下移半格；offset − gap/2 为起点，正值同时移动水印区域边缘 | 未旋转尺寸 max(w,120)×max(h,64) 加 gap，隔行右移半格 | 已补齐（规范修正）：改为 antd 算法；`Grid()` 保留为不错列的扩展 |
| 标记尺寸 width / height | antd：文字按最宽行与行高之和 + 行间 3px；图片默认 120×64 | 见上 | 已补齐：按 antd 计算，`MarkSize(w,h)` 覆盖（TestWatermarkMarkSize） |
| 多行行距 | antd：每行 fontSize + 3 | 行高之和 | 已补齐（规范修正） |
| 负 gap | — | — | 已补齐：格宽 / 格高至少 1px，`Gap(-1000,-1000)` 不再卡死（TestWatermarkNegativeGap） |
| image | antd 图片画进同一画布：随 rotate 旋转、原样不透明 | 不旋转、默认 0.2 透明度 | 已补齐（规范修正）：图片随 `Rotate` 旋转，默认不透明度 0.2 → 1（`ImageOpacity` 仍可调） |
| font.color / fontSize / fontWeight | antd colorFill（rgba(0,0,0,.15)）、fontSizeLG 16、400 | `Color`、`FontSize`、`Weight`；默认 `Fade(Text,.17)` | 已对齐（Ant 下等于 colorFill，并随主题与暗色变化） |
| font.fontStyle | antd normal/italic/oblique | — | 已补齐：`FontStyle(string)` |
| font.fontFamily / textAlign、content 数组里逐行字体 | antd | — | 暂不做（先无需求；字体族选择缺 text 层入口） |
| zIndex / onRemove | antd | 画在内容之上，不挡指针 | 不需要（无 DOM、无防篡改） |
| inherit | antd：嵌套 Modal / Drawer 继承水印 | — | 暂不做，缺浮层继承容器装饰（见基础能力缺口） |
| EP el-watermark | 同族属性，默认值同 antd | — | 已对齐 |

## Masonry

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| columns（含响应式） | antd 默认 3；响应式从 xxl 往下取第一个匹配，没有匹配取 `xs ?? 1` | `Columns`（默认 3）、`ColumnsAt`、Defaults `"columns"` | 已对齐；未匹配时 Tango 退回 `Columns`——等价写法（要 antd 行为写 `Columns(1)`） |
| gutter | antd 默认 0，`[h, v]`、响应式 | `Gutter`、`Gutters`、Defaults `"gutter"` | 已对齐；响应式 gutter 暂不做（先无需求） |
| items / itemRender / key / column / height | antd MasonryItemType | `render(i)`、`ItemKey`、`ItemColumn`、`ItemHeight` | 等价写法 |
| 排布 | antd usePositions：按顺序放进最矮列（同高取最左） | `fillMasonryPlace` | 已对齐 |
| onLayoutChange | antd：项的列变化时回调 | `Assignment` | 已补齐：`LayoutChanged()`（TestMasonryLayoutChangedEmpty） |
| fresh | antd：子项尺寸变化时重新测量 | 每帧按上一帧实测高度重排 | 等价写法（总是 fresh） |
| sequential | 计划里提到 | — | 不需要（antd 6.7 的 Masonry.d.ts 没有此属性） |
| 进出场动效 | antd CSSMotion fade | — | 暂不做（先无需求） |
| 空数据 | — | — | 已对齐：0 项不 panic（TestMasonryLayoutChangedEmpty） |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 背景模糊 | Image 预览（antd `mask.blur`） | paint 层 backdrop blur 原语（与第 1 批 FooterToolbar 同一缺口） | 暂不做，等用户决定 |
| 运动模糊 | Carousel（EP `motion-blur`） | paint 层方向模糊 | 暂不做，等用户决定 |
| 浮层继承容器装饰 | Watermark（antd `inherit`：Modal / Drawer 里继续画水印） | overlay 层把祖先的 `over` 装饰带进浮层（或浮层查询祖先水印） | 暂不做，等用户决定 |
| 字体族选择 | Watermark（`font.fontFamily`） | text 层按名字选字体族 | 暂不做，等用户决定 |

## CHANGELOG 草稿

### 新增

- Image：令牌 `ImageToken.PreviewDblClickZoom`（Ant / shadcn true，Element false）。
- Tabs：`TabPlacement(string)`（antd v6 `top/bottom/start/end`；Defaults `"tabPlacement"`）、`Indicator(size, align)`、`TabBarGutter(px)`（Defaults `"tabBarGutter"`）、`Stretch()`（EP stretch；Defaults `"stretch"`）、`BeforeLeave(fn)`（EP before-leave）、`TabClicked()`、`Variant("line")`（shadcn TabsList line 变体；Defaults `"variant"`）、`AddIcon` / `RemoveIcon` / `MoreIcon`；`Tabs(c, nil, items)` 允许 nil 指针；令牌 `TabsToken.CardHeightSM/LG`、`HorizontalItemPaddingSM/LG`、`CardPaddingH/HSM`、`ItemSelectedBg`、`ItemSelectedBorder`。
- Timeline：`Orientation(string)`、`Variant(string)`、`TitleSpan(span)`、Mode `"alternate-reverse"`；`TimelineItem.Title`、`Placement`、`Loading`、`Center`；Defaults `"orientation"`、`"variant"`。
- Image：`OpenPreview()`、`PreviewSrc(img)`、`MinScale` / `MaxScale` / `ScaleStep`、`Movable`、`MaskClosable`、`Infinite`、`ShowProgress`、`CloseOnEscape`；令牌 `ImageToken.PreviewSwitchColor/HoverColor`、`PreviewCloseOffset`、`PreviewSwitchOffset`、`PreviewActionSize/Gap`、`PreviewActionsPadding`、`PreviewProgressBottom/Color`、`PreviewMinScale/MaxScale/ScaleStep`、`PreviewMaskClosable`、`PreviewInfinite`、`PreviewShowProgress`。
- Carousel：`DotPlacement(string)`（Defaults `"dotPlacement"`）、`DotOutside()`、`DotDuration(bool)`、`PauseOnHover(bool)`（Defaults `"pauseOnHover"`）、`CardScale(k)`、`Trigger(string)`（Defaults `"trigger"`）；令牌 `CarouselToken.ArrowOffset`、`ArrowIconInset`、`DotHoverTrigger`。
- Calendar：`ValidRange(start, end)`、`ShowWeek(bool)`（Defaults `"showWeek"`）、`ControllerType(string)`（Defaults `"controllerType"`）、`Formatter(fn)`；`Calendar(c, nil)` 允许 nil；令牌 `CalendarToken.WeekColumnWidth`、`WeekNumberColor`。
- QRCode：`BoostLevel(bool)`（Defaults `"boostLevel"`）、`MarginSize(n)`（Defaults `"marginSize"`）。
- Watermark：`MarkSize(w, h)`、`FontStyle(string)`。
- Masonry：`LayoutChanged()`。

### 变更

- 已弃用（保留）：`CarouselW.DotPosition` → `DotPlacement` / `DotOutside`；`TimelineItem.Label` → `Title`；Timeline Mode `"left"`/`"right"` → `"start"`/`"end"`（默认值由 `"left"` 改为同义的 `"start"`）。
- Tabs：Ant 主题 `Size(Large)` 的线条标签上下内边距 12 → 16、`Size(Small)` 的卡片高度 40 → 32、内边距 16 → 8、`Size(Large)` 卡片高度 40 → 48（antd cardHeightSM / LG、horizontalItemPaddingLG、cardPaddingSM）；新增按钮随卡片高度。shadcn 暗色选中项背景 neutral-950 → `input/30` 叠在 muted 上并加 `input` 描边（tabs.tsx）。`*active` 不在 items 里时改为回落到上次索引的标签（rc-tabs）；传空串也会回落到第一个标签（不经过 BeforeLeave，不报 Changed）。Dock 的标签条不受这些变更和 `Defaults("Tabs", …)` 影响，外观不变。
- Timeline：Ant 空心圆点描边 3 → 2（antd v6 dotBorderWidth = lineWidthBold）；“进行中”项的虚线规则改为 v6：普通项到 Pending 项的一段为实线，离开 Loading / Pending 项的一段为点线（之前是通往 Pending 的一段为虚线）。
- Image（Ant 主题）：预览缩放范围 0.25–8 → 1–50（rc-image minScale / maxScale），双击放大 ×2 → ×1.5；关闭按钮距角 32 → 12、关闭 / 切换按钮 40 → 42（paddingSM 12 + 图标 18）、工具栏按钮 44 → 42 且间距 12、内边距 24（6 个动作宽 288 → 360）、工具栏图标颜色 0.85 / 1 → 0.65 / 0.85、计数距底 88 → 90；禁用的切换按钮去掉底色；方向键到首尾不再循环（rc-image PrevNext）。Element：缩放范围 0.25–8 → 0.2–7、每步 ×1.5 → ×1.2（EP min-scale / max-scale / zoom-rate），双击不再缩放（EP 无双击动作），“复位”按钮改为切换适应 / 原始尺寸（EP toggleMode），计数可用 `ShowProgress(true)` 显示（之前不显示），其余数值不变。shadcn（v4 无图片预览）跟随 Ant：工具栏颜色 0.85 / 1 → 0.65 / 0.85、按钮 40 / 44 → 42、关闭按钮距角 32 → 12、缩放 1–50、到头即停。修复：预览中焦点在工具栏按钮上时 ←/→ 无效。
- Carousel（Ant 主题）：箭头 28 → 16px、距边 16 → 8、颜色透明度 0.6 → 0.4、到头时透明度 0.35 → 0（antd slick-disabled）；指示点透明度 0.3 → 0.2（antd 按钮 opacity 0.2）。shadcn：箭头位置不变（内侧 16），禁用透明度 0.35 → 0.5。`Current` 越界改为夹到范围（之前按取模）。Element：悬停指示点即切换（EP trigger 默认 hover）。
- QRCode：默认在不增大版本的前提下提升纠错级别（rc-qrcode boostLevel 默认 true），同一内容的码图会变化；边框颜色 BorderSecondary → Split（antd colorSplit）；`Bordered(false)` 去掉 12px 内边距和圆角（antd borderless）；空字符串不再绘制二维码。
- Carousel：`DotDuration` 只在计时中重绘（悬停暂停、停在末张时不再持续请求帧）。
- Watermark：排布改为 antd 算法（按旋转后包围盒排格、隔列下移半格，多行行距 fontSize + 3）；图片标记随 `Rotate` 旋转，默认不透明度 0.2 → 1。

## 共享文件改动请求

1. `ui/a11y_data_display_test.go`（`TestA11yImage`）：已由主线程修复（minScale 1 时断言“缩小”为禁用）。
2. `api/go/ui.txt`、`api/go/ui_theme.txt`：重新生成。全部为新增（`TabsW`、`TabItem` 只加方法；`TabsToken`、`TimelineToken`、`ImageToken`、`CarouselToken`、`CalendarToken` 加字段；`TimelineItem` 加字段），不破坏稳定层（`Tabs`、`TabsW`、`TabItem` 在 `api/stable/ui.txt`，均只增）。Defaults 新键见 CHANGELOG 草稿（`prop` 行）。
3. `CHANGELOG.md`：合并上面的草稿。
4. `ui/AUTHORING.md` §14：表行不变（无新入口）；可在 Tabs 行补“`Variant("line")` shadcn 下划线变体、`Stretch`、`BeforeLeave`”。
5. Locale：本组没有改 `locale.go` / `locale_packs.go`（周数名称复用 `DatePicker.WeekFmt`）。
6. `ui/dock.go`（经主线程许可）：第 366 行 `Tabs(...)` 改为私有 `dockTabs(...)`，其余未动。
7. gallery：新增 `examples/ui-gallery/page_b2h.go`（8 个 `*Align` 页，分别挂在 Tabs、Timeline、Image、Carousel、Calendar、QRCode、Watermark、Masonry 下）。既有页 `page_data_misc2.go`（Carousel / Calendar，主线程）、`page_nav.go`（Tabs）、`page_fill_layouts.go`（Masonry）的截图会随上面的“变更”而变化（Ant 轮播箭头、Tabs 大小尺寸、QRCode、Watermark），按 Review Focus 1 对比时应只出现在这些组件上。
8. 参考：`date_util.go` 不需要改动。
