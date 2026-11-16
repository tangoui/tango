# 第 3 批 · 组 I 菜单类 对照记录

对照日期：2026-11-15。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：antd@6.7.0（`antd/package/es`）、@rc-component/menu@1.6.1（`rc-menu/package/es`）、@rc-component/dropdown@2.0.1（`rc-dropdown/package`）、element-plus@2.14.8（`ep/package`）、shadcn-ui@e62c9435（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）、cmdk@1.1.1（`cmdk/package/dist`）、@radix-ui/react-navigation-menu@1.2.14（`radix-nav/package/dist`）。下文省略这些前缀。

本组测试：`ui/align_b3i_test.go`（行为）、`ui/theme/align_b3i_test.go`（令牌数值，断言值即源码值）；演示页 `examples/ui-gallery/page_b3i.go`（`MenuAlign`、`DropdownAlign`、`CommandAlign`）。

## Menu（含 shadcn NavigationMenu、Sidebar 外观）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| items：label / icon / disabled / danger / children / group / divider | antd `MenuItemType{danger, icon, title}`、`type: group\|divider`（antd/menu/interface.d.ts） | nav.go `MenuItem` | 已对齐 |
| danger 外观 | antd dangerItemColor colorError、dangerItemSelectedBg colorErrorBg；暗色 darkDangerItemColor colorError、darkDangerItemSelectedBg colorError（antd/menu/style/index.js:417-427,445-458、theme.js:101-131） | 只在弹出子菜单生效 | 已补齐（TestMenuDangerItem、TestMenuDarkDanger）：令牌 `DangerItemColor/DangerItemSelectedBg/DarkDangerItemColor/DarkDangerItemSelectedBg`，亮暗 inline / vertical / horizontal 都生效 |
| extra | rc-menu `extra`（rc-menu/es/interface.d.ts:34）；antd `-item-extra` paddingInlineStart padding，颜色 groupTitleColor（antd/menu/style/index.js:212、theme.js:53） | `MenuItem.Shortcut` 只在弹层显示 | 已补齐（gallery MenuAlign）：inline 项末端画 `Shortcut` 作为 extra，令牌 `ExtraColor/ExtraFontSize`；等价写法：`MenuItem.Shortcut` 即 extra（字符串） |
| title（收起时提示文字） | antd `title` | 提示用 Label | 等价写法：Label 即提示；单独 title 需加 `MenuItem` 字段，见共享请求 |
| mode | vertical / horizontal / inline | `Mode(MenuInline\|MenuVertical\|MenuHorizontal)` | 已对齐 |
| inlineIndent 24 | rc-menu `inlineIndent = 24`（rc-menu/es/Menu.js:71）；EP `--el-menu-level-padding` 20 | 写死 24 / 20 | 已补齐（TestMenuInlineIndent）：`InlineIndent(px)`、Defaults 键 `inlineIndent`、令牌 `InlineIndent`（Ant 24、Element 20） |
| inlineCollapsed / collapsedWidth | antd collapsedWidth controlHeightLG*2 = 80、collapsedIconSize fontSizeLG（style/index.js:433,440）；EP 64；shadcn `--sidebar-width-icon` 3rem | 非 Element 一律 80（shadcn 也是 80） | 已补齐（TestMenuShadcnSidebarMetrics）：令牌 `CollapsedWidth/CollapsedIconSize`；**变更**：shadcn 收起宽 80 → 48 |
| tooltip={false} | antd `tooltip?: false \| TooltipProps`（antd/menu/menu.d.ts） | 总是提示 | 已补齐（TestMenuTooltipOff）：`Tooltip(on)`、Defaults 键 `tooltip` |
| theme light / dark | antd `theme` | `Dark()`、Defaults `theme` | 已对齐（shadcn 单一侧栏外观，见 AUTHORING §5） |
| selectable | rc-menu `selectable = true`（Menu.js:64） | 点击叶子总是写选中 | 已补齐（TestMenuSelectableFalse）：`Selectable(on)`、Defaults 键 `selectable` |
| multiple / selectedKeys | rc-menu `multiple = false`、`selectedKeys[]` | `selected *string`（稳定层签名） | 暂不做（多选导航少见；需另加 `[]string` 绑定入口，等用户决定） |
| openKeys / defaultOpenKeys / onOpenChange | rc-menu（Menu.d.ts:20-21,49）；EP `default-openeds` | 只有 `DefaultOpen`，内部状态 | 已补齐（TestMenuOpenKeysControlled、TestMenuOutOfRangeKeys）：`OpenKeys(*[]string)`、`OpenChanged()`；不在 items 中的 key 忽略 |
| accordion | EP `unique-opened` | `Accordion()` | 已对齐 |
| triggerSubMenuAction | rc-menu `'hover'`（Menu.js:76）；EP `menu-trigger` hover | 只有 hover | 已补齐（TestMenuTriggerSubMenuClick、TestMenuCollapsedClickTooltip）：`TriggerSubMenuAction(TriggerClick)`、Defaults 键 `triggerSubMenuAction`；外部按下关闭；收起时叶子提示不受影响（antd Tooltip 与触发方式无关） |
| subMenuOpenDelay / subMenuCloseDelay | rc-menu 0.1 / 0.1 s（Menu.js:55-56）；EP show-timeout / hide-timeout 300 / 300（ep/es/components/menu/src/menu.mjs）；Radix NavigationMenu delayDuration 200、关闭 150 ms（radix-nav/dist/index.mjs:37,72） | 写死 0 / 300 ms | 已补齐（TestMenuSubMenuDelay、TestAlignB3IMenuTokens）：令牌 + `SubMenuOpenDelay/SubMenuCloseDelay`、Defaults 键同名；**变更**：Ant 0/300 → 100/100 ms，Element 0/300 → 300/300，shadcn 0/300 → 200/150 |
| popupOffset | EP `popper-offset` 6；shadcn NavigationMenuContent `mt-1.5` | Element 横向 0、纵向 4 | 已补齐：令牌 `PopupOffset/HorizontalPopupOffset`；**变更**：Element 弹出 0/4 → 6，shadcn 横向 4 → 6 |
| forceSubMenuRender | rc-menu | 即时模式 | 不需要 |
| overflowedIndicator / ellipsis | rc-menu `overflowedIndicator`、antd 默认 EllipsisOutlined；EP `ellipsis` true、`ellipsis-icon` More | 横向溢出折为「…」 | 已补齐（TestMenuHorizontalEllipsis）：`OverflowedIndicator(ic)`、`Ellipsis(on)`、Defaults 键 `ellipsis` |
| expandIcon | antd `expandIcon` | 固定箭头 | 已补齐（gallery）：`ExpandIcon(ic)` |
| popupRender | rc-menu `popupRender`（Menu.d.ts:74） | — | 暂不做（Menu 弹出子菜单的自定义外壳；NavigationMenu 场景由 `ItemPanel` 覆盖） |
| 令牌：itemHeight / itemMarginBlock / itemMarginInline / itemBorderRadius / iconSize / iconMarginInlineEnd | antd itemHeight controlHeightLG 40、marginXXS 4 / 4、borderRadiusLG、fontSize 14、controlHeightSM−fontSize 10（style/index.js:376-440） | 同 | 已对齐；新增令牌 `ItemMarginBlock`（Element 0、shadcn 2）替代组件内 Flavor 分支 |
| subMenuItemBg | antd colorFillAlter（style/index.js:402、theme.js:185） | 无 | 已补齐（TestAlignB3IMenuTokens）；**变更**：Ant inline 展开子菜单底色 无 → colorFillAlter |
| 分组标题 | antd padding paddingXS padding、lineHeight、colorTextDescription，暗色 darkGroupTitleColor（style/index.js:178-183,453）；EP 12px secondary；shadcn SidebarGroupLabel `h-8 px-2 text-xs font-medium text-sidebar-foreground/70`（sidebar.tsx:408） | 组件内按 Flavor 分支 | 已补齐：令牌 `GroupTitle*`、`GroupTitleNested`、`DarkGroupTitleColor`；**变更**：shadcn 分组标签 regular TextTertiary → font-medium、sidebar-foreground/70 |
| horizontalLineHeight / itemPaddingInline / activeBar | antd 46、padding 16、activeBarHeight lineWidthBold 2，下划线 insetInline itemPaddingInline（style/index.js:436-437、theme.js:154-163、horizontal.js:28） | 内边距 20，下划线内缩 20 | 已补齐（TestAlignB3IMenuTokens）：令牌 `Horizontal*`；**变更**：Ant 横向项左右内边距 20 → 16（下划线随之） |
| Element 横向 | `--el-menu-horizontal-height` 60、满宽下划线、hover 底色（theme-chalk/src/menu.scss） | 同 | 已对齐（改为令牌） |
| shadcn NavigationMenu | 触发器 `h-9 px-4 py-2 rounded-md text-sm font-medium`、`gap-1`、hover bg-accent、open bg-accent/50、ChevronDown `ml-1 size-3` open 时 rotate-180；active 链接 `bg-accent/50`（navigation-menu.tsx:61,77,131） | 46 高条内缩 6 的 pill、regular、无箭头 | 已补齐（TestAlignB3IMenuTokens、gallery MenuAlign）：令牌 `Horizontal*`（含 `HorizontalArrowGap/HorizontalArrowTurn/HorizontalBg`，Element 箭头不转由令牌给出）；**变更**：shadcn 横向菜单高 46 → 36、font-medium、项间距 4、带翻转箭头、选中项 accent → accent/50 |
| NavigationMenuContent 任意面板 | navigation-menu.tsx `NavigationMenuContent`（`p-2 pr-2.5`，viewport=false 时 `mt-1.5 rounded-md border shadow`） | — | 已补齐（TestMenuItemPanel）：`ItemPanel(key, fn)`（MenuW 整类型在稳定层，按稳定新增：链式、两个参数），悬停 / ↓ 打开，Esc、离开关闭，面板 role group、名称取项标签；pr-2.5 按 p-2 处理 |
| NavigationMenuIndicator / viewport 尺寸动画 | navigation-menu.tsx | — | 暂不做（装饰性） |
| shadcn Sidebar 项 | SidebarMenuButton `h-8 rounded-md p-2 text-sm gap-2`、active `bg-sidebar-accent font-medium`；SidebarMenu `gap-1`；SidebarGroup `p-2`（sidebar.tsx:390,459,477） | 项间 2px | 已补齐（TestAlignB3IMenuTokens）；**变更**：shadcn 侧栏项间距 2 → 4 |
| shadcn SidebarMenuSub | `mx-3.5 border-l px-2.5 py-0.5 gap-1`、SubButton `h-7 px-2`（sidebar.tsx:646,690） | 子项 32 高、导线 22、子项起点 22 | 已补齐（TestMenuShadcnSidebarMetrics、TestMenuSubItemHeightFollowsItemHeight）：令牌 `SubMenuItemHeight`（零值 = ItemHeight，shadcn 28）、`SubMenuGuideInset` 22、`SubMenuItemInset` 33、`SubMenuPaddingBlock` 2、`ItemContentInset/IconGap/ItemSelectedFontWeight`，组件里不再按 Flavor 分支；**变更**：shadcn 子项高 32 → 28、起点 22 → 33 |
| Sidebar floating / inset 变体、offcanvas、SidebarRail、Ctrl/Cmd+B | sidebar.tsx:33,157-164,287 | — | 暂不做：变体与宽度属 `Sider`（`comp_layout.go`，见共享请求）；Ctrl/Cmd+B 等价写法：应用在 `c.KeysFor` / 全局快捷键里切换 `CollapsibleSider` 的 collapsed；Rail 暂不做（需拖拽热区，代价大） |
| 键盘 | ↑↓、Home/End、←→ 进出子菜单、Esc 关弹出 | 已有（menu_keys_test.go） | 已对齐 |
| 禁用 | antd itemDisabledColor；`ConfigProvider.ComponentDisabled` | 项禁用有；ComponentDisabled 只禁点击、a11y 未标 | 已补齐（TestMenuDisabledConfig、TestMenuA11yDisabledItem）：禁用项标 a11y Disabled，ComponentDisabled 下不选中不展开 |
| 越界 | — | — | 已补齐（TestMenuOutOfRangeKeys）：未知 selected 无选中、未知 openKeys 忽略 |
| a11y | menu / menubar / menuitem，Expanded / HasPopup / Current | 已有 | 已对齐 |
| EP collapse-transition / router / background-color 等 | EP | — | 不需要（router）；颜色等价写法：`ConfigProvider.Components` 覆盖 `MenuToken` |

