# 第 4 批组 C：Layout / List / Provider

核对日期：2026-11-15。基线：`@ant-design/pro-components@3.1.14-2`；下文 `pro/` 均指 `/tmp/tango-ref/pro/package/es/`。Ant `antd@6.7.0`；Pro 模板 `/tmp/tango-ref/adpro/repo` commit `42f2cb1`。模板参考 `config/defaultSettings.ts`、`src/app.tsx`。参考源码只读，不复制实现。

## 复核原则与决定

- 旧 `ui-coverage.md §4` 基于 v2；✅不直接继承。以下按 v3 明确每个组件的覆盖边界，**有未补齐的功能/令牌便不能继续声称无缺口的完整**。
- ProBreadcrumb 是 RouteContext + Breadcrumb 的薄入口，复用 `BreadcrumbW` 的绘制、当前项和键盘。不是第二套面包屑。
- PageLoading 是 `Spin(size=large)` 加 `paddingBlockStart:100` 的包装。React loadable 的 isLoading/pastDelay/timedOut/error/retry 被源码解构丢弃，不能错误地当可见性控制；Go 用 `Spinning`。取消业务请求由应用负责，停止 indicator 不是请求取消。
- TopNavHeader 是既有 `proLayout.header` 的公开薄入口，ProLayout top 反过来调用它。源码 BaseMenu 强制 `collapsed:false`，桌面 top 顶栏本身**没有侧栏折叠按钮**；窄宽度由现有水平 Menu overflow 收起，复用键盘和弹层。移动端 DefaultHeader 换 GlobalHeader 是外层 ProLayout 的责任，不给 TopNavHeader 添加伪造 collapse API。
- Provider 模块的 React Context 不增添同义 ProProvider/ProConfigProvider 构造函数：`ConfigProvider`、`Context.Locale/ThemeFor/ConfigProp` 已是等价写法。自定义 valueTypeMap 使用各组件 Render；目前没有全局 valueType renderer registry，明确列缺口。
- 独立新类型、既有 Pro 类型在 `api/stable/ui.txt` 均不在稳定白名单；本组新增方法不增加稳定承诺。API golden 由主线程生成。

## ProLayout

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| side/top/mix、路由状态 | `pro/layout/ProLayout.js`、`context/RouteContext.d.ts`；2026-11-15 | `ui/prolayout.go:ProLayout`、`routeInfo`；route 作用域扩展到 header/menu render hook | 已补齐（TestProBreadcrumbFromRouteContext） |
| 默认值 | `layout/defaultSettings.js`：light、side、Fluid、fixedHeader=false、fixSiderbar=true；模板是 mix | `ProLayout` 默认 light、scrolling header；`DefaultProSettings` 同步；新增 cfg.NavTheme 供显式 dark、cfg.FixedHeader 供显式固定（NoFixedHeader优先） | 已补齐（TestProLayoutV3Defaults） |
| 受控/非受控 | `ProLayout.js` useControlledState、defaultCollapsed；md 默认折叠、sm/xs 手机 | 现有 Selected/Collapsed 指针与 Breakpoint；nil Collapsed 不保留状态，不自动 mobile drawer | 暂不做（完整移动侧栏 drawer/无指针自动折叠尚未实现；不是 ✅ 无缺口） |
| 菜单/渲染 hook | `ProLayout.d.ts` menuDataRender/menuItemRender、actionRef、splitMenus/menu.request | `ProLayoutConfig` 支持静态 Menu、MenuItemRender、Header/Menu/Footer hooks；无 splitMenus/menu.request/actionRef | 等价写法（应用提供动态 Menu；任意菜单 UI 替换和 splitMenus 尚无等价实现，见待补齐） |
| 尺寸 | `ProLayout.js`：side 256、mix 215、collapsed 64；provider `typing/layoutToken.js` header 56、半透明 elevated .6/.8 | 共享线程新增 `Components.Layout.Pro*` 后，本组按ProSiderWidth256/ProMixSiderWidth215、ProHeaderBg/Height、ProLogoSize/TitleFontSize/Gap消费；基础Layout/Sider不变 | 已补齐（TestProLayoutSharedTokenConsumption；主题数值测试由主线程补） |
| a11y/键盘/主题 | React BaseMenu/按钮；shadcn `sidebar.tsx`、EP `container.scss` 为不同主题外观；2026-11-15 | Menu 复用现有键盘、Sider landmark；TopNavHeader landmark、用户区、窄宽度测试 | 已补齐（TestTopNavHeaderNavigationAndUser、TestTopNavHeaderNarrowFoldsMenu） |
| 浏览器文档标题/背景图片/滚动特效 | `getPageTitle.js`、`ProLayout.js` bgLayoutImgList | PageTitle 为绘制标题；没有 document/title、CSS hashed classname | 不需要（Go 没有 DOM；宿主标题由应用设置；背景图片列表未实现） |

