# 第 1 批 · 组 B：输入与表单

> 对照日期：2026-11-14。参考源码都在 `/tmp/tango-ref`（见 [README](README.md)）。下表“来源”里的缩写：
>
> - `antd/…` = `antd/package/es/…`（antd@6.7.0）
> - `rc-*/…` = `rc-b/<包>/package/es/…`：antd 依赖的 `@rc-component/*`，按 antd 6.7.0 `package.json` 的版本范围用 `npm pack` 取回（input 1.4.1、input-number 1.8.0、checkbox 2.0.0、slider 1.2.1、switch 1.0.3、upload 1.1.1、form 1.8.6、textarea 1.1.2；范围内的最新补丁版，可能比 antd 发布时锁定的补丁版新）
> - `ep/…` = `ep/package/es/components/…`，`chalk/…` = `ep/package/theme-chalk/src/…`（element-plus@2.14.8）
> - `shad/…` = `shadcn-ui/apps/v4/registry/new-york-v4/ui/…`（commit e62c9435）
>
> 测试名不带包名的都在 `ui/align_b1b_test.go`；令牌数值测试在 `ui/theme/align_b1b_test.go`。

## 范围

Input（含 Password）、TextArea、Search、InputNumber、OTP、Compact/Addon（shadcn InputGroup）、Form/FormItem（shadcn Field）、Checkbox/CheckboxGroup/CheckAll、Radio/RadioGroup、Switch、Slider/SliderRange、Upload/DropZone。

## 通用写法（各表不再重复）

| 规范项 | 结论 |
|---|---|
| `ref` / `focus()` / `blur()` | 等价写法：`Focus()`、`ID()` + `c.SetFocus`；Go 里没有 ref 对象 |
| 语义化 `classNames` / `styles`（antd 6） | 等价写法：组件令牌 `ui/theme/comp_<名>.go` 用 `ConfigProvider.Components` 局部覆盖；结构样式不开放 |
| `defaultValue` / `defaultChecked` | 不需要：值由应用持有（指针），初值就是 default |
| `onChange` / `onFocus` / `onBlur` 等事件 | 等价写法：查询方法 `Changed()`、`Focused()` 等，在本帧读取 |
| EP `name` / `id` / `validate-event` / `tabindex` / `aria-label` | 不需要（`name`/`id`/`validate-event` 只服务 HTML 表单，设计 §7）；`aria-label` = `A11yLabel` |
| EP `props` 字段映射、`options` 的 `props` | 不需要（Go 的 `Option` 有类型） |
| antd `size`（small/middle/large）/ EP `size` | 已对齐：`Size(ui.Small/Middle/Large)`，默认取 `ConfigProvider` 的组件尺寸 |
| 表单项状态 | 已对齐：控件读 `c.itemStatus`（AUTHORING §9） |
| 明暗主题 | 已对齐：本组绘制只用令牌，不判断 `th.Dark`；gallery 三套主题 × 明暗出图（见文末） |

---

### Input（含 Password）

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 前后缀 | antd `prefix`/`suffix: ReactNode`（rc-input/interface.d.ts）；EP `prefix-icon`/`suffix-icon` 与插槽（ep/input/src/input.d.ts） | input.go `Prefix/Suffix/PrefixText/SuffixText` | 等价写法（图标或短文字；任意节点用 `Compact`+`Addon`） |
| 前置/后置标签 | antd `addonBefore/After`（v6 已弃用，改 Space.Compact，antd/input/Input.d.ts）；EP `prepend/append` 插槽 | entry_compact.go `Compact` + `Addon` | 已对齐（按 antd 6 推荐写法） |
| 清除 | antd `allowClear: boolean \| {clearIcon}`、`onClear`；EP `clearable`、`clear-icon` | `AllowClear()`；清除后 `Changed()` 为真 | 已对齐（自定义清除图标：不需要，图标随主题替换） |
| 字数 | antd `showCount`、`count {max, strategy, show, exceed}`（rc-input/interface.d.ts）；EP `show-word-limit`、`word-limit-position`、`count-graphemes` | p2d_input.go `ShowCount`、`MaxLength` | 已对齐（按字符 rune 计；`strategy`/`count-graphemes` 自定义计数：暂不做，缺按字素计数的 text 能力） |
| 变体 | antd 6 `variant: outlined \| filled \| borderless \| underlined`，默认 outlined（antd/input/Input.d.ts）；EP 无 | p2d_input.go `Variant`，Defaults `"variant"` | 已补齐测试（TestInputVariantUnderlined；功能已存在，补三主题 + Defaults 行为测试） |
| 状态 | antd `status: error \| warning`；EP 无（靠 FormItem） | `Status` | 已对齐 |
| 密码可见切换 | antd `Input.Password`：`visibilityToggle: boolean \| {visible, onVisibleChange}`、`iconRender`（antd/input/Password.d.ts）；EP `show-password`（有眼睛）与 `type="password"`（无眼睛） | `Password()` 总带眼睛按钮 | 已补齐（`VisibilityToggle(false)`；TestInputPasswordVisibilityToggleA11y）。`iconRender` 不需要（图标随主题） |
| 只读 | antd/EP `readOnly/readonly` | `ReadOnly()`（a11y ReadOnly） | 已对齐 |
| 格式化 | EP `formatter/parser`（输入时显示格式化文本） | — | 暂不做（编辑中光标位置要随格式化映射，editState 没有映射能力；InputNumber 已有 Formatter/Parser） |
| 文本域相关 | EP `type=textarea`、`autosize`、`resize` | 见 TextArea | — |
| 键盘 / IME | 浏览器 input 行为 | editState + processEdit（选区、词跳转、剪贴板、IME） | 已对齐 |
| 禁用 | antd/EP `disabled` | `Disabled`，`ConfigProvider.ComponentDisabled` | 已对齐 |
| 令牌 | antd/input/style/token.js：paddingInline 11 / SM 7 / LG 11，inputFontSize 14 / LG 16，activeBorderColor colorPrimary，hoverBorderColor colorPrimaryHover，addonBg colorFillAlter，activeShadow `0 0 0 2px controlOutline` | theme/comp_input.go `deriveInput` | 已对齐（TestAlignB1bAntTokens） |
| EP 尺寸 | chalk/common/var.scss `$input-font-size` 12/14/14、`$input-padding-horizontal` | comp_input.go Element 分支 | 已对齐（原有，未改） |
| shadcn 外观 | shad/input.tsx：`h-9 rounded-md border-input bg-transparent px-3 shadow-xs`，`focus-visible:border-ring ring-[3px] ring-ring/50`，`aria-invalid:border-destructive ring-destructive/20`，`disabled:opacity-50` | input.go `paintFrameWith` shadcn 分支（50% 透明度、无 hover 边框）、Input 令牌 Shadow | 已对齐（原有） |
| a11y | textbox，名称来自 A11yLabel / 表单项 / 占位符 | `semantics`；眼睛按钮 `loc.A11y.ShowPassword/HidePassword` | 已对齐 |

