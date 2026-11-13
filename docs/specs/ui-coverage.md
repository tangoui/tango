# Tango UI 组件覆盖率扫描

> 扫描日期：2026-11-13
> 复核：2026-11-13 晚，按当前代码逐行更新（Splitter/Dropdown/Menu/Tabs/Form/Slider/TimePicker/Transfer/Carousel/Descriptions/List/Tree/Drawer 等缺口已补，App、SettingDrawer、RouteContext/GridContent、ProFormGroup、CascaderPanel 已有实现，ProTable 补齐 params/polling/columnsState/alert 自定义，货币改为 ISO 代码；Defaults 接线重算为 106/164；计数见 1.2）
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

| 对标 | 总数 | ✅ 完整 | ⚠️ 部分 | ❌ 缺失 | 上次扫描（复核前） |
|---|---|---|---|---|---|
| Ant Design v6 | 72 | 56 | 16 | 0 | 72：42 / 29 / 1 |
| Element Plus | 79 | 59 | 20 | 0 | 79：44 / 35 / 0 |
| Pro Components | 28 | 21 | 7 | 0 | 28：17 / 8 / 3 |

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
  - 晚间按代码复核：Dropdown 分组、Link 着色、Tabs shadcn/溢出、ProFormat 货币、CarouselCard 键盘、RangePicker 粒度与键盘输入、Form 按字段校验、TreeSelect 受控展开等已补，剩余项见第 8 节。复核只读代码，**没有重跑截图和审计**，第 5 节仍是早先那次渲染的结果。

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
| Splitter | `Splitter`、`SplitterPanel` | ✅ | 次要：不能自定义折叠图标 | nav_splitter.go `SplitterW.Lazy`（拖拽只画预览线，松手再提交）、`DragStarted/DragEnded`；`SplitterPanelW.Collapsible`（两侧箭头）、`CollapseToggle(collapseTip, expandTip)`（单箭头只折叠本侧，相当于分 start/end）；`Resizable/Min/Max/Changed` |
| **导航** | | | | |
| Anchor | `Anchor`、`AnchorSection` | ✅ | — | nav_affix.go `Horizontal/TargetOffset/NoInk`、`AnchorItem.Children` |
| Breadcrumb | `Breadcrumb`、`BreadcrumbItems` | ✅ | — | nav.go `BreadcrumbItem.Menu/Href/Icon`、`SeparatorIcon` |
| Dropdown | `Dropdown` | ✅ | — | nav_dropdown.go `navMenuPopup.items` 对 `it.Group` 调 `group`：画分组标题（`groupTitle`，按 Ant/Element/shadcn 取样式）、按风格加分隔线（`groupSeps`），分组是带名字的 group 节点；`estimate` 也计入标题高度 |
| Menu | `Menu` | ✅ | 次要：横向菜单根层与"…"溢出项里的分组仍被拍平成 Divider（`menuFlatGroups`） | 弹出子菜单与折叠态复用 `navMenuPopup`（nav_menu.go `pop := &navMenuPopup{…}`），分组有标题；inline 标题见 nav_menu.go `menu-group-title`；nav.go `menuFlatGroups` 只用于 `fitHorizontal` |
| Pagination | `Pagination` | ✅ | 次要：无 `itemRender` | nav.go `ShowTotal/ShowQuickJumper/Simple/SizeChanger/HideOnSinglePage` |
| Steps | `Steps` | ⚠️ | 无 `type=inline` / `panel`；progressDot 不能自定义渲染 | nav_steps.go `Type` 只认 navigation/simple；有 `Percent/Dot/LabelVertical` |
| Tabs | `Tabs` | ✅ | 次要：不能拖拽排序；竖向 Tabs 不设 `Height` 时随标签增高，不出"…" | nav.go `bar` 里 shadcn 横竖向都走 `shadTrack`（Card 为描边轨道），`addButton`/`tab` 有 shadcn 分支；`scroller` 横向与设了 `Height` 的竖向都调 `overflow/moreMenu`；`ExtraLeft`（左侧 extra）、`BorderCard`、`Centered`、`TabBarStyle` |
| **数据录入** | | | | |
| AutoComplete | `AutoComplete` | ⚠️ | 无 Variant | entry_autocomplete.go `Source(func(q) []Option)`、`Remote/Debounce/Loading`（异步远程搜索，与 Select 共用 remote_search.go）、`AllowClear/Status/Prefix/Highlight` |
| Cascader | `Cascader` | ⚠️ | 无自定义 filter、displayRender、Variant | entry_cascader.go `leaves` 注释：未加载的懒节点代表整棵子树，加载后把勾选交给子节点；p2b_cascader.go `LoadData`（`done(nil)` 才标为叶子）、`ShowCheckedStrategy`、`StrategyValue`（绑定值随策略）、`Searchable` |
| Checkbox | `Checkbox`、`CheckboxGroup`、`CheckAll` | ✅ | 次要：`CheckboxGroup` 是返回 bool 的函数，不能整组 Disabled 或链式配置 | choice.go:304 `func CheckboxGroup(...) bool`；`Indeterminate/Disabled` |
| ColorPicker | `ColorPicker`、`ColorPickerPanel` | ⚠️ | **无渐变**；无 hover 触发 | entry_color.go `cpFormatOK` 接受 hex/rgb/hsb/hsv，`Format` 注释写明 hsb 面板；`PresetGroups`（多组带标题、可折叠）；grep `gradient` 为 0 |
| DatePicker | `DatePicker`、`DateTimePicker`、`RangePicker`、`DatePickerPanel` | ⚠️ | 无 multiple、needConfirm、disabledTime；DateTimePicker 无 DisabledDate/Presets | entry_daterange.go `RangePickerW.Picker`（周/月/季/年）、`Variant`；可键盘输入：date.go `dtTyping`、entry_daterange.go `typed`、p2a_date.go `p2aParsePeriod`；`DateTimePickerW` 无 `DisabledDate/Presets` |
| Form | `Form`/`FormItem`、`FormList` | ✅ | 次要：无 labelCol/wrapperCol（只有 LabelWidth） | p2d_form.go `ValidateFields(names…)`、`ClearValidate(names…)`、`Disabled`（表单级）、`RequiredMark`、`ResetFields(names…)`、`FormItemW.OnReset`（RulesFunc 项由回调重置）；form.go `Validate/ResetValidation/ScrollToFirstError` |
| Input | `Input`、`TextArea`、`Search`、`OTP` | ✅ | 次要：TextArea/Search/OTP 无 Variant；OTP 无 separator/formatter | input.go `AllowClear/Password/Prefix/Suffix/ShowCount/MaxLength/Status`；p2d_input.go `Variant`（4 种） |
| InputNumber | `InputNumber` | ✅ | 次要：无 suffix；不能隐藏控制按钮 | input.go `ControlsPosition`（right/sides）、`Variant`、`Wheel` |
| Mentions | `Mentions` | ⚠️ | 无 AllowClear、Variant | entry_mentions.go `Remote(func(prefix, query))/Debounce/Loading`、`Query()`、`Prefix/Rows/Selected…` |
| Radio | `Radio`、`RadioGroup` | ✅ | 次要：整组不能 Disabled | choice.go `Buttons/Solid/Size/Changed` |
| Rate | `Rate` | ✅ | — | entry_rate.go `AllowHalf/AllowClear/Character/Count/Texts` |
| Select | `Select`、`MultiSelect`、`InputTag`、`VirtualSelect` | ⚠️ | 无 Variant、prefix/suffixIcon | p2b_select.go `Remote/FilterMethod/AllowCreate/Virtual/MaxCount`；fill_virtual_select.go `FilterMethod` |
| Slider | `Slider`、`SliderRange` | ✅ | 次要：无 reverse/included | p2b_slider.go `AllowCross`（默认可交叉）、`Pushable(gap)`；`WithInput` 支持范围与竖向（注释写明各输入框位置） |
| Switch | `Switch` | ✅ | — | choice.go `Small/Loading/Texts` |
| TimePicker | `TimePicker`、`TimePanel`、`TimeRangePicker` | ✅ | 次要：无 needConfirm | p2a_time_range.go `TimeRangePicker` 注释"Each end can also be typed"、`typed`；`HideDisabledOptions`（TimePicker、TimeRangePicker、TimePanel 都有） |
| Transfer | `Transfer` | ✅ | 次要：无自定义 filterOption、footer | p2b_transfer.go `pager` 改用 `Pagination(...).Simple().Small()`；`Render/OneWay/TreeSource/Virtual` |
| TreeSelect | `TreeSelect` | ⚠️ | 无 treeCheckStrictly、filterTreeNode | p2b_treeselect.go `ExpandedKeys(*[]string)`（受控展开）/`ExpandChanged`、`StrategyValue`（绑定值随勾选策略）、`LoadData`、`Virtual` |
| Upload | `Upload`、`DropZone` | ⚠️ | 无 listType=picture；无手动 submit | entry_upload.go `upText/upCard/upDrag` 三种；照片墙已有重试（`upCardIconRow` 预览/重试/删除）；p2d_upload.go `CustomRequest`、`BeforeUpload/Action/Retries` |
| **数据展示** | | | | |
| Avatar | `Avatar`、`AvatarGroup` | ✅ | — | display.go `Size/Square/Image/Icon`；data_badge.go `AvatarGroupW.Max` |
| Badge | `Badge`、`BadgeRibbon` | ✅ | — | data_badge.go `Count/Dot/Max/ShowZero/Status/Offset` |
| Calendar | `Calendar` | ✅ | 次要：无 headerRender、validRange | data_calendar.go `Card/Mode/CellRender/MonthCellRender/NoHeader` |
| Card | `Card`、`CardMeta`、`CardAction` | ⚠️ | 无 `tabList`（卡片内页签） | data_card.go `Extra/Cover/Actions/Inner/Loading`；grep `tabList` 为 0 |
| Carousel | `Carousel` | ✅ | 次要：无 dot 进度条 | data_carousel.go `Draggable`（拖拽/滑动翻页）、`SwipeThreshold`、`Loop(false)`；`Fade/Vertical/DotPosition/Autoplay` |
| Collapse | `Collapse` | ✅ | 次要：无 `collapsible="header"/"icon"` | data_collapse.go `Accordion`、`CollapseItem{Disabled,NoArrow,Extra}` |
| Descriptions | `Descriptions` | ✅ | — | data_card.go `ColumnAt(bp, n)`、`Columns(map[Breakpoint]int)`，按 data_util.go `dataRespCols.resolve` 取断点 |
| Empty | `Empty` | ✅ | — | data_result.go `Simple/Image/ImageSize/Description` |
| Image | `Image` | ✅ | 次要：无 toolbarRender/imageRender | data_image.go `PreviewGroup`，带计数、旋转、翻转、缩放 |
| List（v6 已 deprecated） | `List`、`ListItemMeta`、`ListActions` | ✅ | — | data_list.go `GridAt(bp, columns)`（断点列数）、`Grid(columns, gap)` |
| Listy | `Listy` | ✅ | — | fill_listy.go `Virtual/Group/Sticky/ScrollToKey/ScrollToGroup/EndReached` |
| Popover | `Popover` | ✅ | — | data_popover.go `Title/Content/Trigger/Placement/Open` |
| QRCode | `QRCode` | ✅ | — | data_qrcode.go `Level/Icon/Status/Refreshed/Bordered` |
| Segmented | `Segmented` | ⚠️ | 无 v6 的 `orientation="vertical"`、`shape="round"` | entry_segmented.go 只有 `Icons/Block/Size/Disabled` |
| Statistic | `Statistic`、`Countdown` | ⚠️ | 无 `Statistic.Timer type=countup`；Countdown 无 onChange | display.go `Statistic`；data_skeleton.go `CountdownW.Format/Prefix/Suffix/Finished`；`CountUp` 只是入场动画 |
| Table | `Table`（及 `ProTable`、`EditableProTable`） | ✅ | 次要：列虚拟化未确认；汇总格只能返回字符串 | table*.go `Sortable/MultiSort/Filters/FilterPanel/Fixed/ScrollY/Sticky/Expandable/Tree/TreeLazy/Selection/CheckStrictly/Span/Summary/Virtual/Resizable/TableEditor/ServerPagination/Children/DragSort/Wrap` |
| Tag | `Tag`、`CheckableTag` | ✅ | — | display.go `Closable/Color/Icon/Borderless`；more_checktag.go |
| Timeline | `Timeline` | ⚠️ | 无 v6 的 `orientation="horizontal"` | data_timeline.go:46 `Mode` 只有 left/right/alternate |
| Tooltip | `Tooltip` | ✅ | — | display.go `Color/Arrow/MouseEnterDelay/Trigger` |
| Tour | `Tour` | ⚠️ | 每步不能单独设置 mask/type/按钮；无 indicatorsRender/actionsRender；是否自动把目标滚进可见区未确认 | data_popover.go:408 `TourStep{Target,Title,Description,Cover,Placement}`；`TourW.Primary/NoMask/NoArrow/Gap` |
| Tree | `Tree`（另有 `VirtualTree`） | ✅ | 次要：无 titleRender、allowDrop | data_tree.go `Height(px)`：注释"builds only the rows in view (Ant height / virtual)"；`Draggable/Dropped/LoadData/Search/Filter/CheckStrictly` |
| **反馈** | | | | |
| Alert | `Alert` | ✅ | 次要：不能自定义图标 | feedback.go `Action/Description/NoIcon/Closable/Banner` |
| Drawer | `Drawer` | ✅ | — | feedback.go `Resizable`（边缘可拖，也是可聚焦的分隔条）、`SizeLimits/BindSize/Resized`；嵌套 Drawer 推开父级（`push` 动画，注释"pushes the parent aside"） |
| Message | `Message`、`MessageDestroy` | ✅ | 次要：只能在顶部，无 top 偏移；maxCount 只在 `App` 内（`UseApp().Message`）生效 | nav_notify.go:166 `Place: ScreenTop`；`MessageRef.Key/Duration`；pro4_app.go `AppMessageConfig.MaxCount` |
| Modal | `Modal`、`Confirm`、`MessageBox`、`Prompt` | ✅ | 次要：即时模式下用不上 useModal；ConfirmOptions 不能自定义 icon/footer | feedback.go `Draggable/Fullscreen/ConfirmLoading/Centered`；more_confirm.go `ConfirmOptions` |
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
| ConfigProvider | `ConfigProvider` | ⚠️ | `Components`（组件令牌）与 `Direction`（RTL）已接入（见 rtl-and-component-tokens.md）；`Defaults` 接线到 106/164 个公开组件（见第 6 节） | nav_config.go `Size/Locale/Defaults/ComponentDisabled/Theme/Flavor/Dark/PrimaryColor/Compact/HighContrast` |

