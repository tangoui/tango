# 无障碍（Accessibility）

Tango 的自绘 UI（`ui` 包）不使用原生控件，屏幕阅读器看不到画在画布上的按钮和文字。为此 ui 层每帧在布局之后生成一棵**平台中立的语义树**（`ui/a11y`），由宿主（`uihost` / 平台 canvas 视图）映射到各平台的无障碍 API：macOS NSAccessibility、Windows UI Automation、Linux AT-SPI、iOS UIAccessibility、Android AccessibilityNodeInfo。本文是 ui 层与平台桥接之间的约定：接口、更新协议、角色映射表、动作与事件。平台桥接本身另行实现。

组件作者的标注规则见 `ui/AUTHORING.md` 第 17 节。

## 1. 接口

```go
// ui 包：Window 的可选接口。窗口实现它即启用无障碍。
type AccessibilityHost interface {
	AccessibilityEnabled() bool                                  // 每帧开头调用，必须廉价
	AccessibilityUpdate(tree *a11y.Tree, changed []a11y.NodeID)  // 树有变化的帧之后调用
}

// 宿主把辅助技术的请求交给 Runner（UI 线程）：
func (r *Runner) PerformAction(id a11y.NodeID, act a11y.Action, arg string) error
func (r *Runner) AccessibilityTree() *a11y.Tree   // 最近一帧的树（关闭时为 nil）
func (r *Runner) EnableAccessibility(on bool)     // 强制开启（测试、自动化）
func (r *Runner) SetReduceMotion(on bool)         // 系统「减少动态效果」偏好变化时调用
```

`ui.Window`、`ui.Host` 的签名不变；不实现 `AccessibilityHost` 的宿主零开销。

### 1.1 开关与开销

- `AccessibilityEnabled()` 返回 false 时 Runner **不构建**语义树，组件里的标注只是几次字段赋值（`table_bench` 关闭无障碍时无回退，数据见第 8 节）。
- 平台检测到辅助技术接入（macOS 首次请求 `accessibilityChildren`、Windows 收到 `WM_GETOBJECT`/`UiaRootObjectId`、AT-SPI 总线 `IsEnabled`/`ScreenReaderEnabled`、iOS `UIAccessibilityIsVoiceOverRunning`、Android `AccessibilityManager.isEnabled`）后把标志置为 true 并调用 `Runner.Invalidate()`，下一帧即收到完整的树（所有节点都在 `changed` 里）。
- 辅助技术断开后返回 false，Runner 丢弃缓存的树。

### 1.2 更新协议

- 每帧（只有出帧时才有帧：静止界面不出帧）在 layout + paint 之后构建新树，与上一帧逐节点比较（全部字段，含子节点顺序），只有在有节点增删改、焦点变化或有播报时才调用 `AccessibilityUpdate`。
- `changed` 先列新增/修改的节点（树中深度优先顺序），再列被删除的节点：`tree.Node(id)` 返回 nil 的就是删除。
- `tree` 是只读快照，调用返回后仍有效（Runner 每帧新建一棵），桥接可以直接保存它作为代理对象的数据源。
- 两个回调都在 UI 线程执行；平台的无障碍查询如果来自其他线程（UIA 的 COM 调用、AT-SPI D-Bus），桥接应读取已保存的快照，不要回调 Runner。

### 1.3 树的结构

- `tree.Root` 是 `RoleWindow` 节点，覆盖窗口内容区（逻辑像素）。其子节点依次是页面内容，然后是各浮层（下拉、弹窗、消息）——与绘制叠放顺序一致，后面的在上层。
- 只有带语义的组件成为节点；布局行列、装饰被拍平，子节点挂到最近的语义祖先上。按钮、选项、标签页等「原子」角色的内部文字并入其名称，不单独暴露。
- **模态**：打开带 `Modal` 状态的对话框（Modal、Drawer、Confirm、移动端 Dialog/Popup…）时，对话框所在层以下的内容不出现在树里（等价于 `aria-modal`/`accessibilityViewIsModal`），键盘 Tab 也被限制在对话框内。
- `Node.Bounds` 是窗口内容区内的逻辑像素矩形（乘 `Window.Scale()` 得设备像素；转屏幕坐标由桥接按窗口位置换算）；完全落在窗口或任一裁剪/滚动祖先（`Clip`、`Scroll`、移动端 `ScrollView`）可见范围之外的节点带 `Offscreen` 状态（例如被底部标签栏挡住、滚出视口的列表行）；部分可见的节点不带，`Bounds` 仍是完整矩形。完全滚出视口的节点不再绘制其子树：它自己（若有语义）在树里，内部节点没有矩形、不在树里，对它执行 `scrollIntoView` 后才出现。
- `NodeID` 等于组件的 `ui.ID`，在组件存在期间稳定；列表项用 `ui.Key` 包裹时插入删除兄弟项也不变。

