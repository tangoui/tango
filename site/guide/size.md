---
title: 性能与体积
outline: [2, 3]
---

# 性能与体积

这里记录**可复现的体积测量**，不把目标写成结果。当前没有在这组最小样本上测运行时内存、CPU 或启动时间，因此不提供这些数字。

## 环境与口径

- 测量日期：2026-11-14；Tango `main` @ `be5c620+`；Apple M4 主机，macOS；Go 1.27.2。
- 目标：`darwin/arm64`、`linux/amd64`、`windows/amd64`，均 `CGO_ENABLED=0` 交叉编译。
- 编译参数：`-trimpath -ldflags='-s -w -buildid='`。1 MiB = 1,048,576 字节。
- WebView 样本只有一个窗口并加载 `https://example.com`；自绘样本只有 `uihost.Open` 和一个 `ui.Button`。
- 结果随 Go 版本、提交、工具链和压缩器变化；下面的数字不是应用体积上限。

## 最小样本结果

### 可执行文件

| 样本 | 平台 | 去符号可执行文件 | gzip -9 | xz -9e |
| --- | --- | ---: | ---: | ---: |
| WebView | macOS arm64 | 5.59 MiB（5,861,026 B） | 2.32 MiB | 1.75 MiB |
| WebView | Linux amd64 | 5.70 MiB（5,972,195 B） | 2.40 MiB | 1.92 MiB |
| WebView | Windows amd64 | 5.56 MiB（5,828,096 B） | 2.35 MiB | 1.88 MiB |
| 自绘 GUI | macOS arm64 | 9.68 MiB（10,152,386 B） | 3.87 MiB | — |
| 自绘 GUI | Linux amd64 | 10.09 MiB（10,584,291 B） | 4.08 MiB | — |
| 自绘 GUI | Windows amd64 | 9.84 MiB（10,317,824 B） | 3.97 MiB | — |

### 安装包

macOS WebView 样本的安装包（`tan build` + `tan package`，默认参数，未正式签名）：

| 产物 | 体积 | 说明 |
| --- | ---: | --- |
| `Min-0.1.0-darwin-arm64.dmg` | **1.80 MiB**（1,884,576 B） | `tan package` 的默认产物，LZMA 压缩（`ULMO`）。`hdiutil verify` 通过，挂载后有 `.DS_Store` 和指向“应用程序”的链接。首页“最小安装包 1.8 MiB”指的就是它 |
| `Min.app` | 5.59 MiB | 应用包内全部文件长度之和 |
| `.zip`（`Min.app`） | 2.32 MiB | 对照 |
| `.dmg`，LZFSE（`ULFO`） | 2.39 MiB | 对照 |
| `.dmg`，zlib（`UDZO` 9 级） | 2.32 MiB | 较早版本的 `tan package` 用这种格式 |

`tan package` 默认改用 LZMA dmg 的提交是 `e2de8be`。更早的版本产出的是 zlib dmg，同一个应用为 2.32 MiB。

Windows 安装程序（同一个 WebView 样本，`tan package -platform windows/amd64`，NSIS `SetCompressor /SOLID lzma`，未签名）：

| 产物 | 体积 |
| --- | ---: |
| `Min-0.1.0-windows-amd64-setup.exe` | **1.97 MiB** |
| 安装程序里的 `Min.exe` | 5.60 MiB |
| `Min.exe` 用 zip -9 压缩（对照） | 2.36 MiB |

两个安装程序都是 `tan package` 的默认产物，不需要额外参数：macOS 1.80 MiB，Windows 1.97 MiB。**Linux 这次没有打安装包**（没有生成 AppImage、deb 等），表里只列可执行文件经 gzip、xz 压缩后的大小，不能当作安装包体积。

> **为什么不是更小？** WebView 样本约 5.6 MiB 的二进制主要来自 Go 运行时、`net/http` 与 `crypto/tls`。Tango 的资源服务是一个 `http.Handler`，所以在不改变这部分架构的情况下，体积不会简单地降到很小。安装包不包含浏览器引擎：WebView 使用系统提供的 WKWebView、WebView2 或 WebKitGTK。

### 自绘 UI 的差异