## 3. Element Plus（79）

不计入的设计规范页：Border、Color、Typography（Typography 页的字体规范由 `theme` 令牌与 `Text/Title` 体现）。

| 组件 | Tango 入口 | 状态 | 缺的能力 / 次要缺口 | 代码证据 |
|---|---|---|---|---|
| **Basic** | | | | |
| Button | `Button` | ⚠️ | 无 type=success/warning/info、`plain` 全套、`ButtonGroup` | button.go 只有 Primary/Dashed/Text/Link/Secondary/Danger；grep `ButtonGroup` 为 0 |
| Layout Container | `Layout/Header/Sider/Content/Footer` | ✅ | — | prolayout.go |
| Icon | `Icon` | ✅ | — | text.go `IconW` |
| Layout（row/col） | `GridRow`、`Col` | ✅ | — | nav_grid.go |
| Link | `Link` | ✅ | 次要：无 icon | text.go `LinkW.colors` 按 `Type` 取 success/warning/danger/info 调色板（悬停、按下随之变化），`Color` 可固定颜色 |
| Text | `Text` | ✅ | — | text.go `Type/Size/Ellipsis/Lines` |
| Scrollbar | `Scroll`、`ScrollH` | ✅ | — | scroll.go `Always/Both/ScrollTo/Offset` |
| Space | `Space` | ✅ | — | nav_space.go `Split/Wrap/Size` |
| Splitter | `Splitter` | ✅ | — | nav_splitter.go `Lazy`、`CollapseToggle`（单方向折叠） |
| **Configuration** | | | | |
| Config Provider | `ConfigProvider` | ⚠️ | 同 Ant 那一行；Element 的 button.autoInsertSpace、message.max 等无对应 | nav_config.go |
| **Form** | | | | |
| Autocomplete | `AutoComplete` | ⚠️ | 无 trigger-on-focus（异步取数用 `Remote/Debounce/Loading`） | entry_autocomplete.go |
| Cascader | `Cascader`、`CascaderPanel` | ✅ | 次要：无 show-all-levels/separator、filter-method | entry_cascader_panel.go `CascaderPanel`（内嵌面板，`Multiple/LoadData/ShowCheckedStrategy/StrategyValue/Height/Border`）；懒加载与勾选策略见 Ant 那一行 |
| Checkbox | `Checkbox`、`CheckboxGroup` | ⚠️ | 无 checkbox-button、border 样式；Group 无 min/max；无 size | choice.go:304 `CheckboxGroup` |
| Color Picker Panel | `ColorPickerPanel` | ✅ | — | fill_color_panel.go `Presets/PresetGroups/Format/NoAlpha/AllowClear/Border` |
| Color Picker | `ColorPicker` | ✅ | 次要：无 hsl 格式 | entry_color.go `cpFormatOK`（hex/rgb/hsb/hsv） |
| Date Picker Panel | `DatePickerPanel` | ✅ | — | fill_date_panel.go `PickerType`（日/周/月/季/年）、`Range` |
| Date Picker | `DatePicker`、`RangePicker` | ⚠️ | 无 dates/months 多选 | entry_daterange.go `Picker`（monthrange/yearrange）；可键盘输入，同 Ant DatePicker 那一行 |
| DateTime Picker | `DateTimePicker`、`RangePicker.ShowTime` | ⚠️ | 无 disabled-date、shortcuts | `DateTimePickerW` 无 `DisabledDate/Presets`；entry_daterange.go:158 `ShowTime` |
| Form | `Form` | ✅ | 次要：只有校验失败时滚动（`ScrollToFirstError`），无按字段 scrollToField | p2d_form.go `ValidateFields/ClearValidate/RequiredMark/Disabled/ResetFields`、`FormItemW.OnReset` |
| Input | `Input`、`TextArea` | ✅ | — | input.go、entry_inputx.go |
| Input Number | `InputNumber` | ✅ | 次要：不能 `controls=false` | input.go:719 `ControlsPosition` |
| Input Tag | `InputTag` | ⚠️ | 分隔符固定为英文/中文逗号；无 draggable、save-on-blur | entry_inputtag.go:124 |
| Input OTP | `OTP` | ✅ | 次要：无 separator、formatter | entry_otp.go `Mask/Numeric/Status/Size/Gap` |
| Mention | `Mentions` | ⚠️ | 无 whole 整体删除 | entry_mentions.go（`Remote/Loading` 对应 search/loading） |
| Radio | `Radio`、`RadioGroup` | ⚠️ | 无 border 样式；整组不能 disabled（radio-button 由 `Buttons` 提供） | choice.go:481-494 |
| Rate | `Rate` | ✅ | 次要：无 low/high 阈值分段配色 | entry_rate.go `Color(theme.Color)` |
| Select | `Select`、`MultiSelect` | ✅ | 次要：无 prefix | p2b_select.go `SelectW.FilterMethod`、`MultiSelectW.FilterMethod` |
| Virtualized Select | `VirtualSelect` | ✅ | — | fill_virtual_select.go `Filterable/FilterMethod/Remote/Multiple/MaxTagCount` |
| Slider | `Slider`、`SliderRange` | ✅ | — | p2b_slider.go `WithInput`（范围与竖向都可带输入框）、`AllowCross/Pushable` |
| Switch | `Switch` | ⚠️ | 无 active-value/inactive-value、before-change、inline-prompt、large 尺寸、width | choice.go `Small/Loading/Texts` |
| Time Picker | `TimePicker`、`TimeRangePicker` | ✅ | — | p2a_time_range.go `typed`（范围版可键盘输入） |
| Time Select | `TimeSelect` | ✅ | — | more_timeselect.go `Start/End/Step/MinTime/MaxTime/Clearable` |
| Transfer | `Transfer` | ✅ | 次要：无 filter-method、target-order、format | p2b_transfer.go `pager` 复用 `Pagination`；`Render/OneWay/TreeSource/Virtual` |
| TreeSelect | `TreeSelect` | ⚠️ | 无 check-strictly、filter-node-method | p2b_treeselect.go `ExpandedKeys`（覆盖 default-expanded-keys，且可受控）；entry_treeselect.go `DefaultExpandAll/Searchable` |
| Upload | `Upload` | ⚠️ | 无 auto-upload=false + submit()、list-type=picture | p2d_upload.go `CustomRequest`（对应 http-request）；照片墙已有重试（entry_upload.go `upCardIconRow`） |
| **Data** | | | | |
| Avatar | `Avatar` | ✅ | — | display.go |
| Badge | `Badge` | ✅ | — | data_badge.go |
| Calendar | `Calendar` | ⚠️ | 无 `range` | data_calendar.go `elHeader` |
| Card | `Card` | ✅ | — | data_card.go `Shadow/Footer` |
| Carousel | `Carousel` | ✅ | 次要：无 motion-blur | data_carousel.go `Draggable/SwipeThreshold`、`Card/Vertical` |
| Collapse | `Collapse` | ✅ | — | data_collapse.go `Accordion/IconEnd/IconStart` |
| Descriptions | `Descriptions` | ✅ | 次要：无 rowspan | data_card.go `DescItem{Span}`、`ColumnAt/Columns`（断点列数） |
| Empty | `Empty` | ✅ | — | data_result.go |
| Image | `Image` | ✅ | 次要：无视口懒加载（`c.lazy` 只是延迟构建） | data_image.go |
| Infinite Scroll | `InfiniteScroll` | ✅ | — | more_infinite.go `Distance/Loading/Done/Disabled` |
| Pagination | `Pagination` | ⚠️ | 无 `layout` 自由排序、`pager-count` | nav.go（grep 为 0） |
| Progress | `Progress` | ✅ | — | data_progress.go |
| Result | `Result` | ✅ | — | data_result.go |
| Skeleton | `Skeleton` | ✅ | — | data_skeleton.go |
| Table | `Table` | ✅ | — | table*.go，另有 `HighlightCurrentRow`、`Ellipsis` |
| Virtualized Table | `Table.Virtual`、`VirtualList`、`Listy` | ⚠️ | 有行虚拟化；列虚拟化未确认 | table.go:377 `Virtual`；more_virtual.go |
| Tag | `Tag`、`CheckableTag` | ✅ | — | display.go、more_checktag.go |
| Timeline | `Timeline` | ✅ | — | data_timeline.go `TimelineItem{Time,TimeTop,Hollow,Large}` |
| Tour | `Tour` | ⚠️ | 同 Ant 那一行 | data_popover.go |
| Tree | `Tree` | ✅ | 次要：无 render-content、allow-drop、accordion | data_tree.go `Height`（虚拟滚动）、`Filter`（filter-node-method） |
| Virtualized Tree | `VirtualTree` | ✅ | — | fill_virtual_tree.go `Height/NodeRender/Range` |
| Statistic | `Statistic`、`Countdown` | ✅ | — | display.go、data_skeleton.go:236 |
| Segmented | `Segmented` | ⚠️ | 无 direction=vertical | entry_segmented.go |
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
| Alert | `Alert` | ⚠️ | 无 `effect="dark"`、`center` | feedback.go:385-411 |
| Dialog | `Modal` | ✅ | before-close 靠应用控制 `open` 实现 | feedback.go |
| Drawer | `Drawer` | ✅ | before-close 同 Dialog，靠应用控制 `open` | feedback.go `Resizable/SizeLimits/BindSize` |
| Loading | `Loading` | ✅ | — | feedback.go:295 `Tip/Background/Fullscreen` |
| Message | `Message` | ⚠️ | 无 showClose、placement（只在顶部）、offset、plain；grouping 用 `Key` 原地更新代替 | nav_notify.go:69/166 |
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
| ProLayout | `ProLayout(c, ProLayoutConfig, fn)` | ⚠️ | 面包屑只能静态传入，不会按路由自动生成；无 menuHeaderRender/headerRender 等钩子；DefaultFooter 只有一行文字（无 links/copyright 结构）；sider 总是固定（fixSiderbar 只做透传） | prolayout.go `ProLayoutConfig{Layout side/top/mix, Menu, Selected, Collapsed, LightSider, Breadcrumb []string, Actions, Footer string, Breakpoint, SiderWidth, ContentWidth Fluid/Fixed, NoFixedHeader, NoPadding}` |
| SettingDrawer | `SettingDrawer`、`ProSettingsProvider`、`ProSettings.Apply` | ✅ | 次要：色弱模式用 `theme.HighContrast` 近似（没有 CSS filter） | pro4_setting.go `SettingDrawerW.Open/NoTrigger/Colors/Changed/Show`；`ProSettings{NavTheme,PrimaryColor,Layout,ContentWidth,FixedHeader,FixSiderbar,ColorWeak,Compact}` |
| RouteContext / GridContent | `RouteContext`、`GridContent` | ✅ | — | pro4_route.go `RouteInfo{InLayout,Layout,Collapsed,IsMobile,HasSiderMenu,Breadcrumb,PageTitle,Selected,ContentWidth,FixedHeader}`；`GridContentW.ContentWidth/MaxWidth`（Fixed 居中 1200px） |
| PageContainer | `PageContainer` | ✅ | — | pro2_page.go `Tabs/TabExtra/TabsCard/Extra/Breadcrumb/Back/Tags/SubTitle/Content/ExtraContent/Footer/Loading/Ghost` |
| FooterToolbar | `FooterToolbar` | ⚠️ | **无毛玻璃**：只有 95%/96% 不透明背景加阴影，没有 backdrop blur（仓库里也没有背景模糊绘制原语） | pro2_page.go 约 95-103 行 `b.Bg(theme.Fade(th.BgContainer, 0.96))`（shadcn 0.95）；`Extra/Sticky/Target` |
| ProCard | `ProCard` | ⚠️ | 无 `actions` 底部操作栏（`Extra` 放在头部右侧） | pro2_card.go `Split/Direction/Gutter/Wrap/ColSpan/Tabs/Collapsible/HeaderBordered/Ghost/Centered/Loading/Hoverable` |
| StatisticCard | `StatisticCard`、`StatisticCardGroup` | ✅ | 次要：无独立的 `StatisticCard.Divider`（Group 用 Split 自动加分隔） | pro2_statistic.go `Trend/Chart/ChartPlace/Status/Icon/Desc/Footer`、:303 Group |
| CheckCard | `CheckCard`、`CheckCardGroup` | ✅ | — | pro2_checkcard.go `Avatar/Disabled/Size/Cover/Loading`；Group `Value/Values` |
| WaterMark | `Watermark` | ✅ | 次要：zIndex/fontFamily 未确认 | data_watermark.go |
| ProTable | `ProTable[T]` | ✅ | 次要：列 valueType 只有 11 种 | table_pro.go `Params`（外部参数，变化回第 1 页）、`Polling/PausePolling/ResumePolling`、`ColumnsState/OnColumnsStateChange/PersistColumnsState`、`TableAlertRender/TableAlertOptionRender/NoTableAlert`；`Request/ProColumn{ValueType,HideInSearch,HideInTable,Fixed}/ToolBar/NoSearch`，选项区有刷新/密度/列设置/全屏 |
| EditableProTable | `EditableProTable[T]` | ✅ | — | p2e_editable.go `NewRow/AddAtTop/MaxLength/EditableKeys/Multiple/RowEditable/OnSave/OnDelete` |
| DragSortTable | `ProTable.DragSort`、`EditableProTable.DragSort` | ✅ | 以方法形式提供，没有独立入口；自定义拖拽把手未确认 | table_pro.go:364（注释 "Ant Design Pro DragSortTable"）；p2e_editable.go:173 |
| ProList | `ProList[T]` | ✅ | — | pro3_list.go `ProListMetas{Title,SubTitle,Description,Avatar,Content,Actions,Extra}/Selection/Expandable/Grid/Request/Pagination` |
| ProDescriptions | `ProDescriptions` | ✅ | — | pro3_descriptions.go `Request/Editable/ProDescItem{ValueType,ValueEnum,Span,Render}` |
| ProForm | `ProForm(c, []ProField)` | ⚠️ | 无 `readonly` 只读模式 | form_pro.go `Columns/Horizontal/Request/OnFinish/Submitter/Children/Gutter` |
| ModalForm / DrawerForm | `ModalForm`、`DrawerForm` | ✅ | — | form_pro_modal.go:35/51 |
| QueryFilter | `QueryFilter` | ✅ | — | form_pro_query.go `DefaultExpanded/Searched/Params` |
| LightFilter | `LightFilter` | ✅ | — | pro3_lightfilter.go `Collapse/Bordered/Size/Params` |
| StepsForm | `StepsForm` | ✅ | — | form_pro_steps.go |
| LoginForm / LoginFormPage | `LoginForm`、`LoginFormPage` | ✅ | — | pro3_login.go `OnSendCode/CodeInterval/AccountOnly/MobileOnly/BackgroundImage/Side` |
| ProFormList | `FormList[T]` | ✅ | 写法不同：通过 `ProForm.Children` 嵌入 | form_pro_list.go:54 `Min/Max/Creator/MoveUp/MoveDown` |
| ProFormFieldSet | `ProFormFieldSet`、`FieldSet` | ✅ | — | pro3_fieldset.go:36/60 |
| ProFormDependency | `ProField.Dependencies` + `Hide/Disable/Options` 闭包 | ✅ | 等价实现，不是组件形式 | form_pro_field.go:167-170 |
| ProFormGroup | `ProFormGroup`、`ProField.Group` | ✅ | — | pro4_group.go `ProFormGroupW.Description/Extra/Collapsible/DefaultCollapsed/Collapsed/Columns/Gutter/Body`、`ProFormItems`；form_pro_field.go `ProField.Group`（连续同组字段自动包成 ProFormGroup） |
| SchemaForm（BetaSchemaForm） | 把 `[]ProField` 传给各表单入口 | ⚠️ | 本身就是 schema 驱动，但没有 `layoutType` 统一入口；schema 里写不了 FormList（分组已可用 `ProField.Group`） | ProForm/ModalForm/StepsForm/QueryFilter 都接收 `[]ProField` |
| ProForm 字段组件（ProFormText/Select…） | `ProField.ValueType` | ⚠️ | 没有独立字段组件；缺 dateWeek/Month/Quarter/Year、dateTimeRange、image、avatar、code/jsonCode、fromNow、second、index、captcha 等 valueType | form_pro_field.go:19-37（19 种）；table_pro.go:34-44（11 种） |
| ProField | `ProField` 结构体、`ProFormat()`/`ProFormatLocale()` | ⚠️ | 无可单独渲染的组件、无 mode=read/edit 切换 | form_pro_field.go:144；table_pro.go:1298 `ProFormat`、:1304 `ProFormatLocale`（货币见下） |
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
  - 复核：代码已处理，data_carousel.go 卡片模式给每张卡片设 `aKbd`（注释：键盘经指示点或可聚焦外框的方向键到达同一张），a11y_audit.go 据此不再报 not-keyboard。没有重跑审计。
