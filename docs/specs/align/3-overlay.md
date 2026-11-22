# 第 3 批 · 组 K 浮层与对话框 对照记录

对照日期：2026-11-15。参考源码在 `/tmp/tango-ref`（见 [README](README.md)）：antd@6.7.0（`antd/package/es`）、@rc-component/dialog@1.10.0（`rc-dialog/package`）、@rc-component/drawer@1.4.2（`rc-drawer/package`）、@rc-component/tour@2.4.0（`rc-tour/package`）、element-plus@2.14.8（`ep/package`）、shadcn-ui@e62c9435（`shadcn-ui/apps/v4/registry/new-york-v4/ui`）、@radix-ui/react-hover-card@1.1.15（`radix-hover-card/package`，radix-ui@1.4.3 的依赖版本）。下文源码路径省略这些前缀，`antd` 指 `antd/package/es`，`ep` 指 `ep/package`，`shadcn` 指 `new-york-v4/ui`。

本组测试：`ui/align_b3k_test.go`（行为）、`ui/theme/align_b3k_test.go`（令牌数值，断言值即源码值）；演示页 `examples/ui-gallery/page_b3k.go`（`ModalAlign`、`DrawerAlign`、`PopconfirmAlign`、`TourAlign`、`ConfirmAlign`）。

开工第一步（已定事项 5）：`data_popover.go` 的 `popDrawArrow` 原样移到 `ui/popup_arrow.go`，改名 `drawPopupArrow(l, panel, anchor, bg, border)`；Popover、Tour 改调它（像素不变，`TestGolden` 不变）；Popconfirm 上下方向原来自画 6px 高的箭头，现在四个方向都用 `drawPopupArrow`（见 Popconfirm 表）。

