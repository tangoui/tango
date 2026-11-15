# Tango UI 组件覆盖率扫描

> 扫描日期：2026-11-13
> 复核：2026-11-13 晚，按当前代码逐行更新（Splitter/Dropdown/Menu/Tabs/Form/Slider/TimePicker/Transfer/Carousel/Descriptions/List/Tree/Drawer 等缺口已补，App、SettingDrawer、RouteContext/GridContent、ProFormGroup、CascaderPanel 已有实现，ProTable 补齐 params/polling/columnsState/alert 自定义，货币改为 ISO 代码；Defaults 接线重算为 106/164；计数见 1.2）
> 稳定性验证：2026-11-14，HEAD `f3ba799`，Windows 实机；测试、截图、无障碍与镜像检查全部通过（见 1.2 末尾与第 5 节）
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

| 对标 | 总数 | ✅ 完整 | ⚠️ 部分 | ❌ 缺失 | 补齐前（2026-11-13 复核） |
|---|---|---|---|---|---|
| Ant Design v6 | 72 | 72 | 0 | 0 | 72：56 / 16 / 0 |
| Element Plus | 79 | 79 | 0 | 0 | 79：59 / 20 / 0 |
| Pro Components | 28 | 28 | 0 | 0 | 28：21 / 7 / 0 |

2026-11-14 补齐：剩下的"部分"是 ConfigProvider（`Defaults` 接线，Ant、Element 各一行）和 FooterToolbar（毛玻璃，需要渲染层的背景模糊）。另按 shadcn/ui 新增 Menubar、AspectRatio、Item。同日 groupG 补完 `Defaults` 接线（141 已接线 / 36 不适用 / 0 未接线，见 6.1）并加上 Element 的 button.autoInsertSpace、message.max，ConfigProvider 两行改为 ✅；只剩 FooterToolbar 一项"部分"。

**稳定性验证（2026-11-14，HEAD `f3ba799`，Windows）**：上表全部 ✅ 组件（含当天补齐的 Button/ButtonGroup、Steps、Segmented、Timeline、DatePicker/MultiDatePicker、ColorPicker 渐变、Statistic/StatisticTimer、Switch、Checkbox/Radio、AutoComplete、Mentions、Select、Cascader、TreeSelect、Upload、Card、Tour、Message、Pro 系列，以及 Menubar、AspectRatio、Item）判定为**稳定**：

- `go test -count=1 ./ui/...` 连跑 3 轮，0 失败（无抖动）。
- 画廊 `-theme all -scale 1` 退出码 0，231 个基础页面在 Ant 亮/暗、Element 亮/暗、shadcn 亮/暗六种渲染下均有截图，无缺页（见 5.2）；抽看 ButtonGroup-element-dark、Menubar-shadcn 正常。
- `-audit` 全部为 0（见 5.4）；`-mirror-check` 只剩 Mentions 的 2 处已知项。

2026-11-14 续：渲染层加了背景模糊原语 `paint.BackdropBlur`（CPU 与 GPU 同一算法），FooterToolbar 吸底时改为毛玻璃，Pro Components 的"部分"清零（见 1.3 FooterToolbar 行）。

### 1.3 与上次扫描对比

以下是复核前那次扫描与更早一次的对比（更早一次：Ant 72：46 / 23 / 3，Element 79：53 / 22 / 4，Pro 约 22 个、缺 9）。

- **缺失项大幅减少。**
  - Ant：v6 新组件 Masonry、Listy、BorderBeam 都已有实现；复核时 `App` 也已实现（pro4_app.go），没有缺失项。
  - Element：Input OTP、Virtualized Select/Tree、DatePickerPanel、ColorPickerPanel 都已有实现，没有缺失项；复核时 `CascaderPanel` 也已实现（entry_cascader_panel.go）。
  - Pro：补了 pro2 / pro3 两批，包括 PageContainer、FooterToolbar、ProCard、StatisticCard、CheckCard、ProSkeleton、ProDescriptions、ProList、LightFilter、ProFormFieldSet、LoginForm。缺失项从 9 个降到 3 个；复核时 pro4 批次（SettingDrawer、ProFormGroup、RouteContext/GridContent）补齐了这 3 个。
- **"完整"变少、"部分"变多，主要是本次口径更严，不是能力倒退。**
  - 本次逐个对照了 v6 新增的模式：Timeline 横向、Segmented 竖向、Steps inline/panel、Drawer resizable、Button color×variant。
  - 本次也把 Dropdown 分组失效、Link type 着色无效、shadcn Tabs 无溢出处理等新发现的缺口计入了"部分"。
- **已知缺口的复核结果**：用户列出的已知缺口，除"Pro2 组件 shadcn 下未截图检查"外**全部仍存在**，证据见各表和第 8 节。Pro2 那一项本次已通过 shadcn 渲染和抽查补上，见第 5 节。
  - 晚间按代码复核：Dropdown 分组、Link 着色、Tabs shadcn/溢出、ProFormat 货币、CarouselCard 键盘、RangePicker 粒度与键盘输入、Form 按字段校验、TreeSelect 受控展开等已补，剩余项见第 8 节。复核先只读代码；之后重跑了截图（`-theme all`）与 `-audit`，第 5 节已按新结果更新。

## 2. Ant Design v6（72）

