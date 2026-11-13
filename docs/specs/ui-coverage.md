# Tango UI 组件覆盖率扫描

> 扫描日期：2026-11-13
> 范围：`ui/` 包，以及 `ui/chart`、`ui/mobile`、`ui/editor` 子包
> 对标：Ant Design v6、Element Plus、Ant Design Pro Components

## 1. 摘要

### 1.1 方法

- **官方清单**
  - Ant Design v6 与 Element Plus：2026-11-13 用 WebFetch 读取官网 overview 页。
    - Ant 共 73 项，去掉 `Util` 后计 72 个。v6 新增 `Masonry`、`Listy`、`BorderBeam`，`List` 标为 deprecated。
    - Element 共 82 项，去掉设计规范页 Border / Color / Typography 后计 79 个。
  - Pro Components：WebFetch 只拿到页面元数据，清单与 API **依据对 procomponents v2 的知识整理**。
  - 各组件的 API 细项对照也依据知识，没有逐页抓取官方 API 文档。
- **代码证据**：对每个组件 grep 入口函数 `func Xxx(c *Context…)` 和构建器方法 `func (w *XxxW) Foo`，再读关键注释。证据写成"文件 + 方法/字段名"，代码都在 `ui/` 下，下文省略前缀。拿不准的标"未确认"。
- **状态口径**（三家统一）
  - ✅ 完整：主流 API 与模式都有。缺的只是渲染钩子、个别回调或配置项，这些列在"次要缺口"里，不影响判定。
  - ⚠️ 部分：缺少某种**模式、变体或常用能力**，例如范围粒度、懒加载语义、响应式列数、弹出分组标题。
  - ❌ 缺失：没有对应入口。
- **截图**：`go run ./examples/ui-gallery -out /tmp/ui-cov -theme all -scale 1`，耗时 3 分 55 秒，退出码 0，写出 927 张组件图加首页、总览、仪表盘、浮层图。另跑 `-audit`。
- **交叉核查**：数据录入、展示/导航/反馈、Pro 三块各由一个只读子代理 grep 核查，关键结论（Tabs shadcn 分支、Dropdown 分组、Defaults 接线、Button 变体等）又在主线程复核过。

### 1.2 计数

| 对标 | 总数 | ✅ 完整 | ⚠️ 部分 | ❌ 缺失 | 上次扫描 |
|---|---|---|---|---|---|
| Ant Design v6 | 72 | 42 | 29 | 1（App） | 72：46 / 23 / 3 |
| Element Plus | 79 | 44 | 35 | 0（另：CascaderPanel 子能力缺失） | 79：53 / 22 / 4 |
| Pro Components | 28 | 17 | 8 | 3（SettingDrawer、ProFormGroup、RouteContext/GridContent） | 约 22 个，缺 9 |

### 1.3 与上次扫描对比

- **缺失项大幅减少。**
  - Ant：v6 新组件 Masonry、Listy、BorderBeam 都已有实现，只剩 `App` 缺失。
  - Element：Input OTP、Virtualized Select/Tree、DatePickerPanel、ColorPickerPanel 都已有实现，没有缺失项。
  - Pro：补了 pro2 / pro3 两批，包括 PageContainer、FooterToolbar、ProCard、StatisticCard、CheckCard、ProSkeleton、ProDescriptions、ProList、LightFilter、ProFormFieldSet、LoginForm。缺失项从 9 个降到 3 个。
- **"完整"变少、"部分"变多，主要是本次口径更严，不是能力倒退。**
  - 本次逐个对照了 v6 新增的模式：Timeline 横向、Segmented 竖向、Steps inline/panel、Drawer resizable、Button color×variant。
  - 本次也把 Dropdown 分组失效、Link type 着色无效、shadcn Tabs 无溢出处理等新发现的缺口计入了"部分"。
- **已知缺口的复核结果**：用户列出的已知缺口，除"Pro2 组件 shadcn 下未截图检查"外**全部仍存在**，证据见各表和第 8 节。Pro2 那一项本次已通过 shadcn 渲染和抽查补上，见第 5 节。

## 2. Ant Design v6（72）

