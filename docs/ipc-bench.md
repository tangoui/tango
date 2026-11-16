# IPC 流基准

`examples/duplex` 同时是双向流的示例和基准程序（见
`docs/specs/2026-11-13-ipc-duplex-streams.md` §5.8、§6.3）。第二期每条平台快速通道都拿这里的
兜底传输数据做对照。

## 怎么跑

```sh
go run ./examples/duplex -bench
```

macOS 上 `up:post` 默认开启，这样跑测的就是 up:post 配置；测兜底传输（全部走消息通道）要加
`TANGO_IPC_LANES_DARWIN=0`（iOS 相反：默认兜底，`TANGO_IPC_LANES_DARWIN=1` 才是 up:post）。
macOS 的下行通道 `down:poll` 默认关闭，`TANGO_IPC_LANES_DARWIN=1`（或 `all`）同时开 `down:poll` 和
`up:post`（见下文「down:poll」一节）。

程序打开窗口，加载 `index.html?bench`，页面跑完下面几项后把结果交给 `Bench.Report`，
Go 侧把它原样打印成一行 JSON 到 stdout，然后退出。基准代码在
`examples/duplex/services/bench.js`（`window.duplexBench`，由 `services.BenchRoute` 以
`/bench.js` 提供），iOS 自测示例 `examples/duplex-mobile` 用同一份（启动时设
`TANGO_DUPLEX_BENCH=1`，页面打开 `index.html?bench`，结果记成一行 `bench: {...}` 日志）：

| 项目 | 做法 | 字段 |
| --- | --- | --- |
| 延迟 | 一条 `Echo` 流上 1000 次 64 字节字节帧往返（发一帧、等回显再发下一帧） | `latencyP50Ms`、`latencyP99Ms`（每次往返单独计时）；`latencyMeanMs`（整段耗时 / 1000） |
| 分批延迟 | 新开一条 `Echo` 流，20 批 × 50 次 64 字节往返，每批整体计时 | `latencyBatchMeansMs`（每批耗时 / 50）；`latencyBatchP50Ms`、`latencyBatchP99Ms`（20 个批均值的 p50/p99；20 个样本的 p99 就是最慢的一批） |
| 吞吐 JS→Go | `Sink` 收 1024 帧 × 64 KiB = 64 MiB，计时到 Go 回 `{"bytes":n}` | `upMBps` |
| 吞吐 Go→JS | `Source` 发 1024 帧 × 64 KiB，计时到流结束 | `downMBps` |
| 下行小帧（终端形态） | `Source` 发 20000 帧 × 1 KiB 字节帧，计时到流结束，校验帧数和字节数 | `downSmallFps`（帧/秒）、`downSmallMBps` |
| 下行 JSON（AI / 数据库分块形态） | `SourceJSON` 发 10000 个 JSON 帧，每帧 `{"seq":i,"kind":"rows","rows":[20 行 × 5 列字符串]}`，序列化约 2.3 KB（与 `ipc-bench/tango` 的 `jsonChunk` 完全相同，可与已测的 Wails 数据对比），校验每帧 `rows.length === 20` | `downJsonFps`（帧/秒） |

- 设了 `TANGO_SCHEME_PROBE=1` 时结果多一个 `upMain`（macOS 的协议处理器计数器只编进开发构建，要
  `TANGO_SCHEME_PROBE=1 go run -tags tango_dev ./examples/duplex -bench`；普通构建里这些计数恒为 0）：上行期间 `up:post` 请求数、请求体字节数，
  以及 macOS 协议处理器在主线程上为这些请求花的时间（`serveSchemeTask` 开始到交给处理 goroutine，
  含拷贝请求体；`totalMs`、`perRequestUs`、`maxUs`）。`requests`、`bodyBytes`、`totalMs` 是上行前后计数器的差；
  `maxUs` 不是差值，是整个进程到那时为止最长的一次（包括上行之前的请求，比如 `up:post` 启动时的空包探测）。
  iOS 上为 0（没有计）。
- `MB/s` 按 10^6 字节/秒算，只计负载，不计 base64 和消息外壳。
- `lane` 是跑完后页面 `tango.lanes()` 的值：`{"down":…,"up":…}`，各方向是快速通道名（如 `"down:poll"`、`"up:post"`），或 `"fallback"`（字符串消息通道，字节帧走 base64）。第一期的基线数据（下表）是改成对象之前跑的，当时只有兜底传输，字段是字符串 `"fallback"`。
- **计时精度**：macOS 的 WKWebView 在页面没有跨源隔离时把 `performance.now()` 截到 1 ms
  （实测最小增量 1 ms，`crossOriginIsolated` 为 false，加 COOP/COEP 响应头也不生效）。
  所以 macOS 上单次往返的 p50/p99 只能读成 0 或 1 ms，看 `latencyMeanMs` 和分批延迟更有意义
  （批均值的精度是 1 ms / 50 = 0.02 ms）；
  计时精度更高的平台（Chromium 系的 WebView2 等）p50/p99 才可直接比较。

## 第一期基线（兜底传输）

| 平台 | 机器 | 系统 / WebView | p50 (ms) | p99 (ms) | 平均 (ms) | JS→Go (MB/s) | Go→JS (MB/s) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| macOS | Mac16,10（Apple M4） | macOS 26.6 (25G72)，WKWebView | 0 † | 1 † | 0.124 | 255.17 | 328.97 |
| Windows | 待测 | WebView2 | 待测 | 待测 | 待测 | 待测 | 待测 |
| Linux | 待测 | WebKitGTK | 待测 | 待测 | 待测 | 待测 | 待测 |
| Android | 待测 | Android WebView | 待测 | 待测 | 待测 | 待测 | 待测 |
| iOS | 待测 | WKWebView | 待测 | 待测 | 待测 | 待测 | 待测 |