## 2. 节点字段

| 字段 | 含义 | 平台对应（举例） |
|---|---|---|
| `Role` | 角色（下表） | AXRole / ControlType / AtspiRole / traits / className |
| `Name` | 可访问名 | AXTitle、AXDescription / UIA Name / accessible-name / accessibilityLabel / contentDescription |
| `Description` | 补充说明：校验错误、帮助文字、图表摘要 | AXHelp / UIA HelpText、FullDescription / accessible-description / accessibilityHint / tooltipText 或 hintText |
| `Value` | 文本值（输入内容、已选项、"3 / 5"） | AXValue / ValuePattern / Text 接口 / accessibilityValue / text |
| `Placeholder` | 占位提示 | AXPlaceholderValue / UIA HelpText / placeholder-text 属性 / accessibilityPlaceholderValue / hintText |
| `HasRange`、`ValueNow/Min/Max/Step` | 数值范围（slider、progressbar、spinbutton、meter、separator） | AXValue+AXMinValue/AXMaxValue / RangeValuePattern / Value 接口 / adjustable / RangeInfo |
| `States` | 状态位（第 4 节） | |
| `Actions` | 支持的动作（第 5 节） | |
| `Live` | 实时区域：`polite` / `assertive` | AXLiveRegion / LiveSetting + LiveRegionChanged / live 属性 / UIAccessibility announcement / accessibilityLiveRegion |
| `Level` | 标题级别 1–6，树节点层级（1 起） | AXDisclosureLevel / Level / level 属性 / — / CollectionItemInfo |
| `PosInSet`、`SetSize` | 集合中的位置（1 起）/ 总数 | AXIndex / PositionInSet、SizeOfSet / posinset、setsize 属性 / — / CollectionItemInfo |
| `RowIndex/ColIndex`、`RowCount/ColCount` | 表格行列号（1 起，含未构建的虚拟行）与总数 | AXRowIndexRange / GridItemPattern、GridPattern / Table 接口 / UIAccessibilityContainerDataTable / CollectionInfo、CollectionItemInfo |
| `Sort` | 列头排序方向 | AXSortDirection / ItemStatus 或 TableItem / sort 属性 / — / — |
| `LabelledBy`、`DescribedBy`、`Controls`、`ActiveDescendant` | 节点关系 | AXTitleUIElement、AXLinkedUIElements / LabeledBy、ControllerFor / relation set / — / labeledBy |
| `RoleDescription` | 角色的本地化补充（"图表"、"颜色选择器"） | AXRoleDescription / LocalizedControlType / role-description 属性 / — / roleDescription |
| `Shortcut` | 键盘快捷键 | AXKeyShortcuts / AcceleratorKey / keyboard-shortcuts / — / — |
| `RTL` | 节点所在作用域从右到左排版（`tree.RTL` 是窗口默认方向） | — / —（无文本模式）/ Text 接口的 `direction` 文本属性 / — / — ，见 2.1 节 |

焦点：`tree.Focus` 是当前键盘焦点节点（0 表示无）；组合框、树、表格等自己持有焦点的组件同时给出 `ActiveDescendant`（桥接应把焦点事件发给它，例如 `NSAccessibilityFocusedUIElementChangedNotification`、`UIA_AutomationFocusChangedEventId`、AT-SPI `focus:` 事件）。

播报：`tree.Announcements` 是本帧需要一次性朗读的文字（`ui.Context.Announce`），macOS 用 `NSAccessibilityAnnouncementRequestedNotification`，Windows 用 `UiaRaiseNotificationEvent`，AT-SPI 用 `announcement` 信号（或对一个 live 节点发 `children-changed`），iOS 用 `UIAccessibilityAnnouncementNotification`，Android 用 `View.announceForAccessibility`。

### 2.1 文字方向（RTL）

