# 参与贡献

感谢你愿意改进 Tango！本文说明怎样搭建开发环境、跑测试、提交改动。英文简介见文末。

## 开始之前

- 小的修复（错别字、明显的 bug、文档）直接提 PR。
- 新功能、公开 API 的变化、新增依赖，请先开一个 issue（「功能请求」模板）讨论设计，避免白做。
- 安全问题**不要**公开提 issue，见 [SECURITY.md](SECURITY.md)。
- 参与本项目即表示你同意遵守 [行为准则](CODE_OF_CONDUCT.md)。

## 开发环境

| 需要 | 用途 |
|---|---|
| Go 1.27+ | 全部 Go 代码。仓库根目录的 `go.work` 把根模块和 `cmd/tan` 连在一起 |
| Node.js 与 npm | `runtime-js`（前端运行时）和 `site`（文档站） |
| macOS：Xcode 命令行工具 | 签名、打包；iOS 需要完整 Xcode |
| Linux：`libgtk-3-dev libwebkit2gtk-4.1-dev libayatana-appindicator3-dev` | 运行 Linux 后端（编译本身不需要 cgo） |
| Windows：WebView2 运行时 | 运行 Windows 后端 |
| Android（可选）：JDK 17、SDK、NDK | 移动端 |

```bash
git clone https://github.com/tangoui/tango && cd tango
go build ./... && (cd cmd/tan && go build ./...)
go run ./examples/hello          # 打开一个窗口
go run ./examples/showcase       # 演示应用
(cd cmd/tan && go install .)     # 安装本地的 tan
```

仓库结构：

| 目录 | 内容 |
|---|---|
| 根目录 | 核心运行时（`App`、`Surface`、线程模型、应用事件） |
| `ipc`、`bind`、`asset`、`jsapi` | IPC、反射绑定、资源服务、页面可用的系统能力 |
| `sys/` | 系统能力包，一个能力一个包 |
| `service/` | 服务生命周期、目录、存储、日志、文件服务、自动更新 |
| `ui/`、`uihost` | 自绘 UI 与它的窗口宿主 |
| `internal/platform` | 后端契约与各平台实现（darwin、windows、linux、ios、android、fake） |
| `cmd/tan` | 命令行工具（独立的 Go 模块） |
| `runtime-js` | `@tangoui/runtime` 的 TypeScript 源码；`runtimejs/` 是嵌入 Go 的构建产物 |
| `mobile/` | iOS、Android 原生宿主 |
| `examples/` | 示例，多数带 `-smoke` 自检模式 |
| `docs/` | 设计文档；`site/` 是文档站 |

## 测试与冒烟

提交前至少跑：

```bash
gofmt -l .                                   # 应该没有输出
go vet ./... && go vet -tags tango_dev ./... && (cd cmd/tan && go vet ./...)
go test ./... && (cd cmd/tan && go test ./...)   # 默认构建 = 生产构建
go test -tags tango_dev ./...                    # 开发构建：热加载、自动化、开发代理等
# 改了平台相关代码：三个桌面平台、两种标签都要能编过
for os in darwin windows linux; do for tags in "" tango_dev; do CGO_ENABLED=0 GOOS=$os go vet -tags "$tags" ./...; done; done
# 改了 runtime-js：
(cd runtime-js && npm ci && npm test)        # 并提交更新后的 runtimejs/tango.iife.js
```

**真实窗口冒烟**：改了后端、窗口、IPC 或系统能力时运行

```bash
bash scripts/smoke.sh                        # 本平台全部桌面冒烟；Windows 用 pwsh scripts/smoke.ps1
bash scripts/smoke.sh desktop hello          # 只跑一个
go run ./examples/showcase -smoke
```

开发功能（`tan dev` 热加载、`automation` 通道、`asset.FromEnv` 的开发代理、panic 不退出等）只在 `-tags tango_dev` 构建里存在，默认构建（普通 `go build`/`go test`）就是生产构建；依赖开发功能的测试文件要带 `//go:build tango_dev`，验证生产行为的带 `//go:build !tango_dev`。`tango_release` 已无作用。详见 [docs/dev-builds.md](docs/dev-builds.md)。端到端测试：`TANGO_E2E=1 go test -tags tango_dev -run E2E ./examples/ledger ./examples/hello`。

