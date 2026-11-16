# 第 3 批 · 组 L 提示与状态 对照记录

对照日期：2026-11-15。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：antd@6.7.0（`antd/package/es`）、element-plus@2.14.8（`ep/package`）、shadcn-ui@e62c9435（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）、sonner@2.0.7（`/tmp/tango-ref/sonner/package`，shadcn v4 的 pnpm-lock 锁定版本，本批 `npm pack` 下载）。下文省略这些前缀：`antd` 指 `antd/package/es`，`ep` 指 `ep/package`，`shadcn` 指 `new-york-v4/ui`，`sonner` 指 `sonner/package/dist`。

本组测试：`ui/align_b3l_test.go`（行为）、`ui/theme/align_b3l_test.go`（令牌数值，断言值即源码值）。演示页 `examples/ui-gallery/page_b3l.go`：`AlertAlign`、`MessageAlign`、`NotificationAlign`、`ResultAlign`、`SpinAlign`（均挂在同名父页下）。

## Alert

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| type | antd `type` success/info/warning/error，未给时 banner 为 warning、否则 info（antd/alert/Alert.js `type` useMemo）；EP `type` 默认 info，另有 primary（ep/es/components/alert/src/alert.d.ts） | feedback.go `Alert(c, status, title)` 必填 status；任意状态名走主题色板 | 等价写法：status 必填，`"warning"` 即 banner 默认；EP primary 用 `"primary"`（走色板） |
| title / message | antd 6 `title`，`message` 弃用（Alert.d.ts） | 构造参数 title | 已对齐 |
| description | antd、EP `description` | `Description(s)` | 已对齐 |
| showIcon 默认 | antd `showIcon` 无默认（false），banner 且未设时为 true（Alert.js `isShowIcon`）；EP `showIcon: BooleanConstructor` 默认 false | 默认 true（`Prop(..., "showIcon", true)`） | 已补齐（TestAlertShowIconDefault）：默认 false，Banner 默认 true；`ShowIcon(on)` 新增；`NoIcon()` 保留；**变更** |
| icon | antd `icon`，只在 showIcon 为真时渲染（Alert.js:227） | — | 已补齐（TestAlertCustomIcon）：`Icon(ic)`。与 antd 的差异：`Icon` 同时打开 ShowIcon（换图标的调用方必然想看到它；要隐藏可在其后接 `ShowIcon(false)`） |
| variant | antd 6.4 `variant: outlined \| filled`，默认 outlined；filled 边框透明（Alert.js useVariant、style `&-filled`） | — | 已补齐（TestAlertVariantFilled）：`Variant(v)`，Defaults 键 `"variant"`；Element / shadcn 本身无状态色边框，filled 只去掉边框 |
| closable | antd `closable: boolean \| {closeIcon, onClose, afterClose, disabled}`；`closeText/closeIcon/onClose/afterClose` 弃用（Alert.d.ts）；EP `closable` 默认 **true**、`close-text` | `Closable()`、`Closed()` | 已补齐（TestAlertCloseTextAndIcon、TestAlertCloseDisabled）：`CloseIcon(ic)`、`CloseText(s)`、`CloseDisabled(on)`；`ConfigProvider.ComponentDisabled` 也禁用；onClose = `Closed()` 查询（Go 无回调）；afterClose 不需要（无退出动画）。EP 默认 closable=true **不改**（行为以 antd 为准，见待定 1） |
| action | antd `action` | `Action(fn)` | 已对齐 |
| banner | antd banner：`border: 0 !important; border-radius: 0`（style genBaseStyle `&-banner`） | 有 | 已对齐（Ant 与 Element 一致去边框、圆角） |
| role | antd 根节点 `role="alert"` 固定（Alert.js）；shadcn `role="alert"` | 错误/警告 `alert`，其余 `status` | 已对齐（TestAlertA11yRoles；按 AUTHORING §17 区分礼貌/打断，比 antd 更细） |
| ErrorBoundary | antd `Alert.ErrorBoundary` | — | 不需要（Go 无 React 错误边界） |
| EP effect | `effect: light \| dark` | `Effect(AlertEffect…)` | 已对齐 |
| EP center | `center` | `Center()` | 已对齐 |
| 尺寸令牌（antd） | `defaultPadding` 8/12、`withDescriptionPadding` 20/24、`withDescriptionIconSize` fontSizeHeading3 24、`containerBorderRadius` borderRadiusLG、icon marginXS 8 / 带描述 marginSM 12、带描述时标题 fontSizeLG、`marginBottom: marginXS` 8、**无 fontWeight**（整份样式无 fontWeight）、标题色 colorTextHeading、描述 colorText（antd/alert/style/index.js:97-104） | 标题色 colorText、带描述标题粗体（FontWeightStrong 600）、标题下 4 | 已补齐（TestAlignB3LAlertTokens）：`TitleColor` → TextHeading（Ant 亮色同值 #000000e0）；带描述标题 600 → 常规、标题下 4 → 8；**变更**（Ant 外观）。Element 保留 4（EP gap 4），shadcn 2 |
| 关闭按钮（antd） | fontSizeIcon 12、colorIcon → hover colorIconHover、marginInlineStart marginXS 8、disabled colorTextDisabled（genActionStyle） | 16 框 12 图标，hover 用 IconButton 默认 | 已补齐：令牌 `CloseColor/CloseHoverColor/CloseIconSize/CloseFontSize`；按钮左距 8 |
| 尺寸令牌（EP） | `$alert`：padding 8px 16px、title 14px / 带描述 16px、description 14px、close 16px / close-customed 14px、icon 16px / 28px（theme-chalk/src/common/var.scss）；alert.scss：`__content gap 4px`、light 下标题与描述都是类型色、close 为 `--el-text-color-placeholder`、`align-items: center`、标题非粗体 | 标题 14 粗体 700、描述 12 次要色、带描述时无间距、小图标 14 | 已补齐（TestAlignB3LAlertTokens）：标题 16 常规、描述 14 类型色、间距 4、小图标 14 → 16、大图标垂直居中（`WithDescriptionIconCenter`）、关闭 16 placeholder 色；**变更**（Element 外观） |
| shadcn | `rounded-lg border px-4 py-3 text-sm grid … gap-x-3 gap-y-0.5`、`[&>svg]:size-4 translate-y-0.5`、default `bg-card`、destructive `text-destructive`（边框不变色）、描述 `text-muted-foreground`，destructive 描述 `text-destructive/90`；标题 `font-medium line-clamp-1`（shadcn/alert.tsx） | bg = BgContainer，destructive 边框 destructive/50，标题与描述间距 4 | 已补齐（TestAlignB3LAlertTokens）：背景 `--card`、destructive 边框 = 普通边框、`gap-y-0.5` = 2；**变更**（shadcn 外观）。`line-clamp-1` 暂不做（缺 Text 行数截断与省略号的组合用法，见缺口） |
| 明暗 | 令牌 | 全部来自 AlertToken | 已对齐（四图出图检查） |

