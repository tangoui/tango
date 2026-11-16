# 第 1 批 · 组 D：日期时间与颜色

核对日期：2026-11-14。参考源码（`/tmp/tango-ref`）：

- antd@6.7.0：`antd/package/es/date-picker`（`generatePicker/interface.d.ts`、`style/token.js`、`style/panel.js`）、`es/time-picker/index.d.ts`、`es/color-picker`（`interface.d.ts`、`ColorPicker.js`、`style/index.js`）。
- rc-picker：antd 依赖 `@rc-component/picker ~1.17.0`，本次 `npm pack @rc-component/picker@1.17.0` 解到 `/tmp/tango-ref/rc-picker/package`（`lib/interface.d.ts`、`lib/PickerInput/{SinglePicker,RangePicker}.d.ts`、`lib/PickerInput/hooks/{useFilledProps,useShowNow,useDisabledBoundary,useRangeValue}.js`、`lib/PickerInput/Popup/Footer.js`、`lib/PickerPanel/PanelHeader.js`）。`@rc-component/color-picker@3.1.1` 解到 `/tmp/tango-ref/rc-color`（只作参考）。
- element-plus@2.14.8：`ep/package/es/components/{date-picker-panel,time-select,color-picker,color-picker-panel}`、`ep/package/theme-chalk/src/date-picker/*.scss`。
- shadcn/ui v4（new-york-v4）：`registry/new-york-v4/ui/calendar.tsx`（shadcn 没有时间选择器、颜色选择器；DatePicker 是 Popover + Calendar 的组合示例）。

下文“antd”指 antd@6.7.0（含 rc-picker 1.17.0），“EP”指 element-plus@2.14.8。Tango 现状写 `文件:符号`。