**旧 ✅ 判定：不宜继续无条件 ✅**。核心布局已覆盖，动态菜单/splitMenus和移动端行为仍有实质差异。

## DefaultFooter

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| links/copyright/空态 | `pro/layout/components/Footer.js`、`GlobalFooter/index.js`；2026-11-15 | `ui/prolayout.go:DefaultFooterW`；无内容不绘制；Links/Copyright/Clicked/OnClick | 已对齐 |
| href/target | GlobalFooter links[].href/blankTarget | FooterLink 只有 Key/OnClick，外部打开由应用回调 | 等价写法（不自动外部导航） |
| copyright icon | Footer.js 自动 CopyrightOutlined | Copyright(s) 按原样绘制，调用者自行提供 © | 等价写法（Go 字符串含 ©） |
| 令牌/交互 | `GlobalFooter/style.js`；链接色 secondary、marginInlineEnd 8、marginBlockStart48/End24、paddingInline16 | `DefaultFooterW.Show` 消费Layout.DefaultFooterLinkGap8/PaddingInline16/MarginTop48/MarginBottom24；Link 可键盘 | 已补齐（共享token消费；Link键盘复用） |

## MenuPath

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 匹配菜单路径 | `pro/layout/utils/getMenuData.js`、ProLayout getMatchMenu；2026-11-15 | `ui/prolayout.go:MenuPath` 用显式 key 递归，忽略组标题/Divider | 等价写法（显式 Key 对应路由 pathname；没有 URL pattern engine） |
| 空/未知路径 | 同上 | 找不到返回 nil；应用决定 fallback | 已对齐 |

## PageContainer

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 标题/面包屑上下文与空 header | `pro/layout/components/PageContainer/index.js:memoRenderPageHeader`；2026-11-15 | `ui/pro2_page.go:buildHeader/main` 继承 RouteInfo.PageTitle/BreadcrumbItems，无标题/动作/内容时省略 header | 已补齐（TestPageContainerNoEmptyHeader；RouteContext 测试） |
| 内容与加载 | 同文件 loadingDom：PageLoading 替换 children，不遮住 header | `PageContainerW.Loading` 改用公开 PageLoading | 已补齐（TestPageContainerLoadingUsesPageLoading） |
| tabs/extra/footer | 同文件 renderFooter、FooterToolbar | 现有 Tabs/TabExtra/Footer/FooterExtra/Fill、FooterToolbar | 已对齐 |
| fixedHeader/waterMarkProps/headerRender | 同文件 Affix、Watermark、pageHeaderRender | NoHeader、Content 等 Go callback；没有独立页头 Affix/watermark 配置 | 等价写法（应用组合 Affix/Watermark；不是同 prop 直传） |
| 精确默认边距/背景 | provider `typing/layoutToken.js` pageContainer inline40/block32、bg transparent；style/index.js header top8/bottom16，children top0（无头 top32） | `PageContainerW.main/buildHeader` 消费Layout.Page*令牌，Ant inline40/block32、header top8/bottom16、header/body top0、背景透明；Element20、shad24由令牌设置 | 已补齐（TestProLayoutSharedTokenConsumption、TestP2CPageContainerTabs） |
| 明暗/禁用/控制 | 表头无 disabled 语义；loading false；Tabs 指针受控 | 使用上下文主题、Tabs/Spin/按钮继承 | 已对齐 |

