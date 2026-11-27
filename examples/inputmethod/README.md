# Tango Pinyin

这个示例共用一套 Go 拼音引擎。输入 `nihao` 后按空格提交「你好」，输入 `shijie` 后按 Esc 可以取消，输入 `wo` 后按 Enter 提交原拼音。

## macOS

在示例目录运行 `tan build`。按打包文档将生成的 `Tango Pinyin.app` 安装为输入源。[安装说明](../../docs/packaging.md#macos-输入法)

单击左 Shift 或右 Shift 切换中英文，菜单栏随之显示「中」或「英」。切到英文时，正在组字的拼音会原样上屏。也可以从系统输入源菜单选择模式，或使用输入法菜单里的「切换中/英」。点击「关于」会写入一条日志。

## Linux

在使用 IBus 的桌面会话中，从仓库根目录运行以下命令。宿主进程启动后会向当前会话注册 Tango Pinyin，进程退出后注册随连接消失。GTK 3 和 libibus 由发行版提供。

```sh
go build -o /tmp/tango-pinyin ./examples/inputmethod
/tmp/tango-pinyin
```

保持宿主运行，在另一个终端用 `ibus engine com.tangoui.inputmethod.example` 切换输入法，然后在使用 IBus 的应用里输入。可用 `ibus engine` 记录当前输入源，以便测试完成后切回。

在单独的 IBus 测试会话中，可以使用 `python3 examples/inputmethod/linux/smoke.py` 检查按键、组字、提交和失焦行为。该测试需要系统 Python 的 PyGObject 与 IBus introspection 数据。

## Windows

在示例目录运行 `tan package -platform windows/amd64`，用生成的 `setup.exe` 安装。使用本地 Tango 工作树时，先按桥接构建说明生成 DLL，再将 `TANGO_IME_BRIDGE_DIR` 指向 DLL 目录。[打包说明](../../docs/packaging.md#windows-输入法) · [桥接构建](../../internal/imebridge/windows/README.md)

在「设置 → 时间和语言 → 语言和区域」为简体中文添加 Tango Pinyin，再用 Win+空格切换。登录时系统会启动宿主。单击 Shift 切换中英文，点击光标旁的候选词可直接上屏。语言栏菜单提供「切换中/英」和「关于」。日志位于 `%LOCALAPPDATA%/Tango Pinyin/ime.log`。

在「设置 → 应用」卸载。部分应用仍持有旧 TSF DLL 时，安装器会安排 Windows 在下次重启时删除该文件。