### TextArea

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 行数 / 自适应 | antd `autoSize: boolean \| {minRows, maxRows}`（rc-textarea/interface.d.ts），不设 rows 时是原生 textarea 的默认 2 行（HTML 规范，依据知识，未核对）；EP `rows` 默认 2（ep/input/src/input.d.ts:251） | entry_inputx.go `Rows`（默认 3）、`AutoSize(min,max)` | 已补齐（默认 3 → 2；TestTextAreaDefaultRows）。见 CHANGELOG「变更」 |
| 字数、超出 | antd `showCount`、`count.exceedFormatter`；EP `show-word-limit` | `ShowCount`、`MaxLength`、`Exceed` | 已对齐 |
| 清除 | antd `allowClear` | `AllowClear` | 已对齐 |
| 变体 | antd 6 TextArea `variant`（antd/input/TextArea.d.ts）；ConfigProvider 取值 `useVariant('textArea', …)`，**不回落** input（antd/input/TextArea.js:95） | 无 | 已补齐（`Variant`，Defaults 只读 `"TextArea"` 的 `"variant"`；TestTextAreaVariantReadOnlyA11y、TestVariantDefaultsFallback） |
| 只读 | antd/EP `readOnly/readonly` | 无 | 已补齐（`ReadOnly`；TestTextAreaVariantReadOnlyA11y） |
| 拖拽改尺寸 | EP `resize: none \| both \| horizontal \| vertical`；浏览器 textarea 默认右下角可拖 | — | 暂不做（缺拖拽手柄改尺寸的通用能力，Splitter 的拖拽是容器级） |
| shadcn 外观 | shad/textarea.tsx：`min-h-16 px-3 py-2 rounded-md shadow-xs field-sizing-content` | 共用 `paintFrame`；没有最小高度 | 已补齐（Input 令牌 `TextAreaMinHeight`，shadcn 64；TestTextAreaDefaultRows、TestAlignB1bShadcnTokens）。`field-sizing-content` = `AutoSize`（等价写法） |

### Search

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 变体 | antd `useVariant('inputSearch', …, 'input')`（antd/input/Search.js:55，回落 input） | 内部 Input 读 `"Input"` 键 | 已补齐（`SearchW.Variant`，Defaults `"Search"` 键，没设时回落 `"Input"`；TestVariantDefaultsFallback）。Password 是 Input 的一种模式，直接用 `"Input"` 键（= antd 'inputPassword' 回落 'input'，等价写法） |
| 搜索按钮 | antd `enterButton: boolean \| ReactNode`、`searchIcon`、`loading`（antd/input/Search.d.ts） | entry_inputx.go `EnterButton(label)`、`Loading` | 已对齐（`searchIcon` 自定义：不需要，图标随主题） |
| 搜索事件 | antd `onSearch(value, e, {source: 'clear' \| 'input'})` | `Searched()`（按钮或 Enter） | 等价写法（清除时 antd 也触发 onSearch：清除后 `Changed()` 为真，应用自行判断） |
| EP | 无独立组件，`el-input` + append 按钮 | `Search` | 等价写法 |
| 禁用 / 尺寸 / 状态 | antd `disabled/size/status` | 有 | 已对齐 |

