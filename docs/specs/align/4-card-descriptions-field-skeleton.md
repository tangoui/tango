# 第 4 批组 A：Card、Descriptions、Field、Skeleton

核对日期：2026-11-15。基线 Pro Components 3.1.14-2 / Ant Design Pro v6、antd 6.7.0、Element Plus 2.14.8、shadcn/ui v4 e62c9435。下面 `pro`、`antd`、`ep` 分别指 `/tmp/tango-ref/{pro,antd,ep}/package`；`shadcn` 指 `/tmp/tango-ref/shadcn-ui/apps/v4/registry/new-york-v4/ui`。源码只读；记录不以旧 ui-coverage 的“完整”代替逐项核对。

### ProCard

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状（文件:符号） | 结论 |
|---|---|---|---|
| title | Pro title ReactNode（pro/es/card/typing.d.ts，2026-11-15） | ui/pro2_card.go:ProCard(c,title) | 已对齐 |
| subTitle | 副标题（同上，2026-11-15） | ui/pro2_card.go:SubTitle | 已对齐 |
| tooltip | 标题提示（同上，2026-11-15） | ui/pro2_card.go:Tooltip | 已对齐 |
| extra | 标题右侧内容（同上，2026-11-15） | ui/pro2_card.go:Extra(func()) | 等价写法：用构建函数代替 ReactNode |
| actions | 底部操作区域（同上、components/Card/style.js，2026-11-15） | ui/pro2_card.go:Actions/actionBar | 已对齐（TestProCardActions） |
| variant / 默认边框 | variant 默认 outlined，border 是 lineWidth/colorBorderSecondary；ghost 和 split 子卡无边框（pro/es/card/components/Card/index.js:58,210、style.js，2026-11-15） | ui/pro2_card.go:Bordered/Borderless/Body，未显式配置时所有主题 outlined | 已补齐（TestProCardVariantAndBoxShadow、TestShad2dProCardAntUnchanged） |
| boxShadow | 默认 false；boxShadowTertiary、borderColor transparent（同上 index.js:39、style.js:28，2026-11-15） | ui/pro2_card.go:BoxShadow(bool)，Defaults `boxShadow`，ShadowTertiary；ghost/split 不加阴影 | 已补齐（TestProCardVariantAndBoxShadow） |
| headerBordered | 默认 false（pro/es/card/components/Card/index.js:35，2026-11-15） | ui/pro2_card.go:HeaderBordered | 已对齐 |
| ghost | 默认 false，透明背景/无边框、body 无 padding（同上及 style.js `&&-ghost`，2026-11-15） | ui/pro2_card.go:Ghost/pads/Body | 已对齐 |
| hoverable | 默认 false，hover 三级阴影及透明边框（同上 style.js，2026-11-15） | ui/pro2_card.go:Hoverable 使用现有 hover 阴影；尚未迁移到 Pro 专用 hover token | 暂不做：本轮只补 boxShadow，hover 阴影/边框仍有存量差异，见共享请求 |
| split | vertical/horizontal，共享 surface 与分隔线（pro/es/card/components/Card/index.js、style.js，2026-11-15） | ui/pro2_card.go:Split/content | 已对齐（TestPro2ProCardSplitAndSpan） |
| direction | column/row（pro/es/card/typing.d.ts，2026-11-15） | ui/pro2_card.go:Direction | 已对齐 |
| gutter | 默认 0，官方支持二元方向间距（同上、components/Card/index.js:32，2026-11-15） | ui/pro2_card.go:Gutter；不同方向 gap 用 Layout | 等价写法：普通 gutter 用 Gutter，复杂间距由外部 Row/Column Layout 组合 |
| wrap | 默认 false（pro/es/card/components/Card/index.js:29，2026-11-15） | ui/pro2_card.go:Wrap | 已对齐 |
| colSpan / colStyle | 24 栅格、固定宽度、断点 map / CSS（pro/es/card/typing.d.ts，2026-11-15） | ui/pro2_card.go:ColSpan/ColWidth/Layout | 等价写法：响应式由应用 ScreenBreakpoint 选 ColSpan；CSS 用 layout.Option |
| layout center | 内容居中（同上，2026-11-15） | ui/pro2_card.go:Centered | 已对齐 |
| collapsible / 受控 | 默认 false；collapsed、defaultCollapsed 默认 false，onCollapse（pro/es/card/components/Card/index.js，2026-11-15） | ui/pro2_card.go:Collapsible/Collapsed(*bool)/DefaultCollapsed/Toggled/IsCollapsed | 等价写法：指针受控；内部 State 非受控，Toggled 查询事件（TestPro2ProCardCollapseAndExtra） |
| collapsible icon/header 模式 | `collapsible: icon/header/bool`、collapsibleIconRender（pro/es/card/typing.d.ts，2026-11-15） | ui/pro2_card.go:header 仅整个 header 切换、内置图标 | 暂不做：未开放只点图标模式和自定义折叠图标 API，不是基础层缺口 |
| tabs | tabs.items + 受控 activeKey/onChange（pro/es/card/components/Card/index.js，2026-11-15） | ui/pro2_card.go:Tabs(*string,[]ProCardTab)/TabsCard/TabExtra/TabChanged | 等价写法：指针与查询事件（TestPro2ProCardTabsAndLoading） |
| loading / 禁用 | loading bool 或 ReactNode；无整卡 disabled（同上，2026-11-15） | ui/pro2_card.go:Loading(bool)，内置占位；自定义占位在 Body 分支构建 | 等价写法：自定义 loading 用应用分支；整卡禁用不需要（官方无该 prop） |
| type inner | inner 标题底色/分隔（pro/es/card/components/Card/style.js `type-inner`，2026-11-15） | ui/pro2_card.go:HeaderBordered 只有分隔，无 inner 底色 | 暂不做：缺 inner API/专用令牌，不冒充完全等价 |
| cover | 当前锁定版 Card 解构 cover，但 JSX 未渲染 cover（pro/es/card/components/Card/index.js，2026-11-15） | ui/pro2_card.go:Body 可构建自定义内容 | 不需要：不机械新增锁定版未渲染的入口 |
| checked | checked/onChecked 及选中样式（pro/es/card/typing.d.ts、components/Card/style.js，2026-11-15） | ui/pro2_checkcard.go:CheckCard（本组不改） | 等价写法：交互选择用既有 CheckCard，避免第二套卡片选择模型 |
| Group / Divider | 静态组合入口（pro/es/card/ProCard.js，2026-11-15） | ui/pro2_card.go:Split/Gutter/Direction；通用 Divider | 等价写法：用既有 ProCard 与 Divider 组合，不新增同义构造函数 |
| ref / styles / classNames | React 引用、CSS semantic slots（pro/es/card/typing.d.ts，2026-11-15） | ui/pro2_card.go:ID/Box/BodyPadding/Layout | 不需要：非 DOM 框架；构建器/布局/主题提供原生控制 |
| 键盘 / a11y | 官方 header collapse DOM 交互（pro/es/card/components/Card/index.js，2026-11-15） | ui/pro2_card.go:header button/expanded；root group、actions toolbar | 已对齐（TestPro2Semantics） |
| 尺寸 / 令牌 | header padding 16/24、body 16/24；small header 8/12、body 12/12，圆角 LG（pro/es/card/components/Card/style.js，2026-11-15） | ui/pro2_card.go:pads 默认 16/24，small body 仍 8/12；已有 radius/shadow/border token | 暂不做：small body 纵向 padding 存量差异未扩范围修，建议后续 ProCardToken 统一 |
| EP 外观 | card border 1px、body padding 20、header 18/20、light shadow（ep/theme-chalk/src/card.scss、common/var.scss，2026-11-15） | ui/pro2_card.go:pads/Body，Element border/header/shadow 保留 | 已对齐 |
| shadcn 外观 | border、rounded-xl、py-6、px-6、shadow-sm（shadcn/card.tsx，2026-11-15） | ui/pro2_card.go:pads/Body/shadHeader；shad2dCardRadius、CardShadow | 已对齐 |
| 明暗主题 | 背景/边框/阴影来自 token（上述官方样式，2026-11-15） | ui/pro2_card.go:Body 使用 BgContainer/BorderSecondary/CardBorder/ShadowTertiary，不分支 Dark | 已对齐 |