- `a11y.Node.RTL` 取自节点构建时的作用域（`ConfigProvider(c).Direction(...)`），嵌套作用域可以与窗口不同；`a11y.Tree.RTL` 是窗口根作用域的方向（根窗口节点的 `RTL` 与之相同）。`ui/a11y_tree.go` 用节点的 env 填写，没有 env 的节点用根作用域。
- **坐标不再镜像**：`Bounds` 来自 `c.rects`，已经是布局镜像后的真实屏幕矩形，与命中测试一致。桥接层不得再次翻转矩形（VoiceOver 光标、命中测试、自动化点击都依赖它）。
- **子节点顺序不变**：`Children` 是构建顺序，也就是阅读顺序；RTL 下不要反转，也不要按 x 排序。
- 平台映射：
  - **Linux AT-SPI**：已实现。实现 Text 接口的节点（文本框、搜索框、数字框、文本、标题、段落、链接、可编辑组合框）新增 `GetAttributes`、`GetAttributeRun`、`GetAttributeValue`、`GetDefaultAttributes`、`GetDefaultAttributeSet`（签名按 at-spi2-core `xml/Text.xml`），返回一个覆盖全文的属性段 `{"direction": "ltr" | "rtl"}`（取值同 `ATK_TEXT_ATTR_DIRECTION`）。画布不暴露逐字符样式，所以只有这一个属性。没有 Text 接口的节点（按钮、列表项等）不暴露方向：`direction` 只是文本属性，不放进对象属性。
  - **Windows UIA**：未实现，当前不适用。UIA 元素没有方向属性；方向只通过文本属性 `UIA_FlowDirectionsAttributeId`（`FlowDirections_RightToLeft = 2`）经 `ITextRangeProvider::GetAttributeValue` 暴露，前提是元素实现 TextPattern。现有桥接（`canvas_a11y_windows.go`）只实现 Invoke、Toggle、Value、RangeValue、ExpandCollapse、SelectionItem、ScrollItem 等模式，没有 TextPattern；以后加 TextPattern 时在文本范围上按 `Node.RTL` 返回该属性即可。
  - **macOS NSAccessibility**：未实现。`userInterfaceLayoutDirection` 是 NSView / NSApplication 的属性，不是 NSAccessibility 元素的标准属性；NSAccessibilityElement 协议里没有布局方向属性，VoiceOver 根据内容与几何推断方向。为避免编造私有属性，这里不做映射。
- 只影响平台的空间导航，内容（名称、值、`ColIndex` 的逻辑序号、Slider 的 `Horizontal`）不变。

## 3. 角色映射表

列依次为：Tango 角色、macOS（`NSAccessibility` role，`/` 后为 subrole）、Windows UI Automation ControlType（`UIA_<X>ControlTypeId`）、AT-SPI role（`ATSPI_ROLE_<X>`）、iOS `UIAccessibilityTraits`（container 表示只做容器：`isAccessibilityElement = false`，实现 `accessibilityElements`）、Android `AccessibilityNodeInfo.className`（TalkBack 按类名播报角色；没有对应控件类的用 `android.view.View` 并通过 `AccessibilityNodeInfoCompat.setRoleDescription` 给出本地化角色名）。同一数据也以 `a11y.Mappings` 的形式提供给桥接代码，`ui/a11y` 的测试保证两者一致。

