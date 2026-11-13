# 自动更新（service/updater）

`service/updater` 让 Tango 桌面应用自己更新：拉取**签名清单**，拒绝比当前版本旧（或比见过的旧）的清单，优先下载**二进制补丁**、失败回退完整包，校验后等应用退出再安装，新版本启动后**确认**，确认不了就**回滚**。发布侧由 `tan keygen`、`tan release-manifest`、`tan sign-update` 完成。

## 快速上手

```sh
tan keygen -o update                  # update.key（私钥，别进仓库）+ update.pub
tan package -platform darwin/arm64,windows/amd64,linux/amd64
tan release-manifest -key update.key -prev ../release-1.3.0 -o release -url https://dl.example.com/notes/
# 把 release/ 下的文件（latest.json、完整包、*.patch）上传到 https://dl.example.com/notes/
```

```go
u := &updater.Updater{
	ManifestURL: "https://dl.example.com/notes/latest.json",
	PublicKey:   "…",            // update.pub
	Version:     "1.3.0",        // 当前版本
	AppID:       "com.example.notes", // 必设：清单必须写同一个 app
	RequireExpiry: true,             // 所有已发布清单都带 expires 后开启（见“有效期与迁移”）
	OnEvent:     func(e updater.Event) { /* 自己的界面：进度、准备重启… */ },
}
out, _ := u.Confirm()            // 每次启动尽早调用（见“确认与回滚”）
rel, err := u.Check(ctx)         // nil, nil：已是最新
p, err := u.Download(ctx, rel, "") // 先补丁，失败回退完整包
err = u.Apply(p)                 // 然后退出应用；辅助进程替换并启动新版本
```

`u.Update(ctx)` 把 Check + Download + Apply 串成一步。

## 清单格式

`latest.json` 是一个信封：`manifest` 是清单 JSON 原样字节，`signature` 是对它的 Ed25519 签名。

```json
{
  "manifest": {
    "app": "com.example.notes",
    "version": "1.4.0",
    "pub_date": "2026-11-01T12:00:00Z",
    "expires": "2026-12-01T12:00:00Z",
    "notes": "Faster startup.",
    "platforms": {
      "darwin-arm64": {
        "format": "app",
        "url": "Notes-1.4.0-darwin-arm64.app.tar.gz", "size": 9123456, "sha256": "…",
        "tree_sha256": "…",
        "patches": [{"from": "1.3.0", "url": "Notes-1.3.0-to-1.4.0-darwin-arm64.patch", "size": 81234, "sha256": "…"}]
      },
      "windows-amd64": {"format": "exe", "url": "Notes-1.4.0-windows-amd64.exe", "size": 8000000, "sha256": "…", "patches": […]},
      "linux-amd64":   {"format": "appimage", "url": "Notes-1.4.0-linux-amd64.AppImage", "size": 9000000, "sha256": "…"}
    }
  },
  "signature": "…"
}
```

| 字段 | 说明 |
|---|---|
| `app` | 应用 id；客户端设了 `AppID` 时必须**存在且一致**（防止同一把钥匙签的别的应用清单被拿来用；没写 `app` 的清单会被设了 `AppID` 的客户端拒绝） |
| `version` / `pub_date` | 语义化版本、发布时间（必填） |
| `expires` | 清单失效时间（RFC 3339，可选但强烈建议；须晚于 `pub_date`）。与其它字段一样被签名覆盖。过期的清单被 `Check` 拒绝（`ErrExpired`），见“有效期与迁移” |
| `platforms` | 键为 `GOOS-GOARCH` |
| `format` | `app`（含一个 .app 的 tar.gz/zip）、`dir`（含一个目录的 tar.gz，Linux 安装目录）、`appimage`、`exe`（Windows 可执行文件本身）、`nsis`（NSIS 安装器）、`msi` |
| `url` / `size` / `sha256` | 完整包；相对 URL 以清单 URL 为基准，只允许 http(s) |
| `tree_sha256` | `app`/`dir` 解包后目录的树摘要（`TreeDigest`：按路径排序的目录、符号链接目标、文件可执行位与内容摘要；不含时间/属主） |
| `patches` | 从旧版本 `from` 到本版本的补丁；`nsis`/`msi` 不能有补丁（客户端手里没有旧安装器） |

清单里没有逐文件签名：每个文件的 sha256 已被清单签名覆盖。

