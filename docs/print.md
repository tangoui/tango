# 打印（sys/print）

`sys/print` 用系统打印对话框打印 WebView 页面、PDF 文档和图片，还能不弹对话框把页面导出成 PDF 文件。

```go
import printing "github.com/tangoui/tango/sys/print" // 包名 print 会遮蔽内置函数 print，建议起别名

err := printing.Page(s, printing.Options{Title: "发票 #42"})          // 打印 WebView 当前页面
err  = printing.PDF(app, pdfBytes, printing.Options{Parent: s, Copies: 2}) // 打印 PDF 数据
err  = printing.File(app, "/path/a.pdf", printing.Options{})               // PDF 或 PNG/JPEG/GIF 文件
err  = printing.Image(app, img, printing.Options{Parent: s})               // 打印 image.Image（一页）
err  = printing.SavePDF(s, "/tmp/page.pdf")                                // 页面导出为 PDF，不弹框
doc, _ := printing.ImagePDF(img1, img2)                                    // 图片转 PDF（每张一页）
if errors.Is(err, printing.ErrCanceled) { /* 用户取消 */ }
```

所有函数都会阻塞，直到对话框关闭、任务交给系统打印队列为止，不会等到纸张打印出来。在主线程（回调）里调用时，等待期间事件循环照常运行；在 Android 上要从 goroutine 调用（见 [mobile.md](mobile.md)）。

返回的错误：

| 错误 | 含义 |
| --- | --- |
| `ErrUnsupported` | 平台或该组合不支持（见下表），也包括画布（自绘）界面、已关闭的界面 |
| `ErrCanceled` | 用户取消了对话框，前提是平台会告知（Windows 打印页面时不会告知） |
| `tango.ErrNotRunning` | 应用已退出，或在对话框打开期间退出 |

`Options`：

- `Title`：打印任务名（打印队列里显示；Linux 上用作“打印到文件”的默认文件名）。为空时由平台决定，通常是页面标题。
- `Copies`：对话框里预置的份数，0 表示用系统默认值。用户可以在对话框里修改。
- `Parent`：`PDF`、`File`、`Image` 的对话框挂在哪个窗口上；`Page` 总是挂在自己的窗口上。

`File` 和 `Image` 先在 Go 里把图片转成 PDF（`ImagePDF`：按 96 dpi 换算页面尺寸，透明部分铺白底），然后走 `PDF` 的同一路径，所以各平台只需要能打印 PDF。

## 平台支持

| 能力 | macOS | Windows | Linux | iOS / Android |
| --- | --- | --- | --- | --- |
| `Page` 打印页面 | ✓ macOS 11+，作为窗口 sheet；`Title`、`Copies` 生效 | 部分：WebView2 运行时 1.0.1518+ 用 `ShowPrintUI` 弹系统对话框，**立即返回 nil**、不报告取消，`Options` 不生效；更老的运行时退回 `window.print()`（WebView2 自带的预览对话框） | ✓ WebKitGTK 打印对话框；`Copies` 生效，`Title` 只作“打印到文件”的文件名 | — `ErrUnsupported` |
| `PDF` / `File` / `Image` | ✓ PDFKit；有 `Parent` 时为 sheet，否则为应用模态面板；页面缩小以适应纸张并自动旋转 | 部分：用 `ShellExecute` 的 `print` 动词，交给系统里注册的 PDF 应用去打印，是否弹框、份数都由那个应用决定，`Options` 不生效；**没注册 print 动词时（例如 Edge 作为默认 PDF 阅读器）返回 `ErrUnsupported`** | ✓ `GtkPrintUnixDialog` 选打印机，再由 `GtkPrintJob` 直接发送 PDF；选中的打印机不接受 PDF 时返回 `ErrUnsupported`（CUPS 打印机和“打印到文件”都接受） | — `ErrUnsupported` |
| `SavePDF` 导出 PDF | ✓ `createPDFWithConfiguration`（macOS 11+，整页一张） | ✓ `PrintToPdf` | ✓ 用“打印到文件”打印机 | iOS 14+ 支持（整页一张）；Android 不支持 |