- 对比度：6 个内置主题 0 项失败；35 项设计默认值低于 WCAG AA（设计规范本身如此，HighContrast 能修正）；24 项豁免；HighContrast 下 0 项失败。

## 6. 非组件能力

| 能力 | 状态 | 说明与证据 |
|---|---|---|
| RTL | ✅ | 基础层与组件层均已完成（2026-11-13）：`ConfigProvider.Direction`/Locale 推断、布局镜像（`layout.Physical()` 逃生口）、逻辑滚动、逻辑浮层位置、方向性图标镜像、`TextAlign` Start/End、方向键换向；全部组件已迁移（输入/选择/日期/滑块/导航/菜单/表格/树/反馈/表单/编辑器 RTL 行对齐）。时间列、数字输入、代码固定 LTR。`Driver.MirrorCheck` 与 gallery `-dir rtl`、`-mirror-check` 用于回归。原遗留项均已完成：混排文本视觉光标移动（Q12 第二期，ui/editor.go `visualStep`）；a11y 方向标记（`a11y.Node.RTL`/`Tree.RTL`，Linux AT-SPI 文本属性 `direction` 已实现，Windows UIA / macOS NSAccessibility 无对应元素属性、记为不适用，见 docs/accessibility.md）；图标未镜像检查（`paint.List.IconNote` + `Driver.MirrorCheck`/`Audit`，ui/icon_dir.go）；mobile/chart 子包 RTL（A4，ui/mobile、ui/chart 的 rtl_test.go）。 |
| 组件级主题令牌（类似 `theme.components`） | ✅ | `theme.Components` 共 54 个组件令牌类型（`theme.NumComps`，Button … Form，ui/theme/comp_*.go），全部有派生默认值，对应组件均已改读令牌，零值继承与哨兵、组件 Alias 重派生、HighContrast 覆盖组件令牌、`CSSVarsWith`；`ConfigProvider.Components/Theme/Dark…` 嵌套局部生效；默认值与原绘制逐像素一致。共享下拉面板 `entStylePanel` 读 Select 令牌（含 `PopupRadius/PopupShadow`）；Menu 弹出子菜单读 Menu 令牌自己的 `Popup*` 字段，不受 Dropdown 覆盖影响；TimeSelect 选项读 Select 令牌的 Option 字段。`Theme` 上已有组件令牌对应的扁平组件字段标注 `Deprecated:`（仍作为派生来源保留）。 |
| ConfigProvider.Defaults 接线 | ⚠️ 106/164 ≈ 65% | 见 6.1 |
| 全局尺寸 | ⚠️ | `ConfigProvider.Size` → `c.ComponentSize()` / `defaultSize()` / `sizeOr()`，约 25 个文件读取：button、choice、data_card、data_collapse、data_list、date、entry_color、entry_datepanel、entry_daterange、entry_inputx、entry_mentions、entry_otp、entry_segmented、fill_color_panel、input、more_timeselect、nav、nav_dropdown、nav_steps、p2a_time_range、pro2_card、pro2_checkcard、pro3_lightfilter、select、selector、shad_toggle、table、feedback。另有大量组件读 `Defaults` 里的 "size"：Tag/Tabs/Pagination/Steps/Space/Form，以及 shad2a_defaults.go `shad2aSize`（Button、RadioGroup、Input 系、选择器等）、shad2a_tree.go `shad2aTreeSize`（Cascader、TreeSelect）、shad2c_defaults.go `s2cSize`（Spin、Steps、Toggle、ToggleGroup）、shad2d_util.go `proPropSize`（Pro 组件）。没读的有 Slider、Checkbox（本身也没有 Size）。 |
| 主题 / 设计语言 | ✅ | `theme.AntDesign`、`theme.ElementPlus`、`theme.Shadcn`；`ConfigProvider.Flavor/PrimaryColor`；图表用 `chart.RegisterStyle` 按风格注册。 |
| 暗色 | ✅ | `theme.Dark()`、`ConfigProvider.Dark` |
| 紧凑 | ✅ | `theme.Compact()`、`ConfigProvider.Compact` |
| 高对比度 | ✅ | `theme.HighContrast(th)`（ui/theme/highcontrast.go）、`Theme.HighContrast` 标志，对比度审计 `theme.ContrastIssues`，画廊 `-theme hc`。`ConfigProvider.HighContrast(on)`（nav_config.go，局部固定高对比度或标准变体；根级作用于窗口，宿主随之停止跟随系统）；宿主跟随系统高对比度见 uihost/contrast.go `ContrastSystem`（默认）。 |
| 语言 | ✅ | 四个内置包 `ZhCN/EnUS/ZhTW/Ja`（locale_packs.go）；`ConfigProvider.Locale/LocaleCode`、`SetDefaultLocale`、`LocaleByCode`；`TestLocalePacksComplete`、i18n_wiring_test.go、i18n_scan_test.go。货币按 ISO 代码和地区规则格式化（money.go `FormatMoney`，第 4 节）。 |
| ReduceMotion | ✅ | `RunnerOptions.ReduceMotion` / `Runner.SetReduceMotion`；`Anim/AnimFrom` 直接到终值；装饰性动效用 `c.still()`（anim.go:69），调用方有 data_badge、data_image、data_progress、data_skeleton、nav_util、text、data_carousel。 |
| 无障碍 | ✅ | 语义树 ui/a11y、ui/a11y_tree.go；审计 `Driver.Audit()`（a11y_audit.go:102）和 gallery `-audit`；键盘漫游 `rovingTabStop/rovingStep`；`c.TrapFocus`。本次审计只剩 CarouselCard 12 处键盘不可达（5.4），复核时代码已处理（`aKbd`），未重跑审计。 |
| 响应式断点 | ⚠️ | `Breakpoint` XS…XXL、`ScreenBreakpoint(c)`（nav_grid.go:39）。GridRow/Col（`XS…XXL`、`GutterAt`）、Masonry（`ColumnsAt`）、ProLayout/Sider（`Breakpoint`）、Descriptions（`ColumnAt/Columns`）、List 网格（`GridAt`）用到，后两者共用 data_util.go `dataRespCols`。**ProDescriptions（`Column(n)`）、ProList 网格（`Grid(n)`）、QueryFilter 列数仍不跟随断点。** |