## Modal（含 shadcn Dialog）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| open / title / Ok / Cancel | antd `open`、`title`、`onOk`、`onCancel`（antd/modal/interface.d.ts） | feedback.go `Modal(c,&open,title)`、`Ok()`、`Cancelled()` | 已对齐 |
| footer | antd `footer`：节点、`null`、函数 `(originNode, {OkBtn, CancelBtn})` | `Footer(fn)`、`NoFooter()` | 等价写法：`NoFooter()` = `footer={null}`；函数形式用 `Footer(fn)` 自己放按钮 |
| width | antd 默认 520（antd/modal/Modal.js:52），可为断点对象；EP `width` 50%；shadcn `sm:max-w-lg`（dialog.tsx） | Ant 520、Element `max(320, 50%)` 写在构造函数里按 Flavor 分支 | 已补齐（TestModalWidthDefaults、TestAlignB3KModalTokens）：改为令牌 `ModalToken.Width/WidthRatio`（Ant 520、Element 320 且至少 50%、shadcn 512）；断点对象写等价写法：应用按 `ScreenBreakpoint` 算好再 `Width(px)`；**变更**：shadcn 默认宽 520 → 512 |
| 最大宽度 | antd `maxWidth: calc(100vw - 2 × margin)`（antd/modal/style/index.js:108）；shadcn `max-w-[calc(100%-2rem)]` | 无 | 已补齐（TestModalWidthDefaults）：宽度不超过窗口宽 − 32（全屏除外） |
| top / centered | antd `top: 100`（style:106）、`centered`；EP `top` 默认 15vh、`align-center`；shadcn 总是居中 | `Top(px)`、`Centered()`；默认值按 Flavor 分支 | 已补齐：令牌 `Top/TopRatio/Centered`（Ant 100、Element 15%、shadcn 居中），行为不变 |
| closable / 关闭按钮位置 | antd 关闭按钮绝对定位：`modalCloseBtnSize` = controlHeight 32，`top/insetInlineEnd` = (modalHeaderHeight 56 − 32) / 2 = 12，图标 fontSizeLG 16（style:133-148、prepareToken）；EP `headerbtn` 48×48 贴右上角 `top:0 right:0`，图标 16（ep/theme-chalk/src/dialog.scss）；shadcn `absolute top-4 right-4`、`size-4`、`opacity-70`（dialog.tsx:73） | 22px 按钮放在标题行末尾 | 已补齐（TestModalCloseButtonPlacement、TestModalCloseButtonRTL、TestAlignB3KModalTokens）：令牌 `CloseBtnSize/CloseIconSize/CloseInsetTop/CloseInsetEnd`，按钮绝对定位，标题行末尾留出 `CloseInsetEnd + CloseBtnSize − Padding`（EP 的 `.show-close` padding-right 同理，TestModalCloseReserve 用长标题验证）；RTL 在左侧；**变更**：三套主题关闭按钮位置与大小 |
| 关闭按钮悬停 | antd 悬停 `colorBgTextHover` 底、`colorIconHover`（style:162-170）；EP `headerbtn background: transparent`，图标 `--el-color-info`，悬停/聚焦变 `--el-color-primary`（dialog.scss:70-96）；shadcn `opacity-70 hover:opacity-100` | Element 有悬停底色 | 已补齐（TestModalElementCloseHover）：令牌 `CloseNoHoverBg/CloseHoverColor`（Element 无底色、info → primary；shadcn 无底色、70% → 100%）；**变更**：Element 关闭图标静态色 TextTertiary → info、悬停变主色、无底色；shadcn 悬停 100% |
| closable.closeIcon / disabled / onClose | antd `closable {closeIcon, disabled, onClose, afterClose}` | `Closable(bool)` | 暂不做（自定义关闭图标与禁用关闭；用量少） |
| mask | antd 6 `mask {enabled, blur, closable}`，`maskClosable` 弃用为 `mask.closable`（antd/_util/hooks/useMergedMask.d.ts、Modal.js:126） | `Mask(on)`、`MaskClosable(on)` | 等价写法：`Mask` = `mask.enabled`，`MaskClosable` = `mask.closable`（Go 无对象形式，不另立新名）；`blur` 暂不做（缺 paint 层 backdrop blur，同第 1 批 FooterToolbar 缺口） |
| keyboard | antd `keyboard` 默认 true；EP `close-on-press-escape` 默认 true（ep/es/components/dialog/src/dialog.mjs） | Esc 总是关闭 | 已补齐（TestModalKeyboard）：`Keyboard(on)`、Defaults 键 `keyboard` |
| loading | antd 6 `loading`：内容换成 `Skeleton active`，不显示页脚（Modal.js:137、254） | — | 已补齐（TestModalLoading）：`Loading(on)` |
| confirmLoading | antd | `ConfirmLoading(on)` | 已对齐 |
| okText / cancelText / okType | antd | `OkText`、`CancelText`、`Danger()` | 已对齐；`okType` 其它取值用 `Footer` 等价 |
| okButtonProps / cancelButtonProps | antd | — | 等价写法：`Footer(fn)` 里自己放 `Button` |
| afterOpenChange / afterClose | antd `afterOpenChange(open)`、`afterClose` | — | 已补齐（TestModalAfterOpenChanged）：`AfterOpenChanged()` 在打开或关闭动画结束的那一帧为真，读 `*open` 得到方向；打开动画未结束就关闭时，关闭结束也报告一次（antd afterClose 每次关闭都触发） |
| focusable | antd 6 `focusable {trap, focusTriggerAfterClose}`（antd/drawer/useFocusable.d.ts）；`focusTriggerAfterClose` 弃用 | 有遮罩时 `TrapFocus`，关闭后焦点回到触发元素 | 已对齐（trap 与归还焦点为默认）；`focusTriggerAfterClose: false` 暂不做（需要 `TrapFocus` 选项，focus.go 共享） |
| destroyOnHidden / forceRender / getContainer / zIndex / modalRender | antd | 立即模式：关闭后不构建内容；浮层按创建顺序叠放 | 不需要 |
| draggable / fullscreen | EP `draggable`、`fullscreen`；antd 无 | `Draggable()`、`Fullscreen(&b)`、`FullscreenToggle()` | 已对齐 |
| EP overflow | EP `overflow`：拖动可超出视口 | 拖动夹在窗口内 | 暂不做（低频） |
| EP modal-penetrable | EP 遮罩可穿透 | `Mask(false)` 仍挡住页面 | 暂不做（需要浮层不挡指针的模态语义，待用户决定） |
| EP before-close | EP | `Cancelled()` | 等价写法：`Cancelled()` 为真时应用把 `open` 设回 true 即阻止关闭 |
| EP center / open-delay / close-delay | EP | — | 暂不做（低频） |
| EP header-aria-level | EP | 面板 `RoleDialog`、名称取标题 | 不需要 |
| 令牌：标题、内边距 | antd `titleFontSize` fontSizeHeading5 16、`titleLineHeight` 1.5、`contentPadding` paddingMD 20 / paddingContentHorizontalLG 24、`headerMarginBottom` marginXS 8、`footerMarginTop` marginSM 12、`titleColor` colorTextHeading（style:291-312）；EP padding-primary 16、标题 18/24（var.scss $dialog） | `ModalToken` | 已对齐（TestAlignB3KModalTokens） |
| shadcn 标题行高 | `DialogTitle text-lg leading-none font-semibold`（dialog.tsx:128） | 主题行高（28） | 已补齐：shadcn `TitleLineHeight` 18；**变更** |
| shadcn 遮罩 | `DialogOverlay bg-black/50`（dialog.tsx） | 全局 `BgMask` 0.8（v3 写法） | 已补齐：shadcn `ModalToken.MaskBg` = 黑 50%；**变更**；全局 `BgMask` 见共享文件改动请求 |
| EP 遮罩颜色 | Element Plus 2.14.8 `theme-chalk/el-overlay.css` 使用 `--el-overlay-color-lighter`，`theme-chalk/base.css` 定义为 `#00000080`（2026-11-22 核对） | Element 亮色黑色 alpha 128，暗色黑色 alpha 204 | 保留默认值。用 `theme.WithComponents(theme.DialogMask(theme.Hex("#00000080")))` 给 `theme.ElementPlus` 配置，或传给 `ConfigProvider(c).Components(...)` 以局部覆盖 Modal、Confirm 和 Drawer。 |
| shadcn showCloseButton / 页脚 / 描述 | `showCloseButton`；页脚 `flex-col-reverse sm:flex-row sm:justify-end gap-2`；`DialogDescription text-sm text-muted-foreground` | `Closable`；页脚右对齐 gap 8；无描述槽 | 已对齐；描述写等价写法：Body 里 `Text(c, s).Color(th.TextTertiary)`；窄屏纵向反排暂不做（缺容器宽度断点，同第 2 批缺口） |
| 明暗主题 | 令牌 | 全部来自 `ModalToken` | 已对齐（三套 × 明暗出图） |
| a11y | `role=dialog`、`aria-modal` | `RoleDialog` + `Modal`、`Dismiss`、关闭按钮名取 `Locale().Close` | 已对齐（TestAlignB3KA11yDialogs） |