## DatePicker

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 粒度 picker | antd `picker: date/week/month/quarter/year`（rc-picker interface.d.ts `picker?: PickerMode`）；EP `type` year/month/week/date | date.go:`Picker`，`PickerType` | 已对齐 |
| 多选 multiple | antd `multiple`（SinglePicker.d.ts）；EP `type=dates/months/years` | date_multi.go:`MultiDatePicker`（含 `Picker`、`MaxTagCount`、`NeedConfirm`） | 等价写法：`ui.MultiDatePicker(c, &[]time.Time)`，单独入口因为值类型不同（切片） |
| 值与受控 | antd `value/defaultValue/onChange`；EP `v-model` | date.go:`DatePicker(c, *time.Time)`、`Changed()` | 已对齐（值由应用持有） |
| 格式 format | antd `format`（字符串 / 数组 / `{format,type:'mask'}`）；EP `format`、`value-format` | date.go:`Format`（Go layout） | 已对齐（单个格式）；数组与 mask 写法暂不做（缺多格式解析，需求不明确）；`value-format` 不需要（值是 `time.Time`） |
| 占位 placeholder | antd `placeholder`；EP `placeholder` | date.go:`Placeholder`，按粒度取 Locale | 已对齐 |
| allowClear | antd 默认 `true`（useFilledProps `allowClear ?? true`）；EP `clearable` 默认 true | date.go:`AllowClear`，默认开 | 已对齐；`allowClear.clearIcon` 不需要（图标随主题） |
| disabledDate | antd `disabledDate`；EP `disabled-date` | date.go:`DisabledDate` | 已对齐 |
| minDate / maxDate | antd `minDate/maxDate`：范围外按面板粒度禁用，且限制面板翻页（rc-picker interface.d.ts:267-269；useDisabledBoundary.js 按 `info.type` 比较；PanelHeader.js `disabledOffsetPrev/Next`） | — | 已补齐：date.go:`MinDate/MaxDate`，date_util.go:`dtBounds`；越界日期不可点、不可键入、键盘 Enter 无效；表头箭头禁用（Ant 透明度 0.25，panel.js header `button:disabled`；shadcn 0.5；Element 文字色 `--el-text-color-disabled`）（TestDatePickerMinMaxDate、TestDatePickerMinDateMonthPicker、TestDatePickerMinDateOutOfRangeValue、TestDatePickerNavOffA11y） |
| needConfirm | antd：日期默认不需确认，time/datetime 默认需确认（useFilledProps.js:79 `needConfirm ?? multipleInteractivePicker`） | date.go:`NeedConfirm`，默认关；新增 Defaults 键 `"needConfirm"` | 已对齐 |
| showNow | antd：单选 date/time 默认显示（useShowNow.js），日期面板文字为“今天”，`disabledDate(now)` 时按钮禁用（Footer.js） | date.go：始终显示“今天” | 已补齐：`ShowNow(bool)`、Defaults `"showNow"`；今天被禁用（含 MinDate/MaxDate）时链接置灰（TestDatePickerShowNowShowWeek） |
| showWeek | antd `showWeek`（interface.d.ts:239）；EP `show-week-number`（date-picker-panel props/shared.mjs） | 只有周选择器显示周数 | 已补齐：`ShowWeek(bool)`、Defaults `"showWeek"`（TestDatePickerShowNowShowWeek）。已知偏差（周选择器原有）：Tango 加一列使面板变宽 36px；antd 宽度仍为 288，body 内边距改为 8 12，8 列平分（panel.js:402-405）。暂不做，需加“有周数列时的内边距”令牌 |
| presets | antd `presets: {label,value}[]`；EP `shortcuts` | date.go:`Presets(DatePreset...)` | 已对齐；antd `value` 为函数的写法等价（Go 每帧计算） |
| placement | antd `placement: bottomLeft/bottomRight/topLeft/topRight`（generatePicker/interface.d.ts `_DataPickerPlacements`）；EP `placement`（默认 bottom） | 固定 BottomStart | 已补齐：`Placement(ui.Placement)`、Defaults `"placement"` 接受 antd 名字（TestDatePickerPlacementTop、TestB1DPlacementDefaults）。等价写法说明：名字按逻辑方向理解（Left = Start），RTL 下镜像；rc-picker 的默认值也按 rtl 镜像，但 antd 显式传 `bottomLeft` 时不镜像 |
| variant | antd `variant` 默认 `outlined`，含 `underlined` | date.go:`Variant` | 已对齐 |
| status / size | antd `status`、`size`；EP `size` | date.go:`Status/Size` | 已对齐 |
| open / onOpenChange / defaultOpen | antd | date.go:`Open()`、`IsOpen()` | 等价写法：`Open()` 强制打开、`IsOpen()` 查询 |
| mode / onPanelChange、pickerValue / defaultPickerValue | antd SinglePicker.d.ts | DatePickerPanel 有只读 `View()` | 暂不做（需要受控视图 API，先无需求） |
| cellRender / panelRender / renderExtraFooter / components | antd | — | 暂不做（需单元格与面板自定义渲染钩子，API 待设计） |
| prefix / suffix(Icon) | antd `prefix`、`suffix`；EP `prefix-icon/clear-icon` | 固定日历图标 | 暂不做（需输入外壳的前后缀图标入口，与组 B 的 Input 前后缀一并设计） |
| inputReadOnly | antd `inputReadOnly`；EP `editable=false` | 可键入 | 暂不做（键入逻辑 `dtTypeInput` 在 entry_datepanel.go，不在本组文件，见共享文件请求） |
| preserveInvalidOnBlur / previewValue | antd | 失焦丢弃无效文本；悬停不预览 | 暂不做（同上，entry_datepanel.go） |
| readonly | EP `readonly` | — | 暂不做（与 inputReadOnly 一并） |
| onOk / onCalendarChange | antd | `Changed()` | 等价写法 |
| getPopupContainer / classNames / styles / popupClassName | antd | — | 不需要（自绘无 DOM；外观走令牌） |
| locale | antd `locale` | `ConfigProvider.Locale` | 等价写法 |
| 键盘 | rc-picker：方向键移动、PageUp/PageDown 翻月、Enter 选择、Esc 关闭 | date.go `finish`（含 Ctrl/Cmd+↑ 缩放视图、可键入） | 已对齐 |
| 禁用 | antd `disabled` | date.go:`Disabled` | 已对齐（新选项在禁用时无效：TestDatePickerDisabledNewOptions） |
| 尺寸与令牌 | antd token.js：`cellWidth = controlHeightSM*1.5 = 36`，`cellHeight = controlHeightSM = 24`（内方块），`pickerCellPaddingVertical = paddingXXS*1.5 = 6`，panel.js 中行高 = `cellHeight + 2*pickerCellPaddingVertical = 36`；`textHeight = controlHeightLG = 40`；`pickerDatePanelPaddingHorizontal = 18`；`pickerYearMonthCellWidth = 60`；`presetsWidth 120`（`presetsMaxWidth 200`） | theme/comp_datepicker.go：CellHeight 32、PresetsWidth 110 | 已补齐（规范修正）：Ant `CellHeight 32 → 36`、`PresetsWidth 110 → 120`；Element 预设栏取 `el-picker-panel__sidebar` 110px（picker-panel.scss）；shadcn 单元格保持 32（calendar.tsx `[--cell-size:--spacing(8)]`），预设栏随基值为 120（shadcn 无预设规范）（TestAlignB1DDatePickerTokens）；`presetsMaxWidth` 随内容伸缩暂不做（预设栏定宽） |
| Element 外观 | EP date-table.scss：单元格 30px 高、24px 圆形文字 | theme/comp_datepicker.go Element 分支 40×36、圆角 18 | 已对齐（沿用既有令牌，本次未改） |
| shadcn 外观 | calendar.tsx：`size-(--cell-size)`、`rounded-md`、今日/范围 `bg-accent`、外月 `text-muted-foreground`、`aria-disabled:opacity-50` | Element/shadcn 令牌既有；新增 `NavDisabledOpacity 0.5` | 已对齐 |
| 明暗主题 | 令牌 | 全部取令牌，不判断 `th.Dark` | 已对齐（gallery `-theme all` 已出图） |

