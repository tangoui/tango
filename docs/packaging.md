# 安装包界面定制（tan package）

`tan package` 为桌面目标生成安装包：macOS 生成 `.dmg`，Windows 生成 NSIS 的 `*-setup.exe`，Linux 的格式见 [linux-packaging.md](linux-packaging.md)，Windows 签名与 MSIX 见 [windows.md](windows.md)。本文说明 tan.json 中的 `installer` 段。这一段的所有字段都可以不写，不写时也会得到可用的默认安装界面。

```json
"installer": {
  "windows": {
    "scope": "user",
    "license": "LICENSE",
    "welcome": true,
    "sidebarImage": "build/installer/sidebar.png",
    "headerImage": "build/installer/header.png",
    "runAfterInstall": true,
    "desktopShortcut": "ask",
    "languageDialog": false,
    "include": "build/installer.nsh",
    "template": "build/windows/installer.nsi"
  },
  "macos": {
    "dmg": {
      "background": "build/dmg-background.png",
      "windowSize": [540, 380],
      "iconSize": 128,
      "appPosition": [140, 190],
      "applicationsPosition": [400, 190],
      "title": ""
    }
  }
}
```

路径都相对于项目目录。

## Windows（NSIS）

| 字段 | 默认 | 说明 |
|---|---|---|
| `scope` | `user` | `user`：装到 `%LOCALAPPDATA%\Programs\<名称>`，写 HKCU，不弹 UAC，应用可以自更新（`service/updater` 的 `nsis` 格式）。`machine`：装到 `Program Files`（386 架构是 `Program Files (x86)`），写 HKLM，`RequestExecutionLevel admin`，开始菜单和桌面快捷方式对所有用户可见。 |
| `license` | — | `.txt`、`.rtf` 或无扩展名的纯文本文件。设置后增加许可协议页，用户必须同意才能继续。 |
| `welcome` | `true` | 是否显示欢迎页。 |
| `sidebarImage` | NSIS 自带 | 欢迎页和完成页左侧的图，**164×314**。可以是 `.bmp`，也可以是 `.png`（tan 叠到白底上，转成 24 位 BMP）。尺寸不对时 NSIS 会拉伸。 |
| `headerImage` | 无 | 其余页面顶部右侧的横幅，**150×57**，格式同上。 |
| `runAfterInstall` | 当前用户安装为 `true`，全机（`machine`）安装为 `false` | 完成页上的"运行 <名称>"复选框。全机安装显式开启时，tan 通过 explorer.exe 以非管理员身份启动应用，并给出警告。 |
| `desktopShortcut` | `ask` | `ask`：完成页上的"创建桌面快捷方式"复选框，默认勾选。`always`：总是创建。`never`：不创建。卸载时都会删除桌面快捷方式。 |
| `languageDialog` | `false` | 启动时弹出语言选择框，选择结果会被记住，卸载器沿用。默认不弹框，跟随 Windows 显示语言。 |
| `include` | — | NSIS 脚本片段，见下文"钩子宏"。 |
| `template` | — | Go `text/template` 文件，用它整份替换默认脚本，见下文"模板"。 |

安装器图标使用和 exe 相同的 Windows 图标：`icons.windows`，或从主图标派生出的形状。tan 把它写成 `installer.ico`，同时用作 `MUI_ICON` 和 `MUI_UNICON`。

生成的文件都在 `build/windows-<arch>/` 下：`installer.nsi`（每次打包都会重新生成）、`installer.ico`、`installer-sidebar.bmp`、`installer-header.bmp`。

### 语言

安装器的语言来自 `localizations`，与 Apple 平台的 `CFBundleLocalizations` 是同一份列表（`Config.AppleLocalizations()`），默认是 en、zh-Hans、zh-Hant、ja。语言标签按下表映射到 NSIS 语言：

| 标签 | NSIS |
|---|---|
| `en`、`en-*` | English |
| `zh-Hans`、`zh`、`zh-CN`、`zh-SG` | SimpChinese |
| `zh-Hant`、`zh-TW`、`zh-HK`、`zh-MO` | TradChinese |
| `ja` / `ko` | Japanese / Korean |
| `fr` `de` `es` `it` `pt` `pt-BR` `ru` `uk` `pl` `cs` `sk` `nl` `sv` `da` `fi` `nb`/`no` `nn` `tr` `el` `hu` `ro` `bg` `hr` `sr` `sl` `ar` `he` `fa` `hi` `th` `vi` `id` `ms` `ca` `et` `lv` `lt` | 对应的 NSIS 语言 |