### InputNumber

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 模式 | antd 6 `mode: 'input' \| 'spinner'`（rc-input-number/InputNumber.d.ts；antd/input-number/index.js 里 spinner 用 Plus/Minus 图标）；EP `controls-position: '' \| 'right'`（默认两侧） | input.go `ControlsPosition("right"/"sides")`，Element 默认 sides | 已补齐（`Mode("input"/"spinner")`，Defaults `"mode"`；TestInputNumberSpinnerMode、TestInputNumberModeDefaults） |
| spinner 外观 | antd/input-number/style/index.js「Spinner Mode」：按钮透明、paddingInline、内侧 1px 分隔线、数值居中、colorText，hover colorPrimary；rc-input-number 渲染顺序 down、prefix、input、suffix、up | Ant 的 sides 按钮原来是 FillTertiary 底、TextSecondary、宽 = 高 | 已补齐（Ant 令牌 `SideHandleBg` 透明、`SideHandleColor` = Text、`SideHandleWidth` 37/29/39；TestAlignB1bAntTokens）。见 CHANGELOG「变更」 |
| 到边界禁用 | rc-input-number `upDisabled/downDisabled`，样式 `-action-up-disabled` 光标 not-allowed、colorTextDisabled | 按钮只变色，点击被夹住 | 已补齐（按钮节点禁用 + `UpDisabled()/DownDisabled()`；TestInputNumberSpinnerMode） |
| 显示按钮 | antd `controls: boolean`；EP `controls` | 总显示 | 已补齐（`Controls(false)`，Defaults `"controls"`；TestInputNumberControlsHidden） |
| 只读 | antd `readOnly`：不更新值（rc-input-number/InputNumber.js:268 `!readOnly && !disabled`）；EP `readonly` | 无 | 已补齐（`ReadOnly`：失焦 / 回车不夹紧、不取整；TestInputNumberReadOnlyA11y、TestInputNumberReadOnlyOutOfRange） |
| 禁用 / 只读时的按钮 | antd：`mergedControls = controls && !mergedDisabled && !readOnly`（antd/input-number/index.js:71-77），两种模式都不渲染按钮，spinner 数值仍居中；EP：disabled / readonly 都保留按钮、不响应（disabled 为禁用外观；readonly 外观正常，禁用样式只在到达上下限时出现） | 原来 spinner 禁用仍画灰按钮、ReadOnly 一律隐藏 | 已补齐（令牌 `HideControlsInactive`：Ant 1、shadcn 1（无官方定义，随 antd 行为基线）、Element 0；TestInputNumberDisabledSpinner）。Element readonly 时 input 与两侧模式的按钮都保持正常外观、不响应 |
| 键盘 | antd `keyboard`（默认 true） | 上下键总可步进 | 已补齐（`Keyboard(false)`，Defaults `"keyboard"`；TestInputNumberControlsHidden） |
| 严格步长 | EP `step-strictly` | 无 | 已补齐（`StepStrictly`，只作用于 float64 值；TestInputNumberStepStrictly） |
| 滚轮 | antd `changeOnWheel` | `Wheel()` | 已对齐 |
| 格式化 | antd `formatter/parser`、`decimalSeparator`；EP `formatter/parser` | `Formatter/Parser` | 已对齐（`decimalSeparator`：等价写法，用 Formatter/Parser） |
| 精度 / 字符串模式 | antd `precision`、`stringMode`；EP `precision` | `Precision`、`StringMode` | 已对齐 |
| 前后缀 | antd `prefix/suffix` | `Prefix/PrefixText` | 暂不做 suffix（后缀与 input 模式按钮位置冲突，需重排布局；可用 `Compact`+`Addon`） |
| 清除 | antd `allowClear`、`onClear` | `Empty(&empty)` 可清空 | 暂不做 allowClear 图标（清空用删除文本即可，`Empty` 已支持空值） |
| 空值 | EP `value-on-clear: 'min' \| 'max' \| number \| null` | `Empty` | 等价写法（清空后由应用按需写回 min/max） |
| 对齐 | EP `align: left \| right \| center`（默认 center，chalk/input-number.scss `text-align`） | sides 居中，right 模式左对齐 | 暂不做（EP 默认居中只在 sides 下生效，right 模式按 EP 也居中：需要新增 Element 令牌或 Defaults 键，留到第 5 批统一看截图） |
| 受控越界 | antd：显示原值，步进时 getRangeValue 夹到范围 | 原样显示，− 时夹到 Max | 已补齐测试（TestInputNumberOutOfRange） |
| 禁用 | antd/EP `disabled` | `Disabled`；ComponentDisabled | 已补齐测试（TestInputNumberDisabledSpinner） |
| 令牌 | antd/input-number/style/token.js：controlWidth 90，handleWidth controlHeightSM−2 = 22，handleFontSize fontSize/2 = 7 | comp_inputnumber.go ControlWidth 90、HandleWidth 22、HandleFontSize 10 | ControlWidth/HandleWidth 已对齐（TestAlignB1bAntTokens）；HandleFontSize 暂不做（antd 是图标字号 7，Tango 的 10 是 24 网格图标框，需截图比对后再定） |
| EP 令牌 | chalk/input-number.scss：宽 150（`$input-number-width`），按钮宽 = 输入高、`font-size: 13px`、`--el-fill-color-light` 底 | Element 分支 150、按钮宽 = 高、图标 14 | 已补齐（`SideHandleFontSize` 13；TestAlignB1bElementTokens） |
| shadcn | 无对应组件 | 自绘 lucide 上下箭头 | 不需要（无官方定义，保持现状） |
| a11y | spinbutton，值、范围 | `semantics`；只读时 ReadOnly、去掉 SetValue | 已对齐 |

### OTP

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 长度 | antd `length`（默认 6，antd/input/OTP/index.js）；EP `length`（默认 6） | entry_otp.go `OTP(c, &v, n)` | 等价写法（长度是必填参数） |
| 分隔符 | antd `separator: ReactNode \| (index) => ReactNode`（antd/input/OTP/index.d.ts:36，最后一格后不渲染）；EP `separator: string \| VNode \| fn`；shadcn `InputOTPSeparator`（MinusIcon，shad/input-otp.tsx） | 无 | 已补齐（`Separator(s)`、`SeparatorFunc(func(i int) string)`；shadcn 下分隔符把格子分成独立圆角组；TestOTPSeparator） |
| 变体 | antd `variant`，`useVariant('otp', …, 'input')` 回落 input（antd/input/OTP/index.js:79）；EP `type: outlined \| filled \| underlined` | 无 | 已补齐（`Variant`；Defaults `"OTP"` 键，再回落 `"Input"`；TestOTPVariantUnderlined、TestVariantDefaultsFallback） |
| 只读 | EP `readonly` | 无 | 已补齐（`ReadOnly`，仍可聚焦与方向键移动；TestOTPReadOnlyA11y） |
| 掩码 | antd `mask: boolean \| string`；EP `mask` | `Mask(s)` | 已对齐 |
| 只收数字 / 校验 | antd `type`；EP `validator(char, index)`、`inputmode` | `Numeric()` | 等价写法（逐字符校验函数：暂不做，Numeric 覆盖常见用法） |
| 格式化 | antd `formatter(value)` | — | 暂不做（低频；应用可在 `Changed()` 后改写值） |
| 完成事件 | EP `finish`；antd onChange 在填满时触发 | `Completed()` | 已对齐 |
| 间距 | antd/input/style/otp.js `columnGap: paddingXS`（8）；EP `$input-otp-gap` | `Gap` 默认 8，shadcn 0（一组相连） | 已对齐 |
| 键盘 | 退格回到前一格、左右移动、粘贴分发 | 有 | 已对齐 |
| a11y | antd `aria-label` = locale otpCell「${index}/${total}」 | 每格 `loc.Digit(i, n)`；只读时 ReadOnly | 已对齐 |