## Dropdown（含 shadcn DropdownMenu）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| menu / trigger click\|hover\|contextMenu | antd/dropdown/dropdown.d.ts | `Trigger` | 已对齐 |
| placement | antd v6 14 种（含 left/right 系列；`*Center` 弃用），默认 bottomLeft，RTL bottomRight（dropdown.js:106-114） | 6 种可用，箭头只画上下 | 已补齐（TestDropdownArrowSides）：`Placement` 接受 Left/Right 系列，箭头画在对应侧；Defaults 字符串解析在共享文件，见请求 |
| arrow / pointAtCenter | antd `arrow: boolean \| {pointAtCenter}` | `Arrow()` | 已补齐（TestDropdownArrowSides）：`ArrowPointAtCenter(on)` |
| 箭头复用 drawPopupArrow | 已定事项 5 | `drawArrow` 半宽 6、高 6，靠对齐边 24px 内 | 保留自有实现：组 K 的 `drawPopupArrow` 半宽 / 高 7、总指向锚点中心，像素不同（antd 非 center 放置时箭头靠近对齐边） |
| autoAdjustOverflow | antd 默认 true | Overlay 总是翻转 | 已对齐（不提供关闭：OverlayOptions.Flip 目前总开） |
| open / onOpenChange {source} | antd `onOpenChange(open, {source: 'trigger'\|'menu'})` | `Open`、`OpenChanged` | 已补齐（TestDropdownAutoFocusAndSource）：`OpenSource()` |
| disabled | antd | `Disabled`、ComponentDisabled | 已对齐（TestDropdownDisabledConfig） |
| popupRender（dropdownRender 弃用） | antd dropdown.d.ts | — | 已补齐（TestDropdownPopupRender）：`PopupRender(func(menu func()))` |
| destroyOnHidden / forceRender | antd | 即时模式 | 不需要 |
| mouseEnterDelay 0.15 / mouseLeaveDelay 0.1 | antd dropdown.js:40-41；EP show-timeout / hide-timeout 150 / 150（ep/es/components/dropdown/src/dropdown.mjs） | 写死 150 / 100 | 已补齐（TestDropdownHoverDelays、TestDropdownElementLeaveDelay）：令牌 + `MouseEnterDelay/MouseLeaveDelay`、Defaults 键同名；**变更**：Element 离开延迟 100 → 150 ms |
| autoFocus | antd | 只有键盘打开时聚焦 | 已补齐（TestDropdownAutoFocusAndSource）：`AutoFocus(on)`、Defaults 键 `autoFocus` |
| offset | antd marginXXS 4（dropdown.js:145）；shadcn sideOffset 4 | Ant 4、Element 8 | 已对齐（改为令牌 `PopupOffset`） |
| Dropdown.Button | antd 6 弃用 | `Button(label)` | 已补齐：`Button` 加 `// Deprecated: use Compact + Button + Dropdown`；等价写法见方法注释；EP split-button 同此写法 |
| 令牌：项高 / 内边距 / 面板 | antd paddingBlock (32−22)/2、controlPaddingHorizontal 12、paddingXXS 4、borderRadiusLG、boxShadowSecondary（dropdown/style/index.js:146-215,285） | 同 | 已对齐 |
| 项图标 | antd `-item-icon` fontSizeSM 12、minWidth fontSize、marginInlineEnd marginXS（style/index.js:169-172） | 14 | 已补齐（TestAlignB3IDropdownTokens）；**变更**：Ant 下拉项图标 14 → 12（占位仍 14+8） |
| item extra（快捷键列） | antd `-item-extra` paddingInlineStart padding 16、fontSizeSM、colorTextDescription（style/index.js:193-198） | 间距 24、14px | 已补齐；**变更**：Ant `MenuItem.Shortcut` 列字号 14 → 12、与标签最小间距 24 → 16（Menu 弹出子菜单保持 14，间距 24 → 16，按 antd Menu extra） |
| 子菜单箭头 | antd expand icon fontSizeIcon 12、insetInlineEnd paddingXS 8（style/index.js:236-245）；shadcn ChevronRight `ml-auto size-4`（dropdown-menu.tsx:219） | 10px、距末端 12 | 已补齐：令牌 `SubmenuArrowSize/SubmenuArrowInset`；**变更**：Ant 箭头 10 → 12、距末端 12 → 8；shadcn 10 → 16、12 → 8；Element 不变 |
| danger | antd status.js：colorError，hover colorError 底白字；shadcn destructive/10 | 同 | 已对齐 |
| EP hide-on-click | EP `hideOnClick` true | 选后总关 | 已补齐（TestDropdownHideOnClickFalse）：`HideOnClick(on)`、Defaults 键 `hideOnClick` |
| EP placement 默认 bottom | EP dropdown.mjs:49 | BottomStart | 等价写法：`Placement(ui.Bottom)` 或 `Defaults("Dropdown", Props{"placement": "bottom"})`；行为以 antd 为准 |
| EP type / size / split-button / max-height / tabindex / popper-options / teleported | EP | `Primary/Danger/Size`、`MaxHeight` | 已对齐 / 不需要（tabindex、popper-options、teleported） |
| shadcn CheckboxItem / RadioGroup | dropdown-menu.tsx:92-137（pl-8，指示器 left-2 size-3.5，CheckIcon size-4、CircleIcon size-2） | 只有 Menubar 有 | 已补齐（TestDropdownCheckRadio）：`Check(key, &on)`、`Radio(&value, keys...)`，令牌 `Indicator*` |
| shadcn inset / Label / Separator / Shortcut | dropdown-menu.tsx | Group 标题、Divider、Shortcut | 已对齐；inset 等价写法：有勾选项时整面板自动留指示器列 |
| shadcn 禁用 opacity-50、SubContent shadow-lg | dropdown-menu.tsx:76,232 | 禁用用 TextDisabled，子面板同主面板阴影 | 暂不做（视觉差异小） |
| Esc 叠层 | Review Focus 2 | — | 已补齐（TestDropdownEscTopmostInModal）：Modal 内下拉，Esc 只关下拉、焦点回触发器，再按关 Modal |
| 越界 | — | — | 已补齐（TestDropdownOutOfRangeSelected）：未知 Selected 无选中 |