| 组件 | Tango 入口 | 状态 | 缺的能力 / 次要缺口 | 代码证据 |
|---|---|---|---|---|
| **通用** | | | | |
| Button | `Button`、`IconButton` | ⚠️ | 缺 v6 的 `color` 预设色 × `variant`（solid/outlined/filled/text/link）组合，只有 Primary/Dashed/Text/Link/Secondary + Danger/Ghost | button.go `Primary/Dashed/Text/Link/Secondary/Danger/Ghost/Round/Circle/Loading/Block/Icon/IconRight` |
| FloatButton | `FloatButton`、`FloatButtonGroup`、`BackTop` | ✅ | — | nav_float.go `Badge/Tooltip/Square/Description`；Group 有 `Trigger/Open` |
| Icon | `Icon` | ✅ | — | text.go `IconW.Spin/Rotate/Color`；icon/icon.go `icon.New` |
| Typography | `Text`、`Title`、`Paragraph`、`Link` | ✅ | 省略时悬停显示全文未确认 | data_typography.go `Copyable/CopyText/Editable/Expandable`；text.go `Ellipsis/Lines` |
| **布局** | | | | |
| Divider | `Divider`、`VDivider` | ✅ | — | display.go `Dashed/Orientation/Plain` |
| Flex | `Flex` | ✅ | — | nav_space.go `Vertical/Justify/Align/Gap/Wrap` |
| Grid | `GridRow`、`Col` | ✅ | — | nav_grid.go `GutterAt`；`XS…XXL/Offset/Push/Pull/Order/Flex` |
| Layout | `Layout/Header/Sider/Content/Footer`、`CollapsibleSider` | ✅ | — | prolayout.go `Breakpoint/CollapsedWidth/NoTrigger/ReverseArrow` |
| Masonry | `Masonry` | ✅ | — | fill_masonry.go `Columns/ColumnsAt/Gutters/Virtual` |
| Space | `Space`、`Compact`、`Addon` | ✅ | — | nav_space.go `Size/Split/Wrap`；entry_compact.go `Vertical/Block` |
| Splitter | `Splitter`、`SplitterPanel` | ⚠️ | 无 `lazy` 懒拖拽；collapsible 不能分 start/end，也不能自定义图标；无 resizeStart/End 事件 | nav_splitter.go 只有 `Collapsible/Resizable/Min/Max/Changed`，grep `lazy` 为 0 |
| **导航** | | | | |
| Anchor | `Anchor`、`AnchorSection` | ✅ | — | nav_affix.go `Horizontal/TargetOffset/NoInk`、`AnchorItem.Children` |
| Breadcrumb | `Breadcrumb`、`BreadcrumbItems` | ✅ | — | nav.go `BreadcrumbItem.Menu/Href/Icon`、`SeparatorIcon` |
| Dropdown | `Dropdown` | ⚠️ | **`MenuItem.Group` 完全不处理**，分组项被当成可展开的子菜单（无标题、无分隔线） | nav_dropdown.go:849/851 `pop.panel(d.items…)` 没有调用 `menuFlatGroups`，文件内也没有 `.Group` 分支 |
| Menu | `Menu` | ⚠️ | 弹出子菜单与折叠态里的分组**没有标题**，只用 Divider 隔开；只有 inline 模式画标题 | nav.go:34 `menuFlatGroups`（丢弃 `it.Label`）；调用处 nav_menu.go:493/524/849；inline 标题见 nav_menu.go:452 |
| Pagination | `Pagination` | ✅ | 次要：无 `itemRender` | nav.go `ShowTotal/ShowQuickJumper/Simple/SizeChanger/HideOnSinglePage` |
| Steps | `Steps` | ⚠️ | 无 `type=inline` / `panel`；progressDot 不能自定义渲染 | nav_steps.go `Type` 只认 navigation/simple；有 `Percent/Dot/LabelVertical` |
| Tabs | `Tabs` | ⚠️ | shadcn 下 Closable/OnAdd/非 Top 位置退回 Ant 画法，Card 被画成 segmented；shadcn 横向和所有竖向 Tabs 溢出时都没有"…"；Extra 只有右侧一处；不能拖拽排序 | nav.go:317 `Flavor==StyleShadcn && place==Top && !closable && onAdd==nil` 才走 `showShadcn`；`overflow/moreMenu` 只在横向分支调用 |
| **数据录入** | | | | |
| AutoComplete | `AutoComplete` | ⚠️ | 无 Variant | entry_autocomplete.go `Source(func(q) []Option)`、`Remote/Debounce/Loading`（异步远程搜索，与 Select 共用 remote_search.go）、`AllowClear/Status/Prefix/Highlight` |
| Cascader | `Cascader` | ⚠️ | 懒加载时未加载的父节点按叶子处理；勾选策略只影响显示；无自定义 filter、displayRender、Variant | entry_cascader.go:129-130 注释 "A lazy node whose children are not loaded yet counts as a leaf"；p2b_cascader.go:42-46 |
| Checkbox | `Checkbox`、`CheckboxGroup`、`CheckAll` | ✅ | 次要：`CheckboxGroup` 是返回 bool 的函数，不能整组 Disabled 或链式配置 | choice.go:274 `func CheckboxGroup(...) bool`；`Indeterminate/Disabled` |
| ColorPicker | `ColorPicker`、`ColorPickerPanel` | ⚠️ | **无渐变**；格式只有 hex/rgb（缺 hsb）；预设只能一组；无 hover 触发 | entry_color.go:148-150 只接受 "hex"/"rgb"；:133 `Presets(cols…)`；grep `gradient` 为 0 |
| DatePicker | `DatePicker`、`DateTimePicker`、`RangePicker`、`DatePickerPanel` | ⚠️ | RangePicker 无周/月/季/年粒度；DatePicker 和 RangePicker 不能键盘输入；无 multiple、needConfirm、disabledTime；DateTimePicker 无 DisabledDate/Presets | `RangePickerW` 无 `Picker` 方法；粒度范围只在内嵌 `DatePickerPanel.Range().Picker()`（fill_date_panel.go）；可输入的只有 entry_datetime.go `dtParse` |
| Form | `Form`/`FormItem`、`FormList` | ⚠️ | 不能按字段校验或清除（只有全量 `Validate`/`ResetValidation`）；无 labelCol/wrapperCol（只有 LabelWidth）；无表单级 disabled、requiredMark；绑定 `RulesFunc` 的项不能 ResetFields | form.go:648 `Validate`、:673 `ResetValidation`；p2d_form.go:103-106 "Items bound with RulesFunc cannot be restored"（`RulesOf[T]` 可以） |
| Input | `Input`、`TextArea`、`Search`、`OTP` | ✅ | 次要：TextArea/Search/OTP 无 Variant；OTP 无 separator/formatter | input.go `AllowClear/Password/Prefix/Suffix/ShowCount/MaxLength/Status`；p2d_input.go `Variant`（4 种） |
| InputNumber | `InputNumber` | ✅ | 次要：无 suffix；不能隐藏控制按钮 | input.go `ControlsPosition`（right/sides）、`Variant`、`Wheel` |
| Mentions | `Mentions` | ⚠️ | 无 AllowClear、Variant | entry_mentions.go `Remote(func(prefix, query))/Debounce/Loading`、`Query()`、`Prefix/Rows/Selected…` |
| Radio | `Radio`、`RadioGroup` | ✅ | 次要：整组不能 Disabled | choice.go `Buttons/Solid/Size/Changed` |
| Rate | `Rate` | ✅ | — | entry_rate.go `AllowHalf/AllowClear/Character/Count/Texts` |
| Select | `Select`、`MultiSelect`、`InputTag`、`VirtualSelect` | ⚠️ | 无 Variant、prefix/suffixIcon | p2b_select.go `Remote/FilterMethod/AllowCreate/Virtual/MaxCount`；fill_virtual_select.go `FilterMethod` |
| Slider | `Slider`、`SliderRange` | ⚠️ | 范围滑块两柄不能交叉，也没有 pushable；WithInput 只支持单值横向；无 reverse/included | p2b_slider.go:178-187 `limits`；:143 "single sliders only"；:315 `w.input && w.hi == nil && !w.vertical` |
| Switch | `Switch` | ✅ | — | choice.go `Small/Loading/Texts` |
| TimePicker | `TimePicker`、`TimePanel`、`TimeRangePicker` | ⚠️ | TimeRangePicker 不能直接输入；无 hideDisabledOptions；无 needConfirm | p2a_time_range.go 触发器是 `Text`（:342），`dtTimeRangeState` 无输入缓冲；grep `hideDisabled` 只命中 Pagination 测试 |
| Transfer | `Transfer` | ⚠️ | 分页是自绘的简易版（不复用 Pagination）；无自定义 filterOption、footer | p2b_transfer.go:115 `pager` 注释 "simple pager (‹ 1 / 3 ›)" |
| TreeSelect | `TreeSelect` | ⚠️ | 不能程序化展开（无受控 treeExpandedKeys）；勾选策略只影响显示；无 treeCheckStrictly、filterTreeNode | entry_treeselect.go:102 只有 `DefaultExpandAll`；p2b_treeselect.go:21-24 "The bound value always holds the checked leaves"；`setExpanded` 是私有的 |
| Upload | `Upload`、`DropZone` | ⚠️ | 照片墙（card）无重试；无 listType=picture；无 customRequest、手动 submit | entry_upload.go:33-37 只有 `upText/upCard/upDrag`；重试只在文本列表（:254-267）；p2d_upload.go `BeforeUpload/Action/Retries` |
| **数据展示** | | | | |
| Avatar | `Avatar`、`AvatarGroup` | ✅ | — | display.go `Size/Square/Image/Icon`；data_badge.go `AvatarGroupW.Max` |
| Badge | `Badge`、`BadgeRibbon` | ✅ | — | data_badge.go `Count/Dot/Max/ShowZero/Status/Offset` |
| Calendar | `Calendar` | ✅ | 次要：无 headerRender、validRange | data_calendar.go `Card/Mode/CellRender/MonthCellRender/NoHeader` |
| Card | `Card`、`CardMeta`、`CardAction` | ⚠️ | 无 `tabList`（卡片内页签） | data_card.go `Extra/Cover/Actions/Inner/Loading`；grep `tabList` 为 0 |
| Carousel | `Carousel` | ⚠️ | 不能拖拽/滑动切换；无 dot 进度条；不能关闭循环 | data_carousel.go `Fade/Vertical/DotPosition/Autoplay`；grep `drag/swipe/infinite` 为 0 |
| Collapse | `Collapse` | ✅ | 次要：无 `collapsible="header"/"icon"` | data_collapse.go `Accordion`、`CollapseItem{Disabled,NoArrow,Extra}` |
| Descriptions | `Descriptions` | ⚠️ | **列数不随断点变化**（Ant 的 column 可传 `{xs,sm,…}`） | data_card.go:329 `Column(n int)`；`ScreenBreakpoint` 只在 fill_masonry.go、nav_grid.go、prolayout.go 里用 |
| Empty | `Empty` | ✅ | — | data_result.go `Simple/Image/ImageSize/Description` |
| Image | `Image` | ✅ | 次要：无 toolbarRender/imageRender | data_image.go `PreviewGroup`，带计数、旋转、翻转、缩放 |
| List（v6 已 deprecated） | `List`、`ListItemMeta`、`ListActions` | ⚠️ | **网格列数不随断点变化** | data_list.go:46 `Grid(columns int, gap)` |
| Listy | `Listy` | ✅ | — | fill_listy.go `Virtual/Group/Sticky/ScrollToKey/ScrollToGroup/EndReached` |
| Popover | `Popover` | ✅ | — | data_popover.go `Title/Content/Trigger/Placement/Open` |
| QRCode | `QRCode` | ✅ | — | data_qrcode.go `Level/Icon/Status/Refreshed/Bordered` |
| Segmented | `Segmented` | ⚠️ | 无 v6 的 `orientation="vertical"`、`shape="round"` | entry_segmented.go 只有 `Icons/Block/Size/Disabled` |
| Statistic | `Statistic`、`Countdown` | ⚠️ | 无 `Statistic.Timer type=countup`；Countdown 无 onChange | display.go `Statistic`；data_skeleton.go `CountdownW.Format/Prefix/Suffix/Finished`；`CountUp` 只是入场动画 |
| Table | `Table`（及 `ProTable`、`EditableProTable`） | ✅ | 次要：列虚拟化未确认；汇总格只能返回字符串 | table*.go `Sortable/MultiSort/Filters/FilterPanel/Fixed/ScrollY/Sticky/Expandable/Tree/TreeLazy/Selection/CheckStrictly/Span/Summary/Virtual/Resizable/TableEditor/ServerPagination/Children/DragSort/Wrap` |
| Tag | `Tag`、`CheckableTag` | ✅ | — | display.go `Closable/Color/Icon/Borderless`；more_checktag.go |
| Timeline | `Timeline` | ⚠️ | 无 v6 的 `orientation="horizontal"` | data_timeline.go:46 `Mode` 只有 left/right/alternate |
| Tooltip | `Tooltip` | ✅ | — | display.go `Color/Arrow/MouseEnterDelay/Trigger` |
| Tour | `Tour` | ⚠️ | 每步不能单独设置 mask/type/按钮；无 indicatorsRender/actionsRender；是否自动把目标滚进可见区未确认 | data_popover.go:390 `TourStep{Target,Title,Description,Cover,Placement}`；`TourW.Primary/NoMask/NoArrow/Gap` |
| Tree | `Tree`（虚拟化走 `VirtualTree`） | ⚠️ | **Tree 本身无虚拟滚动**；无 titleRender、allowDrop | data_tree.go `Show` 递归构建全部可见节点，无 `Height/Virtual`；fill_virtual_tree.go:117 `VirtualTree` 是独立实现 |
| **反馈** | | | | |
| Alert | `Alert` | ✅ | 次要：不能自定义图标 | feedback.go `Action/Description/NoIcon/Closable/Banner` |
| Drawer | `Drawer` | ⚠️ | 无 v6 的 `resizable`；无 push | feedback.go grep `resiz` 为 0 |
| Message | `Message`、`MessageDestroy` | ✅ | 次要：只能在顶部，无 top 偏移、maxCount | nav_notify.go:149 `Place: ScreenTop`；`MessageRef.Key/Duration` |
| Modal | `Modal`、`Confirm`、`MessageBox`、`Prompt` | ✅ | 次要：即时模式下用不上 useModal；ConfirmOptions 不能自定义 icon/footer | feedback.go `Draggable/Fullscreen/ConfirmLoading/Centered`；more_confirm.go `ConfirmOptions` |
| Notification | `Notification`、`NotificationDestroy` | ✅ | 次要：无 maxCount；stack 阈值只能用全局变量 | nav_notify.go 六个方位；:238 `NotifyStackThreshold` |
| Popconfirm | `Popconfirm` | ✅ | — | feedback.go `OkDanger/ShowCancel/Icon/Description` |
| Progress | `Progress` | ✅ | — | data_progress.go `Circle/Dashboard/Steps/Gradient/Success/Format` |
| Result | `Result` | ✅ | — | data_result.go |
| Skeleton | `Skeleton` + `SkeletonAvatar/Button/Input/Image` | ✅ | — | data_skeleton.go |
| Spin | `Spin`、`Loading` | ✅ | — | feedback.go `Delay/Percent/Wrap/Tip`、`Loading.Fullscreen` |
| Watermark | `Watermark` | ✅ | 次要：无 fontFamily/fontStyle、textAlign、inherit | data_watermark.go `Rotate/Gap/Offset/FontSize/Image/Grid` |
| **其他** | | | | |
| Affix | `Affix` | ✅ | — | nav_affix.go `OffsetTop/OffsetBottom/Target` |
| App | — | ❌ | 无 App 组件。Message/Notification 直接挂在全局层，即时模式下不需要 App 上下文，所以影响小 | nav_notify.go `globalLayers`；grep `useApp` 为 0 |
| BorderBeam | `BorderBeam` | ✅ | — | fill_border_beam.go |
| ConfigProvider | `ConfigProvider` | ⚠️ | `Components`（组件令牌）与 `Direction`（RTL）已接入（见 rtl-and-component-tokens.md）；`Defaults` 只接线到 33/158 个公开组件（见第 6 节） | nav_config.go `Size/Locale/Defaults/ComponentDisabled/Theme/Flavor/Dark/PrimaryColor/Compact` |