## Confirm / MessageBox / Prompt（含 shadcn AlertDialog、EP MessageBox）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 入口 | antd `Modal.confirm/info/success/warning/error`；EP `ElMessageBox.alert/confirm/prompt`；shadcn AlertDialog | more_confirm.go `Confirm`、`MessageBox`、`Prompt` | 已对齐（命令式，不新建 AlertDialog 入口） |
| type / icon | antd `type`、`icon`（默认 ExclamationCircleFilled 等，antd/modal/ConfirmDialog.js:54-70）；EP `type` | `Status`、`Icon` | 已对齐 |
| okCancel | antd `okCancel`（confirm 默认 true） | `Confirm` 有取消、`MessageBox` 无 | 等价写法：选 `Confirm` 或 `MessageBox` |
| width | antd 416（ConfirmDialog.js:159）；EP `--el-messagebox-width` 420（var.scss $messagebox）；shadcn `sm:max-w-lg` | 按 Flavor 分支 | 已补齐：令牌 `ModalToken.ConfirmWidth`（416 / 420 / 512），并夹到窗口宽 − 32 |
| 按钮上边距 | antd `confirmBtnsMarginTop` = marginSM 12（antd/modal/style/index.js:310，confirm.js `-btns`） | Ant 24（v4 值） | 已补齐（TestConfirmAntButtonsMargin）：令牌 `ConfirmBtnsMarginTop` 12；**变更**：Ant 确认框按钮与正文间距 24 → 12 |
| autoFocusButton | antd 默认 `'ok'`，可 `'cancel'` / `null`（ConfirmDialog.js:73-76） | 总是聚焦确定 | 已补齐（TestConfirmAutoFocusAndKeyboard）：`ConfirmOptions.AutoFocusButton`（""/"ok"、"cancel"、"none"） |
| keyboard | antd `keyboard` 默认 true；EP `close-on-press-escape` 默认 true（alert 为 false，ep/es/components/message-box/src/messageBox.mjs:88-91） | Esc 总是关闭 | 已补齐（TestConfirmAutoFocusAndKeyboard）：`ConfirmOptions.NoKeyboard`；EP alert 默认不响应 Esc 见下行 |
| maskClosable | antd confirm 默认 false；EP confirm/prompt `close-on-click-modal` 默认 true、alert false（同上） | `MaskClosable` 默认 false | 暂不做（Element 主题默认值与 Ant 不同，需要能表达"主题默认 / 开 / 关"的三态字段，`ConfirmOptions` 是稳定层，待用户决定） |
| distinguishCancelAndClose / center / draggable | EP | `DistinguishCancelAndClose`、`Center`、`Draggable` | 已对齐 |
| prompt 校验 | EP `input-pattern`、`input-validator`、`input-error-message` | `Validate func(string) string` | 等价写法：pattern 与错误文案写进 `Validate` |
| EP show-close | EP 默认 true | Element 外观总是有关闭按钮 | 已对齐；隐藏暂不做 |
| EP round-button / button-size / close-on-hash-change | EP | — | 暂不做（前两项低频）；hash 不需要 |
| shadcn AlertDialog 外观 | `p-6 gap-4 rounded-lg border shadow-lg`、标题 `text-lg font-semibold`、描述 `text-sm text-muted-foreground`、页脚右对齐 `gap-2`，无关闭按钮（alert-dialog.tsx） | shad2c_confirm.go | 已对齐；标题与描述间距 `gap-1.5`（6）现为 8，见共享文件改动请求 |
| shadcn AlertDialog `size="sm"` / Media | `max-w-xs`、页脚两列等宽、居中标题（alert-dialog.tsx:49-59、85-99）；`AlertDialogMedia size-16 rounded-md bg-muted` | — | 暂不做（要改 shad2c_confirm.go，不归本组；见共享文件改动请求） |
| shadcn 遮罩 | `bg-black/50` | 用 `ModalToken.MaskBg` | 已补齐（同 Modal） |
| a11y | `role=alertdialog` | `RoleAlertDialog`（Prompt 为 `RoleDialog`）+ `Modal`、`Dismiss` | 已对齐 |

