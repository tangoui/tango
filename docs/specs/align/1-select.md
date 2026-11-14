# 第 1 批 · 组 C 选择器：对照记录

核对日期：2026-11-14。参考源码：`/tmp/tango-ref`（antd@6.7.0、element-plus@2.14.8、shadcn-ui@e62c9435、tailwindcss@4.3.5），另按 antd 6.7.0 的依赖版本解包 `@rc-component/select@1.12.0`、`@rc-component/cascader@1.26.0`、`@rc-component/mentions@1.14.1`、`@rc-component/tree-select@1.19.0` 到 `/tmp/tango-ref/rc-c/`。下文路径缩写：`antd/` = `antd/package/es/`，`ep/` = `ep/package/es/components/`，`chalk/` = `ep/package/theme-chalk/src/`，`rc-select/` = `rc-c/select/package/es/`，`shad/` = `shadcn-ui/apps/v4/registry/new-york-v4/ui/`。

本组共用的新能力放在 `ui/selector.go`（选择器外壳）：`selPlacement`/`selDefaults`（Defaults 键 `variant`、`placement`）、`selPanel`（按 placement 打开下拉）、`selEmptyText`、`selSpinner`（loading 后缀）、`selTagOpts`/`selTagText`（maxTagPlaceholder、maxTagTextLength）。这些改动只在组件显式设置时生效，别组用到外壳的 `MultiDatePicker`（date_multi.go）外观不变。

