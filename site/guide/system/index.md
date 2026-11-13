# 系统能力总览

窗口之外的系统能力每个一个包，路径是 `github.com/tangoui/tango/sys/<名字>`（自动更新在 `github.com/tangoui/tango/service/updater`）。约定统一：

- 函数第一个参数是 `*tango.App`（窗口相关的做成 `*tango.Surface` 的方法）；
- 任意 goroutine 可调用，回调在主线程；
- 当前平台做不到时返回该包的 `ErrUnsupported`（即 `platform.ErrUnsupported`），应用退出后返回 `tango.ErrNotRunning`；
- 需要用户操作的函数（对话框、分享、权限请求）会阻塞调用方，在主线程上调用时继续泵事件。Android 上请从 goroutine 调用，见 [线程模型](/guide/concepts/threading#移动端的差异)。

页面里的 JS 通过 `jsapi` 使用其中一部分，开关见 [安全模型](/guide/concepts/security#_2-jsapi-能力白名单)，函数见 [前端运行时 API](/reference/js-api)。

::: warning 验证状态
桌面三平台的后端都有实现，但只有 **macOS 实机验证过**；Windows、Linux 尚未实机验证。移动端能力在 iOS 模拟器 / Android 模拟器上验证过一部分，**没有上真机**，属于实验性。
:::

## 桌面

| 包 | 作用 | 页面（jsapi） |
|---|---|---|
| [clipboard](./clipboard) | 文本、PNG 图片、文件列表 | `Clipboard`（ClipboardRead / ClipboardWrite） |
| [dialog](./dialog) | 消息框、打开、保存对话框 | `Dialog`（Dialogs） |
| [notify](./notify) | 系统通知与点击 | `Notify`（Notify） |
| [hotkey](./hotkey) | 全局快捷键、窗口内快捷键 | `Hotkey`（Hotkeys） |
| [screen](./screen) | 显示器列表、主屏、坐标所在屏 | `Screens`（Info） |
| [theme](./theme) | 深浅色、强调色、同步到页面 | `Theme`（Info） |
| [shell](./shell) | 打开 URL / 文件、在文件管理器中显示 | `Shell`（OpenURL / OpenPath） |
| [appctl](./appctl) | 隐藏/显示应用、徽标、进度、请求注意、Dock 图标 | `App`（AppControl） |
| [autostart](./autostart) | 登录时启动 | — |
| [env](./env) | 系统、架构、构建类型、WebView 版本 | `App.info`（Info） |
| [recent](./recent) | 最近打开的文档 | — |
| [single](./single) | 单实例与参数转交 | `App.onSecondInstance`（Info） |
| [windowstate](./windowstate) | 记住窗口位置和大小 | — |
| [power](./power) | 电源状态、休眠/唤醒、阻止休眠（macOS、Windows、Linux；移动端暂不支持） | — |
| [menu](./menu) | 应用菜单、窗口菜单、右键菜单 | `Menu`（Menu，仅编辑类 role） |
| [tray](./tray) | 托盘图标 | — |

## 移动端

| 包 | 作用 | tan.json `permissions` |
|---|---|---|
| [permission](./permission) | 查询和请求运行时权限 | 按所请求的权限 |
| [share](./share) | 系统分享面板（macOS 也支持） | — |
| [haptics](./haptics) | 触感反馈 | — |
| [safearea](./safearea) | 安全区内边距、同步为 CSS 变量 | — |
| [orientation](./orientation) | 读取和锁定屏幕方向 | — |
| [biometric](./biometric) | Face ID / Touch ID / BiometricPrompt（macOS 也支持） | `biometric` |
| [securestore](./securestore) | Keychain / Keystore 存小秘密（macOS 也支持） | — |
| [media](./media) | 选照片/视频、拍照 | `camera`（拍照） |
| [location](./location) | 单次和持续定位 | `location` |
| [push](./push) | APNs / FCM 远程推送 | `notifications` |

权限声明的生成规则见 [移动端](/guide/mobile#权限)。

## 服务（service/）

不依赖平台界面的辅助包：

| 包 | 作用 |
|---|---|
| `service` | 把长期存在的 Go 值挂到应用生命周期（Startup / Shutdown），可同时绑定为 IPC 服务 |
| `service/paths` | 各平台的配置、数据、缓存、日志目录 |
| `service/kv` | 线程安全的小型键值存储，可持久化为 JSON |
| `service/log` | 基于 `log/slog` 的日志：stderr + 按大小滚动的文件，页面可通过 IPC 写日志 |
| `service/fileserver` | 把本地目录安全地挂到资源服务下 |
| `service/updater` | 签名的自动更新，见 [打包、签名与自动更新](/guide/packaging#自动更新) |

`sys/` 下还有几个正在开发的包，接口可能变化，以 [API 参考](/reference/api/) 为准。