## Drawer（含 shadcn Sheet、Drawer）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| placement | antd `placement` 默认 right；EP `direction` 默认 rtl | `Placement(DrawerEnd…)`（逻辑方向） | 已对齐 |
| size | antd 6 `size: 'default' \| 'large' \| number`，default 378、large 736（antd/drawer/Drawer.js:20-24、84-99）；`width/height` 弃用；EP `size` 30%；shadcn Sheet `w-3/4 sm:max-w-sm`（384） | `Size`、`Large()`；Ant 378、Element 30% 按 Flavor 分支，shadcn 378 | 已补齐（TestDrawerDefaultSize、TestAlignB3KDrawerTokens）：令牌 `Size/SizeRatio`（Ant 378；Element 30%；shadcn 384 且不超过 75%）；**变更**：shadcn 默认宽 378 → 384（窄窗口取 75%）。差异：sheet.tsx 的 `sm:max-w-sm` 在窗口宽 512–640 时 CSS 结果是 75%（> 384），Tango 一律取 min(384, 75%)；top/bottom 方向 sheet 是 `h-auto`（按内容），Tango 仍用 384 / 75% 作高度——都属容器断点 / 按内容高度的缺口，暂不做 |
| resizable | antd 6 `resizable {onResize, onResizeStart, onResizeEnd}`；EP `resizable` | `Resizable`、`SizeLimits`、`BindSize`、`Resized()`、键盘 | 已对齐；`onResizeStart/End` 暂不做（`Resized()` 已够用） |
| 拖动条外观 | antd `draggerSize` 4，悬停 colorPrimary 不透明度 .2、拖动 .3（antd/drawer/style/index.js:212-282）；EP `--el-drawer-dragger-size` 8，悬停 primary（drawer.scss、var.scss $drawer） | 命中宽 8、悬停 2px 实色主色线 | 已补齐（TestAlignB3KDrawerTokens）：Ant `ResizeHandleSize` 4、整条 4px 主色 20% / 拖动 30%（新令牌 `ResizeLineActiveColor`）；Element / shadcn 保持 8 / 2px 主色；两个颜色字段都带 `contrast:"nontext:Bg" spec:"…"`，Ant 亮/暗与 Element 亮色登记为 DesignDefault（6 条，HighContrast 提升到 3:1）；**变更**：Ant 拖动条外观 |
| mask | antd `mask {enabled, blur, closable}`、`maskClosable` | `Mask`、`MaskClosable` | 等价写法（同 Modal）；blur 暂不做 |
| closable.placement | antd 6 `closable {placement: 'start' \| 'end'}`（antd/drawer/DrawerPanel.js:41-58） | Ant 在前、Element/shadcn 在后，按 Flavor 分支 | 已补齐（TestDrawerClosablePlacement）：`ClosablePlacement(p)`、Defaults 键 `closablePlacement`、令牌 `CloseAtEnd` |
| 关闭按钮 | antd 宽高 fontSizeLG + paddingXS = 24，`marginInlineEnd` marginXS 8（style:145-170）；EP `close-btn` 字号 font-size-extra-large 20，颜色继承标题（drawer.scss） | IconButton 22，Ant 间距 12 | 已补齐（TestDrawerAntCloseButton）：令牌 `CloseBtnSize/CloseIconSize/HeaderGap`（Ant 24/16/8、Element 20/20 且 `CloseIconColor` = 标题色、shadcn 16/16）；**变更**：Ant 关闭按钮 22 → 24、与标题间距 12 → 8；Element 关闭图标 16 → 20、颜色 TextTertiary → Text |
| closeIcon | antd | — | 暂不做 |
| extra / footer | antd | `Extra`、`Footer` | 已对齐 |
| loading | antd 6 `loading` | — | 已补齐（TestDrawerKeyboardLoadingPush）：`Loading(on)` |
| push | antd `push {distance}` 默认 180（Drawer.js:22） | 固定 180 | 已补齐（TestDrawerKeyboardLoadingPush）：`Push(px)`（0 关闭） |
| keyboard | antd `keyboard`；EP `close-on-press-escape` | Esc 总是关闭 | 已补齐（TestDrawerKeyboardLoadingPush）：`Keyboard(on)`、Defaults 键 `keyboard` |
| afterOpenChange | antd | — | 已补齐：`AfterOpenChanged()`（同 Modal） |
| destroyOnHidden / autoFocus / zIndex / getContainer | antd | 立即模式；有遮罩时 `TrapFocus` 移入焦点 | 不需要 |
| EP with-header / modal / before-close / destroy-on-close / header-aria-level | EP | `NoHeader()`、`Mask`、`Closed()` | 已对齐 / 等价写法（before-close 同 Modal）/ 不需要 |
| 令牌：内边距 | antd header `padding paddingLG`（16 24）、body paddingLG 24、footer `footerPaddingBlock` paddingXS 8 / `footerPaddingInline` padding 16（style:126-208、278-283）；EP padding-primary 20、header margin-bottom 32、footer padding-top 10 | `DrawerToken` | 已对齐（TestAlignB3KDrawerTokens） |
| EP 标题颜色 | `.el-drawer__header color: text-color-primary`、标题 16px（drawer.scss） | TextSecondary | 已补齐：Element `TitleColor` = Text；**变更** |
| shadcn Sheet 外观 | `SheetHeader p-4 gap-1.5`、`SheetTitle font-semibold text-foreground`（1rem）、内容 `gap-4`、`SheetFooter mt-auto p-4 gap-2`、`border-l`、关闭 `top-4 right-4`、遮罩 `bg-black/50`（sheet.tsx） | 按旧版 `p-6`、标题 18 | 已补齐（TestAlignB3KDrawerTokens）：header / body / footer 内边距 24 → 16、标题 18 → 16、遮罩 0.8 → 0.5；**变更**；关闭按钮仍在标题行末尾（垂直位置差 ≤ 4px） |
| shadcn 动效 | Sheet 打开 500ms、关闭 300ms `ease-in-out` | 三套共用 `DurationSlow` | 暂不做（要按主题拆开打开/关闭时长令牌，收益小） |
| shadcn Drawer（vaul） | bottom 时 `mt-24 max-h-[80vh] rounded-t-lg border-t`、把手 `mt-4 h-2 w-[100px] rounded-full bg-muted`、header 居中、拖拽关闭（drawer.tsx） | — | 暂不做（把手 + 拖拽关闭手势需新增 Drawer 变体，待用户决定是否加 `Handle()`；Sheet 已由 `Drawer` 覆盖） |
| a11y | `role=dialog` | `RoleDialog`（有遮罩时 `Modal`）、`Dismiss` | 已对齐（TestAlignB3KA11yDialogs） |

