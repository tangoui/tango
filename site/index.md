---
layout: home

hero:
  name: Tango
  text: 用 Go，构建高颜值 GUI
  tagline: 自带丰富组件、三套设计语言和深浅色主题。从精致的桌面界面开始，连接原生系统能力，也能混用 Web 前端。
  image:
    src: /logo.svg
    alt: Tango
  actions:
    - theme: brand
      text: 在线体验 GUI
      link: /components/
    - theme: alt
      text: 开始构建
      link: /guide/ui
    - theme: alt
      text: GitHub
      link: https://github.com/tangoui/tango

features:
  - icon: 🎨
    title: 开箱即用的高颜值 GUI
    details: 按钮、输入框、表单、表格、日期选择、树、弹层、图表等组件，使用 Go 组合出完整界面。从基础控件到工作台布局，都有统一的设计语言。
    link: /components/
  - icon: 🌓
    title: 三套风格，深浅皆宜
    details: Ant Design、Element Plus、shadcn 三套主题，共享语义令牌。切换风格、深浅色或主色，让组件与整个应用保持一致。
    link: /guide/ui#主题
  - icon: 🖱️
    title: 代码编辑器，直接上手
    details: 在浏览器直接编辑 Go、TypeScript、Python、JavaScript，体验语法高亮、撤销重做、只读与行号。表格、图表和其他组件也能在线操作。
    link: /components/code-editor
  - icon: 🪟
    title: Go GUI 与 Web 灵活组合
    details: 用 Go 自绘界面，也可以接入熟悉的 Vue、React 或 TypeScript 前端。macOS 上可在同一个窗口混用 WebView 和 Go 自绘内容。
    link: /guide/ui
  - icon: 🔌
    title: 界面之外，连接原生系统
    details: 窗口、菜单、托盘、通知、剪贴板等系统能力由 Go 提供；Web 前端通过方法绑定、事件和带背压的流与业务协作。
    link: /guide/system/
  - icon: 📦
    title: 从开发到安装包
    details: tan 覆盖模板、热重载、构建、打包、签名与运行，并提供签名自动更新。平台能力和验证范围在对应指南中明确说明。
    link: /guide/packaging

---

## 从好看的界面，到真正好用的应用

**Tango 面向希望用 Go 构建精致 GUI 的开发者。** 组件负责布局、交互和绘制，你用 Go 持有状态、编写业务逻辑，再按需求接入原生系统或 Web 前端。

<div class="gui-showcase">

![Tango Go GUI：导航、统计卡片、筛选表单和数据表格](/screenshots/gui-preview.png)

由 `examples/ui-gallery` 实际生成的 Go GUI，包含导航、统计卡片、筛选表单和数据表格。[在线操作组件 →](/components/)

</div>

## 让界面从第一天就有设计感

| 你想构建什么 | Tango 提供什么 |
| --- | --- |
| 清爽的表单与设置页 | 输入、选择、开关、校验，以及统一的控件状态 |
| 数据工作台与管理工具 | 表格、树、图表、导航和工作区布局组件 |
| 有品牌感的应用 | 三套主题、深浅色、主色与组件级设计令牌 |
| 能与系统协作的 GUI | Go 业务代码与窗口、菜单、托盘、通知等能力 |
| 复用现有 Web 界面 | 系统 WebView、前端模板、Go 方法绑定和事件通信 |

先试试 [代码编辑器](/components/code-editor) 或 [全部 GUI 在线体验](/components/)，再查看 [Go 自绘 UI 入门](/guide/ui) 或 [全部组件目录](/guide/components)。

Tango 当前为 0.x；Go 自绘、混合界面和移动端仍处于实验阶段。各平台能力与验证范围见 [自绘 UI](/guide/ui) 和 [移动端指南](/guide/mobile)。

## 一分钟上手

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan init -template ui hello  # Go 自绘 GUI 模板
cd hello
tan dev                 # 打开窗口，Go 改动自动重建
tan build               # 发布构建，产物在 build/
```

接着读 [自绘 UI 指南](/guide/ui)。需要 Web 前端时，可从 [快速开始](/guide/getting-started) 选择对应模板。