自绘样本多出 Tango 的 GUI、排版和渲染器代码。gzip -9 后为 3.87–4.08 MiB；这只是文件大小，不说明运行速度或内存占用。真实示例有更多代码和资源：`examples/hello` 的 WebView + IPC 约 7.8–8.1 MiB，`examples/ui-window` 的完整自绘演示约 10.8–11.3 MiB。

## 复现

`scripts/size-report.sh` 会写出下面两个最小程序，按同样的参数构建三个桌面目标，再打印字节数、MiB 和 gzip -9 的大小（不生成安装包）：

```bash
bash scripts/size-report.sh
```

在 `f3ba799` 上运行的输出如下，与上表的差别不超过 6 KB（提交不同）：

```text
go version go1.27.2 darwin/arm64, tango f3ba799, 2026-11-14
flags: CGO_ENABLED=0 -trimpath -ldflags='-s -w -buildid='
app  target                bytes       size        gzip -9
web  darwin/arm64        5861010   5.59 MiB       2.32 MiB
web  linux/amd64         5972195   5.70 MiB       2.40 MiB
web  windows/amd64       5828096   5.56 MiB       2.35 MiB
ui   darwin/arm64       10152018   9.68 MiB       3.87 MiB
ui   linux/amd64        10580195  10.09 MiB       4.08 MiB
ui   windows/amd64      10317824   9.84 MiB       3.97 MiB
```

脚本等价的 WebView 源码：

```go
package main
import "github.com/tangoui/tango"
func main() {
    app := tango.New(tango.Options{Name: "Min"})
    app.OnReady(func() { app.NewSurface(tango.SurfaceOptions{
        Title: "Min", URL: "https://example.com",
    }) })
    app.Run()
}
```

自绘源码：

```go
package main
import (
    "github.com/tangoui/tango"
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/uihost"
)
var clicks int
func page(c *ui.Context) {
    if ui.Button(c, "Click").Primary().Clicked() { clicks++ }
}
func main() {
    app := tango.New(tango.Options{Name: "Min"})
    app.OnReady(func() { uihost.Open(app,
        uihost.Options{Surface: tango.SurfaceOptions{Title: "Min"}},
        page, ui.RunnerOptions{}) })
    app.Run()
}
```

打包前先在样本目录里放一个 `tan.json`（`{"name":"Min","id":"com.example.min","version":"0.1.0"}`），`go.mod` 里用 `replace` 指向本地仓库。直接用 `go build` 构建两个样本：

```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
  -trimpath -ldflags='-s -w -buildid=' -o Min .
# 对 linux/amd64、windows/amd64 替换 GOOS/GOARCH
```

用 `tan package` 打安装包：

```bash
tan build -platform darwin/arm64
tan package -platform darwin/arm64
# → build/Min-0.1.0-darwin-arm64.dmg（LZMA，1.80 MiB）
hdiutil imageinfo build/Min-0.1.0-darwin-arm64.dmg | grep Format   # Format: ULMO

# Windows NSIS 安装程序（需要 PATH 里有 makensis）
tan package -platform windows/amd64
# → build/Min-0.1.0-windows-amd64-setup.exe（1.97 MiB）

# 可执行文件压缩对照
xz -9e -k build/darwin-arm64/Min.app/Contents/MacOS/Min
```

`tan package` 会先执行 `tan build`。签名、应用图标、业务代码、前端资源和正式发布元数据都会改变最终体积，应在每次发布时重新测量。

## 测量边界

- 没有生成 Linux 安装包；表中 Linux 的 gzip、xz 数字是可执行文件的压缩结果，不是安装包。
- 系统 WebView 运行时不计入应用包：目标机器仍需要 WebView2、GTK/WebKitGTK 等系统依赖。
- 默认模板（`tan init` 生成的入门应用，不是最小程序）在各平台、各格式下的安装包体积，见仓库里的 [`docs/package-size.md`](https://github.com/tangoui/tango/blob/main/docs/package-size.md)（2026-11-13，十进制 MB，附原始字节数和复现步骤）。那份报告里的 dmg 数字是改用 LZMA 之前测的（`UDZO`），现在的默认 dmg 会小一些。