### Select

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 单选、受控值 | antd `value/onChange`（antd/select/index.d.ts，2026-11-14）；EP `model-value` | select.go:`Select(c, &val, opts)`、`Changed` | 已对齐 |
| 值不在选项里 | rc-select/Select.js:215 `(labelRender ? labelRender(item) : item.label) ?? item.value`（显示原值） | 旧：显示空白 → 现：显示原值 | 已补齐（TestSelectUnknownValue、TestSelectA11yUnknownValue） |
| 值为 `""` 的选项 | antd 只有无值（null/undefined）显示占位符 | 显示该选项标签；无值且无 `""` 选项时显示占位符 | 已对齐（TestSelectEmptyValueOption） |
| showSearch / EP filterable | antd `showSearch`（index.d.ts）；EP `filterable`（ep/select/src/select.d.ts） | `Searchable`、Defaults `showSearch` | 已对齐 |
| filterOption / filter-method | rc-select/Select.d.ts:74；EP `filter-method` | p2b_select.go:`FilterMethod` | 已对齐 |
| remote / remote-method / debounce | EP `remote`、`remote-method`、`debounce` | `Remote`、`Debounce` | 已对齐 |
| allow-create（tags 单值） | EP `allow-create` | `AllowCreate`、`Created` | 已对齐 |
| mode multiple / tags | antd `mode?: 'multiple' \| 'tags'` | 拆成 `MultiSelect`（tags = `MultiSelect.AllowCreate`）、`InputTag` | 等价写法（Go 绑定类型不同：单值 `*string`，多值 `*[]string`） |
| allowClear / clearable | antd `allowClear`；EP `clearable` | `AllowClear`、Defaults `allowClear` | 已对齐 |
| variant（含 underlined） | antd `variant`（5.13 起，`outlined` 默认，config-provider Variant 含 `underlined`） | `Variant(FieldVariant)` 四值；旧：无 Defaults | 已补齐 Defaults `"variant"`（TestSelectDefaultsVariant） |
| placement | antd `placement: SelectCommonPlacement`（bottomLeft/bottomRight/topLeft/topRight）；EP `placement`（bottom-start 等） | 旧：固定 BottomStart → `Placement(ui.Placement)` + Defaults `"placement"`（Ant 名、EP 名或 Placement 值） | 已补齐（TestSelectPlacement、TestSelectPlacementThemes、TestSelectPlacementElementNames） |
| 引擎路径（Virtual/Groups/Remote/Loading/长列表）下的 placement、空文案、OpenChanged | 同上 | `fillVSState.host` 传入引擎 | 已补齐（TestSelectEnginePathHost） |
| open / onOpenChange | antd `open`、`onOpenChange`（index.d.ts）；EP `visible-change` | `Open()` 强制打开、`IsOpen`；新 `OpenChanged()` | 已补齐（TestSelectOpenChanged、TestSelectDisabledOpenChanged、TestSelectDisabledWhileOpen）；受控关闭用 `Open()` 不调用即可，等价写法 |
| labelRender | rc-select/Select.d.ts:117；Select.js:215 也作用于未知值 | 新 `LabelRender(func(Option) string)` | 已补齐（TestSelectLabelRender）；返回字符串而非任意节点，等价写法；未知值时 rc-select 传入的 `item.label` 为 undefined，Tango 传 `Option{Label: value, Value: value}`，等价写法 |
| optionRender | rc-select/Select.d.ts:109 | `OptionRender` | 已对齐 |
| loading | antd/select/useIcons.js:53 后缀换成 LoadingOutlined；EP `loading` + `loading-text` | 旧：只在下拉里显示加载行 → 后缀同时显示转圈；设了 `SuffixIcon` 时保留自定义后缀（useIcons.js:51 suffix 优先） | 已补齐（TestSelectLoadingSuffix、TestSelectLoadingKeepsSuffixIcon） |
| notFoundContent / no-data-text / no-match-text | rc-select BaseSelect `notFoundContent`；EP `no-data-text`、`no-match-text` | 新 `NotFoundContent(s)`；默认 Locale `NoData`/`NotFound` | 已补齐（TestSelectNotFoundContent） |
| 选项分组 | antd OptGroup；EP el-option-group | `Groups`、`Option.Group` | 已对齐 |
| virtual / listHeight / listItemHeight | rc-select `virtual`、`listHeight`(256)、`listItemHeight` | `Virtual`、`ListHeight`；行高来自令牌 | 已对齐 |
| prefix / suffix（6.7 起 suffix，suffixIcon 弃用） | antd/select/index.d.ts `suffix` @since 6.7.0；EP `suffix-icon` | `Prefix`、`PrefixText`、`SuffixIcon` | 等价写法（`SuffixIcon` 同时对应 EP `suffix-icon` 和 antd `suffix`，不另加同义方法） |
| 键盘：上下跳过禁用项 | rc-select/OptionList.js:173-195 `getEnabledActiveIndex` | 旧：简单下拉上下键会停在禁用项 | 已补齐（TestSelectKeyboardSkipsDisabled） |
| 键盘：Tab 选中高亮项 | rc-select/OptionList.js:199 `TAB` 与 Enter 相同 | Tab 只移焦点 | 暂不做（Tab 不在 `c.keys` 中，缺 Runner 对 Tab 的组件级拦截） |
| 键盘：Ctrl+N/P（mac） | 同上:173 | — | 暂不做（同上，低优先） |
| defaultActiveFirstOption | rc-select 默认 true | 打开时高亮当前值或第一项 | 已对齐 |
| popupMatchSelectWidth / fit-input-width | antd 默认 true；EP 下拉最小宽度 = 输入框 | Overlay `MatchWidth`（最小宽度） | 已对齐 |
| popupRender / dropdownRender | antd `popupRender` | — | 暂不做（需要在下拉列表外包一层自定义节点，可用 `Groups`/`OptionRender` 覆盖大部分用法；完整等价需新 API，留下一批） |
| labelInValue | antd `labelInValue` | — | 不需要（Go 里值与 Option 已分开，查 `opts` 即得 label） |
| fieldNames / EP props / value-key | rc-select `fieldNames`；EP `props`、`value-key` | — | 不需要（Go 的 Option 有类型，§7） |
| 禁用 | antd `disabled`；EP `disabled` | `Disabled(true)`、ComponentDisabled | 已对齐（TestSelectDisabledOpenChanged） |
| 状态 | antd `status`；EP 表单校验 | `Status`、`c.itemStatus` | 已对齐 |
| 尺寸 | antd `size`；EP `size` | `Size`、Defaults `size` | 已对齐 |
| 令牌（Ant） | antd/select/style/token.js：optionHeight=controlHeight(32)、optionPadding 水平 12、optionSelectedFontWeight=600、multipleItemHeight min(32-8, 32-2)=24 / SM 16 / LG 32 | theme/comp_select.go | 已对齐（TestAlignB1CSelectTokens） |
| 令牌（Element） | chalk/common/var.scss `$select-option` height 34px、option.scss padding 0 32px 0 20px、`$select-dropdown` max-height 274px | comp_select.go Element 分支；VirtualSelect 列表 274 | 已对齐 |
| shadcn 外观 | shad/select.tsx：触发器 h-9（sm h-8）、rounded-md、shadow-xs、focus ring 3px ring/50、aria-invalid destructive；SelectItem py-1.5 pl-2 text-sm | theme.Shadcn ControlHeight 36/32、OptionHeight 32、OptionPadding 8 | 已对齐（TestAlignB1CSelectTokens） |
| 明暗主题 | — | 全部取令牌，无 `th.Dark` 分支 | 已对齐（gallery SelectAlign 三套 × 明暗） |
| 语义化 classNames/styles | antd `classNames/styles` | — | 不需要（JS 样式钩子；Go 用组件令牌 `ConfigProvider.Components`） |