† 受 1 ms 计时精度限制，见上文。

macOS 原始输出（2026-11-13，连续三次，表里取第三次）：

```json
{"lane":"fallback","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.168,"upMBps":262.14,"downMBps":315.07}
{"lane":"fallback","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.127,"upMBps":247.63,"downMBps":312.13}
{"lane":"fallback","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.124,"upMBps":255.17,"downMBps":328.97}
```

机器信息：`sysctl -n hw.model` → `Mac16,10`；`sysctl -n machdep.cpu.brand_string` → `Apple M4`；
`sw_vers` → macOS 26.6，Build 25G72。

## 第二期 macOS/iOS（up:post）

2026-11-14 测。Apple 平台只提供 `up:post`（WebKit 会扣住大段 `down:scheme` 数据，见规格 §6.2），
所以对比的是两种配置（历史记录：这是当时的情况；`down:scheme` 已删除，macOS 现在另可开 `down:poll`，
见后文）：**兜底**（测量时 `TANGO_IPC_LANES_DARWIN` 未设，当时默认关闭）和 **up:post**
（`TANGO_IPC_LANES_DARWIN=1`）。根据下面的结果 macOS 现在默认开启 `up:post`：现在重跑兜底配置要设
`TANGO_IPC_LANES_DARWIN=0`，up:post 配置不设也一样。下行在两种配置下都走消息通道，应当持平。

- 机器：`sysctl -n hw.model` → `Mac16,10`（Apple M4，10 核），macOS 26.6（25G72），WKWebView。
- iOS：上面这台 Mac 上的 iPhone 17 模拟器，iOS 27.2。**模拟器数据只作参考，不计入判定。**
- 方法：同一个二进制 `go build -o … ./examples/duplex` 跑 `-bench`；两种配置交替（兜底、up:post、兜底……），
  各 5 次。每次跑前看 1 分钟负载，低于 5（核数一半）才跑，最多等 10 分钟，等不到照跑并标"负载下"。
  机器是共享的，测试期间负载在 4～30 之间波动；表里的负载是跑前/跑后读到的 1 分钟值
  （每次只跑约 2 秒，负载值 5 秒才更新一次，只能粗看）。
- 判定（规格 §6.3）：64 KiB 帧吞吐 ≥ 兜底的 2 倍，且 64 B 分批 p99 不比兜底差（允许 +5% 噪声）。
  判定用中位数。分批 p99 只有 20 个样本（20 个批均值），所以它就是最慢的一批。
- **64 B 延迟这一项在两种配置下测的是同一条路**：64 B 帧小于 `up:post` 的门槛（16 KiB），两种配置都走
  消息通道；而且延迟在上传之前测，那时 `up:post` 还没有数据在跑。所以这一项只能说明"开了 up:post
  没有拖慢小帧"，两边的差别是噪声：下面 macOS 的 0.53 **不是**改进，iOS 模拟器的 1.36 也**不是**退步，
  两者按同一个道理看。

### macOS 判定组（10 次全部在负载 < 5 时开始）

| 次 | 配置 | 负载（前 → 后） | JS→Go (MB/s) | Go→JS (MB/s) | 分批 p50 (ms) | 分批 p99 (ms) | 平均 (ms) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 兜底 | 4.73 → 4.99 | 200.32 | 143.40 | 0.16 | 0.30 | 0.161 |
| 1 | up:post | 4.99 → 4.99 | 479.35 | 212.37 | 0.12 | 0.14 | 0.309 |
| 2 | 兜底 | 4.99 → 4.99 | 233.83 | 248.55 | 0.12 | 0.16 | 0.184 |
| 2 | up:post | 4.99 → 12.12 ‡ | 204.60 | 195.65 | 0.38 | 3.02 | 0.595 |
| 3 | 兜底 | 4.76 → 4.76 | 270.60 | 233.83 | 0.10 | 0.16 | 0.206 |
| 3 | up:post | 4.76 → 4.94 | 789.52 | 255.17 | 0.12 | 0.14 | 0.157 |
| 4 | 兜底 | 4.94 → 4.94 | 286.79 | 261.12 | 0.10 | 0.40 | 0.138 |
| 4 | up:post | 4.94 → 4.94 | 958.70 | 293.05 | 0.10 | 0.46 | 0.132 |
| 5 | 兜底 | 4.94 → 4.94 | 270.60 | 288.02 | 0.12 | 0.32 | 0.198 |
| 5 | up:post | 4.94 → 4.94 | 737.46 | 242.27 | 0.12 | 0.16 | 0.162 |

‡ 这一次跑的过程中负载冲到 12，所有指标都偏差；中位数不受它左右，照样保留。

| 指标 | 兜底 中位数（最小～最大） | up:post 中位数（最小～最大） | 比值 | 要求 |
| --- | --- | --- | --- | --- |
| JS→Go 64 KiB (MB/s) | 270.60（200.32～286.79） | 737.46（204.60～958.70） | 737.46 / 270.60 = **2.73×** | ≥ 2× ✔ |
| Go→JS 64 KiB (MB/s) | 248.55（143.40～288.02） | 242.27（195.65～293.05） | 242.27 / 248.55 = 0.97× | 持平（不走通道） |
| 分批 p99 (ms) | 0.30（0.16～0.40） | 0.16（0.14～3.02） | 0.16 / 0.30 = **0.53** | ≤ 1.05 ✔ |
| 分批 p50 (ms) | 0.12（0.10～0.16） | 0.12（0.10～0.38） | 1.00 | — |
| 平均 (ms) | 0.184（0.138～0.206） | 0.162（0.132～0.595） | 0.88 | — |

**判定（macOS）**：上行 `up:post` **达标**（吞吐 2.73×；分批 p99 0.16 ms 不差于 0.30 ms，两边都是消息通道，
这个比值是噪声，不是通道带来的改进）。
下行在 Apple 上没有通道，两种配置都是消息通道，0.97× 属于噪声，不适用 2× 要求。

