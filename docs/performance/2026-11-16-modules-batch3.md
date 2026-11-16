# 第三批模块性能优化（2026-11-16）

## 范围与证据

基于第二批提交 `902c52d7383bda0275d4b4baaad08cf1ec190452`，在 `perf/all-modules-20261116` 分支继续优化 `bind` 和 `asset`，同时审计 `ipc`。第一批已合并本地 main（`e662739`）；第二批及本批仍在优化分支，未推送远程。

环境：Go 1.27.2、macOS 26.6 / arm64、Apple M4。原始证据在 `/Users/admin/min/outputs/tango-perf-batch3-20261116/`，包括旧源码、CPU/分配 profile、失败测试、固定测试二进制、命令/负载/哈希清单和完整测试日志。

## 定位与取舍

- `bind` 的分配 profile 显示参数解码、RawMessage 切片扩容和反射调用是主要来源。按注册时已知参数个数预留 RawMessage 容量，Context 和已解码参数一次构造；合法的精确 `[]`/`null` 无必需参数调用在调用栈内处理。
- 不借用超出自定义 JSON 解码回调生命周期的输入，也不自行解析 JSON。其他输入继续由标准库完整解码，保留语法验证、参数数量检查、逐参数解码的顺序及错误信息。
- `asset` 的路径清理原来为绝对路径重复添加根斜杠，并为穿越检查创建 Split 切片。现在顺序遍历分段，已有根斜杠时直接 Clean。穿越检查仍在 Clean 前，保留 `fs.ValidPath`。
- 资源响应 profile 的大部分分配字节来自 `httptest.ResponseRecorder` 的输出捕获和拷贝，不能把它们称作生产文件读取热点。本批没有更改文件读取、缓存、ServeContent 或响应接收器。
- IPC AppendPacket 的复用缓冲场景已零分配；ParsePacket 每次创建独立帧描述切片（单帧约 48 B、1 次），载荷仍引用输入。本批保留接口和源码，不以新缓存或改变所有权强行移除返回结果的分配。

## 已确认的分配变化

固定二进制，前后版本交替运行，各 5 轮。分配预算回归在普通构建中验证；B/op 为五轮中位数，表示分配流量，不是 RSS。

| 场景 | 原始 B/op | 优化后 B/op | 分配次数 前→后 |
| --- | ---: | ---: | ---: |
| BoundCall/add | 248 | 224 | 11 → 10 |
| BoundCall/context | 80 | 56 | 4 → 3 |
| BoundArguments/context-value | 184 | 160 | 9 → 8 |
| BoundCall/variadic | 552 | 528 | 19 → 18 |
| BoundArguments/context-whitespace | 80 | 80 | 4 → 4 |
| BoundArguments/nested | 320 | 320 | 11 → 11 |
| BoundArguments/large-string | 约 139,450 | 约 139,450 | 8 → 8 |
| AssetPath /app.js | 56 | 0 | 4 → 0 |
| AssetPath /assets/icons/normal/file.svg | 176 | 0 | 4 → 0 |
| AssetPath assets/icons/normal/file.svg | 96 | 32 | 2 → 1 |
| AssetPath /a//b/./c | 128 | 16 | 4 → 2 |
| EmbeddedAssetResponse | 132,281 | 132,223 | 22 → 18 |

最初 20 次测量全部因前后 1 分钟负载未满足 `<6` 门槛而排除。完整测试结束后另做有界复测，并最多追加每模块 5 对：共计 60 次测量、13 次单独合格，按前后成对要求仅取得绑定 4 对、资源 2 对，均未达到预定 5 对。故不报告速度提升百分比，不把不同有效轮次的前后数据拼成完整对照。数据见 `paired.json`、`retry-paired.json`、`topup-paired.json` 和 `final-eligibility.json`；`timing-summary.json` 保持为空。分配表来自首轮各5次记录（`allocation-summary.json`），另有普通构建的确定性分配回归。分配改进不等于所有绑定/资源场景已变快，也不是 MyGo 等价场景比较。

## 正确性与验证

- 分配预算和路径零分配测试先在旧代码实际失败，见 `bind-red.txt`、`asset-red.txt`。优化后的普通构建回归通过。
- 绑定生成 1,000 组参数并补充手工非法输入，对 11 种签名逐一比较旧实现的返回值、错误和实际服务调用次数；覆盖 Context、变参、null、Unicode、自定义解码、RawMessage、slice/map 和非法 JSON。额外验证跨调用返回结果所有权及 16 个并发调用者的 Context/参数隔离。
- 资源路径对照 6,000 组输入，覆盖绝对/相对路径、重复斜杠、点段、穿越、Unicode/非法 UTF-8、NUL 和反斜杠；HTTP 验证 HEAD、Range、If-Modified-Since、If-Range、SPA、配置头和动态文件更新。
- 初次优化暴露 Context-only 以及带空白空参数的额外分配，分别通过逃逸/对象 profile 定位并修正，没有放宽普通构建预算。`-race` 会给反射调用增加分配，旧实现 add/Context-value 也分别从 11/9 增至 12/10；因此分配预算文件仅使用 `!race`，功能/所有权/并发测试继续全部运行 race。
- `go test -p 1 ./bind ./asset ./ipc -count=1 -timeout=3m`、同三包 `go test -race` 和 `go vet` 已通过，日志为 `modules-*-final.txt`。独立只读静态审查未发现 Critical/Important/Minor 问题。
- 完整 `go test -p 1 ./... ./cmd/tan/... -count=1 -timeout=10m` 通过（546.74 秒），输出与当前 171 个包逐一核对一致；全项目/CLI `go vet`、格式、diff check 通过。验证后源码哈希与测试时保持一致，见 `source-manifest.json`。

## 尚未完成

本批低负载耗时对照尚未取得；绑定的大字符串/嵌套数据解码、资源端到端服务及 IPC 多帧/实际传输仍需进一步定位。没有更改或声称完成文本、渲染、宿主、CLI 或跨设备模块的性能优化。各包状态继续见逐模块覆盖表。
