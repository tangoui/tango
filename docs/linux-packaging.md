# Linux 打包：tar.gz、AppImage、deb、rpm、Flatpak

`tan package` 对 Linux 目标默认产出 tar.gz 和 AppDir/AppImage。用 `-format` 可以改选其他格式，逗号分隔可一次产出多种：

```sh
tan package -platform linux/amd64,linux/arm64 -format deb,rpm,flatpak
tan package -platform linux/amd64 -format tar,appimage,deb   # 等价于默认再加 deb
```

| `-format` | 产物（位于 `-o` 目录，默认 `build/`） | 外部工具 |
|---|---|---|
| （不写） | 同 `tar,appimage` | 同下 |
| `tar` | `<Exe>-<版本>-linux-<arch>.tar.gz`（含 `install.sh`） | 无 |
| `appimage` | `<Exe>-<arch>.AppDir/`；PATH 上有 `appimagetool` 时再生成 `.AppImage` | `appimagetool`（可选） |
| `deb` | `<Exe>-<版本>-linux-<arch>.deb` | 无（纯 Go 生成，不需要 dpkg-deb） |
| `rpm` | `<Exe>-<版本>-linux-<arch>.rpm` | 无（纯 Go 生成，不需要 rpmbuild） |
| `flatpak` | `<Exe>-<arch>.flatpak/<id>.json`（清单）+ `files/`；Linux 上有 `flatpak-builder` 时再生成 `<Exe>-<版本>-linux-<arch>.flatpak` | `flatpak-builder`、`flatpak`（可选） |

Linux 格式只能用于 Linux 目标（`-platform` 里混了其他系统会报错）；`-format msix` 仍只用于 Windows。deb、rpm 可以在任何系统（包括 macOS）上生成。

## tan.json 的 `linux` 节

全部可选：

```json
{
  "linux": {
    "package": "my-app",
    "maintainer": "Jane Doe <jane@example.com>",
    "description": "一句话简介\n更长的说明，可以多行。\n\n空行分段。",
    "homepage": "https://example.com",
    "license": "MIT",
    "depends": ["xdg-utils"],
    "rpmRequires": ["xdg-utils"],
    "flatpak": {
      "runtimeVersion": "50",
      "finishArgs": ["--socket=pulseaudio"]
    }
  }
}
```

| 字段 | 用途 | 默认 |
|---|---|---|
| `package` | deb/rpm 包名，须符合 Debian Policy 5.6.1：小写字母、数字、`+ - .`，至少两个字符 | `name` 转小写、单词用 `-` 连接（`My App` → `my-app`）；不足两个字符时用 `id` 的小写 |
| `maintainer` | deb `Maintainer`、rpm `Packager` | 无。Debian Policy 要求有 Maintainer，缺失时 tan 会警告，dpkg 也会警告但仍可安装 |
| `description` | 第一行是摘要（deb `Description` 首行、rpm `Summary`），其余是长描述 | `name` |
| `homepage` | deb `Homepage`、rpm `URL` | 无 |
| `license` | rpm `License`，也写进 deb 的 copyright 文件 | rpm 写 `Proprietary` |
| `depends` | 追加到 deb `Depends`（可带版本，如 `libfoo1 (>= 2)`） | 固定含 `libc6, libgtk-3-0, libwebkit2gtk-4.1-0` |
| `rpmRequires` | 追加到 rpm `Requires`，`名称` 或 `名称 运算符 版本`（`<` `<=` `=` `>=` `>`） | 固定含 `libc.so.6()(64bit)`、`libgtk-3.so.0()(64bit)`、`libwebkit2gtk-4.1.so.0()(64bit)` |
| `flatpak.runtimeVersion` | `org.gnome.Platform` / `org.gnome.Sdk` 分支 | `50` |
| `flatpak.finishArgs` | 追加的沙箱权限，必须以 `--` 开头 | 见下 |

Tango 运行时用 dlopen 加载 GTK 3 和 WebKitGTK 4.1，所以这两个库是默认依赖。rpm 用库的 soname 而不是包名，Fedora（gtk3、webkit2gtk4.1）和 openSUSE（libgtk-3-0、libwebkit2gtk-4_1-0）都能解析。

## 版本号

tan.json 的 `version` 是 SemVer 风格（`1.2.3-rc.1+build.5`）。deb 和 rpm 的版本里 `-` 另有含义，而预发布版本必须排在正式版之前，所以：预发布前的 `-` 换成 `~`（dpkg、rpm 都把 `~` 排在最前），构建元数据里的 `-` 换成 `.`。例如 `1.2.3-rc.1` → `1.2.3~rc.1`。rpm 的 Release 固定为 `1`。

## deb

按 deb(5) 与 Debian Policy 生成：ar 归档依次包含 `debian-binary`（`2.0`）、`control.tar.gz`（`control`、`md5sums`、`postinst`、`postrm`）、`data.tar.gz`。安装位置：