### Compact / Addon（shadcn InputGroup）

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 组合 | antd `Space.Compact`：`block`、`size`、`direction/orientation/vertical`（antd/space/Compact.d.ts，`direction` 已弃用） | entry_compact.go `Compact().Vertical().Block()` | 已对齐 |
| 尺寸 | antd Compact `size` 传给子控件（compactSize） | 无 | 已补齐（`CompactW.Size`，子控件自己的 Size 优先；TestCompactSize） |
| orientation | antd 6 `orientation` | `Vertical()` | 已补齐（`Orientation("vertical")`，与 Vertical 同义） |
| 文字/图标块 | antd `Space.Addon`（6.x）；EP `prepend/append`；shadcn `InputGroupAddon`、`InputGroupText` | `Addon(label).Icon()` | 已对齐 |
| 组内按钮 | shadcn `InputGroupButton`（size xs/sm/icon-xs/icon-sm） | `Addon(...).Clicked()`、Compact 里直接放 `Button` | 等价写法 |
| 组内前后缀 | shadcn `InputGroupAddon align="inline-start/inline-end"`：输入框内侧的图标/文字/kbd | 输入框 `Prefix/Suffix/PrefixText/SuffixText` | 等价写法 |
| 上下方的块 | shadcn `align="block-start/block-end"`（`flex-col`，textarea 上方/下方工具栏） | — | 暂不做（需要“框内纵向区块 + 共享焦点环”的容器，缺 paintFrame 外层的组合框能力；可用 Card/Column 近似） |
| 组内 textarea | shadcn `InputGroupTextarea` | `Compact` 只横向/纵向拼接 | 暂不做（同上） |
| 焦点环 | shadcn：组整体 `has-[…:focus-visible]:ring-[3px]` | 每个控件各自焦点环 | 暂不做（同上，需要容器级焦点环） |

### Form / FormItem（shadcn Field）

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 布局 | antd `layout: horizontal \| vertical \| inline`（antd/form/Form.d.ts）；EP `label-position: left \| right \| top`、`inline`；shadcn `Field orientation: vertical \| horizontal \| responsive`（shad/field.tsx） | form.go `Vertical/Inline`、`LabelAlign`；Defaults `"layout"` | 已对齐；shadcn `responsive` 等价写法（按 `ScreenBreakpoint` 切换 Vertical） |
| 单项布局 | antd FormItem `layout`（5.18+，antd/form/FormItem/index.d.ts） | 只能整表设置 | 暂不做（form.go 多处按整表布局计算标签宽与对齐，需重构为按项；shadcn 横向 Field 常见用法 = `NoLabel` 项里放带标签的 Checkbox/Switch） |
| 标签宽度 | antd `labelCol/wrapperCol`（栅格）；EP `label-width` | `LabelWidth(px)` | 等价写法（像素宽；栅格比例：暂不做） |
| 标签换行 | antd `labelWrap` | 单行，超出溢出 | 暂不做（label 测量为单行） |
| 冒号 | antd `colon`（默认 true）；EP `label-suffix` | `Colon`，Ant 默认开 | 已对齐（EP label-suffix 任意后缀：等价写法，写进标签文字） |
| 必填标记 | antd `requiredMark: boolean \| 'optional' \| render`；EP `hide-required-asterisk`、`require-asterisk-position: left \| right` | p2d_form.go `RequiredMark(bool)` | 已补齐（`RequiredMarkOptional()` 与 Defaults `"requiredMark": "optional"`：必填不显示星号，非必填标签后显示 `Locale().Form.Optional`，顺序 [文字][提示][可选][冒号]，颜色 colorTextDescription（antd/form/FormItemLabel.js、antd/form/style/index.js `.ant-form-item-optional`）；TestFormRequiredMarkOptional）。render 函数：不需要；星号放右侧：暂不做 |
| 提示 | antd FormItem `tooltip`；shadcn 无 | `Tooltip` | 已对齐 |
| 描述 / 额外 | antd `extra`、`help`；shadcn `FieldDescription`（text-sm muted-foreground） | `Extra/ExtraBody/Help` | 已对齐 |
| 错误 | antd `validateStatus`、`help`；EP `error`、`show-message`、`inline-message`；shadcn `FieldError`（text-sm text-destructive，多条时列表） | `Error/ValidateStatus`；Form 令牌 ErrorColor | 已对齐（多条错误列表：等价写法，Rule 只报第一条，和 antd 默认 `validateFirst` 外的展示不同——antd 默认显示全部错误，暂不做） |
| 状态图标 | antd `hasFeedback`；EP `status-icon` | — | 暂不做（需在输入框后缀区按状态画图标，要改 paintFrame 调用方的后缀测量，跨组） |
| 变体 | antd Form `variant`（传给内部控件） | — | 等价写法：`ConfigProvider.Defaults("Input"/"Select"/…, {"variant": …})` |
| 尺寸 / 禁用 | antd/EP `size`、`disabled` | `Size`、`Disabled` | 已对齐 |
| 校验 | antd `rules`、`validateTrigger`、`dependencies`、`validateFirst`；EP `rules`、`validate`、`validateField`、`clearValidate`、`resetFields`、`scrollToField` | `Rules/RulesFunc/AsyncCheck/ValidateTrigger/Dependencies/Validate/ValidateFields/ClearValidate/ResetFields/ScrollToFirstError` | 已对齐 |
| Form.List | antd `Form.List` | ProForm 的 FormList（不在本组） | 不需要（本组范围外，见 Pro 批） |
| 字段集 | shadcn `FieldSet`/`FieldLegend`/`FieldGroup`/`FieldSeparator`/`FieldTitle`/`FieldContent` | — | 等价写法：分组用 `Card`/`Title` + `Form`，分隔用 `Divider`（shadcn Separator = Divider）；不新增同义入口（设计 §7.1） |
| shadcn 标签 | shad/label.tsx `text-sm font-medium leading-none`，`gap-2`；出错时 label 变 destructive | Form 令牌 shadcn 分支 | 已对齐（原有） |
| a11y | 表单项标签为控件名、必填 Required、错误 Invalid+描述 | form.go `semantics` | 已对齐 |