**旧 ✅ 判定：主要行为与共享默认样式已补；复杂header fixed/watermark采用组合等价写法。**

## FooterToolbar

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 固定/extra/actions | `pro/layout/components/FooterToolbar/index.js`；2026-11-15；固定 bottom0，width 扣 siderWidth，extra/children 两端 | `ui/pro2_page.go:FooterToolbarW` Extra/Body、Sticky/Target；默认Fixed使用现有ScreenFill Overlay+绝对布局持续贴viewport/Target底；Fixed(false)显式flow；Sticky保留Affix阈值行为，Target继续可用 | 已补齐（TestFooterToolbarDefaultFixed、TestFooterToolbarFixedBounds、TestFooterToolbarFixedTargetBounds） |
| 模糊与明暗 | `FooterToolbar/style/index.js` backdropFilter blur8、bgContainer .6、lineHeight64、paddingInline24 | paint.BackdropBlur 原语已具备，旧 token BlurRadius/FrostedBg；bar height56 | 暂不做（共享 lineHeight64 token 与默认 sticky 决策；现有三主题 frosted 保留） |
| 语义/禁用/键盘 | toolbar；子控件继承状态 | Locale.PageContainer.FooterToolbar、RoleToolbar，复用 Button | 已对齐 |

**旧 ✅ 判定：吸底可组合、尺寸已对齐，默认fixed、Fixed(false)用于显式flow；已有Sticky保留。**

## PageHeader（Pro layout 导出，并非旧 antd PageHeader）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| API | `pro/layout/components/PageHeader/index.d.ts`；2026-11-15：title/subTitle/backIcon/onBack/breadcrumb/tags/footer/extra/avatar/contentWidth/layout/ghost/children | `ui/nav_pageheader.go:PageHeaderW` 支持对应内容 callback、BreadcrumbItems、Ghost、Padding、BackClicked；本组只读，不跨文件改 | 等价写法（React node→callback；onBack→BackClicked） |
| 默认值/交互/加载 | index.js ghost=true（2026-11-16 重新核对锁定的 @ant-design/pro-components@3.1.14-2 源码）；onBack 存在才显示 backIcon，Role/Button 由 antd | 既有 Back 显式启用，Ghost Defaults；loading 用 PageContainer/Spin | 已对齐 |
| 固定宽度与精确样式 | PageHeader/style/index.js `.wide maxWidth1152` 仅 top+Fixed；avatar、heading responsive | 现有 PageHeader 无独立 ContentWidth/layout；容器可 GridContent 包裹 | 等价写法（布局组合；不是完整 props 复刻） |

## GridContent

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 默认 Fluid/宽度 | `pro/layout/components/GridContent/index.js` 与 `style.js`：Fixed 且 context.layout=top，maxWidth1152；2026-11-15。源码注释1200已过时，以实际 CSS 为准 | `ui/pro4_route.go:GridContentW.Fixed/Body` 按 top 匹配，GridContentFixedWidth=1152；独立使用显式 Fixed/MaxWidth 保留 | 已补齐（TestGridContentWideOnlyInTopLayout） |
| context/覆盖 | props 优先 RouteContext | Defaults→ContentWidth 链式覆盖，未指定继承 RouteInfo | 已对齐 |
| a11y/禁用/明暗 | 无独立交互；内容容器 | 透明 Row/Column，不加无意义状态 | 不需要（纯布局） |

## RouteContext

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| scope | `pro/layout/context/RouteContext.d.ts`、ProLayout.js Provider 包围整个布局；2026-11-15 | `ui/prolayout.go:ProLayout` 包围完整 shell，支持内外层恢复；RouteInfo.BreadcrumbItems 保留原 callback/Href | 已补齐（TestProBreadcrumbFromRouteContext） |
| layout/collapsed/mobile/breadcrumb/selected/title/contentWidth | 同上 | `ui/pro4_route.go:RouteInfo`；外层没有时 InLayout=false | 已对齐 |
| hasHeader/siderWidth/footer flags/setter/matchMenus/waterMark | 同上 | 没有相应字段，PageContainer 内部 filled 标记代替 hasPageContainer | 等价写法（即时模式状态无需 React setter；外部 footer flags 等尚未暴露） |

