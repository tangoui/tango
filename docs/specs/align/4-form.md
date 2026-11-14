# 第 4 批组 B：Pro form v3 复核

核对日期：2026-11-15。Pro Components **3.1.14-2**，本地 `/tmp/tango-ref/pro/package`；下文 P 表示该目录。Ant Design **6.7.0**（`/tmp/tango-ref/antd/package`）。v6 页面模板：`/tmp/tango-ref/adpro/repo`，v6.0.3，commit `42f2cb1`。原 `ui-coverage.md §4` 的 v2“完整”不作为 v3 全 API 覆盖承诺。ProField 本体及 FieldSet 由组 A 负责。

## 入口决定

- **Captcha 不另建 `ProFormCaptcha` 函数**。已有 `ProField.ValueType = ValueCaptcha`、`OnCaptcha` 和 `CaptchaSeconds`，扩展这一个入口；`ProFieldCaptcha` 提供 `PhoneKey`（phoneName）、`Text`（captchaTextRender）、`OnTiming`。现有 callback `done(error)` 等价于 Promise resolve/reject；不会把 React ref 暴露到 Go。`form_pro_captcha.go:proCaptcha` 统一 ProField/LoginForm 的倒计时按钮，计时只用 Context/Driver 时钟。
- **Segmented 不另建函数/枚举**。`ValueSegmented` 已在 `form_pro_field.go`（不是本批新增）。官方 P/es/form/components/Segmented/index.js 本身就是 valueType='segmented' 的 ProFormField 包装；Tango 沿用 `ProField`，新增 `Segmented func(*SegmentedW)` 可配置项内容/大小/方向等，无第二套状态机。选项禁用通过 `ProEnum.Disabled` 透传 `Option.Disabled`（字段由主线程添加）。
- 两项均遵守每组件一个入口；建议覆盖表登记“等价写法（ValueCaptcha / ValueSegmented）”，不是仅为凑名字加薄包装。待主线程确认该登记方式。

## ProForm

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 字段与布局 | P/es/form/layouts/ProForm/index.js：BaseForm layout=vertical；grid/rowProps/colProps（2026-11-15） | `form_pro.go:ProForm` 默认 vertical，Columns/Span/Gutter/Horizontal/Children | 等价写法（类型字段与 Go 布局） |
| 初值、受控、重置 | P/es/form/BaseForm/BaseForm.js initialValues/request；request params 驱动 | `ProFormW.Values/Request/Reset`，initial snapshot；异步回调回 UI 帧 | 已对齐 |
| 校验/禁用/只读 | 同上 readonly；antd Form rules/disabled | `ProField.Required/Rules/Validate/AsyncValidate/Disable/Readonly`；FormItem | 已对齐 |
| 提交与加载 | P/es/form/BaseForm/Submitter/index.js：reset 在 submit 前，gap=token.marginXS=8；onFinish Promise | `ProSubmitter`、OnFinish(done)、Submitting；reset 改为先于 submit | 已补齐（TestProFormV3SubmitterOrder） |
| URL/date formatting/transform | BaseForm.d.ts syncToUrl、dateFormatter、omitNil、transform | Go `ProValues` 保留 time.Time；`Params` 返回查询字符串；应用编码提交数据 | 等价写法（无浏览器 URL 状态，格式化交给调用方） |
| ref/hooks/render-prop | ProForm.useForm/useWatch/Provider/ErrorList/useFormInstance | `Form()/Submit/Reset/Children/Submitter` 与依赖闭包 | 等价写法（不搬 React hooks） |
| 令牌、主题、a11y | 基础 antd Form/Input/Button；EP form.scss；shadcn field.tsx | 复用 FormItem、控件令牌、三套主题；表单标签命名输入 | 已对齐 |
| params 变化重拉、autoFocusFirstInput | BaseForm.js request(params)；autoFocusFirstInput 默认 true | Request 仅首帧；没有 ProForm Params/自动聚焦 API | 暂不做（表单 request 生命周期/自动聚焦尚未补齐，非基础层阻塞） |