## DatePickerPanel

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 粒度与范围 | EP `el-date-picker-panel` `type`、range；antd 无独立面板（rc-picker PickerPanel） | fill_date_panel.go:`Picker`、`Range` | 已对齐 |
| minDate / maxDate | rc-picker PickerPanel `minDate/maxDate`（interface.d.ts:185-186）；EP date-picker-panel props/shared.mjs `minDate/maxDate` | — | 已补齐：`MinDate/MaxDate`（TestDatePickerPanelMinMax） |
| 周数 | EP `show-week-number`；rc-picker `showWeek` | — | 已补齐：`ShowWeek(bool)` |
| border / disabled | EP `border`、`disabled` | `Border`、`Disabled` | 已对齐 |
| unlink-panels | EP panelRangeSharedProps `unlinkPanels` | 两个面板联动 | 暂不做（范围面板需要两个独立视图状态，改动 RangePicker 与面板引擎较大） |
| show-now / show-footer / show-confirm / shortcuts | EP panelSharedProps | 面板无页脚、无快捷栏 | 暂不做（内嵌面板无页脚；DatePicker 弹层已有） |
| cell-class-name | EP | — | 暂不做（同 cellRender） |

## RangePicker

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 粒度 / showTime | antd `picker`、`showTime`；EP `type=daterange/monthrange/yearrange/datetimerange` | entry_daterange.go:`Picker`、`ShowTime` | 已对齐 |
| order | antd `order` 默认 true（useFilledProps.js:46）；false 时不排序（useRangeValue.js:146,180），showTime 同样适用 | 始终排序 | 已补齐：`Order(bool)`、Defaults `"order"`，日 / 周月季年 / ShowTime 三条路径都生效（TestRangePickerOrderFalse、TestRangePickerOrderFalseShowTime） |
| separator | antd `separator`（RangePicker.d.ts:12）；EP `range-separator` | 只有 Defaults `"separator"` | 已补齐：`Separator(s)`（TestRangePickerMinMaxSeparator）；shadcn 主题按结构画箭头，不画分隔文字（既有） |
| minDate / maxDate | 同 DatePicker | — | 已补齐：`MinDate/MaxDate`，两种面板（日 / 周月季年）都生效（TestRangePickerMinMaxSeparator） |
| placement | 同 DatePicker | 固定 BottomStart | 已补齐：`Placement`、Defaults `"placement"` |
| presets | antd `presets: {label, value:[a,b]}`；EP `shortcuts` | `Presets` | 已对齐 |
| disabledTime | antd `disabledTime(date, type)` | `DisabledTime(fn(day, end))` | 已对齐 |
| allowEmpty / disabled [bool,bool] | antd RangePicker.d.ts:44-45 | — | 暂不做（单端禁用需要分段交互重做） |
| unlink-panels | EP | 联动 | 暂不做（同 DatePickerPanel） |
| default-time（区间） | EP `default-time` 数组；antd `showTime.defaultValue` | — | 暂不做（DateTimePicker 已补；RangePicker ShowTime 待需求） |
| showNow | antd：范围选择默认不显示（useShowNow.js `!rangePicker`） | 范围不显示 | 已对齐 |
| needConfirm | antd：showTime 时默认需要确认 | ShowTime 时 OK 提交 | 已对齐 |
| 键盘 / 禁用 / 状态 / variant / size | antd | entry_daterange.go | 已对齐 |