| 组件 | Tango 入口 | 状态 | 缺的能力 / 次要缺口 | 代码证据 |
|---|---|---|---|---|
| **通用** | | | | |
| Button | `Button`、`IconButton`、`ButtonGroup` | ✅ | — | button.go `Color(ButtonColor)` × `Appearance(ButtonAppearance)`：default/primary/danger/success/warning/info + 13 个预设色（及 hex）× solid/outlined/dashed/filled/text/link/plain，原 `Primary/Dashed/Text/Link/Secondary/Danger/Ghost` 映射到同一组合且外观不变；button_group.go `ButtonGroup`（`Size/Color/Appearance/Variant/Vertical`） |
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
| Splitter | `Splitter`、`SplitterPanel` | ✅ | 次要：不能自定义折叠图标 | nav_splitter.go `SplitterW.Lazy`（拖拽只画预览线，松手再提交）、`DragStarted/DragEnded`；`SplitterPanelW.Collapsible`（两侧箭头）、`CollapseToggle(collapseTip, expandTip)`（单箭头只折叠本侧，相当于分 start/end）；`Resizable/Min/Max/Changed` |
| **导航** | | | | |
| Anchor | `Anchor`、`AnchorSection` | ✅ | — | nav_affix.go `Horizontal/TargetOffset/NoInk`、`AnchorItem.Children` |
| Breadcrumb | `Breadcrumb`、`BreadcrumbItems` | ✅ | — | nav.go `BreadcrumbItem.Menu/Href/Icon`、`SeparatorIcon` |
| Dropdown | `Dropdown` | ✅ | — | nav_dropdown.go `navMenuPopup.items` 对 `it.Group` 调 `group`：画分组标题（`groupTitle`，按 Ant/Element/shadcn 取样式）、按风格加分隔线（`groupSeps`），分组是带名字的 group 节点；`estimate` 也计入标题高度 |
| Menu | `Menu` | ✅ | 次要：横向菜单根层与"…"溢出项里的分组仍被拍平成 Divider（`menuFlatGroups`） | 弹出子菜单与折叠态复用 `navMenuPopup`（nav_menu.go `pop := &navMenuPopup{…}`），分组有标题；inline 标题见 nav_menu.go `menu-group-title`；nav.go `menuFlatGroups` 只用于 `fitHorizontal` |
| Pagination | `Pagination` | ✅ | 次要：无 `itemRender` | nav.go `ShowTotal/ShowQuickJumper/Simple/SizeChanger/HideOnSinglePage`；`Layout(PaginationPart...)` 自由排序、`PagerCount(n)`（groupD 补） |
| Steps | `Steps` | ✅ | — | nav_steps.go `Type("inline"/"panel")`、`Inline()/Panel()`；`DotRender(func(*paint.List, StepDot))` 自定义点（拿到 `Index/Status/Rect/Color/Default`）；另有 `Percent/Dot/LabelVertical` |
| Tabs | `Tabs` | ✅ | 次要：不能拖拽排序；竖向 Tabs 不设 `Height` 时随标签增高，不出"…" | nav.go `bar` 里 shadcn 横竖向都走 `shadTrack`（Card 为描边轨道），`addButton`/`tab` 有 shadcn 分支；`scroller` 横向与设了 `Height` 的竖向都调 `overflow/moreMenu`；`ExtraLeft`（左侧 extra）、`BorderCard`、`Centered`、`TabBarStyle` |
| **数据录入** | | | | |
| AutoComplete | `AutoComplete` | ✅ | — | entry_autocomplete.go `Variant(FieldVariant)`（四种变体，复用 Input.Variant）、`TriggerOnFocus`、`Source(func(q) []Option)`、`Remote/Debounce/Loading`（异步远程搜索，与 Select 共用 remote_search.go）、`AllowClear/Status/Prefix/Highlight` |
| Cascader | `Cascader` | ✅ | — | entry_cascader.go `Variant`、`FilterMethod(func(q string, path []CascaderOption) bool)`、`DisplayRender(func(labels []string, path []CascaderOption) string)`；`leaves` 注释：未加载的懒节点代表整棵子树，加载后把勾选交给子节点；p2b_cascader.go `LoadData`（`done(nil)` 才标为叶子）、`ShowCheckedStrategy`、`StrategyValue`（绑定值随策略）、`Searchable` |
| Checkbox | `Checkbox`、`CheckboxGroup`、`CheckAll` | ✅ | — | choice.go `Indeterminate/Disabled/Size/Border/Button`；`CheckboxGroup` 返回 `*CheckboxGroupW`（`Changed/Disabled/Min/Max/Border/Buttons/Size/A11yLabel`） |
| ColorPicker | `ColorPicker`、`ColorPickerPanel`、`ColorPickerGradient`、`ColorPickerPanelGradient` | ✅ | 次要：单色与渐变是两个入口，面板内没有 single/gradient 模式切换 | entry_color.go `ColorPickerGradient(c, *theme.Gradient)`（色标条：点击添加、拖动移动、拖离或 Delete 删除；角度输入；下方编辑选中色标）、`Trigger(TriggerHover)`；fill_color_panel.go `ColorPickerPanelGradient`；theme/color.go `Gradient/GradientStop/ParseGradient/CSS/At/Sorted`；`cpFormatOK` 接受 hex/rgb/hsb/hsv；`PresetGroups` |
| DatePicker | `DatePicker`、`MultiDatePicker`、`DateTimePicker`、`RangePicker`、`DatePickerPanel` | ✅ | 次要：MultiDatePicker 无 Variant、不可键盘输入 | multiple：date_multi.go `MultiDatePicker`（标签显示、`MaxTagCount`、`NeedConfirm`）；needConfirm：date.go `DatePickerW.NeedConfirm`；disabledTime：entry_datetime.go `DateTimePickerW.DisabledTime`、`DisabledTimes`，entry_daterange.go `RangePickerW.DisabledTime`（ShowTime 列）；entry_datetime.go `DateTimePickerW.DisabledDate/Presets`；entry_daterange.go `Picker`（周/月/季/年）、`Variant`；可键盘输入：date.go `dtTyping`、entry_daterange.go `typed`、p2a_date.go `p2aParsePeriod`；测试 date_more_test.go |
| Form | `Form`/`FormItem`、`FormList` | ✅ | 次要：无 labelCol/wrapperCol（只有 LabelWidth） | p2d_form.go `ValidateFields(names…)`、`ClearValidate(names…)`、`Disabled`（表单级）、`RequiredMark`、`ResetFields(names…)`、`FormItemW.OnReset`（RulesFunc 项由回调重置）；form.go `Validate/ResetValidation/ScrollToFirstError` |
| Input | `Input`、`TextArea`、`Search`、`OTP` | ✅ | 次要：TextArea/Search/OTP 无 Variant；OTP 无 separator/formatter | input.go `AllowClear/Password/Prefix/Suffix/ShowCount/MaxLength/Status`；p2d_input.go `Variant`（4 种） |
| InputNumber | `InputNumber` | ✅ | 次要：无 suffix；不能隐藏控制按钮 | input.go `ControlsPosition`（right/sides）、`Variant`、`Wheel` |
| Mentions | `Mentions` | ✅ | — | entry_mentions.go `Variant`、`AllowClear/Cleared`、`Whole/WholeCheck`、`Remote(func(prefix, query))/Debounce/Loading`、`Query()`、`Prefix/Rows/Selected…` |
| Radio | `Radio`、`RadioGroup` | ✅ | — | choice.go `Buttons/Solid/Size/Changed/Disabled/Border`；`Radio.Border/Size` |
| Rate | `Rate` | ✅ | — | entry_rate.go `AllowHalf/AllowClear/Character/Count/Texts` |
| Select | `Select`、`MultiSelect`、`InputTag`、`VirtualSelect` | ✅ | 次要：VirtualSelect / InputTag 未加 Variant | select.go `FieldVariant`、`SelectW.Variant/Prefix/PrefixText/SuffixIcon`；entry_multiselect.go 同名方法（selector.go 外壳）；p2b_select.go `Remote/FilterMethod/AllowCreate/Virtual/MaxCount`；fill_virtual_select.go `FilterMethod` |
| Slider | `Slider`、`SliderRange` | ✅ | 次要：无 reverse/included | p2b_slider.go `AllowCross`（默认可交叉）、`Pushable(gap)`；`WithInput` 支持范围与竖向（注释写明各输入框位置） |
| Switch | `Switch`、`SwitchValue` | ✅ | — | choice.go `Small/Large/Size/Width/Loading/Texts/Icons/OuterTexts/BeforeChange/BeforeChangeAsync` |
| TimePicker | `TimePicker`、`TimePanel`、`TimeRangePicker` | ✅ | 次要：无 needConfirm | p2a_time_range.go `TimeRangePicker` 注释"Each end can also be typed"、`typed`；`HideDisabledOptions`（TimePicker、TimeRangePicker、TimePanel 都有） |
| Transfer | `Transfer` | ✅ | 次要：无自定义 filterOption、footer | p2b_transfer.go `pager` 改用 `Pagination(...).Simple().Small()`；`Render/OneWay/TreeSource/Virtual` |
| TreeSelect | `TreeSelect` | ✅ | — | entry_treeselect.go `CheckStrictly()`、`FilterTreeNode(func(q string, n TreeSelectNode) bool)`、`Variant`；p2b_treeselect.go `ExpandedKeys(*[]string)`（受控展开）/`ExpandChanged`、`StrategyValue`（绑定值随勾选策略）、`LoadData`、`Virtual` |
| Upload | `Upload`、`DropZone` | ✅ | — | entry_upload.go `upText/upCard/upDrag/upPicture`；`PictureList()`/`ListType(UploadListPicture)` 带缩略图列表（`pictureItem`，令牌 `PictureItem*`）；p2d_upload.go `AutoUpload(false)` 排队为 "ready"、`Submit()` 上传；`CustomRequest`、`BeforeUpload/Action/Retries`；测试 groupd_upload_test.go |
| **数据展示** | | | | |
| Avatar | `Avatar`、`AvatarGroup` | ✅ | — | display.go `Size/Square/Image/Icon`；data_badge.go `AvatarGroupW.Max` |
| Badge | `Badge`、`BadgeRibbon` | ✅ | — | data_badge.go `Count/Dot/Max/ShowZero/Status/Offset` |
| Calendar | `Calendar` | ✅ | 次要：无 headerRender、validRange | data_calendar.go `Card/Mode/CellRender/MonthCellRender/NoHeader` |
| Card | `Card`、`CardMeta`、`CardAction` | ✅ | — | data_card.go `Extra/Cover/Actions/Inner/Loading`；`Tabs(&active, items)` 标题栏下方页签（正文作为 tabpanel）、`TabBarExtra`、`TabChanged`；测试 groupd_card_test.go |
| Carousel | `Carousel` | ✅ | 次要：无 dot 进度条 | data_carousel.go `Draggable`（拖拽/滑动翻页）、`SwipeThreshold`、`Loop(false)`；`Fade/Vertical/DotPosition/Autoplay` |
| Collapse | `Collapse` | ✅ | 次要：无 `collapsible="header"/"icon"` | data_collapse.go `Accordion`、`CollapseItem{Disabled,NoArrow,Extra}` |
| Descriptions | `Descriptions` | ✅ | — | data_card.go `ColumnAt(bp, n)`、`Columns(map[Breakpoint]int)`，按 data_util.go `dataRespCols.resolve` 取断点 |
| Empty | `Empty` | ✅ | — | data_result.go `Simple/Image/ImageSize/Description` |
| Image | `Image` | ✅ | 次要：无 toolbarRender/imageRender | data_image.go `PreviewGroup`，带计数、旋转、翻转、缩放 |
| List（v6 已 deprecated） | `List`、`ListItemMeta`、`ListActions` | ✅ | — | data_list.go `GridAt(bp, columns)`（断点列数）、`Grid(columns, gap)` |
| Listy | `Listy` | ✅ | — | fill_listy.go `Virtual/Group/Sticky/ScrollToKey/ScrollToGroup/EndReached` |
| Popover | `Popover` | ✅ | — | data_popover.go `Title/Content/Trigger/Placement/Open` |
| QRCode | `QRCode` | ✅ | — | data_qrcode.go `Level/Icon/Status/Refreshed/Bordered` |
| Segmented | `Segmented` | ✅ | — | entry_segmented.go `Vertical()`（上下键切换）、`Shape(SegmentedShapeRound)`；另有 `Icons/Block/Size/Disabled` |
| Statistic | `Statistic`、`Countdown`、`StatisticTimer` | ✅ | — | display.go `Statistic`；data_skeleton.go `StatisticTimer(c, title, TimerCountdown/TimerCountup, at)`（正计时）、`CountdownW.OnChange`、`Format/Prefix/Suffix/Finished`；测试 groupd_timer_test.go |
| Table | `Table`（及 `ProTable`、`EditableProTable`） | ✅ | 次要：汇总格只能返回字符串 | table*.go `Sortable/MultiSort/Filters/FilterPanel/Fixed/ScrollY/Sticky/Expandable/Tree/TreeLazy/Selection/CheckStrictly/Span/Summary/Virtual/Resizable/TableEditor/ServerPagination/Children/DragSort/Wrap`；列虚拟化已确认（table_show.go `culled`，table_colvirt_test.go） |
| Tag | `Tag`、`CheckableTag` | ✅ | — | display.go `Closable/Color/Icon/Borderless`；more_checktag.go |
| Timeline | `Timeline` | ✅ | — | data_timeline.go `Horizontal()`，`Mode` left/right/alternate 在横向下表示轴下/轴上/交替，Label 在对侧 |
| Tooltip | `Tooltip` | ✅ | — | display.go `Color/Arrow/MouseEnterDelay/Trigger` |
| Tour | `Tour` | ✅ | — | data_popover.go `TourStep{…,Mask,Type,PrevText,NextText,HidePrev,HideNext}`；`TourW.IndicatorsRender/ActionsRender(TourActions)`；切换步骤时自动把目标滚进可见区（`ScrollIntoView(bool)`，默认开；原先没有）；`Primary/NoMask/NoArrow/Gap`；测试 groupd_tour_test.go |
| Tree | `Tree`（另有 `VirtualTree`） | ✅ | 次要：无 titleRender、allowDrop | data_tree.go `Height(px)`：注释"builds only the rows in view (Ant height / virtual)"；`Draggable/Dropped/LoadData/Search/Filter/CheckStrictly` |
| **反馈** | | | | |
| Alert | `Alert` | ✅ | 次要：不能自定义图标 | feedback.go `Action/Description/NoIcon/Closable/Banner` |
| Drawer | `Drawer` | ✅ | — | feedback.go `Resizable`（边缘可拖，也是可聚焦的分隔条）、`SizeLimits/BindSize/Resized`；嵌套 Drawer 推开父级（`push` 动画，注释"pushes the parent aside"） |
| Message | `Message`、`MessageDestroy` | ✅ | 次要：maxCount 只在 `App` 内（`UseApp().Message`）生效 | nav_notify.go `MessageRef.Placement(MessagePlacement)`（顶部/底部 × 起点/居中/终点）、`Offset`、`ShowClose`、`Plain`、`Key/Duration`；pro4_app.go `AppMessageConfig.MaxCount` |
| Modal | `Modal`、`Confirm`、`MessageBox`、`Prompt` | ✅ | 次要：即时模式下用不上 useModal；ConfirmOptions 不能自定义 footer | feedback.go `Draggable/Fullscreen/ConfirmLoading/Centered`；more_confirm.go `ConfirmOptions{…,Icon,Center,Draggable,DistinguishCancelAndClose,OnClose}` |
| Notification | `Notification`、`NotificationDestroy` | ✅ | 次要：maxCount 只在 `App` 内生效；stack 阈值只能用全局变量 | nav_notify.go 六个方位；`NotifyStackThreshold`；pro4_app.go `AppNotificationConfig.MaxCount` |
| Popconfirm | `Popconfirm` | ✅ | — | feedback.go `OkDanger/ShowCancel/Icon/Description` |
| Progress | `Progress` | ✅ | — | data_progress.go `Circle/Dashboard/Steps/Gradient/Success/Format` |
| Result | `Result` | ✅ | — | data_result.go |
| Skeleton | `Skeleton` + `SkeletonAvatar/Button/Input/Image` | ✅ | — | data_skeleton.go |
| Spin | `Spin`、`Loading` | ✅ | — | feedback.go `Delay/Percent/Wrap/Tip`、`Loading.Fullscreen` |
| Watermark | `Watermark` | ✅ | 次要：无 fontFamily/fontStyle、textAlign、inherit | data_watermark.go `Rotate/Gap/Offset/FontSize/Image/Grid` |
| **其他** | | | | |
| Affix | `Affix` | ✅ | — | nav_affix.go `OffsetTop/OffsetBottom/Target` |
| App | `App`、`UseApp` | ✅ | 次要：不做基础文本样式重置（Text 本来就取主题色和字体） | pro4_app.go `AppW.Message(AppMessageConfig{Duration,MaxCount})/Notification(AppNotificationConfig{Placement,Duration,MaxCount,ShowProgress})/Body`；`UseApp` 返回 `AppAPI.Message/Notification/Confirm/Info` |
| BorderBeam | `BorderBeam` | ✅ | — | fill_border_beam.go |
| ConfigProvider | `ConfigProvider` | ✅ | `Components`（组件令牌）与 `Direction`（RTL）已接入（见 rtl-and-component-tokens.md）；`Defaults` 接线 141/177 个公开入口，其余 36 个逐个说明为不适用，未接线 0（见 6.1）；全局 `button.autoInsertSpace` 对应 `Defaults("Button", {"autoInsertSpace"})` / `ButtonW.AutoInsertSpace` | nav_config.go `Size/Locale/Defaults/ComponentDisabled/Theme/Flavor/Dark/PrimaryColor/Compact/HighContrast`；ui/defaults_more.go；ui/defaults_more_test.go |

