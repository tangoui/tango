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

## 桌面构建标签

`tan.json` 的可选字段 `desktopBuildTags`（例如 `["production"]`）适用于 `tan build`（含 `-debug`）和 `tan package` 的桌面主程序。每项必须是一个 Go 构建标签。tan 将它们与 `GOFLAGS` 中的标签合并，重复项只传一次。Tango 保留 `tango_dev`、`tango_release` 和 `tango_backend_` 前缀的标签。后端配置由 `features` 或 `-features` 选择，CGO 由 `cgo` 字段开关。

`tan dev` 和移动端构建会校验这项配置，编译时沿用各自的标签设置。桌面开发构建的额外标签可通过 `GOFLAGS` 提供。

## 随包命令行工具

`tan.json` 的 `desktopHelpers` 是可选列表。每项设置 `name` 和 `build`。名称用字母或数字开头，后面可用字母、数字、下划线和连字符，忽略大小写后须唯一。`build` 从项目根目录执行。例如：

```json
"desktopHelpers": [{ "name": "helper", "build": "node scripts/build-helper.mjs" }]
```

`tan build`、`tan package` 对每个桌面目标传入 `TAN_TARGET_OS`、`TAN_TARGET_ARCH` 和绝对路径 `TAN_HELPER_OUTPUT`。构建命令须自行按目标设置交叉编译参数，并在指定路径写出非空的常规文件。Windows 输出路径已含 `.exe`，macOS 通用包的目标架构为 `universal`。移动端构建跳过这些命令。

- macOS 的辅助工具位于 `.app/Contents/Helpers/`。对 `.app` 运行 `tan sign` 时，tan 先用同一身份签此目录中的可执行文件，再签整个 `.app`。应用的 entitlements 只用于外层 `.app`。
- Windows：主程序旁的 `cli/`，内置 NSIS 脚本会安装并卸载这些文件，MSIX 布局也会包含它们。
- Linux：构建目录、AppImage 和解压的 tar 包在主程序旁的 `cli/`；已安装的 tar 包、DEB、RPM、Flatpak 使用 `lib/<应用可执行名>/`。

使用 `desktopHelpers` 构建 Linux 目标时，主程序名须避开辅助工具的 `cli` 目录名（忽略大小写）。

## Windows（NSIS）

| 字段 | 默认 | 说明 |
|---|---|---|
| `scope` | `user` | `user`：装到 `%LOCALAPPDATA%\Programs\<名称>`，写 HKCU，不弹 UAC，应用可以自更新（`service/updater` 的 `nsis` 格式）。`machine`：装到 `Program Files`（386 架构是 `Program Files (x86)`），写 HKLM，`RequestExecutionLevel admin`，开始菜单和桌面快捷方式对所有用户可见。 |
| `license` | — | `.txt`、`.md`、`.rtf` 或无扩展名的纯文本文件。设置后增加许可协议页，用户必须同意才能继续。 |
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
| `.Helpers` | 随包命令行工具，每项含 `.Name` 和绝对路径 `.Source`；自定义模板需自行安装和卸载 |
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

## macOS 输入法

在 `tan.json` 中设置顶层 `inputMethod` 后，`tan build` 会生成可供 macOS 加载的输入法应用包。应用的 `id` 必须含 `.inputmethod.`，例如 `com.example.inputmethod.pinyin`。

```json
"inputMethod": {
  "language": "zh-Hans",
  "icon": "assets/menu-icon.png",
  "modes": [
    {"id": "zh", "label": "中文", "short": "中", "icon": "assets/mode-zh.tiff"},
    {"id": "en", "label": "英文", "short": "英", "icon": "assets/mode-en.tiff"}
  ]
}
```