## Menubar（shadcn Menubar）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 条 | `h-9 rounded-md border p-1 shadow-xs gap-1`（menubar.tsx:16） | 令牌 | 已对齐 |
| 触发器 | `px-2 py-1 text-sm font-medium rounded-sm`，focus / open bg-accent，无 hover 样式（menubar.tsx:58） | hover 也填 accent | 已补齐（TestAlignB3IMenubarTokens）：令牌 `TriggerFocusBg/TriggerFocusColor`；**变更**：shadcn 触发器悬停不再填 accent（聚焦、打开仍填），组件内 Flavor 分支去掉 |
| 内容面板 | `min-w-[12rem] rounded-md border p-1 shadow-md`、sideOffset 8、alignOffset −4（menubar.tsx:68-81） | 最小宽 180、距离 4、不偏移 | 已补齐（TestMenubarShadcnPopupOffset）：令牌 `PopupMinWidth/PopupOffset/PopupAlignOffset`；**变更**：shadcn 面板最小宽 180 → 192、距离 4 → 8、向起始侧偏 4 |
| 项 / variant destructive / inset / Checkbox / Radio / Label / Separator / Shortcut / Sub | menubar.tsx:101-249 | `MenubarItem` Kind、Danger、Group、Divider、Shortcut、Children | 已对齐（inset 等价写法同 Dropdown） |
| 禁用与键盘 | Radix Menubar | 跳过禁用项 | 已对齐（TestMenubarDisabledItemKeyboard） |
| TitleBar 外观 | 计划约束 | 使用 Menubar | 已核对 TitleBar-shadcn 出图未坏 |