## 3. Element Plus（79）

不计入的设计规范页：Border、Color、Typography（Typography 页的字体规范由 `theme` 令牌与 `Text/Title` 体现）。

| 组件 | Tango 入口 | 状态 | 缺的能力 / 次要缺口 | 代码证据 |
|---|---|---|---|---|
| **Basic** | | | | |
| Button | `Button`、`ButtonGroup` | ✅ | — | button.go `Success/Warning/Info()`、`Plain()`（可与所有颜色组合）；button_group.go `ButtonGroup`（统一 `Size`/`Variant`/`Color`，`Vertical`） |
| Layout Container | `Layout/Header/Sider/Content/Footer` | ✅ | — | prolayout.go |
| Icon | `Icon` | ✅ | — | text.go `IconW` |
| Layout（row/col） | `GridRow`、`Col` | ✅ | — | nav_grid.go |
| Link | `Link` | ✅ | 次要：无 icon | text.go `LinkW.colors` 按 `Type` 取 success/warning/danger/info 调色板（悬停、按下随之变化），`Color` 可固定颜色 |
| Text | `Text` | ✅ | — | text.go `Type/Size/Ellipsis/Lines` |
| Scrollbar | `Scroll`、`ScrollH` | ✅ | — | scroll.go `Always/Both/ScrollTo/Offset` |
| Space | `Space` | ✅ | — | nav_space.go `Split/Wrap/Size` |
| Splitter | `Splitter` | ✅ | — | nav_splitter.go `Lazy`、`CollapseToggle`（单方向折叠） |
| **Configuration** | | | | |
| Config Provider | `ConfigProvider` | ✅ | 同 Ant 那一行；Element 的 button.autoInsertSpace → `Defaults("Button", {"autoInsertSpace": true})`（Element 默认 false，本仓库默认也关）或 `ButtonW.AutoInsertSpace`；message.max → `Defaults("Message", {"maxCount": n})` | nav_config.go；ui/defaults_more.go `messageMaxCount`；button.go `shownLabel`；ui/defaults_more_test.go `TestButtonAutoInsertSpaceRules`、`TestMessageMaxCount` |
| **Form** | | | | |
| Autocomplete | `AutoComplete` | ✅ | — | entry_autocomplete.go `TriggerOnFocus()`（异步取数用 `Remote/Debounce/Loading`，聚焦时立即调用一次） |
| Cascader | `Cascader`、`CascaderPanel` | ✅ | — | entry_cascader.go `FilterMethod`（filter-method）、`DisplayRender`（覆盖 show-all-levels/separator）；entry_cascader_panel.go `CascaderPanel`（内嵌面板，`Multiple/LoadData/ShowCheckedStrategy/StrategyValue/Height/Border`）；懒加载与勾选策略见 Ant 那一行 |
| Checkbox | `Checkbox`、`CheckboxGroup` | ✅ | — | choice.go `CheckboxW.Button`（checkbox-button）、`Border`、`Size`；`CheckboxGroup(...).Min/Max/Disabled/Buttons/Border/Size`；choice_test.go `TestCheckboxGroupMinMax/TestCheckboxBorderButtonSize` |
| Color Picker Panel | `ColorPickerPanel` | ✅ | — | fill_color_panel.go `Presets/PresetGroups/Format/NoAlpha/AllowClear/Border` |
| Color Picker | `ColorPicker` | ✅ | 次要：无 hsl 格式 | entry_color.go `cpFormatOK`（hex/rgb/hsb/hsv） |
| Date Picker Panel | `DatePickerPanel` | ✅ | — | fill_date_panel.go `PickerType`（日/周/月/季/年）、`Range` |
| Date Picker | `DatePicker`、`MultiDatePicker`、`RangePicker` | ✅ | — | type=dates/months/years：date_multi.go `MultiDatePicker(...).Picker(PickerMonth/PickerYear)`；entry_daterange.go `Picker`（monthrange/yearrange）；可键盘输入，同 Ant DatePicker 那一行 |
| DateTime Picker | `DateTimePicker`、`RangePicker.ShowTime` | ✅ | — | entry_datetime.go `DateTimePickerW.DisabledDate/Presets/DisabledTime`；entry_daterange.go `ShowTime`、`DisabledTime` |
| Form | `Form` | ✅ | 次要：只有校验失败时滚动（`ScrollToFirstError`），无按字段 scrollToField | p2d_form.go `ValidateFields/ClearValidate/RequiredMark/Disabled/ResetFields`、`FormItemW.OnReset` |
| Input | `Input`、`TextArea` | ✅ | — | input.go、entry_inputx.go |
| Input Number | `InputNumber` | ✅ | 次要：不能 `controls=false` | input.go:719 `ControlsPosition` |
| Input Tag | `InputTag` | ✅ | 次要：拖动排序只支持指针，没有键盘替代 | entry_inputtag.go `Separators`（delimiter，可多个）、`Draggable`、`SaveOnBlur`；entry_inputtag_test.go `TestEntryInputTagSeparators/DragSort/SaveOnBlur` |
| Input OTP | `OTP` | ✅ | 次要：无 separator、formatter | entry_otp.go `Mask/Numeric/Status/Size/Gap` |
| Mention | `Mentions` | ✅ | — | entry_mentions.go `Whole()/WholeCheck(fn)`（whole / check-is-whole；`Remote/Loading` 对应 search/loading） |
| Radio | `Radio`、`RadioGroup` | ✅ | — | choice.go `RadioW.Border/Size`、`RadioGroupW.Border/Disabled`（radio-button 由 `Buttons` 提供）；choice_test.go `TestRadioGroupDisabledAndBorder` |
| Rate | `Rate` | ✅ | 次要：无 low/high 阈值分段配色 | entry_rate.go `Color(theme.Color)` |
| Select | `Select`、`MultiSelect` | ✅ | — | select.go `Prefix/PrefixText/SuffixIcon`；p2b_select.go `SelectW.FilterMethod`、`MultiSelectW.FilterMethod` |
| Virtualized Select | `VirtualSelect` | ✅ | — | fill_virtual_select.go `Filterable/FilterMethod/Remote/Multiple/MaxTagCount` |
| Slider | `Slider`、`SliderRange` | ✅ | — | p2b_slider.go `WithInput`（范围与竖向都可带输入框）、`AllowCross/Pushable` |
| Switch | `Switch`、`SwitchValue` | ✅ | — | choice.go `SwitchValue[T]`（active-value/inactive-value）、`BeforeChange/BeforeChangeAsync`（异步期间 loading）、`Texts/Icons`（inline-prompt）、`OuterTexts`（active-text/inactive-text 在外侧）、`Large/Size`、`Width`；choice_test.go `TestSwitch*` |
| Time Picker | `TimePicker`、`TimeRangePicker` | ✅ | — | p2a_time_range.go `typed`（范围版可键盘输入） |
| Time Select | `TimeSelect` | ✅ | — | more_timeselect.go `Start/End/Step/MinTime/MaxTime/Clearable` |
| Transfer | `Transfer` | ✅ | 次要：无 filter-method、target-order、format | p2b_transfer.go `pager` 复用 `Pagination`；`Render/OneWay/TreeSource/Virtual` |
| TreeSelect | `TreeSelect` | ✅ | — | entry_treeselect.go `CheckStrictly`（check-strictly）、`FilterTreeNode`（filter-node-method）；p2b_treeselect.go `ExpandedKeys`（覆盖 default-expanded-keys，且可受控）；entry_treeselect.go `DefaultExpandAll/Searchable` |
| Upload | `Upload` | ✅ | — | p2d_upload.go `AutoUpload(false)` + `Submit()`、`CustomRequest`（对应 http-request）；entry_upload.go `ListType(UploadListPicture)`；照片墙已有重试（`upCardIconRow`） |
| **Data** | | | | |
| Avatar | `Avatar` | ✅ | — | display.go |
| Badge | `Badge` | ✅ | — | data_badge.go |
| Calendar | `Calendar` | ✅ | — | data_calendar.go `CalendarW.Range`（只显示范围所在的周，标题显示范围，范围外日期不可选）、`elHeader` |
| Card | `Card` | ✅ | — | data_card.go `Shadow/Footer` |
| Carousel | `Carousel` | ✅ | 次要：无 motion-blur | data_carousel.go `Draggable/SwipeThreshold`、`Card/Vertical` |
| Collapse | `Collapse` | ✅ | — | data_collapse.go `Accordion/IconEnd/IconStart` |
| Descriptions | `Descriptions` | ✅ | 次要：无 rowspan | data_card.go `DescItem{Span}`、`ColumnAt/Columns`（断点列数） |
| Empty | `Empty` | ✅ | — | data_result.go |
| Image | `Image` | ✅ | 次要：无视口懒加载（`c.lazy` 只是延迟构建） | data_image.go |
| Infinite Scroll | `InfiniteScroll` | ✅ | — | more_infinite.go `Distance/Loading/Done/Disabled` |
| Pagination | `Pagination` | ✅ | — | nav.go `Layout(PaginationPrev, PaginationPager, PaginationNext, PaginationJumper, PaginationSizes, PaginationTotal, PaginationSpacer)`（"->" 即 `PaginationSpacer`）、`PagerCount(n)`（5–21 奇数） |
| Progress | `Progress` | ✅ | — | data_progress.go |
| Result | `Result` | ✅ | — | data_result.go |
| Skeleton | `Skeleton` | ✅ | — | data_skeleton.go |
| Table | `Table` | ✅ | — | table*.go，另有 `HighlightCurrentRow`、`Ellipsis` |
| Virtualized Table | `Table.Virtual`、`VirtualList`、`Listy` | ✅ | — | table.go `Virtual`；行虚拟化 + 列虚拟化（table_show.go `culled`：只建可见列的表头/单元格/汇总格，固定列照常；table_colvirt_test.go）；more_virtual.go |
| Tag | `Tag`、`CheckableTag` | ✅ | — | display.go、more_checktag.go |
| Timeline | `Timeline` | ✅ | — | data_timeline.go `TimelineItem{Time,TimeTop,Hollow,Large}` |
| Tour | `Tour` | ✅ | — | 同 Ant 那一行 |
| Tree | `Tree` | ✅ | 次要：无 render-content、allow-drop、accordion | data_tree.go `Height`（虚拟滚动）、`Filter`（filter-node-method） |
| Virtualized Tree | `VirtualTree` | ✅ | — | fill_virtual_tree.go `Height/NodeRender/Range` |
| Statistic | `Statistic`、`Countdown` | ✅ | — | display.go、data_skeleton.go `Countdown/StatisticTimer`、`OnChange` |
| Segmented | `Segmented` | ✅ | — | entry_segmented.go `Vertical()` |
| **Navigation** | | | | |
| Affix | `Affix` | ✅ | — | nav_affix.go |
| Anchor | `Anchor` | ✅ | — | nav_affix.go |
| Backtop | `BackTop` | ✅ | — | nav_float.go:485 |
| Breadcrumb | `Breadcrumb` | ✅ | — | nav.go |
| Dropdown | `Dropdown` | ✅ | — | nav_dropdown.go `navMenuPopup.group`（分组标题，Element 取 item-group 标题样式） |
| Menu | `Menu` | ✅ | — | 弹出层复用 nav_dropdown.go `navMenuPopup.group`，item-group 有标题 |
| Page Header | `PageHeader` | ✅ | — | nav_pageheader.go |
| Steps | `Steps` | ✅ | — | nav_steps.go `Type("simple")/FinishStatus` |
| Tabs | `Tabs` | ✅ | — | nav.go `BorderCard`（type="border-card"）；竖向设 `Height` 后溢出有"…"；shadcn 统一画法（同 Ant 那一行） |
| **Feedback** | | | | |
| Alert | `Alert` | ✅ | — | feedback.go `Effect(AlertEffectDark)`（令牌 `*DarkBg/DarkTextColor`）、`Center()` |
| Dialog | `Modal` | ✅ | before-close 靠应用控制 `open` 实现 | feedback.go |
| Drawer | `Drawer` | ✅ | before-close 同 Dialog，靠应用控制 `open` | feedback.go `Resizable/SizeLimits/BindSize` |
| Loading | `Loading` | ✅ | — | feedback.go:295 `Tip/Background/Fullscreen` |
| Message | `Message` | ✅ | 次要：grouping 用 `Key` 原地更新代替 | nav_notify.go `ShowClose/Placement/Offset/Plain`（令牌 `Top/MarginInline/CloseColor/Plain*`） |
| Message Box | `Confirm`、`MessageBox`、`Prompt` | ✅ | — | more_confirm.go `ConfirmOptions.DistinguishCancelAndClose/OnClose`、`ConfirmRef.Action()`、`Center`、`Draggable`、`Icon` |
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
| ProLayout | `ProLayout(c, ProLayoutConfig, fn)`、`DefaultFooter`、`MenuPath` | ✅ | 次要：MenuItemRender 只能改写菜单项数据（Menu 自己绘制，不能换成任意 UI）；NoFixSiderbar 时侧栏布局下顶栏随页面一起滚动 | prolayout.go `ProLayoutConfig{Layout side/top/mix, Menu, Selected, Collapsed, LightSider, Breadcrumb, AutoBreadcrumb, BreadcrumbRender, MenuHeaderRender, MenuFooterRender, HeaderRender, HeaderContentRender, MenuItemRender, FooterRender, FooterLinks, Footer, NoFixSiderbar, Breakpoint, SiderWidth, ContentWidth, NoFixedHeader, NoPadding}`；`DefaultFooterW.Links/Copyright/Clicked`；`ProSettings.Apply` 写 `NoFixSiderbar`，`RouteInfo.FixSiderbar` |
| SettingDrawer | `SettingDrawer`、`ProSettingsProvider`、`ProSettings.Apply` | ✅ | 次要：色弱模式用 `theme.HighContrast` 近似（没有 CSS filter） | pro4_setting.go `SettingDrawerW.Open/NoTrigger/Colors/Changed/Show`；`ProSettings{NavTheme,PrimaryColor,Layout,ContentWidth,FixedHeader,FixSiderbar,ColorWeak,Compact}` |
| RouteContext / GridContent | `RouteContext`、`GridContent` | ✅ | — | pro4_route.go `RouteInfo{InLayout,Layout,Collapsed,IsMobile,HasSiderMenu,Breadcrumb,PageTitle,Selected,ContentWidth,FixedHeader}`；`GridContentW.ContentWidth/MaxWidth`（Fixed 居中 1200px） |
| PageContainer | `PageContainer` | ✅ | — | pro2_page.go `Tabs/TabExtra/TabsCard/Extra/Breadcrumb/Back/Tags/SubTitle/Content/ExtraContent/Footer/Loading/Ghost` |
| FooterToolbar | `FooterToolbar` | ✅ | 吸底（Affix 固定）时毛玻璃：`paint.BackdropBlur` + 半透明底色（`FooterToolbarToken.BlurRadius/FrostedBg`，Ant/shadcn 8px + 60%，Element 10px + 72%）；不吸底、HighContrast、渲染器不支持模糊时用 `Bg`（95%/96%，HighContrast 不透明） | pro2_page.go `surface`；theme/comp_footertoolbar.go；`Extra/Sticky/Target` |
| ProCard | `ProCard` | ✅ | — | pro2_card.go `Actions`（底部等宽操作栏，分隔线，Card 令牌 ActionsBg）、`Split/Direction/Gutter/Wrap/ColSpan/Tabs/Collapsible/HeaderBordered/Ghost/Centered/Loading/Hoverable` |
| StatisticCard | `StatisticCard`、`StatisticCardGroup` | ✅ | 次要：无独立的 `StatisticCard.Divider`（Group 用 Split 自动加分隔） | pro2_statistic.go `Trend/Chart/ChartPlace/Status/Icon/Desc/Footer`、:303 Group |
| CheckCard | `CheckCard`、`CheckCardGroup` | ✅ | — | pro2_checkcard.go `Avatar/Disabled/Size/Cover/Loading`；Group `Value/Values` |
| WaterMark | `Watermark` | ✅ | 次要：zIndex/fontFamily 未确认 | data_watermark.go |
| ProTable | `ProTable[T]` | ✅ | —（列 valueType 与 ProField 共用，含 index/image/avatar/code/second/fromNow 等） | table_pro.go `Params`（外部参数，变化回第 1 页）、`Polling/PausePolling/ResumePolling`、`ColumnsState/OnColumnsStateChange/PersistColumnsState`、`TableAlertRender/TableAlertOptionRender/NoTableAlert`；`Request/ProColumn{ValueType,HideInSearch,HideInTable,Fixed}/ToolBar/NoSearch`，选项区有刷新/密度/列设置/全屏 |
| EditableProTable | `EditableProTable[T]` | ✅ | — | p2e_editable.go `NewRow/AddAtTop/MaxLength/EditableKeys/Multiple/RowEditable/OnSave/OnDelete` |
| DragSortTable | `ProTable.DragSort`、`EditableProTable.DragSort` | ✅ | 以方法形式提供，没有独立入口；自定义拖拽把手未确认 | table_pro.go:364（注释 "Ant Design Pro DragSortTable"）；p2e_editable.go:173 |
| ProList | `ProList[T]` | ✅ | — | pro3_list.go `ProListMetas{Title,SubTitle,Description,Avatar,Content,Actions,Extra}/Selection/Expandable/Grid/Request/Pagination` |
| ProDescriptions | `ProDescriptions` | ✅ | — | pro3_descriptions.go `Request/Editable/ProDescItem{ValueType,ValueEnum,Span,Render}` |
| ProForm | `ProForm(c, []ProField)` | ✅ | — | form_pro.go `Readonly`（整表只读，隐藏按钮、不校验）、`ProField.Readonly`（单字段）、`Columns/Horizontal/Request/OnFinish/Submitter/Children/Gutter`；`ModalFormW.Readonly` |
| ModalForm / DrawerForm | `ModalForm`、`DrawerForm` | ✅ | — | form_pro_modal.go:35/51 |
| QueryFilter | `QueryFilter` | ✅ | — | form_pro_query.go `DefaultExpanded/Searched/Params` |
| LightFilter | `LightFilter` | ✅ | — | pro3_lightfilter.go `Collapse/Bordered/Size/Params` |
| StepsForm | `StepsForm` | ✅ | — | form_pro_steps.go |
| LoginForm / LoginFormPage | `LoginForm`、`LoginFormPage` | ✅ | — | pro3_login.go `OnSendCode/CodeInterval/AccountOnly/MobileOnly/BackgroundImage/Side` |
| ProFormList | `FormList[T]` | ✅ | 写法不同：通过 `ProForm.Children` 嵌入 | form_pro_list.go:54 `Min/Max/Creator/MoveUp/MoveDown` |
| ProFormFieldSet | `ProFormFieldSet`、`FieldSet` | ✅ | — | pro3_fieldset.go:36/60 |
| ProFormDependency | `ProField.Dependencies` + `Hide/Disable/Options` 闭包 | ✅ | 等价实现，不是组件形式 | form_pro_field.go:167-170 |
| ProFormGroup | `ProFormGroup`、`ProField.Group` | ✅ | — | pro4_group.go `ProFormGroupW.Description/Extra/Collapsible/DefaultCollapsed/Collapsed/Columns/Gutter/Body`、`ProFormItems`；form_pro_field.go `ProField.Group`（连续同组字段自动包成 ProFormGroup） |
| SchemaForm（BetaSchemaForm） | `SchemaForm(c, []ProField)` | ✅ | 次要：StepsForm 按 `ProField.Group` 分步（无单独的 steps 配置） | form_pro_schema.go `LayoutType(SchemaLayoutForm/ModalForm/DrawerForm/StepsForm/QueryFilter/LightFilter)/Values/OnFinish/Columns/Readonly/Open/Title/Width/Finished/Searched/Params`；schema 里 `ValueType: ValueFormList` + `ProField.List *ProFieldList{Fields,Min,Max,AddText}` |
| ProForm 字段组件（ProFormText/Select…） | `ProField.ValueType` | ✅ | 次要：没有一个 valueType 一个的独立字段组件（用 `ProFieldOf` 单独渲染）；image 只读值是 image.Image（不加载 URL） | form_pro_types.go `ValueDateWeek/DateMonth/DateQuarter/DateYear/DateTimeRange/Image/Avatar/Code/JSONCode/FromNow/Second/Index/Captcha/FormList`；`ProField.OnCaptcha/CaptchaSeconds/List` |
| ProField | `ProFieldOf(c, ProField, *any)`、`ProFormat()`/`ProFormatLocale()` | ✅ | — | form_pro_field.go `ProFieldW.Mode(ProFieldRead/ProFieldEdit)/Changed/Show`；只读视图与 ProDescriptions/ProTable 共用 |
| ProSkeleton | `ProSkeleton(c, kind)` | ✅ | — | pro2_skeleton.go `SkeletonList/SkeletonResult/SkeletonDescriptions` |