## DateTimePicker

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 入口 | antd `DatePicker showTime`；EP `type=datetime` | entry_datetime.go:`DateTimePicker` | 等价写法：独立入口（AUTHORING §14） |
| minDate / maxDate | antd | — | 已补齐：`MinDate/MaxDate`（TestDateTimePickerDefaultTimeMinDate） |
| showNow | antd 默认显示“此刻” | 始终显示 | 已补齐：`ShowNow(bool)`、Defaults `"showNow"` |
| 默认时间 | EP `default-time`；antd `showTime.defaultValue` | 选日期且没有时间时为 00:00:00 | 已补齐：`DefaultTime(t)`（TestDateTimePickerDefaultTimeMinDate） |
| placement | antd | 固定 | 已补齐：`Placement`、Defaults `"placement"` |
| disabledDate / disabledTime / presets / 步长 | antd | `DisabledDate`、`DisabledTime`、`Presets`、`Steps` | 已对齐 |
| needConfirm=false | antd：datetime 默认需要确认，可关闭 | 始终需要“确定” | 暂不做（日期与时间列都要即时提交，需重排草稿逻辑；先无需求） |

## TimePicker / TimePanel

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 步长 / 12 小时 / 隐藏禁用项 | antd `hourStep/minuteStep/secondStep`、`use12Hours`、`hideDisabledOptions`（rc-picker interface.d.ts:122-133） | entry_time.go | 已对齐 |
| disabledTime | antd `disabledTime()` 返回 `disabledHours/Minutes/Seconds` | `DisabledHours/Minutes/Seconds` | 等价写法 |
| needConfirm | antd：time 默认需要确认（useFilledProps.js:79），false 时点选即生效 | 始终需要“确定” | 已补齐：`NeedConfirm(bool)`、Defaults `"needConfirm"`；关闭时列点击和方向键即时写值、面板保持打开、不显示确定（TestTimePickerNeedConfirmFalse） |
| showNow | antd 默认显示“此刻” | 始终显示 | 已补齐：`ShowNow(bool)`；“此刻”与确定都不显示时省去页脚（rc-picker Footer 返回 null）（TestTimePickerShowNowFalseDisabled） |
| placement | antd | 固定 | 已补齐：`Placement`、Defaults `"placement"` |
| changeOnScroll | antd `changeOnScroll`（interface.d.ts:147） | — | 暂不做，缺：滚动容器的“滚动停止 / 吸附”通知（基础能力缺口） |
| showMillisecond / millisecondStep | antd | — | 暂不做（无毫秒列，需求不明确） |
| renderExtraFooter / addon | antd | — | 暂不做（同 DatePicker） |
| arrow-control | EP `arrow-control` | — | 暂不做（另一种列交互，先无需求） |
| 禁用 | antd `disabled` | `Disabled` | 已对齐（TestTimePickerShowNowFalseDisabled） |
| 令牌 | antd token.js `timeColumnWidth = controlHeightLG*1.4 = 56`、`timeColumnHeight = 28*8 = 224`、`timeCellHeight = 28` | theme/comp_datepicker.go | 已对齐（TestAlignB1DDatePickerTokens） |
| TimePanel | antd 无独立时间面板；EP time-picker 面板 | entry_time.go:`TimePanel` | 已对齐（沿用） |