| 文件 | 位置 |
|---|---|
| 程序 | `/usr/bin/<Exe>` |
| 桌面文件 | `/usr/share/applications/<id>.desktop`（以应用 id 命名，与 GTK 的 Wayland app id 一致） |
| 图标 | `/usr/share/icons/hicolor/<n>x<n>/apps/<Exe>.png`，以及 `scalable/apps/<Exe>-symbolic.svg` |
| MIME 定义 | `/usr/share/mime/packages/<id>.xml`（有不带 `mime` 的文件关联时） |
| 版权 | `/usr/share/doc/<package>/copyright` |

`postinst`/`postrm` 在命令存在时才执行 `update-desktop-database`、`update-mime-database`、`gtk-update-icon-cache`（Debian 上这些缓存也由 dpkg 触发器刷新，重复刷新无害）。

安装与验证：

```sh
sudo apt install ./Ledger-0.1.0-linux-amd64.deb     # 自动装依赖
# 或：sudo dpkg -i Ledger-0.1.0-linux-amd64.deb && sudo apt-get install -f
dpkg -s ledger            # 包信息
dpkg -L ledger            # 文件列表
desktop-file-validate /usr/share/applications/dev.tango.ledger.desktop
Ledger                    # 启动
sudo apt remove ledger
```

`lintian` 仍会报：没有 changelog、没有手册页、二进制未启用 PIE/RELRO（Go 默认构建方式），这些不影响安装；要进 Debian 官方仓库需另行处理。

## rpm

按 RPM 官方文件格式（v4 包：96 字节 lead + 签名头 + 头部 + gzip 压缩的 cpio（newc）载荷）纯 Go 生成。签名头里有头部的 SHA-256/SHA-1、头部加载荷的 MD5 和大小，头部里有每个文件的 SHA-256 和载荷的 SHA-256。安装位置同 deb（无 copyright 文件），`%post`/`%postun` 同样刷新缓存。

包**没有 OpenPGP 签名**（与 rpmbuild 刚产出、尚未 `rpmsign` 的包一样）。RPM 6（Fedora 43 起）默认要求签名，所以：

```sh
sudo dnf install ./Ledger-0.1.0-linux-amd64.rpm    # dnf 对本地文件不检查签名，会自动装依赖
rpm -qip Ledger-0.1.0-linux-amd64.rpm              # 包信息
rpm -qlp Ledger-0.1.0-linux-amd64.rpm              # 文件列表
rpm -Kv Ledger-0.1.0-linux-amd64.rpm               # 摘要校验（签名项显示 NOTFOUND）
sudo rpm -i --nosignature Ledger-0.1.0-linux-amd64.rpm   # RPM 6 直接用 rpm 安装时
rpm -V ledger                                      # 校验已安装文件
sudo rpm -e ledger
```

正式分发应使用自己的 GPG 密钥签名：`rpmsign --addsign <包>`（需要 rpm-sign 和 `%_gpg_name` 配置），并让用户导入公钥。

## Flatpak

生成 flatpak-builder 清单（JSON）。应用 ID 即 tan.json 的 `id`，运行时 `org.gnome.Platform`（含 GTK 3 与 WebKitGTK 4.1），SDK `org.gnome.Sdk`。唯一的模块用 `simple` 构建系统，把 `files/` 里已构建好的程序和资源 `install` 到 `/app`；导出的文件都以应用 ID 命名（Flatpak 的要求）：`<id>.desktop`（`Icon=<id>`）、`<id>.png`、`<id>-symbolic.svg`、`<id>.xml`。

默认沙箱权限（`finish-args`）：

| 权限 | 原因 |
|---|---|
| `--share=ipc` | X11 共享内存 |
| `--socket=wayland`、`--socket=fallback-x11` | 优先 Wayland，没有时退回 X11 |
| `--device=dri` | GPU 加速（WebKit 合成） |
| `--share=network` | 网页内容通常需要网络 |

不默认开放任何文件系统路径；文件对话框经由 GTK 的文件选择门户（portal）访问用户选中的文件。需要声音、特定目录等时在 `linux.flatpak.finishArgs` 里追加，如 `--socket=pulseaudio`、`--filesystem=xdg-documents`。

在 Linux 上、`flatpak-builder` 在 PATH 上时，tan 直接构建；否则只写清单并打印命令。手动构建：

```sh
flatpak remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
flatpak install --user flathub org.gnome.Platform//50 org.gnome.Sdk//50
cd build/Ledger-amd64.flatpak
flatpak-builder --arch=x86_64 --force-clean --repo=repo build dev.tango.ledger.json
flatpak build-bundle --arch=x86_64 repo ../Ledger-0.1.0-linux-amd64.flatpak dev.tango.ledger
```

安装与验证：

```sh
flatpak install --user ./Ledger-0.1.0-linux-amd64.flatpak
flatpak info --show-permissions dev.tango.ledger
flatpak run dev.tango.ledger
flatpak uninstall --user dev.tango.ledger
```

在 macOS 上生成的 flatpak 清单要拷到 Linux 上构建；交叉架构构建（如在 x86_64 机器上构建 aarch64）需要 qemu-user 与 binfmt 支持。上架 Flathub 另需 AppStream metainfo 文件和 Flathub 的审核流程，tan 目前不生成 metainfo。