## SettingDrawer

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 受控/非受控/open/尺寸 | `pro/layout/components/SettingDrawer/index.js`：collapse→open、Drawer size300、placement right；2026-11-15 | `ui/pro4_setting.go:SettingDrawerW` Open 指针/内部 State、NoTrigger、Drawer宽300 | 已补齐（TestSettingDrawerV3Rules） |
| 配色 | 同文件 colorList 9项：techBlue#1677ff、daybreak#1890ff、dust#f5222d、volcano#fa541c、sunset#faad14、cyan#13c2c2、green#52c41a、geekblue#2f54eb、purple#722ed1 | `ProSettingsPresetColors` 补 daybreak；Colors 覆盖 | 已补齐（TestSettingDrawerV3Rules） |
| 切布局联动 | 同文件 changeSetting：top→Fixed，其他→Fluid；mix强制light | sections 统一在选择后联动 | 已补齐（TestSettingDrawerV3Rules） |
| fixed sider 禁用 | LayoutChange.js：layout=top 时禁用 fixSiderbar | Disabled scope 包裹 Switch | 已补齐（TestSettingDrawerV3Rules） |
| themeOnly/enableDarkTheme | index.js themeOnly 隐藏 layout/others，enableDarkTheme 默认 false，v3 无 dark 菜单选项 | 现有 hideLayout/hideOther、保留 dark/realDark 选项（需扩展公开开关） | 暂不做（本组未把全部 Settings v3 API 冒称完整；应后续补齐） |
| 色弱 | index.js body CSS filter invert80% | ProSettingsProvider 使用 HighContrast 近似 | 暂不做（缺统一整窗颜色滤镜；HighContrast 不是同义行为） |
| URL/copy/hint | index.js disableUrlParams/hideHintAlert/hideCopyButton | 不绑定浏览器 URL，不复制 JSON 配置 | 不需要（宿主应用自行持久化和导出） |
| a11y/键盘/主题 | BlockCheckbox 图片选择 | RadioGroup+命名 Radio、roving keys、Switch/Select继承 | 已对齐（TestShad2dSettingDrawer、TestSettingDrawerV3Rules） |

**旧 ✅ 判定：不宜无条件完整；全局 color filter、enableDarkTheme/themeOnly 尚有差异。**

## ProSettingsProvider

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 设置分发 | Pro v3 没有同名出口；Pro模板 src/app.tsx 合并 settings 到 layout；2026-11-15 | `ui/pro4_setting.go:ProSettings.Apply`、ProSettingsProvider：Dark/Primary/Compact | 等价写法（Tango 辅助，不新增 Provider 重复实现） |
| defaults | pro/layout/defaultSettings.js：light/side/Fluid/fixedHeader=false/fixSiderbar=true | DefaultProSettings 同步；模板 mix 是模板配置，不误当组件默认值 | 已补齐（TestProLayoutV3Defaults） |
| 色弱/隔离 | React Provider context；CSS filter global | ConfigProvider 官方约定 Theme 为窗级；弱色缓存 HighContrast | 暂不做（缺整窗滤镜，不把其当精确对齐） |