### macOS 复核组（判定组之后紧接着又交替跑 5 次）

| 次 | 配置 | 负载（前 → 后） | JS→Go | Go→JS | 分批 p50 | 分批 p99 | 平均 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 兜底 | 4.42 → 4.42 | 260.11 | 268.44 | 0.14 | 0.18 | 0.236 |
| 1 | up:post | 4.42 → 4.42 | 849.48 | 264.21 | 0.12 | 0.16 | 0.159 |
| 2 | 兜底 | 4.42 → 6.46 | 244.03 | 244.92 | 0.12 | 0.22 | 0.171 |
| 2 | up:post | 4.94 → 4.94 | 945.20 | 315.07 | 0.10 | 0.14 | 0.133 |
| 3 | 兜底 | 4.94 → 4.94 | 258.11 | 280.79 | 0.10 | 0.14 | 0.158 |
| 3 | up:post | 4.94 → 4.94 | 578.52 | 136.96 | 0.12 | 0.20 | 0.167 |
| 4 | 兜底 | 4.94 → 10.39 | 256.14 | 279.62 | 0.18 | 0.22 | 0.237 |
| 4 | up:post | 14.55（负载下） | 671.09 | 263.17 | 0.12 | 0.36 | 0.166 |
| 5 | 兜底 | 29.47（负载下） | 221.48 | 154.99 | 0.22 | 0.34 | 0.285 |
| 5 | up:post | 8.16（负载下） | 789.52 | 238.82 | 0.12 | 0.16 | 0.162 |

复核组中位数：JS→Go 兜底 256.14 / up:post 789.52 → **3.08×**；Go→JS 268.44 / 263.17 → 0.98×；
分批 p99 0.22 / 0.16 → 0.73。两组里负载 < 5 开始的 17 次合起来：JS→Go 258.11 / 763.49 → **2.96×**，
分批 p99 0.22 / 0.16 → 0.73。结论与判定组一致。

### up:post 的主线程开销（macOS，`-tags tango_dev` + `TANGO_SCHEME_PROBE=1`，3 次）

上行 64 MiB 期间 `serveSchemeTask` 在主线程上为 `up:post` 请求花的时间（含从 `HTTPBody` 拷贝请求体）：

| 次 | 请求数 | 请求体合计 | 主线程合计 (ms) | 每请求 (µs) | 最长一次 (µs) | 同次 JS→Go (MB/s) |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 99 | ≈ 63 MiB | 4.52 | 45.7 | 247 | 871.54 |
| 2 | 95 | ≈ 63 MiB | 4.48 | 47.1 | 167 | 838.86 |
| 3 | 101 | ≈ 63 MiB | 4.33 | 42.9 | 242 | 919.30 |

请求体合计约 63 MiB 而不是 64 MiB：Go `accept` 这条流之前发的帧走消息通道。"最长一次"是进程内的最大值，
不只算这次上行（见上文 `upMain`）。

每个请求平均约 0.66 MiB，主线程上约 45 µs，最长 0.25 ms；约 63 MiB 的 `up:post` 请求体一共占主线程约 4.5 ms
（上传本身约 75 ms）。Task 2 关于"请求体留在主线程读"的裁定不需要改。

### 参考：`TANGO_IPC_LANES_DARWIN=all`（实验，不计入判定）

（历史记录。当时 `=all` 只在开发构建里生效、开的是 `down:scheme`；`down:scheme` 已删除，现在 `all` 与 `1`
相同，在任何构建里都开 `down:poll` + `up:post`，见下一节。）

同时开 `down:scheme` 和 `up:post`，3 次：第 1 次 JS→Go 798.92 MB/s、**Go→JS 1100.15 MB/s**；
第 2、3 次失败，页面报 `frames lost on a fast lane`（下行大段数据被 WebKit 扣住，与 Task 4 的发现一致）。
`down:scheme` 在 Apple 上继续关闭。

### iOS 模拟器（仅作参考）

iPhone 17 模拟器，iOS 27.2，`examples/duplex-mobile`（`SIMCTL_CHILD_TANGO_DUPLEX_BENCH=1`，
up:post 组再加 `SIMCTL_CHILD_TANGO_IPC_LANES_DARWIN=1`），交替各 5 次，全部在负载 < 5 时开始：

| 次 | 配置 | 负载（前 → 后） | JS→Go | Go→JS | 分批 p50 | 分批 p99 | 平均 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 兜底 | 4.21 → 4.57 | 239.67 | 275.04 | 0.18 | 0.40 | 0.196 |
| 1 | up:post | 4.57 → 5.40 | 528.42 | 227.49 | 0.20 | 0.38 | 0.197 |
| 2 | 兜底 | 4.94 → 4.86 | 267.37 | 279.62 | 0.16 | 0.20 | 0.216 |
| 2 | up:post | 4.87 → 4.87 | 568.72 | 275.04 | 0.16 | 0.24 | 0.201 |
| 3 | 兜底 | 4.87 → 4.72 | 247.63 | 294.34 | 0.16 | 0.28 | 0.213 |
| 3 | up:post | 4.72 → 5.54 | 627.19 | 259.11 | 0.18 | 0.40 | 0.239 |
| 4 | 兜底 | 4.96 → 6.16 | 267.37 | 307.84 | 0.16 | 0.34 | 0.231 |
| 4 | up:post | 4.93 → 4.77 | 516.22 | 270.60 | 0.18 | 0.22 | 0.205 |
| 5 | 兜底 | 4.77 → 4.79 | 251.34 | 233.02 | 0.16 | 0.24 | 0.182 |
| 5 | up:post | 4.79 → 5.05 | 745.65 | 277.31 | 0.20 | 0.50 | 0.224 |

