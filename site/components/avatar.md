---
title: Avatar 头像
---
# Avatar 头像

用头像、首字母或图标表达用户、团队和对象身份。

## 何时使用

- 用户列表、评论、协作成员和账户入口中展示身份。
- 需要叠放多个成员时使用 `AvatarGroup`。
- 图片需要预览、缩放时改用 [Image 图片](/components/image)；只是状态提示时用 [Badge 徽章](/components/badge)。

## 在线体验

<GalleryPreview component="Avatar" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Avatar(c, "张三").Size(40).A11yLabel("张三")
}
```

头像会从文本生成缩写；`label` 为空时可用 `Icon` 或 `Image` 提供内容。

## 常见用法

### 方形与自定义颜色

```go
func avatarVariants(c *ui.Context) {
    ui.Avatar(c, "AB").Square().Size(32)
    ui.Avatar(c, "A").Size(40)
}
```

### 图片头像

```go
func imageAvatar(c *ui.Context, img image.Image) {
    ui.Avatar(c, "").Image(img).Size(40).A11yLabel("项目头像")
}
```

使用时导入标准库 `image`；图片加载和缓存由应用负责。

### 头像组

```go
func members(c *ui.Context) {
    ui.AvatarGroup(c).Max(3).Size(32).Body(func() {
        ui.Avatar(c, "李"); ui.Avatar(c, "王"); ui.Avatar(c, "赵"); ui.Avatar(c, "孙")
    })
}
```

## 主题与样式

三套主题分别通过头像背景、文字和组重叠令牌适配 Ant Design、Element Plus、shadcn；`ConfigProvider.Defaults("AvatarGroup", ...)` 支持 `maxCount`。

## 注意事项

状态放在页面变量或后端数据中；头像本身是展示控件。图片头像应提供 `A11yLabel`，不要只依赖首字母。RTL 下组的重叠方向自动镜像。

## 示例代码

<!--@include: ./_gen/avatar-examples.md-->

## API

<!--@include: ./_gen/avatar-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/display.go)
- [ui Go API](/reference/api/ui)
- [Badge 徽章](/components/badge)