## ProConfigProvider / ProProvider（provider 模块）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| token/dark/intl | `pro/provider/index.d.ts`、index.js；2026-11-15 | `ui/nav_config.go:ConfigProvider` Theme/Dark/Components/Locale/Defaults；Context.ThemeFor/Locale/ConfigProp | 等价写法（一个公共 provider 入口；无需同义 Go ProProvider） |
| 嵌套/Defaults隔离 | React Context 合并默认值 | navConfigStack 继承，局部 Defaults pop；组件用 Prop 读取 | 已对齐（新增三入口 Defaults 测试，现有 ConfigProvider 测试） |
| valueTypeMap | provider/index.d.ts 注册全局 valueType render/formItemRender | ProField.Render/ProColumn.Render/ProDescItem.Render 是局部 callback | 暂不做（全局自定义 valueType 注册表缺失；不需要改基础层，但跨组共同设计） |
| hashed/prefixCls/useStyle/cache/needDeps | DOM CSS-in-JS 与 React deps | 自绘节点和主题令牌，无 CSS class hash | 不需要 |

## App（antd App，非 Pro provider 导出）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| Context API | `/tmp/tango-ref/antd/package/es/app/App.js`、useApp.js；2026-11-15 | `ui/pro4_app.go:App/UseApp` 提供 Message/Notification/Confirm/Info；容器与嵌套 scope | 等价写法（Context-aware 静态调用在 Go 原生可用） |
| message/notification config | 同上与 message/interface.d.ts | AppMessageConfig/NotificationConfig Duration/Placement/ShowProgress/MaxCount；外层回退 | 已对齐（TestShad2dApp） |
| maxCount 与关闭回调 | antd 配置透传 message.useMessage/notification.useNotification | App.Body与AppAPI通过ConfigProvider.Defaults复用共享Message/Notification maxCount；移除裁slice，OnClose按共享生命周期执行 | 已补齐（TestAppMaxCountUsesSharedCloseLifecycle） |
| component/style reset | App component=false 无外框 CSS reset | AppW.Layout+Column；文本直接使用主题 | 不需要（没有 DOM） |

## ProList

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| v3 columns/listSlot | `pro/list/index.d.ts/index.js` metas 已弃用，推荐 columns+listSlot；2026-11-15 | `ui/pro3_list.go:ProListMetas` 仍为类型安全 Go callback；无搜索/编辑 column schema | 等价写法（静态 metas callback 仍受 v3 向后兼容；新 columns API 尚待跨表格共享设计） |
| 默认选项 | index.js search/options/rowSelection/pagination=false；ProListBase split 默认true | 无搜索/选列工具，Selection/Pagination显式开启，split=true | 已对齐 |
| 内容布局/折叠 | Item.js：header+content 纵向，collapsed隐藏description/content/expandedRowDom；ListView.js defaultExpandAllRows=true | item 调整 content在title下；默认全展开；DefaultExpandAllRows（只在初始挂载应用，追加/替换数据不重套）、ExpandedRowKeys、ExpandRowByClick、RowExpandable | 已补齐（TestProListV3ExpandDefaults、TestProListV3ExpandOptions） |
| checkbox/expand顺序 | Item.js header-option：checkbox 在 expand icon 前 | item 调整顺序 | 已补齐（TestProListV3ExpandDefaults） |
| grid/card actions | Item.js cardProps→CheckCard extra=actions，content在body | card 将 actions移到header extra | 已补齐（TestProListV3EmptyAndGrid） |
| empty/loading/paging/selection | ProListBase.js/ListView.js；pagination=false；pageSize10；loading Spin | 复用 List/Loading/Pagination；选择有命名checkbox、Selected语义 | 已对齐（TestProListV3LayoutStates、TestProListV3EmptyAndGrid、既有TestProList*） |
| vertical | ProListBase itemLayout horizontal/vertical；vertical action在内容下 | ProListW.ItemLayout vertical | 已补齐（TestProListV3LayoutStates） |
| request/errors | ProTable request；服务端分页时禁止二次slice | 现有 Request异步seq防旧响应、Retry，服务端pager | 已对齐（既有ProList Request测试） |
| 响应式/令牌/三主题 | style/index.js item padding8、radius token；content marginTop12；SM wrap，MD action margin24；itemCardProps独立于grid | 复用List token/Grid；旧grid分支有Flavor判断，未新增分支；精确ProList token/SM item wrapping未全覆盖 | 暂不做（共享独立ProList token请求；grid与cardProps概念目前耦合） |
| itemHeader/itemTitle/itemRender、类型new/top/inline/subheader、editable | index.d.ts、Item.js | 既有metas仅内容区callback，不能换完整行/通用编辑状态机 | 暂不做（不复制ProTable状态机；与D组后续统一） |