| 角色 | macOS | UIA | AT-SPI | iOS | Android |
|---|---|---|---|---|---|
| `window` | AXGroup | Pane | PANEL | container | android.view.View |
| `group` | AXGroup | Group | PANEL | container | android.view.ViewGroup |
| `text` | AXStaticText | Text | STATIC | staticText | android.widget.TextView |
| `heading` | AXHeading | Text | HEADING | header | android.widget.TextView |
| `paragraph` | AXStaticText | Text | PARAGRAPH | staticText | android.widget.TextView |
| `link` | AXLink | Hyperlink | LINK | link | android.widget.TextView |
| `button` | AXButton | Button | PUSH_BUTTON | button | android.widget.Button |
| `checkbox` | AXCheckBox | CheckBox | CHECK_BOX | button+toggleButton | android.widget.CheckBox |
| `radio` | AXRadioButton | RadioButton | RADIO_BUTTON | button | android.widget.RadioButton |
| `radiogroup` | AXRadioGroup | Group | GROUPING | container | android.widget.RadioGroup |
| `switch` | AXCheckBox/AXSwitch | Button | TOGGLE_BUTTON | button+toggleButton | android.widget.Switch |
| `textbox` | AXTextField | Edit | ENTRY | none (UITextInput) | android.widget.EditText |
| `searchbox` | AXTextField/AXSearchField | Edit | ENTRY | searchField | android.widget.EditText |
| `spinbutton` | AXIncrementor | Spinner | SPIN_BUTTON | adjustable | android.widget.EditText |
| `combobox` | AXComboBox | ComboBox | COMBO_BOX | button | android.widget.Spinner |
| `listbox` | AXList | List | LIST_BOX | container | android.widget.ListView |
| `option` | AXStaticText | ListItem | LIST_ITEM | button | android.widget.CheckedTextView |
| `menu` | AXMenu | Menu | MENU | container | android.widget.ListView |
| `menubar` | AXMenuBar | MenuBar | MENU_BAR | container | android.view.ViewGroup |
| `menuitem` | AXMenuItem | MenuItem | MENU_ITEM | button | android.view.View |
| `tab` | AXRadioButton/AXTabButton | TabItem | PAGE_TAB | button | android.view.View |
| `tablist` | AXTabGroup | Tab | PAGE_TAB_LIST | tabBar | android.widget.TabWidget |
| `tabpanel` | AXGroup | Pane | SCROLL_PANE | container | android.view.ViewGroup |
| `dialog` | AXGroup/AXApplicationDialog | Pane | DIALOG | container (accessibilityViewIsModal) | android.view.ViewGroup |
| `alertdialog` | AXGroup/AXApplicationAlertDialog | Pane | ALERT | container (accessibilityViewIsModal) | android.view.ViewGroup |
| `alert` | AXGroup/AXApplicationAlert | Group | NOTIFICATION | staticText | android.view.View |
| `status` | AXGroup/AXApplicationStatus | StatusBar | STATUS_BAR | staticText | android.view.View |
| `tooltip` | AXGroup/AXUserInterfaceTooltip | ToolTip | TOOL_TIP | staticText | android.widget.TextView |
| `slider` | AXSlider | Slider | SLIDER | adjustable | android.widget.SeekBar |
| `progressbar` | AXProgressIndicator | ProgressBar | PROGRESS_BAR | updatesFrequently | android.widget.ProgressBar |
| `table` | AXTable | Table | TABLE | container (dataTable) | android.widget.GridView |
| `grid` | AXTable | DataGrid | TABLE | container (dataTable) | android.widget.GridView |
| `row` | AXRow | DataItem | TABLE_ROW | container | android.view.View |
| `cell` | AXCell | DataItem | TABLE_CELL | staticText | android.view.View |
| `columnheader` | AXCell | HeaderItem | COLUMN_HEADER | header | android.view.View |
| `rowheader` | AXCell | HeaderItem | ROW_HEADER | header | android.view.View |
| `tree` | AXOutline | Tree | TREE | container | android.widget.ExpandableListView |
| `treeitem` | AXRow/AXOutlineRow | TreeItem | TREE_ITEM | button | android.view.View |
| `list` | AXList | List | LIST | container | android.widget.ListView |
| `listitem` | AXGroup | ListItem | LIST_ITEM | staticText | android.view.View |
| `image` | AXImage | Image | IMAGE | image | android.widget.ImageView |
| `navigation` | AXGroup/AXLandmarkNavigation | Group | LANDMARK | container | android.view.ViewGroup |
| `toolbar` | AXToolbar | ToolBar | TOOL_BAR | container | android.widget.Toolbar |
| `separator` | AXSplitter | Separator | SEPARATOR | adjustable (splitter only) | android.view.View |
| `scrollarea` | AXScrollArea | Pane | SCROLL_PANE | container | android.widget.ScrollView |
| `form` | AXGroup/AXLandmarkForm | Group | FORM | container | android.view.ViewGroup |
| `figure` | AXGroup | Group | CHART | image (AXChart descriptor) | android.view.View |
| `timer` | AXGroup/AXApplicationTimer | Text | TIMER | staticText+updatesFrequently | android.widget.TextView |
| `meter` | AXLevelIndicator | ProgressBar | LEVEL_BAR | staticText | android.widget.ProgressBar |

补充说明：

- `button` 带 `Toggleable` 时是切换按钮：macOS 用 `AXCheckBox/AXToggle`，UIA 加 TogglePattern，AT-SPI 用 `TOGGLE_BUTTON`，iOS 加 `selected` trait（iOS 17+ 用 `toggleButton`），Android 用 `android.widget.ToggleButton`。
- `checkbox`/`radio`/`switch` 的选中状态：macOS `AXValue` 0/1/2（mixed），UIA `TogglePattern.ToggleState`/`SelectionItemPattern`，AT-SPI `CHECKED`/`INDETERMINATE` 状态，iOS `selected` trait + `accessibilityValue`，Android `setCheckable/setChecked`。
- `textbox` 带 `Multiline` 时 macOS 用 `AXTextArea`、AT-SPI 用 `TEXT`；带 `Protected`（密码）时 macOS 用 `AXTextField/AXSecureTextField`、UIA 设 `IsPassword`、AT-SPI 用 `PASSWORD_TEXT`、Android 设 `setPassword(true)`；`Value` 对密码框为空。
- `dialog` 带 `Modal` 时 iOS 设 `accessibilityViewIsModal = true`，Android 给容器设 `setPaneTitle(Name)`，UIA 设 `IsDialog = true`。
- `grid` 与 `table` 的区别是单元格可用键盘移动焦点；行列号与总数见第 2 节。
- `figure`（图表）的 `Description` 是数据摘要；iOS 可进一步实现 `AXChart`（`accessibilityChartDescriptor`）。

## 4. 状态