## 3. Element Plus（79）

不计入的设计规范页：Border、Color、Typography（Typography 页的字体规范由 `theme` 令牌与 `Text/Title` 体现）。

| 组件 | Tango 入口 | 状态 | 缺的能力 / 次要缺口 | 代码证据 |
|---|---|---|---|---|
| **Basic** | | | | |
| Button | `Button` | ⚠️ | 无 type=success/warning/info、`plain` 全套、`ButtonGroup` | button.go 只有 Primary/Dashed/Text/Link/Secondary/Danger；grep `ButtonGroup` 为 0 |
| Layout Container | `Layout/Header/Sider/Content/Footer` | ✅ | — | prolayout.go |
| Icon | `Icon` | ✅ | — | text.go `IconW` |
| Layout（row/col） | `GridRow`、`Col` | ✅ | — | nav_grid.go |
| Link | `Link` | ⚠️ | **type 着色不生效**（paint 固定用 `th.TextLink`）；无 icon | text.go:312-326 paint 强制 `col := th.TextLink` |
| Text | `Text` | ✅ | — | text.go `Type/Size/Ellipsis/Lines` |
| Scrollbar | `Scroll`、`ScrollH` | ✅ | — | scroll.go `Always/Both/ScrollTo/Offset` |
| Space | `Space` | ✅ | — | nav_space.go `Split/Wrap/Size` |
| Splitter | `Splitter` | ⚠️ | 无 lazy；collapsible 不能分方向 | nav_splitter.go |
| **Configuration** | | | | |
| Config Provider | `ConfigProvider` | ⚠️ | 同 Ant 那一行；Element 的 button.autoInsertSpace、message.max 等无对应 | nav_config.go |
| **Form** | | | | |
| Autocomplete | `AutoComplete` | ⚠️ | 无 trigger-on-focus（异步取数用 `Remote/Debounce/Loading`） | entry_autocomplete.go |
| Cascader | `Cascader` | ⚠️ | 同 Ant 那一行；无 show-all-levels/separator、filter-method；**无 `el-cascader-panel` 内嵌面板** | grep `CascaderPanel` 为 0 |
| Checkbox | `Checkbox`、`CheckboxGroup` | ⚠️ | 无 checkbox-button、border 样式；Group 无 min/max；无 size | choice.go:274-292 |
| Color Picker Panel | `ColorPickerPanel` | ✅ | — | fill_color_panel.go `Presets/Format/NoAlpha/AllowClear/Border` |
| Color Picker | `ColorPicker` | ✅ | 次要：无 hsl/hsv 格式 | entry_color.go:148 |
| Date Picker Panel | `DatePickerPanel` | ✅ | — | fill_date_panel.go `PickerType`（日/周/月/季/年）、`Range` |
| Date Picker | `DatePicker`、`RangePicker` | ⚠️ | 无 monthrange/yearrange；无 dates/months 多选；不能键盘输入 | 同 Ant DatePicker 那一行 |
| DateTime Picker | `DateTimePicker`、`RangePicker.ShowTime` | ⚠️ | 无 disabled-date、shortcuts | entry_datetime.go 方法列表；entry_daterange.go:126 `ShowTime` |
| Form | `Form` | ⚠️ | 无 validateField(prop)、clearValidate(props)、scrollToField；无 hide-required-asterisk、表单级 disabled；RulesFunc 不能 reset | form.go:648/673；p2d_form.go:103 |
| Input | `Input`、`TextArea` | ✅ | — | input.go、entry_inputx.go |
| Input Number | `InputNumber` | ✅ | 次要：不能 `controls=false` | input.go:463 |
| Input Tag | `InputTag` | ⚠️ | 分隔符固定为英文/中文逗号；无 draggable、save-on-blur | entry_inputtag.go:124 |
| Input OTP | `OTP` | ✅ | 次要：无 separator、formatter | entry_otp.go `Mask/Numeric/Status/Size/Gap` |
| Mention | `Mentions` | ⚠️ | 无 whole 整体删除 | entry_mentions.go（`Remote/Loading` 对应 search/loading） |
| Radio | `Radio`、`RadioGroup` | ⚠️ | 无 border 样式；整组不能 disabled（radio-button 由 `Buttons` 提供） | choice.go:450-463 |
| Rate | `Rate` | ✅ | 次要：无 low/high 阈值分段配色 | entry_rate.go `Color(theme.Color)` |
| Select | `Select`、`MultiSelect` | ⚠️ | 无 filter-method（只有 VirtualSelect 有）；无 prefix | p2b_select.go |
| Virtualized Select | `VirtualSelect` | ✅ | — | fill_virtual_select.go `Filterable/FilterMethod/Remote/Multiple/MaxTagCount` |
| Slider | `Slider`、`SliderRange` | ⚠️ | range 不能带 show-input；竖向不能带输入框；两柄不能交叉 | p2b_slider.go:143/315 |
| Switch | `Switch` | ⚠️ | 无 active-value/inactive-value、before-change、inline-prompt、large 尺寸、width | choice.go `Small/Loading/Texts` |
| Time Picker | `TimePicker`、`TimeRangePicker` | ⚠️ | 范围版不能键盘输入 | p2a_time_range.go |
| Time Select | `TimeSelect` | ✅ | — | more_timeselect.go `Start/End/Step/MinTime/MaxTime/Clearable` |
| Transfer | `Transfer` | ⚠️ | 无 filter-method、target-order、format；分页是自绘简易版 | p2b_transfer.go |
| TreeSelect | `TreeSelect` | ⚠️ | 无 check-strictly、default-expanded-keys、filter-node-method | entry_treeselect.go:83-102 |
| Upload | `Upload` | ⚠️ | 无 auto-upload=false + submit()、http-request、list-type=picture；照片墙无重试 | entry_upload.go:33-37 |
| **Data** | | | | |
| Avatar | `Avatar` | ✅ | — | display.go |
| Badge | `Badge` | ✅ | — | data_badge.go |
| Calendar | `Calendar` | ⚠️ | 无 `range` | data_calendar.go `elHeader` |
| Card | `Card` | ✅ | — | data_card.go `Shadow/Footer` |
| Carousel | `Carousel` | ⚠️ | 无 motion-blur、拖拽滑动 | data_carousel.go `Card/Vertical` |
| Collapse | `Collapse` | ✅ | — | data_collapse.go `Accordion/IconEnd/IconStart` |
| Descriptions | `Descriptions` | ⚠️ | 无 rowspan；列数不随断点变化 | data_card.go `DescItem{Span}`、`Column(int)` |
| Empty | `Empty` | ✅ | — | data_result.go |
| Image | `Image` | ✅ | 次要：无视口懒加载（`c.lazy` 只是延迟构建） | data_image.go |
| Infinite Scroll | `InfiniteScroll` | ✅ | — | more_infinite.go `Distance/Loading/Done/Disabled` |
| Pagination | `Pagination` | ⚠️ | 无 `layout` 自由排序、`pager-count` | nav.go（grep 为 0） |
| Progress | `Progress` | ✅ | — | data_progress.go |
| Result | `Result` | ✅ | — | data_result.go |
| Skeleton | `Skeleton` | ✅ | — | data_skeleton.go |
| Table | `Table` | ✅ | — | table*.go，另有 `HighlightCurrentRow`、`Ellipsis` |
| Virtualized Table | `Table.Virtual`、`VirtualList`、`Listy` | ⚠️ | 有行虚拟化；列虚拟化未确认 | table.go:361 `Virtual`；more_virtual.go |
| Tag | `Tag`、`CheckableTag` | ✅ | — | display.go、more_checktag.go |
| Timeline | `Timeline` | ✅ | — | data_timeline.go `TimelineItem{Time,TimeTop,Hollow,Large}` |
| Tour | `Tour` | ⚠️ | 同 Ant 那一行 | data_popover.go |
| Tree | `Tree` | ⚠️ | 无 render-content、allow-drop、accordion；无虚拟滚动 | data_tree.go |
| Virtualized Tree | `VirtualTree` | ✅ | — | fill_virtual_tree.go `Height/NodeRender/Range` |
| Statistic | `Statistic`、`Countdown` | ✅ | — | display.go、data_skeleton.go:236 |
| Segmented | `Segmented` | ⚠️ | 无 direction=vertical | entry_segmented.go |
| **Navigation** | | | | |
| Affix | `Affix` | ✅ | — | nav_affix.go |
| Anchor | `Anchor` | ✅ | — | nav_affix.go |
| Backtop | `BackTop` | ✅ | — | nav_float.go:484 |
| Breadcrumb | `Breadcrumb` | ✅ | — | nav.go |
| Dropdown | `Dropdown` | ⚠️ | 分组失效（同 Ant 那一行） | nav_dropdown.go:849-851 |
| Menu | `Menu` | ⚠️ | 弹出层里的 item-group 无标题 | nav.go:34 |
| Page Header | `PageHeader` | ✅ | — | nav_pageheader.go |
| Steps | `Steps` | ✅ | — | nav_steps.go `Type("simple")/FinishStatus` |
| Tabs | `Tabs` | ⚠️ | 无 `type="border-card"`；竖向溢出不处理；shadcn 下有退回（同 Ant 那一行） | nav.go |
| **Feedback** | | | | |
| Alert | `Alert` | ⚠️ | 无 `effect="dark"`、`center` | feedback.go:384-400 |
| Dialog | `Modal` | ✅ | before-close 靠应用控制 `open` 实现 | feedback.go |
| Drawer | `Drawer` | ⚠️ | 无 resizable、before-close | feedback.go |
| Loading | `Loading` | ✅ | — | feedback.go:294 `Tip/Background/Fullscreen` |
| Message | `Message` | ⚠️ | 无 showClose、placement（只在顶部）、offset、plain；grouping 用 `Key` 原地更新代替 | nav_notify.go:68/149 |
| Message Box | `Confirm`、`MessageBox`、`Prompt` | ⚠️ | 无 distinguishCancelAndClose、center、draggable、自定义 icon | more_confirm.go:23 |
| Notification | `Notification` | ✅ | — | nav_notify.go（四角加上下两个方位，Duration≤0 常驻） |
| Popconfirm | `Popconfirm` | ✅ | — | feedback.go |
| Popover | `Popover` | ✅ | — | data_popover.go |
| Tooltip | `Tooltip` | ✅ | — | display.go |
| **Others** | | | | |
| Divider | `Divider` | ✅ | — | display.go |
| Watermark | `Watermark` | ✅ | 次要：无 font family、textAlign | data_watermark.go |