### Checkbox / CheckboxGroup / CheckAll

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 单个 | antd `checked/indeterminate/disabled`（antd/checkbox/Checkbox.d.ts）；EP `indeterminate/border/size/true-value/false-value` | choice.go `Checkbox(c, label, &v).Indeterminate().Border().Size()` | 已对齐；EP true/false-value：等价写法（绑定 bool，应用自行映射） |
| 组 | antd `options[{label, value, disabled}]`；EP `min/max`、`type: checkbox \| button`、`fill/text-color` | `CheckboxGroup(...).Min/Max/Border/Buttons` | 已对齐；fill/text-color：等价写法（Checkbox 令牌 ButtonCheckedBg/Color 局部覆盖） |
| 全选 | antd 示例（indeterminate + 组） | entry_checkall.go `CheckAll` | 已对齐 |
| 令牌 | antd/checkbox/style/index.js：size controlInteractiveSize 16，borderRadiusSM 4；EP `$checkbox` input 14×14、radius border-radius-small 2；shadcn `size-4 rounded-[4px] shadow-xs` | theme.go CheckboxSize/Radius；comp_checkbox.go | 已对齐（TestAlignB1bAntTokens） |
| EP 带边框间距 | chalk/common/var.scss `$checkbox-bordered-padding-left/right` 10/16（默认），padding 写成「值 − 边框」 | BorderedPaddingStart/End 9/15 | 暂不做（Tango 从外缘量 9，EP 内容距外缘 10；差 1px 需截图核对后再改） |
| 键盘 / 禁用 / a11y | 空格切换；checkbox 角色、Checked/Mixed | 有 | 已对齐 |

### Radio / RadioGroup

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 组方向 | antd 6 `orientation`、`vertical`（antd/radio/interface.d.ts；style `&-vertical` 纵向 flex、rowGap marginXS 8，按钮上下相接、首尾圆角） | 只有横向 | 已补齐（`Vertical()`、`Orientation("vertical")`；按钮竖排等宽、共享边框；a11y Vertical；TestRadioGroupVerticalA11y） |
| 撑满 | antd `block`（group `display:flex`，选项 `flex:1; justifyContent:center`，radio/style/index.js:138-141） | — | 已补齐（`Block()`：按钮平分，普通单选在自己那份里居中；TestRadioGroupBlock、TestRadioDarkDotAndBlockCentre） |
| 按钮样式 | antd `optionType: button`、`buttonStyle: outline \| solid`；EP `el-radio-button`、`fill/text-color` | `Buttons()`、`Solid()`；Defaults | 已对齐 |
| 带边框 | EP `border` | `Border` | 已对齐 |
| 圆点尺寸 | antd/radio/style/index.js prepareComponentToken：radioSize fontSizeLG 16，dotSize 16−(4+1)×2 = 6；禁用选中点同尺寸、dotColorDisabled | Ant DotSize 8；禁用选中点固定 8px | 已补齐（Ant DotSize 6，禁用点用 DotSize；TestRadioDotSize、TestAlignB1bAntTokens）。见 CHANGELOG「变更」 |
| 选中内点颜色 | antd 非 wireframe `radioColor: colorWhite`（radio/style/index.js:499）；EP `::after` `--el-color-white`（chalk/radio.scss:130） | 用 BgContainer（暗色下是深色点） | 已补齐（令牌 `DotInnerColor` 白色，亮色外观不变；TestRadioDarkDotAndBlockCentre） |
| 选中悬停 | antd `&:hover .ant-radio-checked { background: colorPrimaryHover }` | 选中后悬停不变色 | 已补齐（令牌 `DotHoverColor`，Ant = Primary.Hover，其它主题 = Primary.Base 不变；用 `ConfigProvider.Components` 覆盖 `DotColor` 时要同时覆盖 `DotHoverColor`，零值字段继承主题值） |
| 键盘 | 方向键在组内移动并选中、漫游 Tab | `rovingStep(rovingBoth)` | 已对齐（竖向也可用上下键，TestRadioGroupVerticalA11y） |
| shadcn | shad/radio-group.tsx：`grid gap-3`，项 `size-4 rounded-full border-input shadow-xs`，指示点 `size-2 fill-primary` | DotSize 8、shadcn 焦点环 | 已对齐（shadcn 组默认 gap-3 = 12，Tango 8：暂不做，需新增组间距令牌，留给 Radio 令牌下一轮） |