## ContextMenu（`ContextMenuAt*`、Dropdown TriggerContextMenu）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| antd 右键 | `Dropdown trigger=["contextMenu"]`，alignPoint（dropdown.js:129） | `Trigger(TriggerContextMenu)`，按指针定位 | 已对齐 |
| shadcn context-menu 外观 | 同 dropdown-menu，无 sideOffset，Label `text-foreground`（context-menu.tsx） | ContextMenuAt 用 Dropdown 令牌 | 已对齐（随 Dropdown 令牌变更） |
| CheckboxItem / RadioItem | context-menu.tsx:144-178 | `ContextMenuAt` 无勾选态 | 等价写法：`Dropdown(...).Trigger(ui.TriggerContextMenu).Check(...).Radio(...)` |
| 键盘 / a11y / 无预选 | — | `ContextMenuAtWith{NoAutoSelect}`、Menu 键 / Shift+F10 | 已对齐（nav_contextmenu_test.go） |

## Command（shadcn command.tsx + cmdk）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 外观令牌 | Command `rounded-md bg-popover`；Input 行 `h-9 gap-2 border-b px-3`、SearchIcon `size-4 opacity-50`；List `max-h-[300px]`；Group `p-1`，标题 `px-2 py-1.5 text-xs font-medium text-muted-foreground`；Item `gap-2 rounded-sm px-2 py-1.5 text-sm`、selected bg-accent、disabled opacity-50；Shortcut `text-xs tracking-widest`；Empty `py-6`（command.tsx） | 全部写死、用全局主题；shadcn 行高 44、圆角 RadiusLG、无字距 | 已补齐（TestAlignB3ICommandTokens）：新令牌组 `CommandToken`；**变更**：shadcn 搜索行 44 → 36、圆角 RadiusLG → rounded-md、边框 / 分隔线改 border 色、快捷键加 tracking-widest；Ant / Element 外观不变 |
| 根边框 | command.tsx 无边框（演示里加 `border shadow-md`） | 有 1px 边框 | 等价写法：保留边框（令牌 `BorderColor`，置零即无） |
| 搜索图标与文字间距 | gap-2（8） | 输入框自身左内边距 | 暂不做（差约 3px，需输入框无内边距变体） |
| 过滤与排序 | cmdk 默认 command-score 模糊匹配，按分数排序，分组按最高分排序 | 子串匹配，不排序 | 已补齐（TestCommandFuzzyRank）：自研评分（整词 > 前缀 > 词首 > 子串 > 顺序字母，未照搬 command-score）；**变更**：搜索时可匹配不连续字母并按分数排序 |
| shouldFilter / filter | cmdk `shouldFilter`、`filter(value, search, keywords)`（cmdk/dist/index.d.ts:32,38） | — | 已补齐（TestCommandShouldFilterAndFilter）：`ShouldFilter(on)`、`Filter(fn)`、Defaults 键 `shouldFilter` |
| loop | cmdk `loop` 默认 false（index.d.ts:54） | 总是循环 | 已补齐（TestCommandLoopHomeEnd）：`Loop(on)`、Defaults 键 `loop`；**变更**：默认不再首尾循环 |
| Home / End、vimBindings | cmdk Home/End、Ctrl+N/J/P/K（vimBindings 默认 true） | 只有 ↑↓ | 已补齐（TestCommandLoopHomeEnd、TestCommandVimBindings）：`VimBindings(on)`、Defaults 键 `vimBindings` |
| value / onValueChange | cmdk `value`、`onValueChange` | — | 已补齐（TestCommandValueControlled、TestCommandValueOutOfRange）：`Value(*string)`、`ValueChanged()`；未知值落在首项 |
| 搜索受控 | CommandInput `value` / `onValueChange` | `Query()` 只读 | 已补齐（TestCommandSearchBinding）：`Search(*string)` |
| disablePointerSelection | cmdk | 悬停即移动 | 已补齐（TestCommandDisablePointerSelection） |
| 禁用项 | cmdk 跳过 `aria-disabled` | 跳过、半透明 | 已补齐（TestCommandA11yDisabled）：禁用项标 a11y Disabled |
| CommandDialog | command.tsx:33-55（title / description sr-only、showCloseButton） | — | 等价写法：`Modal` 内放 `Command(...).Shadow()`；不新增同义入口 |
| CommandSeparator | command.tsx:135；cmdk Separator 默认只在无搜索时渲染（alwaysRender false） | 分组间自动分隔，搜索时也画 | 等价写法：按 `Group` 自动分隔；搜索时也保留分隔线（分组按分数重排后仍便于区分），差异记录在此 |
| Meta+↑/↓、Alt+↑/↓ | cmdk：Meta 跳首 / 末项，Alt 跳上 / 下一组（cmdk/dist/index.mjs） | — | 暂不做（等价写法：Home / End 跳首末；跳组少用） |
| ValueChanged 空闲帧 | — | — | 已补齐（TestCommandValueControlled）：只在当前项变化时报告 |
| a11y | combobox + listbox + option | 已有 | 已对齐 |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 浮层沿锚边偏移 | Menubar alignOffset | OverlayOptions 的交叉轴偏移（现借用 ShiftX，物理方向，组件内按 RTL 取反） | 已绕过，建议主线程评估是否加 `AlignOffset` |
| 关闭自动翻转 | Dropdown autoAdjustOverflow=false | OverlayOptions.Flip 目前总开 | 暂不做 |
| 无内边距输入框 | Command 搜索行 gap-2 | Input 无内边距变体 | 暂不做 |