## 4. Pro Components（28）

清单依据 procomponents v2 的知识整理（WebFetch 只拿到页面元数据）。ModalForm/DrawerForm、LoginForm/LoginFormPage 各算 1 项。RouteContext/GridContent 作为 ProLayout 的配套件合并算 1 项。

| 组件 | Tango 入口 | 状态 | 缺的能力 / 次要缺口 | 代码证据 |
|---|---|---|---|---|
| ProLayout | `ProLayout(c, ProLayoutConfig, fn)` | ⚠️ | 面包屑只能静态传入，不会按路由自动生成；无 fixedHeader/fixSiderbar、contentWidth、menuHeaderRender/headerRender 等钩子；DefaultFooter 只有一行文字（无 links/copyright 结构） | prolayout.go `ProLayoutConfig{Layout side/top/mix, Menu, Selected, Collapsed, Breadcrumb []string, Actions, Footer string, Breakpoint, NoPadding}` |
| SettingDrawer | — | ❌ | 整个缺失 | 全仓 grep `SettingDrawer` 为 0 |
| RouteContext / GridContent | — | ❌ | 只有私有的 `proLayoutCtx`；没有 Fluid/Fixed 内容宽度 | prolayout.go:383 |
| PageContainer | `PageContainer` | ✅ | — | pro2_page.go `Tabs/TabExtra/TabsCard/Extra/Breadcrumb/Back/Tags/SubTitle/Content/ExtraContent/Footer/Loading/Ghost` |
| FooterToolbar | `FooterToolbar` | ⚠️ | **无毛玻璃**：只有 96% 不透明背景加阴影，没有 backdrop blur（仓库里也没有背景模糊绘制原语） | pro2_page.go 约 92-95 行 `b.Bg(theme.Fade(th.BgContainer, 0.96))`；`Extra/Sticky/Target` |
| ProCard | `ProCard` | ⚠️ | 无 `actions` 底部操作栏（`Extra` 放在头部右侧） | pro2_card.go `Split/Direction/Gutter/Wrap/ColSpan/Tabs/Collapsible/HeaderBordered/Ghost/Centered/Loading/Hoverable` |
| StatisticCard | `StatisticCard`、`StatisticCardGroup` | ✅ | 次要：无独立的 `StatisticCard.Divider`（Group 用 Split 自动加分隔） | pro2_statistic.go `Trend/Chart/ChartPlace/Status/Icon/Desc/Footer`、:287 Group |
| CheckCard | `CheckCard`、`CheckCardGroup` | ✅ | — | pro2_checkcard.go `Avatar/Disabled/Size/Cover/Loading`；Group `Value/Values` |
| WaterMark | `Watermark` | ✅ | 次要：zIndex/fontFamily 未确认 | data_watermark.go |
| ProTable | `ProTable[T]` | ⚠️ | 无 polling；无外部 `params`（`ProRequest.Params` 只来自查询表单）；批量操作栏写死，不能自定义（`tableAlertRender`）；列状态不能受控或持久化（`columnsState`）；列 valueType 只有 11 种 | table_pro.go `Request/ProColumn{ValueType,HideInSearch,HideInTable,Fixed}/ToolBar/NoSearch/alertBar`（:807），选项区有刷新/密度/列设置/全屏 |
| EditableProTable | `EditableProTable[T]` | ✅ | — | p2e_editable.go `NewRow/AddAtTop/MaxLength/EditableKeys/Multiple/RowEditable/OnSave/OnDelete` |
| DragSortTable | `ProTable.DragSort`、`EditableProTable.DragSort` | ✅ | 以方法形式提供，没有独立入口；自定义拖拽把手未确认 | table_pro.go:197（注释 "Ant Design Pro DragSortTable"）；p2e_editable.go:157 |
| ProList | `ProList[T]` | ✅ | — | pro3_list.go `ProListMetas{Title,SubTitle,Description,Avatar,Content,Actions,Extra}/Selection/Expandable/Grid/Request/Pagination` |
| ProDescriptions | `ProDescriptions` | ✅ | — | pro3_descriptions.go `Request/Editable/ProDescItem{ValueType,ValueEnum,Span,Render}` |
| ProForm | `ProForm(c, []ProField)` | ⚠️ | 无 `readonly` 只读模式 | form_pro.go `Columns/Horizontal/Request/OnFinish/Submitter/Children/Gutter` |
| ModalForm / DrawerForm | `ModalForm`、`DrawerForm` | ✅ | — | form_pro_modal.go:32/38 |
| QueryFilter | `QueryFilter` | ✅ | — | form_pro_query.go `DefaultExpanded/Searched/Params` |
| LightFilter | `LightFilter` | ✅ | — | pro3_lightfilter.go `Collapse/Bordered/Size/Params` |
| StepsForm | `StepsForm` | ✅ | — | form_pro_steps.go |
| LoginForm / LoginFormPage | `LoginForm`、`LoginFormPage` | ✅ | — | pro3_login.go `OnSendCode/CodeInterval/AccountOnly/MobileOnly/BackgroundImage/Side` |
| ProFormList | `FormList[T]` | ✅ | 写法不同：通过 `ProForm.Children` 嵌入 | form_pro_list.go:54 `Min/Max/Creator/MoveUp/MoveDown` |
| ProFormFieldSet | `ProFormFieldSet`、`FieldSet` | ✅ | — | pro3_fieldset.go:36/55 |
| ProFormDependency | `ProField.Dependencies` + `Hide/Disable/Options` 闭包 | ✅ | 等价实现，不是组件形式 | form_pro_field.go:164-167 |
| ProFormGroup | — | ❌ | 无带标题的字段分组，只能用 Span/Columns 排栅格 | grep 为 0 |
| SchemaForm（BetaSchemaForm） | 把 `[]ProField` 传给各表单入口 | ⚠️ | 本身就是 schema 驱动，但没有 `layoutType` 统一入口；schema 里写不了 FormList/Group | ProForm/ModalForm/StepsForm/QueryFilter 都接收 `[]ProField` |
| ProForm 字段组件（ProFormText/Select…） | `ProField.ValueType` | ⚠️ | 没有独立字段组件；缺 dateWeek/Month/Quarter/Year、dateTimeRange、image、avatar、code/jsonCode、fromNow、second、index、captcha 等 valueType | form_pro_field.go:19-37（19 种）；table_pro.go:34-44（11 种） |
| ProField | `ProField` 结构体、`ProFormat()` | ⚠️ | 无可单独渲染的组件、无 mode=read/edit 切换；**货币符号是近似的**（见下） | form_pro_field.go:144；table_pro.go:993 |
| ProSkeleton | `ProSkeleton(c, kind)` | ✅ | — | pro2_skeleton.go `SkeletonList/SkeletonResult/SkeletonDescriptions` |