## ModalForm / DrawerForm

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 入口、受控打开 | P/es/form/layouts/{ModalForm,DrawerForm}/index.d.ts，open/onOpenChange/trigger（2026-11-15） | `form_pro_modal.go:ModalForm/DrawerForm` 共用专属 ModalFormW，open *bool，应用按钮打开 | 等价写法 |
| 默认宽度 | ModalForm/index.js `width || 800`，DrawerForm/index.js 默认 800 | 构造默认 `width:800`；显式 Width/Defaults 优先 | 已补齐（TestProFormV3Defaults） |
| 校验、异步、错误、关闭 | `_shared/useOverlayForm.js` onFinish truthy closes，rejection stays open | `OnFinish(done(nil))` 关闭，error 留在表单 Alert；OK loading | 等价写法（nil success / error failure） |
| preserve/destroyOnHidden | 同上默认保留，destroyOnHidden 时 resetFields | 默认保留受控值，KeepValues幂等；新增 DestroyOnHidden(true) 在关闭后恢复初始值 | 已补齐（TestProFormModalPreservesValues/TestProFormModalDestroyOnHidden） |
| 禁用/只读/键盘/a11y | antd Modal/Drawer、Form 负责焦点陷阱、Esc | Modal/Drawer 基础组件；Readonly；字段禁用透传 | 已对齐 |
| submitTimeout/resize | DrawerForm resize min/max mouse dragging；overlay submitTimeout | — | 暂不做（本批未扩展超时和可拖宽度；非基础层阻塞） |
| 主题 | 同上 Modal/Drawer 基础令牌，非独立 form 视觉令牌 | 复用 Modal/Drawer/Form 三套主题 | 已对齐 |

## QueryFilter

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 默认折叠与响应式 | P/es/form/layouts/QueryFilter/index.js：defaultCollapsed=true；默认断点 576/768/1600（2026-11-15） | `form_pro_query.go:queryPerRow` 560/900/1500 → 576/768/1600；Show 保留至少一个字段 | 已补齐（TestProFormV3Defaults/TestProFormQueryNarrowKeepsOneField） |
| 值与动作 | 同上 labelWidth='80'、searchGutter=24、reset/onFinish | Values/Params/Searched/ResetClicked，LabelWidth 80，gap24，复用控件 | 已对齐 |
| 文案/加载/键盘 | Actions.js reset、search、collapseRender | SearchText/ResetText/Loading；可聚焦展开按钮，a11y Expanded | 已对齐 |
| validation/ignoreRules | processQueryFilterItems.js 可按 ignoreRules 删除规则 | QueryFilter 不建 FormItem 校验；Required/Rules 在 query 不生效 | 暂不做（需接入 Form 校验流程，非基础层缺口） |
| 精确布局与 span | index.js 小宽 vertical，span 配置、labelWidth='auto'、showHiddenNum、split | 现有水平标签、固定列数与 labelWidth；ProField.Hide | 暂不做（vertical/动态 span 等未实现，不再标“完整”） |
| 令牌/主题 | antd Grid/Form/Button；`getBreakpointsConfig(token)` 动态断点 | 复用现有控件，断点暂用官方默认值；gap shadcn16、其它24为已有行为 | 等价写法（动态主题断点覆盖待后续） |

## LightFilter

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 默认值、状态 | P/es/form/layouts/LightFilter/index.js：size middle、variant borderless、collapse=false（2026-11-15） | `pro3_lightfilter.go:LightFilter` 默认无边框；Values/Params；弹层 draft 确认写回 | 已对齐 |
| 更多筛选 | 同上 collapse=true/secondary 字段进 more popup | `.Collapse(n)` 指定前 n 项显示，其余弹层 | 等价写法（数量表达折叠配置） |
| 内容、清除、键盘、a11y | LightWrapper；确认/重置，popover | pill、popup、clear，键盘进入/退出/焦点恢复；toolbar/button/popup 语义 | 已对齐 |
| v3 variant/placement/footerRender | index.d.ts outlined/filled/borderless、placement、footerRender | Bordered；固定 BottomStart；无 footer hook | 暂不做（filled、placement 与 footerRender 尚未补齐，非基础层阻塞） |
| 条件禁用 | Form field disabled | 字段弹层可透传 Disable；触发/清除路径尚待专门回归 | 暂不做（旧覆盖“完整”未证明全部禁用路径） |
| 主题 | style.js 与基础控件；shadcn dropdown/popover | 复用按钮、Popover/Select、主题令牌；已有 shadcn pill 外观 | 已对齐 |