## Popconfirm

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| title / description / okText / cancelText / showCancel / icon / disabled | antd（antd/popconfirm/index.d.ts、PurePanel.js:25-27） | `Popconfirm(c,title,desc,…)`、`OkText`、`CancelText`、`ShowCancel`、`Icon`、`IconColor`、`Disabled` | 已对齐 |
| okType | antd `okType` 默认 primary（index.js:19） | 只有 `OkDanger()` | 已补齐（TestPopconfirmTypesWidthArrow）：`OkType(t)`、Defaults 键 `okType`；EP 的 success / warning / info 映射为对应状态按钮 |
| EP cancel-button-type / confirm-button-type | EP 默认 text / primary（ep/es/components/popconfirm/src/popconfirm.mjs:30-38） | 取消总是默认按钮 | 已补齐（TestPopconfirmElementDefaults）：`CancelType(t)`、Defaults 键 `cancelType`、令牌 `PopconfirmCancelText`（Element true）；**变更**：Element 取消按钮改为 text 类型 |
| placement | antd 默认 `top`（index.js:17）；EP popper 默认 `bottom`（ep/es/components/popper/src/content.mjs:116） | 三套都是 top | 已补齐（TestPopconfirmElementDefaults）：令牌 `PopconfirmBottom`；**变更**：Element 默认在下方 |
| width | EP `width` 150（popconfirm.mjs:83-85） | 自适应 | 已补齐（TestPopconfirmElementDefaults、TestPopconfirmTypesWidthArrow）：`Width(px)`、Defaults 键 `width`、令牌 `PopconfirmWidth`（Element 150）；**变更** |
| arrow | antd 继承 Popover `arrow`；shadcn 无箭头 | shadcn 按 Flavor 分支去掉；上下方向自画 6px 箭头 | 已补齐（TestPopconfirmTypesWidthArrow）：`Arrow(on)`、Defaults 键 `arrow`、令牌 `PopoverToken.Arrow`（与 Popover 共用）；四个方向统一用 `drawPopupArrow`（Popover 同一箭头），Element 带描边；**变更**：上下箭头高 6 → 7（与 Popover 一致） |
| 面板外观 | antd 与 Popover 相同（圆角 borderRadiusLG、阴影）；EP el-popover 带边框 | 圆角写死 RadiusLG，Element 无边框 | 已补齐：圆角、边框取 `PopoverToken.Radius/BorderColor`；**变更**：Element 气泡加 1px 边框（与 Popover 一致），Ant 不变 |
| 偏移 | antd Popconfirm 渲染 Popover，偏移同 Popover：有箭头 sizePopupArrow/2 + marginXXS = 16/2 + 4 = 12、无箭头 4（antd/_util/placements.js:113-137、tooltip/index.js:115-117、theme/themes/seed.js:56）；EP popper offset 12；shadcn sideOffset 4 | 10 / shadcn 4 | 已补齐：令牌 `PopoverToken.Offset/OffsetNoArrow`（Ant 12/4、Element 12/12、shadcn 4/4），不再按 Flavor 分支；**变更**：Ant / Element 有箭头偏移 10 → 12 |
| 令牌：间距 | antd 图标 fontSize 14、`marginInlineEnd` marginXS 8、message `marginBottom` marginXS 8、描述 `marginTop` marginXXS 4、按钮 `marginInlineStart` marginXS 8（antd/popconfirm/style/index.js） | 同 | 已对齐 |
| ConfigProvider.ComponentDisabled | — | 触发器节点 `disabled` | 已补齐（TestPopconfirmDisabledConfig）：禁用时点击、Enter 都不打开；Defaults 键 `disabled` |
| trigger | antd 继承 Popover `trigger`，默认 click | 只支持点击 | 暂不做（低频） |
| okButtonProps / cancelButtonProps / onPopupClick | antd | — | 等价写法：`OkType/CancelType/OkDanger`；onPopupClick 不需要 |
| EP icon / icon-color / hide-icon | 默认 QuestionFilled、`#f90` | 默认警告实心图标 | 等价写法：`Icon(icon.Question).IconColor(theme.Hex("#f90"))`；`hide-icon` = `Icon(nil)`（内置图标集无实心问号，不新增） |
| EP hide-after / teleported / persistent / effect | EP | — | 不需要（点击触发、立即模式）；`effect: dark` 暂不做 |
| a11y | — | `RoleAlertDialog`、描述、`Dismiss`；关闭后焦点回到触发器 | 已对齐（TestOverlayEscTopmostModalPopconfirm） |