**货币符号（已核实仍是近似实现）**

- p2e_currency.go `p2eCurrency` 的取值顺序：显式覆盖 → `Locale.Currency` → 按语言代码猜（zh/ja 用 ¥，zh-TW 用 NT$，zh-HK 用 HK$，其余用 $）。
- `proFormatMoney`（table_pro.go:982）固定为"符号 + 千分位 + 2 位小数"。没有 ISO 货币代码，也不按地区决定小数位或符号位置。
- **另有一处不一致**：公开函数 `ProFormat(ValueMoney, …)` 写死了 `"¥"`（table_pro.go:1000），不读 Locale。

## 5. 三种风格的截图检查状态

### 5.1 画廊支持的风格

- `-theme antd`（默认）：每个页面出 Ant 亮色图，加上页面自带的 `-Element` / `-Dark` 变体（变体页 `Page.Variant = true`）。
- `-theme shadcn`：把**每个非变体页面**再以 shadcn 亮/暗各渲染一次，输出 `<Name>-shadcn.png` 和 `<Name>-shadcn-dark.png`。
- `-theme hc`：仪表盘、Form、Switch、Checkbox 在各风格下的默认版与 HighContrast 版对照。
- `-theme all` = antd + shadcn。
- 画廊首页 `gallery.Home` 能在 Ant / Element / shadcn 与暗色之间切换。
- shadcn 是**主题轮次**，不是逐页手写的变体。全仓只有 `page_shadcn.go` 一个 shadcn 专属页（ShadcnDashboard，以及 Command/Toggle/Kbd 等）。