| 状态 | 含义 | macOS | UIA | AT-SPI | iOS | Android |
|---|---|---|---|---|---|---|
| `disabled` | 不可用 | AXEnabled=NO | IsEnabled=false | 无 ENABLED/SENSITIVE | notEnabled | setEnabled(false) |
| `focusable` / `focused` | 可聚焦 / 有焦点 | AXFocused | IsKeyboardFocusable / HasKeyboardFocus | FOCUSABLE / FOCUSED | （焦点由 VoiceOver 管理） | setFocusable / setFocused、setAccessibilityFocused |
| `checkable` / `checked` / `mixed` | 选中状态 | AXValue | TogglePattern | CHECKABLE / CHECKED / INDETERMINATE | selected + value | setCheckable / setChecked |
| `selectable` / `selected` | 可选中 / 已选中 | AXSelected | SelectionItemPattern | SELECTABLE / SELECTED | selected | setSelected |
| `expandable` / `expanded` | 可展开 / 已展开 | AXExpanded、AXDisclosing | ExpandCollapsePattern | EXPANDABLE / EXPANDED | value「已展开/已收起」 | ACTION_EXPAND / ACTION_COLLAPSE |
| `toggleable` / `pressed` | 切换按钮 | AXValue | TogglePattern | PRESSED | selected | setChecked |
| `required` | 必填 | AXRequired | IsRequiredForForm | REQUIRED | hint | hintText |
| `invalid` | 校验失败（`Description` 为错误信息） | AXInvalid | IsDataValidForForm=false | INVALID_ENTRY | value/hint | setContentInvalid + setError |
| `readonly` | 只读 | AXEditable=NO | ValuePattern.IsReadOnly | READ_ONLY | — | setEditable(false) |
| `multiline` / `protected` / `editable` | 文本输入特性 | AXTextArea / AXSecureTextField | — / IsPassword | MULTI_LINE / — / EDITABLE | — | setMultiLine / setPassword / setEditable |
| `modal` | 模态对话框 | AXModal | IsDialog + 下层不暴露 | MODAL | accessibilityViewIsModal | setPaneTitle |
| `multiselectable` | 可多选 | — | SelectionPattern.CanSelectMultiple | MULTISELECTABLE | — | CollectionInfo selectionMode |
| `busy` | 加载中 | AXElementBusy | ItemStatus | BUSY | updatesFrequently | — |
| `offscreen` | 在可视区域外 | — | IsOffscreen | 无 SHOWING | — | setVisibleToUser(false) |
| `current` | 当前项（页码、步骤、面包屑） | AXARIACurrent | ItemStatus / AriaProperties | 无（用 SELECTED + description） | selected | setSelected |
| `haspopup` | 打开菜单、列表或对话框 | AXHasPopup | ExpandCollapsePattern | HAS_POPUP | — | — |
| `vertical` / `horizontal` | 方向 | AXOrientation | OrientationType | VERTICAL / HORIZONTAL | — | — |
| `visited` | 已访问链接 | AXVisited | — | VISITED | — | — |

## 5. 动作

宿主收到辅助技术的请求后调用 `Runner.PerformAction(id, action, arg)`：节点必须在最近上报的树里且声明了该动作，否则返回错误。动作在下一帧开始时执行（调用会请求一帧），组件把它当成已经支持的输入处理：

| 动作 | 组件收到的输入 | macOS | UIA | AT-SPI | iOS | Android |
|---|---|---|---|---|---|---|
| `press` | 点击（含 Enter/Space 的效果） | AXPress | Invoke / Toggle / SelectionItem.Select | Action "click" | accessibilityActivate | ACTION_CLICK |
| `increment` / `decrement` | 方向键（slider 为 →/←，其余 ↑/↓） | AXIncrement / AXDecrement | RangeValue.SetValue(±step) | Value.CurrentValue | accessibilityIncrement / Decrement | ACTION_SCROLL_FORWARD / BACKWARD |
| `expand` / `collapse` | 点击触发器（或 `aTarget` 指定的展开箭头） | AXPress / AXShowMenu | ExpandCollapse | Action "expand"/"collapse" | accessibilityActivate | ACTION_EXPAND / ACTION_COLLAPSE |
| `setValue` | 文本框整体替换为 `arg`；slider、数字框解析 `arg` 为数字 | 设置 AXValue | Value.SetValue / RangeValue.SetValue | EditableText.SetTextContents / Value | — | ACTION_SET_TEXT / ACTION_SET_PROGRESS |
| `focus` | 键盘焦点移到该节点（显示焦点环） | 设置 AXFocused | SetFocus | Component.GrabFocus | — | ACTION_FOCUS |
| `scrollIntoView` | 祖先滚动容器滚动到可见（滚动容器内的所有节点都提供；自己移动内容的容器用 `Box.A11yScroller()` 声明并在构建期读 `c.A11yScrollRequest(id)`） | AXScrollToVisible | ScrollItem.ScrollIntoView | Component.ScrollTo | — | ACTION_SHOW_ON_SCREEN |
| `dismiss` | Esc（关闭对话框、浮层、消息） | AXCancel | Window.Close | Action "dismiss" | accessibilityPerformEscape | ACTION_DISMISS |