## Message（含 shadcn Sonner）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| type | antd `info/success/error/warning/loading`（antd/message/interface.d.ts） | status 参数 | 已对齐 |
| duration | antd 3 s（useMessage.js DEFAULT_DURATION）；EP 3000 ms，0 不关闭（message.mjs） | `MessageDuration` 3 s、`Duration(d)`、≤0 常驻 | 已对齐（TestMessageOutOfRangeDuration） |
| pauseOnHover | antd 6.7 `pauseOnHover = true`（useMessage.js:56；ArgsProps 也有） | 无，悬停照样计时 | 已补齐（TestMessagePauseOnHover、TestMessagePauseOnHoverOff）：默认暂停、`PauseOnHover(on)`、Defaults 键 `"pauseOnHover"`；Sonner 外观下悬停整个堆叠时全部暂停（sonner 行为）；**变更** |
| top / offset | antd `top` 默认 8（DEFAULT_OFFSET）；EP `offset` 16、相邻间距 16（instance.mjs getOffsetOrSpace） | `Offset(px)`，令牌 Top 8（Element 也是 8），相邻间距 8 | 已补齐（TestAlignB3LMessageTokens）：Element `Top` 8 → 16；新令牌 `Gap` 16（antd 列表 gap = token.margin 16、EP 16）；**变更**（相邻消息间距 8 → 16，三套主题非 Sonner 都受影响） |
| maxCount | antd `maxCount`；EP 先 grouping 再判断 max（ep/es/components/message/src/method.mjs:90-98） | Defaults 键 `"maxCount"`，构造时直接截断切片 | 已补齐（TestMessageGroupingMaxCount、TestMessageMaxCount）：改在首次渲染时执行，先合并 Grouping / Key，超出的最旧消息淡出并触发 `OnClose`（与 Notification 一致）；`messageMaxCount`（defaults_more.go，共享）不再被 Message 调用 |
| key 更新 | antd `key` | `Key(k)` | 已对齐 |
| icon | antd `icon` | — | 已补齐：`Icon(ic)`（演示 MessageAlign） |
| onClose / then | antd `onClose`、返回 thenable | — | 已补齐（TestMessageOnCloseOnClick）：`OnClose(fn)`（关闭后执行一次）；`then` 等价写法：`OnClose` |
| onClick | antd ArgsProps `onClick` | — | 已补齐（TestMessageOnCloseOnClick）：`OnClick(fn)` |
| destroy | antd `destroy(key?)` | `MessageDestroy(c, key)` | 已对齐 |
| stack | antd message `stack` 默认 false（DEFAULT_STACK_CONFIG） | 不堆叠 | 已对齐 |
| loading 图标色 | antd `icon-loading` 用 colorInfo（notification/style/notification.js genListItemSharedStyle） | Primary | 已对齐（Ant colorInfo = colorPrimary） |
| contentPadding | antd (controlHeightLG − fontSize×lineHeight)/2 = 9、paddingSM 12；contentBg colorBgElevated（antd/message/style/index.js prepareComponentToken） | 令牌 9/12 | 已对齐（TestAlignB3LMessageTokens） |
| zIndexPopup / getContainer / prefixCls / rtl / transitionName | antd | 全局层；RTL 随作用域 | 不需要（全局层在最上；方向随 ConfigProvider） |
| EP plain / show-close / placement | `plain`、`showClose`、`placement` 六向（2.11+） | `Plain()`、`ShowClose()`、`Placement(p)` | 已对齐 |
| EP grouping / repeatNum | `grouping`、`repeatNum` 1（message.mjs）；按 placement 分实例列表（method.mjs:89）；徽标 `top -8px right -8px`（theme-chalk/src/message.scss） | — | 已补齐（TestMessageGrouping、TestMessageGroupingPlacement）：`Grouping()`，同内容同位置合并、计数徽标画在末端角；合并在首次渲染时做，链在 Grouping 之后的 `Placement` 等选项也生效 |
| EP center | `center` 文字居中 | 文字本就居中（宽度随内容） | 不需要 |
| EP appendTo | — | 全局层 | 不需要 |
| EP padding / bg | `$message` padding 11px 15px、bg info light-9（common/var.scss） | 令牌 11/15、状态色浅底 | 已对齐 |
| Sonner 卡片 | sonner `[data-styled=true]`：padding 16px、`border 1px var(--normal-border)`、`border-radius var(--border-radius)`（shadcn 设为 `var(--radius)`）、`box-shadow 0 4px 12px rgba(0,0,0,.1)`、`width var(--width)` = TOAST_WIDTH 356、`font-size 13px`、`gap 6px`（sonner/index.mjs:404、:419）；shadcn sonner.tsx `--normal-bg: var(--popover)` | 356、p-4、ShadowSecondary、14px | 已补齐（TestAlignB3LMessageTokens）：阴影改 sonner 原值、文字 13px（新令牌 `ContentFontSize`）；**变更**（shadcn 外观） |
| Sonner 位置 | sonner `position = 'bottom-right'`（index.mjs:917） | shadcn 主题默认右下 | 已对齐（已定事项 4：其余主题不改） |
| Sonner 可见数 / 间距 / 时长 | VISIBLE_TOASTS_AMOUNT 3、GAP 14、VIEWPORT_OFFSET 24、TOAST_LIFETIME 4000（index.mjs:411-421） | 3 / 14 / 24；Message 时长 3 s | 已对齐（个数、间距、边距）；时长保持 antd 3 s（行为以 antd 为准，见待定 2） |
| Sonner 图标 | shadcn sonner.tsx lucide CircleCheck/Info/TriangleAlert/OctagonX/Loader2 `size-4` | `statusIcon` 16（shadcn 主题图标映射为 lucide） | 已对齐 |
| Sonner 滑动关闭 / 快捷键 hotkey alt+T / richColors / invert / closeButton 位置 | sonner | — | 暂不做（滑动关闭缺触摸手势的通用横扫识别；hotkey 需全局快捷键注册；richColors 等价写法：`Plain()` 之外不提供） |
| a11y | antd 消息无 role；AUTHORING §17 status/alert | `RoleStatus` / `RoleAlert` | 已对齐（TestMessageA11yLive） |