### ProDescriptions

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状（文件:符号） | 结论 |
|---|---|---|---|
| items / columns / dataSource | Pro schema 转 antd items（pro/es/descriptions/typing.d.ts、schemaToDescriptionsItem.js，2026-11-15） | ui/pro3_descriptions.go:ProDescItem/ProDescriptions/DataSource | 等价写法：有类型切片和 *ProValues；应用持有数据 |
| title / extra | 透传 antd Descriptions（pro/es/descriptions/ProDescriptions.js，2026-11-15） | ui/pro3_descriptions.go:Title/Extra | 已对齐 |
| column 默认 | Pro 不强设 column；antd DEFAULT_COLUMN_MAP xs=1、sm=2、md..xxl=3（xxxl=4，Tango 无该断点）（pro/es/descriptions/ProDescriptions.js:120、antd/es/descriptions/constant.js、index.js:65，2026-11-15） | ui/data_card.go:descDefaultCols/DescriptionsW.build；ui/pro3_descriptions.go:list 未指定时不再强传 3 | 已补齐（TestDescriptionsDefaultColumnMap） |
| column 显式 / Defaults / map | antd 数字优先，map 按最大活动断点解析，无命中回落 DEFAULT_COLUMN_MAP（antd/es/descriptions/index.js、_util/responsiveObserver.js:matchScreen，2026-11-15） | ui/pro3_descriptions.go:Column/ColumnAt/Columns；Defaults `column` 支持整数/map；显式 Column 覆盖 map Defaults | 已补齐（TestProDescriptionsResponsiveColumns、TestDescriptionsDefaultColumnMap） |
| bordered / layout / size | antd 默认无边框、horizontal，默认 large 尺寸；Pro 透传（同上，2026-11-15） | ui/pro3_descriptions.go:Bordered/Vertical/Size，复用基础 Descriptions | 已对齐 |
| span / label / children | schema span、label 与 field 渲染（pro/es/descriptions/typing.d.ts、FieldRender.js，2026-11-15） | ui/pro3_descriptions.go:ProDescItem.Span/Label/Render/Field | 等价写法：Render(func) 代替 ReactNode |
| valueType / valueEnum / readOnly | 使用 ProField 渲染与编辑（同上，2026-11-15） | ui/pro3_descriptions.go:field/cell，ProDescItem.ValueType/ValueEnum/ReadOnly | 已对齐（TestProDescriptionsRequestAndInlineEdit） |
| request / params / actionRef | request(params)、actionRef reload（pro/es/descriptions/typing.d.ts、useFetchData.js，2026-11-15） | ui/pro3_descriptions.go:Request/Reload/Loading/Err/Values | 等价写法：params 用闭包捕获，Reload 是构建器方法；状态用查询方法 |
| loading / error | fetch loading/error（pro/es/descriptions/useFetchData.js，2026-11-15） | ui/pro3_descriptions.go:Show/poll，Loading 遮罩、Alert 错误 | 已对齐（TestProDescriptionsRequestError） |
| editable / 受控状态 | editable 保存、取消与 dataSource 更新（pro/es/descriptions/ProDescriptions.js、FieldRender.js，2026-11-15） | ui/pro3_descriptions.go:Editable/Editing/Saved/save/commit；draft 与原值隔离 | 等价写法：异步 done 回调与应用持有 ProValues（TestProDescriptionsRequestAndInlineEdit） |
| emptyText | 默认为 `-`，可自定义（pro/es/descriptions/typing.d.ts、field/AllProField.js，2026-11-15） | ui/form_pro_types.go:proFieldView 默认 `-`；每项 Render 可替换 | 等价写法：Render 处理空值 |
| hide / order | schema hide/order（pro/es/descriptions/typing.d.ts，2026-11-15） | ui/pro3_descriptions.go:输入 []ProDescItem | 等价写法：应用在构建前筛选/排序切片，不新增重复状态 |
| tooltip / copyable / ellipsis | schema 字段展示选项（pro/es/descriptions/typing.d.ts，2026-11-15） | ui/pro3_descriptions.go:ProDescItem.Render | 等价写法：Render 中组合 Tooltip、复制按钮和 Text.Ellipsis |
| 键盘 / 禁用 / 语义 | 编辑操作是可聚焦按钮，readOnly 禁止编辑（pro/es/descriptions/FieldRender.js，2026-11-15） | ui/pro3_descriptions.go:cell；基础 Descriptions table/row/header/cell、标题命名 | 已补齐（TestProDescriptionsA11y） |
| 尺寸 / EP / shadcn / 明暗 | Pro 透传 antd Descriptions；EP sizes/bordered padding（antd/es/descriptions/style/index.js、ep/theme-chalk/src/descriptions.scss，2026-11-15）；shadcn 无 ProDescriptions 对应组件 | ui/data_card.go:DescriptionsW.pads/itemPad/build，DescriptionsToken 统一管理；Pro wrapper 不新增主题分支 | 已对齐 |