## TimeRangePicker

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| order | antd `order` 默认 true，false 不排序 | 始终按时钟排序 | 已补齐：`Order(bool)`、Defaults `"order"`（跨夜 22:00–06:00）（TestTimeRangePickerOrderFalse） |
| separator | antd `separator`；EP `range-separator` | 只有 Defaults | 已补齐：`Separator(s)`（TestTimeRangePickerOrderFalse 断言无障碍名与值） |
| placement | antd | 固定 | 已补齐：`Placement`、Defaults `"placement"` |
| 其余（步长、12 小时、禁用项、variant、键入） | antd | p2a_time_range.go | 已对齐 |

## TimeSelect

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| start / end / step / min-time / max-time | EP time-select.mjs（默认 09:00、18:00、00:30） | more_timeselect.go | 已对齐 |
| include-end-time | EP `includeEndTime`：步长跳过 end 时把 end 追加为最后一项（time-select.vue…mjs:67） | — | 已补齐：`IncludeEndTime(bool)`、Defaults `"includeEndTime"`（TestTimeSelectIncludeEndTime） |
| placement | EP（继承 el-select） | 固定 | 已补齐：`Placement`、Defaults `"placement"` |
| format | EP `format` 默认 `HH:mm`，选项与 v-model 都是格式化后的字符串 | 值固定 `"HH:MM"` | 暂不做（改变值语义需用户决定） |
| editable / prefix-icon / clear-icon / effect | EP | — | 暂不做 / 不需要（effect 是 tooltip 主题，自绘无此概念） |
| clearable / size / disabled | EP | `Clearable`、`Size`、`Disabled` | 已对齐 |