**货币符号（复核：已改为 ISO 代码加地区规则）**

- p2e_currency.go `p2eCurrency` 的取值顺序：显式覆盖 → `Locale.Currency`（ISO 4217 代码或符号）→ 按地区猜（de-CH 用 CHF、fr-FR 用 EUR、zh-HK 用 HKD、ja-JP 用 JPY 等）→ 按语言猜（zh 用 CNY，zh-Hant 用 TWD，ja 用 JPY，ko 用 KRW，de/fr/it/nl/fi/el 用 EUR）→ `$`。
- money.go `FormatMoney(loc, amount, currency)`：ISO 代码决定符号和小数位（JPY/KRW/VND 无小数），地区决定分隔符和符号位置（欧洲语言符号在后，如 "1.234,56 €"）；直接给符号时固定 2 位小数。ProTable（table_pro.go）和 pro3 组件（pro3_util.go `pro3ValueText`）都走它。
- 公开函数 `ProFormat(ValueMoney, …)` 不再写死 `"¥"`：它等于 `ProFormatLocale(nil, …)`，按 zh-CN 格式化；有 Context 的代码应调 `ProFormatLocale(c.Locale(), …)`（注释已写明）。

## 5. 三种风格的截图检查状态

### 5.1 画廊支持的风格

- `-theme antd`（默认）：每个页面出 Ant 亮色图，加上页面自带的 `-Element` / `-Dark` 变体（变体页 `Page.Variant = true`）。
- `-theme shadcn`：把**每个非变体页面**再以 shadcn 亮/暗各渲染一次，输出 `<Name>-shadcn.png` 和 `<Name>-shadcn-dark.png`。
- `-theme hc`：仪表盘、Form、Switch、Checkbox 在各风格下的默认版与 HighContrast 版对照。
- `-theme all` = antd，再加 antd-dark、element（亮 + 暗）、shadcn（亮 + 暗）三轮，每个非变体页面都出这六种图。
- 画廊首页 `gallery.Home` 能在 Ant / Element / shadcn 与暗色之间切换。
- shadcn 是**主题轮次**，不是逐页手写的变体。全仓只有 `page_shadcn.go` 一个 shadcn 专属页（ShadcnDashboard，以及 Command/Toggle/Kbd 等）。
- 对标 shadcn 新增（groupF，2026-11-14）：`Menubar`（✅ menubar.go，下拉面板复用 nav_dropdown.go `navMenuPopup` + `navMarks` 勾选/单选指示；页面 page_menubar.go）、`AspectRatio`（✅ aspect_ratio.go；page_aspect_ratio.go）、`Item`/`ItemGroup`（✅ item.go；page_item.go）。三者在 Ant / Element / shadcn 亮暗下均有截图；令牌 `Components.Menubar` / `Components.Item`。`ConfigProvider.Defaults`：Item（variant/size）、ItemGroup（split/bordered）已接线（groupG）；Menubar、AspectRatio 记为不适用（见 6.1）。