### ProField

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状（文件:符号） | 结论 |
|---|---|---|---|
| read / edit 默认 | mode 默认 read（pro/es/field/ProFieldCore.js:20，2026-11-15） | ui/form_pro_field.go:ProFieldOf/Mode，Defaults `mode` read/edit | 已对齐（TestProFieldOfModes） |
| update | 多数类型按 edit；Radio/Checkbox/Cascader/TreeSelect 只接受严格 edit，update 返回 null（pro/es/field/internal/fieldMode.js、components/{Radio,Checkbox,Cascader,TreeSelect}/index.js，2026-11-15） | ui/form_pro_field.go:撤掉本批新增 ProFieldUpdate | 等价写法：ProFieldEdit；官方 update 在 radio/checkbox/cascader/treeSelect 下不渲染，Tango 不照搬（TestProFieldOfEditEnum） |
| value / 受控 | text / fieldProps.value、onChange（pro/es/field/ProFieldCore.js:43，2026-11-15） | ui/form_pro_field.go:ProFieldOf(c,fd,*any)/Changed | 等价写法：应用值指针和 Changed 查询；nil 指针构建临时空值 |
| readonly | 强制 effectiveMode=read（pro/es/field/ProFieldCore.js:41，2026-11-15） | ui/form_pro_field.go:Show 的 fd.Readonly 优先 | 已对齐（TestProFieldOfModes） |
| valueType / valueEnum | Pro valueType 分派及枚举映射（pro/es/field/AllProField.js，2026-11-15） | ui/form_pro_types.go:proFieldView/proFieldControl；ProField.ValueType/ValueEnum | 已对齐（TestProFieldOfModes、TestProFieldOfEditEnum） |
| emptyText | 默认 `-`（pro/es/field/AllProField.js:422，2026-11-15） | ui/form_pro_types.go:proFieldView；ProField.Render 自定义展示 | 等价写法：Render 自定义空值 |
| render / renderFormItem / fieldProps / request | 读写渲染和编辑控件配置（pro/es/field/ProFieldCore.js、PureProField.js，2026-11-15） | ui/form_pro_field.go:ProField；ui/form_pro_types.go:proFieldControl；Render/Options 等 | 等价写法：ProField 配置与应用闭包代替 React props/ref/render-prop |
| 键盘 / disabled / loading | 由各 valueType 的控件承担（pro/es/field/components/*/index.js，2026-11-15） | ui/form_pro_field.go:Show；复用具体控件交互，label 命名 editor | 已对齐（TestProFieldOfEditEnum） |
| 尺寸 / EP / shadcn / 明暗 | ProField 不独立定义各控件尺寸，交由 antd 控件（同上，2026-11-15）；EP/shadcn 无 ProField 包装层 | ui/form_pro_types.go:proFieldControl/view 复用现有控件令牌 | 已对齐 |

### ProFormFieldSet / FieldSet

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状（文件:符号） | 结论 |
|---|---|---|---|
| value / parts / onChange | 数组值组合子控件（pro/es/form/components/FieldSet/index.d.ts、index.js，2026-11-15） | ui/pro3_fieldset.go:ProFieldSet.Parts/ProValues.Parts/pro3FieldSetControl | 等价写法：[]any 为组合值；ProFormFieldSet 返回 ProField 描述配置，并非新 widget 构造函数 |
| 外部 Form 入口 | 官方同一个 ProFormFieldSet 可用于 Form（同上，2026-11-15） | ui/pro3_fieldset.go:FieldSet(c,*FormW,fs,vals)；配置转换与直接构建不重复实现编辑器 | 等价写法：FieldSet 复用 ProFormFieldSet 描述和 proFieldItem |
| Initial / 非受控缺省 | Form initialValue（pro/es/form/components/FieldSet/index.js、FormItem，2026-11-15） | ui/pro3_fieldset.go:FieldSet 缺 key 时复制 Initial，每帧检查；已存在 key（含全 nil parts）不覆盖；清空写 nil parts，不 delete key | 已补齐（TestFieldSetInitialValues） |
| separator / space | type 默认 space、可传 SpaceProps（pro/es/form/components/FieldSet/index.js:28，2026-11-15） | ui/pro3_fieldset.go:Separator，Defaults `ProFormFieldSet.separator` | 等价写法：separator 字符串与布局间隔代替 Space props |
| group / transform / convertValue | type group = Space.Compact；组合值转换（pro/es/form/components/FieldSet/index.js、index.d.ts，2026-11-15） | ui/pro3_fieldset.go:Parts 可定义各控件；值转换由应用完成，无 compact 外观 API | 暂不做：缺 FieldSet compact API/关联控件连续边框表达；普通组合和外部转换可用 |
| required / validate | FormItem rules 与组合校验（pro/es/form/components/FieldSet/index.js，2026-11-15） | ui/pro3_fieldset.go:Required/Validate/pro3FieldSetCheck/Help | 已对齐（TestFieldSetInForm） |
| 受控 / 键盘 / 禁用 | 数组 value、每项独立控件（同上，2026-11-15） | ui/pro3_fieldset.go:pro3FieldSetControl，vals 持有值；每项使用 ProField 控件键盘/禁用状态 | 已对齐（TestFieldSetInForm） |
| a11y | 组合 FormItem 及控件标签（同上，2026-11-15） | ui/pro3_fieldset.go:group 以 Label 命名 | 已补齐（TestFieldSetA11y） |
| 尺寸 / EP / shadcn / 明暗 | 无独立 Pro FieldSet 主题尺寸；由 Space/FormItem/控件组成（同上，2026-11-15）；EP/shadcn 无同名组件 | ui/pro3_fieldset.go:布局复用 Form/控件令牌，无新增 theme 分支 | 已对齐 |

### ProSkeleton

| 规范项 | 官方定义（来源，2026-11-15） | Tango 现状（文件:符号） | 结论 |
|---|---|---|---|
| type / 默认 | type 默认 list，另有 result/descriptions（pro/es/skeleton/index.js，2026-11-15） | ui/pro2_skeleton.go:ProSkeleton/SkeletonList/Result/Descriptions | 已对齐（TestPro2ProSkeleton） |
| active / pageHeader | active 默认 true、pageHeader 非 false 显示（pro/es/skeleton/components/{List,Descriptions,Result}/index.js，2026-11-15） | ui/pro2_skeleton.go:Active/PageHeader；Defaults active/pageHeader | 已对齐 |
| statistic 默认 | 官方取 Object.keys(screens) 第一个 true 键；xs 和 sm 均为 2；antd sm 起累计 min-width 且键序 xs,sm,md,lg,xl,xxl,xxxl，因此浏览器稳定为 2，SSR md 回退可为 4（pro/es/skeleton/components/List/index.js:22-45、antd/es/_util/responsiveObserver.js:6-14,77、grid/hooks/useBreakpoint.js，2026-11-15） | ui/pro2_skeleton.go:默认 statistic=2；已删除 2/4/6 断点逻辑，不追随 SSR 暂态 | 已补齐（TestProSkeletonStatisticDefault） |
| statistic 显式 | size 覆盖默认，false 隐藏（pro/es/skeleton/components/List/index.js，2026-11-15） | ui/pro2_skeleton.go:Statistic(n)，Defaults statistic 优先于 2；0 隐藏 | 已补齐（TestProSkeletonStatisticDefault） |
| list 页 list / toolbar | list 默认 5；toolbar 与 list 独立，二者非 false 才渲染 card（pro/es/skeleton/components/List/index.js:251-282，2026-11-15） | ui/pro2_skeleton.go:List(n)/Toolbar；Defaults list/toolbar；List(0) 不再隐藏 toolbar | 已补齐（TestProSkeletonListAndToolbar） |
| descriptions 页 list | TableSkeleton size 默认 4；list=false 同时隐藏 Line 和表格（pro/es/skeleton/components/Descriptions/index.js:227-300，2026-11-15） | ui/pro2_skeleton.go:descriptions；未设 List 用 4，List(n)/Defaults list 控制正文行数，另加表头；0 同时隐藏分隔线和表格 | 已补齐（TestProSkeletonListAndToolbar） |
| descriptions / table 列数 | 首个 true 键规则；Descriptions 的 enum xs=1、sm=2，表格另有一个 flex-3 尾列（pro/es/skeleton/components/Descriptions/index.js:10-17,112-114,TableItemSkeleton，2026-11-15） | ui/pro2_skeleton.go:descCols，XS=1，其余=2；表格加末尾 flex-3 列 | 已补齐（TestProSkeletonListAndToolbar） |
| result | 结果页图标/标题/描述/操作占位（pro/es/skeleton/components/Result/index.js，2026-11-15） | ui/pro2_skeleton.go:result | 已对齐（TestPro2ProSkeleton） |
| actionButton | list 内可控制动作占位（pro/es/skeleton/components/List/index.js，2026-11-15） | ui/pro2_skeleton.go:无 ActionButton modifier | 暂不做：缺独立 API；不伪装已有 Toolbar 即该行为 |
| 子 skeleton 导出 | PageHeaderSkeleton/TableSkeleton 等（pro/es/skeleton/index.js，2026-11-15） | ui/pro2_skeleton.go:三种 page 模式私有组合 | 不需要：不新增同义细粒度入口；普通 Skeleton 可由应用组合 |
| loading / 受控 / 键盘 / 禁用 | 本身只占位、无真实内容状态/交互（同上，2026-11-15） | ui/pro2_skeleton.go:Show；应用分支控制显示；root busy status，bar 为装饰，无键盘节点 | 不需要：静态占位不是可禁用的交互控件 |
| 尺寸 / 令牌 | 页面组合 Card/Skeleton/Space（pro/es/skeleton/components/*/index.js，2026-11-15） | ui/pro2_skeleton.go:card/bar；ui/data_skeleton.go:skelBlock，复用 SkeletonToken；未像素重建全部私有子组合 | 等价写法：基于现有骨架原语的页面组合，非 DOM 像素复制 |
| EP / shadcn / 明暗 | EP skeleton.scss animation、shadcn Skeleton animate-pulse rounded-md bg-accent（ep/theme-chalk/src/skeleton.scss、shadcn/skeleton.tsx，2026-11-15） | ui/pro2_skeleton.go:bar 使用基础 SkeletonToken 与 CardShadow，Active(false) 关闭动画 | 已对齐 |