`SavePDF` 就是 `tango.Surface.PrintToPDF` 加写文件，要设置纸张、方向、背景时直接用 `PrintToPDF`（见 [webview.md](webview.md)）。

### 实现细节

- **macOS**：`-[WKWebView printOperationWithPrintInfo:]` 配合 `runOperationModalForWindow:delegate:didRunSelector:contextInfo:`（WebKit 的打印操作必须挂在窗口上运行，所以窗口隐藏或已有 sheet 时返回错误）；PDF 用 `-[PDFDocument printOperationForPrintInfo:scalingMode:autoRotate:]`。打印信息从 `+[NSPrintInfo sharedPrintInfo]` 复制一份再改，不会影响之后的打印任务。AppKit 回调里取消和失败都表现为 `success == NO`，统一报告为 `ErrCanceled`。
- **Windows**：`ICoreWebView2_16::ShowPrintUI(COREWEBVIEW2_PRINT_DIALOG_KIND_SYSTEM)`。PDF 先写到临时文件再交给 `ShellExecuteW`；由于接手的应用会异步读取文件，临时文件在 10 分钟后删除，进程提前退出时会留在 `%TEMP%`（`tango-print-*.pdf`）。
- **Linux**：`webkit_print_operation_run_dialog` 是模态的（嵌套主循环）；PDF 对话框不嵌套主循环，通过 `response` 信号应答。所需符号在第一次使用时用 `dlsym` 绑定，缺失时返回 `ErrUnsupported`。

### 需要在实机上验证的点（Windows 只做了交叉编译）

- `ShowPrintUI` 的 vtable 序号（114）和 `ICoreWebView2_16` 的 IID 在真实运行时上是否正确；运行时低于 1.0.1518 时是否正确退回 `window.print()`。
- 在装了 Adobe Reader / SumatraPDF（注册了 `print` 动词）和只有 Edge 的机器上分别调用 `printing.PDF`：前者应交给对应应用，后者应返回 `ErrUnsupported`。
- `SavePDF` 在 Windows 上与 `PrintToPDF` 的已有行为一致。

## 自绘界面（tango/ui）怎么打印

`Page` 只打印 WebView 页面；画布界面（`tango/ui` 窗口）调用 `Page` 会得到 `ErrUnsupported`。本包不直接打印 ui 窗口，可以先把界面渲染成图片再打印：

```go
// 用无窗口的 ui.Driver 按打印尺寸渲染同一个页面函数
d := ui.NewDriver(invoicePage, ui.DriverOptions{Width: 794, Height: 1123, Scale: 2}) // A4 @96dpi，2 倍清晰度
img := d.Settle()                                   // *image.RGBA
err := printing.Image(app, img, printing.Options{Parent: s, Title: "发票"})
// 或者保存为 PDF：doc, _ := printing.ImagePDF(img); os.WriteFile("invoice.pdf", doc, 0o644)
```

要点：

- 打印用的页面函数最好单独写（白底、无动画、无滚动），并按纸张大小设置 `Width/Height`；多页内容就渲染多张图片，传给 `ImagePDF(img1, img2, ...)`。
- 图片打印出来是位图，文字不能被选中；需要矢量文字时，把要打印的内容做成 HTML，放进一个（可隐藏的）WebView 界面，再用 `Page` 或 `SavePDF`。

## 示例

[`examples/print-demo`](../examples/print-demo)：WebView 页面带“打印页面 / 导出 PDF / 打印 PDF 文档 / 打印图片”四个按钮。

```sh
go run ./examples/print-demo          # 交互
go run ./examples/print-demo -smoke   # 只把页面导出为临时 PDF 并检查 %PDF 头，不弹对话框
```

## 后端契约

见 `internal/platform/print_ext.go`：后端可选实现 `PagePrinter`（`PrintPage(surface, opts, done)`）和 `DocumentPrinter`（`PrintDocument(parent, pdf, opts, done)`），核心通过类型断言发现。页面导出 PDF 复用已有的 `WebPDF`，没有新增接口。fake 后端实现了两者，并通过 `PrintJobs` / `SetPrintErr` 供测试使用。