### Switch

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 尺寸 | antd `size: small \| default`（antd/switch/index.d.ts）；EP `large/default/small`；shadcn v4 `size: sm \| default`（shad/switch.tsx） | `Small/Large/Size` | 已对齐（Large 来自 EP） |
| 内嵌文字 / 图标 | antd `checkedChildren/unCheckedChildren`；EP `inline-prompt` + `active-text`/`active-icon` | `Texts`、`Icons` | 已对齐 |
| 外侧文字 | EP `active-text/inactive-text`（非 inline-prompt） | `OuterTexts` | 已对齐 |
| 滑块图标 | EP `active-action-icon/inactive-action-icon`（ep/switch/src/switch.d.ts）；颜色 on: `--el-switch-on-color`，off: `--el-switch-off-color`（chalk/switch.scss `__action`） | 无 | 已补齐（`ActionIcons(on, off)`；TestSwitchActionIcons） |
| 任意值 | EP `active-value/inactive-value` | `SwitchValue` | 已对齐 |
| 切换前确认 | EP `before-change: () => Promise<boolean> \| boolean` | `BeforeChange`、`BeforeChangeAsync` | 已对齐 |
| 加载 | antd/EP `loading` | `Loading` | 已对齐 |
| 宽度 | EP `width` | `Width` | 已对齐 |
| 令牌 | antd/switch/style/index.js：trackHeight 22、trackHeightSM 16、handleSize 18 / SM 12、trackMinWidth 44 / SM 28、padding 2、handleShadow `0 2px 4px 0 rgba(0,35,11,.2)` | comp_switch.go | 已对齐（TestAlignB1bAntTokens） |
| EP / shadcn 令牌 | EP 40×20、按钮 16；shadcn `h-[1.15rem] w-8`、thumb `size-4`、sm `size-3` | theme.go / shadcn.go / comp_switch.go | 已对齐（原有） |
| a11y | switch 角色、Checked | `semantics` | 已对齐 |

### Slider / SliderRange

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 范围 | antd `range: boolean \| {draggableTrack, editable, minCount, maxCount}`（antd/slider/index.d.ts）；EP `range` | p2b_slider.go `SliderRange`、`AllowCross`、`Pushable` | 已对齐；`draggableTrack`（拖动轨道整体平移）、`editable`（增删手柄）：暂不做（交互较大，留待后续） |
| 刻度 / 点 | antd `marks`、`dots`；EP `marks`、`show-stops` | `Marks`、`Dots` | 已对齐 |
| 反向 | antd `reverse` | 无 | 已补齐（`Reverse()`，Defaults `"reverse"`；方向键随视觉方向；TestSliderReverse） |
| 不包含 | antd `included`（默认 true） | 总填充 | 已补齐（`Included(false)`，Defaults `"included"`；TestSliderIncludedFalse） |
| 完成事件 | antd `onChangeComplete`（`onAfterChange` 已弃用） | 只有 `Changed` | 已补齐（`ChangeComplete()`：拖动松手、键盘、数字框；TestSliderChangeComplete） |
| 键盘开关 | antd `keyboard` | 总可用 | 已补齐（`Keyboard(false)`；TestSliderChangeComplete） |
| 竖向 | antd 6 `orientation`/`vertical`；EP `vertical` + `height` | `Vertical(height)` | 已对齐（`orientation` 字符串写法：等价写法，Vertical） |
| 提示 | antd `tooltip {open, placement, formatter}`；EP `show-tooltip`、`format-tooltip`、`placement`、`persistent` | `Tooltip(fmt)`、`TooltipOpen(bool)` | 已对齐；`placement`：暂不做（固定上方 / 竖向时在侧边） |
| 输入框 | EP `show-input`、`show-input-controls`、`input-size` | `WithInput` | 已对齐（`show-input-controls`/`input-size`：暂不做，低频） |
| 无障碍文案 | EP `range-start-label`、`range-end-label`、`format-value-text`；antd `ariaLabelForHandle`、`ariaValueTextFormatterForHandle` | 手柄名 `Locale().Slider.Min/Max`，值文本取 Tooltip 格式 | 已对齐 |
| 令牌 | antd/slider/style/index.js prepareComponentToken：handleSize controlHeightLG/4 = 10、handleSizeHover controlHeightSM/2 = 12、handleLineWidth 2、**handleLineWidthHover lineWidth+1.5 = 2.5**、railSize 4、dotSize 8、**trackBgDisabled colorBgContainerDisabled**、handleActiveOutlineColor colorPrimary@0.2，hover/active/focus 时 `outline: 6px` | comp_slider.go：HandleLineWidthHover 4、TrackBgDisabled TextDisabled；p2b_slider.go 光晕只在键盘焦点时画、宽 4 且在描边外 | 已补齐（HandleLineWidthHover 2.5、TrackBgDisabled = BgContainerDisabled、新令牌 `HandleActiveOutlineWidth` 6；光晕在悬停/按下/聚焦时画，从手柄边缘向外 6px；TestAlignB1bAntTokens）。见 CHANGELOG「变更」 |
| EP 令牌 | chalk/common/var.scss `$slider`：height 6、button-size 20、**disabled-color --el-text-color-placeholder** | Element 禁用色 TextDisabled（暗色下与 placeholder 不同） | 已补齐（TrackBgDisabled、HandleColorDisabled = TextQuaternary；TestAlignB1bElementTokens） |
| shadcn | shad/slider.tsx：thumb `size-4 border-primary bg-white shadow-sm`，`hover:ring-4 focus-visible:ring-4 ring-ring/50`，`disabled:opacity-50`；track `bg-muted`，range `bg-primary` | p2b_slider.go shadcn 分支 | 已对齐（ring 宽改为令牌 4；TestAlignB1bShadcnTokens） |
| 禁用 | antd/EP `disabled` | `Disabled`；ChangeComplete 不报告 | 已补齐测试（TestSliderChangeComplete） |