**旧 ✅ 判定：v3 核心行为已补，完整列 schema/editable/render 回调/响应式精确视觉仍有缺口，不能继续无条件 ✅。**

## ProBreadcrumb

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| RouteContext breadcrumb/props覆盖 | `pro/layout/components/PageContainer/index.js:ProBreadcrumb`；2026-11-15 | `ui/pro4_breadcrumb.go:ProBreadcrumb` 读结构化 RouteInfo.BreadcrumbItems；Items显式覆盖（可空） | 已补齐（TestProBreadcrumbFromRouteContext、TestProBreadcrumbItemsOverride） |
| hierarchy/current/link/a11y | 底层 antd Breadcrumb | 嵌入 BreadcrumbW，使用相同 RoleNavigation、Current 和链接/菜单行为 | 已补齐（TestProBreadcrumbFromRouteContext） |
| defaults/theme | Breadcrumb props透传 | ProBreadcrumb separator Defaults，底层Breadcrumb主题与默认值 | 已补齐（Defaults测试） |

## PageLoading

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 结构和默认值 | `pro/layout/components/PageLoading/index.js`：paddingBlockStart100、textAlign center、Spin size large；2026-11-15 | `ui/pro4_pageloading.go:PageLoadingW` Row center、top100、Spin large | 已补齐（TestPageLoadingSemantics） |
| spinning/delay/description | rest透传Spin；loadable参数丢弃 | Spinning/Delay/Description，Spin自身状态/Driver时钟 | 已补齐（TestPageLoadingDelayAndDescription） |
| a11y/明暗/禁用 | Spin loading语义；无disabled prop | 底层Spin Busy/RoleStatus/Locale.A11y.Loading；三主题共用令牌 | 已补齐（TestPageLoadingSemantics） |

## TopNavHeader

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 导出与结构 | `pro/layout/index.d.ts`、components/TopNavHeader/index.js；2026-11-15 | `ui/pro4_topnav.go:TopNavHeaderW`：Logo/Title、水平Menu、Actions/UserName；ProLayout复用 | 已补齐（TestTopNavHeaderNavigationAndUser、TestProLayoutTopUsesTopNavHeader） |
| 折叠/窄宽度 | BaseMenu collapsed=false、horizontal；移动端由外层DefaultHeader换GlobalHeader | 水平Menu overflow，未做第二套菜单；桌面不加无来源的sider collapse按钮 | 已补齐（TestTopNavHeaderNarrowFoldsMenu） |
| 固定宽度 | style.js wide max1152；paddingInlineStart16、menu6、logo32/title16 gap6 | 消费Layout.ProFixedWidth1152/ProLogoSize32/ProHeaderTitleFontSize16/ProLogoTitleGap6/ProTopNavPaddingInline16/ProMenuPadding6；Ant基础Layout不改 | 已补齐（TestTopNavHeaderFixedWidth、TestProLayoutSharedTokenConsumption） |
| header click/用户区/a11y | onMenuHeaderClick、ActionsContent/avatarProps | OnMenuHeaderClick可键盘button、nav landmark、应用Actions与UserName | 已补齐（TestTopNavHeaderNavigationAndUser） |
| render/branding | React render props | 现有ProLayout Header/MenuHeader hooks，不公开重复React API | 等价写法 |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 整窗 invert80% 色彩滤镜 | SettingDrawer ColorWeak | 统一窗口级色彩变换（CPU/GPU一致） | 暂不做；HighContrast 仅近似，不能称精确对齐 |

其它待补齐项大多是共享令牌或 Pro 功能层，而非基础层能力不足，不能用“缺基础能力”掩盖。

## CHANGELOG 草稿

### 新增