| 指标 | 兜底 中位数（最小～最大） | up:post 中位数（最小～最大） | 比值 |
| --- | --- | --- | --- |
| JS→Go (MB/s) | 251.34（239.67～267.37） | 568.72（516.22～745.65） | **2.26×** |
| Go→JS (MB/s) | 279.62（233.02～307.84） | 270.60（227.49～277.31） | 0.97× |
| 分批 p99 (ms) | 0.28（0.20～0.40） | 0.38（0.22～0.50） | **1.36**（> 1.05） |
| 分批 p50 (ms) | 0.16（0.16～0.18） | 0.18（0.16～0.20） | 1.13 |
| 平均 (ms) | 0.213（0.182～0.231） | 0.205（0.197～0.239） | 0.96 |

模拟器上吞吐过了 2×，分批 p99 按字面没过（0.38 对 0.28）。但和 macOS 一样：64 B 帧小于 `up:post` 的门槛
（16 KiB），两种配置下延迟测的都是消息通道；延迟在上传之前测，那时通道空闲（macOS 的 0.53 同理不算改进）；两组 p99 的范围
（0.20～0.40 对 0.22～0.50）大面积重叠，1000 次往返的平均反而 up:post 略快（0.205 对 0.213）。
这更像模拟器上的噪声，而不是 up:post 拖慢了小帧；但模拟器不能代替真机，iOS 的判定要等真机数据。
注意 iOS 上 `upMain` 没有计（主线程读请求体在 `bridge.m` 里）。

## 第二期 Android（down:android / up:android）

数据来自 **模拟器（arm64 系统镜像跑在 Apple Silicon 上）**，没有真机数据。宿主机 Mac16,10（Apple M4，10 核），macOS 26.6 (25G72)；AVD `tango36`（API 36，`android-36;default;arm64-v8a`），WebView 133.0.6943.137。应用是 `examples/duplex-mobile` 的 bench 模式，`tango_selftest` 可调试 APK（arm64-v8a），启动参数 `--esa TANGO_TEST_ENV TANGO_DUPLEX_BENCH=1`，通道组再加 `TANGO_IPC_LANES_ANDROID=1`。兜底 = 下行 `evaluateJavascript`、上行字符串 `postMessage`（base64）。两种配置交替，每次全新启动（`am force-stop` 后 `am start`）；开始前等宿主机 1 分钟负载低于 5（核数一半），最多等 10 分钟，仍不满足就标"负载下"。测试钩子构建只多了几个原子计数器。

| 次 | 配置 | 负载（前 → 后） | JS→Go (MB/s) | Go→JS (MB/s) | 分批 p50 (ms) | 分批 p99 (ms) | 平均 (ms) |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 兜底 | 4.82 → 4.59 | 37.85 | 60.93 | 0.45 | 6.878 | 0.938 |
| 1 | android | 4.59 → 6.71 | 228.03 | 259.91 | 0.648 | 0.966 | 1.213 |
| 2 | 兜底 | 4.86 → 7.33 | 20.21 | 45.84 | 0.726 | 1.364 | 1.881 |
| 2 | android | 37.44（负载下） | 14.66 | 13.48 | 5.87 | 20.77 | 17.062 |
| 3 | 兜底 | 8.14（负载下） | 13.26 | 22.05 | 4.998 | 12.654 | 3.65 |
| 3 | android | 5.34（负载下） | 197.09 | 233.91 | 0.738 | 1.184 | 1.36 |
| 4 | 兜底 | 4.98 → 4.98 | 32.54 | 45.08 | 0.58 | 8.016 | 0.851 |
| 4 | android | 4.98 → 5.48 | 190.60 | 207.13 | 0.674 | 1.088 | 1.678 |
| 5 | 兜底 | 4.97 → 5.05 | 29.35 | 48.81 | 0.602 | 8.184 | 0.82 |
| 5 | android | 4.88 → 6.60 | 199.67 | 116.51 | 0.69 | 1.22 | 1.478 |
| 6 | 兜底 | 4.97 → 6.08 | 41.57 | 59.18 | 0.504 | 7.052 | 0.946 |
| 6 | android | 4.89 → 4.58 | 255.17 | 303.94 | 0.576 | 7.496 | 0.976 |
| 7 | 兜底 | 4.58 → 4.45 | 37.35 | 59.96 | 0.522 | 7.698 | 0.822 |
| 7 | android | 4.45 → 4.66 | 265.46 | 304.90 | 0.582 | 6.906 | 0.98 |

第 2、3 次开始时宿主机上另有负载（第 2 次通道组开始时 1 分钟负载 37），所以接着补了第 6、7 次。判定用负载正常的 5 组（第 1、4、5、6、7 次）：

| 指标 | 兜底 中位数（最小～最大） | android 中位数（最小～最大） | 比值 | 要求 |
| --- | --- | --- | --- | --- |
| JS→Go 64 KiB (MB/s) | 37.35（29.35～41.57） | 228.03（190.60～265.46） | **6.11×** | ≥ 2× ✔ |
| Go→JS 64 KiB (MB/s) | 59.18（45.08～60.93） | 259.91（116.51～304.90） | **4.39×** | ≥ 2× ✔ |
| 分批 p99 (ms) | 7.698（6.878～8.184） | 1.22（0.966～7.496） | **0.16** | ≤ 1.05 ✔ |
| 分批 p50 (ms) | 0.522（0.45～0.602） | 0.648（0.576～0.69） | 1.24 | — |
| 平均 (ms) | 0.851（0.82～0.946） | 1.213（0.976～1.678） | 1.43 | — |

（7 次全算：JS→Go 6.14×、Go→JS 4.79×、分批 p99 0.16，结论相同。）

按规格 §6.3 的两条判定都过了：两个方向的大帧吞吐都远超 2 倍，分批 p99 不比兜底差。需要注意的两点：