### Upload / DropZone

| 规范项 | 官方定义（来源，2026-11-14） | Tango 现状 | 结论 |
|---|---|---|---|
| 列表类型 | antd `listType: text \| picture \| picture-card \| picture-circle`；EP `list-type: text \| picture \| picture-card` | entry_upload.go `ListType/PictureList/PictureCard`，Defaults `"listType"` | 已对齐（picture-circle 按 picture-card 显示：暂不做圆形卡片） |
| 拖拽 | antd `Upload.Dragger`；EP `drag` | `Dragger()`、p2e_drop.go `DropZone` | 已对齐 |
| 数量上限 | antd `maxCount`：**为 1 时新文件替换旧文件**，>1 时截断（antd/upload/Upload.js:89）；EP `limit` + `on-exceed` | `MaxCount`：到上限后禁用触发器并拒绝 | 已补齐（`MaxCount(1)` 改为替换、触发器保持可用；>1 仍拒绝并在 `Rejections()` 里给出 = EP on-exceed；TestUploadMaxCountOneReplaces）。见 CHANGELOG「变更」 |
| 显示列表 | antd `showUploadList: boolean \| {…}`；EP `show-file-list` | 总显示 | 已补齐（`ShowUploadList(false)`，Defaults `"showUploadList"`；TestUploadShowUploadListFalse）。逐个图标开关（showRemoveIcon 等）：暂不做 |
| 上传请求 | antd `action/headers/data/method/withCredentials/customRequest`；EP `action/headers/data/method/http-request/auto-upload` | `Action/Headers/Data/Method/FieldName/CustomRequest/AutoUpload/Submit/Retries/Client` | 已对齐（withCredentials：等价写法，`Client` 自带 cookie jar） |
| 过滤 | antd `accept`、`beforeUpload`；EP `accept`、`before-upload`、`before-remove` | `Accept/MaxSize/BeforeUpload` | 已对齐；`before-remove`：暂不做（删除前确认，可在 `Removed()` 后由应用撤销） |
| 多选 / 目录 | antd/EP `multiple`、`directory` | `Multiple/Directory` | 已对齐 |
| 预览 / 下载 | antd `onPreview/onDownload/previewFile/isImageUrl/iconRender/itemRender` | `Previewed()`；缩略图由宿主解码 | 已对齐预览；下载与 itemRender：暂不做 |
| 进度 | antd `progress` 配置 | 内置进度条（Upload 令牌 ProgressColor） | 已对齐 |
| 禁用 | antd/EP `disabled` | `Disabled`；Defaults | 已对齐 |
| shadcn | 无对应组件 | 自定义：边框行、虚线拖放区 | 不需要（无官方定义） |

---

## gallery

新页 `examples/ui-gallery/page_b1b.go`：`InputNumberAlign`（Parent InputNumber）、`OTPAlign`（OTP）、`RadioAlign`（Radio）、`SwitchAlign`（Switch，含 Slider）、`FormAlign`（Form，含 Upload）。

```
go run ./examples/ui-gallery -only <页名> -theme all -out /tmp/spec-align-b1/B
```

五页三套主题 × 明暗均已出图；看过 InputNumberAlign（Ant）、OTPAlign（shadcn 暗）、RadioAlign（Ant）、SwitchAlign（Element）、FormAlign（Ant、Ant 暗），无错位、无低对比度。

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 容器级输入框外壳（框内纵向区块 + 子控件共享焦点环） | shadcn InputGroup（block-start/block-end、InputGroupTextarea） | paintFrame 之外的“组合框”外壳：子编辑器聚焦时由容器画边框与焦点环 | 暂不做，等用户决定 |
| 拖拽改尺寸手柄 | TextArea（EP `resize`、浏览器默认） | 通用的拖动改尺寸（宽/高）手柄与指针接管 | 暂不做 |
| 按字素计数 | Input / TextArea（antd `count.strategy`、EP `count-graphemes`） | text 包的字素簇切分 | 暂不做 |
| 输入格式化时的光标映射 | Input（EP `formatter/parser`） | editState 在显示文本与原值之间映射光标/选区 | 暂不做 |

## CHANGELOG 草稿

### 新增

- InputNumber：`Mode("input" | "spinner")`（antd 6 mode，Defaults `"mode"`）、`Controls(bool)`（Defaults `"controls"`）、`ReadOnly()`、`Keyboard(bool)`（Defaults `"keyboard"`）、`StepStrictly()`（Element step-strictly）、`UpDisabled()` / `DownDisabled()`；令牌 `SideHandleWidth` / `SideHandleWidthSM` / `SideHandleWidthLG`。
- OTP：`Separator(s)`、`SeparatorFunc(func(i int) string)`、`Variant(v)`（Defaults `"OTP"` 键，回落 `"Input"`）、`ReadOnly()`。
- Search：`Variant(v)`（Defaults `"Search"` 键，回落 `"Input"`）。
- Input：`VisibilityToggle(bool)`（antd Input.Password visibilityToggle）。
- TextArea：`Variant(v)`（Defaults 只读 `"TextArea"` 键，和 antd 一样不回落 Input）、`ReadOnly()`；令牌 `Input.TextAreaMinHeight`（shadcn 64 = min-h-16）。
- Compact：`Size(s)`（antd Space.Compact size）、`Orientation(o)`。
- Form：`RequiredMarkOptional()`（antd requiredMark="optional"，Defaults `"requiredMark": "optional"`）；令牌 `Form.LabelOptionalColor`；文案 `Locale.Form.Optional`。
- RadioGroup：`Vertical()`、`Orientation(o)`、`Block()`；令牌 `Radio.DotHoverColor`、`Radio.DotInnerColor`。
- InputNumber：令牌 `HideControlsInactive`。
- Switch：`ActionIcons(on, off)`（Element active-action-icon / inactive-action-icon）。
- Slider：`Reverse()`（Defaults `"reverse"`）、`Included(bool)`（Defaults `"included"`）、`Keyboard(bool)`、`ChangeComplete()`（antd onChangeComplete）；令牌 `Slider.HandleActiveOutlineWidth`。
- Upload：`ShowUploadList(bool)`（Defaults `"showUploadList"`）。

