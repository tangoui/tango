# 发布流程

> 文档站：[发布 Tango 自己](https://tangoui.github.io/tango/guide/packaging#发布-tango-自己) · 本文是维护者的发布手册。

Tango 一次发布包含三样东西，它们**使用同一个版本号**：

| 产物 | 位置 | 版本标记 |
|---|---|---|
| Go 模块 `github.com/tangoui/tango` | 仓库根目录 | git tag `vX.Y.Z` |
| 命令行 `tan`（独立 Go 模块 `github.com/tangoui/tango/cmd/tan`） | `cmd/tan` | git tag `cmd/tan/vX.Y.Z`；另附各平台二进制 |
| npm 包 `@tangoui/runtime` | `runtime-js` | `runtime-js/package.json` 的 `version` = `X.Y.Z` |

## 版本号规则

遵循 [语义化版本 2.0.0](https://semver.org/lang/zh-CN/)：

- **0.x 阶段**（当前）：`0.MINOR.0` 可以包含不兼容的公开 API 变化，须在 CHANGELOG 的「变更」/「移除」里写明迁移方法；`0.MINOR.PATCH` 只做向后兼容的修复。
- **1.0 之后**：不兼容变化只进 MAJOR；新增功能进 MINOR；修复进 PATCH。
- **预发布**：`vX.Y.Z-rc.N`、`-beta.N`。发布工作流把它们标为 GitHub pre-release，npm 用 `next` 标签发布（不影响 `latest`），不部署文档站。
- **三者同步**：即使某次只改了其中一部分，也一起升到同一个版本，方便用户对应。`tan init` 生成的项目引用同版本的 `@tangoui/runtime`。
- **Go 模块路径**：进入 v2 时模块路径要加 `/v2` 后缀（Go 的规则），届时另行规划。
- `tan version` 的版本号来自构建时的 `-ldflags "-X main.version=X.Y.Z"`；`go install …@vX.Y.Z` 安装的则取模块版本。

## API 稳定性

> 背景与路线：[docs/specs/api-stability.md](specs/api-stability.md)。

### 分层

Go 模块的每个非 internal 包在 [`api/tiers.txt`](../api/tiers.txt) 里有且只有一个层级（`x/...` 覆盖子树；精确路径优先，其次最长的 `/...`）：

| 层 | 含义 | 当前的包 |
|---|---|---|
| `stable` | 不兼容变化默认不允许，走下面的 `api-break` 流程 | 根包 `tango`、`asset`、`automation`、`bind`、`i18n`、`ipc`、`jsapi`、`runtime-js/runtimejs`、`service/...`（含 `service/updater`）、`sys/...`、`ui/a11y`、`ui/geom`、`ui/icon` |
| `split` | 包内分层：[`api/stable/<包>.txt`](../api/stable) 白名单里的符号是稳定层，其余是实验层 | `ui`、`ui/input`、`ui/layout`、`ui/paint`、`ui/text`、`ui/theme`、`uihost` |
| `experimental` | 允许不兼容变化，CI 只警告；仍要在 CHANGELOG 写一句 | `ui/chart`、`ui/mobile`、`ui/editor/...` 及其余 `ui/*` |
| `exclude` | 不是 API | `examples/...`、`site/...` |

- 白名单每行一个符号：顶层名（`Button`；类型包含它的全部字段和方法）或 `类型.成员`（`Label.Color`）。不支持"稳定类型上的实验性方法"。
- **白名单默认不承诺**：新写的符号自动是实验层，升为稳定是一次可审查的 diff。`api/stable/ui.txt` 是 `ui` 的稳定核心（框架、纯容器和约 45 个基础组件，见 [api-stability.md §2.3](specs/api-stability.md)）；`Context`、`Box`、`Runner`、`Driver`、`TableW`、`Locale` 这类只有部分成员是核心的类型，白名单逐个列出稳定成员。
- **稳定符号只能引用稳定符号**：签名、字段、方法里出现的类型必须来自标准库、稳定包或白名单。把一个符号升为稳定时，它引用的符号要一起升级，或者把引用它的成员留在实验层（例如 `ConfigProviderW.Components` 引用 `theme.Components`，所以留在实验层）。
- 参考站（`site/tools/apigen` 生成）读取 `api/tiers.txt` 和白名单，给实验性的包和符号加「实验性」徽标。
- 新增的包必须先在 `api/tiers.txt` 里分层，否则 CI 失败。
- 实验性包、split 包的包文档要写明 "Experimental" 或「实验性」；split 包还要说明只有白名单里的符号受兼容性承诺约束。

### 设计规则

每条都对应 apidiff 实际报出过的破坏。

**R1 枚举只追加，值永不改变，零值语义在创建时定死。** 不插入、不重排；想换默认值就新增 `XxxAuto` 或由 `ConfigProvider` 解析，不改零值。不导出计数常量（`NumComps` 这类每加一项就变值），开放集合用带类型的字符串常量。

```go
type DrawerPlacement uint8

const (
	DrawerRight DrawerPlacement = iota // 0: the zero value, forever
	DrawerLeft
	DrawerEnd // added later: appended, never inserted
)
```

**R2 构造函数从第一天起返回专属类型**，即使它暂时什么都不加；只有纯布局容器（`Row/Column/Div/StackBox/Layout/Header/Content/Footer`）返回 `*Box`。

```go
type TooltipW struct {
	_   [0]func()
	box *Box
}

func Tooltip(c *Context, tip string, fn func()) *TooltipW
```

**R3 不给已有函数加参数，可变参数也不行**（把函数当值用的代码会编译失败，apidiff 也判为不兼容）。组件用链式方法加选项；非组件函数从第一天就收 `XxxOptions`；漏了就新增 `XxxWith`。

```go
func DevProxy(target string) http.Handler                              // v0.1, unchanged
func DevProxyWith(target string, opts DevProxyOptions) http.Handler    // v0.2 adds this
```

**R4 可比较性在创建时决定。** 带未导出字段的导出结构体、会增长的 Options/Locale 结构体，第一个字段写 `_ [0]func()`：一开始就不可比较，以后加 slice/map/func 字段不算破坏；包外也写不了无键字面量。放在第一位不会引入尾部填充。

```go
type SurfaceOptions struct {
	_     [0]func()
	Title string
}
```

**R5 接口。** 不导出给用户实现的接口，除非专门设计过；要封闭就加未导出方法（`input.Event`、`paint.Op`）；扩展能力用可选接口（`BackWindow`、`SafeAreaWindow` 之于 `ui.Window`），不往已有接口加方法。允许用户实现的接口提供可嵌入的基类：

```go
type Window interface {
	Size() geom.Size
	window() // sealed: implementations embed WindowBase
}

type WindowBase struct{}

func (WindowBase) window() {}
```

**R6 先弃用再删除。** 写 `// Deprecated: use X. Removed in v0.(N+1).`，至少保留一个 MINOR 版本。

**R7 零值有意义且永不改变。** 零值 Options 等于默认行为；新字段的零值必须等于旧行为。

**R8 公开 API 不新增指向 internal 类型的别名。** 新类型在公开包里定义，由 internal 转换。存量别名（根包、`sys/*`、`automation`）是"钉住的类型"：黄金文件会展开别名目标的字段和方法，改 internal 类型同样会出现在 API diff 里。

**R9 不导出可变的包级配置变量**（`var MessageDuration = …`），用函数或 Options。

**R10 泛型类型不新增类型参数，不收紧约束。**

**R11 字符串键也是 API。** `ui.Prop(c, "Card", "bordered", def)` 和 `ConfigProvider.Defaults` 的组件名、键名、值类型记录在黄金文件的 `prop` 行里，改名按 R6 处理。

**R12 跨 IPC 的结构体，JSON 标签是线上 API**（`jsapi` 参数与返回值、`devhost` 消息、`updater` 清单）。改字段名等于破坏；黄金文件的字段行带结构体标签。

### 黄金文件 `api/go/*.txt`

每个非 internal、非 main、未排除的包一个文件，文件名是相对模块的包路径把 `/` 换成 `_`（`ui/theme` → `api/go/ui_theme.txt`，根包是 `tango.txt`）。每行一个事实、按字节排序，格式仿照 Go 的 `api/go1.N.txt`：

```text
pkg github.com/tangoui/tango, type AppEvent = internal/platform.AppEvent
pkg github.com/tangoui/tango, type AppEvent struct, Kind AppEventKind
pkg github.com/tangoui/tango/ui, const DrawerRight DrawerPlacement = 1
pkg github.com/tangoui/tango/ui, method (*TooltipW) Placement(Placement) *TooltipW
pkg github.com/tangoui/tango/ui, type AlertW struct, noncomparable
pkg github.com/tangoui/tango/ui, prop "Card" "bordered" bool
pkg github.com/tangoui/tango/<pkg>, func DarwinOnly() [darwin]
```

- 常量带值，枚举移位直接改动行；`noncomparable` 让 R4 可见；只在部分平台存在的行带 `[darwin]` 这样的后缀（合并 linux、darwin、windows，`CGO_ENABLED=0`）。
- 别名指向模块 internal 包的类型时，展开它的字段和方法；经未导出或 internal 嵌入字段提升的字段、方法也列出来。
- 生成器 `internal/cmd/apitxt` 只用标准库（`go list -export` 加 `go/importer`、`go/types`）：

```bash
go run ./internal/cmd/apitxt -w        # 改了公开 API 后重写 api/go，和代码一起提交
go run ./internal/cmd/apitxt -check    # CI：不一致、分层缺失、白名单条目不存在时退出 1
```

`-check` 还检查两类问题，都会失败：实验性包和 split 包的包文档缺少 "Experimental"/「实验性」；稳定符号引用了实验层或未列入白名单的符号（跨层泄漏，`-v` 列出全部）。另外 `internal/apitest/retired_test.go` 守卫已经迁走的旧包路径，旧路径下再出现 `.go` 文件就失败。

并行分支在黄金文件上冲突时，不要手工合并，rebase 后重新运行 `-w`。

### 兼容性检查与 `api-break` 流程

CI 的 `API` 工作流（`.github/workflows/api.yml`）依次运行 `apitxt -check`、`scripts/apidiff.sh`（对比 PR 的基准提交，推送时是推送前的 `main`；linux、darwin、windows 各导出一次）和 CHANGELOG 检查。apidiff 报出的每个不兼容变化按 `api/tiers.txt` 和白名单归层：稳定包里的变化、split 包里白名单符号的变化算**稳定层破坏**，检查失败；其余只警告。本地可以先跑：

```bash
bash scripts/apidiff.sh            # 对比 origin/main（含未提交的改动）
bash scripts/apidiff.sh v0.1.0     # 对比某个标签
```

改稳定层时，按上面的规则用兼容的写法：新增函数、`XxxWith` 或 Options 字段，旧名字保留并标 `// Deprecated:`。确实要破坏兼容时：

1. 给 PR 加 `api-break` 标签（直接推送 `main` 时在提交说明里写 `[api-break]`），检查转为警告；
2. 在 CHANGELOG「未发布」的「变更」或「移除」里写明迁移方法；
3. 这类变化只能进下一个 `0.MINOR.0`（1.0 之后只进 MAJOR）。

apidiff 视为不兼容的情况比"编译不过"更宽：给函数追加可变参数、结构体不再可比较、导出常量的值变化都算。

### CHANGELOG

`api/go/**` 有变化（新增也算，每个新增都是承诺）时，同一个 PR 必须在 CHANGELOG「未发布」里写一句：稳定层的破坏写迁移方法，新增和实验层变化写一句即可。CI 目前只警告，2026-11-21 起改为失败；确实不需要的 PR（例如只改了注释导致的签名无关变化）加 `no-changelog` 标签。

### 实验性升为稳定

满足下面全部条件后，把符号加进 `api/stable/<包>.txt`（或把整包在 `api/tiers.txt` 改为 `stable`），作为单独的、可审查的变更：

- 连续一个 MINOR 版本没有不兼容变化；
- 有文档（参考站页面或包文档示例）；
- 有黄金测试或单元测试；
- 通过上面 R1–R12 的审查，尤其是零值、可比较性、返回类型；
- 它在签名里引用的符号也已经是稳定层（`apitxt -check` 没有新的跨层泄漏）。

## 发布前提（一次性）

- 仓库 Settings → Pages → Source 选 **GitHub Actions**（文档站由 `release.yml` 的 `docs` 作业部署）。
- Settings → Secrets and variables → Actions 添加 **`NPM_TOKEN`**：npm 上 `@tangoui` 组织的 Automation（或细粒度、可发布 `@tangoui/runtime`）令牌。没有它时 `npm` 作业会给出警告并跳过，其余照常。
- Settings → Code security 打开 **Private vulnerability reporting**（SECURITY.md 依赖它）。
- `.github/workflows/ci.yml` 里还有早期的 `release` / `publish` 作业（同样由 `v*` 标签触发、创建草稿 Release）。启用 `release.yml` 后应删除这两个作业，否则同一标签会出现两个草稿 Release。

## 发布清单

以发布 `0.2.0` 为例。

### 1. 准备

- [ ] `main` 上 CI 全绿（lint、三平台 test 与 smoke、mobile 作业）。
- [ ] 在 macOS 上手动跑 `bash scripts/smoke.sh` 和 `go run ./examples/showcase -smoke`；有条件的话在 Windows、Linux 实机上各跑一遍，并在发布说明里如实写明验证范围。
- [ ] `CHANGELOG.md`：把「未发布」里的内容移到新的 `## [0.2.0] - YYYY-MM-DD` 一节，更新文末的比较链接。
- [ ] `runtime-js/package.json` 的 `version` 改成 `0.2.0`，在 `runtime-js` 里 `npm install --package-lock-only` 同步 lock 文件，`npm test` 通过，提交更新后的 `runtimejs/tango.iife.js`（如有变化）。
- [ ] `cmd/tan` 模板里引用的 `@tangoui/runtime` 版本与本次一致（`tan init` 生成的 package.json）。
- [ ] 文档：新功能有对应页面；实验性功能有标注。`cd site && npm run docs:build && npm run docs:links` 通过。

### 2. 打根模块标签

```bash
git switch main && git pull
git tag -a v0.2.0 -m "Tango v0.2.0"
git push origin v0.2.0
```

推送 `v0.2.0` 触发 `.github/workflows/release.yml`：

| 作业 | 做什么 |
|---|---|
| `verify` | 标签格式；`runtime-js/package.json` 版本与标签一致；CHANGELOG 有 `## [0.2.0]`；`go vet` / `go test`（根模块与 cmd/tan）；`runtime-js` 的 `npm test` |
| `tan` | `CGO_ENABLED=0` 交叉编译 `tan`（darwin / linux / windows × amd64 / arm64），打成 `tan_0.2.0_<os>_<arch>.tar.gz` / `.zip`，附 `tan_0.2.0_checksums.txt` |
| `npm` | 有 `NPM_TOKEN` 时 `npm publish --provenance --access public` 发布 `@tangoui/runtime@0.2.0`（`prepublishOnly` 会先构建并测试）；该版本已存在则跳过 |
| `release` | 从 CHANGELOG 取本版本一节作为说明，附加 GitHub 自动生成的 PR 列表，创建**草稿** Release 并上传二进制；Release 已存在时只更新附件和说明 |
| `docs` | 正式版本才执行：在 Linux 上生成组件截图、构建文档站（含 API 参考）、检查死链，部署到 GitHub Pages |

### 3. 检查并发布 Release

- [ ] 打开 GitHub Releases 里的草稿，核对说明、附件和校验和，按 **Publish release**。
- [ ] npm：`npm view @tangoui/runtime@0.2.0`，页面上应显示 provenance。
- [ ] 文档站已更新。

### 4. 打 cmd/tan 子模块标签

`cmd/tan` 是独立模块。`cmd/tan/go.mod` 必须依赖一个可下载的根模块版本（正式标签或伪版本），不能用 `v0.0.0` 占位，也不能带 `replace`。本地开发由仓库根目录的 `go.work` 连接两个模块，无需把本地路径写进发布模块。根模块发布后，再把 CLI 的依赖更新到该版本：

```bash
cd cmd/tan
GOWORK=off GOPROXY=https://proxy.golang.org go get github.com/tangoui/tango@v0.2.0
GOWORK=off go mod tidy
cd ../..
git commit -s -am "cmd/tan: require github.com/tangoui/tango v0.2.0"
git tag -a cmd/tan/v0.2.0 -m "tan v0.2.0"
git push origin main cmd/tan/v0.2.0
```

- [ ] 验证：在一个空目录里 `go install github.com/tangoui/tango/cmd/tan@v0.2.0 && tan version`。
- [ ] 本地开发继续由仓库根目录的 `go.work` 把两个模块连在一起，不需要恢复 `replace`。

普通开发提交也必须保持 CLI 可独立构建：在 `cmd/tan` 目录执行 `GOWORK=off go build -mod=readonly .`。CI 会执行同一检查，并拒绝 CLI 模块里的 `replace`；发布校验也会将它视为错误。没有正式版本时，可以依赖已经推送、能被 Go 模块代理下载的根模块伪版本。本地修复尚未推送时，远程的 `go install …@latest` 仍会读取旧的 `go.mod`；可先在本地 `cmd/tan` 目录执行 `go install .`。

> `cmd/tan/v*` 标签不匹配 `v*` 触发条件，不会再跑一遍发布工作流。

### 5. 收尾

- [ ] `CHANGELOG.md` 顶部保留空的「未发布」一节。
- [ ] 需要的话在讨论区 / 社交渠道发布公告，链接到 Release 和文档站。

## 出问题时

- **`verify` 失败**：修正后删除并重打标签（`git push --delete origin v0.2.0 && git tag -d v0.2.0`），前提是 npm 还没发布这个版本。
- **npm 已发布但有问题**：npm 上的版本号不能复用。发布 `0.2.1` 修复，并 `npm deprecate @tangoui/runtime@0.2.0 "<原因>"`。Go 模块同理：已推送的标签不要移动（Go 校验和数据库已经记录），用新补丁版本修复，必要时在 `go.mod` 里用 `retract` 撤回。
- **只想重建附件**：Actions → Release → Run workflow，填已有标签；这时不会发布 npm、不会部署文档。