## Notification（含 shadcn Sonner）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| placement | antd 六向，默认 topRight（useNotification.js DEFAULT_PLACEMENT）；EP `position` top-right | `Placement(NotifyPlacement)`，零值右上 | 已对齐 |
| duration | antd 4.5 s，`false`/≤0 不关闭；EP 4500 | 4.5 s，`Duration(≤0)` 常驻 | 已对齐（TestNotificationOutOfRangeDuration） |
| showProgress | antd `showProgress`；progressBg `linear-gradient(90deg, colorPrimaryBorderHover, colorPrimary)`，高 2，左右内缩 borderRadiusLG，底槽 rgba(0,0,0,.04)（antd/notification/style/index.js、notification.js）；EP `progress`：ElProgress strokeWidth 3、按类型 status 着色（notification.vue） | Primary 纯色 2px 贴边 | 已补齐（TestAlignB3LNotificationTokens）：令牌 `ProgressColorStart/ProgressColor/ProgressRail/ProgressInset/ProgressHeight/ProgressStatusColor`；**变更**（Ant 渐变 + 内缩 + 底槽；Element 3px 按类型色） |
| pauseOnHover | antd、EP 默认 true | 有（`NoPause()`，Defaults 键） | 已对齐 |
| stack | antd `stack {threshold}` 默认开启（DEFAULT_STACK_CONFIG `{offset: 8}`），堆叠后按 0.06 缩放 | `NotifyStackThreshold` 3、悬停展开 | 已对齐（缩放细节与 antd 略异，未改） |
| maxCount | antd `maxCount` | — | 已补齐（TestNotificationMaxCount）：Defaults 键 `"maxCount"`，超出时关闭最旧 |
| closable / closeIcon | antd `closable`（可 false）、`closeIcon`；EP `showClose` 默认 true、`closeIcon` | 关闭按钮总在 | 已补齐（TestNotificationClosable）：`Closable(on)`、`CloseIcon(ic)` |
| actions / btn | antd 6 `actions`（`btn` 弃用），float right，marginTop marginSM 12；rc-notification 中 actionsNode 与图标/文字/关闭按钮同级（@rc-component/notification@2.1.1 es/Notification.js:116-164） | `Actions(fn)` 在文字列内，上距 16 | 已补齐：令牌 `ActionsMarginTop` 12；操作区移到卡片内容区第二行、右对齐到内容区右缘（不再让出关闭按钮宽度）；**变更**（Ant 16 → 12，右缘右移约 24px）。Sonner 外观仍在文字列内（sonner 的 action 按钮在内容旁） |
| title / message | antd 6 `title`（`message` 弃用） | 构造参数 title | 已对齐 |
| description | antd、EP | `Description(s)` | 已对齐 |
| icon | antd、EP `icon` | — | 已补齐：`Icon(ic)` |
| role | antd `role: alert \| status` | 按状态 | 已补齐（TestNotificationRoleOnClose）：`Role(r)` |
| onClick / onClose | antd | `OnClick` | 已补齐（TestNotificationRoleOnClose）：`OnClose(fn)` |
| top / bottom | antd `top/bottom` 24（marginEdge marginLG） | 固定 24（Element 16） | 已对齐：改为令牌 `MarginEdge`（值不变）；单独的 top/bottom 偏移 暂不做（缺按调用方配置的全局层偏移，可用 `ConfigProvider.Defaults` 扩展，未加） |
| 间距令牌（antd） | width 384、padding 20/24、section gap marginXS 8、wrapper gap marginSM 12、list gap margin 16、icon fontSizeLG×lineHeightLG = 24、close 22 框 colorIcon（notification/style/index.js prepareNotificationToken） | 写死在代码里 | 已补齐（TestAlignB3LNotificationTokens）：`TitleMarginBottom/IconGap/Gap/MarginEdge/CloseColor/CloseGap` 令牌化；图标到文字 4+12=16 → 12；**变更**（Ant 图标间距 16 → 12） |
| 间距令牌（EP） | width 330、padding 14/26/14/13、group margin-left 13、content margin-top 6、title 16 bold、icon 24、close `top 18px right 15px` 色 `--el-text-color-secondary`、`.right 16px`、GAP_SIZE 16（$notification、notification.scss、notify.mjs） | 宽 330，图标到文字 1+12=13，标题下 6 | 已对齐：改为令牌（数值不变）；关闭按钮色 → TextSecondary（原 TextTertiary） |
| Sonner | 卡片同 Message；title `font-weight 500; line-height 1.5`、description `line-height 1.4`、`[data-content] gap 2px`、13px（sonner/index.mjs:404） | 14px 标题/描述、PopupBorder、ShadowSecondary | 已补齐（TestAlignB3LNotificationTokens）：13px、行高 19.5 / 18.2、阴影改 sonner 原值；**变更**（shadcn 外观）；Sonner 时长 4000 ms 未改（见待定 2） |
| EP offset / zIndex / appendTo / dangerouslyUseHTMLString | — | — | 不需要（全局层；无 HTML） |
| a11y | 实时区域，带 OnClick 可聚焦 | 同 | 已对齐 |