## 安全模型

- **先验签名再信任内容**：`Check` 拿到信封后先用 `PublicKey`（及轮换用的 `PublicKeys`）验证签名，通过后才解析清单（拒绝未知字段）。签名覆盖清单原样字节，任何改动（版本、URL、摘要）都会失败；不需要规范化 JSON。上下文前缀 `tango-update-manifest-v1\0` 防止签名被挪作他用。
- **必须用 https，必须设 `AppID`**：`ManifestURL` 和文件 URL 请一律用 https（`http` 只为本地测试保留；签名防篡改，但不防看见、拦截和重放）。`AppID` 不设时任何同钥匙签的清单都会被接受，多个应用共用一把钥匙时这等于允许中间人把 A 应用的清单交给 B 应用。
- **防回滚 / 防重放**：清单版本比当前 `Version` 旧 → `ErrDowngrade`；相同 → 视为最新。可设 `MinVersion`：低于它的清单一律拒绝（例如已知有问题的版本）。设了 `StateDir` 或 `AppID` 时，客户端记住接受过的最新清单（`seen.json`：版本 + pub_date），之后比它旧的清单（攻击者重放一份旧的、但仍比当前新的合法清单）也被拒绝。
- **有效期（防冻结）**：清单的 `expires` 过了（允许 `ClockSkew`，默认 `DefaultClockSkew` = 10 分钟的时钟偏差；负数表示不容忍）就被拒绝，`ErrExpired`。这限制了中间人反复重放“与当前版本相同”的旧清单、让安全修复送不到的时间；没设 `StateDir`/`AppID`（不记 `seen.json`）时，它也是挡住“比当前新但不是最新”的旧清单重放的唯一手段。
- **文件校验**：下载按清单 `size` 限长读取，sha256 不符即删除；补丁应用后再算一遍最终文件的 sha256（或树摘要）与清单比对。`Apply` 把要安装的文件复制进私有（0700）工作目录后**对副本重新校验** sha256 / 树摘要再使用，所以 `Download` 的目录即使别人可写（共享下载目录），校验与使用之间也不能被换掉。
- **解包安全**：tar/zip 和树补丁都经 `os.Root` 写入，拒绝 `..`、绝对路径、指向外部的符号链接。符号链接按**已写入的实际路径逐级解析**（经过的链接都跟进，最多 40 跳），所以 `a → .` 再 `a/b → ..` 这类层层叠加的链接会被识破；全部写完后再对整棵树的每个链接复查一遍（后写的链接可能改变先写链接的解析结果）。
- **二进制补丁**：`Patch`/`PatchMax` 对所有位置、长度做溢出与边界检查，畸形补丁返回错误而不是 panic；差值流 + 额外流的长度必须正好等于补丁头的新文件大小，否则在分配输出前就拒绝。`Patch` 允许最大 2 GiB 输出；处理不可信补丁请用 `PatchMax(old, patch, 期望大小)`。updater 自己用清单里的 `size`（树补丁用索引里的文件大小）作上限。
- **macOS 代码签名**：运行中的 app 有签名时，新 bundle 必须 `codesign --verify --deep --strict` 通过；运行中的 app 有 Team ID 时，新 bundle 的 Team ID 必须相同。
- 不防：私钥泄露（请离线保管、用环境变量 `TANGO_UPDATE_KEY(_FILE)` 注入 CI）；有效期之内的冻结（中间人在 `expires` 之前一直给同一份旧但合法的清单）——有效期越短窗口越小，但要更勤地重新签名。

### 有效期与迁移（`expires`）