## StepsForm

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 当前步骤、字段、校验 | P/es/form/layouts/StepsForm/{index,StepForm}.js；每步完成再 next（2026-11-15） | `form_pro_steps.go:StepsFormStep`，ProForm 校验后前进；Values 共享 | 已对齐 |
| 返回、异步、单步骤提交 | index.js first/single/last 按钮分支，loading | 当前实现 Prev/Next/Submit、异步 done | 已对齐 |
| controlled current/allowStepSelect/resetSteps | index.d.ts current/onCurrentChange/allowStepSelect/resetSteps | Current、Show 后 SetCurrent；完成后仍留末步，无 reset all | 暂不做（本批未补受控 current 与完成重置；不标“完整”） |
| 内容插槽 | stepFormRender/stepsFormRender/layoutRender | StepsFormStep.Children，应用组合布局 | 等价写法 |
| 尺寸与主题 | style.js min-width420/max100%，step margin-top32；steps-container max1160 | Width 默认600，steps 间距32；复用 Steps/Form令牌 | 等价写法（Go 显式宽度；420最小宽未强加以保留手机布局） |
| a11y/键盘/禁用 | antd Steps / Button / Form | 基础 Steps 有名称；Prev loading禁用，提交 loading | 已对齐 |

## LoginForm / LoginFormPage

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 原组件与模板 | P/es/form/layouts/LoginForm/index.js 是可组合 ProForm；adpro/src/pages/user/login/index.tsx 提供账号/手机号 tabs（2026-11-15） | `pro3_login.go:LoginForm` 内置该 v6 页面模式；AccountOnly/MobileOnly | 等价写法（模板收敛为 Go 链式组件，不声称官方 LoginForm 固有 tabs） |
| 内容与提交 | LoginForm.js logo/title/subTitle/message/actions；submit 按钮 large、width100% | Logo/Title/SubTitle/Actions/OnForgot/SubmitText/OnFinish；失败 Alert | 已对齐 |
| Captcha | v6 template ProFormCaptcha onGetCaptcha/captchaTextRender；P Captcha/index.js countDown default60 | OnSendCode/CodeInterval；共享 proCaptcha 状态，失败不倒计时，倒计时结束恢复 | 已补齐（TestProFormCaptchaSendCountdown/FailureRecovers；既有 TestLoginFormMobileCountdown） |
| 默认尺寸、视觉 | LoginForm/style.js width328/logo44/title33 weight600/desc margin12,40；LoginFormPage activity区域 | Width328（shadcn384）；现有logo44/title33/header gap12/bottom32 | 暂不做（header gap16/desc margin40 精确差异需组件令牌请求；当前未改外观） |
| 背景/activity | LoginFormPage backgroundImageUrl/backgroundVideoUrl/activityConfig | Background/BackgroundImage(image.Image)/Side/Placement | 等价写法（图片由应用加载，Side 是 activity 区域） |
| 视频 | LoginFormPage/index.d.ts backgroundVideoUrl | — | 暂不做（缺跨平台视频纹理/播放基础能力） |
| 语义/禁用/主题 | 基础表单、tabs、button | form/tabs/textbox/buttons；验证码按钮 loading/disabled；三主题沿现有 token | 已对齐 |