## Result

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| status | antd success/error/info/warning/404/403/500（antd/result/index.d.ts）；EP `icon` primary/success/warning/info/error | data_result.go `Result(c, status, title)` | 已对齐 |
| title / subTitle / icon / extra / children | antd；EP 插槽 | `SubTitle`、`Icon(fn)`、`Extra(fn)`、`Body(fn)` | 已对齐 |
| 令牌（antd） | padding paddingLG×2 48 / paddingXL 32、iconFontSize fontSizeHeading3×3 72、icon marginBottom paddingLG 24、titleFontSize 24、lineHeightHeading3、title `marginBlock marginXS 8`、subtitle colorTextDescription、extraMargin `24px 0 0 0`、body marginTop 24 padding 24/40 colorFillAlter、无圆角（antd/result/style/index.js）；异常图 250×295 | 副标题上距 4、内容区圆角 RadiusSM、插画 252×210 | 已补齐（TestAlignB3LResultTokens）：副标题上距 4 → 8（marginBlock 8；图标下 24 与标题上 8 折叠为 24，操作区 24 与标题下 8 折叠为 24，故其余不变）；内容区圆角 4 → 0；**变更**。异常插画为自绘原创，尺寸不跟 antd 250×295（不复制第三方插画，AUTHORING §13） |
| 令牌（EP） | `$result`：padding 40/30、icon 64、title 20 margin-top 20、subtitle margin-top 10、extra margin-top 30；title/subtitle `line-height: 1.3`、subtitle `--el-text-color-regular` 14px（result.scss） | 行高用主题默认（28 / 22） | 已补齐（TestAlignB3LResultTokens）：标题行高 28 → 26、副标题行高 22 → 18.2（新令牌 `SubtitleLineHeight`）；**变更**（Element） |
| a11y | 标题 heading | `RoleHeading` level 2、副标题 paragraph、图标装饰 | 已对齐 |
| 明暗 | 令牌 | ResultToken | 已对齐 |