### 6.1 ConfigProvider.Defaults 接线清单

- **统计方法**：grep `ConfigProp(`、`Prop(c, "X"`、`propString/propBool/propFloat/propInt64`、`p2bBool/p2bNum/p2bStr/p2bDur`、`p2bSelectDefaults`、p2a_date.go 的 `p.comp`，复核时另加 `proPropInt/proPropSize/proPropPageSize`（shad2d_util.go）、`shad2aSize/shad2aTreeSize`、`s2cSize/messageDuration`（shad2c_defaults.go）、`dataPropCols/dataPropSize`（data_util.go）、`formPropSize`，以及以组件名为参数的 `sizeDefaults("Input")`、`shadDefaults("InputTag")`、`ModalFormW.defaults("ModalForm")` 等，取出读取到的组件名。入口直接调用另一个已接线入口、或读别的组件键的，也算接线（与 MultiSelect 读 "Select" 键同一口径）。
- **分母**：164 个公开入口。即 `func Xxx(c *Context…)`（含 ProTable/EditableProTable/ProList 这类泛型入口），去掉 Row/Column/Div/StackBox/Custom/Spacer/Gap/Key/Disabled/Overlay/HoverDelay/SetHoverOpen/ScreenBreakpoint/OpenDialogs/MessageDestroy/NotificationDestroy/ConfigProvider 这些基础件；复核时新出现的入口里，再去掉工具函数和取值钩子 Prop/State/UseApp/RouteContext/CloseContextMenu/ContextMenuOpen/ProSettingsProvider/ProFormItems，以及 ContextMenuAt 的选项变体 ContextMenuAtWith。比上次多出的 6 个是 App、CascaderPanel、ContextMenuAt、GridContent、ProFormGroup、SettingDrawer。`FormList[T]`（读 "ProFormList" 键）的第一个参数是 `*FormW` 不是 `*Context`，不计入分母。

