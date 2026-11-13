# Windows：代码签名与 MSIX

本文说明 `tan sign` 和 `tan package` 在 Windows 目标上的签名流程，以及 `tan package -format msix`。

> 可执行文件里的资源（图标、应用清单 manifest、版本信息，即 `.syso`）由另一项工作负责，不在本文范围内。

## tan.json 的 `windows` 节

```json
{
  "windows": {
    "certificate": "windows/codesign.pfx",
    "timestampURL": "http://timestamp.digicert.com",
    "publisher": "CN=Example Ltd, O=Example Ltd, C=US"
  }
}
```

| 字段 | 用途 | 校验 |
|---|---|---|
| `certificate` | 代码签名证书，PKCS#12 文件（`.pfx`/`.p12`） | 必须是相对项目目录的路径，扩展名为 `.pfx` 或 `.p12` |
| `timestampURL` | RFC 3161 时间戳服务器，默认 `http://timestamp.digicert.com` | 必须是 `http://` 或 `https://` 开头 |
| `publisher` | 证书主题（Subject），MSIX 的 `Identity Publisher` 用到，必须与签名证书的主题完全一致 | 必须以 `CN=` 开头；用 `-format msix` 打包时必填 |

**证书密码不能写进 tan.json**（`password` 字段会因为是未知字段而报错），只能来自环境变量或命令行参数。

## 代码签名

### 证书与密码从哪里来

| 项 | 优先级（从高到低） |
|---|---|
| 证书路径 | `-cert` 参数 > 环境变量 `TANGO_WINDOWS_CERT` > tan.json `windows.certificate` |
| 证书密码 | `-password` 参数 > 环境变量 `TANGO_WINDOWS_CERT_PASSWORD` |
| 时间戳服务器 | `-timestamp` 参数 > tan.json `windows.timestampURL` > `http://timestamp.digicert.com` |

缺少证书或密码时，tan 会明确报错并指出该设置哪个参数或变量。

**密码在进程列表中的可见性**：用 osslsigncode 签名时，tan 把密码写入一个权限 0600 的临时文件，用 `-readpass` 传入，用完即删，不会出现在命令行上。signtool 只支持 `/p <密码>`（没有文件、环境变量或标准输入方式），所以用 signtool 签名时密码会短暂出现在进程命令行里，tan 每次运行会给出一次警告；在共享机器上签名请改用 osslsigncode，或使用证书存储/硬件令牌（`/sha1`、`/csp`）。macOS 公证的 notarytool 同理只支持 `--password`，建议用 `tan` 的 `-profile`（钥匙串中保存的凭据）。推荐在 CI 里用环境变量传密码：命令行参数会出现在进程列表里。tan 自己的输出与错误信息里，密码一律被替换成 `******`。

### 签名工具

- **Windows**：优先使用 Windows SDK 的 `signtool`（需在 PATH 中，通常位于 `C:\Program Files (x86)\Windows Kits\10\bin\<版本>\x64`）。等价命令：

  ```
  signtool sign /fd sha256 /f cert.pfx /p <密码> /tr <时间戳URL> /td sha256 <文件>
  ```

  找不到 signtool 时退回 `osslsigncode`。
- **macOS / Linux**：使用 `osslsigncode`（macOS：`brew install osslsigncode`；Debian/Ubuntu：`apt install osslsigncode`；Fedora：`dnf install osslsigncode`）。osslsigncode 不能原地签名，tan 先输出到同目录的临时文件，成功后原子替换原文件（保留文件权限），失败时原文件不变：

  ```
  osslsigncode sign -pkcs12 cert.pfx -readpass <临时文件> -h sha256 -ts <时间戳URL> -in <文件> -out <文件>.signing.tmp
  ```

两种工具都没有时，tan 报错并给出对应平台的安装方式。

### tan sign

`tan sign` 按文件扩展名分派：

- `.exe`、`.msi`、`.msix`、`.appx`：Windows Authenticode 签名（可一次传多个文件）。
- `.app`、`.dmg`：仍走原有的 macOS codesign / 公证流程，行为不变。