- 映射时先取完整标签；匹配不到再依次去掉末尾的段，例如 `fr-CA` → `fr`。
- 映射后重复的语言只保留一次。列表里第一个语言是兜底语言：Windows 的显示语言不在列表中时使用它。
- 列表中没有英语时，会自动把 English 加到末尾。
- 没有对应 NSIS 语言的标签会在打包时给出警告，然后跳过。
- 页面上的文字由 Modern UI 自带翻译。脚本自己的文字（目前是"创建桌面快捷方式"）用 `LangString` 提供英、简、繁、日四种语言，其他语言显示英文。
- 脚本文件以 UTF-8 BOM 开头，所以 makensis 在任何代码页下都能读对中日文。

### 钩子宏（include）

`include` 指向的文件会在 `MUI2.nsh` 之后被 `!include`。如果其中定义了下面这些宏，它们会在对应时机展开；没有定义的宏会被跳过（脚本用 `!ifmacrodef` 判断），不会报错：

| 宏 | 时机 |
|---|---|
| `customInit` | 安装器的 `.onInit`（显示任何页面之前） |
| `customInstall` | 安装区段末尾：文件、快捷方式和注册表项都写完之后 |
| `customUnInstall` | 卸载区段开头：删除任何文件之前 |

```nsis
!macro customInstall
  CreateDirectory "$INSTDIR\data"
  WriteRegStr HKCU "Software\Example\Hello" "Installed" "1"
!macroend

!macro customUnInstall
  DeleteRegKey HKCU "Software\Example\Hello"
  RMDir /r "$INSTDIR\data"
!macroend
```

### 模板（template）

设置了 `template` 时，tan 用 Go `text/template` 渲染这个文件，结果整份替换默认脚本。模板引用了不存在的变量时会报错（`missingkey=error`）。默认脚本本身也是用同一套变量渲染出来的，可以照着 `build/windows-<arch>/installer.nsi` 来写模板。

| 变量 | 示例 / 说明 |
|---|---|
| `.Name` `.ID` `.Version` `.Copyright` | tan.json 中对应的字段 |
| `.VersionQuad` | `1.2.3.0`，用于 `VIProductVersion` |
| `.Arch` | `amd64` / `arm64` / `386` |
| `.Exe` | 安装后的文件名，例如 `Hello.exe` |
| `.ExeSource` | 构建产物的绝对路径（`File` 指令的参数） |
| `.OutFile` | 要生成的 setup.exe 的路径 |
| `.Scope` `.RegRoot` `.ExecutionLevel` | `user`/`HKCU`/`user`，或 `machine`/`HKLM`/`admin` |
| `.InstallDir` | NSIS 表达式，例如 `$LOCALAPPDATA\Programs\Hello` |
| `.UninstallKey` | `Software\Microsoft\Windows\CurrentVersion\Uninstall\<id>` |
| `.ShortcutName` | 快捷方式文件名（不含 `.lnk`） |
| `.Languages` | 列表，每项有 `.Tag`（如 `zh-Hans`）和 `.Name`（如 `SimpChinese`） |
| `.LanguageDialog` `.Welcome` `.RunAfterInstall` | 布尔值 |
| `.DesktopShortcut` | `ask` / `always` / `never` |
| `.Icon` `.SidebarImage` `.HeaderImage` `.License` `.Include` | 绝对路径，没有时为空字符串 |
| `.RegisterAssociations` `.UnregisterAssociations` | 注册和反注册 URL scheme 与文件关联的 NSIS 语句 |
| `.LangStrings` | 脚本自身文字的 `LangString` 语句 |

模板中可用的函数：

- `q`：把字面字符串加上引号，并转义其中的 `$` 和 `"`。
- `qv`：加引号但保留 NSIS 变量，例如 `{{qv .InstallDir}}`。
- `upper`

```nsis
Unicode true
!include "MUI2.nsh"
Name {{q .Name}}
OutFile {{q .OutFile}}
InstallDir {{qv .InstallDir}}
RequestExecutionLevel {{.ExecutionLevel}}
!insertmacro MUI_PAGE_INSTFILES
{{range .Languages}}!insertmacro MUI_LANGUAGE "{{.Name}}"
{{end}}
Section
  SetOutPath "$INSTDIR"
  File {{q .ExeSource}}
SectionEnd
```

使用模板时，签名流程不变：先签 exe，再运行 makensis，最后签 setup.exe。

### makensis

