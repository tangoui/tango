---
title: PageContainer 页容器
---
# PageContainer 页容器

Pro 页面的标准外壳：上方页头（面包屑、返回、标题、标签、操作区、标签页），中间内容区，底部可带 FooterToolbar 底部操作栏。对标 Ant Design Pro Components v3 的 PageContainer 和 FooterToolbar。

## 何时使用

- ProLayout 内容区里的每一个业务页面：详情页、编辑页、高级表单页，需要统一的页头和间距。
- 长表单或编辑页需要一条始终可见的「提交 / 取消」操作栏时，用 `FooterToolbar`（或 PageContainer 的 `Footer`）。
- 只需要一个带标题的内容块时用 [Card 卡片](/components/card) 或 [ProCard 高级卡片](/components/procard)；整体导航框架用 [ProLayout](/components/prolayout)。

## 在线体验

PageContainer 场景展示完整页头与底部操作栏。

<GalleryPreview component="PageContainer" />

FooterToolbar 场景：滚动内容区，可以看到吸底操作栏下方的色块和文字被模糊、透出半透明底色。

<GalleryPreview scene="FooterToolbar" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var activeTab = "base"

func page(c *ui.Context) {
    ui.PageContainer(c, "高级表单").
        Breadcrumb("首页", "表单页", "高级表单").
        SubTitle("这是一个副标题").
        Extra(func() {
            ui.Button(c, "次要按钮")
            ui.Button(c, "主操作").Primary()
        }).
        Tabs(&activeTab, []ui.TabItem{{Key: "base", Label: "基本信息"}, {Key: "log", Label: "操作日志"}}).
        Footer(func() {
            ui.Button(c, "重置")
            ui.Button(c, "提交").Primary()
        }).
        Body(func() {
            ui.ProCard(c, "页面内容").Body(func() { ui.Text(c, "当前标签：" + activeTab) })
        })
}
```

链式设置页头后调用 `Body` 构建内容。页头本身是一个 `PageHeader`；`Footer` 会在页面底部生成一条 `FooterToolbar`，`FooterExtra` 设置它左侧的内容。

## 常见用法

以下片段省略了 `import`。

### 返回、标签与页头内容

`Back()` 显示返回按钮，`BackClicked()` 报告本帧是否被点击；`Tags` 放在标题右侧，`Content` 和 `ExtraContent` 分别是页头下方左侧与右侧的区域。

```go
func detailPage(c *ui.Context, goBack func()) {
    pc := ui.PageContainer(c, "订单 #1024").Back().
        Tags(func() { ui.Tag(c, "进行中").Color("processing") }).
        Content(func() { ui.Text(c, "创建人：曲丽丽") }).
        ExtraContent(func() { ui.Statistic(c, "订单金额", 568.08, 2).Prefix("¥") }).
        Body(func() { ui.Text(c, "详情内容") })
    if pc.BackClicked() {
        goBack()
    }
}
```

### 填满窗口与 Ghost

`Fill()` 让页容器填满父容器高度，内容区单独滚动、底部操作栏固定在最下方；`Ghost()` 去掉页头背景和外边距，适合嵌在 ProLayout 内容区里；`NoHeader()` 完全不显示页头；`Loading(true)` 显示加载态。

```go
func fillPage(c *ui.Context) {
    ui.PageContainer(c, "编辑规则").Ghost().Fill().
        Footer(func() { ui.Button(c, "保存").Primary() }).
        FooterExtra(func() { ui.Text(c, "已修改 2 项").Secondary() }).
        Body(func() { ui.Skeleton(c).Rows(8).Show() })
}
```

### FooterToolbar：独立使用

`FooterToolbar` 也可以单独放在任何页面底部。`Body` 里是右侧的按钮，`Extra` 是左侧的提示。不吸底时，它就是普通流式布局里一条宽度跟随父容器的操作栏。

```go
func footer(c *ui.Context) {
    ui.FooterToolbar(c).
        Extra(func() { ui.Text(c, "表单有 2 处修改尚未保存") }).
        Body(func() {
            ui.Button(c, "取消")
            ui.Button(c, "提交").Primary()
        })
}
```

### FooterToolbar：吸底与毛玻璃

`Sticky()` 通过 Affix 把操作栏固定在窗口底部；`Target(id)` 改为固定在某个滚动容器底部（同时隐含 `Sticky`）。固定期间，操作栏下面滚过的内容会被背景模糊（`paint.BackdropBlur`），再叠上一层半透明底色，形成毛玻璃效果；回到正常流式位置后恢复普通背景。

滚动容器的 ID 要在它构建之后才知道，即时模式下的惯用做法是把本帧的 ID 存起来，下一帧传给 `Target`：

```go
var scrollID ui.ID