## 验证与稳定层

- 新增公开 API：`(*ProCardW).BoxShadow(bool) *ProCardW`、`(*ProDescriptionsW).ColumnAt(Breakpoint,int)`、`Columns(map[Breakpoint]int)`；Defaults 键 `ProCard.boxShadow`。`ProFieldUpdate` 已撤掉，不生成其 API 记录。
- 上述两个构建器不在 `api/stable/ui.txt`，本轮没有稳定类型新增方法；已有签名/枚举值未破坏。
- 测试覆盖：ProCard 默认边框及三主题显式无边框/ghost/split/阴影、ProDescriptions 显式/Defaults map 优先级、基础与 Pro 的 xs/sm/md+ 默认列数、FieldSet Initial 已有值及跨帧不覆盖、ProFieldEdit enum、Skeleton 默认/显式数量与独立显隐。
- 最终测试与 gallery 实际结果见 `.superpowers/sdd/2026-11-14-spec-align-b4/group-A-fix-report.md`。不将别组暂态编译错误记作本组通过。

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 无新增基础层 blocker | 本组全部 | 已有 ScreenBreakpoint、布局、控件、paint 支持本轮修复 | 已实现；未借本轮修改基础层 |
| 连续复合控件边框表达 | FieldSet group | compact 子控件统一首尾圆角/边框衔接约定 | 暂不做 group；普通 parts/space 不受影响 |

