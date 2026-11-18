# 兼容优先的应用体积优化（2026-11-15～16）

本轮不删能力、不改公开 API、不用 UPX，不改变系统依赖及最低版本。

## 改动与适用范围

1. **自绘全局浮层按首次使用安装**：确认框、右键菜单、消息、通知不再通过包初始化无条件挂到全局渲染器列表，而由对应 API 自动安装到窗口的 Context。固定绘制顺序不变，同帧打开、关闭/动画后续帧、主题/语言/RTL 作用域与窗口隔离保留。未使用某项功能的应用可由 Go 链接器自然裁掉该实现；使用它的应用仍自动获得完整功能，不需要新增注册代码。
2. **Linux tar.gz 采用 gzip.BestCompression**：归档格式、文件内容、路径和权限不变；减少下载字节，不减少解压后的有效载荷。deb/rpm 本来已用最高 gzip 级别，本轮不变；macOS 仍是 ULMO/LZMA、Windows 仍是 NSIS solid LZMA。
3. **测量脚本对齐实际发布参数**：`scripts/size-report.sh` 改用 `-trimpath -ldflags='-s -w'`，不再额外清空 build ID。实测清空 build ID 未减少本次 macOS 样本文件长度，不计为优化收益。

## 测量条件

- 基线 `b42c75b`；改后代码 `5add4b4`（含 `58d1a1b`），Go 1.27.2，macOS arm64 主机。
- 四样本：最小 WebView（一个加载外部 URL 的窗口）、最小自绘（一个 Button）、`tan init -template vanilla`、`tan init -template ui`。模板的默认前端、绑定、图标保留；两边使用完全相同的样本源文件与配置。
- 实际调用 `tan package`，目标 macOS arm64、Windows amd64、Linux amd64（`-format tar`）。发布参数为 `-trimpath -s -w`、`CGO_ENABLED=0`，含 tan 的发布标签，Windows 另有 `-H windowsgui`。
- 以字节精确记录，1 MiB = 1,048,576 B。下表不是旧版最小样本宣传数值的重用；源码、完整数据与口径见 [JSON](package-size-optimization-data.json)。

## 应用有效载荷（与安装后体积相关）

macOS 统计完整 `.app` 的逻辑文件长度之和（含图标和 ad-hoc 签名）；Linux 统计 tar 内全部普通文件（程序、图标、desktop、安装脚本）。Windows 统计安装器携带的应用主程序，不含安装时生成的卸载器、注册表和系统运行时。不是磁盘分配块、运行内存或系统依赖大小；Windows/Linux 本轮没有实际安装验收。

| 样本 | 目标 | 改前 B | 改后 B | 减少 B | 减少比例 |
|---|---|---:|---:|---:|---:|
| 最小 WebView | darwin-arm64 | 5,933,146 | 5,933,146 | 0 | 0.00% |
| 最小 WebView | windows-amd64 | 5,919,744 | 5,919,744 | 0 | 0.00% |
| 最小 WebView | linux-amd64 | 6,050,645 | 6,050,645 | 0 | 0.00% |
| 最小自绘 | darwin-arm64 | 10,525,978 | 10,140,106 | 385,872 | 3.67% |
| 最小自绘 | windows-amd64 | 10,964,480 | 10,535,936 | 428,544 | 3.91% |
| 最小自绘 | linux-amd64 | 11,268,949 | 10,855,253 | 413,696 | 3.67% |
| 默认 WebView 模板 | darwin-arm64 | 8,525,222 | 8,525,222 | 0 | 0.00% |
| 默认 WebView 模板 | windows-amd64 | 8,693,760 | 8,693,760 | 0 | 0.00% |
| 默认 WebView 模板 | linux-amd64 | 8,601,214 | 8,601,214 | 0 | 0.00% |
| 默认自绘模板 | darwin-arm64 | 11,712,959 | 11,645,823 | 67,136 | 0.57% |
| 默认自绘模板 | windows-amd64 | 12,368,896 | 12,297,216 | 71,680 | 0.58% |
| 默认自绘模板 | linux-amd64 | 12,426,969 | 12,357,337 | 69,632 | 0.56% |

