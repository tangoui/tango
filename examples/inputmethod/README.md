# Tango Pinyin

这个示例共用一套 Go 拼音引擎。输入 `nihao` 后按空格提交「你好」，输入 `shijie` 后按 Esc 可以取消，输入 `wo` 后按 Enter 提交原拼音。

## macOS

在示例目录运行 `tan build`。按打包文档将生成的 `Tango Pinyin.app` 安装为输入源。[安装说明](../../docs/packaging.md#macos-输入法)

单击左 Shift 或右 Shift 切换中英文，菜单栏随之显示「中」或「英」。切到英文时，正在组字的拼音会原样上屏。也可以从系统输入源菜单选择模式，或使用输入法菜单里的「切换中/英」。点击「关于」会写入一条日志。

## Linux

在示例目录运行 `tan package -format deb` 或 `tan package -format rpm`。在匹配目标架构的 Linux 上打包时，安装 CMake 和本发行版的 Fcitx5 开发包即可一并构建 Fcitx5 插件。从 macOS 交叉打包得到的是 IBus 引擎包。

用发行版的包管理器安装生成的文件，然后重启 IBus 或 Fcitx5。在 IBus 首选项或 Fcitx5 配置工具中搜索并添加 Tango Pinyin。单击 Shift 切换中英文，系统面板显示候选、模式和菜单。键入 `nihao` 后按空格提交「你好」，按数字键可选候选。

开发时可在 IBus 桌面会话中运行 `go run ./examples/inputmethod` 临时注册引擎，然后在同一会话使用 `ibus engine com.tangoui.inputmethod.example` 切换。可用 `ibus engine` 记录并恢复原输入源。系统需提供 GTK 3 和 libibus。

在隔离的 IBus 会话中可运行 `python3 examples/inputmethod/linux/smoke.py` 验证按键、组字、提交和失焦，测试依赖系统 Python 的 PyGObject 与 IBus introspection 数据。

## Windows

在示例目录运行 `tan package -platform windows/amd64`，用生成的 `setup.exe` 安装。使用本地 Tango 工作树时，先按桥接构建说明生成 DLL，再将 `TANGO_IME_BRIDGE_DIR` 指向 DLL 目录。[打包说明](../../docs/packaging.md#windows-输入法) · [桥接构建](../../internal/imebridge/windows/README.md)

在「设置 → 时间和语言 → 语言和区域」为简体中文添加 Tango Pinyin，再用 Win+空格切换。登录时系统会启动宿主。单击 Shift 切换中英文，点击光标旁的候选词可直接上屏。语言栏菜单提供「切换中/英」和「关于」。日志位于 `%LOCALAPPDATA%/Tango Pinyin/ime.log`。

在「设置 → 应用」卸载。部分应用仍持有旧 TSF DLL 时，安装器会安排 Windows 在下次重启时删除该文件。