## ProFormList（FormList）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 列表编辑、值与校验 | P/es/form/components/List/{index,ListContainer,ListItem}.js（2026-11-15） | `form_pro_list.go:FormList[T]` slice绑定、Creator、Min/Max、Rule、row.Name、Remove/MoveUp/Down；schema ValueFormList | 已对齐 |
| 默认动作与上限 | ListItem.js 默认copy/delete，arrowSort才显示上下；ListContainer 满 max 隐藏add | Tango 默认上下/删除，无copy；满max显示禁用add | 暂不做（默认动作差异与 copy 尚未补，旧“完整”降级） |
| onAfterAdd/remove/actionGuard | index.d.ts / ListItem wrapperAction | Body中应用自管操作；无异步actionGuard | 等价写法（同步可用Changed；异步guard未提供） |
| readonly、creator position top | ListContainer.js readonly隐藏creator，position默认bottom | 外层ProForm只读可视图；FormList.NoActions只隐藏行操作，add仍存在；无top | 暂不做（独立 FormList readonly/top creator未补） |
| 键盘/a11y/主题 | icon actions、Form.Item | icon button具有名称；add dashed/block；复用Form/Button/主题令牌 | 已对齐 |

## ProFormDependency

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 依赖闭包 | P/es/form/components/Dependency/index.js：Form.Item shouldUpdate，通过name读值后调用children（2026-11-15） | `form_pro_field.go:ProField.Dependencies/Options/Hide/Disable/Validate`，刷新后丢弃失效选项 | 等价写法（Go每帧闭包，无独立组件） |
| ignoreFormListField、嵌套namePath | 同上列表上下文解析路径 | FormListRow.Name 为稳定key名称；应用闭包读typed row | 等价写法（无JS path解析） |
| 无视觉/令牌/键盘 | Dependency不生成可见DOM | 复用依赖字段的控件 | 不需要（自身没有可聚焦UI） |

## ProFormGroup

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| title/extra/collapse | P/es/form/components/FormItem/Group/index.js；defaultCollapsed=false（2026-11-15） | `pro4_group.go:ProFormGroup` Description/Extra/Collapsed/DefaultCollapsed/Collapsible/Toggled | 已对齐 |
| 分组schema/Children | 同上 Space/RowWrapper | ProField.Group连续同组自动包装；ProFormItems/Body/Columns/Gutter | 等价写法 |
| tooltip/titleRender/direction | utils/typing.d.ts ProFormBaseGroupProps | Description显示帮助，Extra可组合；无tooltip/titleRender/direction | 暂不做（未补完整标题自定义/方向，非基础层缺口） |
| 精确视觉 | Group/index.js size32、align start、rowGap0；style.js title marginXL32/fontWeight bold | stacked默认（文档明确差异）、Gutter24、titleBottom16、Medium | 暂不做（现有布局与 v3 的 Space不同，未调整全部默认） |
| 受控、键盘、语义、主题 | title role button/Enter/Space | Collapsed指针；头部button Expanded，焦点环；复用主题 | 已对齐 |

## SchemaForm（BetaSchemaForm）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 表单外壳 | P/es/form/components/SchemaForm/{index,typing}.d.ts及layoutType/index.js（2026-11-15） | `form_pro_schema.go:SchemaForm` 六种shell + Values/Open/OnFinish/Columns/Readonly | 已对齐 |
| valueType递归 | valueType/{field,group,formList,formSet,dependency,divider,ignore}.js | []ProField，Group、ValueFormList，FieldSet由组A；Hide/Options闭包 | 等价写法（Go强类型字段/Children替代JS union） |
| steps/embed/update规则 | typing.d.ts steps[]、Embed、shouldUpdate | schemaSteps按Group拆步；无Embed/steps配置/shouldUpdate | 暂不做（未补v3全部schema布局API） |
| 主题/键盘/a11y | 对应外壳与ProField负责 | 复用对应组件，承接其覆盖和缺口 | 等价写法 |

## ProForm 字段组件（Text/Select/…）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 值类型入口 | P/es/form/components/index.d.ts；Text/TextArea/Select/Checkbox/Radio/Rate/Slider/Switch/Digit/Money/Date/DateRange/TreeSelect/Cascader/ColorPicker/Upload（2026-11-15） | `form_pro_field.go/ form_pro_types.go/ p2e_field.go` ValueType；`ProFieldOf`独立渲染 | 等价写法（统一ProField，不新增每种React包装） |
| enum/request | Select/index.d.ts request/params/debounceTime/valueEnum | ValueEnum/Options(ProValues)；动态异步数据由应用提供Options | 等价写法（应用持有异步选项；没有内置远端缓存） |
| disabled选项 | P/es/field/components/Select/index.js:38-51 valueEnum转换保留disabled | ProEnum.Disabled → Option.Disabled，所有choice字段共用 | 已补齐（TestProFormSegmentedConfiguration） |
| 字段prop转发 | fieldProps/proFieldProps/rules | typed ProField与 Render；新增 Segmented 配置钩子 | 等价写法 |
| 只读/空值/主题/标签 | ProField read/edit/update和emptyText | ProField.Readonly、label/enum映射、'-'；控件复用主题与a11y | 已对齐 |
| URL图片加载 | ProFormUpload/FieldImage | image.Image/UploadFile.Thumb，应用加载 | 等价写法（不在绘制帧联网） |