### VirtualSelect（EP el-select-v2）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| options / multiple / filterable / allow-create / remote / loading / multiple-limit / item-height / height | ep/select-v2/src/defaults.d.ts（2026-11-14） | fill_virtual_select.go：`VirtualSelect`、`Multiple`、`Filterable`、`AllowCreate`、`Remote`、`Loading`、`MaxCount`、`ItemHeight`、`ListHeight`（Element 274） | 已对齐 |
| collapse-tags / max-collapse-tags | 同上 | `MaxTagCount` | 等价写法 |
| collapse-tags-tooltip / tag-tooltip | 同上 | — | 暂不做（+N 标签悬停浮层：需要在外壳的折叠标签上挂 Tooltip，外壳目前无此节点挂点，留下一批） |
| placement | 同上 `placement` | 新 `Placement`，Defaults `Select.placement` | 已补齐（TestVirtualSelectPlacementEmptyLoading） |
| no-data-text / no-match-text | 同上 | 新 `NotFoundContent` | 已补齐（TestVirtualSelectPlacementEmptyLoading） |
| loading 后缀 | antd 同 Select | `Loading` 同时显示转圈 | 已补齐（TestVirtualSelectPlacementEmptyLoading） |
| visible-change | EP `visible-change` | `IsOpen`、`Open` | 等价写法（读 `IsOpen` 前后比较）；`OpenChanged` 只在 Select/MultiSelect 上，其余暂不做 |
| 未知值 | 同 Select | labelOf 回退原值 | 已对齐（TestSelectUnknownValue） |
| reserve-keyword | EP 默认 true（选中后保留搜索词）；antd `autoClearSearchValue` 默认 true（清空） | 清空 | 已对齐（行为以 antd 为准）；保留关键词的开关暂不做 |
| value-key / props | — | — | 不需要（§7） |

### MultiSelect

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 多选、受控 | antd `mode="multiple"`；EP `multiple` | entry_multiselect.go:`MultiSelect(c, &vals, opts)` | 已对齐 |
| 值不在选项里 | rc-select 同上 | 标签显示原值 | 已对齐（TestSelectUnknownValue） |
| maxCount / multiple-limit | rc-select `maxCount`；EP `multiple-limit` | p2b_select.go:`MaxCount` | 已对齐 |
| maxTagCount | rc-select BaseSelect `maxTagCount: number \| 'responsive'` | `MaxTagCount(n)` | 数字已对齐；`'responsive'` 暂不做（缺：在构建期按可用宽度折叠，需要布局层提供"本帧可放下几个"的测量回调） |
| maxTagPlaceholder | rc-select/SelectInput/Content/MultipleContent.js:57 默认 `+ ${n} ...` | 旧：固定 → 新 `MaxTagPlaceholder(func(omitted []string) string)` | 已补齐（TestMultiSelectMaxTagPlaceholder） |
| maxTagTextLength | MultipleContent.js:111-115 截断为 n 个字符 + `...` | 新 `MaxTagTextLength(n)`，Defaults `Select.maxTagTextLength` | 已补齐（TestMultiSelectMaxTagPlaceholder；a11y 名仍是全文） |
| tagRender | rc-select `tagRender` | — | 暂不做（外壳标签是固定绘制，自定义标签内容需要外壳支持子树，留下一批） |
| tokenSeparators | rc-select `tokenSeparators` | `InputTag.Separators` 有；MultiSelect tags 模式没有 | 暂不做（MultiSelect.AllowCreate 的粘贴拆分，下批与 tagRender 一起做） |
| placement / notFoundContent / onOpenChange / variant 默认 | 同 Select | 新 `Placement`、`NotFoundContent`、`OpenChanged`；Defaults `variant`、`placement` | 已补齐（TestSelectPlacement、TestSelectNotFoundContent、TestSelectOpenChanged、TestSelectDefaultsVariant） |
| loading 后缀 | antd useIcons.js | 新：后缀转圈 | 已补齐（TestSelectLoadingSuffix） |
| 键盘：Backspace 删最后一个、上下跳过禁用 | rc-select | `msStep`、Backspace | 已对齐 |
| 令牌 | antd multipleItemBg=colorFillSecondary、multipleItemHeight 24/16/32 | comp_select.go | 已对齐 |
| collapse-tags-tooltip | EP | — | 暂不做（同 VirtualSelect） |