`ui.Context.A11yAction(id)` 让不经过 `Interact` 的组件（手势区域内的移动端控件等）读取本帧的动作。

## 6. 键盘可达性与焦点

- 所有可交互组件都在 Tab 顺序中；Enter/Space 激活。组合控件使用漫游 Tab 停靠：Tab 只停在当前项（选中的单选、当前标签页），方向键在 radiogroup、tablist、listbox、menu、tree、grid、toolbar、segmented 内移动，Home/End 到首尾。
- Esc 逐层关闭浮层；焦点若在浮层内，会回到浮层的锚点（触发元素）。
- 对话框打开时焦点移入对话框，Tab/Shift+Tab 在对话框内循环（焦点陷阱，`Context.TrapFocus`），关闭后焦点回到打开前的元素。
- 焦点环只在键盘操作时显示（`Interaction.FocusVisible`），颜色令牌 `FocusOutline/FocusRing` 以及输入框聚焦边框 `ControlFocusBorder` 在 6 套内置主题中都达到 3:1（Element Plus 亮色 `#409eff → #3c94f0`、shadcn 亮色 `--ring #a3a3a3 → #949494` 两处为此微调，其余颜色保持设计规范原值）。
- 「减少动态效果」：`RunnerOptions.ReduceMotion` 或 `Runner.SetReduceMotion(true)` 时所有过渡直接到终值，骨架屏闪烁、徽标脉冲、轮播自动播放、通告栏滚动等装饰动效停止。宿主应读取系统设置（macOS `accessibilityDisplayShouldReduceMotion`、Windows `SPI_GETCLIENTAREAANIMATION`、GNOME `org.gnome.desktop.interface enable-animations`、iOS `UIAccessibilityIsReduceMotionEnabled`、Android `ANIMATOR_DURATION_SCALE == 0`）。

### 6.1 对比度分类与高对比度主题

`theme.CheckContrast(th)` 按 WCAG 2.2 给每个令牌组合一个 `Status`：

| Status | 含义 | 依据 |
|---|---|---|
| `ok` | 达到最低要求 | 正文与次要文字 4.5:1（1.4.3），大字 3:1；识别控件边界、状态或焦点的非文本元素 3:1（1.4.11、2.4.7）：输入框/复选框外框 `Border`、开关关闭态轨道、选中态主色、焦点环、聚焦边框 |
| `FAIL` | 必须达标的组合不达标，且颜色不是设计规范原值 | 审计失败（`theme.ContrastFailures`） |
| `DesignDefault` | 必须达标的组合不达标，但两种颜色都**等于**该设计系统官方色板的值（`Spec` 给出变量名，如 `colorTextPlaceholder`、`--el-border-color`、`--input`） | 报告数值和 `HighContrast` 的替代色，不算失败；`WithPrimary` 等品牌色或应用自行覆盖的颜色不属于此类。焦点指示（`ControlFocusBorder`、`FocusRing`、`FocusOutline`）永远不能列为 DesignDefault |
| `exempt` | 只报告不要求 | 禁用态控件和文字（1.4.3 / 1.4.11 对 inactive 组件的豁免）、纯装饰的分隔线和卡片/表格边框 `BorderSecondary`、`Split`（不承担识别控件边界的作用）、Logo（不是主题令牌，不检查） |

内置 6 套主题当前：0 条 FAIL，35 条 DesignDefault（Ant/Element 三级文字、占位符、链接、主按钮与危险色上的白字、校验错误文字、Element 表头文字、输入框边框、开关关闭态、Element 主色），每条的分类理由写在 `ui/theme/contrast.go` 的 `designSpec` 注释里。`theme.ContrastIssues` 返回 FAIL 与 DesignDefault（`Driver.Audit` 列出它们），`theme.ContrastFailures` 只返回 FAIL。