## CHANGELOG 草稿

### 新增

- `ui`：Menu `OpenKeys(*[]string)`、`OpenChanged()`、`Selectable(on)`、`Tooltip(on)`、`InlineIndent(px)`、`SubMenuOpenDelay/SubMenuCloseDelay(d)`、`TriggerSubMenuAction(t)`、`ExpandIcon(ic)`、`OverflowedIndicator(ic)`、`Ellipsis(on)`、`ItemPanel(key, fn)`（shadcn NavigationMenuContent；MenuW 整类型稳定，按稳定方法新增）；Defaults 键 `selectable`、`tooltip`、`ellipsis`、`inlineIndent`、`subMenuOpenDelay`、`subMenuCloseDelay`、`triggerSubMenuAction`；`MenuItem.Danger` 在 inline / vertical / horizontal 生效，`MenuItem.Shortcut` 在 inline 项显示为 extra。
- `ui`：Dropdown `MouseEnterDelay/MouseLeaveDelay(d)`、`ArrowPointAtCenter(on)`、`AutoFocus(on)`、`HideOnClick(on)`、`PopupRender(fn)`、`Check(key, &on)`、`Radio(&value, keys...)`、`OpenSource()`；Left/Right 系列 placement 画侧边箭头；Defaults 键 `mouseEnterDelay`、`mouseLeaveDelay`、`autoFocus`、`hideOnClick`。`DropdownW.Button` 弃用（用 Compact + Button + Dropdown）。
- `ui/theme`：`MenuToken.DarkTheme`（Dark() 是否切换暗色侧栏；shadcn false，替代组件内的 Flavor 判断）。
- `ui`：Command `Loop`、`VimBindings`、`ShouldFilter`、`Filter`、`Value`、`ValueChanged`、`Search`、`DisablePointerSelection`，Home/End；Defaults 键 `loop`、`shouldFilter`、`vimBindings`。
- `ui/theme`：`CommandToken` 全部字段；`MenuToken.CollapsedWidth/CollapsedIconSize/ItemMarginBlock/SubMenuItemHeight/InlineIndent/GroupTitle*/DangerItemColor/DangerItemSelectedBg/ExtraColor/ExtraFontSize/SubMenuOpenDelay/SubMenuCloseDelay/PopupOffset/HorizontalPopupOffset/Horizontal*/PopupArrowSize/PopupArrowInset`；`DropdownToken.SubmenuArrowSize/SubmenuArrowInset/Indicator*/MouseEnterDelay/MouseLeaveDelay/PopupOffset`；`MenubarToken.TriggerFocusBg/TriggerFocusColor/PopupMinWidth/PopupOffset/PopupAlignOffset`。