## Spin / Loading（含 shadcn Spinner、EP v-loading）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| spinning | antd 默认 true | `Spinning(on)` 默认 true | 已对齐 |
| size | antd small/default(medium)/large（antd/spin/index.d.ts） | `Size(sz)`、`IndicatorSize(px)` | 已对齐 |
| 尺寸令牌 | antd `dotSize` controlHeightLG/2 = 20、`dotSizeSM` ×0.35 = 14、`dotSizeLG` controlHeight 32（antd/spin/style/index.js prepareComponentToken）；EP `--el-loading-spinner-size` 42、fullscreen 50（$loading）；shadcn `size-4` = 16（shadcn/spinner.tsx，无尺寸档） | 写死 14/20/32、Element 28/42/50（`navEl` 分支） | 已补齐（TestAlignB3LSpinTokens、TestSpinTokenSizes）：新令牌 `SpinToken.DotSize/DotSizeSM/DotSizeLG/FullscreenDotSize`，Element 小号 28 沿用；shadcn 16 / 小 12 / 大 24（size-4 / size-3 / size-6，原 20 / 14 / 32）；**变更**（shadcn 尺寸）；去掉了组件里的 `navEl` 分支 |
| 指示器外观 | antd 四点：item (size − marginXXS/2)/2，scale .75，透明度 .3→1，1.2 s 旋转（genIndicatorStyle）；EP 圆弧 r=20/50、stroke-width 2（loading.scss）；shadcn lucide Loader2 `animate-spin` | Ant 四点（点径 9/20 size）、Element/shadcn 自画圆弧（按 `th.Flavor` 分支） | 已补齐：令牌 `Indicator`（SpinDots / SpinCircular / SpinIcon）；Ant 点径按 antd 公式（20px 下圆半径 4.5 → 3.4，**变更**，细微）；Element 线宽按 2/50；shadcn 改为旋转主题的 Loading 图标（lucide LoaderCircle） |
| 颜色 | antd section colorPrimary；fullscreen colorWhite；shadcn 继承文字色（text-current） | Primary | 已补齐：令牌 `Color`（shadcn = foreground），**变更**（shadcn 指示器 primary → foreground，两者在 neutral 主题接近） |
| description / tip | antd 6 `description`（`tip` 弃用），fontSize，gap paddingSM 12（genSpinStyle）；EP text margin 3px 0、14px | `Tip`，间距 8 | 已补齐（TestSpinDescription）：`Description(s)`，`Tip` 标 Deprecated；Defaults 键 `"description"`（`"tip"` 保留）；间距令牌 `DescriptionGap`（Ant 8 → 12、Element 8 → 3、shadcn 8）；**变更** |
| delay | antd `delay`：debounce 只作用于进入，停止时立即隐藏（antd/spin/index.js shouldDelay） | `Delay(d)` | 已补齐（TestSpinDelayHidesAtOnce）：改用通用计时辅助 `throttledOn`（feedback.go，Skeleton 共用） |
| indicator | antd `indicator`；EP `spinner` | — | 已补齐：`Spin.Indicator(ic)`、`Loading.Spinner(ic)`（旋转图标） |
| percent | antd `number \| 'auto'`；auto 每 200 ms 前进剩余的 5%（≤30）/3%（≤70）/1%（≤96）（antd/spin/usePercent.js）；进度环 stroke viewSize/5、底色 colorFillSecondary、0 时仍转圈（Indicator/Progress.js、Looper.js） | `Percent(p)`；p=0 时画空环 | 已补齐（TestSpinPercentAuto、TestSpinA11y）：`PercentAuto()`（减少动效时照常前进，只在截图模式停在 0）；0 时显示转圈（**变更**）；环宽 size/8 → size/5（令牌 `ProgressStrokeRatio`，**变更**） |
| fullscreen | antd `fullscreen`：colorBgMask 底、白色指示器（genSpinStyle `&-fullscreen`）；EP 全屏 `--el-mask-color` 底、主色、50px | `Loading(c, on).Fullscreen()` | 已对齐：改为令牌 `FullscreenMaskBg/FullscreenColor`（值不变；Element 指示器 50 px） |
| 嵌套 Wrap | antd 容器 opacity .5，`::after` colorBgContainer opacity .4（genSpinStyle `&-spinning`） | 内容 .5，遮罩 BgContainer×.5 | 已补齐：遮罩 .5 → .4（令牌 `MaskBg`，**变更**，细微） |
| EP Loading 遮罩 | `--el-mask-color` rgba(255,255,255,.9)，暗色 rgba(0,0,0,.8)（common/var.scss、dark/var.scss）；内容不变淡 | BgContainer×.9 | 已补齐（TestAlignB3LSpinTokens）：Element 暗色遮罩 = rgba(0,0,0,.8)（原 #141414×.9），**变更**；Ant 区域遮罩 BgContainer×.6 → 令牌 `MaskBg`（×.4）+ 内容 .5（与 antd 嵌套 Spin 一致），**变更** |
| EP text / background / lock / fullscreen | `element-loading-text`、`-background`、`lock`、`fullscreen` | `Tip`、`Background`、`Fullscreen` | 已补齐（TestLoadingTextAndSpinner）：`Text(s)`（`Tip` 弃用）；`lock` 不需要（无页面滚动锁，全屏遮罩已挡住指针） |
| wrapperClassName / custom-class | — | 令牌 | 不需要 |
| a11y | antd `aria-live polite` `aria-busy`；Progress `role=progressbar`；shadcn `role="status" aria-label="Loading"` | status + Busy，名称 Locale.A11y.Loading；Percent → progressbar | 已对齐（TestSpinA11y） |