- `tan release-manifest` 默认写 `expires = pub_date + 30 天`（`-expires 720h` 可改，`-expires 0` 不写）。`tan sign-update` 默认把 `expires` 设为**当前时间 + 30 天**（`-expires 0` 保留清单里原有的值）。
- **清单过期前要重新签名**，哪怕没有新版本：`tan sign-update -key update.key latest.json` 就地刷新 `expires` 并重签（版本、`pub_date` 不变，客户端照常接受），再上传。建议在 CI 里每周跑一次。
- 客户端时钟快于真实时间超过 `ClockSkew` 时会把有效清单当成过期；时钟严重错误的机器会收不到更新，`Check` 的错误里带有清单的过期时间与本机时间，便于诊断。
- **`RequireExpiry`（默认 false）**：开启后没有 `expires` 的清单一律拒绝（`ErrExpired`）。默认不开，是因为开了以后在此之前用旧版 tan 签的、没有 `expires` 的清单会全部失效（包括手写后 `sign-update` 签的），直接用 `updater.SignManifest` 的发布脚本若没填 `Expires` 也会立刻收不到更新。不开时仍有一层保护：客户端（设了 `StateDir` 或 `AppID`）**一旦接受过带 `expires` 的清单，之后就拒绝不带 `expires` 的清单**（记在 `seen.json` 的 `expiring`），所以发布侧改用带有效期的清单后，攻击者也无法拿以前没有有效期的旧清单来绕过。**强烈建议**：所有在线清单都重新签过（带 `expires`）之后，在下一个版本里设 `RequireExpiry: true`。
- **旧客户端的兼容问题**：本改动之前的 updater 解析清单时拒绝未知字段，**看不懂 `expires`**，会对带 `expires` 的清单报 `unknown field "expires"`。如果已经有用旧 updater 的版本在用户手里，过渡期请给旧版本一个单独的清单地址（用 `-expires 0` 生成、不含 `expires`），或先发一个用新 updater 的版本、再开始在主清单里写 `expires`。

## 补丁

### 二进制补丁（exe、appimage）

`updater.Diff` / `updater.Patch` 是按 Colin Percival《Naive differences of executable code》描述自写的 bsdiff 风格算法：

1. 对旧文件建后缀数组（自写 SA-IS，线性时间）。
2. 扫描新文件，用后缀数组找最长匹配；匹配只是延续当前对齐时跳过，比当前对齐多 8 字节以上才切段。
3. 每段向前、向后做“近似扩展”（匹配字节过半即接受），重叠处取最优分割。
4. 输出三个流：控制三元组（add, copy, seek，zigzag varint）、差值字节（新−旧，大多为 0）、额外字节；三个流各自 DEFLATE 压缩。

格式：`"TGOPATCH"`、旧/新大小、旧/新 sha256、三个压缩流长度、三个流。应用前检查旧文件 sha256（不符返回 `ErrPatchBase`），应用后检查新文件 sha256。

### 树补丁（app、dir）

`DiffTree` / `PatchTree`：gzip 的 tar，首个条目 `index.json` 列出新树的每一项；文件三种来源：`copy`（旧树里内容相同的文件，可跨路径，处理改名）、`patch`（同路径旧文件 + 二进制补丁）、`data`（整份存放）。应用时逐个校验来源文件摘要，最后整树 `TreeDigest` 必须等于清单 `tree_sha256`。

### 客户端策略

`Download` 发现清单里有 `from == 当前版本` 的补丁时：下载补丁 → 应用到本机安装（`Target`）→ 校验；任何一步失败（本地文件被改过、补丁损坏、摘要不符）发出 `patch-failed` 事件，改为下载完整包。

### 压缩比（实测，darwin/arm64）

| 样本 | 完整包 | 补丁 | 比例 |
|---|---|---|---|
| 1 MiB 随机二进制，200 处改字节 + 插入 20 KB + 删除 10 KB | 1,058,576 B | 21,932 B | 2.1% |
| 小 Go 程序，v2 新增 encoding/json、sort 依赖（5.5 MB → 6.2 MB，新增 0.7 MB 代码） | 6,184,706 B | 752,434 B | 12.2% |
| 演练 app（链接 tango 的 Go 程序，7.5 MB 可执行文件），0.0.1 → 0.0.2 有代码改动 | 3.0 MB（.app.tar.gz） | 252 KB | 8.1% |
| 演练 app 0.0.2 → 0.0.3（只改链接期常量） | 3.0 MB | 17 KB | 0.6% |

`tan release-manifest` 在补丁达到完整包 80% 以上时不发布它。

## 各平台的安装方式

`Apply` 先把新版本放到目标旁边的工作目录 `.tango-update-*`（同一卷，重命名是原子的），写下 `pending.json`，启动辅助进程，然后应用应尽快退出。辅助进程等应用退出后：把旧版本移进工作目录（`previous`）、新版本移到原位、启动新版本、等待确认。

