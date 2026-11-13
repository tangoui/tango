# autostart：用户登录时自动启动应用

`autostart` 把应用注册为登录项，用户下次登录系统时自动运行。它只读写当前用户的配置文件或注册表，不需要应用已经在运行，也不需要主线程，可以在 `App.Run` 之前或之外调用（例如设置页里的开关、安装脚本）。

登录项启动应用时会在命令行末尾加上 `LoginArg`（`--tango-login`），应用用 `OpenedAtLogin` 判断是否是开机自启，从而决定是否直接隐藏到托盘。

```go
import "github.com/tangoui/tango/sys/autostart"
```

## 用法

```go
o := autostart.Options{ID: "com.example.notes", Name: "Notes"}

if err := autostart.Enable(o); err != nil {
	log.Println("enable autostart:", err)
}
on, err := autostart.IsEnabled(o)
// ...
_ = autostart.Disable(o) // 不存在也不算错
```

开机自启时不弹主窗口：

```go
win, _ := app.NewSurface(tango.SurfaceOptions{Hidden: autostart.OpenedAtLogin()})
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Options` | `ID`（必填，反向域名，不能含路径分隔符）、`Name`、`Exec`（默认 `os.Executable()`）、`Args`、`Dir`（覆盖存放位置，用于测试或便携安装） |
| `Enable(o)` | 注册登录项，同 ID 的旧项被替换 |
| `Disable(o)` | 删除登录项 |
| `IsEnabled(o)` | 登录项存在且没有被用户关掉 |
| `OpenedAtLogin()` | 本进程是否由登录项启动（命令行含 `LoginArg`） |
| `LoginArg` | 常量 `"--tango-login"` |

完整签名见 [API 参考](/reference/api/sys-autostart)。

## 平台支持

| 平台 | 实现 |
|---|---|
| macOS | ✓ `~/Library/LaunchAgents/<ID>.plist`（launch agent），下次登录生效；系统设置「登录项」里会列出 |
| Windows | ✓ `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 下名为 ID 的值 |
| Linux | ✓ XDG 自启动项 `~/.config/autostart/<ID>.desktop`（遵循 `$XDG_CONFIG_HOME`） |
| iOS | — |
| Android | — |

> Windows、Linux 实现尚未实机验证。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Settings struct{ o autostart.Options }

func (s Settings) SetAutostart(on bool) error {
	if on {
		return autostart.Enable(s.o)
	}
	return autostart.Disable(s.o)
}

bind.Register(r, "Settings", Settings{o})
```

页面里 `await tango.call("Settings.SetAutostart", true)`。

## 注意

- 不支持的平台返回 `autostart.ErrUnsupported`。
- Windows 任务管理器的「启动」页可以禁用登录项而不删除它，此时 `IsEnabled` 返回 false；Linux 桌面把它关掉（`Hidden=true` 或 `X-GNOME-Autostart-enabled=false`）同样读作未启用。
- macOS 的 launch agent 在下次登录时才生效，`Enable` 不会立即启动应用。
- `Exec` 默认是当前可执行文件路径：从临时目录运行（如 `go run`）时注册的是临时路径，正式使用应在打包后的应用里调用。
