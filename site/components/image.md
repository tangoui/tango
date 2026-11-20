---
title: Image 图片
---
# Image 图片

展示图片并提供尺寸、裁剪、占位、失败回退和预览能力。

## 何时使用

- 头像、封面、缩略图和内容图片展示。
- 需要点击放大、旋转或浏览图片组时使用预览。
- 仅需图标时使用 `icon` 包，避免加载图片。

## 在线体验

<GalleryPreview component="Image" />

## 基本用法

```go
import (
    "image"
    "github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
    ui.Image(c, image.NewRGBA(image.Rect(0, 0, 320, 180))).Size(320, 180).Alt("示例图片")
}
```

## 常见用法

### 裁剪与圆角

```go
func crop(c *ui.Context, img image.Image) {
    ui.Image(c, img).Size(240, 160).Fit("cover").Radius(8)
}
```

### 加载和失败回退

```go
func loading(c *ui.Context, img, fallback image.Image, pending, failed bool) {
    ui.Image(c, img).Loading(pending).Failed(failed).Fallback(fallback).Alt("产品封面")
}
```

### 预览组

```go
func gallery(c *ui.Context, imgs []image.Image) {
    if len(imgs) == 0 {
        return
    }
    ui.Image(c, imgs[0]).Size(160, 100).PreviewGroup(imgs, 0).Alt("相册封面")
}
```

`PreviewGroup` 启用点击预览并提供上一张 / 下一张操作。`PreviewSrc(img)` 为缩略图指定单独的预览原图，`OpenPreview()` 可在按钮点击等事件中打开预览。`MinScale`、`MaxScale` 和 `ScaleStep` 调整缩放，`Movable`、`MaskClosable`、`CloseOnEscape` 控制交互，`Infinite` 和 `ShowProgress` 控制图片组循环及页码。

## 主题与样式

图片令牌负责占位背景、边框、预览遮罩和圆角。`Fit` 使用 `contain` 或 `cover` 等裁剪策略；尺寸优先用 `Size` 明确给出，避免布局跳动。

## 注意事项

给内容图设置 `Alt` 或 `A11yLabel`；装饰图可使用空替代文本。预览组的索引必须对应图片数组。大图应在进入页面前缩放，避免在即时模式中反复解码。

## 示例代码

<!--@include: ./_gen/image-examples.md-->

## API

<!--@include: ./_gen/image-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_image.go)
- [Avatar 头像](/components/avatar)