## ProFormCaptcha（ValueCaptcha）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 默认倒计时、成功/失败 | P/es/form/components/Captcha/index.js：countDown||60；await成功setTiming(true)，catch false/loading false（2026-11-15） | `form_pro_captcha.go:proCaptcha` poll成功start，失败恢复idle；发送中loading | 已补齐（TestProFormCaptchaSendCountdown/FailureRecovers） |
| countdown禁用与恢复 | 同上disabled=timing、每秒减1，<=1恢复countDown | Driver时钟、WakeAt、Ceil秒；完成恢复 | 已补齐（TestProFormCaptchaSendCountdown） |
| 文案与计时回调 | captchaTextRender(timing,count)、onTiming(count)初次/变化调用 | ProField.Captcha.Text/OnTiming；默认Locale.LoginForm文案 | 已补齐（TestProFormCaptchaTextTimingDefaults） |
| 手机号验证 | phoneName validateFields后取mobile再发送 | ProFieldCaptcha.PhoneKey；form中验证目标再调用OnCaptcha(allValues) | 等价写法（OnCaptcha可直接拿vals.String(PhoneKey)） |
| 外部值、禁用、键盘、a11y | Input value/onChange，Button loading/disabled | ProValues；Disable/ConfigProvider.disabled；input表单名，button名称；开始/结束播报 | 已补齐（TestProFormCaptchaDisabled/A11y） |
| 尺寸/主题 | inline flex，marginRight8；基础 Input/Button tokens | gap8、复用Input/Button主题；最小按钮宽120为既有Tango差异 | 等价写法 |
| ref/startTiming/endTiming | CaptFieldRef nativeElement/focus/start/end | 不暴露DOM ref；当前无外部start/end计时 | 暂不做（无DOM需要；外部计时控制本批未提供） |

## ProFormSegmented（ValueSegmented）

| 规范项 | 官方定义（来源，日期） | Tango 现状 | 结论 |
|---|---|---|---|
| 编辑、受控、选项禁用 | P/es/form/components/Segmented/index.js；es/field/components/Segmented/FieldSegmentedEdit.js；antd Segmented（2026-11-15） | ProField.ValueSegmented绑定ProValues；Option.Disabled | 已补齐（TestProFormSegmentedConfiguration） |
| 自定义项 | antd options[].label ReactNode；EP segmented default slot | ProField.Segmented(func(*SegmentedW)) + ItemRender | 等价写法（TestProFormSegmentedConfiguration） |
| 键盘与只读 | antd Segmented方向键；FieldSegmentedRead.js proFieldParsingText | 复用Segmented；只读enum label、未知值原样 | 已补齐（TestProFormSegmentedConfiguration） |
| 远端选项/加载 | FieldSegmented/index.js useFieldFetchData/Spin | Options闭包/应用加载，Render可自定义加载UI | 等价写法 |
| 尺寸/主题 | antd segmented/style；EP segmented.scss；shadcn tabs.tsx | 基础Segmented令牌和状态，不另绘新组件 | 已对齐 |

## 基础能力缺口

| 缺口 | 影响的组件 | 需要的基础能力 | 现状 |
|---|---|---|---|
| 视频背景 | LoginFormPage | 跨平台视频播放/纹理 | 暂不做 |

以上表内其它“暂不做”均为现有高级API未补齐，不能冒充基础能力缺口。原28项覆盖需按这些明确缺口降级，不维持“全API完整”。