**高对比度变体**：`theme.HighContrast(th) *Theme` 返回 `th` 的副本（`th.HighContrast == true`，报告中标记为 `antd-hc` 等），所有必须达标和 DesignDefault 的组合都满足 WCAG AA（文字 4.5:1、非文本 3:1），亮暗主题都适用，也适用于 `WithPrimary`、shadcn 的 base/brand 选项和应用自定义过的主题。尺寸、字体、阴影、圆角、色相都不变：不达标的颜色以 2% 的步长朝主题自身的正文色、再朝黑色（暗色主题为白色）移动到刚好达标；主色和危险色作为整个色板移动（hover/active 跟随），派生令牌（链接、选中项、聚焦边框等）随之更新。暗色主题里危险色既是页面上的文字又是白字按钮的底色，没有同时满足两者的红色，此时按钮文字改为页面深色（`TextOnSolid`）。禁用态和装饰线保持不变。`go run ./examples/ui-gallery -theme hc` 把三种风格亮暗的 dashboard 与 Form、Switch、Checkbox 页各渲染默认版与 `-hc` 版（`hc/` 目录）供对比。

**系统「增强对比度」偏好（自动跟随）**：`uihost` 窗口默认跟随系统的增强对比度 / 高对比度设置：系统开启时窗口画 `theme.HighContrast` 处理过的主题，关闭时换回标准主题，切换即时生效。

平台侧是 Backend 的可选能力，写法与 `MotionPreference` 相同：

```go
// internal/platform（ext.go）：任意 goroutine 可调用；false 也表示平台无法判断。
type ContrastPreference interface {
	HighContrast() bool
}

// 偏好变化时后端经 HostEx.OnAppEvent 上报，无负载；读取新值用 HighContrast。
const AppContrastChanged AppEventKind = 201 // AppReduceMotionChanged 为 200
```

| 平台 | 读取 | 变化通知 | 验证情况 |
|---|---|---|---|
| macOS | `NSWorkspace.accessibilityDisplayShouldIncreaseContrast`（`darwin/contrast.go`） | `NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification`，与减少动态效果共用一个观察者（`watchDisplayOptions`） | 本机读到关闭状态；开启状态和变化通知未实机切换验证 |
| Windows | `SystemParametersInfo(SPI_GETHIGHCONTRAST)` 的 `HCF_HIGHCONTRASTON`（Windows 11 的对比度主题同样设置此标志，`windows/contrast_windows.go`） | 每次 `WM_SETTINGCHANGE` 重新读取，值变化才上报 | 仅交叉编译 |
| Linux | portal `org.freedesktop.appearance contrast == 1`（xdg-desktop-portal 1.19+，GNOME 47+ 由 `org.gnome.desktop.a11y.interface high-contrast` 提供），否则 GTK `gtk-theme-name` 名称含 `HighContrast`（`linux/contrast_linux.go`） | portal `SettingChanged` 与 `notify::gtk-theme-name`（复用主题监听） | 仅交叉编译；判定逻辑有单元测试 |
| iOS / Android | 未实现。可用来源：`UIAccessibility.isDarkerSystemColorsEnabled`、API 34+ `UiModeManager.getContrast()` | — | — |

Linux 的限制：既没有 portal `contrast` 键、GTK 主题又不叫 HighContrast 的桌面（GNOME 46 及更早只打开了 GSettings 开关、KDE Plasma 自带的高对比度配色方案）读不到开启状态，`HighContrast()` 返回 false。后端不直接读 GSettings，因为该 schema 可能不存在，而 GLib 在 schema 缺失时会直接终止进程。

**uihost 跟随**（`uihost/contrast.go`，接在 `follow.go` 的主题跟随上）：

- `Options.Contrast ContrastMode`：`ContrastSystem`（零值，跟随系统）、`ContrastStandard`（始终标准对比度）、`ContrastHigh`（始终高对比度）。
- 先按 `Flavor`、明暗、强调色和 `Customize` 生成主题，再套用 `theme.HighContrast`，所以明暗切换、强调色和应用的自定义都保留，并且一起提到 AA。明暗和对比度互相独立：系统切换明暗时仍沿用当前对比度，切换对比度时仍沿用当前明暗。
- 只对 uihost 自己生成主题的窗口生效。显式主题（`RunnerOptions.Theme`、`ThemeFixed`）不会被替换；应用之后自己 `Runner.SetTheme` 或根 `ConfigProvider` 改了窗口主题，该窗口就停止跟随主题，与明暗跟随的规则相同。`FollowSystemTheme(r, true)` 恢复跟随时，会同时应用当前的明暗和对比度。
- `uihost.SystemHighContrast(app)` 读取当前系统值（返回 on 和 ok）。`Options.ThemeFor(sys)` 不读系统对比度（`ContrastHigh` 时返回高对比度版本）；网页需要同一套令牌时用 `Options.ThemeForContrast(sys, high)`。