### 5.2 本次渲染结果（2026-11-13，`-theme all -scale 1`）

- 全部成功，退出码 0，没有编译或渲染错误。
- 基础页面（非变体）共 201 个，含 Table、Image、ConfigProvider 等的子演示页。

| 维度 | 有截图的页面 | 缺的页面 |
|---|---|---|
| Ant 亮色 | 201 / 201 | 0 |
| shadcn 亮 + 暗 | 201 / 201（各 201 张） | **0**（所有页面都经主题轮次渲染） |
| Element 变体（`-Element`/`-element`） | 158 / 201 | 43 |
| 暗色变体（`-Dark`/`-dark`） | 132 / 201 | 69 |
| Element 暗色（`-element-dark`） | 34 | — |
| 移动端（`mobile/`，antd/vant × 亮/暗） | 44 张 | — |

**没有 Element 变体的页面（43）**

- 基础组件页：Alert、Breadcrumb、Button、Checkbox、Divider、Form、Icon、Input、InputNumber、Pagination、Popconfirm、Select、Switch、Table、Tabs、Tag、Tooltip、Typography。
  - 其中不少有带 Element 变体的兄弟页，例如 `TableBasic/TableFixed…-element`、`TabsMore-element`、`InputVariants-Element`、`SelectAdvanced-Element`、`InputNumberMore-Element`。总览图 `overview` 只有 Ant 和 shadcn 两版。
- shadcn 专属页：Command、Toggle、ShadcnDashboard。
- 多语言子页（9）：`ConfigProvider-{en-US,ja-JP,zh-TW}-{Data,Date,Modal}`。
- 移动端页（13）：`Mobile*`，用 antd/vant 两套令牌，本来就不按 Element 出图。

**没有暗色变体的页面（69）**

- 上面 43 页。
- 另外 26 页：BorderBeam、CheckableTag、CheckCard、ColorPickerPanel、Confirm、DatePickerPanel、FooterToolbar、IconButton、InfiniteScroll、Listy、Markdown、MarkdownEditor、Masonry、MessageBox、PageContainer、ProCard、Prompt、ProSkeleton、Radio、RichText、StatisticCard、TimePanel、TimeSelect、VirtualList、VirtualSelect、VirtualTree。
- 这些页面都有 `-shadcn-dark` 图，可以间接看暗色表现，但 Ant 暗色和 Element 暗色没有截图。

**Pro2 组件的 shadcn 检查**：之前记为"未截图检查"，本次已渲染并抽查了 ProCard、StatisticCard、PageContainer（含 FooterToolbar）、CheckCard、ProSkeleton（shadcn-dark），结果见 5.3，这个缺口可以关闭。

### 5.3 shadcn 抽查结果

抽查了以下 10 张：TabsMore、Menu、ProCard、StatisticCard、PageContainer、CheckCard、LoginForm、ProSkeleton-dark、Table-dark，以及 Calendar（只看了尺寸）。

| 图 | 结论 |
|---|---|
| TabsMore-shadcn | **有问题**：<br>① 竖向（Left/Right）和 Bottom 位置仍是 Ant 的 line/card 画法，和顶部的 shadcn segmented 风格不统一；<br>② 第 4 段"溢出滚动与「更多」"的标签条直接裁到容器外，**看不到"…"更多按钮**，这与 nav.go `showShadcn` 没有 overflow 处理的结论一致；<br>③ 可编辑卡片是 Ant 卡片式样，可以接受。 |
| Menu-shadcn | "内嵌模式 / 暗色"一栏右侧的暗色菜单**没有变暗**。这是设计取舍：nav_menu.go:104 写明 shadcn 只有一套 Sidebar 外观，`Menu.Dark()` 在 shadcn 下被忽略，需要在文档里说明。弹出子菜单正常。 |
| ProCard-shadcn | 整体正常。"默认尺寸"卡片的头部"操作"文字没有贴右，位置偏中间，是轻微对齐问题，待确认。 |
| StatisticCard-shadcn | 正常。趋势红涨绿跌，迷你图用 `--chart-1` 色。 |
| PageContainer-shadcn | 正常。页头、标签（segmented 风格）、FooterToolbar 吸底都对；FooterToolbar 没有毛玻璃（与第 4 节一致）。 |
| CheckCard-shadcn | 正常。选中态是深色边框加右上角三角标。 |
| LoginForm-shadcn | 正常。右侧表单的初始校验错误应该是画廊 Setup 触发的。 |
| ProSkeleton-shadcn-dark | 能用。页头区块是一整块浅灰，层次比 Ant 暗色更"反"（外层比卡片亮），属于 shadcn 暗色令牌本身的特点。 |
| Table-shadcn-dark | 能用。排序列有整列底色高亮（这是 Ant 的行为，shadcn 原版没有）；未勾选复选框的边框在暗色下很淡。两点都是轻微问题。 |

### 5.4 无障碍审计（`-audit`，603 次页面渲染 × 3 种风格）

- missing-name 0，no-role 0；**not-keyboard 12**，全部来自 `CarouselCard`：卡片模式下第 2 张和第 5 张卡片（group 角色）不能用键盘到达，三种风格都有。
- 对比度：6 个内置主题 0 项失败；35 项设计默认值低于 WCAG AA（设计规范本身如此，HighContrast 能修正）；24 项豁免；HighContrast 下 0 项失败。

## 6. 非组件能力