## CHANGELOG 草稿

### 新增

- ProField.Captcha（ProFieldCaptcha.PhoneKey/Text/OnTiming），等价 ProFormCaptcha 的 phoneName/captchaTextRender/onTiming；沿用 ValueCaptcha、OnCaptcha/CaptchaSeconds。
- ProField.Segmented(func(*SegmentedW)) 配置已有 ValueSegmented；选项disabled贯通ProEnum到基础Option。
- ProField/LoginForm共享验证码发送/倒计时状态实现；Driver可确定验证成功/失败与倒计时恢复，不依赖sleep。

### 变更

- ModalForm/DrawerForm 默认宽度：基础Modal/Drawer默认 → 800（v3对应index.js）。
- QueryFilter 列断点：560/900/1500 → 576/768/1600；窄屏折叠时0字段 → 至少1字段（v3 getSpanConfig/showLength）。
- ModalForm/DrawerForm 默认首次打开/重开清空 → 保留值；新增 DestroyOnHidden(true) 显式清空（v3默认不销毁）。TestModalFormClosesAfterSubmit 末尾重开期望从空字符串改为 x，与本行为修正对应；未删用例。
- ProForm 默认提交按钮顺序 submit/reset → reset/submit（Submitter/index.js）。
- 不修改插件版本、VERSION 或历史发布日志。

## 共享文件改动请求

1. 主线程已加 `ui/table_pro.go:ProEnum.Disabled bool`（组B请求，组D同文件）；组B不编辑该文件。
2. `ui/AUTHORING.md §14` ProForm行补ValueCaptcha/ValueSegmented及配置；`docs/specs/ui-coverage.md §4` 改为链接本记录，不再按v2泛称全部完整。
3. 主线程运行API工具登记实验类型 `ProFieldCaptcha`、ProField.Captcha/Segmented字段、Defaults ProFormCaptcha.countDown；本组未新增稳定类型的方法。新增实验方法 ModalFormW.DestroyOnHidden(bool)。
4. 精确LoginForm视觉令牌建议由主线程后续确认：HeaderGap16、HeaderTitleWeight600、DescriptionMarginBottom40，依据 P/es/form/layouts/LoginForm/style.js；现有12/Medium/32，三套主题值应沿既有体系分别核对，不在组件里加Flavor分支。
5. Locale无需新增字段，验证码所有默认文案复用四语言完整的 LoginForm.GetCode/ResendFmt/CodeSentFmt/CodeReady。

## 验证与自查

已完成组内自查。普通组内测试、build/vet和gallery命令与结果见组B报告。普通测试编译暂受其它组 RED 测试影响时，只用 /tmp/b4b/ov.json overlay 隐藏其它组新增测试隔离验证，不修改别组文件。

## 主线程共享令牌整合（2026-11-15）

- FormToken.LoginLogoGap=16、LoginTitleFontWeight=600、LoginDescriptionMarginBottom=40；LoginForm.header 从 token 读取，修正原 12/500/32（Pro v3 form/layouts/LoginForm/style.js）。三主题 token 定义完整；shadcn 特有 header 仍保留既有独立布局。TestAlignB4ProTokens、LoginForm 回归通过。


## 独立审阅修复（2026-11-15）

- PhoneKey 后置字段：发送意图改为 frame deferred，在所有 FormItem 注册完成后验证目标。测试 `TestProFormCaptchaPhoneAfterCaptcha` RED→GREEN。
- 异步 PhoneKey：等待期间保留发送意图，成功后只发送一次，失败取消；等待时手机号改变取消旧意图，要求用户重新触发，避免把已验证的旧手机号授权用于新号码。`TestProFormCaptchaAsyncPhoneIntent` success/failure/changed RED→GREEN；无 sleep。
- DestroyOnHidden：首次打开保留受控初值并合并 ProField.Initial；仅从打开切换关闭时恢复初始快照，不清空整个 map。`TestProFormModalDestroyRestoresInitial` 覆盖 Modal/Drawer RED→GREEN。
- 主线程新增的 LoginForm header 三个 FormToken 接线完整保留，没有重写 pro3_login.go。