**ConfigProvider**：`ConfigProvider(c).HighContrast(true)` 让作用域使用当前主题的高对比度版本，`HighContrast(false)` 让作用域使用标准版本。不设置时继承外层：`Dark`、`Flavor`、`PrimaryColor`、`Compact` 重新生成主题时会保留外层的高对比度（以前会丢失）。根 provider 设置时作用于整个窗口，规则与 `Theme`/`Dark` 相同，uihost 随后停止跟随该窗口的主题。在高对比度作用域里使用 `HighContrast(false)` 时，主题会按风格、明暗和组件覆盖重新生成，外层主题里生成器无法还原的令牌自定义会丢失，这一点与 `Dark` 相同。

Windows 高对比度模式本身要求使用系统配色（`GetSysColor`）。`HighContrast` 保持应用风格，只保证 AA 对比度，是在系统配色之外的折中；完整照搬系统配色尚未实现。

## 7. 测试与审计

- `ui.Driver`：`EnableAccessibility()`、`AccessibilityTree()`、`A11yFind(role, name)`、`A11yFindAll`、`A11yFocused()`、`PerformAction(id, act, arg)`、`A11yPerform(role, name, act, arg)`、`A11yUpdates()`；`DriverOptions{Accessibility: true, ReduceMotion: true}`。
- `Driver.Audit()` 报告：缺可访问名的可交互节点（`missing-name`）、没有角色的指针目标（`no-role`）、Tab 不可达的控件（`not-keyboard`）、当前主题的对比度问题（`contrast`），以及从右到左作用域里未镜像的方向性图标（`unmirrored-icon`，警告：组件用了 `icon.Draw` 而不是随方向镜像的 `c.paintIcon`）。图标通过 `paint.List.IconNote` 记录：`Audit` 与 `Driver.MirrorCheck` 只在检查的那几帧给 Driver 的显示列表装上记录器，记录器属于单个列表，并行测试互不影响；正常运行时为 nil，没有开销。
- `Driver.MirrorCheck(page)` 的 `Icons` 列出 RTL 帧中未镜像的方向性图标（定位到包含图标的最深节点；LTR 作用域内的图标不报，`NoMirror` 节点或子树内的计入 `IconsSkipped`）；`go run ./examples/ui-gallery -mirror-check` 会一并打印。
- `go run ./examples/ui-gallery -audit [-only 名称]` 以 Ant Design、Element Plus、shadcn/ui 三种风格审计全部 gallery 页面（初始状态和 Setup 交互之后），可交互节点缺名必须为 0；内置 6 套主题的必须达标组合 FAIL 必须为 0（DesignDefault 与豁免单独列出，不算失败），它们的 `HighContrast` 版本任何类别都不得低于最低要求（`TestGalleryAccessibility`）。
- 对比度：`theme.CheckContrast(th)`、`theme.ContrastIssues(th)`、`theme.ContrastFailures(th)`、`theme.ContrastReport(theme.BuiltinThemes()...)` 按 WCAG 2.2 检查正文（4.5:1）、大字（3:1）、非文本（3:1）令牌组合，分类见 6.1 节。

## 8. 性能

Apple M4，`go test ./ui -bench 'TableVirtualScroll|FrameA11y' -benchtime 300x -benchmem`（10 万行 × 50 列虚拟表格，1440×900，2 倍缩放；机器同时有其他负载，时间波动较大，以分配数为准）：

| 基准 | 关闭无障碍 | 开启无障碍 |
|---|---|---|
| TableVirtualScrollNoRaster（构建+布局+显示列表） | 约 180–260 µs，4819 次分配/帧（与改动前相同） | 约 260–650 µs，5712 次分配/帧 |
| TableVirtualScroll（含 CPU 光栅化） | 约 3.6–10 ms，与改动前持平 | 约 4.1–6 ms |
| FrameA11y（300 个控件的表单页，每帧改一个节点） | 约 450 µs，9075 次分配 | 约 590–790 µs，9443 次分配 |

关闭时每个组件只多几次字段赋值，分配数与字节数不变；开启时的额外开销是每帧重建语义树与逐节点比较。

## 键盘陷阱与 Tab 捕获（CaptureTab）

默认情况下 Tab / Shift+Tab 只用于在可聚焦控件之间移动焦点，组件收不到。需要自己使用 Tab 的控件（代码编辑器缩进、终端）在获得焦点期间每帧调用 `c.CaptureTab(id)`，Tab 就会作为普通按键进入它的 `KeysFor`。为满足 WCAG 2.1.2（不得形成键盘陷阱），以下操作始终可以离开该控件：

- Ctrl+Tab / Ctrl+Shift+Tab：直接移动焦点；
- 先按 Esc 再按 Tab：移动一次焦点（中间按了其他键则取消）。

编辑器类组件可以额外提供「Tab 移动焦点」模式（比如 Ctrl+M 切换），切换后不再调用 `CaptureTab` 即可。