| 平台 / 格式 | 目标 | 替换 | 启动 |
|---|---|---|---|
| macOS `app` | 运行中的 `X.app` | 去掉 `com.apple.quarantine`（避免“从互联网下载”提示）、校验签名与 Team ID 后整 bundle 重命名替换；文件只移动不改写，签名保持有效 | `/usr/bin/open X.app` |
| Linux `appimage` | `$APPIMAGE` | 单文件重命名替换 | 直接执行 |
| Linux `dir` | 可执行文件所在目录（如 `/opt/notes`） | 整个目录重命名替换；包内应是一个顶层目录（`tan package` 的 tar.gz 即是）。见下方“`dir` 的保护” | 新目录里的同名可执行文件 |
| Windows `exe` | 运行中的 `X.exe` | 新 exe 写到同目录工作目录；旧 exe 重命名到 `previous`，新 exe 重命名到原位 | 直接执行；确认后删除旧 exe |
| Windows `nsis` | 运行中的 `X.exe` | 旧 exe 先挪开，运行 `setup.exe /S /D=<exe 所在目录>`（静默安装到原目录），失败则放回 | 直接执行 |
| Windows `msi` | — | `msiexec /i x.msi /qn /norestart`（MSI 自身事务，不做回滚） | 直接执行 |

辅助进程：macOS/Linux 是 `/bin/sh` 脚本（`setsid`，不随应用进程组退出）；Windows 上不依赖任何 shell，`Apply` 把应用自己的 exe 复制到临时目录、带环境变量 `TANGO_UPDATE_HELPER` 启动，updater 包的 `init` 在应用 `main` 之前执行任务后退出（应用其它包的 init 仍会执行，不应在 init 里开窗口）。辅助程序副本由下次 `Confirm` 清理。

**`dir` 的保护**：`dir` 格式把可执行文件所在的**整个目录**当作安装目录换掉，所以只适合“应用独占一个目录”的安装（`/opt/notes`、`~/.local/opt/notes`、解压出来的 `notes-1.4.0-linux-amd64/`）。`Apply` 拒绝（`ErrUnsupported`）明显不是独占安装目录的目标：根目录与任何一级目录（`/usr`、`/opt`、`/tmp`…）、`/home/*`、`/Users/*`、家目录本身及其上级、`/usr/bin`、`/usr/local/bin`、`/usr/lib`、`/usr/share` 等共享系统目录、家目录下的 `bin`、`.local`、`.local/bin`、`.local/share`、`.config`、`opt`、`apps`、`Applications`、`Desktop`、`Downloads`、`Documents`。另外，应用从目标目录运行时，新版本的目录里必须在同一相对路径有可执行文件（否则新版本起不来），不满足就不安装。把可执行文件单独拷到 `/usr/local/bin` 之类的位置运行时请改用 AppImage，或显式设置 `Updater.Target`（仍受上述检查）。

**目录不可写**：`Apply` 不提权。目标所在目录不可写（如普通用户的 `/Applications`、只读卷）返回 `ErrNotWritable`，消息里给出目录和建议；macOS 从 App Translocation 或挂载的 dmg 里运行时也直接报错，提示先拖到“应用程序”。

## 确认与回滚

状态文件在 `StateDir`（默认 `<用户配置目录>/tango-updater/<AppID 或可执行文件名>`）：

- 新版本启动后调用 `u.Confirm()` → 返回 `StatusUpdated`，写 `confirmed`；辅助进程看到后删除旧版本与 `pending.json`。
- 新版本在 `ConfirmTimeout`（默认 60 秒）内没确认（崩溃、卡死、根本起不来）→ 辅助进程把新版本挪开、恢复旧版本、写 `rolledback`（原因）并重新启动旧版本；旧版本里 `Confirm()` 返回一次 `StatusRolledBack`。
- 新版本自己发现问题（比如数据迁移失败）：未确认前调用 `u.Rollback()` 然后退出，辅助进程等它退出后回滚。
- 机器在等待确认期间重启、辅助进程不在了：新版本的 `Confirm` 发现超时已过很久会自己清理旧版本；旧版本发现更新从未发生会报告 `StatusRolledBack`（"the update was not installed"）。

所以：**应用必须在“启动成功”的那个点调用 `Confirm()`**，否则每次更新都会被回滚。

## UI 钩子

`Updater.OnEvent` 收到 `Event{Kind, Release, Patch, Done, Total, Error}`：

