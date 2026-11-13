# 命令行 tan

`tan` 负责创建、运行、构建和发布 Tango 应用。除 `init`、`keygen`、`doctor`、`version` 外，每个命令都读取当前目录（或 `-C <dir>` 指定目录）下的 [tan.json](/reference/tan-json)。

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan help              # 命令列表
tan <command> -h      # 某个命令的参数
```

## 命令一览

| 命令 | 作用 |
|---|---|
| `tan init [dir]` | 从模板创建项目；终端里不带目录时逐项询问，`-yes` 用默认值 |
| `tan dev` | 开发模式：启动前端开发服务器（如有），编译运行应用，Go 改动后重建；自绘窗口热加载 |
| `tan build` | 发布构建，一个或多个平台：桌面、iOS、Android |
| `tan run` | 构建、安装并在 iOS 模拟器或 Android 设备 / 模拟器上运行，日志接到终端 |
| `tan package` | 构建后生成安装包：`.dmg`、NSIS 安装器、MSIX、`.tar.gz` 与 AppImage |
| `tan sign` | macOS 签名与公证；Windows exe / msi / msix 的 Authenticode 签名 |
| `tan keygen` | 生成更新签名用的 Ed25519 密钥对 |
| `tan release-manifest` | 为一次发布生成更新文件、二进制补丁和签名的 `latest.json` |
| `tan sign-update` | 用 Ed25519 私钥签名更新清单 |
| `tan generate bindings` | 为 `bind.Register` 注册的服务生成 TypeScript 客户端 |
| `tan doctor` | 检查工具链和平台依赖 |
| `tan version` | 打印版本 |

## 平台目标

`tan build`、`tan package` 的 `-platform` 接受逗号分隔的目标：

| 写法 | 含义 |
|---|---|
| （空） | 本机平台 |
| `darwin/arm64`、`windows/amd64`、`linux/arm64` | 桌面 OS/架构；只写 OS 用本机架构 |
| `darwin/universal` | 两种 Mac 架构合成通用二进制（需要 `lipo`） |
| `ios-simulator` / `ios` | iOS 模拟器 `.app` / 真机 `.app` + `.ipa` |
| `android`、`android/arm64,android/amd64` | Android；只写 `android` 等于 arm64 + amd64 |

桌面构建产物在 `build/<os>-<arch>/`，`-o` 修改输出根目录；`-debug` 保留符号、允许打开 Web 检查器（Android 上产出 debug APK）。`tan build`（含 `-debug`）都是生产构建：`GOFLAGS` 里已有的 `-tags` 会合并进去，但去掉 `tango_dev`（另外仍传一个已无作用的兼容标签 `tango_release`）。`tan dev` 的构建（桌面与移动端）加 `-tags tango_dev`，开启热加载、自动化、前端开发代理等开发功能。

## 常见流程

```bash
tan init -template react-ts notes && cd notes
tan dev
tan generate bindings                       # Go 服务改了之后重新生成 TS 客户端
tan build -platform darwin/universal
tan package -platform darwin/universal
tan sign -notarize build/darwin-universal/Notes.app
tan run -platform ios-simulator
tan build -platform android/arm64,android/amd64
```

## 环境变量

| 变量 | 用于 |
|---|---|
| `TANGO_DEV_URL` | `tan dev` 设置给应用的前端开发服务器地址（只有 `-tags tango_dev` 构建的 `asset.FromEnv` 读取，且只接受回环地址） |
| `TANGO_HOTRELOAD=0` | 关闭自绘窗口热加载 |
| `TANGO_SIGN_IDENTITY`、`TANGO_NOTARY_PROFILE`、`TANGO_APPLE_ID`、`TANGO_APPLE_TEAM_ID`、`TANGO_APPLE_PASSWORD` | macOS 签名与公证 |
| `TANGO_WINDOWS_CERT`、`TANGO_WINDOWS_CERT_PASSWORD` | Windows 签名证书与密码 |
| `TANGO_UPDATE_KEY_FILE`、`TANGO_UPDATE_KEY` | `tan release-manifest`、`tan sign-update` 的私钥 |
| `TANGO_IOS_SIGN_IDENTITY` | iOS 真机签名身份 |
| `TANGO_ANDROID_KEYSTORE_PASSWORD`、`TANGO_ANDROID_KEY_PASSWORD` | Android release 签名 |
| `JAVA_HOME`、`ANDROID_HOME`、`ANDROID_NDK_HOME` | Android 构建 |

## 完整参数

下面的内容在构建文档时由 `tan <command> -h` 自动生成。

<!--@include: ./_cli-flags.md-->