**已接线（106）**

Affix、Alert、Anchor、App、AutoComplete、Avatar、Badge、Breadcrumb、BreadcrumbItems（读 "Breadcrumb" 键）、Button、Calendar、Card、Carousel、Cascader、CheckCard、CheckCardGroup（读 "CheckCard" 键）、Collapse、CollapsibleSider（读 "Sider" 键）、ColorPicker、Confirm、DatePicker、DateTimePicker、Descriptions、Divider、Drawer、DrawerForm、Dropdown、EditableProTable、Empty、FloatButton、FooterToolbar、Form、GridContent、GridRow、Header、Image、Input、InputNumber、InputTag、LightFilter、List、LoginForm、LoginFormPage、Masonry、Mentions、Menu、Message、MessageBox（读 "Confirm"/"Modal" 键）、Modal、ModalForm、MultiSelect（读 "Select" 键）、Notification、OTP、PageContainer、PageHeader、Pagination、Popconfirm、Popover、ProCard、ProDescriptions、ProForm、ProFormFieldSet、ProFormGroup、Progress、ProLayout、ProList、Prompt（读 "Confirm"/"Modal" 键）、ProSkeleton、ProTable、QRCode、QueryFilter、RadioGroup（读 "Radio" 键）、RangePicker、Rate、Search、Segmented、Select、SettingDrawer、Sider、Skeleton、Slider、SliderRange（直接调用 Slider）、Space、Spin、Splitter、StatisticCard、StatisticCardGroup（直接调用 ProCard）、Steps、StepsForm、Switch、Table、Tabs、Tag、TextArea、Timeline、TimePicker、TimeRangePicker、TimeSelect、Toggle、ToggleGroup、Tooltip、Transfer、Tree、TreeSelect、Upload、VirtualSelect（读 "Select" 键）。