- `language` 是输入法的主要语言，默认为 `zh-Hans`。
- `icon` 是项目内的菜单图标 PNG。省略时使用应用图标（如果有）。
- `connection` 默认为 `<id>_Connection`。macOS 从应用包读取该值。若在 `ime.Handler.Connection` 中设置覆盖值，两者必须一致。
- `modes` 按显示顺序声明可选模式。每项需要唯一的 `id`、显示名 `label` 和状态短文字 `short`。应用用 `ime.SetMode` 传入模式 `id`。macOS 输入源 ID 采用 `<应用 id>.<模式 id>`。模式 `en` 的语言为英语，其余模式沿用 `language`。每个模式可以覆盖菜单图标，省略时使用输入法的默认图标。
- `windows.clsid`、`windows.profile` 和 `windows.langid` 配置 Windows TSF 注册。`linux.engine` 为 Linux 打包预留。

`macos.inputMethod` 是已弃用的兼容别名。迁移时把整个配置段移到顶层，两种写法只能使用一种。

把生成的 `.app` 复制到 `~/Library/Input Methods/`。首次安装或新增输入模式后，请注销并重新登录，让 macOS 重新加载系统输入菜单。在「系统设置 → 键盘 → 文字输入 → 编辑」中添加要用的输入法和模式，再从输入菜单选择。后台通过 TIS 启用输入源时，其他进程可能仍读不到启用状态。

更新已运行的输入法时，用 `killall <可执行文件名>` 结束旧进程，macOS 会在下次使用该输入法时重新启动它。

## Windows 输入法

顶层 `inputMethod` 会让 `tan package -platform windows/amd64` 生成机器级 NSIS 输入法安装包。安装需要 UAC，默认装到 Program Files。登录时系统启动输入法宿主，TSF 桥接 DLL 与当前登录会话中的宿主通信。应用仍通过 `ime.Serve` 启动宿主。安装包为目标架构放入对应的 TSF DLL。

`inputMethod.windows.clsid` 和 `profile` 省略时由应用 ID 稳定生成。升级时请保留同一应用 ID、CLSID 和语言配置 GUID。`langid` 默认为 `0804`（简体中文，中国）。Windows 语言栏接受模式的 ICO 或 PNG 图标，其他格式采用模式短文字绘制图标。

`tan` 根据应用所用的 Tango 模块版本下载桥接 DLL，并校验发布构建内置的 SHA-256。使用本地 Tango 时，设置 `TANGO_IME_BRIDGE_DIR` 指向包含 `TangoIMEBridge-x64.dll` 或 `TangoIMEBridge-arm64.dll` 的目录。

桥接 DLL、宿主和安装器默认使用未签名的构建。启用智能应用控制的 Windows 11 可能拒绝加载未签名产物。需要签名时配置 `windows.certificate`，`tan package` 会沿用 Windows 签名流程同时签名。ARM64 桥接提供原生 DLL。Windows 32 位客户端不在支持范围内。

## macOS 磁盘映像

| 字段 | 默认 | 说明 |
|---|---|---|
| `background` | 无（白底） | PNG、JPEG 或 TIFF。按图片的**点**尺寸画在窗口内容区左上角：72 dpi 的图 1 像素 = 1 点。如果 `x.png` 旁边有 `x@2x.png`，tan 会用 `tiffutil -cathidpicheck` 把两张合成多分辨率的 `background.tiff`，用于 Retina。 |
| `windowSize` | `[540, 380]` | 窗口内容区尺寸（点），标题栏由 Finder 另外加上。背景图建议与它同尺寸：540×380，Retina 版 1080×760。 |
| `iconSize` | `128` | 16–512 |
| `appPosition` | `[140, 190]` | app 图标中心，从内容区左上角起算 |
| `applicationsPosition` | `[400, 190]` | Applications 链接图标的中心 |
| `title` | 应用名 | 卷名，即 Finder 窗口标题。单行，最多 255 字节，不能含 `:` 或 `/`。 |

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
- Windows 安装包不内置 WebView2 Runtime，WebView 应用的运行环境需要提供它。
