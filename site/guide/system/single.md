# single：单实例运行

`single` 保证同一用户只运行一个应用实例。第一个启动的进程成为主实例并监听后续启动；之后再启动的进程把自己的命令行参数和工作目录转给主实例，然后应当退出。主实例收到后会处理其中的深链和文件、调用回调，并把第一个打开的窗口显示到前台。

```go
import "github.com/tangoui/tango/sys/single"
```

## 用法

在 `main` 里、`App.Run` 之前调用 `Claim`：

```go
app := tango.New(tango.Options{ID: "com.example.notes", URLSchemes: []string{"notes"}})

primary, err := single.Claim(app, single.Options{
	OnSecondInstance: func(args []string, cwd string) {
		log.Println("relaunched with", args, "in", cwd)
	},
})
if !primary {
	return // 已转交给正在运行的实例
}
if err != nil {
	log.Println("single instance disabled:", err) // 仍可正常运行
}
```

其他包不拿 `Options` 也能监听：

```go
single.OnSecondInstance(app, func(args []string, cwd string) { /* ... */ })
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Claim(a, Options) (primary bool, err error)` | 成为主实例，或把参数转给主实例 |
| `OnSecondInstance(a, fn)` | 追加一个回调，在 `Options.OnSecondInstance` 之后于主线程运行；`Claim` 前后、任意 goroutine 均可调用 |
| `Options.Key` | 区分应用的键，默认用 `tango.Options.ID`（否则 `Name`） |
| `Options.OnSecondInstance` | 后续启动转来时在主线程调用（参数不含程序名） |
| `Options.NoFocus` | 不把主实例窗口带到前台 |
| `Options.Args` | 要转发的参数，默认 `os.Args[1:]` |
| `Options.Timeout` | 后续实例连接主实例的超时，默认 3 秒 |

完整签名见 [API 参考](/reference/api/sys-single)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| ✓ flock 锁文件 + Unix 域套接字 | ✓ 命名管道（`FILE_FLAG_FIRST_PIPE_INSTANCE`） | ✓ 同 macOS，位于 `$XDG_RUNTIME_DIR` | 系统本身只运行一个实例，`Claim` 什么也不做并返回 primary | 同 iOS |

> Windows、Linux 实现尚未实机验证。

## 在页面里调用

`jsapi.Install` 会自动注册 `OnSecondInstance`，页面用 `@tangoui/runtime` 的 `App.onSecondInstance` 订阅（属于默认开启的 `Info`）。前提是 Go 侧调用了 `single.Claim`：

```ts
import { App } from "@tangoui/runtime";

const off = App.onSecondInstance(({ args, cwd }) => console.log(args, cwd));
```

转发参数里的深链和文件同时也会以 `App.onOpenURL` / `App.onOpenFiles` 送达。见 [JS API](/reference/js-api)。

## 注意

- **返回值**：`primary=false` 只出现在参数已转发（此时应退出），或主实例存在但连不上（err 说明原因，退出仍是稳妥做法）。`primary=true` 且 err 非 nil 表示单实例模式没能建立，应用照常运行。
- 只调用一次，并且在 `App.Run` 之前；应用结束时主实例停止监听。
- 主实例的选举是原子的，两个同时启动的进程不会都成为主实例；主实例崩溃后内核会释放锁，残留的套接字文件会被替换，不会阻塞下次启动。
- 转发的启动在主实例中经 `App.HandleArgs` 处理，产生 `EventOpenURL` / `EventOpenFiles`，见 [生命周期](/guide/concepts/lifecycle)。