- 实验性 `ProBreadcrumb/ProBreadcrumbW`：RouteContext 面包屑，复用 Breadcrumb；`Items/Separator/Len/Show`。
- 实验性 `PageLoading/PageLoadingW`：居中大 Spin、100px 顶部间距，`Spinning/Size/Delay/Description/Show`。
- 实验性 `TopNavHeader/TopNavHeaderW`、`TopNavHeaderWideWidth`：复用 ProLayout 顶栏，`Title/Logo/UserName/Actions/ContentWidth/OnMenuHeaderClick/Show`。
- `RouteInfo.BreadcrumbItems`：保留结构化导航数据；完整shell可读RouteContext。
- `ProLayoutConfig.NavTheme` 显式设置 light/dark；旧 LightSider 仍强制 light；`FixedHeader` 明确开启固定且 `NoFixedHeader` 优先。
- ProList `DefaultExpandAllRows/ExpandedRowKeys/ExpandRowByClick/RowExpandable/ItemLayout`。

### 变更

- ProLayout/DefaultProSettings：默认深侧栏→light，fixedHeader true→false（`layout/defaultSettings.js`）。需要深侧栏请显式 `NavTheme:"dark"`。
- GridContent：Fixed上限1200→1152；在side/mix不再收窄（源码 `GridContent/style.js` 而非过时1200注释）；独立显式Fixed/MaxWidth保留。
- PageContainer loading改用PageLoading（原minHeight200居中）；空页头不再占位，默认继承RouteContext的标题和面包屑；PageContainer存在时由其独占内容页头，避免top/mix重复自动标题与面包屑。
- ProList 默认折叠→全展开；checkbox移动到expand图标前；content由标题右侧→下方；折叠同时隐藏description/content；grid卡片actions由body底部→header右侧。
- SettingDrawer宽320→300；配色8→9补daybreak；切top自动Fixed，其他Fluid，mix强制light；top的fixed sider禁用。
- TopNavHeader统一复用原普通header并消费新增独立Pro token：title18→16、logo/title间距12→6、logo行64→56、菜单padding6；Sider默认Ant208→256（mix215），基础Layout/Header/Sider默认令牌不改。shadcn top不再单独绘制shadBrandMark。
- PageContainer Ant inline24→40、body block24→32、header top16→8/bottom16、children top24→0（无header时32）、背景surface→透明；FooterToolbar高56→min64；DefaultFooter gap40→8、margin top48/bottom24、inline16。以上均有Pro v3源码与共享token依据。
- 已有测试期望：P2CPageContainerTabs body gap24/20→0；P2CPageContainerInProLayout改按ProSiderWidth/ProHeaderHeight而非基础Layout派生宽高断言；ProList expand测试先点击Collapse再Expand，维持该用例“点击展开”断言；P2C top模式原假定navigation仅代表侧栏，改为断言唯一顶栏navigation且无额外侧栏；不删除原断言。

## 共享文件改动请求