### AutoComplete

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| options + 过滤 | antd AutoComplete（基于 rc-select combobox）；EP `fetch-suggestions` | entry_autocomplete.go:`AutoComplete`、`Source` | 已对齐 |
| 远程 / debounce / loading | EP `fetch-suggestions`、`debounce`(300)、`hide-loading` | `Remote`、`Debounce`、`Loading` | 已对齐（hide-loading = 不调用 Loading，等价写法） |
| trigger-on-focus | EP 默认 true | `TriggerOnFocus()`（默认关） | 已对齐（行为以 antd 为准：antd 聚焦不自动展开） |
| highlight-first-item | EP 默认 false；antd defaultActiveFirstOption true | 打开时高亮第一项 | 已对齐（以 antd 为准） |
| placement | antd placement；ep/autocomplete `placement`（bottom-start） | 新 `Placement`，Defaults `AutoComplete.placement` | 已补齐（TestAutoCompleteNotFoundPlacement） |
| open / onOpenChange / EP 无 | antd `open`、`onOpenChange` | `Open`、`IsOpen` | 等价写法（OpenChanged 暂不做） |
| notFoundContent | antd：设了才在无结果时显示，未设则关闭 | 新 `NotFoundContent` | 已补齐（TestAutoCompleteNotFoundPlacement） |
| variant 默认 | antd `variant` | Defaults `AutoComplete.variant` | 已补齐（TestSelectDefaultsVariant） |
| backfill | antd `backfill`（键盘移动时回填输入框） | — | 暂不做（需要把"输入值"与"搜索词"分开存，否则回填会改变过滤结果；下批随 Select combobox 一起做） |
| select-when-unmatched / loop-navigation | EP | Enter 无匹配时不选；上下循环 | 已对齐（loop 已是循环；select-when-unmatched 由 `Changed` 读取输入值即可，等价写法） |
| value-key / fit-input-width | EP | — | 不需要（§7 / MatchWidth 已是最小宽度） |
| 禁用 / 状态 / 尺寸 / allowClear | antd、EP | `Disabled`、`Status`、`Size`、`AllowClear` | 已对齐 |

### Mentions

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| prefix（多个） | antd `prefix: string \| string[]`；EP `prefix` | entry_mentions.go:`Prefix(p ...string)` | 已对齐 |
| split | rc-c/mentions/package/es/Mentions.js:22 默认 `' '`；EP `split` | 旧：固定空格 → 新 `Split(s)` | 已补齐（TestMentionsSplitPlacement） |
| placement 'top' \| 'bottom' | rc mentions Mentions.d.ts:7；EP `placement` | 旧：固定下方 → 新 `Placement(TopStart/BottomStart)`、Defaults `"placement"`（"top"/"bottom"、EP 名或 Placement 值） | 已补齐（TestMentionsSplitPlacement） |
| notFoundContent | Mentions.js:23 默认 'Not Found'；antd 用 renderEmpty | 无匹配时关闭 | 暂不做（无匹配时 antd 仍显示空态；Tango 关闭，改动牵涉 Whole/remote 的开合逻辑，下批处理） |
| filterOption / validateSearch | rc mentions | 固定 label/value 包含匹配；前缀前须为空白 | 暂不做（自定义过滤：可用 `Remote` 自行过滤，等价写法；validateSearch 暂不做） |
| whole / check-is-whole | EP | `Whole`、`WholeCheck` | 已对齐 |
| loading / remote | antd `loading`；EP `loading` | `Loading`、`Remote` | 已对齐 |
| autoSize / rows | antd 继承 TextArea | `Rows` | 等价写法（autoSize 暂不做） |
| variant / status / allowClear | antd | `Variant`、`Status`、`AllowClear`；Defaults `Mentions.variant` 新增 | 已补齐（TestSelectDefaultsVariant） |