## 真正生成的下载包

| 样本 | 格式/目标 | 改前 B | 改后 B | 减少 B | 减少比例 |
|---|---|---:|---:|---:|---:|
| 最小 WebView | darwin-arm64 | 1,892,120 | 1,892,888 | -768 | -0.04% |
| 最小 WebView | windows-amd64 | 2,079,808 | 2,081,106 | -1,298 | -0.06% |
| 最小 WebView | linux-amd64 | 2,667,172 | 2,598,303 | 68,869 | 2.58% |
| 最小自绘 | darwin-arm64 | 3,155,822 | 3,040,164 | 115,658 | 3.66% |
| 最小自绘 | windows-amd64 | 3,526,857 | 3,389,415 | 137,442 | 3.90% |
| 最小自绘 | linux-amd64 | 4,749,565 | 4,449,081 | 300,484 | 6.33% |
| 默认 WebView 模板 | darwin-arm64 | 2,731,199 | 2,732,247 | -1,048 | -0.04% |
| 默认 WebView 模板 | windows-amd64 | 2,860,157 | 2,859,507 | 650 | 0.02% |
| 默认 WebView 模板 | linux-amd64 | 3,676,961 | 3,581,416 | 95,545 | 2.60% |
| 默认自绘模板 | darwin-arm64 | 3,602,339 | 3,627,903 | -25,564 | -0.71% |
| 默认自绘模板 | windows-amd64 | 3,989,954 | 3,965,765 | 24,189 | 0.61% |
| 默认自绘模板 | linux-amd64 | 5,299,478 | 5,127,906 | 171,572 | 3.24% |

**不要把表格解读成每种安装包都变小：** 默认自绘模板的 dmg 稳定增大约 25 KiB（0.71%），虽然 `.app` 少约 66 KiB。两侧各追加两次打包得到相同方向与几乎相同的字节数（基线 3,602,339 / 3,602,331；改后 3,627,903 / 3,627,895）。这是该产物的压缩结果，未用重复打包挑选更小值。保留这项优化的理由是最小自绘显著下降、所有自绘应用有效载荷下降、Windows/Linux 下载包下降；不是声称每个 dmg 都下降。WebView 的 dmg/NSIS 小幅变化也不计为程序本体优化。

## 验证

- UI 全包通过；全局层、作用域、对话框、消息、通知、菜单相关测试及 `-race` 通过。
- 新增回归：首次使用即安装、固定层序、重复安装无重复渲染、Context 隔离、前层渲染中打开后层仍当帧渲染。
- CLI 全包通过；tar 回归核对原字节、成员路径、可执行权限并与默认 gzip 对照。
- `TANGO_SMOKE_TIMEOUT_SCALE=3 go test -short -p 1 -timeout 40m ./...` 通过。完整组件文字对比度扫描不属于本次短模式验证。
- macOS 真实窗口通过 automation 点击 Message / Notify / Confirm / Menu，确认框关闭和菜单项选择成功，查看了确认框截图；不是只构建或只打开空窗口。
- 所有生成的 dmg 均通过 `hdiutil verify`。Windows 安装器交叉构建成功，未在 Windows 上安装；Linux tar 已验证归档内容，未做 Linux 安装/真实窗口验收。本轮不声称 AppImage、deb/rpm、移动端得到同等收益。
- API 检查发现基线已存在但快照遗漏的 `ui/text.ShapeInto`（来自 `38d0c1b`），只补记已有符号；本轮优化未新增、移除或改签名。

## 复现

对基线与改后各建立源码快照；使用相同 Go、NSIS、macOS 工具版本，固定应用名、图标及样本源文件：