**未接线（58）**

- 通用 / 布局：Addon、BackTop、BorderBeam、Col、Compact、Content、Flex、FloatButtonGroup、Footer、Icon、IconButton、Kbd、Layout、Scroll、ScrollH、SplitterPanel、VDivider。
- 导航：AnchorSection、Command、ContextMenuAt。
- 录入：CascaderPanel、CheckAll、Checkbox、CheckboxGroup、ColorPickerPanel、DatePickerPanel、DropZone、Radio、TimePanel、MarkdownEditor。
- 展示：AvatarGroup、BadgeRibbon、CardAction、CardMeta、CheckableTag、Countdown、InfiniteScroll、Link、ListActions、ListItemMeta、Listy、Markdown、Paragraph、Result、RichText、SkeletonAvatar、SkeletonButton、SkeletonImage、SkeletonInput、Statistic、Text、Title、Tour、VirtualList、VirtualTree、Watermark。
- 反馈：Loading。
- Pro：FieldSet。

**说明**

- 未接线的组件里，有些通过全局 Size 间接受 ConfigProvider 影响，例如 Checkbox/Radio（choice.go）、ColorPickerPanel（fill_color_panel.go 用 `c.defaultSize()`）。
- 单个 `Radio` 不读 "Radio" 键（只有 `RadioGroupW.configDefaults` 读），所以 Radio 计为未接线、RadioGroup 计为已接线。
- **最值得补的**：Checkbox/CheckboxGroup 与单个 Radio；Panel 类（CascaderPanel/ColorPickerPanel/DatePickerPanel）复用对应选择器的键。

