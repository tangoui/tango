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

从仓库根目录运行以下命令，需要 Go、CMake 和 Visual Studio 的 C++ 工具链。DLL 和宿主 EXE 需要安装在同一目录。

```powershell
cmake -S examples/inputmethod/windows -B examples/inputmethod/build/tsf -A x64
cmake --build examples/inputmethod/build/tsf --config Release
go build -ldflags=-H=windowsgui -o examples/inputmethod/build/tsf/Release/TangoPinyin.exe ./examples/inputmethod
```

用管理员 PowerShell 把生成的文件安装到 Program Files，再注册 TSF 服务。

```powershell
$dest = Join-Path $env:ProgramFiles 'Tango Pinyin'
New-Item -ItemType Directory -Force $dest | Out-Null
Copy-Item examples/inputmethod/build/tsf/Release/TangoPinyin*.dll $dest
Copy-Item examples/inputmethod/build/tsf/Release/TangoPinyin.exe $dest
regsvr32.exe (Join-Path $dest 'TangoPinyinTIP.dll')
```

在 Windows 的「语言和区域」设置中为「中文（简体，中国）」添加 Tango Pinyin，用 Win+空格切换。TSF 加载 DLL 后会按需启动同目录的宿主。DLL 的架构需要与使用输入法的应用相同。上述命令构建 x64 版。在 ARM64 Windows 上，可以用 `-A ARM64` 构建原生 DLL，并用 ARM64 Go 构建宿主。

卸载前先切换到其他输入源，在管理员终端对同一 DLL 运行 `regsvr32.exe /u`。关闭宿主和使用过该输入法的程序后，可以删除安装目录。

TSF DLL 保留在原生应用进程中，Go 引擎通过当前用户和登录会话的命名管道运行在独立进程。框架使用 `ime.Handler.Connection` 匹配 DLL 的 `TANGO_IME_CONNECTION` 编译常量。这个示例使用自己的 TSF 类和语言配置 GUID。