- 分批 p99 呈两档：通道组 7 次里 5 次在 1 ms 左右，第 6、7 次约 7 ms（与兜底同档）；兜底组 7 次里 6 次在 7～13 ms。中位数明显更好，但不是每次都更好。
- **小帧的中位延迟略差**：分批 p50 慢约 24%（0.648 对 0.522 ms），1000 次往返的平均慢约 43%。64 B 帧在通道上每帧要走一次 ArrayBuffer `postMessage`、一次 JNI 拷贝，下行还要排队等主线程 `drain`；兜底的字符串消息只有一次跨进程。§6.3 只要求 p99，这一点不影响判定，但如果应用大量收发很小的字节帧，打开通道的收益主要在吞吐和尾延迟上。没有做攒批（计划 Task 3：每帧一包）。

**结论：Android 默认开启 `down:android` / `up:android`**（用户 2026-11-14 根据上面的数据决定）。`TANGO_IPC_LANES_ANDROID=0`（或 `false`、`off`）关闭，`TANGO_IPC_LANES=0` / `Options.DisableFastLanes` 关闭所有平台的通道。真机数据仍待补。

## down:poll（macOS 下行轮询通道）

2026-11-15 测。`down:poll` 取代了 `down:scheme`（设计见
`docs/superpowers/specs/2026-11-14-down-poll-design.md`）：页面始终挂一个 `GET /__tango/lane/poll`，
Go 有消息时用一批消息（字符串消息、字节包、`Eval` 栅栏）结束这次响应，页面立即再发下一个请求；
每条 `Surface.Send` 都走它，不只是流的字节帧。门槛是设计第 6 节：

- 下行三项（64 KiB 字节帧、1 KiB 小帧、约 2.3 KB JSON 帧）的中位数都不低于 Wails v3.1.0；
- 64 B 平均延迟和分批 p99 不比改造前的 Tango 差超过 10%；上行吞吐不下降；
- Task 8 的真实 WebKit 场景全部通过。

**判定：未达标，`down:poll` 在 macOS 上保持默认关闭**，`TANGO_IPC_LANES_DARWIN=1`（或 `all`、`down`）打开。

### 方法

- 机器：`Mac16,10`（Apple M4，10 核），macOS 26.6（25G72），WKWebView。
- 三种配置交替各 5 次（A、B、C、A、B、C……），脚本 `/Users/admin/min/ipc-bench/run-downpoll.sh`
  （开跑前等 1 分钟负载 < 5），汇总 `summarize-downpoll.py`，原始数据
  `ipc-bench/results/downpoll-20261115-003757.jsonl`：
  - **A 改造前**：`main` 的 `e215b91` 加上 Task 7 的基准补丁（小帧、JSON 两项，`5310d1b`），
    `go build ./examples/duplex`，默认配置（下行走消息通道 + `up:post`）；
  - **B down:poll**：本分支（`feat/down-poll`，含下文三个优化），`TANGO_IPC_LANES_DARWIN=1`
    （`down:poll` + `up:post`）；
  - **C Wails v3.1.0 Streams**：`ipc-bench/wails`（`-tags production` 构建，同样六项）。
- 15 次的负载（跑前 1 分钟值）：2.86～3.85，全部 < 5。

### 结果（5 次中位数，括号内为范围）

| 项目 | A 改造前 | B down:poll | C Wails | B / A | B / C |
| --- | --- | --- | --- | --- | --- |
| 64 B 平均延迟 (ms) | 0.127（0.123～0.133） | 0.139（0.124～0.147） | 0.254（0.244～0.274） | +9.4% | — |
| 分批 p99 (ms) | 0.14（0.12～0.14） | 0.14（0.14～0.20） | 0.28（0.28～0.28） | +0% | — |
| 上行 64 KiB (MB/s) | 860（839～883） | 818（809～860） | 147（146～153） | −4.9% | — |
| 下行 64 KiB (MB/s) | 306（292～314） | 1017（895～1082） | 1766（1678～1917） | 3.3× | **0.58** |
| 下行 1 KiB 小帧 (帧/s) | 4.5 万（4.3～4.8 万） | 50.0 万（42.6～51.3 万） | 62.5 万（60.6～69.0 万） | 11.2× | **0.80** |
| 下行 2.3 KB JSON (帧/s) | 3.4 万（3.1～3.5 万） | 11.1 万（8.5～11.9 万） | 10.8 万（3.3～15.9 万） | 3.3× | 1.03 |

| 门槛 | 结果 |
| --- | --- |
| 下行 64 KiB ≥ Wails | 1017 对 1766 MB/s，**未达标**（范围不重叠） |
| 下行小帧 ≥ Wails | 50.0 万对 62.5 万帧/s，**未达标**（范围不重叠） |
| 下行 JSON ≥ Wails | 11.1 万对 10.8 万帧/s，达标（Wails 的范围很宽） |
| 平均延迟 ≤ A × 1.1 | 0.139 对 0.140（0.127 × 1.1），达标，余量很小 |
| 分批 p99 ≤ A × 1.1 | 0.14 对 0.14，达标 |
| 上行不下降 | 818 对 860 MB/s（−4.9%，范围 809～860 对 839～883 部分重叠），按字面**未达标** |
| Task 8 真实 WebKit 场景 | 全部通过（见设计文档状态） |

注意 Wails 这次的下行数字比 2026-11-14 那份对比（负载 12～14：64 KiB 919 MB/s、小帧 39.2 万帧/s）
高得多：负载低时 Wails 的下行快很多，门槛是按这次交替测量的中位数判的。

### 为什么字节帧下行追不上 Wails