| 能力 | 状态 | 说明与证据 |
|---|---|---|
| RTL | ✅ | 基础层与组件层均已完成（2026-11-13）：`ConfigProvider.Direction`/Locale 推断、布局镜像（`layout.Physical()` 逃生口）、逻辑滚动、逻辑浮层位置、方向性图标镜像、`TextAlign` Start/End、方向键换向；全部组件已迁移（输入/选择/日期/滑块/导航/菜单/表格/树/反馈/表单/编辑器 RTL 行对齐）。时间列、数字输入、代码固定 LTR。`Driver.MirrorCheck` 与 gallery `-dir rtl`、`-mirror-check` 用于回归。原遗留项均已完成：混排文本视觉光标移动（Q12 第二期，ui/editor.go `visualStep`）；a11y 方向标记（`a11y.Node.RTL`/`Tree.RTL`，Linux AT-SPI 文本属性 `direction` 已实现，Windows UIA / macOS NSAccessibility 无对应元素属性、记为不适用，见 docs/accessibility.md）；图标未镜像检查（`paint.List.IconNote` + `Driver.MirrorCheck`/`Audit`，ui/icon_dir.go）；mobile/chart 子包 RTL（A4，ui/mobile、ui/chart 的 rtl_test.go）。 |
| 组件级主题令牌（类似 `theme.components`） | ✅ | `theme.Components` 共 54 个组件令牌类型（`theme.NumComps`，Button … Form，ui/theme/comp_*.go），全部有派生默认值，对应组件均已改读令牌，零值继承与哨兵、组件 Alias 重派生、HighContrast 覆盖组件令牌、`CSSVarsWith`；`ConfigProvider.Components/Theme/Dark…` 嵌套局部生效；默认值与原绘制逐像素一致。共享下拉面板 `entStylePanel` 读 Select 令牌（含 `PopupRadius/PopupShadow`）；Menu 弹出子菜单读 Menu 令牌自己的 `Popup*` 字段，不受 Dropdown 覆盖影响；TimeSelect 选项读 Select 令牌的 Option 字段。`Theme` 上已有组件令牌对应的扁平组件字段标注 `Deprecated:`（仍作为派生来源保留）。 |
| ConfigProvider.Defaults 接线 | ⚠️ 33/158 ≈ 21% | 见 6.1 |
| 全局尺寸 | ⚠️ | `ConfigProvider.Size` → `c.ComponentSize()` / `defaultSize()` / `sizeOr()`，约 25 个文件读取：button、choice、data_card、data_collapse、data_list、date、entry_color、entry_datepanel、entry_daterange、entry_inputx、entry_mentions、entry_otp、entry_segmented、fill_color_panel、input、more_timeselect、nav、nav_dropdown、nav_steps、p2a_time_range、pro2_card、pro2_checkcard、pro3_lightfilter、select、selector、shad_toggle、table、feedback。另有 Tag/Tabs/Pagination/Steps/Space/Form 读 `Defaults` 里的 "size"。没读的有 Slider、Checkbox（本身也没有 Size）、Cascader/TreeSelect 是否读取未确认。 |
| 主题 / 设计语言 | ✅ | `theme.AntDesign`、`theme.ElementPlus`、`theme.Shadcn`；`ConfigProvider.Flavor/PrimaryColor`；图表用 `chart.RegisterStyle` 按风格注册。 |
| 暗色 | ✅ | `theme.Dark()`、`ConfigProvider.Dark` |
| 紧凑 | ✅ | `theme.Compact()`、`ConfigProvider.Compact` |
| 高对比度 | ⚠️ | `theme.HighContrast(th)`（ui/theme/highcontrast.go）、`Theme.HighContrast` 标志，对比度审计 `theme.ContrastIssues`，画廊 `-theme hc`。**ConfigProvider 没有 HighContrast 开关**，宿主也不会跟随系统高对比度（见第 7 节）。 |
| 语言 | ✅ | 四个内置包 `ZhCN/EnUS/ZhTW/Ja`（locale_packs.go）；`ConfigProvider.Locale/LocaleCode`、`SetDefaultLocale`、`LocaleByCode`；`TestLocalePacksComplete`、i18n_wiring_test.go、i18n_scan_test.go。货币符号是近似的（第 4 节）。 |
| ReduceMotion | ✅ | `RunnerOptions.ReduceMotion` / `Runner.SetReduceMotion`；`Anim/AnimFrom` 直接到终值；装饰性动效用 `c.still()`（anim.go:69），调用方有 data_badge、data_image、data_progress、data_skeleton、nav_util、text、data_carousel。 |
| 无障碍 | ✅ | 语义树 ui/a11y、ui/a11y_tree.go；审计 `Driver.Audit()`（a11y_audit.go:102）和 gallery `-audit`；键盘漫游 `rovingTabStop/rovingStep`；`c.TrapFocus`。本次审计只剩 CarouselCard 12 处键盘不可达（5.4）。 |
| 响应式断点 | ⚠️ | `Breakpoint` XS…XXL、`ScreenBreakpoint(c)`（nav_grid.go:39）。只有 GridRow/Col（`XS…XXL`、`GutterAt`）、Masonry（`ColumnsAt`）、ProLayout/Sider（`Breakpoint`）用到。**Descriptions、List 网格、ProDescriptions、ProList 网格、QueryFilter 列数都不跟随断点。** |

### 6.1 ConfigProvider.Defaults 接线清单

- **统计方法**：grep `ConfigProp(`、`Prop(c, "X"`、`propString/propBool/propFloat/propInt64`、`p2bBool/p2bNum/p2bStr/p2bDur`、`p2bSelectDefaults`、p2a_date.go 的 `p.comp`，取出读取到的组件名。
- **分母**：158 个公开入口。即 `func Xxx(c *Context…)` 加上 ProTable/EditableProTable/ProList，去掉 Row/Column/Div/StackBox/Custom/Spacer/Gap/Key/Disabled/Overlay/HoverDelay/SetHoverOpen/ScreenBreakpoint/OpenDialogs/MessageDestroy/NotificationDestroy/ConfigProvider 这些基础件。

**已接线（33）**

Alert、Avatar、Breadcrumb、Carousel、Cascader、DatePicker、DateTimePicker、Divider、Form、GridRow、Input、InputNumber、Menu、Modal、MultiSelect（读 "Select" 键）、Pagination、Popconfirm、ProLayout、RangePicker、Select、Sider、Slider、Space、Steps、Tabs、Tag、TimePicker、TimeRangePicker、Tooltip、Transfer、TreeSelect、Upload、VirtualSelect（读 "Select" 键）。

**未接线（125）**

- 通用 / 布局：Addon、Affix、BackTop、BorderBeam、Button、Col、CollapsibleSider、Compact、Content、Flex、FloatButton、FloatButtonGroup、Footer、Header、Icon、IconButton、Kbd、Layout、Masonry、Scroll、ScrollH、Splitter、SplitterPanel、Toggle、ToggleGroup、VDivider。
- 导航：Anchor、AnchorSection、BreadcrumbItems、Command、Dropdown、PageHeader。
- 录入：AutoComplete、CheckAll、Checkbox、CheckboxGroup、CheckCard、CheckCardGroup、ColorPicker、ColorPickerPanel、DatePickerPanel、DropZone、InputTag、Mentions、OTP、Radio、RadioGroup、Rate、Search、Segmented、SliderRange、Switch、TextArea、TimePanel、TimeSelect、MarkdownEditor。
- 展示：AvatarGroup、Badge、BadgeRibbon、Calendar、Card、CardAction、CardMeta、CheckableTag、Collapse、Countdown、Descriptions、Empty、Image、InfiniteScroll、Link、List、ListActions、ListItemMeta、Listy、Markdown、Paragraph、Popover、Progress、QRCode、Result、RichText、Skeleton、SkeletonAvatar、SkeletonButton、SkeletonImage、SkeletonInput、Statistic、Table、Text、Timeline、Title、Tour、Tree、VirtualList、VirtualTree、Watermark。
- 反馈：Confirm、Drawer、Loading、Message、MessageBox、Notification、Prompt、Spin。
- Pro：DrawerForm、EditableProTable、FieldSet、FooterToolbar、LightFilter、LoginForm、LoginFormPage、ModalForm、PageContainer、ProCard、ProDescriptions、ProForm、ProFormFieldSet、ProList、ProSkeleton、ProTable、QueryFilter、StatisticCard、StatisticCardGroup、StepsForm。