func stickyFooter(c *ui.Context) {
    sb := ui.Scroll(c, func() {
        for i := 0; i < 30; i++ {
            ui.Paragraph(c, "长表单内容……")
        }
        ui.FooterToolbar(c).Target(scrollID).Body(func() {
            ui.Button(c, "提交").Primary()
        })
    })
    scrollID = sb.ID()
}
```

第一帧 `scrollID` 为 0，操作栏退化为固定在窗口底部；从第二帧起固定在滚动容器底部。

## 主题与样式

| 风格 | 差异 |
| --- | --- |
| Ant Design | 操作栏高 56、左右内边距 24，上方分隔线和向上的阴影；吸底时 `FrostedBg` 为容器色 60% 不透明、模糊半径 8（对应 Pro 的 `rgba(255,255,255,0.6)` + `blur(8px)`）。 |
| Element Plus | 操作栏高 60、内边距 20；Element Plus 没有对应组件，取值偏离基线：吸底时底色 72% 不透明、模糊半径 10。 |
| shadcn | 上边框、无阴影；流式底色为容器色 95%，吸底时 60% + 模糊 8（对应 `bg-background/60 backdrop-blur`）。 |

FooterToolbar 组件令牌在 `theme.Components.FooterToolbar`（`theme.FooterToolbarToken`）：`Bg`（流式时的背景）、`FrostedBg`（吸底时叠在模糊内容上的半透明底色）、`BlurRadius`（背景模糊半径，逻辑像素；设为 0 则永不模糊）。以下情况不使用毛玻璃，而是绘制 `Bg`：未吸底、高对比度主题（此时为不透明背景）、渲染器声明不支持背景模糊。

`ConfigProvider.Defaults` 键：`"PageContainer"` 支持 `"ghost"`、`"fill"`、`"tabsCard"`（bool）；`"FooterToolbar"` 支持 `"sticky"`（bool）。

## 注意事项

- **状态**：标签页的当前键绑定在应用变量上（`Tabs(&activeTab, ...)`），`TabChanged()` 报告本帧是否切换；页容器本身不保存业务状态。
- **吸底目标**：`Target` 使用上一帧的滚动容器 ID，见上文示例；`PageContainer.Fill()` 自带的底部栏不需要手动设置 Target。
- **性能**：背景模糊只覆盖操作栏自身的矩形，并按损坏区域局部重绘；内容在下方滚动时只重算这条区域。
- **无障碍**：操作栏的无障碍角色是 toolbar；按钮按重要性从左到右排列，主操作放在最右。
- **RTL**：模糊区域就是操作栏的矩形，在 RTL 下同样生效；操作栏内容不做额外镜像。

## 示例代码

<!--@include: ./_gen/pagecontainer-examples.md-->

## API

<!--@include: ./_gen/pagecontainer-api.md-->

## 相关链接

- [源码 pro2_page.go](https://github.com/tangoui/tango/blob/main/ui/pro2_page.go)
- [ui Go API](/reference/api/ui)
- [ProLayout](/components/prolayout)、[ProCard 高级卡片](/components/procard)、[ProForm 高级表单](/components/proform)