## Skeleton throttle（已定事项 8）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| throttle | EP `throttle: number \| {leading, trailing, initVal}`；onMounted 总按 leading 派发（ep/es/components/skeleton/src/skeleton.mjs:35、ep/es/hooks/use-throttle-render/index.mjs） | —（2-display.md 暂不做） | 已补齐（TestSkeletonThrottle、TestSkeletonThrottleInitial）：`Throttle(leading, trailing)`、`ThrottleInitial(on)`，与 Spin delay 共用 `throttledOn`（首帧按 leading） |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 单行截断 + 省略号 | shadcn AlertTitle `line-clamp-1` | Label 的 MaxLines(1) 与 NoWrap 省略的统一入口（现有能力分散） | 暂不做 |
| 横扫手势 | Sonner 滑动关闭 | Driver/输入层的横扫（swipe）识别与阈值（sonner SWIPE_THRESHOLD 45） | 暂不做 |
| 全局快捷键 | Sonner hotkey（alt+T 聚焦通知区） | 不依赖焦点的应用级快捷键注册 | 暂不做 |

## 审阅（group-L-review.md）处理

一般 1–6 与建议 1–5 全部按建议修正（一般-5 由主线程处理，见上）。新方法的签名复核：`AlertW`、`MessageRef`、`NotificationRef`、`SpinW`、`ResultW` 在 `api/stable/ui.txt` 里，新方法随类型进入稳定层；全部为不加参数的链式方法，参数类型只用标准库、`*icon.Icon`（stable 包）、`time.Duration`、`bool`、`string`、`func()`，无实验层类型。`LoadingW`、`SkeletonW` 不在白名单（实验层）。

## 待用户决定

1. EP `el-alert` 默认 `closable: true`；Tango 三套主题保持 antd 默认（不可关闭）。如需 Element 主题默认可关闭，可加令牌或由应用设 `ConfigProvider.Defaults("Alert", {"closable": true})`。
2. sonner 默认时长 4000 ms（Message 的 antd 默认是 3 s、Notification 4.5 s）；shadcn 主题未改时长，等价写法：`Defaults("Message"/"Notification", {"duration": 4})`。

## CHANGELOG 草稿

### 新增

