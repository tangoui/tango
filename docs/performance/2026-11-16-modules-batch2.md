# 第二批模块性能优化（2026-11-16）

## 集成状态与范围

第一批已合并到本地 `main`：`e66273903caf4ddcd82096ded49ce7d88e8202d0`。合并提交上的完整 Go/CLI 测试通过，耗时 606.9 秒。未推送远程。第二批基于该提交，在 `perf/all-modules-20261116` 分支优化 `i18n` 和 `ui/editor/buffer`，不代表其余所有模块已完成。

证据目录：`/Users/admin/min/outputs/tango-perf-next-20261116/`。包含原始日志、旧实现源码、固定测试二进制、CPU/分配 profile、失败回归日志和测量清单。环境为 Go 1.27.2、Darwin arm64、Apple M4。

## 测量方法与结果

前后版本各使用固定二进制，每轮启动新进程，交替先后顺序，各 5 轮。语言匹配和换行归一化每轮 300 ms；100MB 快照每轮 5 次。最初 30 次测量的前后 1 分钟负载均未满足 `<6` 门槛，全部排除；之后另做 30 次测量，全部满足门槛（约 4.09–4.67）。日志分别保留，未覆盖旧证据。

有效数据为 `lowload-paired.json`、`lowload-*.txt`，其中记录每次命令、退出码、二进制 SHA256 和前后负载；汇总为 `lowload-summary.json`。下表为每轮平均 ns/op 的跨轮中位数，不是请求延迟分位数。仅比较 Tango 自身前后版本，不能用于宣称比 MyGo 领先。

| 场景 | 原始 ns/op | 优化后 ns/op | 耗时下降 | 分配流量/次数 |
| --- | ---: | ---: | ---: | --- |
| Canonical/en | 26.98 | 17.83 | 33.9% | 16 → 0 B；1 → 0 次 |
| Canonical/zh-Hant-HK | 99.23 | 43.29 | 56.4% | 68 → 0 B；3 → 0 次 |
| Canonical/sr-Latn-RS | 100.10 | 39.46 | 60.6% | 68 → 0 B；3 → 0 次 |
| LocaleMatch | 759.70 | 394.20 | 48.1% | 512 → 0 B；23 → 0 次 |
| NormalizeEOL/crlf | 1,411,461 | 1,266,371 | 10.3% | 约 1.11 MB、1 次，基本不变 |
| NormalizeEOL/mixed | 3,309,492 | 2,718,963 | 17.8% | 约 2.23 → 1.11 MB；2 → 1 次 |
| NewSnapshot100MBCRLF | 42,961,242 | 28,683,725 | 33.2% | 约 111.39 MB，未宣称改善 |

POSIX 输入 `zh_hant_hk.UTF-8` 的耗时中位数 127.1 → 121.8 ns/op，仍为 88 B/op、5 allocs/op，不作为明确收益。无 CR 的归一化和 100MB LF 快照分别相差约 -1.9% 和 +0.5%，不作为收益或回归结论。大文件快照每轮只有 5 次，分配次数会受运行时活动影响（前后均有 5–7 次的记录）；本批仅对独立归一化的单次输出分配和语言匹配零分配建立确定性回归。

## 实现与语义保护

- `Canonical` 对已经满足原有大小写规则的 ASCII 标签返回原字符串；Unicode、POSIX 名称、下划线和重复分隔符继续走原有路径。仍不额外验证标签是否合法。`parseTag` 顺序遍历分段，消除 Split 临时切片；没有增加全局缓存或锁。
- 换行归一化按已知 CRLF 数量预留一次输出空间，逐段复制并统一 CRLF/孤立 CR。`NewSnapshot` 复用原有换行统计；EOL 投票规则、不可变快照、编辑偏移和 UTF-8 行为保持不变。
- 语言匹配旧实现 oracle 覆盖 8,000 组生成输入，每组同时检查精确匹配、无精确候选的脚本/区域选择、未知语言、偏好回退和空候选，比较 `Canonical`、`Match` 和 `MatchOK`。
- 换行对照覆盖 4,000 组输入，包括连续/尾部 CR、CRLF、LF、中文、emoji、NUL 和非法 UTF-8 字节；另外保留现有 EOL 往返、Apply 和快照测试。

原始 profile 中，语言匹配的分配对象主要来自 FieldsFunc（47.35%）、Split（26.63%）和 Join/Builder（20.68%）；换行 profile 的分配字节约 98.2% 来自旧归一化输出构造。记录见 `i18n-alloc-top.txt`、`buffer-alloc-space.txt`；换行的对象数量 profile 受测试初始化影响，未据此定位业务对象数量。

## 验证与审查

优化前的零分配/单次输出回归均实际失败，日志为 `i18n-red.txt` 和 `normalize-red.txt`。优化后模块完整测试、`go test -race -p 1 ./i18n ./ui/editor/buffer -count=1` 和同两包 `go vet` 通过。独立静态审查未发现 Critical/Important 问题；已补充审查指出的无精确候选匹配覆盖。

第二批完整 `go test -p 1 ./... ./cmd/tan/... -count=1 -timeout=10m` 通过（563.12 秒），结果见 `batch2-full-tests.txt`、`batch2-full-tests.json`。全项目含 CLI 的 `go vet -p 1 ./... ./cmd/tan/...`、改动文件 gofmt 和 diff check 也通过。相关源码和测试 SHA256 见 `batch2-source-manifest.json`；已重新核对合并后全部 171 个包均列入覆盖表。

## 后续覆盖

资源 HTTP、反射绑定、IPC 和 Shader 前端已有基线，尚未完成定点优化。继续时需要先补充多参数/嵌套 JSON、批量帧、资源 HEAD/Range/条件响应等代表场景并采样，不能只因存在分配就改用共享可变缓冲。文本/GPU/宿主和各设备路径仍按逐模块覆盖表跟踪。