### 5.2 本次渲染结果（2026-11-14 重跑，HEAD `f3ba799`，Windows，`-theme all -scale 1`）

- 全部成功，退出码 0，耗时 3 分 30 秒；`components/` 下 1590 张，`mobile/` 下 66 张，另有首页、总览、仪表盘、浮层图（各六种风格）。
- 基础页面（非变体）共 231 个（上次 219，新增 Menubar、AspectRatio、Item、ButtonGroup 等）。

| 维度 | 有截图的页面 | 缺的页面 | 上次（2026-11-13 晚，`22a1c1a`） |
|---|---|---|---|
| Ant 亮色 | 231 / 231 | 0 | 219 / 219 |
| Ant 暗色（`-antd-dark`） | 231 / 231 | 0 | 196 / 219（`-Dark` 变体，缺 23） |
| Element 亮色（`-element`） | 231 / 231 | 0 | 196 / 219（`-Element` 变体，缺 23） |
| Element 暗色（`-element-dark`） | 231 / 231 | 0 | 40 张 |
| shadcn 亮 + 暗 | 231 / 231（各 231 张） | 0 | 219 / 219 |
| 移动端（`mobile/`） | 66 张 | — | 66 |

上次缺 Element 与暗色的 23 页（移动端 13 页、`ConfigProvider-{en-US,ja-JP,zh-TW}-{Data,Date,Modal}` 9 页、ShadcnDashboard）现在由 `-theme all` 的 antd-dark / element 两轮补齐。页面自带的 `-Element` / `-Dark` 手写变体仍然保留，作为额外对照图。