## ColorPicker / ColorPickerPanel

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| mode single / gradient | antd `mode: 'single' \| 'gradient' \| 数组` | entry_color.go:`ColorPicker`、`ColorPickerGradient` | 等价写法（单一模式各自入口）；`mode` 数组（面板内切换）暂不做 |
| trigger | antd 默认 `click`（ColorPicker.js:35），可 `hover` | `Trigger` | 已对齐 |
| placement | antd 默认 `bottomLeft`（ColorPicker.js:38）；EP 继承 tooltip | 固定 BottomStart | 已补齐：`Placement`、Defaults `"placement"`（TestColorPickerPlacementTop） |
| format / defaultFormat | antd `hex/rgb/hsb`；EP `color-format`（tinycolor，含 hsl、hsv） | `Format`（hex/rgb/hsb/hsv）、Defaults `"format"` | 已对齐（antd）；EP `hsl` 暂不做，缺 theme HSL 格式化与解析（基础能力缺口） |
| disabledFormat | antd `disabledFormat`（interface.d.ts）；`components/ColorInput.js:55` `!disabledFormat && <Select>`：不渲染格式下拉 | — | 已补齐：`DisabledFormat(bool)`、Defaults `"disabledFormat"`，不显示格式下拉，ColorPickerPanel 同名方法（TestColorPickerDisabledFormat） |
| disabledAlpha / show-alpha | antd `disabledAlpha` 默认 false；EP `show-alpha` 默认 false | `NoAlpha()`、Defaults `"disabledAlpha"` | 已对齐（行为按 antd：默认显示透明度，§7.1） |
| allowClear / clearable | antd 默认 false；EP `clearable` 默认 true | `AllowClear()`；Element 面板固定有“清空” | 已对齐 |
| presets / predefine | antd `presets: {label, colors, defaultOpen}`；EP `predefine` | `Presets`、`PresetGroups`（`Closed`） | 已对齐 |
| showText | antd `boolean \| (color) => ReactNode` | `ShowText()` | 已对齐（布尔）；函数形式暂不做（先无需求） |
| arrow | antd `arrow` | — | 暂不做（弹层箭头只在 nav_dropdown.go 私有实现，需先提到共享层） |
| open / onOpenChange / onClear / onChange | antd | `Open/IsOpen/Cleared/Changed` | 等价写法 |
| onChangeComplete / onFormatChange | antd | — | 暂不做（拖动结束查询，先无需求） |
| panelRender | antd | — | 暂不做 |
| size / disabled / status | antd | `Size/Disabled/Status` | 已对齐 |
| 令牌 | antd style/index.js：`colorPickerWidth 234`、`colorPickerHandlerSize 16`、`colorPickerSliderHeight 8`、`colorPickerPresetColorSize 24`、预览块 `8*2 + marginSM = 28` | theme/comp_colorpicker.go、entry_color.go `cp-current` 28 | 已对齐（TestAlignB1DColorPickerTokens） |
| shadcn | shadcn v4 无颜色选择器 | 随令牌 | 不需要 |
| ColorPickerPanel | EP `el-color-picker-panel`（show-alpha、color-format、predefine、border、disabled） | fill_color_panel.go | 已对齐（`DisabledFormat` 同步补上） |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| HSL 颜色字符串 | ColorPicker、ColorPickerPanel（EP `color-format="hsl"`） | `ui/theme` 颜色工具：`HSL` 转换、格式化、`ParseColor` 识别 `hsl()/hsla()` | 暂不做，等用户决定 |
| 滚动停止 / 吸附通知 | TimePicker、TimeRangePicker（antd `changeOnScroll`） | `ui.Scroll` 报告滚动停止（或吸附到项） | 暂不做，等用户决定 |
| 弹层箭头共享绘制 | ColorPicker（antd `arrow`），以后 DatePicker 等 | 把 `nav_dropdown.go` 的 `drawArrow` 提到浮层层（overlay / entry_util） | 暂不做，等用户决定 |

## CHANGELOG 草稿

### 新增

- DatePicker / DatePickerPanel / MultiDatePicker / RangePicker / DateTimePicker：`MinDate`、`MaxDate`（antd minDate / maxDate：范围外不可选，面板翻页箭头停在边界）。
- DatePicker / RangePicker / MultiDatePicker / DateTimePicker / TimePicker / TimeRangePicker / TimeSelect / ColorPicker：`Placement(ui.Placement)` 与 Defaults 键 `"placement"`（接受 `"bottomLeft"`、`"bottomRight"`、`"topLeft"`、`"topRight"`）。
- DatePicker / DatePickerPanel：`ShowWeek(bool)`（周数列；Defaults `"showWeek"`）；DatePicker / DateTimePicker / TimePicker：`ShowNow(bool)`（Defaults `"showNow"`）；DatePicker 的 Defaults 新增 `"needConfirm"`。
- RangePicker：`Order(bool)`、`Separator(s)`；TimeRangePicker：`Order(bool)`（跨夜范围）、`Separator(s)`；Defaults 键 `"order"`。
- TimePicker：`NeedConfirm(bool)`（关闭后点选即生效；Defaults `"needConfirm"`）。
- DateTimePicker：`DefaultTime(t)`（EP default-time）。
- TimeSelect：`IncludeEndTime(bool)`（EP include-end-time；Defaults `"includeEndTime"`）。
- ColorPicker / ColorPickerPanel：`DisabledFormat(bool)`（不显示格式下拉；Defaults `"disabledFormat"`）。
- 主题令牌 `DatePickerToken.NavDisabledOpacity`、`NavDisabledColor`（禁用翻页箭头：Ant 0.25、shadcn 0.5、Element 用禁用文字色）。