- 优先使用 PATH 中的 `makensis`。
- **Windows 主机**：如果 PATH 中没有，tan 会下载固定版本的 NSIS 3.13 官方 zip（SourceForge），用内置的 SHA-256 做校验，然后解压到 `%LOCALAPPDATA%\tango\nsis\nsis-3.13\`（用户缓存目录），之后的打包直接复用。下载有 5 分钟超时和 64 MB 上限，校验失败时不保留任何文件。
- **macOS / Linux**：需要自行安装，`brew install makensis`；`sudo apt install nsis`；`sudo dnf install mingw32-nsis`。没有 makensis 时只生成 `installer.nsi`，并给出警告。

### 卸载与升级

- 卸载器删除 exe、`uninstall.exe`、安装目录（仅在目录为空时）、开始菜单和桌面快捷方式、卸载注册表项，以及安装时写入的 URL scheme 和文件关联。`machine` 安装的卸载器同样使用所有用户的 shell 目录和 HKLM。
- `InstallDirRegKey` 会记住上次的安装目录。`/S /D=<目录>` 静默安装（自更新使用这种方式）不显示完成页，所以不会运行应用，也不会新建桌面快捷方式。
- `machine` 安装默认不在完成页提供"运行"：安装器以管理员权限运行，直接启动会让应用也带着管理员权限。显式开启 `runAfterInstall` 时，tan 改为通过 explorer.exe 以当前登录用户、非提升身份启动（这是社区通行做法，没有官方文档背书，请在目标系统上验证）。

## macOS 磁盘映像

| 字段 | 默认 | 说明 |
|---|---|---|
| `background` | 无（白底） | PNG、JPEG 或 TIFF。按图片的**点**尺寸画在窗口内容区左上角：72 dpi 的图 1 像素 = 1 点。如果 `x.png` 旁边有 `x@2x.png`，tan 会用 `tiffutil -cathidpicheck` 把两张合成多分辨率的 `background.tiff`，用于 Retina。 |
| `windowSize` | `[540, 380]` | 窗口内容区尺寸（点），标题栏由 Finder 另外加上。背景图建议与它同尺寸：540×380，Retina 版 1080×760。 |
| `iconSize` | `128` | 16–512 |
| `appPosition` | `[140, 190]` | app 图标中心，从内容区左上角起算 |
| `applicationsPosition` | `[400, 190]` | Applications 链接图标的中心 |
| `title` | 应用名 | 卷名，即 Finder 窗口标题。不能含 `:` 或 `/`。 |

镜像里包含：`<名称>.app`、指向 `/Applications` 的符号链接、`.DS_Store`（窗口布局）、`.background/`（背景图），以及 `.VolumeIcon.icns`（app 的图标；卷上设置了"自定义图标"标志，Finder 和桌面会显示它）。这几个文件名都以点开头，并设置了 `UF_HIDDEN`。用户打开"显示隐藏文件"时，它们会排在窗口可见区域的右侧之外。

### 实现：纯 Go 写 .DS_Store

tan **不用 AppleScript 驱动 Finder**。那种做法需要图形会话，在 CI 上不稳定。tan 自己写出 Finder 的 `.DS_Store`（`cmd/tan/dsstore.go`），格式依据公开资料：

- 文件头是 `Bud1` 伙伴分配器：info 块记录块地址表、目录（`DSDB`）和 32 条空闲链表；所有块按 2 的幂分配，并且整个 2³¹ 地址空间要么属于某个块，要么在空闲链表上。
- `DSDB` 是 B 树头。B 树只有一个叶子节点，记录按文件名（不区分大小写）和属性码排序。
- `.` 的 `bwsp`（窗口：位置尺寸、隐藏工具栏、侧栏、状态栏）和 `icvp`（图标视图：图标大小、不自动排列、背景）是二进制 plist（bplist00，tan 自己编码）；每个图标的位置是一条 `Iloc` 记录。
- 背景图由 `icvp` 中的 `backgroundImageAlias` 引用。这是 Alias Manager 第 2 版记录，需要卷名、卷创建日期、文件及其父目录的 catalog id。所以流程是：`hdiutil create -format UDRW` → `hdiutil attach -nobrowse`（不在 Finder 中出现）→ 读取挂载后文件的 id 和日期，写入 `.DS_Store`，设置卷图标标志（`com.apple.FinderInfo` 中的 `kHasCustomIcon`），删除 `.fseventsd` → `hdiutil detach` → `hdiutil convert -format ULMO`（LZMA 压缩，macOS 10.15 起可挂载）。

alias 固定字段中的日期使用本地时间，与 Finder 一致；另有 UTC 高精度日期标签。生成的 alias 已逐字节对照 Finder 本机写出的版本，同一卷同一文件下，只差 Finder 额外附带的"磁盘映像文件自身的 alias"（tag 20）。

## 平台限制

- `.dmg` 只能在 macOS 上生成（依赖 `hdiutil`、`ditto`；Retina 背景还需要 `tiffutil`）。在其他平台上会跳过并给出警告。
- 卷名含非 ASCII 字符时，alias 固定字段里的卷名会被降为 ASCII（Unicode 卷名放在 tag 15 中）。
- NSIS 安装包可以在任何平台生成，只要有 makensis。只有 Windows 主机会自动下载 NSIS。
- 安装包中不包含 WebView2（Windows 10 1803 及以上、Windows 11 都自带）。