**流控窗口**。ipc 的流有按字节计的信用窗口（`DefaultWindowBytes` 1 MiB，读走半个窗口才归还一次信用，
规格 §5.3），一条流最多 1 MiB 在途；Wails 的流没有流控，Go 把帧全部排进队列。64 KiB 帧的下行实测是
每批正好半个窗口（约 8 帧，64 MiB 用约 128 次轮询），每轮"页面读完 → 归还信用 → Go 写出 → 轮询响应
到达"约 450 µs，所以吞吐被"窗口 / 往返"封顶。诊断实验（不提交）：把两端窗口临时改成 8 MiB，同一台机器
（负载约 9）下行 64 KiB 到 1400～1560 MB/s、小帧 53～57 万帧/s，轮询次数从约 128 降到 18。窗口大小和
归还规则是协议的一部分（runtime-js 不通告接收窗口，两端都按 1 MiB 算），本项目不改协议，所以停在这里。

小帧也受同样的限制（1 MiB 约 1000 帧），另外每个走通道的字节帧都要再发一条 `boundary` 控制消息
（规格 §6.1 的丢帧检测），页面上的条目数是帧数的两倍。

### 优化过程（都在 B 里）

先测了每次轮询带多少条、时间花在哪（页面侧临时计时 + Go 侧 pprof，结束后都已删除）：

1. **`Surface.Send` 不再每次查主线程**（`perf(core): Send skips the main-thread check…`）。
   pprof：小帧下行时写帧的 goroutine 约 90% 的 CPU 花在 `isMain` 里——它用 `runtime.Stack` 取
   goroutine id，每次约 10 µs，限制了 Go 每秒能排进队列的帧数，每次轮询只带约 14 帧（1424 次轮询 / 2 万帧）。
   改为只有兜底路径真的有待送内容时才查。之后每次轮询带约 900 帧（43 次），小帧 7 万 → 约 50 万帧/s，
   JSON 5.1 万 → 约 11 万帧/s。
2. **空闲时小消息走 Eval**（设计第 6 节的延迟退路，`perf(downq): small items on an idle lane go by Eval`）。
   优化 1 之后 64 B 平均延迟仍是 0.16～0.19 ms（A 约 0.13）。测得一次轮询响应在 Go 里：从入队到挂起的请求被
   唤醒约 8 µs，到主线程开始回 WebKit 约 11 µs；之后 WebKit 还要三次调用（响应头、数据、结束）和页面
   `fetch`、`arrayBuffer` 两次异步跳转，整体比一次 `evaluateJavaScript` 多约 40 µs。规则：有轮询请求挂着、
   没有待送也没有未确认的条目时，≤ 1 KiB 的条目改走兜底（Eval），每轮主线程最多 8 条
   （`downq.IdleEvalBytes`、`IdleEvalItems`），超出的照常排队等轮询，所以流量大时仍走通道。之后平均延迟
   约 0.13 ms，下行吞吐不变。
3. **页面按序批次直接交付**（`perf(core): downShim delivers an in-order poll batch directly`）：批次从下一个
   序号开始、没有缓存的乱序条目时不经过重排表；小帧一轮的页面处理时间 11～15 ms → 5～7 ms。
   在总时间里占比小，基准上看不出明显差别。

试过但没保留：轮询请求延后到页面处理完再发（延迟无变化）；Go 被唤醒后多等 20～200 µs 攒批（每批本来就
是半个窗口，无变化）；大负载分段写出省一次拷贝（WebKit 收到多次 `didReceiveData`，反而变慢）。

### `Eval` 栅栏往返

专门测了一次（临时代码，不提交）：页面调用一个 ipc 方法，Go 在处理函数里 `s.Eval("…__evalPong()")`，
页面等到这段脚本运行；另测只调用不 Eval 的往返作对照。每项 1000 次取平均，各 3 次：

| 配置 | 调用往返 (ms) | 调用 + Eval 脚本运行 (ms) |
| --- | --- | --- |
| A 改造前 | 0.089～0.098 | 0.086～0.090 |
| B down:poll（空闲规则生效，Eval 走兜底） | 0.088～0.093 | 0.088～0.105 |
| B 关掉空闲规则（`IdleEvalItems=0`，Eval 是轮询通道上的栅栏） | 0.137～0.142 | 0.202～0.206 |

通道空闲时 `Eval` 走兜底，和改造前一样快；通道忙（有排队的条目）时 `Eval` 成为栅栏，比轮询送达的
消息多一次页面 → Go 往返，约 +0.06 ms。

### 原始数据