## CHANGELOG 草稿

### 新增

- ProCard `BoxShadow(on bool)`，Defaults 键 `boxShadow`（默认 false），使用已有 `ShadowTertiary`，边框透明。
- ProDescriptions `ColumnAt` / `Columns`，Defaults `column` 支持 `map[Breakpoint]int`；显式 `Column` 优先于 map Defaults。
- 直接 `FieldSet` 路径支持 `Initial`：仅 key 缺失时复制，已有值（包括全 nil parts）不覆盖；删除 key 会重新初始化，清空应保留 key 并写 nil parts。

### 变更

- Ant ProCard 默认无边框 → outlined（所有主题未显式设置时 bordered=true）；显式 Borderless、ghost、split 子卡仍无边框。依据 Pro v3 Card/index.js variant 默认 outlined。已有 `TestShad2dProCardAntUnchanged` 保留半径断言并新增 outlined 期望，未删用例。
- ProSkeleton statistic 默认固定 4 → 2；遵循 Pro v3 浏览器实际行为，而不是 lookup 表表面上的 2/4/6。显式 Statistic(n) 和 Defaults statistic 保持优先。
- 基础 Descriptions / ProDescriptions 默认固定 3 列 → xs 1、sm 2、md 起 3 列（Tango 无 xxxl）。窄宽度下布局行数会增加；显式 Column / Defaults 数字仍保持指定列数。
- ProSkeleton descriptions 原固定 4 个总表格行且忽略 List → 默认 4 正文行 + 表头；List(n) 指定正文行，List(0) 隐藏分隔线/表格；描述项列数固定 3 → xs 1、其余 2，表格另有 flex-3 尾列。
- ProSkeleton list 页 List(0) 原连 toolbar 一起隐藏 → 只隐藏 rows；Toolbar(false) 独立隐藏工具栏。已有 `TestPro2ProSkeleton` 的“全空”调用增加 Toolbar(false)，不删除原用例。
- 本批尚未发布的 `ProFieldUpdate` 被撤回；等价写法是 `ProFieldEdit`。对应新增测试替换为编辑态 enum 断言，不改变既有发布 API。