**上次（2026-11-13 晚）的记录**：219 个基础页面，`components/` 下 1085 张；移动端页用 antd/vant 两套令牌，本来就不按 Element 出图；CodeEditor、CodeEditorLarge、DescriptionsResponsive、Icon、TreeVirtual 在 `page_shad2c_variants.go` 补注册了 `-Element` / `-Dark`（代码编辑器宽 1100，暗色下用 vs-dark；DescriptionsResponsive-Element 在 960 宽下是 Element 的 sm 断点，列表单列，属正常）。

**Pro2 组件的 shadcn 检查**：之前记为"未截图检查"，本次已渲染并抽查了 ProCard、StatisticCard、PageContainer（含 FooterToolbar）、CheckCard、ProSkeleton（shadcn-dark），结果见 5.3，这个缺口可以关闭。

### 5.3 shadcn 抽查结果

（2026-11-13 晚重跑后复看了 TabsMore、ProCard、Menu 三张，结论已写进下表。）

抽查了以下 10 张：TabsMore、Menu、ProCard、StatisticCard、PageContainer、CheckCard、LoginForm、ProSkeleton-dark、Table-dark，以及 Calendar（只看了尺寸）。

| 图 | 结论 |
|---|---|
| TabsMore-shadcn | **已修（重跑确认）**：Left/Right/Bottom 位置与可编辑卡片都改用 shadcn 分段样式；"溢出滚动与「更多」"一栏右侧出现"…"按钮，`ExtraLeft`/额外操作正常。上次的三点问题都不再出现。 |
| Menu-shadcn | "内嵌模式 / 暗色"一栏右侧的暗色菜单**没有变暗**。这是设计取舍：nav_menu.go:104 写明 shadcn 只有一套 Sidebar 外观，`Menu.Dark()` 在 shadcn 下被忽略，需要在文档里说明。弹出子菜单正常。重跑后不变。 |
| ProCard-shadcn | 整体正常。"默认尺寸"卡片头部的"操作"离右边缘约 16 px，**不是缺陷**：它是 Link 按钮，按钮两侧保留内边距（button.go `measure` 用 `padX()`；shadcn 16 px 即 `px-4`，Ant 15 px），与 shadcn/ui 和 Ant Design 原版一致，Ant 版截图同样如此。同排"带边框"卡片的日期是纯文本，所以贴右。Element 风格的 link 按钮按 `el-link` 去掉了内边距。 |
| StatisticCard-shadcn | 正常。趋势红涨绿跌，迷你图用 `--chart-1` 色。 |
| PageContainer-shadcn | 正常。页头、标签（segmented 风格）、FooterToolbar 吸底都对；FooterToolbar 没有毛玻璃（与第 4 节一致）。 |
| CheckCard-shadcn | 正常。选中态是深色边框加右上角三角标。 |
| LoginForm-shadcn | 正常。右侧表单的初始校验错误应该是画廊 Setup 触发的。 |
| ProSkeleton-shadcn-dark | 能用。页头区块是一整块浅灰，层次比 Ant 暗色更"反"（外层比卡片亮），属于 shadcn 暗色令牌本身的特点。 |
| Table-shadcn-dark | 能用。排序列有整列底色高亮（这是 Ant 的行为，shadcn 原版没有）；未勾选复选框的边框在暗色下很淡。两点都是轻微问题。 |

### 5.4 无障碍审计（`-audit`，2026-11-14 重跑，HEAD `f3ba799`：693 次页面渲染 × 3 种风格）

- missing-name 0，no-role 0，not-keyboard 0；无名交互节点 0（上次 657 次渲染，同样全为 0；2026-11-13 之前的 12 项 not-keyboard 来自 `CarouselCard`，data_carousel.go 卡片模式给每张卡片设 `aKbd` 后已消失）。
- 对比度：6 个内置主题 0 项失败；108 项设计默认值低于 WCAG AA（58 项来自各设计规范本身，另有 50 项是终端 ANSI 调色板；HighContrast 能修正）；24 项豁免（禁用态、装饰性分隔线）；HighContrast 下 0 项失败。与上次相同。
- 运行：`go run ./examples/ui-gallery -audit`，31 秒，退出码 0。
- 镜像检查：`go run ./examples/ui-gallery -mirror-check`，231 页、16667 个节点（298 个豁免），只有 Mentions 的 2 处（已知项）；方向性图标未镜像 0。

## 6. 非组件能力

| 能力 | 状态 | 说明与证据 |
|---|---|---|
| RTL | ✅ | 基础层与组件层均已完成（2026-11-13）：`ConfigProvider.Direction`/Locale 推断、布局镜像（`layout.Physical()` 逃生口）、逻辑滚动、逻辑浮层位置、方向性图标镜像、`TextAlign` Start/End、方向键换向；全部组件已迁移（输入/选择/日期/滑块/导航/菜单/表格/树/反馈/表单/编辑器 RTL 行对齐）。时间列、数字输入、代码固定 LTR。`Driver.MirrorCheck` 与 gallery `-dir rtl`、`-mirror-check` 用于回归。原遗留项均已完成：混排文本视觉光标移动（Q12 第二期，ui/editor.go `visualStep`）；a11y 方向标记（`a11y.Node.RTL`/`Tree.RTL`，Linux AT-SPI 文本属性 `direction` 已实现，Windows UIA / macOS NSAccessibility 无对应元素属性、记为不适用，见 docs/accessibility.md）；图标未镜像检查（`paint.List.IconNote` + `Driver.MirrorCheck`/`Audit`，ui/icon_dir.go）；mobile/chart 子包 RTL（A4，ui/mobile、ui/chart 的 rtl_test.go）。 |
| 组件级主题令牌（类似 `theme.components`） | ✅ | `theme.Components` 共 54 个组件令牌类型（`theme.NumComps`，Button … Form，ui/theme/comp_*.go），全部有派生默认值，对应组件均已改读令牌，零值继承与哨兵、组件 Alias 重派生、HighContrast 覆盖组件令牌、`CSSVarsWith`；`ConfigProvider.Components/Theme/Dark…` 嵌套局部生效；默认值与原绘制逐像素一致。共享下拉面板 `entStylePanel` 读 Select 令牌（含 `PopupRadius/PopupShadow`）；Menu 弹出子菜单读 Menu 令牌自己的 `Popup*` 字段，不受 Dropdown 覆盖影响；TimeSelect 选项读 Select 令牌的 Option 字段。`Theme` 上已有组件令牌对应的扁平组件字段标注 `Deprecated:`（仍作为派生来源保留）。 |
| ConfigProvider.Defaults 接线 | ✅ 141/177，其余 36 不适用 | 见 6.1 |
| 全局尺寸 | ⚠️ | `ConfigProvider.Size` → `c.ComponentSize()` / `defaultSize()` / `sizeOr()`，约 25 个文件读取：button、choice、data_card、data_collapse、data_list、date、entry_color、entry_datepanel、entry_daterange、entry_inputx、entry_mentions、entry_otp、entry_segmented、fill_color_panel、input、more_timeselect、nav、nav_dropdown、nav_steps、p2a_time_range、pro2_card、pro2_checkcard、pro3_lightfilter、select、selector、shad_toggle、table、feedback。另有大量组件读 `Defaults` 里的 "size"：Tag/Tabs/Pagination/Steps/Space/Form，以及 shad2a_defaults.go `shad2aSize`（Button、RadioGroup、Input 系、选择器等）、shad2a_tree.go `shad2aTreeSize`（Cascader、TreeSelect）、shad2c_defaults.go `s2cSize`（Spin、Steps、Toggle、ToggleGroup）、shad2d_util.go `proPropSize`（Pro 组件）。没读的有 Slider（本身没有 Size）。Checkbox/Radio 另读 `Defaults("Checkbox"/"Radio", {"size"})`（ui/defaults_more.go）。 |
| 主题 / 设计语言 | ✅ | `theme.AntDesign`、`theme.ElementPlus`、`theme.Shadcn`；`ConfigProvider.Flavor/PrimaryColor`；图表用 `chart.RegisterStyle` 按风格注册。 |
| 暗色 | ✅ | `theme.Dark()`、`ConfigProvider.Dark` |
| 紧凑 | ✅ | `theme.Compact()`、`ConfigProvider.Compact` |
| 高对比度 | ✅ | `theme.HighContrast(th)`（ui/theme/highcontrast.go）、`Theme.HighContrast` 标志，对比度审计 `theme.ContrastIssues`，画廊 `-theme hc`。`ConfigProvider.HighContrast(on)`（nav_config.go，局部固定高对比度或标准变体；根级作用于窗口，宿主随之停止跟随系统）；宿主跟随系统高对比度见 uihost/contrast.go `ContrastSystem`（默认）。 |
| 语言 | ✅ | 四个内置包 `ZhCN/EnUS/ZhTW/Ja`（locale_packs.go）；`ConfigProvider.Locale/LocaleCode`、`SetDefaultLocale`、`LocaleByCode`；`TestLocalePacksComplete`、i18n_wiring_test.go、i18n_scan_test.go。货币按 ISO 代码和地区规则格式化（money.go `FormatMoney`，第 4 节）。 |
| ReduceMotion | ✅ | `RunnerOptions.ReduceMotion` / `Runner.SetReduceMotion`；`Anim/AnimFrom` 直接到终值；装饰性动效用 `c.still()`（anim.go:69），调用方有 data_badge、data_image、data_progress、data_skeleton、nav_util、text、data_carousel。 |
| 无障碍 | ✅ | 语义树 ui/a11y、ui/a11y_tree.go；审计 `Driver.Audit()`（a11y_audit.go:102）和 gallery `-audit`；键盘漫游 `rovingTabStop/rovingStep`；`c.TrapFocus`。重跑审计（5.4）：missing-name、no-role、not-keyboard 都为 0，上次 CarouselCard 的 12 处键盘不可达已消失。 |
| 响应式断点 | ⚠️ | `Breakpoint` XS…XXL、`ScreenBreakpoint(c)`（nav_grid.go:39）。GridRow/Col（`XS…XXL`、`GutterAt`）、Masonry（`ColumnsAt`）、ProLayout/Sider（`Breakpoint`）、Descriptions（`ColumnAt/Columns`）、List 网格（`GridAt`）用到，后两者共用 data_util.go `dataRespCols`。**ProDescriptions（`Column(n)`）、ProList 网格（`Grid(n)`）、QueryFilter 列数仍不跟随断点。** |