```sh
# 在各源码快照内（go.work 已连接根模块与 cmd/tan）
go build -o /tmp/tan-size ./cmd/tan
/tmp/tan-size init -yes -template vanilla -mobile=false -name SizeWeb -local "$PWD" /tmp/size-web
/tmp/tan-size init -yes -template ui -mobile=false -name SizeUI -local "$PWD" /tmp/size-ui
# 对应最小样本源码见 JSON 的 minimal_sources；配置无 frontend 和 icon。
/tmp/tan-size package -C /tmp/size-ui -platform darwin/arm64,windows/amd64 -o /tmp/size-packages
/tmp/tan-size package -C /tmp/size-ui -platform linux/amd64 -format tar -o /tmp/size-packages
```

两阶段应用目录要分别创建或调整 `go.mod` 的本地 replace，确保它指向被测源码而非另一阶段。不要把安装器生成目录中的 installer.nsi 等构建辅助文件计入安装后有效载荷，也不要将不同 Go 版本/提交/样本口径混为同一组数据。

## 2026-11-16 backend profile self-check

本轮增加了 Darwin 的 `all`、`webview`、`canvas`、`webview,canvas` 编译 profile 和确定性的能力边界：不包含某模式的 profile 在调用对应入口时返回 `ErrUnsupported`，不会创建空白窗口。Windows/Linux 的非 `all` profile 本阶段在 `tan build/package` 的目标校验阶段明确拒绝。

| 样本 | profile | raw B | gzip-9 B |
|---|---|---:|---:|
| WebView | all | 5,946,402 | 2,466,798 |
| WebView | webview | 5,946,386 | 2,465,696 |
| WebView | canvas | 5,946,402 | 2,466,641 |
| WebView | webview,canvas | 5,946,402 | 2,466,833 |
| Canvas | all | 10,178,034 | 4,036,805 |
| Canvas | webview | 10,177,986 | 4,036,448 |
| Canvas | canvas | 10,178,034 | 4,036,856 |
| Canvas | webview,canvas | 10,178,034 | 4,037,016 |
| Mixed | all | 10,178,034 | 4,036,768 |
| Mixed | webview | 10,177,986 | 4,036,420 |
| Mixed | canvas | 10,178,034 | 4,036,732 |
| Mixed | webview,canvas | 10,178,034 | 4,036,900 |

结论：Go 链接器已经自动裁掉了最小样本中没有被调用的多数后端代码。`webview` profile 只减少 16–48 B，canvas/mixed 没有安装后体积收益；因此本轮不把 backend profile 宣传成瘦身方案，也不做高风险的 Darwin 全文件拆分。profile 保留为后续平台拆分和能力校验的实验性基础。

## 2026-11-18 macOS 可选 CoreText 文字引擎（`-tags tango_coretext`）

带构建标签 `tango_coretext` 时，macOS 的自绘文字（整形、断行、双向文字、字体回退、光栅化）全部交给 CoreText，不链接 `github.com/go-text/typesetting`；默认构建仍是 go-text。下表的“改后”是带该标签的构建（当时还是默认构建，代码相同）。测量条件：Go 1.27.3，darwin/arm64，`CGO_ENABLED=0 -trimpath -ldflags='-s -w'`；改前 origin/main `9f0d478`，改后 `feat/coretext-engine` `c2b3493`。最小自绘样本即 `scripts/size-report.sh` 的 `ui` 应用（`uihost.Open` + 一个 `ui.Button`）；模板样本由 `tan init -yes -template ui -local <checkout>` 生成，`.app`/`.dmg` 由同一棵源码树构建的 `tan package -platform darwin/arm64` 生成。