- `ui`：Alert `ShowIcon(on)`、`Icon(ic)`、`Variant("outlined"|"filled")`（Defaults 键 `variant`）、`CloseIcon(ic)`、`CloseText(s)`、`CloseDisabled(on)`。
- `ui`：Message `PauseOnHover(on)`（Defaults 键 `pauseOnHover`，默认开）、`Icon(ic)`、`OnClose(fn)`、`OnClick(fn)`、`Grouping()`（EP grouping，重复计数徽标）。
- `ui`：Notification `Closable(on)`、`CloseIcon(ic)`、`Icon(ic)`、`Role(r)`、`OnClose(fn)`；Defaults 键 `maxCount`。
- `ui`：Spin `Description(s)`（`Tip` 弃用；Defaults 键 `description`）、`Indicator(ic)`、`PercentAuto()`；Loading `Text(s)`（`Tip` 弃用）、`Spinner(ic)`。
- `ui`：Skeleton `Throttle(leading, trailing)`、`ThrottleInitial(on)`（EP throttle）。
- `ui/theme`：第 2 批复审遗留的 `SkeletonToken.Pulse`、`EmptyToken.Picture`：shadcn Skeleton pulse 与 Element Empty 插画由令牌决定，不再在组件中读取 Flavor（外观保持不变）。
- `ui/theme`：`SpinToken`（`Indicator`、`DotSize/DotSizeSM/DotSizeLG/FullscreenDotSize`、`Color`、`DescriptionGap/DescriptionFontSize`、`ContainerOpacity`、`MaskBg`、`FullscreenMaskBg/FullscreenColor`、`ProgressStrokeRatio`）与 `SpinIndicator`（`SpinDots/SpinCircular/SpinIcon`）；`AlertToken.WithDescriptionIconCenter/CloseColor/CloseHoverColor/CloseIconSize/CloseFontSize`；`MessageToken.Gap/ContentFontSize`；`NotificationToken.DescriptionFontSize/DescriptionLineHeight/TitleMarginBottom/IconGap/ActionsMarginTop/MarginEdge/Gap/CloseColor/CloseGap/ProgressColorStart/ProgressColor/ProgressRail/ProgressInset/ProgressHeight/ProgressStatusColor`；`ResultToken.SubtitleLineHeight`。

### 变更

- Alert：图标默认不显示（原默认显示）；Banner 仍默认显示（antd showIcon 默认 false、banner 时 true；EP show-icon 默认 false）。迁移：需要图标的调用加 `ShowIcon(true)`，或 `ConfigProvider.Defaults("Alert", {"showIcon": true})`。
- Alert（Element）：标题 14 粗体 → 16 常规（单独标题 14 不变）、描述 12 次要色 → 14 类型色、标题与描述间距 0 → 4、带描述时大图标垂直居中、关闭按钮 12 → 16 placeholder 色（theme-chalk alert.scss / $alert）。
- Alert（shadcn）：背景 background → `--card`；destructive 边框 destructive/50 → 普通边框；标题与描述间距 4 → 2（alert.tsx）。
- Alert：关闭按钮左边距 0 → 8，颜色走令牌（Ant colorIcon → hover colorText）。
- Alert（Ant）：带描述时标题 粗体 600 → 常规、标题到描述 4 → 8（antd `&-with-description` title marginBottom marginXS，样式无 fontWeight）。
- Alert（Element）：小图标 14 → 16（$alert icon-size）；Banner 一律圆角 0（原无边框时保留圆角）。
- Message：相邻消息间距 8 → 16（antd 列表 gap = margin 16、EP 16）；悬停暂停计时（antd 6.7 pauseOnHover 默认 true）；Element 顶部距离 8 → 16（ElMessage offset 16）。
- Message / Notification（shadcn Sonner）：阴影 → `0 4px 12px rgba(0,0,0,.1)`，文字 14 → 13px，通知标题行高 → 1.5（19.5）、描述 1.4（18.2）（sonner 2.0.7 styles）。
- Notification（Ant）：图标到文字 16 → 12（marginSM）、操作区上边距 16 → 12、操作区移出文字列并右对齐到卡片内容区右缘（rc-notification actionsNode 同级）、文字到关闭按钮 12 → 2（标题 paddingInlineEnd paddingLG 24 − 关闭按钮 22，Element 12 → 3）；进度条 Primary 纯色贴边 → colorPrimaryBorderHover→colorPrimary 渐变、左右内缩 8、rgba(0,0,0,.04) 底槽。Element：进度条 2 → 3 px、按类型着色、`--el-border-color-lighter` 底槽；关闭按钮色 TextTertiary → TextSecondary。
- Result（Ant）：副标题上边距 4 → 8（marginBlock 8）、内容区圆角 4 → 0；Element：标题行高 28 → 26、副标题行高 22 → 18.2（line-height 1.3）。
- Spin：尺寸、颜色、指示器种类改由 `SpinToken` 给出；shadcn 20 / 14 / 32 → 16 / 12 / 24（spinner size-4，小大取 size-3 / size-6）；小号的描述字号 FontSizeSM → FontSize（antd genSizeStyle 不改描述字号）、颜色 primary → foreground、指示器改为旋转 lucide Loader 图标；Ant 四点点径按 antd 公式（20px 时半径 4.5 → 3.4）；描述间距 Ant 8 → 12、Element 8 → 3；Percent 环宽 size/8 → size/5，Percent(0) 显示转圈而非空环（antd Looper）；`Delay` 只延迟显示，停止时立即隐藏。
- Spin.Wrap / Loading：Ant 与 shadcn 遮罩 BgContainer×.5（Loading ×.6）→ ×.4 且内容 .5 透明（antd 嵌套 Spin；Loading 原内容不变淡）；Element 遮罩 → rgba(255,255,255,.9)、暗色 rgba(0,0,0,.8)（$mask-color），Spin.Wrap 内容不再变淡（原 .5）。
- Message：maxCount 改在首次渲染时执行（先 Grouping / Key），被挤掉的消息淡出并触发 OnClose（原直接删除）；关闭动画关闭时（截图 / Driver 默认）消息到时即移除。
- 测试期望：无已有用例期望值修改。