### 6.2 子包简述（不计入三家对标）

- `ui/chart`：Line/Area、Column/Bar、Pie/Donut、Scatter/Bubble、Radar、Gauge、Funnel、Heatmap、DualAxes、Tiny 系列、Liquid。三种风格都有样式（chart/style_shadcn.go），有无障碍测试（chart/a11y_chart_test.go）。
- `ui/mobile`：对标 Ant Design Mobile / Vant 两套令牌，共 61 个入口，截图 44 张（antd/vant × 亮/暗）。
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
| P0 | Tabs：shadcn 横向和所有竖向 Tabs 的溢出加"…"更多下拉；shadcn 下 Card/Closable/OnAdd/竖向给出统一画法 | 中 | 已完成（复核：nav.go `shadTrack`、竖向 `Height` 溢出；未重跑截图） |
| P0 | Link 的 `Type` 着色生效 | 小 | 已完成（复核：text.go `LinkW.colors`） |
| P0 | `ProFormat(ValueMoney)` 改为读 Locale/Currency，不再写死 "¥"；货币格式改成 ISO 代码加地区规则 | 小（前者）/ 中（后者） | 已完成（复核：money.go `FormatMoney`、table_pro.go `ProFormatLocale`，见第 4 节） |
| P0 | CarouselCard 的卡片键盘可达 | 小 | 已完成（复核：data_carousel.go 卡片 `aKbd`；未重跑审计） |
| P1 | RangePicker 支持 week/month/quarter/year 粒度（面板 `DatePickerPanel.Range().Picker()` 已有，接到触发器上）；DatePicker/RangePicker/TimeRangePicker 支持键盘输入（复用 DateTimePicker 的 `dtParse` 和输入框） | 中 | 已完成（复核：entry_daterange.go `Picker/typed`、p2a_time_range.go `typed`、date.go `dtTyping`） |
| P1 | ProDescriptions、ProList、QueryFilter 的列数跟随断点（Descriptions `ColumnAt`、List `GridAt` 已完成，可复用 data_util.go `dataRespCols`） | 小 | 已有断点基础设施 |
| P1 | Form 按字段校验、清除（validateFields(names)/clearValidate）；RulesFunc 支持 reset；表单级 Disabled | 中 | 已完成（复核：p2d_form.go `ValidateFields/ClearValidate/Disabled/OnReset`） |
| P1 | TreeSelect 受控展开；勾选策略改为真正影响绑定值（Cascader 同理）；Cascader 懒加载父节点不再按叶子处理 | 中 | 已完成（复核：p2b_treeselect.go `ExpandedKeys/StrategyValue`、p2b_cascader.go `StrategyValue`、entry_cascader.go `leaves`） |
| P1 | ConfigProvider：给 Checkbox/CheckboxGroup、单个 Radio、各 Panel 补 Defaults 接线（Button/Table/Card/Drawer/Message/Notification/Switch/Empty/ProTable 与 HighContrast 开关已完成） | 小 | 接线率已到 106/164 ≈ 65% |
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