1. `api/go/ui.txt`、prop golden：主线程运行apitxt生成本组新增实验符号/Defaults键；不加入稳定白名单。本组无稳定类型新增方法。
2. `ui/AUTHORING.md §14`：增加3入口；§13注明ProBreadcrumb复用Breadcrumb、PageLoading复用Spin、TopNavHeader复用ProLayout顶栏；RouteInfo新增结构化面包屑字段。
3. `ui/theme`：新增独立 ProLayout token，避免基础 Layout Ant 外观变动：SiderWidth=256 / MixSiderWidth=215 / CollapsedWidth64；HeaderTitleFontSize16 / LogoSize32 / LogoTitleGap6；TopNavPaddingStart16 / MenuPadding6 / FixedWidth1152；HeaderBg elevated*.6、ScrolledHeaderBg*.8。Ant 来源 `ProLayout.js`、`TopNavHeader/style.js`、provider `typing/layoutToken.js`；EP/shadcn保持各自现有Layout/Sidebar令牌派生。不要更改全局基础 Layout.SiderWidth208。
4. `ui/theme`：PageContainer 独立 PaddingInline40 / PaddingBlock32、HeaderPaddingTop8 / Bottom16、Bg transparent、FixedBg elevated（Ant来源provider `typing/layoutToken.js`）；Element/shadcn分别现有20/24间距派生，不在组件里按Flavor写分支。
5. `ui/theme`：FooterToolbar MinHeight64（当前56）、PaddingInline24；DefaultFooter LinkGap8 / PaddingInline16 / MarginTop48 / MarginBottom24；ProList RowPadding8 / ContentGap12 / GridCardPadding24，响应式 SM wrap。主线程令牌到位后再替换旧常量；不要影响基础Layout。
6. `ui/nav_pageheader.go`（不在本组清单）：如需精确PageHeader宽度/token，与本组记录对照；已有足够callback，本组不越权改。
7. `ui/pro4_app.go`已通过Message/Notification Defaults统一maxCount，移除AppAPI裁slice；无需改共享消息层。
8. `docs/specs/ui-coverage.md §4`：按上文判定更新，不继续写ProLayout/PageContainer/FooterToolbar/SettingDrawer/ProList无缺口✅；新增3组件并标精确视觉待令牌合入。
9. `CHANGELOG.md` 合入上面草稿；gallery总注册/首页如需要，由主线程处理；新page_b4c自注册。
10. Locale：本组没有修改共享Locale；新语义全部复用A11y.Navigation/Breadcrumb/Loading、Collapse/Expand等现有四语言字段。

## 待用户/主线程决定

- Provider等价入口不新增同义Go名是否接受（建议接受，遵守单入口原则）。
- TopNavHeader桌面无collapse按钮符合真实v3，mobile切换GlobalHeader仍需后续ProLayout移动适配；不以增加多余按钮伪造完成。
- 独立GridContent显式Fixed仍可限制宽度（原Go扩展）；严格React仅top限制与存量Go API语义的折中已记录。

## 验证

测试、截图及遗留问题以组C报告的最终运行结果为准；记录中测试名指覆盖目标，不在未跑绿时冒称已通过。

## 独立审阅修正

1. fixedHeader=false 默认解析会覆盖 ProSettings.Apply(true)：确认，新增 cfg.FixedHeader 显式开启标志，NoFixedHeader优先；TestProLayoutExplicitFixedHeader 已看到失败后修复。
2. top/mix 自动头和PageContainer继承重复：确认，main预留空header box，内容只构建一次，PageContainer报告hasPage后只有无PageContainer才填自动头；TestProLayoutPageContainerOwnsHeader 覆盖有/无容器切换。
3. 默认展开每帧套用新key：确认，改为只初始化一次；TestProListV3ExpansionInitializesOnce 覆盖追加、替换和改默认参数。

## 共享令牌合入状态（组C消费完成）

2026-11-15 主线程通知已加入Layout.Pro*/Page*/DefaultFooter*及FooterToolbar.MinHeight/PaddingInline。本组已全部消费，不再等待上面请求3–5的这些部分；尚余ProList专用令牌、ProLayout移动端/动态菜单等功能差异；App maxCount生命周期已补齐。Token消费测试先红（Pro sider仍208）后绿。基础Layout/Header/Sider没有改字段或默认值。

## 最后一轮现有能力修正

- FooterToolbar默认flow→持续fixed（短页/长页均贴viewport或Target底），新增Fixed(bool)允许显式flow；Sticky()保持原Affix阈值吸附，不与Fixed混同；PageContainer.Fill内部显式Fixed(false)保留容器内底栏。原TestPro2FooterToolbar与TestFooterToolbarFrosted的in-flow用例改显式Fixed(false)；gallery FooterToolbar页补flow例子与新默认说明。
- App maxCount的Message/Notification均通过共享Defaults执行正常关闭流程，不再自行裁slice；AppAPI和App子树内plain调用都覆盖。TestAppMaxCountUsesSharedCloseLifecycle先红（OnClose0/0），改后绿（各1次）。
- CHANGELOG新增：FooterToolbarW.Fixed(on bool)；App maxCount淘汰反馈时现在运行OnClose，plain调用同样继承App配置。