### 变更

- Menu：悬停弹出子菜单延迟 Ant 0/300 ms → 100/100 ms（rc-menu subMenuOpenDelay / subMenuCloseDelay 0.1），Element → 300/300（show-timeout / hide-timeout），shadcn → 200/150（Radix NavigationMenu）。
- Menu：inline 项设了 `MenuItem.Shortcut` 时，末端显示为 antd item extra（以前 inline 项不显示）；暗色菜单危险项改用 darkDangerItemColor / darkDangerItemSelectedBg（以前暗色下无危险样式）。
- Menu（Ant）：inline 展开的子菜单底色 无 → colorFillAlter；横向项左右内边距与下划线内缩 20 → 16（itemPaddingInline）；弹出子菜单快捷键列间距 24 → 16。
- Menu（Element）：弹出子菜单距离（横向 0、纵向 4）→ 6（popper-offset）。
- Menu（shadcn）：收起宽 80 → 48；侧栏项间距 2 → 4；子项高 32 → 28、起点 22 → 33；横向选中项 accent → accent/50、箭头间距 4；分组标签改 font-medium、sidebar-foreground/70；横向（NavigationMenu）高 46 → 36、font-medium、项间距 4、hover 整项 accent、打开 accent/50、带翻转箭头、面板距离 4 → 6。
- Dropdown（Ant）：项图标 14 → 12；快捷键列字号 14 → 12、间距 24 → 16；子菜单箭头 10 → 12、距末端 12 → 8（antd dropdown style）。Element 离开延迟 100 → 150 ms。shadcn 子菜单箭头 10 → 16、距末端 12 → 8；开启 `Arrow()` 时箭头带面板描边。
- Menubar（shadcn）：触发器悬停不再填 accent（Radix 只有 focus / open）；面板最小宽 180 → 192、距离 4 → 8、向起始侧偏 4。
- Command：默认不再首尾循环（cmdk loop=false）；搜索改为顺序字母模糊匹配并按分数排序；shadcn 搜索行 44 → 36、圆角 RadiusLG → rounded-md、边框与分隔线用 border 色、快捷键加 tracking-widest。
- 测试期望：`ui/r2_tokens_test.go` `TestR2MenuPopupDefaults` 对 Ant 不再要求 Menu 弹出图标 / 快捷键与 Dropdown 相同（改为断言 14 / 14 / 16）。