## Tour

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| steps / open / current / onChange / onClose / onFinish / type | antd（antd/tour/interface.d.ts）、rc-tour（rc-tour/package/lib/interface.d.ts） | `Tour(c,&open,&cur,steps)`、`Changed/Closed/Finished`、`Primary()`、`TourStep.Type` | 已对齐 |
| current 越界 | rc-tour：`current < 0 \|\| current >= steps.length` 时不显示（mergedOpen false，es/Tour.js:62） | 夹到范围内 | 已补齐（TestTourOutOfRange）：越界时不显示，不改 `*open`、`*current`；**变更** |
| keyboard | antd `keyboard` 默认 true：Esc 关闭、←/→ 切换步骤，输入框里不响应（rc-tour es/Tour.js:123-160） | 只有 Esc | 已补齐（TestTourKeyboardArrows）：←/→、`Keyboard(on)`、Defaults 键 `keyboard`。差异：rc-tour 在 window 上监听；Tango 只在焦点位于面板内或无焦点时处理 ←/→，避免焦点在可点击目标或页面上的 Tabs / Slider 时一次按键同时切步骤和改控件 |
| disabledInteraction / target-area-clickable | antd 默认目标可交互；EP `target-area-clickable` 默认 true；rc-tour `Mask` 无论 `mask` 都渲染，外层 `pointerEvents: pos && !disabledInteraction ? 'none' : 'auto'`，有遮罩时四块透明挡板（rc-tour es/Mask.js） | 遮罩挡住整个页面（含目标） | 已补齐（TestTourTargetInteraction、TestTourNoMaskBlocking）：有遮罩时镂空四周四条挡块、镂空处可点击；无遮罩时只有"有目标且允许交互"页面可点，居中步骤或 `DisabledInteraction` 时透明层挡住整页；`DisabledInteraction(on)`、Defaults 键 `disabledInteraction`；**变更** |
| mask | antd `mask: boolean \| {style, color}`；step 覆盖 | `NoMask`、`TourStep.Mask` | 已补齐（TestTourClosableAndGap）：`MaskColor(col)`（可设全透明：只挡指针不变暗）；style 不需要 |
| gap | rc-tour `gap {offset 6, radius 2}`（hooks/useTarget.js:59-80）；EP 同 | `Gap(px)`，圆角取 `th.Radius`（6） | 已补齐（TestTourClosableAndGap、TestAlignB3KTourTokens）：`GapRadius(px)`，令牌 `HoleRadius` 2；**变更**：镂空圆角 6 → 2 |
| arrow / step.arrow | antd `arrow: boolean \| {pointAtCenter}`、step 覆盖 | `NoArrow()` | 已补齐：`TourStep.HideArrow`；`pointAtCenter` 暂不做（`drawPopupArrow` 总是指向目标中心并夹在面板内） |
| placement | antd `placement`（有目标默认 bottom，无目标居中） | 只有 `TourStep.Placement` | 已补齐（TestTourClosableAndGap）：`Placement(s)`、Defaults 键 `placement` |
| closable / closeIcon | rc-tour `closable`（null 时无关闭按钮、Esc 不关闭） | 总是有关闭按钮 | 已补齐（TestTourClosableAndGap）：`Closable(on)`；closeIcon 暂不做 |
| indicatorsRender / actionsRender / scrollIntoViewOptions | antd | `IndicatorsRender`、`ActionsRender`、`ScrollIntoView` | 已对齐 |
| zIndex / getPopupContainer / animated | antd | — | 不需要 |
| 面板宽度 | antd `width: 520` + `max-width: fit-content`（antd/tour/style/index.js:41-43）；EP `--el-tour-width` 520（var.scss $tour） | Ant 固定 320 | 已补齐（TestAlignB3KTourTokens）：Ant 最宽 520、按内容收缩（令牌 `FitContent`），Element 固定 520；**变更** |
| 面板间距 | antd header `padding-bottom` paddingXS 8、footer `padding-top` paddingXS 8（style:96-110）；EP padding-primary 12 | 标题下 8、页脚上 16 | 已补齐：令牌 `HeaderMarginBottom/FooterMarginTop`（Ant 8/8、Element 12/12）；**变更**：Ant 页脚上边距 16 → 8、Element 标题下 8 → 12、页脚上 16 → 12 |
| 阴影 / 指示点 | antd `boxShadowTertiary`、指示点 6×6 `colorFill`、间距 6（style:59、113-125） | ShadowSecondary、FillSecondary | 已补齐：Ant 阴影 → ShadowTertiary、指示点 → Fill；**变更** |
| 令牌：closeBtnSize / primary 按钮 | antd `closeBtnSize` fontSize × lineHeight = 22；`primaryPrevBtnBg` 白 15%、`primaryNextBtnHoverBg`（style:197-206） | 22；primary 时上一步 Ghost、下一步默认按钮 | 已对齐（closeBtnSize）；primary 按钮精确底色暂不做 |
| EP show-arrow / show-close / close-on-press-escape / z-index / content-style | EP | `NoArrow`、`Closable`、`Keyboard`、—、— | 已对齐 / 不需要 |
| a11y | — | 面板 `RoleDialog`、有遮罩时 `Modal`、描述、`Dismiss`；指示点 `RoleText` | 已对齐（TestAlignB3KA11yDialogs） |

## HoverCard（= Popover hover）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 入口 | shadcn HoverCard（hover-card.tsx） | `Popover(c).Trigger("hover")`（默认 hover） | 等价写法（AUTHORING §13，不新建入口） |
| 延迟 | Radix HoverCard Root `openDelay = 700`、`closeDelay = 300`（radix-hover-card/package/dist/index.mjs:30-31） | shadcn `PopoverToken` 0 / 100 ms | 已补齐（TestHoverCardDelays、TestAlignB3KPopoverTokens）：shadcn `MouseEnterDelay` 700ms、`MouseLeaveDelay` 300ms（Radix Popover 没有 hover，hover Popover 即 HoverCard）；**变更** |
| 宽度 | HoverCardContent `w-64`；Popover `w-72` | shadcn Popover 默认 288 | 等价写法：HoverCard 用 `Popover(c).Width(256)` |
| 外观 / 位置 | `rounded-md border bg-popover p-4 shadow-md`、`sideOffset` 4、`align` center | shadcn Popover 令牌、无箭头、偏移 4、居中 | 已对齐 |

## Popover 箭头与偏移（第 2 批复审遗留，转交本组）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| arrow 默认 | antd `arrow` 默认 true；EP `show-arrow` true；shadcn popover.tsx 无箭头 | `Prop(c,"Popover","arrow", c.Theme.Flavor != StyleShadcn)`：按 Flavor 取且读全局主题，组件级覆盖不生效 | 已补齐（TestPopoverArrowOffsetTokens、TestAlignB3KPopoverTokens）：令牌 `PopoverToken.Arrow`（shadcn false），从 `ThemeFor(CompPopover)` 读 |
| 偏移 | antd 有箭头 16/2 + 4 = 12、无箭头 4（同上）；EP 12；shadcn sideOffset 4 | 有箭头 10（Element 12）、无箭头 4，按 Flavor 分支 | 已补齐（TestPopoverArrowOffsetTokens）：令牌 `Offset/OffsetNoArrow`；**变更**：Ant 有箭头偏移 10 → 12。说明：Tango 箭头高 7（`drawPopupArrow`），antd 箭头实际可见高约 8 = 16/2，取源码值 12 后箭头尖与触发器间留约 5px，略大于 antd 的 4px，视觉可接受，按源码取值 |
| a11y | Radix HoverCard 内容不进 Tab 顺序 | hover 面板 `RoleTooltip` | 已对齐 |

## 叠加浮层（Review Focus 2）