### 变更

- InputNumber（Ant 主题）：`ControlsPosition("sides")` / `Mode("spinner")` 的按钮由 FillTertiary 底、TextSecondary 色、宽 = 控件高 → 透明底、colorText、宽 37 / 29（小）/ 39（大）（antd 6 spinner 样式：图标字号 + 两侧 paddingInline + 1px 分隔线）。
- InputNumber（Element 主题）：两侧按钮图标 14px → 13px（theme-chalk input-number.scss `font-size: 13px`）。
- InputNumber：步进按钮到达 Min/Max 时节点禁用（光标 not-allowed，点击无效），原来只变灰。
- InputNumber（Ant、shadcn 主题）：禁用或只读时不显示步进按钮（antd mergedControls），spinner 数值保持居中；原来 spinner 禁用仍显示灰按钮、shadcn 禁用显示淡化按钮。Element 主题禁用 / 只读保留按钮（不响应）。
- Radio（Ant 主题）：选中圆点直径 8 → 6（antd radio dotSize = 16 − (4 + 1) × 2）；禁用选中点直径 8 → 6；选中后悬停填充 colorPrimary → colorPrimaryHover；暗色下选中内点由背景色（#141414）→ 白色（radioColor colorWhite）。
- Radio（Element 主题）：禁用选中点直径 8 → 4（与正常态同尺寸）；暗色下选中内点由背景色 → 白色（--el-color-white）。
- Slider（Ant 主题）：hover 手柄描边 4 → 2.5（handleLineWidthHover = lineWidth + 1.5）；禁用轨道色 colorTextDisabled → colorBgContainerDisabled（trackBgDisabled）；手柄光晕从“仅键盘焦点、描边外 4px” → “悬停 / 按下 / 聚焦时、手柄边缘外 6px”（outline 6px handleActiveOutlineColor）。
- Slider（Ant 主题）：禁用时悬停不再放大手柄、不再换 railHoverBg（antd `:not(-handle-disabled)`、`railBg !important`）。
- Slider：竖向 + Reverse 时 Right 减小、Left 增大（rc-slider "ttb"）。
- Slider（Element 主题）：禁用色 TextDisabled → TextQuaternary（--el-text-color-placeholder；亮色不变，暗色 #6c6e72 → #8d9095）。
- TextArea：默认行数 3 → 2（antd 原生 textarea 默认 rows、Element `rows` 默认 2）；shadcn 主题下框高至少 64（new-york-v4 `min-h-16`）。
- Upload：`MaxCount(1)` 由“到上限禁用并拒绝新文件” → “新文件替换旧文件，触发器保持可用”（antd maxCount=1）；`MaxCount(n>1)` 行为不变。

## 共享文件改动请求

1. **`ui/g2_tokens_test.go` TestG2SliderTokenDefaults**（因规范修正失效）：Ant 分支 `lwHov` 4 → 2.5；`k.TrackBgDisabled != th.TextDisabled` 改为按主题：Ant `th.BgContainerDisabled`、Element `th.TextQuaternary`、shadcn `th.TextDisabled`；`k.HandleColorDisabled` Element 改为 `th.TextQuaternary`（Ant / shadcn 仍 `th.TextDisabled`）。
2. **`ui/p2d_input_test.go` TestP2DInputNumberSidesWheel**（因规范修正失效）：`up.W != br.H` 改为期望 `th.Components.InputNumber.SideHandleWidth`（为 0 时 = `br.H`）；Ant 下是 37。
3. **`ui/t1b_tokens_test.go` InputNumber 用例**：Ant 的 `SideHandleBg` 现在是透明（不绘制），“默认值在作用域外被绘制”断言失败；把该用例的令牌换成 `SideHandleColor`（`theme.InputNumberToken{SideHandleColor: t1bMark}`，取值函数同步改）。
4. **`ui/i18n_wiring_test.go` TestI18nPaintedTexts**：加 `Form.Optional`（只在 label 的 paint 里画；用 `Form(c).RequiredMarkOptional()` + 一个非必填项构建）。
5. **Locale（已按例外直接改）**：`ui/locale.go` `FormLocale` 新增字段 `Optional`；`ui/locale_packs.go` 四个包各补一项：ZhCN「（可选）」、EnUS「(optional)」、ZhTW「（選填）」、Ja「(オプション)」（取自 antd/locale 的 `Form.optional`）。
6. **`CHANGELOG.md`**：合并上面的「CHANGELOG 草稿」到 `## [未发布]`。
7. **`api/**`**：重新生成（新增方法、令牌字段、Locale 字段见 CHANGELOG「新增」；全部是新增，没有删除或改签名）。
8. **`ui/AUTHORING.md`**：§14 不需要改（没有新组件）；§5「选择控件」一句可补 `Radio.DotHoverColor`、`Slider.HandleActiveOutlineWidth`（可选）。
9. **`ui/golden_test.go` / `ui/testdata`**：本组没有改黄金页上的组件外观（黄金页里的 Input、Checkbox、Switch 像素不变；TestGolden 当前失败来自 Button 两字加空格，组 A）。
10. **`ui/p2d_form.go`（主线程授权，审阅修复 M1）**：只改 `labelTip`，新增参数 `extra float32`（[文字][提示][可选][冒号] 里提示之后、冒号之前的宽度）计入端对齐的总宽；form.go 不再事后挪动提示节点。