### 6.1 ConfigProvider.Defaults 接线清单

- **统计方法**：grep `ConfigProp(`、`Prop(c, "X"`、`propString/propBool/propFloat/propInt64`、`p2bBool/p2bNum/p2bStr/p2bDur`、`p2bSelectDefaults`、p2a_date.go 的 `p.comp`，复核时另加 `proPropInt/proPropSize/proPropPageSize`（shad2d_util.go）、`shad2aSize/shad2aTreeSize`、`s2cSize/messageDuration`（shad2c_defaults.go）、`dataPropCols/dataPropSize`（data_util.go）、`formPropSize`，以及以组件名为参数的 `sizeDefaults("Input")`、`shadDefaults("InputTag")`、`ModalFormW.defaults("ModalForm")` 等，取出读取到的组件名。入口直接调用另一个已接线入口、或读别的组件键的，也算接线（与 MultiSelect 读 "Select" 键同一口径）。2026-11-14 补的接线集中在 ui/defaults_more.go（各构造函数里一行 `configDefaults()`），新键一律用常量组件名和键名调用 `Prop` / `c.ConfigProp`，因此都进了 api/go/ui.txt 的 prop 行（R11）。
- **分母**：177 个公开入口。即 `func Xxx(c *Context…)`（含 ProTable/EditableProTable/ProList 这类泛型入口），去掉 Row/Column/Div/StackBox/Custom/Spacer/Gap/Key/Disabled/Overlay/HoverDelay/SetHoverOpen/ScreenBreakpoint/OpenDialogs/MessageDestroy/NotificationDestroy/ConfigProvider 这些基础件，以及工具函数和取值钩子 Prop/State/UseApp/RouteContext/CloseContextMenu/ContextMenuOpen/ProSettingsProvider/ProFormItems 和 ContextMenuAt 的选项变体 ContextMenuAtWith。比 164 多出的 13 个是 AspectRatio、ButtonGroup、ColorPickerGradient、ColorPickerPanelGradient、DefaultFooter、Item、ItemGroup、Menubar、MultiDatePicker、ProFieldOf、SchemaForm、StatisticTimer、SwitchValue。`FormList[T]`（读 "ProFormList" 键）的第一个参数是 `*FormW` 不是 `*Context`，不计入分母。
- **结果**：已接线 141，不适用 36，未接线 0。

**已接线（141）**

Addon、Affix、Alert、Anchor、App、AutoComplete、Avatar、AvatarGroup、BackTop、Badge、BadgeRibbon、Breadcrumb、BreadcrumbItems（读 "Breadcrumb" 键）、Button、ButtonGroup、Calendar、Card、Carousel、Cascader、CascaderPanel（读 "Cascader" 键）、CheckAll（直接调用 Checkbox/CheckboxGroup）、Checkbox、CheckboxGroup（读 "Checkbox" 键）、CheckCard、CheckCardGroup（读 "CheckCard" 键）、Collapse、CollapsibleSider（读 "Sider" 键）、ColorPicker、ColorPickerGradient（直接调用 ColorPicker）、ColorPickerPanel（读 "ColorPicker" 键）、ColorPickerPanelGradient（直接调用 ColorPickerPanel）、Command、Compact、Confirm、Countdown、DatePicker、DatePickerPanel（读 "DatePicker" 键）、DateTimePicker、Descriptions、Divider、Drawer、DrawerForm、Dropdown、EditableProTable、Empty、FieldSet（直接调用 ProFormFieldSet）、Flex、FloatButton、FloatButtonGroup、FooterToolbar、Form、GridContent、GridRow、Header、Image、InfiniteScroll、Input、InputNumber、InputTag、Item、ItemGroup、LightFilter、List、Listy、Loading（读 "Spin" 键）、LoginForm、LoginFormPage、MarkdownEditor、Masonry、Mentions、Menu、Message、MessageBox（读 "Confirm"/"Modal" 键）、Modal、ModalForm、MultiDatePicker（读 "DatePicker" 键）、MultiSelect（读 "Select" 键）、Notification、OTP、PageContainer、PageHeader、Pagination、Popconfirm、Popover、ProCard、ProDescriptions、ProFieldOf（读 "ProField" 键）、ProForm、ProFormFieldSet、ProFormGroup、Progress、ProLayout、ProList、Prompt（读 "Confirm"/"Modal" 键）、ProSkeleton、ProTable、QRCode、QueryFilter、Radio、RadioGroup（读 "Radio" 键）、RangePicker、Rate、SchemaForm、Search、Segmented、Select、SettingDrawer、Sider、Skeleton、Slider、SliderRange（直接调用 Slider）、Space、Spin、Splitter、SplitterPanel、StatisticCard、StatisticCardGroup（直接调用 ProCard）、StatisticTimer（读 "Countdown" 键）、Steps、StepsForm、Switch、SwitchValue（直接调用 Switch）、Table、Tabs、Tag、TextArea、Timeline、TimePanel（读 "TimePicker" 键）、TimePicker、TimeRangePicker、TimeSelect、Toggle、ToggleGroup、Tooltip、Tour、Transfer、Tree、TreeSelect、Upload、VirtualSelect（读 "Select" 键）、VirtualTree。

**2026-11-14 新增的键**（测试：ui/defaults_more_test.go `TestDefaultsWiring` 表驱动，每项断言默认值生效、显式调用优先；另有 `TestButtonAutoInsertSpaceRules`、`TestMessageMaxCount`、`TestFieldSetDefaults`、`TestTimeRangePanelsIgnoreTimePickerDefaults`）

| 组件键 | 键 | 值类型 | 读取方 |
|---|---|---|---|
| Addon | size | ControlSize 或 "small"/"middle"/"large" | Addon |
| BackTop | visibilityHeight、duration | 数字（px）；time.Duration 或毫秒数 | BackTop |
| Compact | block | bool | Compact |
| Flex | gap | ControlSize、尺寸名或 px 数字 | Flex |
| FloatButtonGroup | shape（"square"）、trigger（"click"/"hover"）、type（"primary"） | string | FloatButtonGroup |
| SplitterPanel | collapsible、resizable | bool | SplitterPanel |
| Command | placeholder、emptyText | string | Command |
| Checkbox | size | 同 Addon | Checkbox、CheckboxGroup（CheckAll 经由二者） |
| Radio | size（原来只有 RadioGroup 读） | 同 Addon | Radio |
| MarkdownEditor | mode（"edit"/"split"/"preview"）、placeholder | string | MarkdownEditor |
| AvatarGroup | maxCount | 数字 | AvatarGroup |
| BadgeRibbon | placement（"start"/"end"，参数为 "" 时生效） | string | BadgeRibbon |
| Countdown | format | string | Countdown、StatisticTimer |
| InfiniteScroll | distance | 数字（px） | InfiniteScroll |
| Listy | split、rowHoverable | bool | Listy |
| Tour | type（"primary"）、mask、arrow | string / bool | Tour |
| VirtualTree | checkStrictly、expandOnClickNode、checkOnClickNode | bool | VirtualTree |
| Item | variant、size | ItemVariant / ItemSize 或其字符串 | Item |
| ItemGroup | split、bordered | bool | ItemGroup |
| ButtonGroup | size、type（Button 的 type/variant 名） | 同 Addon / string | ButtonGroup |
| Button | autoInsertSpace | bool | Button（另有 `ButtonW.AutoInsertSpace(on)`） |
| Message | maxCount | 数字（0 不限） | Message |

复用已有键（不新增 API）：CascaderPanel 读 "Cascader" 的 expandTrigger/changeOnSelect/showCheckedStrategy/strategyValue；ColorPickerPanel 读 "ColorPicker" 的 size/allowClear/disabledAlpha/format；DatePickerPanel 读 "DatePicker" 的 picker；TimePanel 读 "TimePicker" 的 format/use12Hours/hourStep/minuteStep/secondStep/hideDisabledOptions（TimeRangePicker 内部的时间列改用不读默认值的 `timePanel`，行为不变）；MultiDatePicker 读 "DatePicker" 的 format/placeholder/picker/allowClear/width/size；Loading 读 "Spin" 的 size/tip。

**不适用（36）**——没有值得在作用域里统一设默认值的行为/结构属性，不硬凑：