### InputTag（EP el-input-tag；antd Select mode=tags）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| max / delimiter / draggable / save-on-blur / clearable | ep/input-tag/src/input-tag.d.ts | entry_inputtag.go:`MaxTags`、`Separators`、`Draggable`、`SaveOnBlur`、`AllowClear` | 已对齐 |
| trigger 'Enter' \| 'Space' | input-tag.d.ts:34；composables/use-input-tag.mjs:78-104 | 新 `Trigger(key)`、Defaults `InputTag.trigger` | 已补齐（TestInputTagTriggerSpace、TestInputTagTriggerDisabled） |
| collapse-tags / max-collapse-tags | EP | `MaxTagCount` | 等价写法 |
| collapse-tags-tooltip | EP | — | 暂不做（同 VirtualSelect） |
| tag-type / tag-effect | EP | 标签取 Select 令牌 | 暂不做（需要外壳标签支持 Tag 的状态色，下批随 tagRender） |
| readonly | EP `readonly` | — | 暂不做（外壳没有只读态：可聚焦但不可编辑、不可删除，需在 selector 加状态） |
| 禁用 | EP `disabled` | `Disabled` | 已对齐（TestInputTagTriggerDisabled） |

### Cascader

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| options / value / changeOnSelect / expandTrigger | antd Cascader；rc-cascader 1.26 | entry_cascader.go:`ChangeOnSelect`、`ExpandOnHover` | 已对齐 |
| multiple / showCheckedStrategy | antd `multiple`、`showCheckedStrategy` | `Multiple`、`ShowCheckedStrategy`、`StrategyValue` | 已对齐 |
| EP props.checkStrictly | ep/cascader-panel `props.checkStrictly` | 单选 `ChangeOnSelect`；多选父子联动 | 等价写法（单选 = ChangeOnSelect；多选不联动暂不做） |
| loadData / props.lazy | antd `loadData`；EP `lazy/lazyLoad` | `LoadData` | 已对齐 |
| showSearch / filter-method | antd `showSearch.filter`；EP `filterable`、`filter-method` | `Searchable`、`FilterMethod` | 已对齐 |
| displayRender / show-all-levels | antd `displayRender`；EP `show-all-levels` | `DisplayRender` | 已对齐（show-all-levels=false 用 DisplayRender 取最后一项，等价写法） |
| separator | ep/cascader/src/cascader.d.ts:48 默认 `' / '` | 新 `Separator(s)` | 已补齐（TestCascaderSeparator、TestCascaderEmptySeparator；`Separator("")` 直接拼接）；p2b_cascader.go 里 ShowChild/ShowAll 的 Element 标签仍用 " / "，见共享文件请求 |
| placement | antd `placement`；EP `placement` | 新 `Placement`、Defaults `Cascader.placement` | 已补齐（TestCascaderTreeSelectPlacementTags） |
| open / onOpenChange / visible-change | antd、EP | `Open`、`IsOpen` | 等价写法（OpenChanged 暂不做） |
| notFoundContent | antd `notFoundContent` | 新 `NotFoundContent` | 已补齐（TestSelectNotFoundContent） |
| maxTagPlaceholder | antd 继承 rc-select | 新 `MaxTagPlaceholder` | 已补齐（TestCascaderTreeSelectPlacementTags） |
| variant 默认 | antd `variant` | Defaults `Cascader.variant` | 已补齐（TestSelectDefaultsVariant） |
| optionRender / expandIcon | antd | — | 暂不做（列内自定义渲染，下批） |
| 令牌 | antd/cascader/style/index.js：controlItemWidth 111、dropdownHeight 180、optionPadding paddingSM 12；chalk/cascader-panel.scss：min-width 180、height 204、node 34px、padding 0 30px 0 20px | theme/comp_cascader.go | 已对齐（TestAlignB1CCascaderTokens） |

### CascaderPanel

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 内嵌面板、单/多选、懒加载、策略 | antd `Cascader.Panel`；EP el-cascader-panel | entry_cascader_panel.go 全部方法 | 已对齐 |
| 键盘 | EP 方向键在列间移动 | 已有 | 已对齐 |
| border | EP 面板边框 | `Border(on)` | 已对齐 |