## 共享文件改动请求

1. 主线程统一运行 API 生成器；本组仅新增 `ProCardW.BoxShadow`、`ProDescriptionsW.ColumnAt/Columns`，不应包含 `ProFieldUpdate`。不提升稳定承诺。
2. 主线程将上述草稿合入 CHANGELOG.md 未发布节，并更新 AUTHORING §14 及 ui-coverage 的旧“完整”结论，保留本文列出的存量未做项。
3. ProCardToken 后续统一请求（本轮不改 theme）：默认/小号 header/body padding、hover 阴影/透明边框、inner 底色。已有 ShadowTertiary 足够本轮 boxShadow，无需新阴影令牌。存量 pro2El/pro2Shad 外观分支未扩大；后续宜搬到 token，不能称本文件已完全无 Flavor 分支。
4. 黄金图：未运行 TestGolden 或 -update；Descriptions/ProCard 默认变化可能影响涉及它们的共享 golden/布局断言，请主线程统一验收与批准重录，禁止机械覆盖。
5. Locale / theme / Defaults 注册文件本轮未改；组件内直接读取已有 ConfigProvider.Defaults，未新增硬编码组件界面文案。
6. 按主线程转来的组 A2 请求，ui/pro2_card.go:content 遇前兄弟 label `pro2-stat-op` / `pro2-stat-divider` 时不再叠画 split 自动分隔线；补 TestProCardOwnDividerBefore。节点不保存 kind，识别当前 StatisticCardOperation/Divider 已写入的 Label。
7. Gallery 的 ProCard、ProSkeleton、ProDescriptions 页面已组内出图（具体结果见 fix report）；ProDescriptions 示例保留请求/行内编辑，默认列示例取消显式 Column。后续建议主线程补全 viewport 窄宽度的共享 gallery 场景；本轮默认断点已由行为测试覆盖。