CI 会在 macOS、Windows、Linux 上跑同样的命令，详见 [docs/ci.md](docs/ci.md)。你没有的平台可以交给 CI，但请在 PR 里说明哪些平台是你亲自验证过的。

**自绘 UI**：先读 [ui/AUTHORING.md](ui/AUTHORING.md)；`go test ./ui/...`，必要时 `go run ./examples/ui-gallery -only <组件>` 生成截图对照。

**文档站**：`cd site && npm install && npm run docs:build && npm run docs:links`。

## 代码风格

- Go：`gofmt`；导出标识符都要有注释，注释用英文、以名字开头（`go doc` 和 API 参考由它生成）。
- 能力包遵循统一约定：一个能力一个包，第一个参数是 `*tango.App`，平台不支持时返回 `ErrUnsupported`，回调在主线程。
- 公开 API 的改动要同步更新 `docs/` 与 `site/` 里对应的页面，用户可见的变化写进 [CHANGELOG.md](CHANGELOG.md) 的「未发布」一节。
- 新依赖需要先讨论；桌面后端保持不使用 cgo。
- 平台契约（`internal/platform` 的 `backend.go`、`types.go`、`caps.go`、`ext.go` 等）的变化需要在 issue 里先说明。
- 测试优先用 `internal/platform/fake` 后端写单元测试；需要真实窗口的再写冒烟程序（能自动退出、用退出码表示结果、默认不等待人操作）。

## 不复制他人代码

Tango 参考过其他框架的设计思路，但**代码、文档文字、常量表、图标数据都必须是自己写的**：

- 不要从其他项目（包括 MIT / Apache 等宽松许可的项目）复制代码或文档段落，也不要「改写式」地逐段翻译。
- 确实需要引入第三方代码时，先在 issue 里说明来源和许可证；经同意后保留原许可证声明，放在单独的文件或目录里，并在 PR 描述中列出。
- 设计令牌、色值等按公开的设计规范文档自行实现。
- 用 AI 工具辅助时同样适用：你要对提交内容的来源负责。

## 提交规范

提交信息用「范围: 摘要」的形式，范围是主要改动的包或目录，摘要用祈使语气、简短（中英文均可）：

```
ipc: cancel streams when the connection drops
sys/tray: 支持 Linux 上的菜单更新
docs: 补充移动端签名说明
```

正文说明「为什么」，必要时附上验证方式（跑了哪些测试、在哪些平台上验证）。一个 PR 尽量只做一件事。

### DCO（开发者原创声明）

本项目使用 [Developer Certificate of Origin 1.1](https://developercertificate.org/)，不需要签 CLA。每个提交都要带 `Signed-off-by` 行，表示你有权以本项目的许可证（MIT）提交这些内容：

```bash
git commit -s -m "ipc: cancel streams when the connection drops"
# 提交信息末尾会加上：
# Signed-off-by: 你的名字 <you@example.com>
```

名字和邮箱要与提交作者一致。忘了加可以用 `git commit --amend -s`（多个提交用 `git rebase --signoff main`）补上。

## PR 流程

1. Fork 并从 `main` 建分支。
2. 改代码、加测试、更新文档和 CHANGELOG。
3. 确认上面的检查通过，提交时带 `-s`。
4. 开 PR，按模板填写：改了什么、为什么、怎么验证的、在哪些平台上验证过。
5. 维护者审阅后合并；CI 必须通过。

---

## English summary

Thanks for contributing! Discuss new features and public API changes in an issue first; report security problems privately (see [SECURITY.md](SECURITY.md)). Before sending a PR run `gofmt`, `go vet` and `go test` for the root module (both the default production build and `-tags tango_dev`, which enables the development features; see docs/dev-builds.md) and `cmd/tan` (plus cross-platform `go vet` for platform code and `npm test` in `runtime-js` when you touch it), and the real-window smoke tests (`bash scripts/smoke.sh`) for backend changes. Do not copy code or documentation text from other projects. Commit messages look like `pkg: short summary`, and every commit must be signed off under the [DCO](https://developercertificate.org/) with `git commit -s`.