- Modal 里开 Popconfirm：第一次 Esc 只关 Popconfirm，焦点回到它的触发器；第二次 Esc 关 Modal，焦点回到页面上的打开按钮（TestOverlayEscTopmostModalPopconfirm）。
- Drawer 里开 Modal：第一次 Esc 只关 Modal（TestOverlayEscTopmostDrawerModal）。

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 背景模糊 | Modal / Drawer `mask.blur` | paint 层 backdrop blur 原语（同第 1 批 FooterToolbar） | 暂不做，等用户决定 |
| 关闭后不归还焦点 | Modal / Drawer `focusable.focusTriggerAfterClose: false` | `TrapFocus` 的选项（focus.go） | 暂不做 |
| 遮罩可穿透的模态 | EP Dialog `modal-penetrable` | 浮层"显示遮罩但不挡指针"的语义 | 暂不做，等用户决定 |
| 容器宽度断点 | shadcn Dialog 页脚窄屏纵向反排、Modal 响应式宽度 | 按窗口宽切换数值的通用写法（同第 2 批） | 暂不做 |

## 待用户决定

1. Element 主题下 `ElMessageBox.confirm/prompt` 的 `close-on-click-modal` 默认 true、`alert` 默认不响应 Esc 和遮罩：`ConfirmOptions.MaskClosable` 是稳定层 bool，零值只能表示"关"。要对齐需要新增三态字段（如 `MaskClose ConfirmToggle`），还是保持 Ant 行为。
2. shadcn Drawer（vaul）：是否给 `Drawer` 加 `Handle()` 变体（底部把手 + 圆角顶 + 拖拽关闭）。
3. Ant Drawer 拖动条按源码改为命中宽 4px、主色 20%/30%（对比度低于 3:1，已登记为 DesignDefault，HighContrast 主题会提升）。命中宽 8 → 4 让可拖区域变窄，影响可用性；如希望保留原先 8px 命中宽或 2px 实色线，改令牌即可。

## CHANGELOG 草稿

### 新增

- `ui/theme`（审阅后补）：`ModalToken.CloseNoHoverBg/CloseHoverColor`、`PopoverToken.Arrow/Offset/OffsetNoArrow`（Popover 与 Popconfirm 共用）；`DrawerToken.ResizeLineColor/ResizeLineActiveColor` 加对比度登记（spec 标签）。

- `ui`：新文件 `popup_arrow.go`，共享弹层箭头 `drawPopupArrow`（原 `popDrawArrow`，Popover / Tour / Popconfirm 共用）。
- `ui`：Modal `Keyboard(on)`（Defaults 键 `keyboard`）、`Loading(on)`、`AfterOpenChanged()`。
- `ui`：Drawer `ClosablePlacement(p)`（Defaults 键 `closablePlacement`）、`Keyboard(on)`（Defaults 键 `keyboard`）、`Loading(on)`、`Push(px)`、`AfterOpenChanged()`。
- `ui`：`ConfirmOptions.AutoFocusButton`（"ok"/"cancel"/"none"）、`ConfirmOptions.NoKeyboard`。
- `ui`：Popconfirm `Arrow(on)`、`Width(px)`、`OkType(t)`、`CancelType(t)`；Defaults 键 `arrow`、`width`、`okType`、`cancelType`、`disabled`。
- `ui`：Tour `Keyboard(on)`（←/→ 切换步骤）、`DisabledInteraction(on)`、`Closable(on)`、`MaskColor(col)`、`GapRadius(px)`、`Placement(s)`；`TourStep.HideArrow`；Defaults 键 `keyboard`、`disabledInteraction`、`placement`。
- `ui/theme`：`ModalToken.Width/WidthRatio/Top/TopRatio/Centered/CloseBtnSize/CloseIconSize/CloseInsetTop/CloseInsetEnd/ConfirmWidth/ConfirmBtnsMarginTop`；`DrawerToken.ResizeLineActiveColor/Size/SizeRatio/CloseAtEnd/CloseBtnSize/CloseIconSize/HeaderGap`；`TourToken.FitContent/HeaderMarginBottom/FooterMarginTop`；`PopoverToken.PopconfirmBottom/PopconfirmWidth/PopconfirmCancelText`。

### 变更

- Modal：Element 关闭按钮无悬停底色、图标 TextTertiary → info（#909399）、悬停 / 聚焦变主色；shadcn 关闭图标悬停 70% → 100%。关闭按钮改为绝对定位于面板右上（RTL 左上），标题行留出位置——Ant 22px 行内 → 32×32、距边 12（antd modalCloseBtnSize / (56 − 32) / 2）；Element → 48×48 贴角（el-dialog headerbtn）；shadcn → 16×16、top-4 right-4。默认宽度不超过窗口宽 − 32（antd maxWidth、shadcn max-w-[calc(100%-2rem)]）。shadcn 默认宽 520 → 512（sm:max-w-lg）、标题行高 28 → 18（leading-none）、遮罩 黑 80% → 50%（bg-black/50）。默认宽度 / 顶部距离 / 居中改由令牌给出（数值不变）。
- Confirm / MessageBox：Ant 按钮与正文间距 24 → 12（antd confirmBtnsMarginTop = marginSM）；默认宽度改由令牌给出（416 / 420 / 512 不变），并不超过窗口宽 − 32；shadcn 遮罩 80% → 50%。
- Drawer：Ant 关闭按钮 22 → 24、与标题间距 12 → 8（antd fontSizeLG + paddingXS、marginXS）；Ant 拖动条命中宽 8 → 4、悬停 2px 主色线 → 4px 主色 20%、拖动 30%（antd draggerSize / hover / dragging）；Element 标题色 TextSecondary → Text（el-drawer header text-color-primary）、关闭按钮外框 22 → 20、图标 16 → 20 且用标题色（font-size-extra-large、color inherit）；shadcn Sheet 关闭按钮 22×22 → 16×16（size-4）；shadcn Sheet 默认宽 378 → 384 且不超过 75%（w-3/4 sm:max-w-sm），header / body / footer 内边距 24 → 16（p-4）、标题 18 → 16（font-semibold，无 text-lg）、遮罩 80% → 50%。默认尺寸与关闭按钮位置改由令牌给出。
- Popconfirm：圆角、边框取 Popover 令牌（Element 圆角 RadiusLG 8 → 4，Ant 仍 8），偏移改由令牌给出（Element 10 → 12），上下箭头改用 Popover 的 `drawPopupArrow`（高 6 → 7，Element 带描边）；Element 默认在下方（popper placement bottom）、宽 150、取消按钮为 text 类型（el-popconfirm）、气泡加 1px 边框。
- Tour：`current` 越界时不显示（原先夹到范围内；rc-tour mergedOpen）；高亮目标默认可点击、无遮罩时页面可交互（原先遮罩层挡住全部指针；antd disabledInteraction 默认 false、EP target-area-clickable true）；镂空圆角 6 → 2（gap.radius）；Ant 面板宽 320 → 最宽 520 按内容收缩（width 520 + max-width fit-content）、页脚上边距 16 → 8、阴影 ShadowSecondary → ShadowTertiary、指示点 FillSecondary → Fill；Element 标题下 8 → 12、页脚上 16 → 12（padding-primary）。测试 `TestT2aTokenDefaults` 的 Ant Tour 宽度期望 320 → 520。
- Popover：默认箭头改由令牌 `PopoverToken.Arrow` 给出（读组件作用域主题，组件级覆盖生效）；Ant 有箭头时与触发器距离 10 → 12（antd sizePopupArrow/2 + marginXXS）；Element 12、无箭头 4 不变。
- Popover（shadcn）：hover 延迟 0 / 100ms → 700 / 300ms（Radix HoverCard openDelay / closeDelay，hover Popover 即 shadcn HoverCard）。