```json
{"cfg":"A","round":1,"load":2.86,"r":{"lane":{"down":"fallback","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.123,"latencyBatchP50Ms":0.12,"latencyBatchP99Ms":0.14,"latencyBatchMeansMs":[0.12,0.12,0.12,0.14,0.1,0.1,0.12,0.12,0.1,0.1,0.12,0.1,0.12,0.12,0.1,0.12,0.1,0.12,0.12,0.12],"upMBps":860.37,"downMBps":306.43,"downSmallFps":44543,"downSmallMBps":45.61,"downJsonFps":35088}}
{"cfg":"B","round":1,"load":3.03,"r":{"lane":{"down":"down:poll","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.146,"latencyBatchP50Ms":0.14,"latencyBatchP99Ms":0.2,"latencyBatchMeansMs":[0.2,0.14,0.16,0.12,0.14,0.14,0.16,0.14,0.12,0.16,0.1,0.18,0.2,0.18,0.14,0.16,0.14,0.18,0.16,0.14],"upMBps":818.4,"downMBps":894.78,"downSmallFps":425532,"downSmallMBps":435.74,"downJsonFps":85470}}
{"cfg":"C","round":1,"load":3.43,"r":{"downSmallFps":645161,"downSmallMBps":660.65,"downJsonFps":43478,"impl":"wails-3.1.0-streams","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) wails.io","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.274,"latencyBatchP50Ms":0.24,"latencyBatchP99Ms":0.28,"latencyBatchMeansMs":[0.28,0.26,0.26,0.26,0.24,0.26,0.26,0.24,0.24,0.26,0.24,0.24,0.26,0.22,0.26,0.24,0.24,0.26,0.22,0.24],"upMBps":146.53,"downMBps":1720.74}}
{"cfg":"A","round":2,"load":3.43,"r":{"lane":{"down":"fallback","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.133,"latencyBatchP50Ms":0.12,"latencyBatchP99Ms":0.14,"latencyBatchMeansMs":[0.14,0.1,0.12,0.14,0.12,0.1,0.12,0.12,0.12,0.12,0.1,0.12,0.12,0.1,0.12,0.12,0.12,0.1,0.12,0.12],"upMBps":838.86,"downMBps":291.78,"downSmallFps":43197,"downSmallMBps":44.23,"downJsonFps":31646}}
{"cfg":"B","round":2,"load":3.64,"r":{"lane":{"down":"down:poll","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.147,"latencyBatchP50Ms":0.12,"latencyBatchP99Ms":0.14,"latencyBatchMeansMs":[0.12,0.12,0.1,0.12,0.12,0.1,0.14,0.12,0.1,0.12,0.12,0.14,0.12,0.12,0.12,0.12,0.12,0.12,0.12,0.12],"upMBps":818.4,"downMBps":1001.62,"downSmallFps":500000,"downSmallMBps":512,"downJsonFps":111111}}
{"cfg":"C","round":2,"load":3.42,"r":{"downSmallFps":625000,"downSmallMBps":640,"downJsonFps":107527,"impl":"wails-3.1.0-streams","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) wails.io","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.244,"latencyBatchP50Ms":0.24,"latencyBatchP99Ms":0.28,"latencyBatchMeansMs":[0.26,0.22,0.24,0.24,0.24,0.28,0.26,0.26,0.28,0.26,0.24,0.24,0.24,0.24,0.22,0.22,0.24,0.24,0.22,0.24],"upMBps":146.21,"downMBps":1766.02}}
{"cfg":"A","round":3,"load":3.42,"r":{"lane":{"down":"fallback","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.128,"latencyBatchP50Ms":0.1,"latencyBatchP99Ms":0.12,"latencyBatchMeansMs":[0.12,0.12,0.12,0.1,0.1,0.1,0.12,0.12,0.1,0.12,0.1,0.1,0.12,0.1,0.12,0.1,0.1,0.1,0.1,0.1],"upMBps":871.54,"downMBps":313.59,"downSmallFps":47847,"downSmallMBps":49,"downJsonFps":33557}}
{"cfg":"B","round":3,"load":3.55,"r":{"lane":{"down":"down:poll","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.124,"latencyBatchP50Ms":0.12,"latencyBatchP99Ms":0.14,"latencyBatchMeansMs":[0.14,0.12,0.12,0.12,0.12,0.1,0.12,0.1,0.12,0.12,0.1,0.12,0.1,0.12,0.1,0.12,0.14,0.1,0.12,0.12],"upMBps":860.37,"downMBps":1082.4,"downSmallFps":512821,"downSmallMBps":525.13,"downJsonFps":119048}}
{"cfg":"C","round":3,"load":3.43,"r":{"downSmallFps":606061,"downSmallMBps":620.61,"downJsonFps":32895,"impl":"wails-3.1.0-streams","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) wails.io","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.264,"latencyBatchP50Ms":0.24,"latencyBatchP99Ms":0.28,"latencyBatchMeansMs":[0.26,0.24,0.28,0.24,0.24,0.26,0.24,0.24,0.26,0.24,0.22,0.26,0.24,0.24,0.24,0.22,0.24,0.24,0.24,0.26],"upMBps":146.53,"downMBps":1677.72}}
{"cfg":"A","round":4,"load":3.43,"r":{"lane":{"down":"fallback","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.123,"latencyBatchP50Ms":0.1,"latencyBatchP99Ms":0.12,"latencyBatchMeansMs":[0.12,0.12,0.1,0.12,0.1,0.1,0.12,0.1,0.1,0.12,0.1,0.12,0.1,0.12,0.12,0.1,0.1,0.12,0.1,0.12],"upMBps":883.01,"downMBps":312.13,"downSmallFps":46404,"downSmallMBps":47.52,"downJsonFps":34247}}
{"cfg":"B","round":4,"load":3.63,"r":{"lane":{"down":"down:poll","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.139,"latencyBatchP50Ms":0.12,"latencyBatchP99Ms":0.16,"latencyBatchMeansMs":[0.12,0.12,0.16,0.12,0.12,0.12,0.12,0.12,0.12,0.12,0.12,0.12,0.12,0.12,0.12,0.14,0.12,0.1,0.14,0.12],"upMBps":808.54,"downMBps":1016.8,"downSmallFps":476190,"downSmallMBps":487.62,"downJsonFps":106383}}
{"cfg":"C","round":4,"load":3.66,"r":{"downSmallFps":625000,"downSmallMBps":640,"downJsonFps":120482,"impl":"wails-3.1.0-streams","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) wails.io","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.253,"latencyBatchP50Ms":0.24,"latencyBatchP99Ms":0.28,"latencyBatchMeansMs":[0.24,0.26,0.28,0.24,0.24,0.22,0.24,0.24,0.24,0.22,0.24,0.22,0.24,0.24,0.22,0.24,0.26,0.28,0.24,0.24],"upMBps":150.47,"downMBps":1766.02}}
{"cfg":"A","round":5,"load":3.66,"r":{"lane":{"down":"fallback","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.127,"latencyBatchP50Ms":0.12,"latencyBatchP99Ms":0.14,"latencyBatchMeansMs":[0.12,0.12,0.12,0.12,0.1,0.12,0.12,0.1,0.12,0.12,0.12,0.12,0.1,0.1,0.12,0.12,0.1,0.12,0.14,0.1],"upMBps":860.37,"downMBps":296.94,"downSmallFps":42827,"downSmallMBps":43.85,"downJsonFps":30960}}
{"cfg":"B","round":5,"load":3.85,"r":{"lane":{"down":"down:poll","up":"up:post"},"platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.13,"latencyBatchP50Ms":0.12,"latencyBatchP99Ms":0.14,"latencyBatchMeansMs":[0.14,0.12,0.12,0.14,0.1,0.12,0.12,0.12,0.12,0.12,0.1,0.14,0.12,0.12,0.12,0.14,0.12,0.12,0.12,0.14],"upMBps":849.48,"downMBps":1048.58,"downSmallFps":500000,"downSmallMBps":512,"downJsonFps":116279}}
{"cfg":"C","round":5,"load":3.78,"r":{"downSmallFps":689655,"downSmallMBps":706.21,"downJsonFps":158730,"impl":"wails-3.1.0-streams","platform":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) wails.io","latencyP50Ms":0,"latencyP99Ms":1,"latencyMeanMs":0.254,"latencyBatchP50Ms":0.24,"latencyBatchP99Ms":0.28,"latencyBatchMeansMs":[0.28,0.24,0.26,0.24,0.24,0.24,0.24,0.24,0.22,0.24,0.24,0.22,0.24,0.26,0.22,0.24,0.22,0.26,0.22,0.24],"upMBps":153.22,"downMBps":1917.4}}
```