```sh
export TANGO_WINDOWS_CERT=$PWD/windows/codesign.pfx
export TANGO_WINDOWS_CERT_PASSWORD=...
tan sign build/windows-amd64/Hello.exe build/Hello-1.0.0-windows-amd64-setup.exe
```

Windows 文件不需要 tan.json；有 tan.json 时会读取其中的 `windows.certificate` 与 `windows.timestampURL`。`-notarize` 只用于 macOS，对 Windows 文件会报错。

### tan package 中的签名

`tan package -platform windows/amd64`（NSIS 安装包）（安装界面、语言、钩子与模板见 [packaging.md](packaging.md)）：

1. 配置了证书（`TANGO_WINDOWS_CERT` 或 `windows.certificate`）时：先签名构建出的 app exe，再交给 NSIS 打包，最后签名生成的 `*-setup.exe`。此时缺少密码或签名工具会直接报错。
2. 未配置证书时：跳过签名，每次运行只警告一次（未签名的程序会触发 SmartScreen 提示）。

## MSIX

```sh
tan package -platform windows/amd64 -format msix
```

`-format` 默认为空，即各平台原有的产物（macOS 磁盘映像、Linux tar.gz 与 AppImage、Windows NSIS 安装包）。`msix` 只能用于 Windows 目标，且要求 tan.json 中有 `windows.publisher`。

tan 生成布局目录 `build/<Exe>-<版本>-windows-<arch>-msix/`：

- `<Exe>.exe`（配置了证书时先签名）
- `AppxManifest.xml`：
  - `Identity`：`Name` = tan.json `id`（MSIX 要求 3–50 个字符），`Publisher` = `windows.publisher`，`Version` = 版本号的 `a.b.c.0` 形式（丢弃预发布/构建后缀），`ProcessorArchitecture` = `x64` / `arm64`
  - `Properties`：显示名、发布者显示名（取自 `publisher` 的 CN）、商店徽标
  - `Dependencies`：`TargetDeviceFamily Windows.Desktop`，`MinVersion 10.0.17763.0`（Windows 10 1809）
  - `Applications/Application`：`EntryPoint="Windows.FullTrustApplication"`，带 `uap:VisualElements`；tan.json 的 `urlSchemes` 与 `fileAssociations` 转为 `windows.protocol` / `windows.fileTypeAssociation` 扩展
  - `Capabilities`：`rescap:runFullTrust`
- `Assets/`：Windows 桌面图标（`icons.windows` 为 PNG 时用它，否则由 tan.json `icon` 派生的桌面形状，与 exe 图标同源）缩放得到的 `StoreLogo.png`（50）、`Square44x44Logo.png`（44）、`Square150x150Logo.png`（150）；没有 icon 时用 tan 的默认图标。

之后：

- PATH 中有 Windows SDK 的 `makeappx` 时执行 `makeappx pack /d <布局目录> /p <名称>.msix /o`，并在配置了证书时签名 `.msix`。
- 没有 `makeappx`（例如在 macOS / Linux 上）时只警告并保留布局目录，可在 Windows 上手动打包后再 `tan sign`。

### 限制

- Windows 只安装已签名的 MSIX，且签名证书的主题必须与 `Identity Publisher` 一致；自签名证书需要先导入到“受信任的人”存储中。
- 未生成 `Square310x310Logo`、宽磁贴、启动画面等可选资源，也没有按缩放比例（`.scale-200` 等）提供多份图片。
- 只覆盖桌面全信任应用；不处理商店提交所需的保留名称、多语言资源等。
- macOS / Linux 上无法打包 `.msix`（`makeappx` 只在 Windows SDK 中提供）。

## 验证状态

签名参数与流程、AppxManifest 内容由 `cmd/tan` 的单元测试覆盖（用假的 osslsigncode / makensis 脚本）。真实的 signtool、osslsigncode 签名、`makeappx` 打包与在 Windows 上安装运行尚未在真机上验证。