**说明**

- 未接线的组件里，有些通过全局 Size 间接受 ConfigProvider 影响，例如 Button、Checkbox/Radio（choice.go）、Card、Collapse、List、Table、Segmented、OTP。
- SliderRange 与 Slider 共用实现，TextArea/Search 与 Input 是否共用 "Input" 键未确认，所以这几个按"未接线"计。
- **最值得补的**：Button（autoInsertSpace/shape）、Table（size/bordered/pagination）、Card、Drawer、Message/Notification（maxCount/placement/duration）、Checkbox/Radio/Switch、Empty（全局空状态渲染，相当于 Ant 的 `renderEmpty`）、ProTable。

### 6.2 子包简述（不计入三家对标）

- `ui/chart`：Line/Area、Column/Bar、Pie/Donut、Scatter/Bubble、Radar、Gauge、Funnel、Heatmap、DualAxes、Tiny 系列、Liquid。三种风格都有样式（chart/style_shadcn.go），有无障碍测试（chart/a11y_chart_test.go）。
- `ui/mobile`：对标 Ant Design Mobile / Vant 两套令牌，共 61 个入口，截图 44 张（antd/vant × 亮/暗）。
- `ui/editor`：代码编辑器（buffer、monarch 高亮、IME、gutter）。规格与进度见 docs/specs/code-editor*.md，不属于组件库对标范围。

## 7. 已知的平台层与非 UI 缺口（来自项目记录，未做代码核实）

- 平台层没有 DamagePresenter（局部重绘 / 增量上屏还没接到各平台呈现器）。
- Windows 和 Linux 上的触屏输入没有接入。
- uihost 不会跟随系统高对比度（`theme.HighContrast` 已有，但宿主不会按系统设置自动切换）。
- Windows 的 AppAppearance（跟随系统亮暗、强调色）没有实现。
- Windows 和 Linux 没有在真机上验证过。

## 8. 建议的补齐优先级

工作量：小 ≤ 1 天，中 2–4 天，大 ≥ 1 周。

| 优先级 | 项 | 工作量 | 理由 |
|---|---|---|---|
| P0 | Dropdown 支持 `MenuItem.Group`（复用 `menuFlatGroups`）；Menu/Dropdown 弹出层的分组画标题（nav_menu.go 已有 inline `menu-group-title` 可以复用） | 小 | 行为错误：分组被当成子菜单，影响所有用 Dropdown 的菜单 |
| P0 | Tabs：shadcn 横向和所有竖向 Tabs 的溢出加"…"更多下拉；shadcn 下 Card/Closable/OnAdd/竖向给出统一画法 | 中 | 截图已证实溢出时标签被直接裁掉 |
| P0 | Link 的 `Type` 着色生效 | 小 | 修饰器静默无效 |
| P0 | `ProFormat(ValueMoney)` 改为读 Locale/Currency，不再写死 "¥"；货币格式改成 ISO 代码加地区规则 | 小（前者）/ 中（后者） | 同一个值在不同 API 下显示不一致 |
| P0 | CarouselCard 的卡片键盘可达 | 小 | 审计中唯一剩下的键盘问题 |
| P1 | RangePicker 支持 week/month/quarter/year 粒度（面板 `DatePickerPanel.Range().Picker()` 已有，接到触发器上）；DatePicker/RangePicker/TimeRangePicker 支持键盘输入（复用 DateTimePicker 的 `dtParse` 和输入框） | 中 | 中后台高频需求 |
| P1 | Descriptions、List 网格、ProDescriptions、ProList、QueryFilter 的列数跟随断点（参照 `Masonry.ColumnsAt`） | 小 | 已有断点基础设施 |
| P1 | Form 按字段校验、清除（validateFields(names)/clearValidate）；RulesFunc 支持 reset；表单级 Disabled | 中 | Form 是核心组件 |
| P1 | TreeSelect 受控展开；勾选策略改为真正影响绑定值（Cascader 同理）；Cascader 懒加载父节点不再按叶子处理 | 中 | 语义与 Ant 不一致，容易在数据层出错 |
| P1 | ConfigProvider：给 Button/Table/Card/Drawer/Message/Notification/Checkbox/Radio/Switch/Empty/ProTable 补 Defaults 接线；加 HighContrast 开关 | 中 | 接线率只有 21% |
| P1 | ProTable：外部 params、polling、可自定义批量操作栏、受控/持久化 columnsState | 中 | Pro 中使用最多的组件 |
| P1 | Slider 范围柄可交叉/pushable；WithInput 支持范围与竖向 | 小 | 已知缺口 |
| P1 | Variant（filled/borderless/underlined）推广到 Select/Cascader/TreeSelect/DatePicker/TimePicker/AutoComplete/Mentions/TextArea | 中 | v6 的统一外观能力，现在只有 Input/InputNumber 有 |
| P2 | ColorPicker 渐变与 hsb；Upload 照片墙重试、listType=picture、手动 submit；Transfer 改用 Pagination | 中 | 已知缺口 |
| P2 | Tree 虚拟滚动（或在 Tree 内部委托给 VirtualTree）以及 titleRender/allowDrop | 中 | 已有 VirtualTree 可以先顶用 |
| P2 | v6 新模式：Timeline 横向、Segmented 竖向/round、Steps inline/panel、Drawer resizable、Button color×variant、Splitter lazy、Carousel 拖拽、Statistic.Timer | 中（合计） | 跟进 v6 |
| P2 | Element 专有：Button success/warning/info/plain + ButtonGroup、Switch active-value/inline-prompt、Alert effect=dark、Pagination layout、Tabs border-card、Calendar range、CascaderPanel | 中（合计） | 提高 Element 风格的完整度 |
| P2 | Pro：SettingDrawer、ProFormGroup、ProCard actions、ProForm readonly、ProField 独立组件与 read/edit 模式、FooterToolbar 毛玻璃（需要先给 paint 加背景模糊原语） | 大（合计） | 缺失项和次要项 |
| P2 | 组件级主题令牌（ConfigProvider 局部 Theme 或 `Components` 覆盖表） | 大 | 已完成（2026-11-13，54 个组件令牌，见第 6 节） |
| P2 | RTL：布局逻辑方向（start/end）、镜像、ConfigProvider.Direction，再逐个组件改图标和箭头方向 | 大 | 已完成（2026-11-13，含 mobile/chart，见第 6 节） |
| P2 | 画廊：给 43 个缺 Element 变体、69 个缺暗色变体的页面补变体（至少覆盖 Button/Input/Select/Table/Tabs/Form/Checkbox/Switch 主页面） | 小–中 | 方便做视觉回归 |