### 变更

- Ant 主题日期面板行高 32 → 36（antd `cellHeight 24 + 2 × pickerCellPaddingVertical 6`，date-picker/style/panel.js）：DatePicker、DatePickerPanel、RangePicker、DateTimePicker、MultiDatePicker 的弹层变高 28px；DateTimePicker 时间列随之变高。
- Ant 主题预设栏宽 110 → 120（antd `presetsWidth`）；shadcn 随之 110 → 120；Element 保持 110（`el-picker-panel__sidebar`）。
- Ant 主题月 / 季 / 年面板网格高度 224 → 252（`bodyH = 7 × CellHeight`），4 行时行高 56 → 63（antd `withoutTimeCellHeight × 4 = 264`，更接近规范）。
- DatePicker（含 NeedConfirm 页脚）的“今天”、DateTimePicker 的“此刻”在当前时刻不可选（DisabledDate / MinDate / MaxDate / DisabledTime）时显示为禁用（之前可点但不生效）。
- 禁用的翻页箭头显示“禁止”光标（EP disable mixin `cursor: not-allowed`）。

## 共享文件改动请求

1. `ui/t1b_tokens_test.go`（`TestT1bDerivedDefaults`）：Ant 的 `DatePicker.CellHeight` 期望值 32 → 36（`w, h := float32(36), float32(32)` 改为 `float32(36), float32(36)`；Element 40/36、shadcn 32/32 不变）。原因：上面的规范修正。
2. `api/go/ui.txt`、`api/go/ui_theme.txt`：重新生成（新增方法见 CHANGELOG 草稿；`DatePickerToken` 新增两个字段）。全部为新增，不破坏稳定层（`DatePicker`、`TimePicker` 只加方法）。
3. `CHANGELOG.md`：合并上面的草稿。
4. `ui/AUTHORING.md` §14：表行不变（没有新入口）。
5. 建议（非必需）：`ui/entry_util.go` 的 `entPanel` 支持 placement；本组为不改共享文件，在 `date_util.go` 加了 `dtPanel`（BottomStart 时直接调用 `entPanel`，其它位置用相同的样式和动画）。合并后可以把 `dtPanel` 收回 `entPanel`。
6. 建议：`ui/entry_datepanel.go`（无主文件）的 `dtField` / `dtTypeInput` 增加“只读键入”开关，用于 antd `inputReadOnly`、EP `editable=false`、`readonly`；以及 `preserveInvalidOnBlur`。
7. 建议（组 A）：`IconButtonW` 没有 `Disabled`；本组用 `ui.Disabled(c, off, …)` 包住翻页箭头并设节点透明度。若组 A 加了 `IconButtonW.Disabled`，可以改用它。
8. Locale：本组没有改 `locale.go` / `locale_packs.go`。
9. gallery：新增 `examples/ui-gallery/page_b1d.go`（`DatePickerAlign`、`RangePickerAlign`、`TimePickerAlign`、`ColorPickerAlign`，分别挂在各自父页面下）；`page_date_more.go` 的 `DatePickerMultiple` 高度 900 → 960（面板变高）。