`cfg`：A 改造前、B down:poll、C Wails；`load` 是跑前的 1 分钟负载。

### 2026-11-15 流控窗口与有序下行优化

本轮在 `feat/down-poll` 上完成三项协议兼容优化：有序可靠的 `down:poll` 不再发送 `boundary`；runtime-js 在 `hello` 通告 1 MiB 页面接收窗口，`down:poll` ready 后发送 `{"tango":"win","win":4194304}`，Go 对页面的发送窗口升至 4 MiB；runtime-js 读侧累计四分之一窗口即 ack。4 MiB 是在 2/4/8 MiB 交替测量中吞吐与延迟的折中：在低负载重跑的中位数中 4 MiB 的 64 KiB 下行 1458.89 MB/s，小帧 740741 帧/s；8 MiB 为 1369.57 MB/s、689655 帧/s，2 MiB 为 1100.15 MB/s、606061 帧/s。大帧仍未达到 Wails，因此不打开默认开关。

原始结果文件：`/Users/admin/min/ipc-bench/results/flow-sweep-20261115-133643.jsonl`（首轮含 2/4/8 MiB 和 ack 组合）、`flow-win2-20261115-145158.jsonl`（每个配置开始前等 1 分钟负载 < 5；机器背景负载随后波动）。

| 配置（5 次中位数） | 64 KiB MB/s | 小帧/s | JSON/s | up MB/s | latency mean ms | batch p99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| B：当前 head，1 MiB/半窗 | 1048.58 | 487805 | 117647 | 828.50 | 0.135 | 0.440 |
| 新：4 MiB/四分之一窗 | 1458.89 | 740741 | 112360 | 818.40 | 0.127 | 0.460 |
| Wails v3.1.0 | 1766.02 | 625000 | 123457 | 135.85 | 0.323 | 0.480 |

比较基准文档中的 Wails 目标是 1766 MB/s、625000 小帧/s、约 111000 JSON/s；本轮新配置超过小帧目标，JSON 接近并高于旧记录的 111000，但 64 KiB 大帧仍落后约 17%。上行相对 B 略低，且受机器负载影响；不能宣称通过完整 gate。`downPollDefault` 保持 `false`。

内存边界：Go 每流在途发送信用由页面通告窗口限制为 4 MiB；runtime-js 的 `sendWindow` 同样由 Go 的 `win` 限制，页面接收队列只保留未读帧并在读空时立即归还。`MaxPeerWindowBytes` 将恶意/错误页面窗口限制到 64 MiB，缺省/非法回落 1 MiB；`downq` 单轮仍受 4 MiB/4096 条上限。未保留临时计时插桩；协议上可证明页面每流未读 charged bytes 不超过通告窗口（另加当前包的单帧/解码对象开销）。

兼容性：旧 runtime-js 不带 `win`，Go 回落 1 MiB；旧 Go 忽略 hello 的 `win` 和未知 `win` 消息，页面仍按 1 MiB 工作；非法、过小和超大窗口分别有 Go 测试。`go test -race`、runtime-js 173 tests、contract check、三平台 vet 均通过；WKWebView e2e 结果见本轮报告。

本轮专项验证：`go test -count=1 -race . ./ipc ./internal/downq ./internal/lane`、runtime-js 173 项测试、`npm run contract:check`、Node shim 测试和三平台 `go vet` 通过。完整 `go test ./...` 仍报告已有 `jsapi/TestWireContract`，并报告 `runtime-js/runtimejs/TestEndToEndWithNode`；本轮 hello wire contract 变化需重新生成 contract，运行记录中尚未执行 `contract:update`，Node e2e 也需要单独复核。WKWebView 选择性 e2e 中 `PollAdvertised`、`EvalOrderedWithSend`、`PollBurstNoLoss` 通过（后者 5 次均 1024 帧/64 MiB 无损）；`EvalOrderedWithSend` 的轮询请求计数断言提示已有计数差异。`smokeext` 16/16 通过。

（合并 `feat/down-poll` 到 main 时补记：上面两项已解决。hello 的 `win` 不在 wire contract（`api/jsapi-methods.txt`）里，不需要 `contract:update`，`npm run contract:check` 与 `jsapi/TestWireContract` 通过；`runtime-js/runtimejs/TestEndToEndWithNode` 已改为按 JSON 比较带 `win` 的 hello，Node e2e 通过。）