### TreeSelect

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| treeData / multiple / treeCheckable / treeCheckStrictly / showCheckedStrategy | antd TreeSelect；rc-tree-select 1.19 | entry_treeselect.go、p2b_treeselect.go | 已对齐 |
| treeDefaultExpandAll / treeExpandedKeys / loadData / filterTreeNode / virtual / listHeight | 同上 | 已有 | 已对齐 |
| placement / notFoundContent / maxTagPlaceholder / variant 默认 | antd 继承 rc-select | 新 `Placement`、`NotFoundContent`、`MaxTagPlaceholder`；Defaults `TreeSelect.variant/placement` | 已补齐（TestCascaderTreeSelectPlacementTags、TestSelectNotFoundContent、TestSelectDefaultsVariant） |
| open / onOpenChange / visible-change | antd、EP | `Open`、`IsOpen`、`ExpandChanged` | 等价写法（OpenChanged 暂不做） |
| treeLine / treeIcon / switcherIcon | antd | — | 暂不做（树线绘制在 data_tree.go，下拉里的树是另一套行渲染，需复用后再加） |
| EP tree-select | = el-select + el-tree 属性 | 同上 | 已对齐 |

### Transfer

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| dataSource / targetKeys / titles / render / oneWay / pagination / showSearch / status / disabled | antd/transfer/index.d.ts | entry_transfer.go、p2b_transfer.go | 已对齐 |
| 右侧顺序与新移入键的位置 | antd/transfer/index.js:129 `newMoveKeys.concat(targetKeys)`；hooks/useData.js 右侧按 targetKeys 排序；EP `target-order` 默认 original（use-move.mjs:24-25） | 旧：追加到末尾、右侧按数据源顺序 → 新：默认 `unshift`（按 antd），`TargetOrder("push"\|"original")` | 已补齐（TestTransferTargetOrder、TestTransferUnknownTargetKey、TestTransferDuplicateTargetKey、TestTransferDisabledTargetOrder） |
| actions（6.x，operations 弃用） | antd/transfer/index.js:80、313 | 新 `Actions`；`Operations` 标 Deprecated | 已补齐（TestTransferTargetOrder 使用 Actions） |
| showSelectAll | antd `showSelectAll` 默认 true | 新 `ShowSelectAll(bool)`、Defaults `Transfer.showSelectAll` | 已补齐（TestTransferShowSelectAll） |
| selectedKeys / onSelectChange（受控勾选） | antd | `DefaultChecked`、`Checked(side)` | 等价写法（读 Checked；完全受控暂不做） |
| filterOption / filter-method / filter-placeholder | antd `filterOption`；EP | 固定标题包含匹配；占位符来自 Locale | 暂不做（自定义过滤函数，下批） |
| selectAllLabels / format / footer / locale | antd、EP | 数量文案走 Locale | 暂不做（自定义表头文字与底部插槽，下批） |
| 令牌（Ant） | antd/transfer/style/index.js:302-303 listWidth 180、listHeight 200；headerHeight controlHeightLG、itemHeight controlHeight | 旧 200×240 → 180×200 | 已补齐（TestAlignB1CTransferTokens） |
| 令牌（Element） | chalk/common/var.scss:1433-1439 panel-width 200、header 40 + body 278、item-height 30 | 旧高 280 → 318 | 已补齐（TestAlignB1CTransferTokens） |

### shadcn NativeSelect

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 原生 `<select>` | shad/native-select.tsx：h-9（sm h-8）、rounded-md、px-3 pr-9、shadow-xs、ChevronDown size-4 opacity-50、focus-visible ring 3px、aria-invalid destructive、disabled opacity-50 | `Select`（shadcn 主题下同尺寸、同箭头、同焦点环） | 等价写法：`ui.Select`（自绘没有原生下拉；尺寸 sm/default 对应 `Size(ui.Small)`/默认，ControlHeight 32/36 已对齐） |
| optgroup | NativeSelectOptGroup | `Groups` / `Option.Group` | 等价写法 |

### shadcn Combobox

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 可输入搜索的单选 | shad/combobox.tsx（Base UI Combobox：Input + List + Clear + Empty） | `Select(...).Searchable().AllowClear().NotFoundContent(...)` | 等价写法 |
| 多选 chips | ComboboxChips / ComboboxChip / ComboboxChipsInput | `MultiSelect(...).Searchable()` | 等价写法 |
| 自由输入 + 建议 | ComboboxInput 自由文本 | `AutoComplete` | 等价写法 |
| 分组、分隔线 | ComboboxGroup / ComboboxLabel / ComboboxSeparator | `Groups`（分隔线由分组标题代替） | 等价写法 |