`checking` → `available`（带 Release）/ `up-to-date` → `downloading`（`Patch` 表示是否补丁）→ `progress`（Done/Total，约每 50 ms 一次）→ 可能 `patch-failed` 后再 `downloading` → `verified` → `ready`（`Apply` 成功，提示用户重启）；任一步失败 `error`。

### 页面（jsapi）

默认关闭。需要同时 `Options.Allow` 里加 `jsapi.Updates` 并调用 `api.InstallUpdater(u, jsapi.UpdaterOptions{...})`：

| 方法 / 事件 | 说明 |
|---|---|
| `tango:updater.check` | 返回 release（`version, notes, pubDate, format, full, patch`）或 null |
| `tango:updater.download` | 下载上次 check 找到的版本，返回 `{version, patched}` |
| `tango:updater.restart` | 安装并退出应用；还需 `UpdaterOptions.AllowRestart`，否则 `forbidden` |
| `tango:updater.event` | `OnEvent` 的每个事件广播给页面（原有 `OnEvent` 照常调用） |

check/download 同时只允许一个，重复调用返回 `busy`。不开 `AllowRestart` 时，可用 `UpdaterOptions.OnDownloaded` 拿到 Payload，由 Go 侧决定何时 `Apply`。

## tan 命令

- `tan keygen [-o name]`：生成 `name.key` / `name.pub`。
- `tan release-manifest [-C dir] [-build build] [-o release] -key update.key [-prev 旧发布目录]… [-url 基础URL] [-notes 文本|@文件] [-windows exe|nsis] [-linux appimage|dir] [-expires 720h]`
  读取 `tan build`/`tan package` 的输出：darwin 把 .app 打成确定性的 `.app.tar.gz`；windows 用 exe（或 `-windows nsis` 用 `*-setup.exe`）；linux 有 AppImage 用 AppImage，否则用 `tan package` 的 tar.gz（`dir`）。`-prev` 指向上一次发布目录（含其 `latest.json` 与文件，签名须用同一把钥匙），为每个平台生成补丁。输出完整包、补丁、签名后的 `latest.json`。
- `tan sign-update [-key f] [-o out] [-expires 720h] manifest.json`：校验并签名手写/脚本生成的清单（也可对已签名信封重新签名，用来在过期前刷新 `expires`）。`expires` 默认设为现在 + 30 天，`-expires 0` 保留原值；清单没写 `app` 时给出警告。

密钥也可用环境变量 `TANGO_UPDATE_KEY_FILE` 或 `TANGO_UPDATE_KEY`。

## 验证情况与限制

- `go test ./service/updater/...`：签名篡改、换钥匙、未签名、防回滚/最低版本/重放、补丁（随机二进制、真实 Go 程序、树）、补丁失败回退、`/bin/sh` 辅助脚本的确认/超时回滚/主动回滚、Windows 辅助逻辑（用假进程操作在任意平台测试）、完整 `Update`→`Confirm` 流程；`security_test.go`：无 `app` 的清单、过期/无有效期清单与时钟偏差、层层叠加与延后改向的符号链接、畸形补丁（seek 溢出、超大输出声明）、校验后被替换的下载文件、`dir` 目标保护。
- macOS 真实演练：`TANGO_UPDATER_DRILL=1 go test -v -run Drill ./service/updater/internal/e2e`——用 `tan release-manifest` 做三个版本，本地 HTTP 服务器，`open` 启动 0.0.1 → 补丁更新到 0.0.2 并确认；再更新到不确认的 0.0.3 → 3 秒后自动回滚到 0.0.2。演练 app 是 `internal/e2e/drillapp`（链接 tango 的无界面程序），不是 `examples/hello`（后者没有接入 updater）。
- **未在真机验证**：Windows（exe 重命名替换、NSIS/MSI 静默安装、自复制辅助进程）、Linux（AppImage、安装目录）。逻辑有跨平台单元测试，Windows 代码交叉编译通过。
- 回滚只恢复文件，不恢复应用数据；超时回滚时若新版本其实还在运行（只是没调用 `Confirm`），可能出现新旧两个实例。
- macOS 上替换整个 bundle 依赖其所在目录可写；放在 `/Applications` 且属主为管理员时普通用户更新会得到 `ErrNotWritable`。
- 补丁与树补丁在内存中处理整个文件，超大文件（几百 MB 以上）请只发完整包。
