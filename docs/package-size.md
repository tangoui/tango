# 应用包体积（实测）

Tango 默认桌面模板的发布构建中，WebView 可执行文件约 **7.86–8.69 MB**，Go 自绘 UI 可执行文件约 **9.77–11.65 MB**；各平台安装包（macOS `.dmg`，Windows NSIS `.exe`，Linux `.deb`、`.rpm`、`.AppImage`、`.tar.gz`）约 **2.78–5.11 MB**。

这些数字描述应用文件与安装包，不是运行内存、框架源码大小或理论最小值。业务代码、前端资源、字体、媒体文件、额外依赖和签名都会影响最终结果。

## 测量条件

- 日期：2026-11-13。
- 源码：提交 `36322bce11341ab6476aa6b23ce665d05ea74ea0` 的干净 worktree；未使用工作区中未提交的改动。
- 构建主机与 Go：macOS arm64，`go1.27.2`。全部目标都在这台机器上交叉构建与打包，AppImage 一步在本机 podman 的 linux/arm64 容器中完成。
- 样本：用 `tan init` 生成默认 `vanilla` 和 `ui` 项目，名称分别为 `SizeWeb`、`SizeUI`；通过 `-local` 引用同一源码快照。
- `vanilla` 包含默认 HTML/CSS/JS、IPC 与绑定服务；`ui` 包含默认的自绘表单、表格和主题切换。它们是入门应用样本，不是空程序，也不代表链接所有 Tango 扩展后的体积。
- 发布参数：`tan package` 的默认生产构建，`CGO_ENABLED=0`、`-trimpath`、`-ldflags="-s -w"`；Windows 另有 `-H windowsgui`。没有启用 `tango_dev`，也没有使用 UPX。
- 单位：**1 MB = 1,000,000 字节；1 MiB = 1,048,576 字节**。下文和 README 使用 MB，精确字节数见 [JSON](package-size-data.json)。

## 安装包

| 平台 / 架构 | 格式 | WebView | Go 自绘 UI |
| --- | --- | ---: | ---: |
| macOS / arm64（Apple 芯片） | `.dmg` | 3.51 MB | 4.57 MB |
| macOS / amd64（Intel） | `.dmg` | 3.86 MB | 5.11 MB |
| Windows / amd64 | NSIS `.exe` | 2.78 MB | 3.64 MB |
| Linux / amd64 | `.deb` | 3.49 MB | 4.50 MB |
| Linux / amd64 | `.rpm` | 3.49 MB | 4.51 MB |
| Linux / amd64 | `.AppImage` | 4.10 MB | 5.02 MB |
| Linux / amd64 | `.tar.gz` | 3.58 MB | 4.62 MB |
| Linux / arm64 | `.deb` | 3.18 MB | 4.06 MB |
| Linux / arm64 | `.rpm` | 3.19 MB | 4.06 MB |
| Linux / arm64 | `.AppImage` | 3.77 MB | 4.56 MB |
| Linux / arm64 | `.tar.gz` | 3.26 MB | 4.15 MB |

AppImage 比其他格式大约 0.5 MB，是因为它带有约 0.94 MB 的自解压运行时（type2 runtime），并使用 squashfs 压缩。

## 可执行文件

| 平台 / 架构 | WebView | Go 自绘 UI |
| --- | ---: | ---: |
| macOS / arm64 | 8.08 MB | 10.54 MB |
| macOS / amd64 | 8.69 MB | 11.65 MB |
| Windows / amd64 | 8.42 MB | 11.10 MB |
| Linux / amd64 | 8.30 MB | 10.67 MB |
| Linux / arm64 | 7.86 MB | 9.77 MB |

macOS 一列只计 `.app/Contents/MacOS/` 中的主程序。整个 `.app`（含 ICNS 图标）的逻辑文件长度之和：arm64 为 8.26 MB / 10.72 MB，amd64 为 8.87 MB / 11.83 MB（WebView / 自绘 UI）。应用包大小使用文件长度统计，不使用文件系统分配块数量；Finder 显示的占用空间可能略有不同。

## 分发格式与本次限制