结论：两者都有对应物，不新建 `shad_` 文件。

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 组件拦截 Tab 键 | Select（rc-select Tab 选中高亮项） | Runner 允许聚焦组件在 Tab 移焦前消费 Tab | 暂不做，等用户决定 |
| 按可用宽度折叠标签 | MultiSelect/Cascader/TreeSelect `maxTagCount: 'responsive'` | 构建期可得本帧容器宽度或布局后回调重排 | 暂不做，等用户决定 |

## CHANGELOG 草稿

### 新增

- Select：`Placement`、`LabelRender`、`NotFoundContent`、`OpenChanged`；Defaults `Select` 读 `variant`、`placement`。
- MultiSelect：`Placement`、`NotFoundContent`、`OpenChanged`、`MaxTagPlaceholder`、`MaxTagTextLength`（Defaults `Select.maxTagTextLength`）。
- VirtualSelect：`Placement`、`NotFoundContent`。
- AutoComplete：`Placement`、`NotFoundContent`；Defaults `AutoComplete` 读 `variant`、`placement`。
- Mentions：`Split`、`Placement`；Defaults `Mentions` 读 `variant`、`placement`（"top"/"bottom"）。
- InputTag：`Trigger("Enter"|"Space")`，Defaults `InputTag.trigger`。
- Cascader：`Separator`、`Placement`、`NotFoundContent`、`MaxTagPlaceholder`；Defaults `Cascader` 读 `variant`、`placement`。
- TreeSelect：`Placement`、`NotFoundContent`、`MaxTagPlaceholder`；Defaults `TreeSelect` 读 `variant`、`placement`。
- Transfer：`Actions`（antd 6 `actions`）、`TargetOrder`、`ShowSelectAll`；Defaults `Transfer` 读 `targetOrder`、`showSelectAll`。`Operations` 标为 Deprecated。

### 变更

- Select：值不在选项里时显示值本身（旧：空白），依据 rc-select Select.js `label ?? val`。
- Select：键盘上下移动跳过禁用选项（旧：会停在禁用项），依据 rc-select OptionList.js `getEnabledActiveIndex`。
- Select / MultiSelect / VirtualSelect：`Loading(true)` 时箭头位置显示加载转圈（旧：只在下拉里显示），依据 antd select/useIcons.js。
- Transfer：移入的键默认放到目标切片最前，右侧列表按目标切片顺序显示（旧：追加到末尾、右侧按数据源顺序），依据 antd transfer/index.js `newMoveKeys.concat(targetKeys)` 与 hooks/useData.js；要旧行为用 `TargetOrder("original")`（EP 默认）。
- Transfer（Ant 主题，shadcn 主题继承）：列表默认尺寸 200×240 → 180×200，依据 antd transfer/style `listWidth: 180, listHeight: 200`。
- Transfer（Element 主题）：列表默认高度 280 → 318（表头 40 + 主体 278），依据 theme-chalk `$transfer` `panel-body-height: 278px`。

## 共享文件改动请求

1. `ui/a11y_dataentry_select_test.go:283`（TestA11yTransfer）：`target[1] != "2"` → `target[0] != "2"`。原因：Transfer 默认 targetOrder 改为 antd 的 unshift（新移入键在前）。
2. `ui/p2b_entry_test.go:265`（TestP2BTransferTreeSource）：`target[1] != "vue"` → `target[0] != "vue"`。原因同上。
3. `ui/t1a_tokens_test.go:200`（TestT1aTokenDefaults，Element 分支）：`tf.ListHeight != 280` → `tf.ListHeight != 318`。原因：Element Transfer 高度对齐 theme-chalk（40 + 278）。
4. `ui/p2b_cascader.go:242`（strategyChips 的 Element 标签）：`strings.Join(w.labelsOf(p), " / ")` → `strings.Join(w.labelsOf(p), w.separator())`，让 `Separator` 也作用于 ShowChild/ShowAll 策略下的 Element 标签。
5. `ui/AUTHORING.md` §14 无需改行（没有新组件）；§13 去重说明可补一句：shadcn NativeSelect = `Select`、Combobox = `Select.Searchable` / `MultiSelect.Searchable` / `AutoComplete`。
6. `api/**` 由主线程生成：新增方法见「CHANGELOG 草稿 · 新增」；`(*TransferW).Operations` 加了 Deprecated 注释（签名不变）。
7. Locale：本组没有新增或修改 Locale 字段。