## 共享文件改动请求

1. `ui/testdata/basic_antd.png`、`basic_element.png`、`basic_dark.png`（黄金图）：golden 页的 `Alert(c, "info", …)` 不再显示图标（规范修正：showIcon 默认 false），`TestGolden` 三张图约 1% 像素不同而失败。若主线程同意该变更，请 `go test ./ui -run 'TestGolden$' -update` 重录；若希望 golden 页保持原画面，改 golden_test.go:70 为 `.ShowIcon(true)`。
2. `ui/AUTHORING.md`：§6 第 96 行「加载转圈用 `drawSpinner`」仍有效（现按 SpinToken 绘制）；§14 总表 Alert 行补 `ShowIcon/Icon/Variant/CloseIcon/CloseText/CloseDisabled`，Message 行补 `PauseOnHover/Icon/OnClose/OnClick/Grouping`，Notification 行补 `Closable/CloseIcon/Icon/Role/OnClose`，Spin 行注明「shadcn Spinner = Spin（SpinToken.Indicator = SpinIcon）」，Loading 行注明「EP v-loading / ElLoading.service = Loading（Text、Spinner、Background、Fullscreen）」；§13 去重补「Spinner = `Spin`」。
3. Gallery 混合页（主线程独占）里的弃用调用可迁移（不迁移也能编译）：`page_nav_feedback.go:203,209` `Spin.Tip` → `Description`、`:216,219,227` `Loading.Tip` → `Text`；`page_nav_global.go:31` `Spin.Tip` → `Description`。另外 Alert 图标默认关闭后，`page_feedback.go`、`page_p2c_display.go`、`page_nav_feedback.go` 里原本依赖默认图标的演示需要加 `.ShowIcon(true)` 才保持原截图（我未改，主线程决定）。
4. 归属外的 Alert 调用（只读文件，行为随默认变化：错误提示不再带图标）：`ui/pro3_descriptions.go:230`、`ui/shad2d_table.go:54`、`ui/form_pro.go:300`、`ui/jsonview.go:434`、`ui/pro3_login.go:311`、`ui/pro3_list.go:262`、`ui/table_pro.go:647`、`internal/devhost/host/banner.go:72`（Banner，图标仍在）、`examples/ui-window/main.go:184`。如要保持原外观请给它们加 `.ShowIcon(true)`（antd ProTable/LoginForm 的错误 Alert 本身带 `showIcon`）。
5. `ui/theme/highcontrast.go`（如需）：新令牌 `SpinToken.Color`、`AlertToken.CloseColor`、`NotificationToken.CloseColor` 未在高对比度主题单独处理。
6. `ui/shad2c_defaults.go` / `ui/defaults_more.go`（共享）：Spin `"description"`、Notification `"maxCount"` 与 Message `"maxCount"` 的 Defaults 读取我写在了自己文件里（构造函数中）；`defaults_more.go` 的 `messageMaxCount` 已无调用方（审阅一般-1：maxCount 需在 Grouping 之后执行），请主线程删除它，或保留待用。
7. Locale：本组未改 `ui/locale.go` / `ui/locale_packs.go`。
8. `ui/feedback.go` 第 18 行起本组段落已扩到约 900 行（Spin/Loading/Alert），K 组段落相应下移；`MessageDuration`、`NotifyStackThreshold` 仍是导出可变包级变量（违反 R9 但为存量，不在本批处理）。