- 纯布局容器：Col（span/offset 等只对单个格子有意义）、Content、Footer、Layout（只有 layout.Option）、Scroll、ScrollH（滚动容器，Both/Always 是单实例的滚动区设置）、AspectRatio（比例是构造参数，Radius/MaxWidth 是视觉数值）。
- 只有视觉数值（应走组件令牌）：BorderBeam（颜色/渐变/时长/线宽/尺寸）、Icon（尺寸/颜色/旋转）、IconButton（尺寸/颜色/去悬停底色）、Watermark（旋转/间距/字号/颜色/图片）、VDivider（无参数，尺寸来自 Divider 令牌）、DropZone（只有高亮开关）、Kbd（无选项）、Markdown、RichText（字号/颜色/对齐；NoSelect/NoCopy 是针对某段内容的设置）。
- 内容由参数或子件决定、没有选项：AnchorSection（Anchor 的目标区块）、CardAction、CardMeta（Card 的子件）、ListActions、ListItemMeta（List 的子件）、Menubar（只有 A11yLabel）、Result（status/title 是参数，其余是内容插槽）、DefaultFooter（链接与版权是页面内容）、ContextMenuAt（一次性打开的菜单，唯一选项 NoAutoSelect 是给编辑器用的单次设置）。
- 排版：Text、Title、Paragraph（Type/Strong/Ellipsis 等逐段决定，Ant ConfigProvider 的 typography 也只有 className/style）、Link（Type 是颜色语义；且日期面板"今天/此刻"、可编辑表格的保存/取消等内部都用 Link，默认 type 会串进这些组件）、CheckableTag（只有颜色 type、图标和禁用）。
- 参数已全部显式：SkeletonAvatar、SkeletonButton、SkeletonImage、SkeletonInput（size/shape/active 都是必填参数，没有"未设置"状态，不改签名就无法套默认值；整体骨架屏用 Skeleton 的 "active"/"round"）、Statistic（precision 是参数，千分位随 Locale）。
- 单实例数值：VirtualList（itemHeight 是参数，Height/Overscan 依赖具体列表）。

**说明**

- Checkbox/Radio 只接 "size"，不接 border 一类外观键：Table、Tree、Transfer、ProList 等 20 多处内部直接复用 Checkbox/Radio，外观默认值会串进去；"size" 与全局 `ConfigProvider.Size` 原本就会影响这些内部复选框，口径一致。
- 只有"打开"方法、没有"关闭"方法的开关（Compact.block、Tour 的 mask/arrow、ItemGroup 的 split/bordered），默认值设成另一侧后无法逐个改回，与 Button "block" 等已有键相同。
- `button.autoInsertSpace`：Element 文档默认 false，Ant 默认 true。本仓库三套风格默认都关（保持现有渲染），Ant 应用需要时用 `Defaults("Button", Props{"autoInsertSpace": true})` 打开；只对两个汉字、无图标、非 text/link 外观的按钮生效，无障碍名称保持原文。
- `message.max`：键名按 Ant 的 `maxCount`（Element 叫 `max`），作用于普通 `Message` 调用；超出时关闭最早的消息。App 的 `AppMessageConfig.MaxCount` 仍只作用于 `UseApp` 句柄。

### 6.2 子包简述（不计入三家对标）

- `ui/chart`：Line/Area、Column/Bar、Pie/Donut、Scatter/Bubble、Radar、Gauge、Funnel、Heatmap、DualAxes、Tiny 系列、Liquid。三种风格都有样式（chart/style_shadcn.go），有无障碍测试（chart/a11y_chart_test.go）。
- `ui/mobile`：对标 Ant Design Mobile / Vant 两套令牌，另有 shadcn/ui 的移动端派生风格，共 61 个入口，截图 66 张（antd/vant/shadcn × 亮/暗，见 5.2）。在 iOS / Android 上经自绘 canvas 上屏（[mobile-canvas.md](mobile-canvas.md)，M1–M5 已实现，示例 `examples/mobile-ui`）。
- `ui/editor`：代码编辑器（buffer、monarch 高亮、IME、gutter）。规格与进度见 docs/specs/code-editor*.md，不属于组件库对标范围。

## 7. 已知的平台层与非 UI 缺口（来自项目记录，未做代码核实）

- 平台层没有 DamagePresenter（局部重绘 / 增量上屏还没接到各平台呈现器）。
- Windows 和 Linux 上的触屏输入没有接入。
- ~~uihost 不会跟随系统高对比度~~：复核时已有 uihost/contrast.go（`ContrastSystem` 为默认，跟随系统主题的窗口在系统要求高对比度时画 `theme.HighContrast`），见第 6 节。
- Windows 的 AppAppearance（跟随系统亮暗、强调色）没有实现。
- Windows 和 Linux 没有在真机上验证过。

## 8. 建议的补齐优先级

工作量：小 ≤ 1 天，中 2–4 天，大 ≥ 1 周。

| 优先级 | 项 | 工作量 | 理由 |
|---|---|---|---|
| P0 | Dropdown 支持 `MenuItem.Group`（复用 `menuFlatGroups`）；Menu/Dropdown 弹出层的分组画标题（nav_menu.go 已有 inline `menu-group-title` 可以复用） | 小 | 已完成（复核：nav_dropdown.go `navMenuPopup.group`，见第 2 节 Dropdown/Menu） |
| P0 | Tabs：shadcn 横向和所有竖向 Tabs 的溢出加"…"更多下拉；shadcn 下 Card/Closable/OnAdd/竖向给出统一画法 | 中 | 已完成（复核：nav.go `shadTrack`、竖向 `Height` 溢出；重跑截图 TabsMore-shadcn 确认） |
| P0 | Link 的 `Type` 着色生效 | 小 | 已完成（复核：text.go `LinkW.colors`） |
| P0 | `ProFormat(ValueMoney)` 改为读 Locale/Currency，不再写死 "¥"；货币格式改成 ISO 代码加地区规则 | 小（前者）/ 中（后者） | 已完成（复核：money.go `FormatMoney`、table_pro.go `ProFormatLocale`，见第 4 节） |
| P0 | CarouselCard 的卡片键盘可达 | 小 | 已完成（复核：data_carousel.go 卡片 `aKbd`；重跑审计 not-keyboard 0 确认） |
| P1 | RangePicker 支持 week/month/quarter/year 粒度（面板 `DatePickerPanel.Range().Picker()` 已有，接到触发器上）；DatePicker/RangePicker/TimeRangePicker 支持键盘输入（复用 DateTimePicker 的 `dtParse` 和输入框） | 中 | 已完成（复核：entry_daterange.go `Picker/typed`、p2a_time_range.go `typed`、date.go `dtTyping`） |
| P1 | ProDescriptions、ProList、QueryFilter 的列数跟随断点（Descriptions `ColumnAt`、List `GridAt` 已完成，可复用 data_util.go `dataRespCols`） | 小 | 已有断点基础设施 |
| P1 | Form 按字段校验、清除（validateFields(names)/clearValidate）；RulesFunc 支持 reset；表单级 Disabled | 中 | 已完成（复核：p2d_form.go `ValidateFields/ClearValidate/Disabled/OnReset`） |
| P1 | TreeSelect 受控展开；勾选策略改为真正影响绑定值（Cascader 同理）；Cascader 懒加载父节点不再按叶子处理 | 中 | 已完成（复核：p2b_treeselect.go `ExpandedKeys/StrategyValue`、p2b_cascader.go `StrategyValue`、entry_cascader.go `leaves`） |
| P1 | ConfigProvider：给 Checkbox/CheckboxGroup、单个 Radio、各 Panel 补 Defaults 接线（Button/Table/Card/Drawer/Message/Notification/Switch/Empty/ProTable 与 HighContrast 开关已完成） | 小 | 已完成（2026-11-14，141 已接线 / 36 不适用，见 6.1） |
| P1 | ProTable：外部 params、polling、可自定义批量操作栏、受控/持久化 columnsState | 中 | 已完成（复核：table_pro.go `Params/Polling/TableAlertRender/ColumnsState/PersistColumnsState`） |
| P1 | Slider 范围柄可交叉/pushable；WithInput 支持范围与竖向 | 小 | 已完成（复核：p2b_slider.go `AllowCross/Pushable/WithInput`） |
| P1 | Variant（filled/borderless/underlined）推广到 Select/Cascader/TreeSelect/AutoComplete/Mentions/TextArea | 中 | v6 的统一外观能力；现在 Input/InputNumber 和日期时间类（DatePicker/RangePicker/DateTimePicker/TimePicker/TimeRangePicker/TimeSelect 的 `Variant`）有 |
| P2 | ColorPicker 渐变；Upload listType=picture、手动 submit（hsb、照片墙重试、Transfer 用 Pagination 已完成） | 中 | 已知缺口 |
| P2 | Tree 的 titleRender/allowDrop（虚拟滚动已完成：data_tree.go `Height`） | 小 | 次要缺口 |
| P2 | v6 新模式：Timeline 横向、Segmented 竖向/round、Steps inline/panel、Button color×variant、Statistic.Timer（Drawer resizable、Splitter lazy、Carousel 拖拽已完成） | 中（合计） | 跟进 v6 |
| P2 | Element 专有：Button success/warning/info/plain + ButtonGroup、Switch active-value/inline-prompt、Alert effect=dark、Pagination layout、Calendar range（Tabs border-card、CascaderPanel 已完成） | 中（合计） | 提高 Element 风格的完整度 |
| P2 | Pro：ProCard actions、ProForm readonly、ProField 独立组件与 read/edit 模式、FooterToolbar 毛玻璃（需要先给 paint 加背景模糊原语）、ProLayout 按路由生成面包屑（SettingDrawer、ProFormGroup 已完成） | 大（合计） | 次要项 |
| P2 | 组件级主题令牌（ConfigProvider 局部 Theme 或 `Components` 覆盖表） | 大 | 已完成（2026-11-13，54 个组件令牌，见第 6 节） |
| P2 | RTL：布局逻辑方向（start/end）、镜像、ConfigProvider.Direction，再逐个组件改图标和箭头方向 | 大 | 已完成（2026-11-13，含 mobile/chart，见第 6 节） |
| P2 | 画廊：给 43 个缺 Element 变体、69 个缺暗色变体的页面补变体（至少覆盖 Button/Input/Select/Table/Tabs/Form/Checkbox/Switch 主页面） | 小–中 | 方便做视觉回归 |