## 共享文件改动请求

1. `ui/shad2c_defaults.go` `s2cPlacement`：加 antd v6 的 `left`、`leftTop`、`leftBottom`、`right`、`rightTop`、`rightBottom`（→ `Left/LeftStart/LeftEnd/Right/RightStart/RightEnd`），使 `Defaults("Dropdown", Props{"placement": "right"})` 可用。文件头的 Defaults 键清单补 Menu / Dropdown 新键。
2. `ui/nav.go` `MenuItem`（组 J 文件，Breadcrumb/Pagination 之外）：可选加 `Title string`（antd 收起时提示文字），本组未改。
3. `ui/theme/comp_layout.go`：shadcn Sider 宽 256 / 收起 48 已由 `SiderWidth/SiderCollapsed` 给出；floating / inset 变体（`p-2`、`rounded-lg border shadow-sm`）若要做需在 `LayoutToken` 加字段，暂不做，等用户决定。
4. `ui/AUTHORING.md` §13/§14：Menu 一行注明 shadcn NavigationMenu = `Menu(MenuHorizontal)` + `ItemPanel`、Sidebar = `Sider/CollapsibleSider` + `Menu`；Dropdown 一行补 `Check/Radio/PopupRender`、`Button` 弃用；Command 一行补 cmdk 行为方法；ContextMenu 勾选项写等价写法（Dropdown TriggerContextMenu）。
5. `ui/r2_tokens_test.go`（归属外）：已小改 `TestR2MenuPopupDefaults`（见 CHANGELOG 变更末条），请主线程确认。
6. `CompCommand` 新令牌如需进 `highcontrast.go`，请主线程复核（`go test ./ui/theme -run Contrast` 已过）。
7. Locale：本组未改。
8. `ui/titlebar.go`（只读）：shadcn Menubar 触发器按 Radix 去掉了悬停底色，TitleBar 里的菜单悬停因此没有底色反馈。若主线程认为窗口标题栏需要保留，可在 TitleBar 的 Components 覆盖里设 `MenubarToken.TriggerHoverBg`。
9. 已知别组问题（非本组）：`TestComponentCSSVars` 因 Drawer `ClosablePlacement`、Pagination `PageSizeOptions`、Spin `Indicator` 字段数不符；`TestGolden/basic_dark` 差异来自 Alert（golden 页无菜单类组件）。