## 共享文件改动请求

1. `ui/t2a_tokens_test.go`（不在归属表）：本组已用一次 Edit 小块把 Ant Tour 期望宽度 320 改为 520（注释写明出处），请主线程合并时确认；同文件的 steps / splitter / breadcrumb 失败属组 J。
2. `ui/theme/shadcn.go` / `theme.go`：shadcn 全局 `BgMask` 仍为黑 80%（shadcn v3 `bg-black/80`）。v4 的 Dialog / AlertDialog / Sheet / Drawer 遮罩都是 `bg-black/50`；本组只在 `ModalToken.MaskBg`、`DrawerToken.MaskBg` 里改成 50%。建议把 shadcn `BgMask` 改为 `RGBA(0,0,0,.5)`（影响 Tour 遮罩、Image 预览等其它用 `BgMask` 的组件，需要另记 CHANGELOG）。
3. `ui/shad2c_confirm.go`：shadcn AlertDialog 标题与描述间距 `gap-1.5` = 6（现为 8）；`size="sm"`（max-w-xs、页脚两列等宽、居中标题）与 `AlertDialogMedia` 如需要在此实现（`ConfirmOptions` 需再加 `Size` 字段，稳定层只增）。
4. `ui/shad2c_defaults.go`：`(*ModalW).defaults` 读的 `width` 现在覆盖令牌宽度（行为不变）；`keyboard`、`closablePlacement` 等新键直接在 feedback.go 构造函数里用 `Prop` 读，未改此文件。
5. `ui/AUTHORING.md` §13/§14：Modal 一行补 `Keyboard/Loading/AfterOpenChanged`，注明 shadcn Dialog = `Modal`；Drawer 一行补 `ClosablePlacement/Keyboard/Loading/Push/AfterOpenChanged`，注明 Sheet / shadcn Drawer = `Drawer`（vaul 把手暂不做）；Confirm 一行注明 shadcn AlertDialog = `Confirm`、补 `AutoFocusButton/NoKeyboard`；Popconfirm 一行补 `Arrow/Width/OkType/CancelType`；Tour 一行补 `Keyboard/DisabledInteraction/Closable/MaskColor/GapRadius/Placement`；HoverCard 写明 `Popover.Trigger("hover")` + `Width(256)`，shadcn 延迟 700/300。§6 可加一句"弹层箭头统一用 popup_arrow.go 的 `drawPopupArrow`"。
6. `api/stable/ui.txt`：`ModalW`、`DrawerW`、`PopconfirmW`、`ConfirmOptions` 在稳定层，新方法 / 字段随类型进入稳定层（只增，签名无改动）；请主线程跑 `apitxt -w` 与 `apidiff` 确认。
7. Locale：本组未改 `ui/locale.go` / `ui/locale_packs.go`（新界面文案都复用已有字段：`Close`、`OK`、`Cancel`、`Tour.*`）。
9. `ui/theme/element_sizes_test.go`（主线程授权）：Ant / shadcn Tour 宽期望 320 → 520，已一次小块 Edit。
10. `ui/theme/components_test.go`、`ui/theme/contrast_test.go`：Drawer 拖动条加 spec 标签后成为常驻审计对，已小块 Edit：stock 主题组件对白名单加 `Drawer.ResizeLine*`，DesignDefault 计数 105 → 111（注释写明 6 条来源）。请确认（与其它组改计数时需合并）。
11. 已按主线程转交（第 2 批复审 Important 4 / Minor 1）处理 `ui/data_popover.go` Popover 构造函数与偏移：`Prop(c, "Popover", "arrow", tk.Arrow)`，读 `ThemeFor(CompPopover)` 的令牌，去掉 Flavor 分支。`ui/theme/comp_tooltip.go` 的 shadcn `Offset = 8`（注释"not yet checked"）不归本组，仍待核对 Radix Tooltip sideOffset。
8. 黄金图：本组改动不涉及 `goldenPage` 里的组件；当前 `TestGolden` 的差异来自 Alert（组 L）。