- **macOS**：`tan package` 生成的 DMG（`hdiutil`，UDZO、zlib 9 级压缩），内含带 ICNS 图标的 `.app` 和指向「应用程序」的链接；四个 DMG 均通过 `hdiutil verify`。Intel 与 Apple 芯片是两个独立的 DMG，未测 universal 包。应用为 ad-hoc 签名，未做 Developer ID 签名与公证，正式签名会略微增加体积。
- **Windows**：`tan package` 通过本机 NSIS **3.13** 生成的 amd64 安装器；应用和安装器均未做 Authenticode 签名。未测 Windows arm64 与 MSIX。
- **Linux**：
  - `.deb`、`.rpm`、`.tar.gz` 由 tan 自己写出（不依赖 `dpkg-deb`、`rpmbuild`），包含程序、桌面入口和图标；deb 与 rpm 声明了 GTK 3、WebKitGTK 4.1 依赖，由系统包管理器安装。样本的 `tan.json` 没有设置 `linux.maintainer`，打包时有警告，正式发布应补上。
  - `.AppImage`：tan 生成 AppDir，再用 appimagetool（continuous build 370，aarch64 版）在 linux/arm64 容器中打包，用 `--runtime-file` 指定对应架构的 type2 runtime。tan 不会自动下载 appimagetool，PATH 中没有时只输出 AppDir。
  - 未测 Flatpak：需要 `flatpak-builder` 和 GNOME 运行时，体积口径也和上面不同（运行时单独安装）。
- **移动端**：本轮没有构建或测量 APK、AAB、IPA。

这些限制也是 README 数字的适用边界。带自定义资源和正式签名的最终应用需要重新测量。

## 安装与运行验证

在 Debian 12 arm64 容器（已装 WebKitGTK 2.50）中：

- `dpkg -i` 安装 linux-arm64 WebView 的 `.deb` 成功，文件落到 `/usr/bin`、`/usr/share/applications`、`/usr/share/icons/hicolor`；在 `xvfb-run` 下启动后持续运行，8 秒后被超时结束，没有崩溃。
- linux-arm64 Go 自绘 UI 的 `.AppImage` 以 `--appimage-extract-and-run` 在 `xvfb-run` 下启动后持续运行（无 GPU，回退到 CPU 渲染）。
- 四个 `.rpm` 用 Fedora 容器中的 `rpm -qip`、`rpm -qRp`、`rpm -K` 检查元数据、依赖与摘要，未实际安装。

amd64 的 Linux 包、Windows 安装器与 Intel DMG 只验证了生成和结构，未在对应机器上安装运行。

## 系统依赖与 CLI

WebView 应用使用系统 WebView，并没有把完整浏览器引擎装进上述安装包。目标机器需要的 WebView2、GTK / WebKitGTK 等系统依赖不计入这些数字；AppImage 也不捆绑 GTK / WebKitGTK，仍依赖系统提供。

本轮 `tan` 开发工具自身的 macOS arm64 发布可执行文件为 **12.99 MB**。它是开发者使用的 CLI，**不会随每个应用一起分发**，不应加到上表的应用体积中。

## 复现步骤

在 macOS 的 Tango 仓库根目录，使用相同提交与 Go 版本执行。DMG 需要系统自带的 `hdiutil`，Windows 安装器要求 PATH 中有 `makensis`；deb、rpm、tar.gz 不需要额外工具。

```bash
TANGO_SOURCE="$(pwd)"
TANGO_SIZE_DIR="$(mktemp -d)"

CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
  -o "$TANGO_SIZE_DIR/tan" ./cmd/tan

"$TANGO_SIZE_DIR/tan" init -template vanilla -mobile=false \
  -name SizeWeb -local "$TANGO_SOURCE" "$TANGO_SIZE_DIR/vanilla"
"$TANGO_SIZE_DIR/tan" init -template ui -mobile=false \
  -name SizeUI -local "$TANGO_SOURCE" "$TANGO_SIZE_DIR/ui"

for sample in vanilla ui; do
  "$TANGO_SIZE_DIR/tan" package -C "$TANGO_SIZE_DIR/$sample" \
    -platform darwin/arm64,darwin/amd64,windows/amd64 -o "$TANGO_SIZE_DIR/packages/$sample"
  "$TANGO_SIZE_DIR/tan" package -C "$TANGO_SIZE_DIR/$sample" \
    -platform linux/amd64,linux/arm64 -format tar,deb,rpm,appimage \
    -o "$TANGO_SIZE_DIR/packages/$sample"
done
```

没有 appimagetool 时，最后一步只生成 `<名称>-<架构>.AppDir`。在 Linux 上（或容器中）对每个 AppDir 运行：

```bash
ARCH=x86_64 appimagetool --no-appstream --runtime-file runtime-x86_64 \
  SizeWeb-amd64.AppDir SizeWeb-0.1.0-linux-amd64.AppImage
# arm64：ARCH=aarch64，runtime-aarch64
```

runtime 文件来自 [AppImage/type2-runtime](https://github.com/AppImage/type2-runtime/releases)。源码、工具链或打包环境变化后，应重新运行，不把本表当作体积上限。

## 原始数据与校验

[原始字节数、构建口径、工具版本和 SHA-256](package-size-data.json)记录了十个“模板 × 平台”样本、它们的全部安装包和 CLI。JSON 中的产物路径相对本轮临时测量目录，不是仓库文件链接。