| 样本 | 改前 B | 改后 B | 差值 B | 变化 |
|---|---:|---:|---:|---:|
| 最小自绘样本，可执行文件 | 10,520,354 | 8,544,834 | -1,975,520 | -18.8% |
| 最小自绘样本，gzip -9 | 4,168,007 | 3,459,328 | -708,679 | -17.0% |
| `tan init -template ui`，可执行文件 | 11,975,746 | 10,000,386 | -1,975,360 | -16.5% |
| `tan init -template ui`，gzip -9 | 4,807,190 | 4,098,485 | -708,705 | -14.7% |
| `tan init -template ui`，`.app` 内全部文件 | 12,100,962 | 10,137,090 | -1,963,872 | -16.2% |
| `tan init -template ui`，`.dmg` | 3,720,203 | 3,198,304 | -521,899 | -14.0% |
| 6 路基准 Tango counter（`outputs/tango-6way-full-20261117/bench`，`-buildvcs=false`） | 10,553,698 | 8,578,226 | -1,975,472 | -18.7% |

按包汇总代码量（pclntab，最小样本）：总代码 4.25 MB → 3.48 MB。改前 go-text 各包合计约 732 KB（`font/opentype/tables` 340 KB、`harfbuzz` 242 KB、`font` 72 KB、`font/cff` 28 KB、`font/cff/interpreter` 12 KB、`bidi` 12 KB、`font/opentype` 9 KB、`segmenter` 7 KB、`shaping` 7 KB、`internal/unicodedata` 3 KB），另有只被它用到的 `golang.org/x/image/tiff`、`ccitt`、`tiff/lzw` 约 27 KB；改后两个样本的包列表和 `go version -m` 的依赖里都没有 `github.com/go-text/`，`ui/text` 自身从 101 KB 增至 105 KB。段大小：`__text` 4,248,868 → 3,475,556 B，`__gopclntab` 3,493,870 → 2,836,637 B，`__noptrdata` 405,025 → 296,129 B。`scripts/check-gotext-free.sh` 在整个模块的 `go list -tags tango_coretext -deps ./...`（darwin/arm64）上确认没有 go-text。

同批性能测量（`outputs/coretext-bench-20261118/`，优化前，超出验收门槛）：

- `ui/text` 行缓存基准（各 5 次取中位数，改前 → 改后）：`LineCacheMono200/hit` 53 → 42 ns，`hit-tokens` 276 → 281 ns，`miss-fast` 4.7 → 7.1 µs（+53%），`miss-fast-tokens` 8.7 → 11.7 µs（+35%），`miss-general` 26 → 97 µs（+267%），`miss-general-tokens` 35 → 359 µs（+914%）；`LineCacheMixed200/miss-fast` 14 → 780 µs，`miss-general` 36 → 476 µs。门槛为耗时增幅不超过 20%。
- 6 路基准的 Tango 程序，5 轮交错（改前 → 改后，中位数）：进程 CPU counter 0.1% → 0%、quotes 15.8% → 37.4%（+21.6 个百分点）、list 8.6% → 12.4%（+3.8）、grid 19.2% → 19.1%；内存 counter 32.3 → 27.6 MB、quotes 146.9 → 137.2 MB、list 133.4 → 127.0 MB、grid 146.9 → 138.5 MB；首帧 254/291/268/293 ms → 218/265/236/231 ms；帧间隔 P99 16.76–16.89 ms，两边相同。门槛为 CPU 增幅不超过 2 个百分点，quotes 和 list 超出。

之后的优化（`outputs/coretext-perf-20261118/`，nice 19 的相对数字）：能放下的标签不再求断行机会、纯左到右文本不再建 bidi CTLine、短标签进 prepared 缓存、数字快路径覆盖 `%` `,` 和放得下的换行标签。新增的行情帧基准 `BenchmarkCTQuotesFrame` 从 4.27 ms 降到 0.20 ms（go-text 0.60 ms）。行缓存未命中仍慢：`LineCacheMono200/miss-general` 约 88–107 µs、`miss-general-tokens` 约 350 µs、`LineCacheMixed200` 约 0.35–0.9 ms，是 go-text 的 3–12 倍；单是一条 200 字符 CTLine 的创建就超过 go-text 整条路径，无法在保持精确的前提下缓存掉。因此按用户的规则（时限内未达门槛走方案 2）CoreText 引擎改为可选构建标签，默认构建保持 go-text。
